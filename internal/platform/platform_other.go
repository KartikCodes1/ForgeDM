//go:build !windows

package platform

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// MakeSparse is a no-op outside Windows (most Unix filesystems are sparse already).
func MakeSparse(f *os.File) {}

// KeepAwake is a no-op outside Windows.
func KeepAwake(on bool) {}

// Shutdown powers the machine off.
func Shutdown() error { return exec.Command("shutdown", "-h", "+1").Run() }

// Sleep suspends the machine.
func Sleep() error { return exec.Command("systemctl", "suspend").Run() }

// Hibernate hibernates the machine.
func Hibernate() error { return exec.Command("systemctl", "hibernate").Run() }

// OpenPath opens a file with its default app.
func OpenPath(p string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", p).Start()
	}
	return exec.Command("xdg-open", p).Start()
}

// ShowInFolder opens the folder containing p.
func ShowInFolder(p string) error { return OpenPath(filepath.Dir(p)) }

// Scan runs an antivirus scan (only available on Windows).
func Scan(p string) ScanResult { return ScanResult{Clean: true, Note: "No antivirus scanner available", Available: false} }

// SetStartup registers ForgeDM to start with the system (Windows only).
func SetStartup(enabled bool, exe string) error { return nil }

// IsMetered reports whether the current connection is metered (Windows only).
func IsMetered() bool { return false }

func downloadsDir() string { return "" }

func systemProxy() func(*http.Request) (*url.URL, error) { return nil }

type noWake struct{}

func (noWake) Cancel() {}

func scheduleWake(t time.Time) Wake { return noWake{} }

// HideWindow configures cmd to run without flashing a console window.
func HideWindow(cmd *exec.Cmd) {}
