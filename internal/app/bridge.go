package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/KartikCodes1/forgedm/internal/config"
	"github.com/KartikCodes1/forgedm/internal/engine"
	"github.com/KartikCodes1/forgedm/internal/grabber"
	"github.com/KartikCodes1/forgedm/internal/platform"
)

// App is the object the user interface calls (bound into JavaScript as
// window.go.app.App). Every exported method here is callable from the UI.
type App struct {
	c *Core
}

// NewApp wraps a Core for binding.
func NewApp(c *Core) *App { return &App{c: c} }

// GetSnapshot returns everything the UI shows.
func (a *App) GetSnapshot() Snapshot { return a.c.snapshot() }

// Probe checks a link before adding it.
func (a *App) Probe(rawURL string, headers map[string]string) ProbeView {
	return a.c.probe(a.c.ctx, strings.TrimSpace(rawURL), headers, "")
}

// Add creates a download.
func (a *App) Add(o AddOptions) (engine.Download, error) {
	if o.Source == "" {
		o.Source = "manual"
	}
	return a.c.add(o)
}

// AddMany creates several downloads (imported lists, batches).
func (a *App) AddMany(list []AddOptions) (int, error) {
	n := 0
	var firstErr error
	for _, o := range list {
		if _, err := a.c.add(o); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		n++
	}
	if n == 0 && firstErr != nil {
		return 0, firstErr
	}
	return n, nil
}

// Resume starts downloads now.
func (a *App) Resume(ids []string) { a.c.eng.Resume(ids...) }

// Pause stops downloads, keeping progress.
func (a *App) Pause(ids []string) { a.c.eng.Pause(ids...) }

// Enqueue sends downloads back to wait in their queue.
func (a *App) Enqueue(ids []string) { a.c.eng.Enqueue(ids...) }

// PauseAll pauses everything.
func (a *App) PauseAll() { a.c.eng.PauseAll() }

// ResumeAll resumes paused and failed downloads.
func (a *App) ResumeAll() { a.c.eng.ResumeAll() }

// Remove deletes downloads from the list (and their files if asked).
func (a *App) Remove(ids []string, deleteFiles bool) {
	for _, id := range ids {
		_ = a.c.eng.Remove(id, deleteFiles)
	}
}

// Redownload starts a download again from the beginning.
func (a *App) Redownload(id string) error { return a.c.eng.Redownload(id) }

// RefreshLink gives an expired download a new address.
func (a *App) RefreshLink(id, newURL string) error {
	ctx, cancel := context.WithTimeout(a.c.ctx, 40*time.Second)
	defer cancel()
	return a.c.eng.RefreshLink(ctx, id, strings.TrimSpace(newURL), nil)
}

// SetTags replaces a download's tags.
func (a *App) SetTags(id string, tags []string) error {
	_, err := a.c.eng.Update(id, func(d *engine.Download) { d.Tags = tags })
	if err == nil {
		_, _ = a.c.cfg.Update(func(s *config.State) { s.Settings.Tags = sortedTags(nil, append(s.Settings.Tags, tags...)) })
	}
	return err
}

// MoveToQueue moves downloads to a queue.
func (a *App) MoveToQueue(ids []string, queue string) {
	for _, id := range ids {
		_, _ = a.c.eng.Update(id, func(d *engine.Download) { d.Queue = queue })
	}
	a.c.applyGates(a.c.cfg.Get(), time.Now())
}

// SetSpeedLimit limits one download (bytes per second, 0 = none).
func (a *App) SetSpeedLimit(id string, bps int64) error {
	_, err := a.c.eng.Update(id, func(d *engine.Download) { d.SpeedLimit = bps })
	return err
}

// SetPost changes what happens when a download finishes.
func (a *App) SetPost(id string, post engine.PostActions) error {
	_, err := a.c.eng.Update(id, func(d *engine.Download) { d.Post = post })
	return err
}

// Rename changes the file name of a download that hasn't started yet or has finished.
func (a *App) Rename(id, name string) error {
	name = engine.SanitizeFilename(name)
	d, ok := a.c.eng.Get(id)
	if !ok {
		return errors.New("download not found")
	}
	if d.Status == engine.StatusCompleted {
		if err := os.Rename(d.Path(), filepath.Join(d.Dir, name)); err != nil {
			return err
		}
	} else if d.Downloaded > 0 {
		return errors.New("you can rename a file once it has finished")
	}
	_, err := a.c.eng.Update(id, func(x *engine.Download) { x.Filename = name; x.NameFixed = true })
	return err
}

// Reorder sets the order of downloads inside a queue.
func (a *App) Reorder(queue string, ids []string) error {
	_, err := a.c.cfg.Update(func(st *config.State) {
		for i := range st.Queues {
			if st.Queues[i].ID == queue {
				st.Queues[i].Order = ids
			}
		}
	})
	a.c.applyGates(a.c.cfg.Get(), time.Now())
	a.c.pushState()
	return err
}

// OpenFile opens a finished download.
func (a *App) OpenFile(id string) error {
	d, ok := a.c.eng.Get(id)
	if !ok {
		return errors.New("download not found")
	}
	return platform.OpenPath(d.Path())
}

// ShowInFolder opens Explorer at the file.
func (a *App) ShowInFolder(id string) error {
	d, ok := a.c.eng.Get(id)
	if !ok {
		return errors.New("download not found")
	}
	p := d.Path()
	if d.Status != engine.StatusCompleted {
		return platform.OpenPath(d.Dir)
	}
	return platform.ShowInFolder(p)
}

// OpenFolder opens any folder.
func (a *App) OpenFolder(path string) error {
	_ = os.MkdirAll(path, 0o755)
	return platform.OpenPath(path)
}

// GetDetails returns one download with its file map and speed history, and
// asks the core to keep sending its history while the progress window is open.
func (a *App) GetDetails(id string) (engine.Download, error) {
	d, ok := a.c.eng.Get(id)
	if !ok {
		return engine.Download{}, errors.New("download not found")
	}
	a.c.mu.Lock()
	a.c.detailsID = id
	a.c.mu.Unlock()
	d.Headers = nil
	if d.HLS != nil {
		d.HLS.Parts = nil
	}
	return d, nil
}

// CloseDetails stops sending history for the progress window.
func (a *App) CloseDetails() {
	a.c.mu.Lock()
	a.c.detailsID = ""
	a.c.mu.Unlock()
}

// ---- settings ----

// SaveSettings stores preferences and applies them.
func (a *App) SaveSettings(s config.Settings) error {
	st, err := a.c.cfg.Update(func(cur *config.State) {
		// Secrets and paired browsers are never edited from the UI.
		s.Browsers = cur.Settings.Browsers
		s.InternalToken = cur.Settings.InternalToken
		if s.BaseDir == "" {
			s.BaseDir = cur.Settings.BaseDir
		}
		if s.APIPort == 0 {
			s.APIPort = cur.Settings.APIPort
		}
		cur.Settings = s
		for i := range cur.Queues {
			if cur.Queues[i].ID == "main" {
				cur.Queues[i].Concurrent = s.MaxActive
			}
		}
	})
	if err != nil {
		return err
	}
	a.c.applySettings(st)
	a.c.setWidget(st.Settings.DropTarget)
	a.c.pushState()
	return nil
}

// SetSpeedLimiter turns the global speed limit on or off.
func (a *App) SetSpeedLimiter(on bool) {
	st, _ := a.c.cfg.Update(func(s *config.State) { s.Settings.SpeedLimitOn = on })
	a.c.applySettings(st)
	a.c.pushState()
}

// SetDropTarget shows or hides the floating drop target.
func (a *App) SetDropTarget(on bool) {
	_, _ = a.c.cfg.Update(func(s *config.State) { s.Settings.DropTarget = on })
	a.c.setWidget(on)
	a.c.pushState()
}

// SaveRules replaces the save rules.
func (a *App) SaveRules(list []config.Rule) error {
	for i := range list {
		if list[i].ID == "" {
			list[i].ID = config.NewID()
		}
	}
	_, err := a.c.cfg.Update(func(s *config.State) { s.Rules = list })
	a.c.pushState()
	return err
}

// SaveQueue creates or updates a queue.
func (a *App) SaveQueue(q config.Queue) (config.Queue, error) {
	if strings.TrimSpace(q.Name) == "" {
		return q, errors.New("give the queue a name")
	}
	if q.ID == "" {
		q.ID = config.NewID()
	}
	st, err := a.c.cfg.Update(func(s *config.State) {
		for i := range s.Queues {
			if s.Queues[i].ID == q.ID {
				q.Builtin = s.Queues[i].Builtin
				q.Order = s.Queues[i].Order
				s.Queues[i] = q
				return
			}
		}
		s.Queues = append(s.Queues, q)
	})
	if err != nil {
		return q, err
	}
	if q.ID == "main" {
		_, _ = a.c.cfg.Update(func(s *config.State) { s.Settings.MaxActive = q.Concurrent })
		st = a.c.cfg.Get()
		a.c.applySettings(st)
	}
	a.c.applyGates(st, time.Now())
	a.c.armWake(st, time.Now())
	a.c.pushState()
	return q, nil
}

// DeleteQueue removes a queue; its downloads move to the main queue.
func (a *App) DeleteQueue(id string) error {
	if id == "main" {
		return errors.New("the main queue can't be deleted")
	}
	a.c.StopQueue(id)
	for _, d := range a.c.eng.List() {
		if d.Queue == id {
			_, _ = a.c.eng.Update(d.ID, func(x *engine.Download) { x.Queue = "main" })
		}
	}
	_, err := a.c.cfg.Update(func(s *config.State) {
		var out []config.Queue
		for _, q := range s.Queues {
			if q.ID != id {
				out = append(out, q)
			}
		}
		s.Queues = out
	})
	a.c.pushState()
	return err
}

// RunQueue starts a queue now.
func (a *App) RunQueue(id string) { a.c.RunQueueNow(id) }

// StopQueue stops a running queue.
func (a *App) StopQueue(id string) { a.c.StopQueue(id) }

// ---- dialogs ----

// ChooseFolder shows a folder picker.
func (a *App) ChooseFolder(start string) string {
	return a.c.ui.ChooseFolder("Choose a folder", start)
}

// ImportList reads links from a text file the user picks.
func (a *App) ImportList() ([]string, error) {
	p := a.c.ui.OpenFile("Import a list of links", "*.txt;*.csv;*.lst")
	if p == "" {
		return nil, nil
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		for _, field := range strings.FieldsFunc(sc.Text(), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == ';' }) {
			field = strings.Trim(field, `"' `)
			if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
				out = append(out, field)
			}
		}
	}
	return out, sc.Err()
}

// ExportSettings saves settings, rules, queues and projects to a file.
func (a *App) ExportSettings() (string, error) {
	p := a.c.ui.SaveFile("Export ForgeDM settings", "forgedm-settings.json")
	if p == "" {
		return "", nil
	}
	st := a.c.cfg.Get()
	st.Settings.Browsers = nil
	st.Settings.InternalToken = ""
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return "", err
	}
	return p, os.WriteFile(p, b, 0o644)
}

// ImportSettings loads a file made by ExportSettings.
func (a *App) ImportSettings() error {
	p := a.c.ui.OpenFile("Import ForgeDM settings", "*.json")
	if p == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	var in config.State
	if err := json.Unmarshal(b, &in); err != nil {
		return errors.New("that file isn't a ForgeDM settings export")
	}
	st, err := a.c.cfg.Update(func(cur *config.State) {
		in.Settings.Browsers = cur.Settings.Browsers
		in.Settings.InternalToken = cur.Settings.InternalToken
		if in.Settings.BaseDir == "" {
			in.Settings.BaseDir = cur.Settings.BaseDir
		}
		*cur = in
	})
	if err != nil {
		return err
	}
	a.c.applySettings(st)
	a.c.pushState()
	return nil
}

// ---- browser ----

// AnswerPairing approves or declines a browser that wants to connect.
func (a *App) AnswerPairing(id string, allow bool) {
	a.c.mu.Lock()
	req := a.c.pairs[id]
	a.c.mu.Unlock()
	if req != nil {
		select {
		case req.ch <- allow:
		default:
		}
	}
}

// RemoveBrowser disconnects a paired browser.
func (a *App) RemoveBrowser(id string) error {
	_, err := a.c.cfg.Update(func(s *config.State) {
		var out []config.Browser
		for _, b := range s.Settings.Browsers {
			if b.ID != id {
				out = append(out, b)
			}
		}
		s.Settings.Browsers = out
	})
	a.c.pushState()
	return err
}

// AnswerCapture handles the user's choice for a download caught from the
// browser: "start", "later" or "cancel" (let the browser have it).
func (a *App) AnswerCapture(id string, o AddOptions, action string) error {
	a.c.mu.Lock()
	pc := a.c.captures[id]
	delete(a.c.captures, id)
	a.c.mu.Unlock()
	if pc == nil {
		return errors.New("this download is no longer waiting")
	}
	if action == "cancel" {
		return nil
	}
	o.URL = pc.Options.URL
	o.PageURL = pc.Options.PageURL
	o.Headers = pc.Options.Headers
	if o.Kind == "" {
		o.Kind = pc.Options.Kind
	}
	o.Source = "browser"
	o.Start = action == "start"
	_, err := a.c.add(o)
	return err
}

// AnswerBatch adds the links the user picked from a batch.
func (a *App) AnswerBatch(id string, urls []string, dir string, queue string, start bool) (int, error) {
	a.c.mu.Lock()
	b, ok := a.c.batches[id]
	delete(a.c.batches, id)
	a.c.mu.Unlock()
	if !ok {
		return 0, errors.New("these links are no longer waiting")
	}
	want := map[string]bool{}
	for _, u := range urls {
		want[u] = true
	}
	headers := headersFrom(b.Cookies, b.PageURL, b.UserAgent, nil)
	n := 0
	for _, l := range b.Links {
		if !want[l.URL] {
			continue
		}
		if _, err := a.c.add(AddOptions{URL: l.URL, PageURL: b.PageURL, Filename: "", Dir: dir, Queue: queue, Start: start, Headers: headers, Source: "browser"}); err == nil {
			n++
		}
	}
	return n, nil
}

// DismissClipboard stops offering a copied link.
func (a *App) DismissClipboard(link string) {
	a.c.mu.Lock()
	a.c.clipSkip[a.c.clipLast] = true
	a.c.mu.Unlock()
}

// ExtensionFolder unpacks the browser extension that ships inside ForgeDM
// and returns the folder holding its chrome/ and firefox/ versions.
func (a *App) ExtensionFolder() string {
	if a.c.extFS != nil {
		dir := filepath.Join(filepath.Dir(a.c.cfg.Path()), "extension")
		_ = os.RemoveAll(dir)
		err := fs.WalkDir(a.c.extFS, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			target := filepath.Join(dir, filepath.FromSlash(p))
			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			b, err := fs.ReadFile(a.c.extFS, p)
			if err != nil {
				return err
			}
			return os.WriteFile(target, b, 0o644)
		})
		if err == nil {
			return dir
		}
	}
	if a.c.exe != "" {
		dir := filepath.Join(filepath.Dir(a.c.exe), "extension")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	return ""
}

// ---- Site Grabber ----

// ScanRequest starts a Site Grabber scan.
type ScanRequest struct {
	URL      string `json:"url"`
	Depth    int    `json:"depth"`
	SameSite bool   `json:"sameSite"`
}

// StartScan begins scanning and returns an id. Results arrive as events:
// "grabber" (progress) and "grabber:done" (items or error).
func (a *App) StartScan(req ScanRequest) string {
	id := config.NewID()
	ctx, cancel := context.WithTimeout(a.c.ctx, 30*time.Minute)
	a.c.mu.Lock()
	a.c.scans[id] = cancel
	a.c.mu.Unlock()
	st := a.c.cfg.Get()
	go func() {
		defer cancel()
		start := time.Now()
		items, err := grabber.Scan(ctx, a.c.httpClient(), grabber.Options{URL: req.URL, Depth: req.Depth, SameSite: req.SameSite, UserAgent: st.Settings.UserAgent},
			func(p grabber.Progress) { a.c.ui.Emit("grabber", map[string]any{"id": id, "progress": p}) })
		have := a.c.existingByURL()
		type itemView struct {
			grabber.Item
			Have bool `json:"have"`
		}
		views := make([]itemView, 0, len(items))
		for _, it := range items {
			_, ok := have[normalizeURL(it.URL)]
			views = append(views, itemView{Item: it, Have: ok})
		}
		msg := ""
		if err != nil && !errors.Is(err, context.Canceled) {
			msg = err.Error()
		}
		a.c.mu.Lock()
		delete(a.c.scans, id)
		a.c.mu.Unlock()
		a.c.ui.Emit("grabber:done", map[string]any{"id": id, "items": views, "error": msg, "seconds": time.Since(start).Seconds()})
	}()
	return id
}

// CancelScan stops a scan.
func (a *App) CancelScan(id string) {
	a.c.mu.Lock()
	cancel := a.c.scans[id]
	a.c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// DownloadGrabbed adds the files picked in Site Grabber.
func (a *App) DownloadGrabbed(items []grabber.Item, dir string, keepStructure bool, queue string, start bool) (int, error) {
	if queue == "" {
		queue = "main"
	}
	return a.c.addGrabbed(items, dir, keepStructure, queue, start, "grabber")
}

// SaveProject stores a Site Grabber project (and its sync schedule).
func (a *App) SaveProject(p config.Project) (config.Project, error) {
	if p.ID == "" {
		p.ID = config.NewID()
	}
	if p.Name == "" {
		p.Name = p.URL
	}
	_, err := a.c.cfg.Update(func(s *config.State) {
		for i := range s.Projects {
			if s.Projects[i].ID == p.ID {
				p.LastRun = s.Projects[i].LastRun
				s.Projects[i] = p
				return
			}
		}
		s.Projects = append(s.Projects, p)
	})
	a.c.pushState()
	return p, err
}

// DeleteProject removes a saved project.
func (a *App) DeleteProject(id string) error {
	_, err := a.c.cfg.Update(func(s *config.State) {
		var out []config.Project
		for _, p := range s.Projects {
			if p.ID != id {
				out = append(out, p)
			}
		}
		s.Projects = out
	})
	a.c.pushState()
	return err
}

// ---- window ----

// CloseWindow hides to the tray or quits, depending on settings.
func (a *App) CloseWindow() {
	if a.c.cfg.Get().Settings.CloseToTray {
		a.c.ui.Hide()
		return
	}
	a.c.ui.Quit()
}

// Quit exits ForgeDM.
func (a *App) Quit() { a.c.ui.Quit() }

// MarkOnboarded records that the welcome screen was seen.
func (a *App) MarkOnboarded() {
	_, _ = a.c.cfg.Update(func(s *config.State) { s.Settings.Onboarded = true })
}
