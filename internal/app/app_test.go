package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KartikCodes1/forgedm/internal/config"
	"github.com/KartikCodes1/forgedm/internal/engine"
	"github.com/KartikCodes1/forgedm/internal/grabber"
)

type fakeUI struct {
	mu     sync.Mutex
	events map[string][]any
	clip   string
	notes  []string
}

func newFakeUI() *fakeUI { return &fakeUI{events: map[string][]any{}} }

func (f *fakeUI) Emit(ev string, data any) {
	f.mu.Lock()
	f.events[ev] = append(f.events[ev], data)
	f.mu.Unlock()
}
func (f *fakeUI) Show()                         {}
func (f *fakeUI) Hide()                         {}
func (f *fakeUI) Notify(t, b string)            { f.mu.Lock(); f.notes = append(f.notes, t+": "+b); f.mu.Unlock() }
func (f *fakeUI) ClipboardText() string         { f.mu.Lock(); defer f.mu.Unlock(); return f.clip }
func (f *fakeUI) ChooseFolder(_, _ string) string { return "" }
func (f *fakeUI) OpenFile(_, _ string) string   { return "" }
func (f *fakeUI) SaveFile(_, _ string) string   { return "" }
func (f *fakeUI) Quit()                         {}

func (f *fakeUI) count(ev string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events[ev])
}

func (f *fakeUI) last(ev string) any {
	f.mu.Lock()
	defer f.mu.Unlock()
	l := f.events[ev]
	if len(l) == 0 {
		return nil
	}
	return l[len(l)-1]
}

func zipBytes(t *testing.T) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("docs/readme.txt")
	w.Write([]byte("hello from the archive"))
	zw.Close()
	return buf.Bytes()
}

func newTestCore(t *testing.T) (*Core, *App, *fakeUI, string) {
	t.Helper()
	data := t.TempDir()
	dl := t.TempDir()
	ui := newFakeUI()
	c, err := NewCore(ui, Options{DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.cfg.Update(func(s *config.State) {
		s.Settings.BaseDir = dl
		s.Settings.APIPort = 53871
	})
	c.applySettings(c.cfg.Get())
	c.Startup(context.Background())
	t.Cleanup(c.Shutdown)
	return c, NewApp(c), ui, dl
}

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestAddWithRulesChecksumAndExtract(t *testing.T) {
	archive := zipBytes(t)
	sum := sha256.Sum256(archive)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "pack.zip", time.Now(), bytes.NewReader(archive))
	}))
	defer srv.Close()
	c, a, ui, dl := newTestCore(t)
	_ = ui
	if err := a.SaveRules([]config.Rule{{Name: "Unpack", Enabled: true, Conditions: []config.Condition{{Field: "type", Value: "Compressed"}}, Actions: config.RuleActions{Extract: true, Tags: []string{"work"}}}}); err != nil {
		t.Fatal(err)
	}
	pv := a.Probe(srv.URL+"/files/pack.zip", nil)
	if pv.Error != "" || pv.Filename != "pack.zip" || pv.Size != int64(len(archive)) || pv.Decision.Category != "Compressed" || !pv.Decision.Extract {
		t.Fatalf("probe: %+v", pv)
	}
	d, err := a.Add(AddOptions{URL: srv.URL + "/files/pack.zip", Start: true, Post: engine.PostActions{Verify: true, Extract: pv.Decision.Extract}, Checksum: hex.EncodeToString(sum[:])})
	if err != nil {
		t.Fatal(err)
	}
	if d.Dir != filepath.Join(dl, "Compressed") {
		t.Fatalf("dir = %s", d.Dir)
	}
	waitFor(t, "post-processing", 15*time.Second, func() bool {
		x, _ := c.eng.Get(d.ID)
		return x.Status == engine.StatusCompleted && x.PostState == "" && x.PostNote != ""
	})
	x, _ := c.eng.Get(d.ID)
	if !x.Checksum.Verified {
		t.Errorf("checksum not verified: %+v", x.Checksum)
	}
	if !strings.Contains(x.PostNote, "Extracted") {
		t.Errorf("note = %q", x.PostNote)
	}
	b, err := os.ReadFile(filepath.Join(dl, "Compressed", "pack", "docs", "readme.txt"))
	if err != nil || string(b) != "hello from the archive" {
		t.Errorf("extracted content: %q %v", b, err)
	}
	if dup := c.findDuplicate(srv.URL+"/files/pack.zip", "", 0); dup == nil {
		t.Error("duplicate not detected")
	}
}

func TestBrowserPairingAndCapture(t *testing.T) {
	data := []byte(strings.Repeat("x", 300000))
	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "session=abc" {
			http.Error(w, "need cookie", http.StatusForbidden)
			return
		}
		http.ServeContent(w, r, "x.bin", time.Now(), bytes.NewReader(data))
	}))
	defer files.Close()
	c, a, ui, _ := newTestCore(t)
	base := fmt.Sprintf("http://127.0.0.1:%d", c.api.Port)

	// A web page (not an extension) is refused.
	req, _ := http.NewRequest("GET", base+"/v1/ping", nil)
	req.Header.Set("Origin", "https://evil.example")
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("web origin not refused: %v %v", resp, err)
	}

	// Pair: the app asks the user, who approves.
	go func() {
		waitFor(t, "pair event", 5*time.Second, func() bool { return ui.count("pair") > 0 })
		a.AnswerPairing(ui.last("pair").(*pairRequest).ID, true)
	}()
	req, _ = http.NewRequest("POST", base+"/v1/pair", strings.NewReader(`{"name":"Chrome"}`))
	req.Header.Set("Origin", "chrome-extension://abcdef")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("pair: %v %v", resp, err)
	}
	var pr struct{ Token string }
	json.NewDecoder(resp.Body).Decode(&pr)
	if pr.Token == "" {
		t.Fatal("no token")
	}

	// Unpaired requests are rejected.
	req, _ = http.NewRequest("POST", base+"/v1/download", strings.NewReader(`{"url":"`+files.URL+`/x.bin"}`))
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	// Captured download with cookies: user picks "start".
	body := fmt.Sprintf(`{"url":"%s/x.bin","cookies":"session=abc","referrer":"https://shop.example/page"}`, files.URL)
	req, _ = http.NewRequest("POST", base+"/v1/download", strings.NewReader(body))
	req.Header.Set("X-ForgeDM-Token", pr.Token)
	req.Header.Set("Origin", "chrome-extension://abcdef")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("capture: %v %v", resp, err)
	}
	waitFor(t, "capture dialog", 5*time.Second, func() bool { return ui.count("capture") > 0 })
	pc := ui.last("capture").(*pendingCapture)
	if err := a.AnswerCapture(pc.ID, AddOptions{}, "start"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "download", 10*time.Second, func() bool {
		for _, d := range c.eng.List() {
			if d.Status == engine.StatusCompleted && d.Source == "browser" {
				return true
			}
		}
		return false
	})
	if len(a.GetSnapshot().Browsers) != 1 {
		t.Error("paired browser not listed")
	}
}

func TestClipboardDetection(t *testing.T) {
	c, _, ui, _ := newTestCore(t)
	ui.mu.Lock()
	ui.clip = "https://example.invalid/files/setup-2026.exe"
	ui.mu.Unlock()
	c.checkClipboard()
	if ui.count("clipboard") != 1 {
		t.Fatal("link not offered")
	}
	ui.mu.Lock()
	ui.clip = "just some text"
	ui.mu.Unlock()
	c.checkClipboard()
	if ui.count("clipboard") != 1 {
		t.Fatal("plain text should be ignored")
	}
}

func TestScheduleWindows(t *testing.T) {
	loc := time.Local
	mon := time.Date(2026, 11, 16, 2, 0, 0, 0, loc) // a Monday
	night := config.Queue{ID: "night", Enabled: true, Start: "23:00", Stop: "06:00", Days: []int{0}} // Sundays
	if !inWindow(night, mon, "") {
		t.Error("overnight window from Sunday should still be open at 02:00 Monday")
	}
	if inWindow(night, mon.Add(5*time.Hour), "") {
		t.Error("window should be closed at 07:00")
	}
	day := config.Queue{ID: "d", Enabled: true, Start: "09:00", Stop: "13:00", Days: []int{6}}
	sat := time.Date(2026, 11, 21, 10, 0, 0, 0, loc)
	if !inWindow(day, sat, "") || inWindow(day, sat.AddDate(0, 0, 1), "") {
		t.Error("weekend window wrong")
	}
	open := config.Queue{ID: "o", Enabled: true, Start: "01:00"}
	if !inWindow(open, mon, "") || inWindow(open, mon, mon.Format("2006-01-02")) {
		t.Error("open-ended window wrong")
	}
	if n := nextStart(day, sat); n.Weekday() != time.Saturday || !n.After(sat) {
		t.Errorf("next start = %v", n)
	}
}

func TestSiteGrabberScan(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/gallery/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><a href="page2.html">next</a><img src="/img/peak-1.jpg"><a href="/files/guide.pdf">guide</a><a href="https://other.example/x.html">other</a></body></html>`)
	})
	mux.HandleFunc("/gallery/page2.html", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><img srcset="/img/peak-2.jpg 1x, /img/peak-2@2x.jpg 2x"><video src="/v/clip.mp4"></video></body></html>`)
	})
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) { w.Write(make([]byte, 1234)) })
	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) { w.Write(make([]byte, 99)) })
	mux.HandleFunc("/v/", func(w http.ResponseWriter, r *http.Request) { w.Write(make([]byte, 50)) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	items, err := grabber.Scan(context.Background(), http.DefaultClient, grabber.Options{URL: srv.URL + "/gallery/", Depth: 1, SameSite: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]grabber.Item{}
	for _, it := range items {
		names[it.Name] = it
	}
	for _, want := range []string{"peak-1.jpg", "peak-2.jpg", "peak-2@2x.jpg", "guide.pdf", "clip.mp4"} {
		if _, ok := names[want]; !ok {
			t.Errorf("missing %s (got %v)", want, items)
		}
	}
	if names["peak-1.jpg"].Size != 1234 || names["guide.pdf"].Type != "Documents" {
		t.Errorf("sizes/types wrong: %+v %+v", names["peak-1.jpg"], names["guide.pdf"])
	}
	if got := filterItems(items, []string{"Images"}, "", 0); len(got) != 3 {
		t.Errorf("filter images: %d", len(got))
	}
}
