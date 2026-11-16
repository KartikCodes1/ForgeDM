package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/KartikCodes1/forgedm/internal/api"
	"github.com/KartikCodes1/forgedm/internal/platform"
)

// Widget is bound into the floating drop target window (a separate process).
// It talks to the main ForgeDM process over the local API.
type Widget struct {
	mu     sync.Mutex
	port   int
	token  string
	client *http.Client
}

// NewWidget reads the connection details the main app saved.
func NewWidget() *Widget {
	w := &Widget{client: &http.Client{Timeout: 4 * time.Second}}
	w.load()
	return w
}

func (w *Widget) load() {
	b, err := os.ReadFile(filepath.Join(platform.DataDir(), "config.json"))
	if err != nil {
		return
	}
	var st struct {
		Settings struct {
			APIPort       int    `json:"apiPort"`
			InternalToken string `json:"internalToken"`
		} `json:"settings"`
	}
	if json.Unmarshal(b, &st) == nil {
		w.mu.Lock()
		w.port, w.token = st.Settings.APIPort, st.Settings.InternalToken
		w.mu.Unlock()
	}
}

func (w *Widget) do(method, path string, body any, out any) error {
	w.mu.Lock()
	port, token := w.port, w.token
	w.mu.Unlock()
	if port == 0 {
		w.load()
		return errors.New("ForgeDM isn't running")
	}
	var lastErr error
	for p := port; p < port+10; p++ {
		var rd *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		} else {
			rd = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", p, path), rd)
		req.Header.Set("X-ForgeDM-Token", token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := w.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			w.load()
			return errors.New("not authorized")
		}
		if resp.StatusCode >= 300 {
			return fmt.Errorf("ForgeDM answered %s", resp.Status)
		}
		if p != port {
			w.mu.Lock()
			w.port = p
			w.mu.Unlock()
		}
		if out != nil {
			return json.NewDecoder(resp.Body).Decode(out)
		}
		return nil
	}
	return lastErr
}

// Status returns the current speed and counts.
func (w *Widget) Status() (*api.Status, error) {
	var s api.Status
	if err := w.do(http.MethodGet, "/v1/status", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Drop downloads every link found in dropped text. Returns how many were added.
func (w *Widget) Drop(text string) (int, error) {
	n := 0
	for _, f := range strings.Fields(text) {
		if !strings.HasPrefix(f, "http://") && !strings.HasPrefix(f, "https://") {
			continue
		}
		if err := w.do(http.MethodPost, "/v1/download", api.Capture{URL: f, Silent: true}, nil); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// OpenMain brings the main window to the front.
func (w *Widget) OpenMain() error { return w.do(http.MethodPost, "/v1/show", map[string]any{}, nil) }

// QuitRunning asks a running ForgeDM to save and close. It returns an error
// if no copy is running. The installer uses it through `ForgeDM.exe --quit`.
func QuitRunning() error {
	w := &Widget{client: &http.Client{Timeout: 3 * time.Second}}
	w.load()
	return w.do(http.MethodPost, "/v1/quit", map[string]any{}, nil)
}
