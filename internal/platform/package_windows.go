//go:build windows

package platform

// Support for running as an MSIX package (the Microsoft Store version).
// Packaged apps get their AppData writes redirected into a private folder and
// can't use the registry Run key, so a few things work differently there.

import (
	"errors"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetCurrentPackageFamilyName = kernel32.NewProc("GetCurrentPackageFamilyName")

// Must match the package manifest (build/windows/msix/AppxManifest.xml).
const (
	packageAppID  = "ForgeDM"
	startupTaskID = "ForgeDMStartup"
)

var (
	iidStartupTaskStatics = windows.GUID{Data1: 0xee5b60bd, Data2: 0xa148, Data3: 0x41a7, Data4: [8]byte{0xb2, 0x6e, 0xe8, 0xb8, 0x8a, 0x1e, 0x62, 0xf8}}
	iidAppInstanceStatics = windows.GUID{Data1: 0x9d11e77f, Data2: 0x9ea6, Data3: 0x47af, Data4: [8]byte{0xa6, 0xec, 0x46, 0x78, 0x4c, 0x5b, 0xa2, 0x54}}
)

// Windows.ApplicationModel.StartupTaskState
const (
	startupDisabled         = 0
	startupDisabledByUser   = 1
	startupEnabled          = 2
	startupDisabledByPolicy = 3
	startupEnabledByPolicy  = 4
)

var (
	pkgOnce   sync.Once
	pkgFamily string
)

func packageFamily() string {
	pkgOnce.Do(func() {
		if procGetCurrentPackageFamilyName.Find() != nil {
			return
		}
		n := uint32(256)
		buf := make([]uint16, n)
		r, _, _ := procGetCurrentPackageFamilyName.Call(uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])))
		if r == 0 && n > 0 && int(n) <= len(buf) {
			pkgFamily = windows.UTF16ToString(buf[:n])
		}
	})
	return pkgFamily
}

// Packaged reports whether ForgeDM runs as an MSIX package (Microsoft Store).
func Packaged() bool { return packageFamily() != "" }

// PackageAppID returns the app's AppUserModelID when packaged, else "".
func PackageAppID() string {
	if f := packageFamily(); f != "" {
		return f + "!" + packageAppID
	}
	return ""
}

// packagedDataDir is the package's own LocalState folder. It's a real path,
// so Explorer and browsers can open things ForgeDM puts there (such as the
// unpacked browser extension), unlike redirected AppData writes.
func packagedDataDir() string {
	f := packageFamily()
	if f == "" {
		return ""
	}
	local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil || local == "" {
		return ""
	}
	return filepath.Join(local, "Packages", f, "LocalState", "ForgeDM")
}

// withStartupTask runs fn with the package's StartupTask object.
func withStartupTask(fn func(task com) error) error {
	return inMTA(func() error {
		f, err := activationFactory("Windows.ApplicationModel.StartupTask", &iidStartupTaskStatics)
		if err != nil {
			return err
		}
		defer f.release()
		id, err := newHString(startupTaskID)
		if err != nil {
			return err
		}
		defer deleteHString(id)
		var p unsafe.Pointer
		if err := f.call(7, uintptr(id), uintptr(unsafe.Pointer(&p))); err != nil { // GetAsync
			return err
		}
		op := com{p}
		defer op.release()
		if err := await(op, 10*time.Second); err != nil {
			return err
		}
		var t unsafe.Pointer
		if err := op.call(8, uintptr(unsafe.Pointer(&t))); err != nil { // GetResults
			return err
		}
		if t == nil {
			return errors.New("the startup task isn't in the package manifest")
		}
		task := com{t}
		defer task.release()
		return fn(task)
	})
}

func startupTaskState() (int32, error) {
	var s int32
	err := withStartupTask(func(t com) error { return t.call(8, uintptr(unsafe.Pointer(&s))) }) // get_State
	return s, err
}

func setStartupTask(on bool) error {
	return withStartupTask(func(t com) error {
		var s int32
		if err := t.call(8, uintptr(unsafe.Pointer(&s))); err != nil {
			return err
		}
		if !on {
			if s == startupEnabled {
				return t.call(7) // Disable
			}
			return nil
		}
		switch s {
		case startupEnabled, startupEnabledByPolicy:
			return nil
		case startupDisabledByUser, startupDisabledByPolicy:
			return ErrStartupBlocked
		}
		var p unsafe.Pointer
		if err := t.call(6, uintptr(unsafe.Pointer(&p))); err != nil { // RequestEnableAsync
			return err
		}
		op := com{p}
		defer op.release()
		if err := await(op, 30*time.Second); err != nil {
			return err
		}
		var ns int32
		if err := op.call(8, uintptr(unsafe.Pointer(&ns))); err != nil {
			return err
		}
		if ns != startupEnabled && ns != startupEnabledByPolicy {
			return ErrStartupBlocked
		}
		return nil
	})
}

// StartupEnabled reports whether Windows will start the packaged app at
// sign-in. ok is false when not packaged or the state can't be read.
func StartupEnabled() (enabled, ok bool) {
	if !Packaged() {
		return false, false
	}
	s, err := startupTaskState()
	if err != nil {
		return false, false
	}
	return s == startupEnabled || s == startupEnabledByPolicy, true
}

// LaunchedAtStartup reports whether Windows started the packaged app at
// sign-in (through its startup task), in which case it should start hidden.
func LaunchedAtStartup() bool {
	if !Packaged() {
		return false
	}
	kind := int32(-1)
	_ = inMTA(func() error {
		f, err := activationFactory("Windows.ApplicationModel.AppInstance", &iidAppInstanceStatics)
		if err != nil {
			return err
		}
		defer f.release()
		var p unsafe.Pointer
		if err := f.call(7, uintptr(unsafe.Pointer(&p))); err != nil || p == nil { // GetActivatedEventArgs
			return err
		}
		args := com{p}
		defer args.release()
		return args.call(6, uintptr(unsafe.Pointer(&kind))) // IActivatedEventArgs.Kind
	})
	return kind == 1020 // ActivationKind.StartupTask
}

// StartupDiagnostics exercises the startup task (used by `ForgeDM.exe
// --diagnose` in CI): it reads the state, enables, and restores it.
func StartupDiagnostics() map[string]any {
	out := map[string]any{}
	before, err := startupTaskState()
	out["before"], out["readError"] = before, errString(err)
	out["enableError"] = errString(setStartupTask(true))
	after, _ := startupTaskState()
	out["afterEnable"] = after
	if before != startupEnabled {
		out["disableError"] = errString(setStartupTask(false))
		restored, _ := startupTaskState()
		out["afterDisable"] = restored
	}
	return out
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
