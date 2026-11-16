package engine

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// Config tunes the engine. It can be changed at runtime with SetConfig.
type Config struct {
	MaxActive     int           // downloads the main queue runs at once
	DefaultConns  int           // 0 = automatic by file size
	MaxConns      int           // hard cap per download
	UserAgent     string        // used when the browser didn't send one
	Proxy         func(*http.Request) (*url.URL, error)
	RetryCount    int           // retries per connection before giving up
	RetryDelay    time.Duration // base delay, doubled per attempt
	SmartPriority bool          // start small files first
	MinSplit      int64         // smallest piece worth a new connection
	IdleTimeout   time.Duration // reconnect when a connection goes quiet
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxActive:     4,
		MaxConns:      32,
		UserAgent:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36 ForgeDM/1.0",
		RetryCount:    5,
		RetryDelay:    2 * time.Second,
		SmartPriority: true,
		MinSplit:      1 << 20,
		IdleTimeout:   30 * time.Second,
	}
}

// Listener receives engine notifications. Calls may come from any goroutine.
type Listener interface {
	Changed(d Download)
	Removed(id string)
	Completed(d Download)
}

// Gate controls when a queue may start downloads.
type Gate struct {
	Active     bool
	Concurrent int
	Limit      int64 // bytes/s shared by the queue, 0 = none
	Order      []string
}

type gateState struct {
	Gate
	limiter *rate.Limiter
}

// Engine owns all downloads.
type Engine struct {
	mu     sync.Mutex
	cfg    Config
	tasks  map[string]*task
	order  []string
	gates  map[string]*gateState
	store  *Store
	client *http.Client
	global *rate.Limiter
	lis    Listener

	totalBytes atomic.Int64
	dirty      atomic.Bool
	pumpCh     chan struct{}
	stopCh     chan struct{}
	wg         sync.WaitGroup
	started    bool
}

// New loads saved downloads and prepares the engine. Call Start to begin.
func New(cfg Config, storePath string, lis Listener) (*Engine, error) {
	if lis == nil {
		lis = nopListener{}
	}
	e := &Engine{
		cfg:    cfg,
		tasks:  map[string]*task{},
		gates:  map[string]*gateState{"main": {Gate: Gate{Active: true, Concurrent: cfg.MaxActive}}},
		store:  &Store{Path: storePath},
		global: rate.NewLimiter(rate.Inf, 0),
		lis:    lis,
		pumpCh: make(chan struct{}, 1),
		stopCh: make(chan struct{}),
	}
	e.client = e.newClient()
	items, err := e.store.Load()
	if err != nil {
		return nil, err
	}
	for _, d := range items {
		if d.Status == StatusDownloading {
			// The app stopped unexpectedly mid-download: pick it up again.
			d.Status = StatusQueued
			d.Manual = true
		}
		for _, s := range d.Segments {
			s.active = false
		}
		e.tasks[d.ID] = &task{d: d}
		e.order = append(e.order, d.ID)
	}
	return e, nil
}

type nopListener struct{}

func (nopListener) Changed(Download)   {}
func (nopListener) Removed(string)     {}
func (nopListener) Completed(Download) {}

func (e *Engine) newClient() *http.Client {
	proxy := e.cfg.Proxy
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}
	tr := &http.Transport{
		Proxy:                 proxy,
		DialContext:           (&net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   64,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableCompression:    true,
		// Separate TCP connections are what make segmented downloads fast;
		// HTTP/2 would multiplex every segment over a single connection.
		TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
	}
	return &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 15 {
				return errors.New("too many redirects")
			}
			// Keep our headers (cookies, referer, range) across redirects.
			for k, v := range via[0].Header {
				if _, ok := req.Header[k]; !ok {
					req.Header[k] = v
				}
			}
			return nil
		},
	}
}

// Start launches the scheduler, speed meter and autosave loops.
func (e *Engine) Start() {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return
	}
	e.started = true
	e.mu.Unlock()
	e.wg.Add(2)
	go e.loop()
	go e.saver()
	e.kick()
}

// Close stops all transfers (leaving them paused) and saves state.
func (e *Engine) Close() {
	e.mu.Lock()
	running := e.runningLocked()
	e.mu.Unlock()
	for _, t := range running {
		e.stopTask(t, StatusPaused)
	}
	select {
	case <-e.stopCh:
	default:
		close(e.stopCh)
	}
	e.wg.Wait()
	e.save()
}

// SetConfig replaces the configuration (connections, retries, proxy...).
func (e *Engine) SetConfig(cfg Config) {
	e.mu.Lock()
	proxyChanged := fmt.Sprintf("%p", cfg.Proxy) != fmt.Sprintf("%p", e.cfg.Proxy)
	e.cfg = cfg
	if g := e.gates["main"]; g != nil {
		g.Concurrent = cfg.MaxActive
	}
	if proxyChanged {
		e.client = e.newClient()
	}
	e.mu.Unlock()
	e.kick()
}

// SetGlobalLimit caps total speed in bytes per second (0 = unlimited).
func (e *Engine) SetGlobalLimit(bps int64) {
	setLimit(e.global, bps)
}

func setLimit(l *rate.Limiter, bps int64) {
	if bps <= 0 {
		l.SetLimit(rate.Inf)
		l.SetBurst(0)
		return
	}
	l.SetLimit(rate.Limit(bps))
	burst := int(bps)
	if burst < readChunk {
		burst = readChunk
	}
	if burst > 4<<20 {
		burst = 4 << 20
	}
	l.SetBurst(burst)
}

// SetGate configures a queue: whether it may run now, how many at once and
// its shared speed limit.
func (e *Engine) SetGate(queue string, g Gate) {
	e.mu.Lock()
	gs := e.gates[queue]
	if gs == nil {
		gs = &gateState{limiter: rate.NewLimiter(rate.Inf, 0)}
		e.gates[queue] = gs
	}
	if gs.limiter == nil {
		gs.limiter = rate.NewLimiter(rate.Inf, 0)
	}
	if queue == "main" && g.Concurrent == 0 {
		g.Concurrent = e.cfg.MaxActive
	}
	wasActive := gs.Active
	gs.Gate = g
	setLimit(gs.limiter, g.Limit)
	// A queue that just closed stops the downloads it started (not manual ones).
	var stop []*task
	if wasActive && !g.Active {
		for _, t := range e.tasks {
			t.mu.Lock()
			if t.run != nil && t.d.Queue == queue && !t.d.Manual {
				stop = append(stop, t)
			}
			t.mu.Unlock()
		}
	}
	e.mu.Unlock()
	for _, t := range stop {
		e.stopTask(t, StatusQueued)
	}
	e.kick()
}

// QueueIdle reports whether a queue has nothing left to download.
func (e *Engine) QueueIdle(queue string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, t := range e.tasks {
		t.mu.Lock()
		busy := t.d.Queue == queue && (t.run != nil || t.d.Status == StatusQueued || t.d.Status == StatusDownloading)
		t.mu.Unlock()
		if busy {
			return false
		}
	}
	return true
}

// runningLocked lists tasks with an active transfer. Caller holds e.mu.
func (e *Engine) runningLocked() []*task {
	var out []*task
	for _, id := range e.order {
		t := e.tasks[id]
		t.mu.Lock()
		if t.run != nil {
			out = append(out, t)
		}
		t.mu.Unlock()
	}
	return out
}

func (t *task) running() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.run != nil
}

// TotalBytes is the number of bytes received since the engine started.
func (e *Engine) TotalBytes() int64 { return e.totalBytes.Load() }

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// AutoConnections picks a connection count for a file size.
func AutoConnections(size int64) int {
	switch {
	case size <= 0:
		return 8
	case size < 2<<20:
		return 1
	case size < 20<<20:
		return 4
	case size < 200<<20:
		return 8
	default:
		return 16
	}
}

// Add registers a new download. With req.Start it begins immediately,
// otherwise it waits in its queue.
func (e *Engine) Add(req Request) (Download, error) {
	if strings.TrimSpace(req.URL) == "" {
		return Download{}, errors.New("missing link")
	}
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Download{}, errors.New("only http and https links can be downloaded")
	}
	if req.Dir == "" {
		return Download{}, errors.New("missing save folder")
	}
	if err := os.MkdirAll(req.Dir, 0o755); err != nil {
		return Download{}, fmt.Errorf("can't create folder: %w", err)
	}
	kind := req.Kind
	if kind == "" {
		kind = KindFile
	}
	queue := req.Queue
	if queue == "" {
		queue = "main"
	}
	d := &Download{
		ID:          newID(),
		URL:         u.String(),
		PageURL:     req.PageURL,
		Kind:        kind,
		Filename:    SanitizeFilename(req.Filename),
		NameFixed:   req.Filename != "",
		Dir:         req.Dir,
		Size:        -1,
		Status:      StatusQueued,
		Category:    req.Category,
		Tags:        req.Tags,
		Queue:       queue,
		Connections: req.Connections,
		SpeedLimit:  req.SpeedLimit,
		Headers:     req.Headers,
		Post:        req.Post,
		Source:      req.Source,
		Rule:        req.Rule,
		CreatedAt:   time.Now(),
		Manual:      req.Start,
	}
	if req.Size > 0 {
		d.Size = req.Size
	}
	if req.Filename == "" {
		d.Filename = SanitizeFilename(FilenameFromURL(d.URL))
	}
	if req.Checksum != "" {
		d.Checksum.Expected = strings.ToLower(strings.TrimSpace(req.Checksum))
		d.Checksum.Algo = algoForHash(d.Checksum.Expected)
		d.Post.Verify = true
	}
	e.mu.Lock()
	reserved := e.reservedPathsLocked(nil)
	d.Filename = UniqueName(d.Dir, d.Filename, reserved)
	e.tasks[d.ID] = &task{d: d}
	e.order = append(e.order, d.ID)
	snap := d.Snapshot()
	e.mu.Unlock()
	e.markDirty()
	e.lis.Changed(snap)
	e.kick()
	return snap, nil
}

func algoForHash(h string) string {
	switch len(h) {
	case 32:
		return "md5"
	case 40:
		return "sha1"
	case 128:
		return "sha512"
	default:
		return "sha256"
	}
}

// reservedPathsLocked lists target paths of unfinished downloads so two
// downloads never write the same file. Caller holds e.mu and must not hold
// the mutex of any task other than `except`.
func (e *Engine) reservedPathsLocked(except *task) map[string]bool {
	m := make(map[string]bool, len(e.tasks))
	for _, t := range e.tasks {
		if t == except {
			continue
		}
		t.mu.Lock()
		if t.d.Status != StatusCompleted {
			m[strings.ToLower(t.d.Path())] = true
		}
		t.mu.Unlock()
	}
	return m
}

// reservedPaths is reservedPathsLocked for callers that hold no locks.
func (e *Engine) reservedPaths(except *task) map[string]bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.reservedPathsLocked(except)
}

// List returns every download in the order they were added.
func (e *Engine) List() []Download {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Download, 0, len(e.order))
	for _, id := range e.order {
		if t, ok := e.tasks[id]; ok {
			t.mu.Lock()
			out = append(out, t.d.Snapshot())
			t.mu.Unlock()
		}
	}
	return out
}

// Get returns one download.
func (e *Engine) Get(id string) (Download, bool) {
	e.mu.Lock()
	t, ok := e.tasks[id]
	e.mu.Unlock()
	if !ok {
		return Download{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.d.Snapshot(), true
}

// Update changes editable fields (tags, queue, limits, post actions...).
func (e *Engine) Update(id string, fn func(d *Download)) (Download, error) {
	e.mu.Lock()
	t, ok := e.tasks[id]
	e.mu.Unlock()
	if !ok {
		return Download{}, errors.New("download not found")
	}
	t.mu.Lock()
	fn(t.d)
	if t.run != nil {
		setLimit(t.run.limiter, t.d.SpeedLimit)
	}
	snap := t.d.Snapshot()
	t.mu.Unlock()
	e.markDirty()
	e.lis.Changed(snap)
	e.kick()
	return snap, nil
}

// Resume starts (or restarts) downloads right away, bypassing queue timing.
func (e *Engine) Resume(ids ...string) {
	for _, id := range ids {
		e.mu.Lock()
		t, ok := e.tasks[id]
		e.mu.Unlock()
		if !ok {
			continue
		}
		t.mu.Lock()
		if t.d.Status == StatusCompleted || t.run != nil {
			t.mu.Unlock()
			continue
		}
		t.d.Status = StatusQueued
		t.d.Manual = true
		t.d.Error, t.d.ErrorCode = "", ""
		snap := t.d.Snapshot()
		t.mu.Unlock()
		e.lis.Changed(snap)
	}
	e.markDirty()
	e.kick()
}

// Enqueue puts downloads back in their queue to wait for its schedule.
func (e *Engine) Enqueue(ids ...string) {
	for _, id := range ids {
		e.mu.Lock()
		t, ok := e.tasks[id]
		e.mu.Unlock()
		if !ok {
			continue
		}
		if t.running() {
			e.stopTask(t, StatusQueued)
		}
		t.mu.Lock()
		if t.d.Status != StatusCompleted {
			t.d.Status = StatusQueued
			t.d.Manual = false
		}
		snap := t.d.Snapshot()
		t.mu.Unlock()
		e.lis.Changed(snap)
	}
	e.markDirty()
	e.kick()
}

// Pause stops downloads and keeps their progress.
func (e *Engine) Pause(ids ...string) {
	for _, id := range ids {
		e.mu.Lock()
		t, ok := e.tasks[id]
		e.mu.Unlock()
		if !ok {
			continue
		}
		if t.running() {
			e.stopTask(t, StatusPaused)
			continue
		}
		t.mu.Lock()
		if t.d.Status == StatusQueued || t.d.Status == StatusDownloading {
			t.d.Status = StatusPaused
			t.d.Manual = false
		}
		snap := t.d.Snapshot()
		t.mu.Unlock()
		e.lis.Changed(snap)
	}
	e.markDirty()
}

// PauseAll pauses everything that is running or waiting.
func (e *Engine) PauseAll() {
	e.Pause(e.ids(func(d *Download) bool {
		return d.Status == StatusDownloading || d.Status == StatusQueued
	})...)
}

// ResumeAll restarts every paused or failed download.
func (e *Engine) ResumeAll() {
	e.Resume(e.ids(func(d *Download) bool {
		return d.Status == StatusPaused || d.Status == StatusFailed
	})...)
}

func (e *Engine) ids(match func(d *Download) bool) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, id := range e.order {
		t := e.tasks[id]
		t.mu.Lock()
		if match(t.d) {
			out = append(out, id)
		}
		t.mu.Unlock()
	}
	return out
}

// Remove deletes a download from the list, optionally deleting its file.
func (e *Engine) Remove(id string, deleteFile bool) error {
	e.mu.Lock()
	t, ok := e.tasks[id]
	e.mu.Unlock()
	if !ok {
		return nil
	}
	if t.running() {
		e.stopTask(t, StatusPaused)
	}
	t.mu.Lock()
	d := t.d
	t.mu.Unlock()
	_ = os.Remove(d.TempPath())
	_ = os.RemoveAll(d.Path() + ".forgedm.parts")
	if deleteFile && d.Status == StatusCompleted {
		_ = os.Remove(d.Path())
	}
	e.mu.Lock()
	delete(e.tasks, id)
	for i, x := range e.order {
		if x == id {
			e.order = append(e.order[:i], e.order[i+1:]...)
			break
		}
	}
	e.mu.Unlock()
	e.markDirty()
	e.lis.Removed(id)
	return nil
}

// Redownload starts a finished or failed download again from scratch.
func (e *Engine) Redownload(id string) error {
	e.mu.Lock()
	t, ok := e.tasks[id]
	e.mu.Unlock()
	if !ok {
		return errors.New("download not found")
	}
	if t.running() {
		e.stopTask(t, StatusPaused)
	}
	t.mu.Lock()
	_ = os.Remove(t.d.TempPath())
	t.d.Segments = nil
	t.d.Downloaded = 0
	t.d.Status = StatusQueued
	t.d.Manual = true
	t.d.Error, t.d.ErrorCode, t.d.PostNote, t.d.PostState = "", "", "", ""
	t.d.Checksum.Actual, t.d.Checksum.Verified, t.d.Checksum.Mismatch = "", false, false
	if t.d.HLS != nil {
		t.d.HLS = nil
	}
	snap := t.d.Snapshot()
	t.mu.Unlock()
	e.markDirty()
	e.lis.Changed(snap)
	e.kick()
	return nil
}

// RefreshLink points a download at a new address (for expired links) while
// keeping the data already downloaded when the file is the same.
func (e *Engine) RefreshLink(ctx context.Context, id, newURL string, headers map[string]string) error {
	e.mu.Lock()
	t, ok := e.tasks[id]
	e.mu.Unlock()
	if !ok {
		return errors.New("download not found")
	}
	pr, err := e.Probe(ctx, newURL, headers)
	if err != nil {
		return err
	}
	if t.running() {
		e.stopTask(t, StatusPaused)
	}
	t.mu.Lock()
	sameFile := t.d.Size > 0 && pr.Size == t.d.Size && pr.Resumable
	t.d.URL = newURL
	t.d.FinalURL = pr.FinalURL
	if headers != nil {
		t.d.Headers = headers
	}
	if !sameFile {
		t.d.Segments = nil
		t.d.Downloaded = 0
		_ = os.Remove(t.d.TempPath())
	}
	t.d.ETag, t.d.LastMod = pr.ETag, pr.LastMod
	t.d.Status = StatusQueued
	t.d.Manual = true
	t.d.Error, t.d.ErrorCode = "", ""
	snap := t.d.Snapshot()
	t.mu.Unlock()
	e.markDirty()
	e.lis.Changed(snap)
	e.kick()
	if !sameFile {
		return errors.New("the new link points to a different file, so the download starts over")
	}
	return nil
}

// Reorder sets the order downloads start in within the main list.
func (e *Engine) Reorder(ids []string) {
	e.mu.Lock()
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if _, ok := e.tasks[id]; ok && !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	for _, id := range e.order {
		if !seen[id] {
			out = append(out, id)
		}
	}
	e.order = out
	e.mu.Unlock()
	e.markDirty()
}

// ---- scheduling ----

func (e *Engine) kick() {
	select {
	case e.pumpCh <- struct{}{}:
	default:
	}
}

func (e *Engine) loop() {
	defer e.wg.Done()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	last := time.Now()
	for {
		select {
		case <-e.stopCh:
			return
		case <-e.pumpCh:
			e.pump()
		case now := <-tick.C:
			e.measure(now.Sub(last))
			last = now
			e.pump()
		}
	}
}

func (e *Engine) pump() {
	e.mu.Lock()
	if !e.started {
		e.mu.Unlock()
		return
	}
	type cand struct {
		t      *task
		rank   int
		manual bool
		queue  string
		size   int64
	}
	pos := map[string]int{}
	for i, id := range e.order {
		pos[id] = i
	}
	for _, g := range e.gates {
		for i, id := range g.Order {
			if _, ok := pos[id]; ok {
				pos[id] = -100000 + i // explicit queue order wins
			}
		}
	}
	running := map[string]int{}
	var cands []cand
	for _, id := range e.order {
		t := e.tasks[id]
		t.mu.Lock()
		switch {
		case t.run != nil:
			if !t.d.Manual {
				running[t.d.Queue]++
			}
		case t.d.Status == StatusQueued:
			cands = append(cands, cand{t, pos[id], t.d.Manual, t.d.Queue, t.d.Size})
		}
		t.mu.Unlock()
	}
	smart := e.cfg.SmartPriority
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.manual != b.manual {
			return a.manual
		}
		if smart {
			as, bs := a.size > 0 && a.size < 20<<20, b.size > 0 && b.size < 20<<20
			if as != bs {
				return as
			}
		}
		return a.rank < b.rank
	})
	var toStart []*task
	for _, c := range cands {
		if c.manual {
			toStart = append(toStart, c.t)
			continue
		}
		g := e.gates[c.queue]
		if g == nil || !g.Active {
			continue
		}
		limit := g.Concurrent
		if limit <= 0 {
			limit = 1
		}
		if running[c.queue] >= limit {
			continue
		}
		running[c.queue]++
		toStart = append(toStart, c.t)
	}
	for _, t := range toStart {
		e.startTaskLocked(t)
	}
	e.mu.Unlock()
}

const readChunk = 32 << 10

type runState struct {
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	stopAs   Status
	limiter  *rate.Limiter
	qlimiter *rate.Limiter
	conns    atomic.Int32
	bytes    atomic.Int64
	wbytes   [64]atomic.Int64
	wlast    [64]int64
	last     int64
	speed    float64
	history  []float64
	histAcc  float64
	histN    int
	failOnce sync.Once
	failErr  error
	started  time.Time
}

func (rs *runState) fail(err error) {
	rs.failOnce.Do(func() {
		rs.failErr = err
		rs.cancel()
	})
}

func (e *Engine) startTaskLocked(t *task) {
	ctx, cancel := context.WithCancel(context.Background())
	rs := &runState{ctx: ctx, cancel: cancel, done: make(chan struct{}), stopAs: StatusPaused, started: time.Now()}
	rs.limiter = rate.NewLimiter(rate.Inf, 0)
	t.mu.Lock()
	if t.run != nil || t.d.Status != StatusQueued {
		t.mu.Unlock()
		cancel()
		return
	}
	setLimit(rs.limiter, t.d.SpeedLimit)
	if g := e.gates[t.d.Queue]; g != nil && g.limiter != nil && !t.d.Manual {
		rs.qlimiter = g.limiter
	}
	t.d.Status = StatusDownloading
	t.d.Error, t.d.ErrorCode = "", ""
	if t.d.StartedAt.IsZero() {
		t.d.StartedAt = time.Now()
	}
	rs.last = t.d.Downloaded
	t.run = rs
	snap := t.d.Snapshot()
	t.mu.Unlock()
	e.lis.Changed(snap)
	e.markDirty()
	go e.execute(t, rs)
}

// stopTask cancels a running transfer and waits for it to finish writing.
func (e *Engine) stopTask(t *task, as Status) {
	t.mu.Lock()
	rs := t.run
	if rs == nil {
		t.mu.Unlock()
		return
	}
	rs.stopAs = as
	t.mu.Unlock()
	rs.cancel()
	<-rs.done
}

func (e *Engine) execute(t *task, rs *runState) {
	var err error
	t.mu.Lock()
	kind := t.d.Kind
	t.mu.Unlock()
	if kind == KindHLS {
		err = e.runHLS(t, rs)
	} else {
		err = e.runFile(t, rs)
	}
	if err == nil && rs.failErr != nil {
		err = rs.failErr
	}
	if rs.failErr != nil {
		err = rs.failErr
	}

	t.mu.Lock()
	d := t.d
	d.ActiveMs += time.Since(rs.started).Milliseconds()
	completed := false
	switch {
	case err == nil:
		d.Status = StatusCompleted
		d.CompletedAt = time.Now()
		d.Manual = false
		d.Segments = nil
		if d.HLS != nil {
			d.HLS.Parts = nil
		}
		completed = true
	case errors.Is(err, context.Canceled) && rs.failErr == nil:
		d.Status = rs.stopAs
		if rs.stopAs == StatusQueued {
			d.Manual = false
		}
	default:
		d.Status = StatusFailed
		d.Manual = false
		d.Error, d.ErrorCode = describe(err)
	}
	t.run = nil
	snap := d.Snapshot()
	t.mu.Unlock()
	close(rs.done)
	e.markDirty()
	e.lis.Changed(snap)
	if completed {
		e.lis.Completed(snap)
	}
	e.kick()
}

// measure updates speeds and the "time saved by acceleration" estimate.
func (e *Engine) measure(dt time.Duration) {
	secs := dt.Seconds()
	if secs <= 0 {
		return
	}
	e.mu.Lock()
	running := e.runningLocked()
	e.mu.Unlock()
	for _, t := range running {
		t.mu.Lock()
		rs := t.run
		if rs == nil {
			t.mu.Unlock()
			continue
		}
		total := rs.bytes.Load()
		inst := float64(total-rs.last) / secs
		rs.last = total
		if rs.speed == 0 {
			rs.speed = inst
		} else {
			rs.speed = rs.speed*0.6 + inst*0.4
		}
		if rs.speed > t.peak {
			t.peak = rs.speed
		}
		// Estimate what a single connection could have done.
		var best float64
		for i := range rs.wbytes {
			b := rs.wbytes[i].Load()
			s := float64(b-rs.wlast[i]) / secs
			rs.wlast[i] = b
			if s > best {
				best = s
			}
		}
		if best > 0 && inst > best*1.05 {
			saved := secs * (inst/best - 1)
			if c := float64(rs.conns.Load()); saved > secs*(c-1) {
				saved = secs * (c - 1)
			}
			t.d.SavedMs += int64(saved * 1000)
		}
		rs.histAcc += inst
		rs.histN++
		if rs.histN >= 2 {
			rs.history = append(rs.history, rs.histAcc/float64(rs.histN))
			if len(rs.history) > 60 {
				rs.history = rs.history[len(rs.history)-60:]
			}
			rs.histAcc, rs.histN = 0, 0
		}
		t.mu.Unlock()
	}
}

// Live reports progress for everything that is downloading.
func (e *Engine) Live(withHistory string) []Live {
	e.mu.Lock()
	running := e.runningLocked()
	e.mu.Unlock()
	out := make([]Live, 0, len(running))
	for _, t := range running {
		t.mu.Lock()
		rs := t.run
		if rs == nil {
			t.mu.Unlock()
			continue
		}
		d := t.d
		l := Live{ID: d.ID, Status: d.Status, Downloaded: d.Downloaded, Size: d.Size, Speed: rs.speed, Conns: int(rs.conns.Load()), PeakSpeed: t.peak}
		if d.Size > 0 && rs.speed > 1 {
			l.ETA = float64(d.Size-d.Downloaded) / rs.speed
		}
		l.Map = progressMap(d, 32)
		if withHistory == d.ID {
			l.History = append([]float64(nil), rs.history...)
		}
		t.mu.Unlock()
		out = append(out, l)
	}
	return out
}

// progressMap summarizes which parts of the file are done, as n fractions.
func progressMap(d *Download, n int) []float64 {
	if d.Kind == KindHLS && d.HLS != nil && len(d.HLS.Parts) > 0 {
		parts := d.HLS.Parts
		if n > len(parts) {
			n = len(parts)
		}
		out := make([]float64, n)
		for i := 0; i < n; i++ {
			lo, hi := i*len(parts)/n, (i+1)*len(parts)/n
			done := 0
			for _, p := range parts[lo:hi] {
				if p.Done {
					done++
				}
			}
			if hi > lo {
				out[i] = float64(done) / float64(hi-lo)
			}
		}
		return out
	}
	if d.Size <= 0 {
		return nil
	}
	out := make([]float64, n)
	bucket := float64(d.Size) / float64(n)
	for _, s := range d.Segments {
		if s.Done <= 0 {
			continue
		}
		a, b := float64(s.Start), float64(s.Start+s.Done)
		for i := int(a / bucket); i < n && float64(i)*bucket < b; i++ {
			lo, hi := float64(i)*bucket, float64(i+1)*bucket
			ov := minf(hi, b) - maxf(lo, a)
			if ov > 0 {
				out[i] += ov / bucket
			}
		}
	}
	for i := range out {
		if out[i] > 1 {
			out[i] = 1
		}
	}
	return out
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// ---- persistence ----

func (e *Engine) markDirty() { e.dirty.Store(true) }

func (e *Engine) saver() {
	defer e.wg.Done()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-e.stopCh:
			return
		case <-tick.C:
			if e.dirty.Load() || e.anyRunning() {
				e.save()
			}
		}
	}
}

func (e *Engine) anyRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.runningLocked()) > 0
}

func (e *Engine) save() {
	e.dirty.Store(false)
	list := e.List()
	if err := e.store.Save(list); err != nil {
		e.dirty.Store(true)
	}
}

// ---- errors ----

type httpError struct {
	Code   int
	Status string
}

func (h *httpError) Error() string { return "server answered " + h.Status }

type fatalError struct{ err error }

func (f *fatalError) Error() string { return f.err.Error() }
func (f *fatalError) Unwrap() error { return f.err }

type codedError struct {
	code string
	msg  string
}

func (c *codedError) Error() string { return c.msg }

func describe(err error) (string, string) {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.msg, ce.code
	}
	var he *httpError
	if errors.As(err, &he) {
		switch he.Code {
		case 401, 403, 410:
			return "The link has expired or needs you to sign in again. Refresh the link to continue.", ErrCodeLinkExpired
		case 404:
			return "The file isn't on the server any more (404).", ErrCodeNotFound
		default:
			return "The server answered " + he.Status + ".", ErrCodeNetwork
		}
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		return "Couldn't write the file: " + pe.Err.Error(), ErrCodeDisk
	}
	msg := err.Error()
	if strings.Contains(msg, "no space left") || strings.Contains(msg, "not enough space") {
		return "The disk is full.", ErrCodeDisk
	}
	return "Network problem: " + msg, ErrCodeNetwork
}

func isFatal(err error) bool {
	var fe *fatalError
	if errors.As(err, &fe) {
		return true
	}
	var he *httpError
	if errors.As(err, &he) {
		switch he.Code {
		case 401, 403, 404, 410:
			return true
		}
	}
	var ce *codedError
	return errors.As(err, &ce)
}
