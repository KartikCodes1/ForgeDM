package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// wailsUI adapts the Wails runtime to the app.UI interface.
type wailsUI struct {
	mu       sync.RWMutex
	ctx      context.Context
	quitting atomic.Bool
}

func (u *wailsUI) setContext(ctx context.Context) {
	u.mu.Lock()
	u.ctx = ctx
	u.mu.Unlock()
}

func (u *wailsUI) c() context.Context {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.ctx
}

func (u *wailsUI) Emit(event string, data any) {
	if ctx := u.c(); ctx != nil {
		runtime.EventsEmit(ctx, event, data)
	}
}

func (u *wailsUI) Show() {
	ctx := u.c()
	if ctx == nil {
		return
	}
	runtime.WindowShow(ctx)
	runtime.WindowUnminimise(ctx)
	// Briefly pin on top so the window comes to the front.
	runtime.WindowSetAlwaysOnTop(ctx, true)
	go func() {
		time.Sleep(400 * time.Millisecond)
		runtime.WindowSetAlwaysOnTop(ctx, false)
	}()
}

func (u *wailsUI) Hide() {
	if ctx := u.c(); ctx != nil {
		runtime.WindowHide(ctx)
	}
}

func (u *wailsUI) Notify(title, body string) {
	ctx := u.c()
	if ctx == nil {
		return
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	_ = runtime.SendNotification(ctx, runtime.NotificationOptions{ID: hex.EncodeToString(b), Title: title, Body: body})
}

func (u *wailsUI) ClipboardText() string {
	ctx := u.c()
	if ctx == nil {
		return ""
	}
	s, _ := runtime.ClipboardGetText(ctx)
	return s
}

func (u *wailsUI) ChooseFolder(title, start string) string {
	ctx := u.c()
	if ctx == nil {
		return ""
	}
	opts := runtime.OpenDialogOptions{Title: title, CanCreateDirectories: true}
	if st, err := os.Stat(start); err == nil && st.IsDir() {
		opts.DefaultDirectory = start
	}
	p, _ := runtime.OpenDirectoryDialog(ctx, opts)
	return p
}

func (u *wailsUI) OpenFile(title, pattern string) string {
	ctx := u.c()
	if ctx == nil {
		return ""
	}
	p, _ := runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: title, Filters: []runtime.FileFilter{{DisplayName: "Supported files (" + pattern + ")", Pattern: pattern}}})
	return p
}

func (u *wailsUI) SaveFile(title, name string) string {
	ctx := u.c()
	if ctx == nil {
		return ""
	}
	p, _ := runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{Title: title, DefaultFilename: name, Filters: []runtime.FileFilter{{DisplayName: "JSON (*.json)", Pattern: "*.json"}}})
	return p
}

func (u *wailsUI) Quit() {
	u.quitting.Store(true)
	if ctx := u.c(); ctx != nil {
		runtime.Quit(ctx)
	}
}
