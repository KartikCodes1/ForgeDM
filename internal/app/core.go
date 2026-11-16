// Package app ties the engine, settings, rules, scheduler, browser API and
// post-download steps together, and exposes them to the user interface.
package app

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/KartikCodes1/forgedm/internal/api"
	"github.com/KartikCodes1/forgedm/internal/config"
	"github.com/KartikCodes1/forgedm/internal/engine"
	"github.com/KartikCodes1/forgedm/internal/platform"
	"github.com/KartikCodes1/forgedm/internal/stats"
)

// Version of ForgeDM.
const Version = "1.0.0"

// UI is what the core needs from the window toolkit (Wails in production).
type UI interface {
	Emit(event string, data any)
	Show()
	Hide()
	Notify(title, body string)
	ClipboardText() string
	ChooseFolder(title, start string) string
	OpenFile(title string, pattern string) string
	SaveFile(title, name string) string
	Quit()
}

// Core is the application service. It is not exposed to JavaScript directly;
// the App type in bridge.go is.
type Core struct {
	ui    UI
	cfg   *config.Store
	eng   *engine.Engine
	stats *stats.Stats
	api   *api.Server
	exe   string

	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	pairs       map[string]*pairRequest
	captures    map[string]*pendingCapture
	batches     map[string]api.Batch
	scans       map[string]context.CancelFunc
	clipLast    string
	clipSkip    map[string]bool
	runs        map[string]*queueRun
	wake        platform.Wake
	wakeAt      time.Time
	widget      *widgetProc
	speed       float64
	awake       bool
	stateTimer  *time.Timer
	lastSeenAt  map[string]time.Time
	detailsID   string
	startHidden bool
}

// Options configure NewCore.
type Options struct {
	DataDir     string
	StartHidden bool
}

// NewCore loads settings and downloads. Call Startup once the UI exists.
func NewCore(ui UI, opts Options) (*Core, error) {
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = platform.DataDir()
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	store, err := config.Open(dataDir, platform.DefaultDownloadsDir())
	if err != nil {
		return nil, err
	}
	exe, _ := os.Executable()
	c := &Core{
		ui:          ui,
		cfg:         store,
		stats:       stats.Open(filepath.Join(dataDir, "stats.json")),
		exe:         exe,
		pairs:       map[string]*pairRequest{},
		captures:    map[string]*pendingCapture{},
		batches:     map[string]api.Batch{},
		scans:       map[string]context.CancelFunc{},
		clipSkip:    map[string]bool{},
		runs:        map[string]*queueRun{},
		lastSeenAt:  map[string]time.Time{},
		startHidden: opts.StartHidden,
	}
	st := store.Get()
	eng, err := engine.New(c.engineConfig(st.Settings), filepath.Join(dataDir, "downloads.json"), hooks{c})
	if err != nil {
		return nil, err
	}
	c.eng = eng
	c.applySettings(st)
	return c, nil
}

func (c *Core) engineConfig(s config.Settings) engine.Config {
	ec := engine.DefaultConfig()
	ec.MaxActive = s.MaxActive
	ec.DefaultConns = s.DefaultConnections
	if s.RetryCount > 0 {
		ec.RetryCount = s.RetryCount
	}
	ec.SmartPriority = s.SmartPriority
	if s.UserAgent != "" {
		ec.UserAgent = s.UserAgent
	}
	ec.Proxy = platform.ProxyFunc(s.ProxyMode, s.ProxyURL)
	return ec
}

// applySettings pushes settings into the engine and OS integration.
func (c *Core) applySettings(st config.State) {
	s := st.Settings
	c.eng.SetConfig(c.engineConfig(s))
	if s.SpeedLimitOn {
		c.eng.SetGlobalLimit(s.SpeedLimit)
	} else {
		c.eng.SetGlobalLimit(0)
	}
	if c.exe != "" {
		_ = platform.SetStartup(s.StartWithSystem, c.exe)
	}
	c.applyGates(st, time.Now())
}

// Startup starts the engine, the browser connection and background loops.
func (c *Core) Startup(ctx context.Context) {
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.eng.Start()
	st := c.cfg.Get()
	if srv, err := api.Start(apiHooks{c}, st.Settings.APIPort); err == nil {
		c.api = srv
	} else {
		c.toast("error", "The browser connection couldn't start: "+err.Error())
	}
	go c.loop()
	if st.Settings.DropTarget {
		c.setWidget(true)
	}
}

// Shutdown stops everything and saves.
func (c *Core) Shutdown() {
	if c.cancel != nil {
		c.cancel()
	}
	c.setWidget(false)
	c.api.Close()
	c.eng.Close()
	c.stats.Save()
	platform.KeepAwake(false)
	c.mu.Lock()
	if c.wake != nil {
		c.wake.Cancel()
	}
	c.mu.Unlock()
}

// StartHidden reports whether the app was launched minimized to the tray.
func (c *Core) StartHidden() bool { return c.startHidden }

// Engine exposes the engine (used by the tray menu).
func (c *Core) Engine() *engine.Engine { return c.eng }

func (c *Core) loop() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	n := 0
	for {
		select {
		case <-c.ctx.Done():
			return
		case now := <-tick.C:
			n++
			c.tick(now)
			if n%2 == 0 {
				c.checkClipboard()
			}
			if n%15 == 1 {
				c.schedule(now)
			}
			if n%60 == 0 {
				c.stats.Save()
			}
		}
	}
}

// TickPayload is sent to the UI every second.
type TickPayload struct {
	Live   []engine.Live  `json:"live"`
	Speed  float64        `json:"speed"`
	Active int            `json:"active"`
	Stats  stats.Snapshot `json:"stats"`
}

func (c *Core) tick(now time.Time) {
	speed := c.stats.Tick(c.eng.TotalBytes(), now)
	c.mu.Lock()
	c.speed = speed
	details := c.detailsID
	c.mu.Unlock()
	live := c.eng.Live(details)
	if live == nil {
		live = []engine.Live{}
	}
	c.ui.Emit("tick", TickPayload{Live: live, Speed: speed, Active: len(live), Stats: c.stats.Snapshot()})
	st := c.cfg.Get()
	want := st.Settings.KeepAwake && len(live) > 0
	c.mu.Lock()
	changed := want != c.awake
	c.awake = want
	c.mu.Unlock()
	if changed {
		platform.KeepAwake(want)
	}
}

// ---- snapshot for the UI ----

// QueueView is a queue plus its live state.
type QueueView struct {
	config.Queue
	Active  bool      `json:"active"`
	NextRun time.Time `json:"nextRun,omitempty"`
	Count   int       `json:"count"`
	Bytes   int64     `json:"bytes"`
}

// CategoryView summarizes a category.
type CategoryView struct {
	Name  string   `json:"name"`
	Count int      `json:"count"`
	Bytes int64    `json:"bytes"`
	Dir   string   `json:"dir"`
	Exts  []string `json:"exts"`
}

// BrowserView is a paired browser without its secret.
type BrowserView struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	PairedAt time.Time `json:"pairedAt"`
	LastSeen time.Time `json:"lastSeen"`
}

// Snapshot is the full state the UI renders.
type Snapshot struct {
	Downloads  []engine.Download `json:"downloads"`
	Settings   config.Settings   `json:"settings"`
	Rules      []config.Rule     `json:"rules"`
	Queues     []QueueView       `json:"queues"`
	Projects   []config.Project  `json:"projects"`
	Categories []CategoryView    `json:"categories"`
	Browsers   []BrowserView     `json:"browsers"`
	Stats      stats.Snapshot    `json:"stats"`
	DiskFree   uint64            `json:"diskFree"`
	DiskTotal  uint64            `json:"diskTotal"`
	Port       int               `json:"port"`
	Version    string            `json:"version"`
	DataDir    string            `json:"dataDir"`
}

func (c *Core) snapshot() Snapshot {
	st := c.cfg.Get()
	list := c.eng.List()
	for i := range list {
		list[i].Segments = nil
		if list[i].HLS != nil {
			h := *list[i].HLS
			h.Parts = nil
			list[i].HLS = &h
		}
		list[i].Headers = nil
	}
	s := Snapshot{
		Downloads: list,
		Settings:  st.Settings,
		Rules:     st.Rules,
		Projects:  st.Projects,
		Stats:     c.stats.Snapshot(),
		Version:   Version,
		DataDir:   filepath.Dir(c.cfg.Path()),
	}
	if s.Downloads == nil {
		s.Downloads = []engine.Download{}
	}
	for _, b := range st.Settings.Browsers {
		s.Browsers = append(s.Browsers, BrowserView{ID: b.ID, Name: b.Name, PairedAt: b.PairedAt, LastSeen: b.LastSeen})
	}
	s.Settings.Browsers = nil
	s.Settings.InternalToken = ""
	if c.api != nil {
		s.Port = c.api.Port
	}
	s.DiskFree, s.DiskTotal = platform.DiskSpace(st.Settings.BaseDir)

	cats := map[string]*CategoryView{}
	for _, name := range config.Categories {
		cats[name] = &CategoryView{Name: name, Dir: st.Settings.DirFor(name), Exts: config.CategoryExtensions()[name]}
	}
	qcount := map[string]*QueueView{}
	for _, q := range st.Queues {
		qcount[q.ID] = &QueueView{Queue: q}
	}
	for _, d := range list {
		cv := cats[d.Category]
		if cv == nil {
			cv = cats["Other"]
		}
		cv.Count++
		if d.Size > 0 {
			cv.Bytes += d.Size
		}
		if qv := qcount[d.Queue]; qv != nil && d.Status != engine.StatusCompleted {
			qv.Count++
			if d.Size > 0 {
				qv.Bytes += d.Size - d.Downloaded
			}
		}
	}
	for _, name := range config.Categories {
		s.Categories = append(s.Categories, *cats[name])
	}
	now := time.Now()
	c.mu.Lock()
	for _, q := range st.Queues {
		qv := qcount[q.ID]
		if r := c.runs[q.ID]; r != nil {
			qv.Active = r.active
		}
		if q.ID == "main" {
			qv.Active = q.Enabled
		} else {
			qv.NextRun = nextStart(q, now)
		}
		s.Queues = append(s.Queues, *qv)
	}
	c.mu.Unlock()
	return s
}

// pushState sends the full state to the UI, coalescing rapid changes.
func (c *Core) pushState() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stateTimer != nil {
		return
	}
	c.stateTimer = time.AfterFunc(150*time.Millisecond, func() {
		c.mu.Lock()
		c.stateTimer = nil
		c.mu.Unlock()
		c.ui.Emit("state", c.snapshot())
	})
}

// toast shows a short message inside the app.
func (c *Core) toast(kind, text string) {
	c.ui.Emit("toast", map[string]string{"kind": kind, "text": text})
}

// ---- engine hooks ----

type hooks struct{ c *Core }

func (h hooks) Changed(engine.Download) { h.c.pushState() }
func (h hooks) Removed(string)          { h.c.pushState() }
func (h hooks) Completed(d engine.Download) {
	go h.c.finish(d)
}

// ---- helpers ----

func (c *Core) existingByURL() map[string]engine.Download {
	m := map[string]engine.Download{}
	for _, d := range c.eng.List() {
		m[normalizeURL(d.URL)] = d
	}
	return m
}

func normalizeURL(u string) string {
	u = strings.TrimSpace(u)
	if i := strings.Index(u, "#"); i >= 0 {
		u = u[:i]
	}
	return strings.TrimSuffix(u, "/")
}

func headersFrom(cookies, referrer, ua string, extra map[string]string) map[string]string {
	h := map[string]string{}
	for k, v := range extra {
		h[k] = v
	}
	if cookies != "" {
		h["Cookie"] = cookies
	}
	if referrer != "" {
		h["Referer"] = referrer
	}
	if ua != "" {
		h["User-Agent"] = ua
	}
	if len(h) == 0 {
		return nil
	}
	return h
}

func sortedTags(list []engine.Download, extra []string) []string {
	set := map[string]bool{}
	for _, t := range extra {
		set[t] = true
	}
	for _, d := range list {
		for _, t := range d.Tags {
			set[t] = true
		}
	}
	var out []string
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func (c *Core) httpClient() *http.Client {
	st := c.cfg.Get()
	proxy := platform.ProxyFunc(st.Settings.ProxyMode, st.Settings.ProxyURL)
	return &http.Client{Timeout: 45 * time.Second, Transport: &http.Transport{Proxy: proxy, MaxIdleConnsPerHost: 8}}
}

// CloseToTray reports whether closing the window should keep ForgeDM running.
func (c *Core) CloseToTray() bool { return c.cfg.Get().Settings.CloseToTray }

// TrayInfo is shown in the tray tooltip.
type TrayInfo struct {
	Speed  string
	Active int
}

// TrayStatus summarizes activity for the tray icon.
func (c *Core) TrayStatus() TrayInfo {
	c.mu.Lock()
	speed := c.speed
	c.mu.Unlock()
	return TrayInfo{Speed: humanBytes(int64(speed)) + "/s", Active: len(c.eng.Live(""))}
}
