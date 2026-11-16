//go:build windows

package platform

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
	procCreateWaitableTimerW    = kernel32.NewProc("CreateWaitableTimerW")
	procSetWaitableTimer        = kernel32.NewProc("SetWaitableTimer")
	procCancelWaitableTimer     = kernel32.NewProc("CancelWaitableTimer")
	powrprof                    = windows.NewLazySystemDLL("powrprof.dll")
	procSetSuspendState         = powrprof.NewProc("SetSuspendState")
)

const (
	fsctlSetSparse     = 0x000900C4
	esContinuous       = 0x80000000
	esSystemRequired   = 0x00000001
	createNoWindow     = 0x08000000
)

// HideWindow configures cmd to run without flashing a console window.
func HideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

// MakeSparse marks a file sparse so writing far into a new file doesn't make
// NTFS fill the gap with zeros first (that would stall multi-part downloads).
func MakeSparse(f *os.File) {
	var ret uint32
	_ = windows.DeviceIoControl(windows.Handle(f.Fd()), fsctlSetSparse, nil, 0, nil, 0, &ret, nil)
}

var (
	awakeOnce sync.Once
	awakeCh   = make(chan bool, 8)
)

// KeepAwake stops Windows from sleeping while downloads run.
func KeepAwake(on bool) {
	awakeOnce.Do(func() {
		go func() {
			// The execution state belongs to a thread, so keep using one.
			runtime.LockOSThread()
			for v := range awakeCh {
				flags := uintptr(esContinuous)
				if v {
					flags |= esSystemRequired
				}
				procSetThreadExecutionState.Call(flags)
			}
		}()
	})
	select {
	case awakeCh <- on:
	default:
	}
}

// Shutdown turns the PC off after a one-minute warning.
func Shutdown() error {
	cmd := exec.Command("shutdown", "/s", "/t", "60", "/c", "ForgeDM finished downloading. Windows will shut down in 1 minute.")
	HideWindow(cmd)
	return cmd.Run()
}

// Sleep puts the PC to sleep.
func Sleep() error {
	r, _, err := procSetSuspendState.Call(0, 0, 0)
	if r == 0 {
		return err
	}
	return nil
}

// Hibernate hibernates the PC.
func Hibernate() error {
	r, _, err := procSetSuspendState.Call(1, 0, 0)
	if r == 0 {
		return err
	}
	return nil
}

// OpenPath opens a file or folder with its default app.
func OpenPath(p string) error {
	return windows.ShellExecute(0, windows.StringToUTF16Ptr("open"), windows.StringToUTF16Ptr(p), nil, windows.StringToUTF16Ptr(filepath.Dir(p)), windows.SW_SHOWNORMAL)
}

// ShowInFolder opens Explorer with the file selected.
func ShowInFolder(p string) error {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + p + `"`}
	return cmd.Start()
}

func defenderPath() string {
	// Newer Defender builds live under ProgramData\...\Platform\<version>.
	base := filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Windows Defender", "Platform")
	if entries, err := os.ReadDir(base); err == nil {
		for i := len(entries) - 1; i >= 0; i-- {
			p := filepath.Join(base, entries[i].Name(), "MpCmdRun.exe")
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	p := filepath.Join(os.Getenv("ProgramFiles"), "Windows Defender", "MpCmdRun.exe")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// Scan checks a file with Microsoft Defender.
func Scan(p string) ScanResult {
	exe := defenderPath()
	if exe == "" {
		return ScanResult{Clean: true, Note: "Microsoft Defender isn't available", Available: false}
	}
	cmd := exec.Command(exe, "-Scan", "-ScanType", "3", "-File", p, "-DisableRemediation")
	HideWindow(cmd)
	err := cmd.Run()
	if err == nil {
		return ScanResult{Clean: true, Note: "No threats found", Available: true}
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 2 {
		return ScanResult{Clean: false, Note: "Microsoft Defender found a threat in this file", Available: true}
	}
	return ScanResult{Clean: true, Note: "Scan couldn't run", Available: false}
}

// SetStartup adds or removes ForgeDM from the user's startup apps.
func SetStartup(enabled bool, exe string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !enabled {
		err := k.DeleteValue("ForgeDM")
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	return k.SetStringValue("ForgeDM", `"`+exe+`" --minimized`)
}

func downloadsDir() string {
	p, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, 0)
	if err != nil {
		return ""
	}
	return p
}

// systemProxy reads the proxy configured in Windows Internet Settings.
func systemProxy() func(*http.Request) (*url.URL, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	enabled, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return nil
	}
	server, _, err := k.GetStringValue("ProxyServer")
	if err != nil || server == "" {
		return nil
	}
	override, _, _ := k.GetStringValue("ProxyOverride")
	byScheme := map[string]string{}
	if strings.Contains(server, "=") {
		for _, part := range strings.Split(server, ";") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				byScheme[strings.ToLower(strings.TrimSpace(kv[0]))] = strings.TrimSpace(kv[1])
			}
		}
	} else {
		byScheme["http"], byScheme["https"] = server, server
	}
	var bypass []string
	for _, b := range strings.Split(override, ";") {
		if b = strings.TrimSpace(strings.ToLower(b)); b != "" {
			bypass = append(bypass, b)
		}
	}
	return func(r *http.Request) (*url.URL, error) {
		host := strings.ToLower(r.URL.Hostname())
		for _, b := range bypass {
			if b == "<local>" && !strings.Contains(host, ".") {
				return nil, nil
			}
			if ok, _ := filepath.Match(b, host); ok {
				return nil, nil
			}
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil, nil
		}
		s := byScheme[r.URL.Scheme]
		if s == "" {
			s = byScheme["http"]
		}
		if s == "" {
			return nil, nil
		}
		if !strings.Contains(s, "://") {
			s = "http://" + s
		}
		return url.Parse(s)
	}
}

var (
	meteredMu    sync.Mutex
	meteredAt    time.Time
	meteredValue bool
)

// IsMetered asks Windows whether the active connection is metered.
func IsMetered() bool {
	meteredMu.Lock()
	defer meteredMu.Unlock()
	if time.Since(meteredAt) < time.Minute {
		return meteredValue
	}
	script := `$p=[Windows.Networking.Connectivity.NetworkInformation,Windows.Networking.Connectivity,ContentType=WindowsRuntime]::GetInternetConnectionProfile(); if($p){ $c=$p.GetConnectionCost(); Write-Output ([int]$c.NetworkCostType) } else { Write-Output 0 }`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	HideWindow(cmd)
	out, err := cmd.Output()
	meteredAt = time.Now()
	if err != nil {
		meteredValue = false
		return false
	}
	v := strings.TrimSpace(string(out))
	// 1 = unrestricted; 2 = fixed (data cap); 3 = variable (pay per use).
	meteredValue = v == "2" || v == "3"
	return meteredValue
}

type winWake struct{ h uintptr }

func (w *winWake) Cancel() {
	if w.h != 0 {
		procCancelWaitableTimer.Call(w.h)
		windows.CloseHandle(windows.Handle(w.h))
		w.h = 0
	}
}

func scheduleWake(t time.Time) Wake {
	h, _, _ := procCreateWaitableTimerW.Call(0, 1, 0)
	if h == 0 {
		return &winWake{}
	}
	// Absolute FILETIME: 100ns intervals since 1601-01-01 UTC.
	due := windows.NsecToFiletime(t.UnixNano())
	dueTime := int64(due.HighDateTime)<<32 | int64(due.LowDateTime)
	r, _, _ := procSetWaitableTimer.Call(h, uintptr(unsafe.Pointer(&dueTime)), 0, 0, 0, 1)
	if r == 0 {
		windows.CloseHandle(windows.Handle(h))
		return &winWake{}
	}
	return &winWake{h: h}
}
