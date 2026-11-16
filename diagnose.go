package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/KartikCodes1/forgedm/internal/app"
	"github.com/KartikCodes1/forgedm/internal/platform"
)

// runDiagnose writes a JSON report about how ForgeDM sees this PC: package
// identity, data folder, startup task, network cost and notifications.
// CI runs it inside the Store package (`forgedm.exe --diagnose report.json`)
// to check the Windows integrations work there.
func runDiagnose(path string) {
	errText := func(err error) string {
		if err == nil {
			return ""
		}
		return err.Error()
	}
	dataDir := platform.DataDir()
	out := map[string]any{
		"version":           app.Version,
		"packaged":          platform.Packaged(),
		"appId":             platform.PackageAppID(),
		"dataDir":           dataDir,
		"launchedAtStartup": platform.LaunchedAtStartup(),
	}
	probe := filepath.Join(dataDir, "diagnose.txt")
	err := os.MkdirAll(dataDir, 0o755)
	if err == nil {
		err = os.WriteFile(probe, []byte("ForgeDM wrote this file to check its data folder.\n"), 0o644)
	}
	out["dataDirWriteError"] = errText(err)
	cost, err := platform.ConnectionCost()
	out["connectionCost"], out["connectionCostError"] = cost, errText(err)
	out["metered"] = platform.IsMetered()
	if platform.Packaged() {
		out["startup"] = platform.StartupDiagnostics()
		out["notificationError"] = errText(testNotification())
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		os.Exit(1)
	}
}
