package app

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/KartikCodes1/forgedm/internal/api"
	"github.com/KartikCodes1/forgedm/internal/config"
	"github.com/KartikCodes1/forgedm/internal/engine"
	"github.com/KartikCodes1/forgedm/internal/grabber"
	"github.com/KartikCodes1/forgedm/internal/rules"
)

// AddOptions is what the New download dialog (and captures) submit.
type AddOptions struct {
	URL         string             `json:"url"`
	PageURL     string             `json:"pageUrl,omitempty"`
	Kind        string             `json:"kind,omitempty"`
	Filename    string             `json:"filename,omitempty"`
	Dir         string             `json:"dir,omitempty"`
	Category    string             `json:"category,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Queue       string             `json:"queue,omitempty"`
	Connections int                `json:"connections,omitempty"`
	SpeedLimit  int64              `json:"speedLimit,omitempty"`
	Post        engine.PostActions `json:"post"`
	Checksum    string             `json:"checksum,omitempty"`
	Start       bool               `json:"start"`
	Headers     map[string]string  `json:"headers,omitempty"`
	Size        int64              `json:"size,omitempty"`
	Source      string             `json:"source,omitempty"`
	Rule        string             `json:"rule,omitempty"`
}

// ProbeView is the result of checking a link in the New download dialog.
type ProbeView struct {
	engine.ProbeResult
	Decision  rules.Decision   `json:"decision"`
	Duplicate *engine.Download `json:"duplicate,omitempty"`
	Error     string           `json:"error,omitempty"`
}

// probe checks a link and applies the save rules.
func (c *Core) probe(ctx context.Context, rawURL string, headers map[string]string, pageURL string) ProbeView {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pv := ProbeView{}
	pr, err := c.eng.Probe(ctx, rawURL, headers)
	pv.ProbeResult = pr
	if err != nil {
		pv.Error = friendlyProbeError(err)
		if pv.Filename == "" {
			pv.Filename = engine.SanitizeFilename(engine.FilenameFromURL(rawURL))
		}
	}
	st := c.cfg.Get()
	pv.Decision = rules.Decide(st, rules.Input{URL: rawURL, PageURL: pageURL, Filename: pv.Filename, Size: pv.Size})
	pv.Duplicate = c.findDuplicate(rawURL, pv.Filename, pv.Size)
	return pv
}

func friendlyProbeError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no such host"):
		return "The website couldn't be found. Check the address."
	case strings.Contains(msg, "403"), strings.Contains(msg, "401"):
		return "The server refused the request. The link may have expired or need you to sign in."
	case strings.Contains(msg, "404"):
		return "The file isn't on the server (404)."
	case strings.Contains(msg, "deadline"), strings.Contains(msg, "timeout"):
		return "The server took too long to answer."
	}
	return msg
}

// findDuplicate returns an existing download of the same link or file.
func (c *Core) findDuplicate(rawURL, name string, size int64) *engine.Download {
	key := normalizeURL(rawURL)
	for _, d := range c.eng.List() {
		if normalizeURL(d.URL) == key || (name != "" && size > 0 && strings.EqualFold(d.Filename, name) && d.Size == size) {
			dd := d
			dd.Segments, dd.Headers = nil, nil
			return &dd
		}
	}
	return nil
}

// add applies rules and defaults, then adds the download to the engine.
func (c *Core) add(o AddOptions) (engine.Download, error) {
	st := c.cfg.Get()
	name := o.Filename
	if name == "" {
		name = engine.FilenameFromURL(o.URL)
	}
	dec := rules.Decide(st, rules.Input{URL: o.URL, PageURL: o.PageURL, Filename: name, Size: o.Size})
	if o.Dir == "" {
		o.Dir = dec.Dir
	}
	if o.Category == "" {
		o.Category = dec.Category
	}
	if o.Queue == "" {
		o.Queue = dec.Queue
	}
	if len(o.Tags) == 0 {
		o.Tags = dec.Tags
	}
	if o.Rule == "" {
		o.Rule = dec.RuleName
	}
	if o.Source == "" {
		o.Source = "manual"
	}
	if o.Source != "manual" {
		// Captured downloads take the defaults from settings and rules.
		o.Post.Verify = o.Post.Verify || st.Settings.VerifyChecksum
		o.Post.Scan = o.Post.Scan || st.Settings.ScanAfter
		o.Post.Open = o.Post.Open || st.Settings.OpenWhenDone || dec.Open
		o.Post.Extract = o.Post.Extract || dec.Extract
	}
	kind := engine.Kind(o.Kind)
	d, err := c.eng.Add(engine.Request{
		URL: o.URL, PageURL: o.PageURL, Kind: kind, Filename: o.Filename, Dir: o.Dir,
		Headers: o.Headers, Category: o.Category, Tags: o.Tags, Queue: o.Queue,
		Connections: o.Connections, SpeedLimit: o.SpeedLimit, Post: o.Post, Checksum: o.Checksum,
		Start: o.Start, Source: o.Source, Rule: o.Rule, Size: o.Size,
	})
	if err == nil && len(o.Tags) > 0 {
		_, _ = c.cfg.Update(func(s *config.State) {
			s.Settings.Tags = sortedTags(nil, append(s.Settings.Tags, o.Tags...))
		})
	}
	return d, err
}

// ---- browser API ----

type apiHooks struct{ c *Core }

type pendingCapture struct {
	ID      string      `json:"id"`
	Capture api.Capture `json:"capture"`
	Browser string      `json:"browser"`
	Probe   *ProbeView  `json:"probe,omitempty"`
	Options AddOptions  `json:"options"`
	At      time.Time   `json:"at"`
}

func (h apiHooks) Capture(cp api.Capture, browser string) (string, error) {
	c := h.c
	st := c.cfg.Get()
	u, err := url.Parse(cp.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("only http and https links can be downloaded")
	}
	headers := headersFrom(cp.Cookies, cp.Referrer, cp.UserAgent, cp.Headers)
	kind := ""
	name := cp.Filename
	if cp.Kind == "hls" {
		kind = string(engine.KindHLS)
		if name == "" {
			name = titleToName(cp.PageTitle, ".ts")
		}
	}
	opts := AddOptions{URL: cp.URL, PageURL: cp.PageURL, Kind: kind, Filename: name, Headers: headers, Size: cp.Size, Source: "browser"}
	if cp.Silent || !st.Settings.AskOnCapture {
		opts.Start = true
		d, err := c.add(opts)
		if err != nil {
			return "", err
		}
		c.toast("info", "Downloading "+d.Filename)
		return "added", nil
	}
	pc := &pendingCapture{ID: config.NewID(), Capture: cp, Browser: browser, Options: opts, At: time.Now()}
	// Show the dialog right away, then fill in what the server says.
	st2 := c.cfg.Get()
	dec := rules.Decide(st2, rules.Input{URL: cp.URL, PageURL: cp.PageURL, Filename: firstNonEmpty(name, engine.FilenameFromURL(cp.URL)), Size: cp.Size})
	pc.Probe = &ProbeView{ProbeResult: engine.ProbeResult{URL: cp.URL, Filename: firstNonEmpty(name, engine.SanitizeFilename(engine.FilenameFromURL(cp.URL))), Size: sizeOr(cp.Size), Kind: engine.Kind(firstNonEmpty(kind, "file"))}, Decision: dec}
	c.mu.Lock()
	c.captures[pc.ID] = pc
	c.mu.Unlock()
	c.ui.Emit("capture", pc)
	c.ui.Show()
	go func() {
		pv := c.probe(c.ctx, cp.URL, headers, cp.PageURL)
		if name != "" {
			pv.Filename = name
		}
		c.mu.Lock()
		if _, ok := c.captures[pc.ID]; ok {
			pc.Probe = &pv
		}
		c.mu.Unlock()
		c.ui.Emit("capture", pc)
	}()
	return "asked", nil
}

func sizeOr(n int64) int64 {
	if n > 0 {
		return n
	}
	return -1
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func titleToName(title, ext string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		t = "video"
	}
	if len(t) > 120 {
		t = t[:120]
	}
	return engine.SanitizeFilename(t + ext)
}

func (h apiHooks) Batch(b api.Batch, browser string) error {
	c := h.c
	id := config.NewID()
	c.mu.Lock()
	c.batches[id] = b
	c.mu.Unlock()
	type row struct {
		URL  string `json:"url"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	var rows []row
	for _, l := range b.Links {
		n := l.Name
		if n == "" || !strings.Contains(n, ".") {
			n = engine.SanitizeFilename(engine.FilenameFromURL(l.URL))
		}
		rows = append(rows, row{URL: l.URL, Name: n, Type: config.CategoryFor(n)})
	}
	c.ui.Emit("batch", map[string]any{"id": id, "pageUrl": b.PageURL, "links": rows, "browser": browser})
	c.ui.Show()
	return nil
}

func (h apiHooks) Status() api.Status {
	c := h.c
	live := c.eng.Live("")
	c.mu.Lock()
	speed := c.speed
	c.mu.Unlock()
	s := api.Status{Speed: speed, Active: len(live)}
	byID := map[string]engine.Download{}
	for _, d := range c.eng.List() {
		byID[d.ID] = d
		if d.Status == engine.StatusQueued {
			s.Queued++
		}
	}
	for _, l := range live {
		p := 0.0
		if l.Size > 0 {
			p = float64(l.Downloaded) / float64(l.Size)
		}
		s.Items = append(s.Items, api.StatusItem{Name: byID[l.ID].Filename, Progress: p, Speed: l.Speed})
	}
	return s
}

func (h apiHooks) Config() api.ExtConfig {
	st := h.c.cfg.Get()
	return api.ExtConfig{
		CaptureEnabled: st.Settings.CaptureEnabled,
		Exts:           st.Settings.CaptureExts,
		MinSize:        st.Settings.CaptureMinSize,
		VideoButton:    st.Settings.VideoButton,
		AltBypass:      st.Settings.AltBypass,
	}
}

type pairRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	ch   chan bool
}

func (h apiHooks) Pair(ctx context.Context, name string) (string, error) {
	c := h.c
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Browser"
	}
	if len(name) > 40 {
		name = name[:40]
	}
	req := &pairRequest{ID: config.NewID(), Name: name, ch: make(chan bool, 1)}
	c.mu.Lock()
	c.pairs[req.ID] = req
	c.mu.Unlock()
	c.ui.Emit("pair", req)
	c.ui.Show()
	defer func() {
		c.mu.Lock()
		delete(c.pairs, req.ID)
		c.mu.Unlock()
		c.ui.Emit("pair:closed", req.ID)
	}()
	select {
	case ok := <-req.ch:
		if !ok {
			return "", errors.New("the connection was declined in ForgeDM")
		}
	case <-ctx.Done():
		return "", errors.New("no answer in ForgeDM")
	}
	b := config.Browser{ID: config.NewID(), Name: name, Token: config.NewToken(), PairedAt: time.Now(), LastSeen: time.Now()}
	if _, err := c.cfg.Update(func(st *config.State) { st.Settings.Browsers = append(st.Settings.Browsers, b) }); err != nil {
		return "", err
	}
	c.toast("success", name+" is connected to ForgeDM")
	c.pushState()
	return b.Token, nil
}

func (h apiHooks) Authorize(token string) (string, bool) {
	c := h.c
	if token == "" {
		return "", false
	}
	st := c.cfg.Get()
	if token == st.Settings.InternalToken {
		return "Drop target", true
	}
	for _, b := range st.Settings.Browsers {
		if b.Token == token {
			c.mu.Lock()
			last := c.lastSeenAt[b.ID]
			due := time.Since(last) > 5*time.Minute
			if due {
				c.lastSeenAt[b.ID] = time.Now()
			}
			c.mu.Unlock()
			if due {
				_, _ = c.cfg.Update(func(st *config.State) {
					for i := range st.Settings.Browsers {
						if st.Settings.Browsers[i].ID == b.ID {
							st.Settings.Browsers[i].LastSeen = time.Now()
						}
					}
				})
			}
			return b.Name, true
		}
	}
	return "", false
}

func (h apiHooks) Show() { h.c.ui.Show() }

func (h apiHooks) Quit(token string) bool {
	if token == "" || token != h.c.cfg.Get().Settings.InternalToken {
		return false
	}
	// Let the HTTP response go out before the app (and this server) shuts down.
	go func() {
		time.Sleep(150 * time.Millisecond)
		h.c.ui.Quit()
	}()
	return true
}

// ---- clipboard ----

func looksDownloadable(raw string, exts []string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 4000 || strings.ContainsAny(raw, " \n\t") {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(u.Path)), ".")
	if ext == "" {
		return "", false
	}
	if ext == "m3u8" {
		return u.String(), true
	}
	for _, e := range exts {
		if strings.EqualFold(strings.TrimPrefix(e, "."), ext) {
			return u.String(), true
		}
	}
	if cat := config.CategoryFor("x." + ext); cat != "Other" {
		return u.String(), true
	}
	return "", false
}

func (c *Core) checkClipboard() {
	st := c.cfg.Get()
	if !st.Settings.ClipboardWatch {
		return
	}
	text := c.ui.ClipboardText()
	c.mu.Lock()
	if text == c.clipLast {
		c.mu.Unlock()
		return
	}
	c.clipLast = text
	skip := c.clipSkip[text]
	c.mu.Unlock()
	if skip {
		return
	}
	link, ok := looksDownloadable(text, st.Settings.CaptureExts)
	if !ok {
		return
	}
	if _, exists := c.existingByURL()[normalizeURL(link)]; exists {
		return
	}
	name := engine.SanitizeFilename(engine.FilenameFromURL(link))
	c.ui.Emit("clipboard", map[string]any{"url": link, "name": name, "size": -1})
	go func() {
		pv := c.probe(c.ctx, link, nil, "")
		if pv.Error == "" {
			c.ui.Emit("clipboard", map[string]any{"url": link, "name": pv.Filename, "size": pv.Size, "resumable": pv.Resumable})
		}
	}()
}

// ---- Site Grabber ----

func filterItems(items []grabber.Item, types []string, exts string, minSize int64) []grabber.Item {
	tset := map[string]bool{}
	for _, t := range types {
		tset[t] = true
	}
	eset := map[string]bool{}
	for _, e := range rules.ParseList(exts) {
		eset[e] = true
	}
	var out []grabber.Item
	for _, it := range items {
		if len(tset) > 0 && !tset[it.Type] {
			continue
		}
		if len(eset) > 0 && !eset[strings.ToLower(it.Ext)] {
			continue
		}
		if minSize > 0 && it.Size >= 0 && it.Size < minSize {
			continue
		}
		out = append(out, it)
	}
	return out
}

func (c *Core) addGrabbed(items []grabber.Item, dir string, keep bool, queue string, start bool, source string) (int, error) {
	n := 0
	var firstErr error
	for _, it := range items {
		target := dir
		if target != "" && keep && it.Path != "" && it.Path != "." {
			target = filepath.Join(dir, it.Path)
		}
		_, err := c.add(AddOptions{URL: it.URL, PageURL: it.Page, Filename: it.Name, Dir: target, Queue: queue, Start: false, Size: it.Size, Source: source})
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		n++
	}
	if start && n > 0 {
		c.applyGates(c.cfg.Get(), time.Now())
	}
	return n, firstErr
}
