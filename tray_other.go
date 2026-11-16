//go:build !windows

package main

import "github.com/KartikCodes1/forgedm/internal/app"

// The tray icon is only implemented for Windows.
func startTray(core *app.Core, bridge *app.App, ui *wailsUI) {}
func stopTray()                                              {}
func showError(msg string)                                   {}
