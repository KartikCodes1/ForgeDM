package main

import (
	"os"

	"git.sr.ht/~jackmordaunt/go-toast/v2"

	"github.com/KartikCodes1/forgedm/internal/platform"
)

// usePackageNotifications makes notifications work in the Store (MSIX)
// version. Packaged apps must show toasts under their package's app ID;
// Wails registers one based on the exe name, which only works unpackaged.
func usePackageNotifications() {
	aumid := platform.PackageAppID()
	if aumid == "" {
		return
	}
	exe, _ := os.Executable()
	_ = toast.SetAppData(toast.AppData{AppID: aumid, ActivationExe: exe})
}

// testNotification shows a notification the way the packaged app does.
func testNotification() error {
	usePackageNotifications()
	n := toast.Notification{AppID: platform.PackageAppID(), Title: "ForgeDM", Body: "Notifications are working."}
	return n.Push()
}
