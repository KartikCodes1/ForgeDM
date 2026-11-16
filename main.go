// ForgeDM — a fast download manager for Windows.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/KartikCodes1/forgedm/internal/app"
	"github.com/KartikCodes1/forgedm/internal/platform"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

//go:embed all:extension/dist
var extensionFiles embed.FS

func hasArg(name string) bool {
	for _, a := range os.Args[1:] {
		if strings.EqualFold(a, name) {
			return true
		}
	}
	return false
}

func argIndex(name string) int {
	for i, a := range os.Args {
		if i > 0 && strings.EqualFold(a, name) {
			return i
		}
	}
	return -1
}

func linkArg(args []string) string {
	for _, a := range args {
		if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") {
			return a
		}
	}
	return ""
}

func main() {
	platform.KeepMTA()
	if hasArg("--widget") {
		runWidget()
		return
	}
	if i := argIndex("--diagnose"); i > 0 && i+1 < len(os.Args) {
		runDiagnose(os.Args[i+1])
		return
	}
	if hasArg("--quit") {
		// Used by the installer and uninstaller to close ForgeDM cleanly.
		if app.QuitRunning() != nil {
			os.Exit(1)
		}
		return
	}
	// Started hidden by the Run key (installer version) or the startup task
	// (Store version) at sign-in.
	minimized := hasArg("--minimized") || platform.LaunchedAtStartup()
	ui := &wailsUI{}
	extFS, _ := fs.Sub(extensionFiles, "extension/dist")
	core, err := app.NewCore(ui, app.Options{StartHidden: minimized, ExtensionFS: extFS})
	if err != nil {
		fail(err)
		return
	}
	bridge := app.NewApp(core)
	startLink := linkArg(os.Args[1:])

	err = wails.Run(&options.App{
		Title:            "ForgeDM",
		Width:            1360,
		Height:           880,
		MinWidth:         980,
		MinHeight:        640,
		Frameless:        true,
		StartHidden:      minimized,
		BackgroundColour: &options.RGBA{R: 10, G: 14, B: 23, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup: func(ctx context.Context) {
			ui.setContext(ctx)
			_ = runtime.InitializeNotifications(ctx)
			usePackageNotifications()
			core.Startup(ctx)
			startTray(core, bridge, ui)
			if startLink != "" {
				ui.Emit("open:add", startLink)
			}
		},
		OnBeforeClose: func(ctx context.Context) bool {
			if ui.quitting.Load() {
				return false
			}
			if core.CloseToTray() {
				runtime.WindowHide(ctx)
				return true
			}
			return false
		},
		OnShutdown: func(ctx context.Context) {
			stopTray()
			core.Shutdown()
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "forgedm-main-5b7e2c41",
			OnSecondInstanceLaunch: func(d options.SecondInstanceData) {
				ui.Show()
				if l := linkArg(d.Args); l != "" {
					ui.Emit("open:add", l)
				}
			},
		},
		Bind: []interface{}{bridge},
		Windows: &windows.Options{
			Theme:               windows.Dark,
			WebviewUserDataPath: filepath.Join(platform.DataDir(), "webview"),
			Messages: &windows.Messages{
				InstallationRequired: "ForgeDM needs Microsoft Edge WebView2, which is part of Windows 10 and 11. It looks like it's missing.",
				UpdateRequired:       "ForgeDM needs a newer version of Microsoft Edge WebView2.",
				MissingRequirements:  "Missing requirements",
				Webview2NotInstalled: "Microsoft Edge WebView2 is not installed",
				Error:                "Error",
				FailedToInstall:      "WebView2 couldn't be installed",
				DownloadPage:         "Open the download page?",
				PressOKToInstall:     "Press OK to install WebView2.",
				ContactAdmin:         "Ask your administrator to install Microsoft Edge WebView2.",
				InvalidFixedWebview2: "The bundled WebView2 is invalid.",
			},
		},
	})
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	msg := fmt.Sprintf("ForgeDM couldn't start: %v", err)
	showError(msg)
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

// runWidget runs the floating drop target in its own small window.
func runWidget() {
	w := app.NewWidget()
	_ = wails.Run(&options.App{
		Title:            "ForgeDM drop target",
		Width:            236,
		Height:           60,
		Frameless:        true,
		AlwaysOnTop:      true,
		DisableResize:    true,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup: func(ctx context.Context) {
			if screens, err := runtime.ScreenGetAll(ctx); err == nil {
				for _, s := range screens {
					if s.IsPrimary {
						runtime.WindowSetPosition(ctx, s.Size.Width-236-28, s.Size.Height-60-96)
					}
				}
			}
		},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "forgedm-widget-5b7e2c41"},
		Bind:               []interface{}{w},
		Windows: &windows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    true,
			Theme:                windows.Dark,
			WebviewUserDataPath:  filepath.Join(platform.DataDir(), "webview-widget"),
		},
	})
}
