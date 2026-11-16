// Package platform wraps operating-system features ForgeDM needs: sparse
// files, keeping the PC awake, power actions, opening files, antivirus scans,
// startup registration, proxy discovery, metered-network detection and wake
// timers. Windows gets the real implementations; other systems get safe
// fallbacks so the engine can be developed and tested anywhere.
package platform

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// ScanResult is the outcome of an antivirus scan.
type ScanResult struct {
	Clean     bool   `json:"clean"`
	Note      string `json:"note"`
	Available bool   `json:"available"`
}

// DefaultDownloadsDir returns the user's Downloads folder.
func DefaultDownloadsDir() string {
	if d := downloadsDir(); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

// DataDir is where ForgeDM keeps its settings and download list.
func DataDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = "."
	}
	return filepath.Join(base, "ForgeDM")
}

// ProxyFunc returns the proxy function for a mode: "system", "none" or "custom".
func ProxyFunc(mode, custom string) func(*http.Request) (*url.URL, error) {
	switch mode {
	case "none":
		return nil
	case "custom":
		u, err := url.Parse(custom)
		if err != nil || u.Host == "" {
			return nil
		}
		return http.ProxyURL(u)
	default:
		if p := systemProxy(); p != nil {
			return p
		}
		return http.ProxyFromEnvironment
	}
}

// Wake represents a pending wake-from-sleep timer.
type Wake interface{ Cancel() }

// ScheduleWake asks the PC to wake from sleep at t (best effort).
func ScheduleWake(t time.Time) Wake { return scheduleWake(t) }
