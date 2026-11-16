//go:build !windows

package platform

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
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
func Scan(p string) ScanResult {
	return ScanResult{Clean: true, Note: "No antivirus scanner available", Available: false}
}

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

// DiskSpace returns free and total bytes for the drive holding path.
func DiskSpace(path string) (free, total uint64) {
	var st syscall.Statfs_t
	for p := path; p != "" && p != filepath.Dir(p); p = filepath.Dir(p) {
		if syscall.Statfs(p, &st) == nil {
			return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize)
		}
	}
	if syscall.Statfs("/", &st) == nil {
		return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize)
	}
	return 0, 0
}

// TakeInstallerStartup returns the installer's start-with-the-system choice
// (Windows only).
func TakeInstallerStartup() (on, ok bool) { return false, false }

// Packaged reports whether ForgeDM runs as an MSIX package (Windows only).
func Packaged() bool { return false }

// PackageAppID returns the app's AppUserModelID when packaged (Windows only).
func PackageAppID() string { return "" }

func packagedDataDir() string { return "" }

// StartupEnabled reports the packaged app's startup task state (Windows only).
func StartupEnabled() (enabled, ok bool) { return false, false }

// LaunchedAtStartup reports whether Windows started the packaged app at
// sign-in (Windows only).
func LaunchedAtStartup() bool { return false }

// StartupDiagnostics exercises the startup task (Windows only).
func StartupDiagnostics() map[string]any { return map[string]any{"supported": false} }

// ConnectionCost returns Windows' NetworkCostType for the internet
// connection (Windows only).
func ConnectionCost() (uint32, error) { return 0, nil }

// KeepMTA keeps a multithreaded COM apartment alive (Windows only).
func KeepMTA() {}
