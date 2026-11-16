package app

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/KartikCodes1/forgedm/internal/config"
	"github.com/KartikCodes1/forgedm/internal/engine"
	"github.com/KartikCodes1/forgedm/internal/grabber"
	"github.com/KartikCodes1/forgedm/internal/platform"
)

// queueRun is the runtime state of a scheduled queue.
type queueRun struct {
	active    bool
	manual    bool
	startedAt time.Time
	idleSince time.Time
	hadWork   bool
	doneDay   string
	retried   bool
}

func parseClock(s string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

func dayAllowed(q config.Queue, wd time.Weekday) bool {
	if len(q.Days) == 0 {
		return true
	}
	for _, d := range q.Days {
		if d == int(wd) {
			return true
		}
	}
	return false
}

func at(day time.Time, h, m int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, day.Location())
}

// inWindow reports whether a scheduled queue should be running at now.
// doneDay is the date (YYYY-MM-DD) on which an open-ended run already finished.
func inWindow(q config.Queue, now time.Time, doneDay string) bool {
	if !q.Enabled {
		return false
	}
	sh, sm, ok := parseClock(q.Start)
	if !ok {
		return false
	}
	start := at(now, sh, sm)
	eh, em, hasStop := parseClock(q.Stop)
	if !hasStop {
		// Runs from the start time until the queue is empty.
		return dayAllowed(q, now.Weekday()) && !now.Before(start) && doneDay != now.Format("2006-01-02")
	}
	end := at(now, eh, em)
	if end.After(start) {
		return dayAllowed(q, now.Weekday()) && !now.Before(start) && now.Before(end)
	}
	// Overnight window, e.g. 23:00–06:00.
	yesterday := now.AddDate(0, 0, -1)
	return (dayAllowed(q, now.Weekday()) && !now.Before(start)) || (dayAllowed(q, yesterday.Weekday()) && now.Before(end))
}

// nextStart returns the next time a scheduled queue opens.
func nextStart(q config.Queue, now time.Time) time.Time {
	sh, sm, ok := parseClock(q.Start)
	if !ok || !q.Enabled || q.ID == "main" {
		return time.Time{}
	}
	for i := 0; i < 8; i++ {
		day := now.AddDate(0, 0, i)
		t := at(day, sh, sm)
		if t.After(now) && dayAllowed(q, t.Weekday()) {
			return t
		}
	}
	return time.Time{}
}

// applyGates tells the engine which queues may run.
func (c *Core) applyGates(st config.State, now time.Time) {
	peak := c.stats.Peak()
	metered := -1
	isMetered := func() bool {
		if metered < 0 {
			metered = 0
			if platform.IsMetered() {
				metered = 1
			}
		}
		return metered == 1
	}
	for _, q := range st.Queues {
		if q.ID == "main" {
			c.eng.SetGate("main", engine.Gate{Active: q.Enabled, Concurrent: st.Settings.MaxActive, Limit: q.SpeedLimit, Order: q.Order})
			continue
		}
		c.mu.Lock()
		r := c.runs[q.ID]
		if r == nil {
			r = &queueRun{}
			c.runs[q.ID] = r
		}
		want := r.manual || inWindow(q, now, r.doneDay)
		c.mu.Unlock()
		if want && q.PauseOnMetered && isMetered() {
			want = false
		}
		limit := q.SpeedLimit
		if q.HalfBandwidth && peak > 0 {
			limit = int64(peak / 2)
		}
		conc := q.Concurrent
		if conc <= 0 {
			conc = 1
		}
		c.eng.SetGate(q.ID, engine.Gate{Active: want, Concurrent: conc, Limit: limit, Order: q.Order})
		c.mu.Lock()
		if want && !r.active {
			r.active, r.startedAt, r.hadWork, r.idleSince, r.retried = true, now, false, time.Time{}, false
		} else if !want && r.active {
			r.active, r.manual = false, false
		}
		c.mu.Unlock()
	}
}

// schedule runs every few seconds: opens and closes queues, retries failed
// downloads, runs "when finished" actions, site syncs and wake timers.
func (c *Core) schedule(now time.Time) {
	st := c.cfg.Get()
	c.applyGates(st, now)
	list := c.eng.List()

	for _, q := range st.Queues {
		if q.ID == "main" {
			continue
		}
		c.mu.Lock()
		r := c.runs[q.ID]
		c.mu.Unlock()
		if r == nil || !r.active {
			continue
		}
		// Retry failed downloads once per run.
		if q.RetryFailed && !r.retried {
			var ids []string
			for _, d := range list {
				if d.Queue == q.ID && d.Status == engine.StatusFailed && d.ErrorCode == engine.ErrCodeNetwork {
					ids = append(ids, d.ID)
				}
			}
			if len(ids) > 0 {
				c.eng.Enqueue(ids...)
			}
			c.mu.Lock()
			r.retried = true
			c.mu.Unlock()
		}
		idle := c.eng.QueueIdle(q.ID)
		c.mu.Lock()
		if !idle {
			r.hadWork = true
			r.idleSince = time.Time{}
			c.mu.Unlock()
			continue
		}
		if r.idleSince.IsZero() {
			r.idleSince = now
			c.mu.Unlock()
			continue
		}
		finished := r.hadWork && now.Sub(r.idleSince) > 20*time.Second
		emptyOpenEnded := !r.hadWork && now.Sub(r.idleSince) > 20*time.Second && q.Stop == ""
		if finished || emptyOpenEnded {
			r.doneDay = now.Format("2006-01-02")
			r.manual = false
			r.hadWork = false
		}
		c.mu.Unlock()
		if finished {
			c.queueFinished(q)
		}
	}
	c.syncProjects(st, now)
	c.armWake(st, now)
	c.pushState()
}

func (c *Core) queueFinished(q config.Queue) {
	switch q.WhenDone {
	case "shutdown":
		c.notify("Queue finished", q.Name+" is done. Shutting down in 1 minute.")
		c.eng.Close()
		_ = platform.Shutdown()
	case "sleep":
		c.notify("Queue finished", q.Name+" is done. Going to sleep.")
		_ = platform.Sleep()
	case "hibernate":
		c.notify("Queue finished", q.Name+" is done. Hibernating.")
		_ = platform.Hibernate()
	case "exit":
		c.notify("Queue finished", q.Name+" is done. Closing ForgeDM.")
		c.ui.Quit()
	default:
		c.notify("Queue finished", q.Name+" is done.")
	}
}

// armWake sets a wake timer for the next queue that wants to wake the PC.
func (c *Core) armWake(st config.State, now time.Time) {
	var next time.Time
	for _, q := range st.Queues {
		if !q.WakeFromSleep || !q.Enabled {
			continue
		}
		if t := nextStart(q, now); !t.IsZero() && (next.IsZero() || t.Before(next)) {
			next = t
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if next.Equal(c.wakeAt) {
		return
	}
	if c.wake != nil {
		c.wake.Cancel()
		c.wake = nil
	}
	c.wakeAt = next
	if !next.IsZero() {
		c.wake = platform.ScheduleWake(next.Add(-time.Minute))
	}
}

// RunQueueNow starts a queue immediately, ignoring its schedule.
func (c *Core) RunQueueNow(id string) {
	c.mu.Lock()
	r := c.runs[id]
	if r == nil {
		r = &queueRun{}
		c.runs[id] = r
	}
	r.manual = true
	r.doneDay = ""
	c.mu.Unlock()
	c.applyGates(c.cfg.Get(), time.Now())
	c.pushState()
}

// StopQueue closes a queue until its next scheduled time.
func (c *Core) StopQueue(id string) {
	c.mu.Lock()
	if r := c.runs[id]; r != nil {
		r.manual = false
		r.doneDay = time.Now().Format("2006-01-02")
	}
	c.mu.Unlock()
	st := c.cfg.Get()
	for _, q := range st.Queues {
		if q.ID == id && q.ID != "main" {
			c.eng.SetGate(id, engine.Gate{Active: false, Concurrent: q.Concurrent})
		}
	}
	c.mu.Lock()
	if r := c.runs[id]; r != nil {
		r.active = false
	}
	c.mu.Unlock()
	c.pushState()
}

// syncProjects re-runs Site Grabber projects that sync on a schedule.
func (c *Core) syncProjects(st config.State, now time.Time) {
	for _, p := range st.Projects {
		if !p.SyncEnabled {
			continue
		}
		h, m, ok := parseClock(p.SyncTime)
		if !ok || int(now.Weekday()) != p.SyncDay {
			continue
		}
		due := at(now, h, m)
		if now.Before(due) || now.Sub(due) > 6*time.Hour || p.LastRun.After(due) {
			continue
		}
		go c.runSync(p)
	}
}

func (c *Core) runSync(p config.Project) {
	_, _ = c.cfg.Update(func(st *config.State) {
		for i := range st.Projects {
			if st.Projects[i].ID == p.ID {
				st.Projects[i].LastRun = time.Now()
			}
		}
	})
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Minute)
	defer cancel()
	st := c.cfg.Get()
	items, err := grabber.Scan(ctx, c.httpClient(), grabber.Options{URL: p.URL, Depth: p.Depth, SameSite: p.SameSite, UserAgent: st.Settings.UserAgent}, nil)
	if err != nil && len(items) == 0 {
		c.notify("Site sync failed", p.Name+": "+err.Error())
		return
	}
	items = filterItems(items, p.Types, p.Exts, p.MinSize)
	have := c.existingByURL()
	var fresh []grabber.Item
	for _, it := range items {
		if _, ok := have[normalizeURL(it.URL)]; !ok {
			fresh = append(fresh, it)
		}
	}
	n, _ := c.addGrabbed(fresh, p.Dir, p.KeepStructure, "main", true, "sync")
	_, _ = c.cfg.Update(func(st *config.State) {
		for i := range st.Projects {
			if st.Projects[i].ID == p.ID {
				st.Projects[i].LastFound = n
			}
		}
	})
	if n > 0 {
		c.notify("Site sync", p.Name+": "+strconv.Itoa(n)+" new files are downloading.")
	}
	c.pushState()
}
