//go:build e2e

// Command devbridge runs the real ForgeDM core and exposes the bound App
// methods over HTTP (POST /call/Method) and its events over Server-Sent Events
// (GET /events), so the UI can be tested in a normal browser against the real
// backend.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/KartikCodes1/forgedm/internal/app"
)

type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]bool
}

func (h *hub) send(ev string, data any) {
	b, _ := json.Marshal(map[string]any{"event": ev, "data": data})
	h.mu.Lock()
	for c := range h.subs {
		select {
		case c <- b:
		default:
		}
	}
	h.mu.Unlock()
}

type ui struct {
	h    *hub
	clip string
}

func (u *ui) Emit(ev string, data any)        { u.h.send(ev, data) }
func (u *ui) Show()                           {}
func (u *ui) Hide()                           {}
func (u *ui) Notify(t, b string)              { log.Printf("notify: %s — %s", t, b) }
func (u *ui) ClipboardText() string           { return u.clip }
func (u *ui) ChooseFolder(_, s string) string { return s }
func (u *ui) OpenFile(_, _ string) string     { return "" }
func (u *ui) SaveFile(_, _ string) string     { return "" }
func (u *ui) Quit()                           { os.Exit(0) }

func main() {
	data, dl := os.Args[1], os.Args[2]
	_ = os.MkdirAll(data, 0o755)
	if _, err := os.Stat(data + "/config.json"); os.IsNotExist(err) {
		b, _ := json.Marshal(map[string]any{"settings": map[string]any{"baseDir": dl, "apiPort": 52771}})
		_ = os.WriteFile(data+"/config.json", b, 0o644)
	}
	h := &hub{subs: map[chan []byte]bool{}}
	u := &ui{h: h}
	core, err := app.NewCore(u, app.Options{DataDir: data})
	if err != nil {
		log.Fatal(err)
	}
	a := app.NewApp(core)
	core.Startup(context.Background())

	cors := func(w http.ResponseWriter) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}
	http.HandleFunc("/call/", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		if r.Method == http.MethodOptions {
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/call/")
		m := reflect.ValueOf(a).MethodByName(name)
		if !m.IsValid() {
			http.Error(w, "no method "+name, 404)
			return
		}
		var raw []json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&raw)
		t := m.Type()
		args := make([]reflect.Value, t.NumIn())
		for i := range args {
			p := reflect.New(t.In(i))
			if i < len(raw) {
				if err := json.Unmarshal(raw[i], p.Interface()); err != nil {
					http.Error(w, fmt.Sprintf("arg %d: %v", i, err), 400)
					return
				}
			}
			args[i] = p.Elem()
		}
		out := m.Call(args)
		res := map[string]any{}
		for _, o := range out {
			if o.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
				if !o.IsNil() {
					res["error"] = o.Interface().(error).Error()
				}
			} else {
				res["result"] = o.Interface()
			}
		}
		json.NewEncoder(w).Encode(res)
	})
	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		w.Header().Set("Content-Type", "text/event-stream")
		c := make(chan []byte, 256)
		h.mu.Lock()
		h.subs[c] = true
		h.mu.Unlock()
		defer func() { h.mu.Lock(); delete(h.subs, c); h.mu.Unlock() }()
		fl := w.(http.Flusher)
		fl.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case b := <-c:
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
		}
	})
	http.HandleFunc("/clipboard", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		u.clip = r.URL.Query().Get("text")
	})
	payload := make([]byte, 24<<20)
	for i := range payload {
		payload[i] = byte(i * 31)
	}
	files := http.NewServeMux()
	files.HandleFunc("/gallery/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><a href="/files/guide.pdf">guide</a><img src="/files/peak-1.jpg"><img src="/files/peak-2.jpg"><a href="/files/pack.zip">pack</a></body></html>`)
	})
	files.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		n := len(payload)
		if strings.HasSuffix(r.URL.Path, ".jpg") || strings.HasSuffix(r.URL.Path, ".pdf") {
			n = 300 << 10
		}
		// Throttle so the progress window has something to show.
		http.ServeContent(slowWriter{w}, r, r.URL.Path, time0, bytesReader(payload[:n]))
	})
	go http.ListenAndServe("127.0.0.1:8765", files)
	log.Println("devbridge on :8790")
	log.Fatal(http.ListenAndServe("127.0.0.1:8790", nil))
}

var time0 = time.Now()

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

type slowWriter struct{ http.ResponseWriter }

func (s slowWriter) Write(p []byte) (int, error) {
	time.Sleep(time.Duration(len(p)) * time.Second / (1 << 20)) // ~1 MB/s per connection
	return s.ResponseWriter.Write(p)
}
