//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These run on the Windows CI runner, outside any package.

func TestNotPackaged(t *testing.T) {
	if Packaged() || PackageAppID() != "" {
		t.Fatal("tests shouldn't see a package identity")
	}
	if LaunchedAtStartup() {
		t.Fatal("LaunchedAtStartup without a package")
	}
	if _, ok := StartupEnabled(); ok {
		t.Fatal("StartupEnabled should be unknown without a package")
	}
	cfg, _ := os.UserConfigDir()
	if got, want := DataDir(), filepath.Join(cfg, "ForgeDM"); !strings.EqualFold(got, want) {
		t.Fatalf("DataDir = %s, want %s", got, want)
	}
}

func TestConnectionCost(t *testing.T) {
	cost, err := ConnectionCost()
	if err != nil {
		t.Fatalf("ConnectionCost: %v", err)
	}
	t.Logf("connection cost flags: 0x%x, metered: %v", cost, IsMetered())
}

func TestStartupTaskNeedsPackage(t *testing.T) {
	// Without package identity Windows refuses; the call must fail cleanly.
	if _, err := startupTaskState(); err == nil {
		t.Fatal("expected an error outside a package")
	} else {
		t.Logf("as expected: %v", err)
	}
}
