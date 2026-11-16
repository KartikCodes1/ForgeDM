// Package api is the small local web server the ForgeDM browser extension and
// the floating drop target talk to. It only listens on 127.0.0.1, accepts
// requests from browser extensions (never from web pages) and requires a
// token the user approved when pairing.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Capture is a download handed over by the browser.
type Capture struct {
	URL       string            `json:"url"`
	Filename  string            `json:"filename,omitempty"`
	Referrer  string            `json:"referrer,omitempty"`
	PageURL   string            `json:"pageUrl,omitempty"`
	PageTitle string            `json:"pageTitle,omitempty"`
	Cookies   string            `json:"cookies,omitempty"`
	UserAgent string            `json:"userAgent,omitempty"`
	Mime      string            `json:"mime,omitempty"`
	Size      int64             `json:"size,omitempty"`
	Kind      string            `json:"kind,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Silent    bool              `json:"silent,omitempty"`
}

// Link is one entry of a batch ("Download all links").
type Link struct {
	URL  string `json:"url"`
	Name string `json:"name,omitempty"`
}

// Batch is several links sent at once.
type Batch struct {
	Links     []Link `json:"links"`
	PageURL   string `json:"pageUrl,omitempty"`
	Cookies   string `json:"cookies,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
}

// ExtConfig is what the extension needs to know to decide what to capture.
type ExtConfig struct {
	CaptureEnabled bool     `json:"captureEnabled"`
	Exts           []string `json:"exts"`
	MinSize        int64    `json:"minSize"`
	VideoButton    bool     `json:"videoButton"`
	AltBypass      bool     `json:"altBypass"`
}

// Status is a small summary for the extension popup and drop target.
type Status struct {
	Speed  float64      `json:"speed"`
	Active int          `json:"active"`
	Queued int          `json:"queued"`
	Items  []StatusItem `json:"items"`
}

// StatusItem is one active download.
type StatusItem struct {
	Name     string  `json:"name"`
	Progress float64 `json:"progress"`
	Speed    float64 `json:"speed"`
}

// Handler is implemented by the app.
type Handler interface {
	Capture(c Capture, browser string) (string, error)
	Batch(b Batch, browser string) error
	Status() Status
	Config() ExtConfig
	Pair(ctx context.Context, name string) (string, error)
	Authorize(token string) (browser string, ok bool)
	Show()
	// Quit closes ForgeDM if token is the app's own internal token (used by
	// the drop target and `ForgeDM.exe --quit`), never a browser's token.
	Quit(token string) bool
}

// Server is the local API server.
type Server struct {
	h    Handler
	srv  *http.Server
	Port int
}

// Version is reported to the extension.
var Version = "1.0.0"

// Start listens on the first free port from port to port+9.
func Start(h Handler, port int) (*Server, error) {
	s := &Server{h: h}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ping", s.ping)
	mux.HandleFunc("/v1/pair", s.pair)
	mux.HandleFunc("/v1/config", s.auth(s.config))
	mux.HandleFunc("/v1/status", s.auth(s.status))
	mux.HandleFunc("/v1/download", s.auth(s.download))
	mux.HandleFunc("/v1/batch", s.auth(s.batch))
	mux.HandleFunc("/v1/show", s.auth(s.show))
	mux.HandleFunc("/v1/quit", s.quit)
	var ln net.Listener
	var err error
	for p := port; p < port+10; p++ {
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			s.Port = p
			break
		}
	}
	if ln == nil {
		return nil, fmt.Errorf("no free port for the browser connection: %w", err)
	}
	s.srv = &http.Server{Handler: s.guard(mux), ReadHeaderTimeout: 10 * time.Second}
	go s.srv.Serve(ln)
	return s, nil
}

// Close stops the server.
func (s *Server) Close() {
	if s != nil && s.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(ctx)
	}
}

// guard rejects anything that isn't from this machine's browser extensions.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			allowed := strings.HasPrefix(origin, "chrome-extension://") || strings.HasPrefix(origin, "moz-extension://") ||
				strings.HasPrefix(origin, "extension://") || strings.HasPrefix(origin, "safari-web-extension://")
			if !allowed {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-ForgeDM-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) auth(fn func(w http.ResponseWriter, r *http.Request, browser string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		browser, ok := s.h.Authorize(r.Header.Get("X-ForgeDM-Token"))
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "not paired"})
			return
		}
		fn(w, r, browser)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	if r.Method != http.MethodPost {
		return errors.New("use POST")
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20))
	return dec.Decode(v)
}

func (s *Server) ping(w http.ResponseWriter, r *http.Request) {
	_, paired := s.h.Authorize(r.Header.Get("X-ForgeDM-Token"))
	writeJSON(w, http.StatusOK, map[string]any{"app": "ForgeDM", "version": Version, "paired": paired})
}

func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	token, err := s.h.Pair(ctx, body.Name)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token})
}

func (s *Server) config(w http.ResponseWriter, r *http.Request, _ string) {
	writeJSON(w, http.StatusOK, s.h.Config())
}

func (s *Server) status(w http.ResponseWriter, r *http.Request, _ string) {
	writeJSON(w, http.StatusOK, s.h.Status())
}

func (s *Server) download(w http.ResponseWriter, r *http.Request, browser string) {
	var c Capture
	if err := readJSON(r, &c); err != nil || c.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing link"})
		return
	}
	action, err := s.h.Capture(c, browser)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": action})
}

func (s *Server) batch(w http.ResponseWriter, r *http.Request, browser string) {
	var b Batch
	if err := readJSON(r, &b); err != nil || len(b.Links) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no links"})
		return
	}
	if err := s.h.Batch(b, browser); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) show(w http.ResponseWriter, r *http.Request, _ string) {
	s.h.Show()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// quit closes ForgeDM cleanly. Only ForgeDM's own helpers may call it, so the
// installer can close a running copy without losing download progress.
func (s *Server) quit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("Origin") != "" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if !s.h.Quit(r.Header.Get("X-ForgeDM-Token")) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
