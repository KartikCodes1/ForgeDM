// Package stats keeps speed history for the charts and daily totals for the
// dashboard.
package stats

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/KartikCodes1/forgedm/internal/engine"
)

// Day is the total for one calendar day.
type Day struct {
	Date    string `json:"date"`
	Bytes   int64  `json:"bytes"`
	Files   int    `json:"files"`
	SavedMs int64  `json:"savedMs"`
}

type series struct {
	step   time.Duration
	size   int
	values []float64
	acc    float64
	n      int
	start  time.Time
}

func (s *series) add(now time.Time, v float64) {
	if s.start.IsZero() {
		s.start = now
	}
	s.acc += v
	s.n++
	if now.Sub(s.start) >= s.step {
		s.values = append(s.values, s.acc/float64(s.n))
		if len(s.values) > s.size {
			s.values = s.values[len(s.values)-s.size:]
		}
		s.acc, s.n, s.start = 0, 0, now
	}
}

// Stats records speed samples and daily totals.
type Stats struct {
	mu        sync.Mutex
	path      string
	days      map[string]*Day
	lastTotal int64
	lastTick  time.Time
	ranges    map[string]*series
	peak      float64
	dirty     bool
}

// Snapshot is what the dashboard draws.
type Snapshot struct {
	Series map[string][]float64 `json:"series"`
	Week   []Day                `json:"week"`
	Today  Day                  `json:"today"`
	Yest   Day                  `json:"yesterday"`
	Peak   float64              `json:"peak"`
}

// Open loads saved daily totals.
func Open(path string) *Stats {
	s := &Stats{path: path, days: map[string]*Day{}, ranges: map[string]*series{
		"1m":  {step: time.Second, size: 60},
		"10m": {step: 10 * time.Second, size: 60},
		"1h":  {step: time.Minute, size: 60},
		"24h": {step: 15 * time.Minute, size: 96},
	}}
	if b, err := os.ReadFile(path); err == nil {
		var list []Day
		if json.Unmarshal(b, &list) == nil {
			for i := range list {
				d := list[i]
				s.days[d.Date] = &d
			}
		}
	}
	return s
}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

func (s *Stats) day(t time.Time) *Day {
	k := dayKey(t)
	d := s.days[k]
	if d == nil {
		d = &Day{Date: k}
		s.days[k] = d
	}
	return d
}

// Tick records the engine's byte counter; call it about once a second.
func (s *Stats) Tick(totalBytes int64, now time.Time) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastTick.IsZero() {
		s.lastTick, s.lastTotal = now, totalBytes
		return 0
	}
	dt := now.Sub(s.lastTick).Seconds()
	delta := totalBytes - s.lastTotal
	s.lastTick, s.lastTotal = now, totalBytes
	if dt <= 0 || delta < 0 {
		return 0
	}
	speed := float64(delta) / dt
	for _, r := range s.ranges {
		r.add(now, speed)
	}
	if speed > s.peak {
		s.peak = speed
	}
	if delta > 0 {
		s.day(now).Bytes += delta
		s.dirty = true
	}
	return speed
}

// Completed counts a finished download.
func (s *Stats) Completed(d engine.Download) {
	s.mu.Lock()
	defer s.mu.Unlock()
	day := s.day(time.Now())
	day.Files++
	day.SavedMs += d.SavedMs
	s.dirty = true
}

// Peak is the highest total speed seen this session.
func (s *Stats) Peak() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peak
}

// Snapshot returns chart data.
func (s *Stats) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	out := Snapshot{Series: map[string][]float64{}, Peak: s.peak}
	for k, r := range s.ranges {
		out.Series[k] = append([]float64(nil), r.values...)
	}
	for i := 6; i >= 0; i-- {
		t := now.AddDate(0, 0, -i)
		if d, ok := s.days[dayKey(t)]; ok {
			out.Week = append(out.Week, *d)
		} else {
			out.Week = append(out.Week, Day{Date: dayKey(t)})
		}
	}
	out.Today = out.Week[6]
	out.Yest = out.Week[5]
	return out
}

// Save writes daily totals (keeping 90 days).
func (s *Stats) Save() {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	cutoff := dayKey(time.Now().AddDate(0, 0, -90))
	var list []Day
	for k, d := range s.days {
		if k < cutoff {
			delete(s.days, k)
			continue
		}
		list = append(list, *d)
	}
	s.dirty = false
	s.mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].Date < list[j].Date })
	b, _ := json.Marshal(list)
	_ = engine.WriteFileAtomic(s.path, b)
}
