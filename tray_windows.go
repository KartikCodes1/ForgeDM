//go:build windows

package main

import (
	"fmt"
	goruntime "runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/energye/systray"
	"golang.org/x/sys/windows"

	"github.com/KartikCodes1/forgedm/internal/app"
)

var trayStop = make(chan struct{})

// startTray shows the ForgeDM icon in the notification area with a menu.
func startTray(core *app.Core, bridge *app.App, ui *wailsUI) {
	go func() {
		goruntime.LockOSThread()
		systray.Run(func() { trayReady(core, bridge, ui) }, func() {})
	}()
}

func stopTray() {
	select {
	case <-trayStop:
	default:
		close(trayStop)
	}
	systray.Quit()
}

func trayReady(core *app.Core, bridge *app.App, ui *wailsUI) {
	systray.SetIcon(trayIcon)
	systray.SetTitle("ForgeDM")
	systray.SetTooltip("ForgeDM")
	systray.SetOnClick(func(menu systray.IMenu) { ui.Show() })
	systray.SetOnDClick(func(menu systray.IMenu) { ui.Show() })
	systray.SetOnRClick(func(menu systray.IMenu) { _ = menu.ShowMenu() })

	open := systray.AddMenuItem("Open ForgeDM", "")
	add := systray.AddMenuItem("New download…", "")
	systray.AddSeparator()
	pause := systray.AddMenuItem("Pause all", "")
	resume := systray.AddMenuItem("Resume all", "")
	snap := bridge.GetSnapshot()
	limit := systray.AddMenuItemCheckbox("Limit speed", "", snap.Settings.SpeedLimitOn)
	drop := systray.AddMenuItemCheckbox("Show drop target", "", snap.Settings.DropTarget)
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit ForgeDM", "")

	open.Click(func() { ui.Show() })
	add.Click(func() {
		ui.Show()
		ui.Emit("open:add", "")
	})
	pause.Click(func() { bridge.PauseAll() })
	resume.Click(func() { bridge.ResumeAll() })
	limit.Click(func() {
		on := !limit.Checked()
		bridge.SetSpeedLimiter(on)
		if on {
			limit.Check()
		} else {
			limit.Uncheck()
		}
	})
	drop.Click(func() {
		on := !drop.Checked()
		bridge.SetDropTarget(on)
		if on {
			drop.Check()
		} else {
			drop.Uncheck()
		}
	})
	quit.Click(func() { ui.Quit() })

	// Keep the tooltip showing the live speed.
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-trayStop:
				return
			case <-t.C:
				s := core.TrayStatus()
				tip := "ForgeDM"
				if s.Active > 0 {
					tip = fmt.Sprintf("ForgeDM · %s · %d downloading", s.Speed, s.Active)
				}
				systray.SetTooltip(strings.TrimSpace(tip))
				st := bridge.GetSnapshot().Settings
				if st.SpeedLimitOn != limit.Checked() {
					if st.SpeedLimitOn {
						limit.Check()
					} else {
						limit.Uncheck()
					}
				}
			}
		}
	}()
}

func showError(msg string) {
	title, _ := windows.UTF16PtrFromString("ForgeDM")
	text, _ := windows.UTF16PtrFromString(msg)
	user32 := windows.NewLazySystemDLL("user32.dll")
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
