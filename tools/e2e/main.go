//go:build e2e

// Command e2e runs the ForgeDM core without a window, for end-to-end tests of
// the browser extension. It approves pairing and captured downloads
// automatically, serves a test page with a download link on :8765, and
// reports the download list on :8766/downloads.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/KartikCodes1/forgedm/internal/app"
	"github.com/KartikCodes1/forgedm/internal/config"
)

type autoUI struct{ a *app.App }

func (u *autoUI) Emit(ev string, data any) {
	b, _ := json.Marshal(data)
	switch ev {
	case "pair":
		var p struct{ ID, Name string }
		json.Unmarshal(b, &p)
		log.Printf("pair request from %s: approving", p.Name)
		go u.a.AnswerPairing(p.ID, true)
	case "capture":
		var c struct {
			ID    string
			Probe *struct{ Size int64 } `json:"probe"`
		}
		json.Unmarshal(b, &c)
		if c.Probe != nil && c.Probe.Size > 0 {
			log.Printf("capture %s: starting", c.ID)
			go u.a.AnswerCapture(c.ID, app.AddOptions{}, "start")
		}
	case "batch":
		log.Printf("batch: %s", b)
	case "tick", "state":
	default:
		log.Printf("event %s: %s", ev, b)
	}
}
func (u *autoUI) Show()                           {}
func (u *autoUI) Hide()                           {}
func (u *autoUI) Notify(t, b string)              { log.Printf("notify: %s — %s", t, b) }
func (u *autoUI) ClipboardText() string           { return "" }
func (u *autoUI) ChooseFolder(_, _ string) string { return "" }
func (u *autoUI) OpenFile(_, _ string) string     { return "" }
func (u *autoUI) SaveFile(_, _ string) string     { return "" }
func (u *autoUI) Quit()                           { os.Exit(0) }

func main() {
	data := os.Args[1]
	dl := os.Args[2]
	_ = os.MkdirAll(data, 0o755)
	b, _ := json.Marshal(map[string]any{"settings": map[string]any{"baseDir": dl, "apiPort": 52771}})
	if _, err := os.Stat(data + "/config.json"); os.IsNotExist(err) {
		_ = os.WriteFile(data+"/config.json", b, 0o644)
	}
	ui := &autoUI{}
	core, err := app.NewCore(ui, app.Options{DataDir: data})
	if err != nil {
		log.Fatal(err)
	}
	ui.a = app.NewApp(core)
	_ = config.NewID
	core.Startup(context.Background())

	payload := make([]byte, 3<<20)
	_, _ = rand.Read(payload)
	files := http.NewServeMux()
	files.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><title>Test page</title><h1>Downloads</h1>
<a id="dl" href="/files/test-archive.zip">test-archive.zip</a>
<a href="/files/manual.pdf">manual</a>`)
	})
	files.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "" {
			log.Printf("download request without cookies from %s", r.UserAgent())
		}
		w.Header().Set("Content-Disposition", "attachment")
		http.ServeContent(w, r, r.URL.Path, time.Now(), bytes.NewReader(payload))
	})
	go http.ListenAndServe("127.0.0.1:8765", files)

	report := http.NewServeMux()
	report.HandleFunc("/downloads", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ui.a.GetSnapshot().Downloads)
	})
	log.Println("e2e harness ready")
	log.Fatal(http.ListenAndServe("127.0.0.1:8766", report))
}
