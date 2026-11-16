//go:build windows

package platform

// Minimal COM / Windows Runtime plumbing, without cgo. Every call runs on a
// dedicated OS thread in the multithreaded apartment (see inMTA) so it never
// changes the apartment of the UI thread, which WebView2 needs as STA.

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	combase                    = windows.NewLazySystemDLL("combase.dll")
	procRoInitialize           = combase.NewProc("RoInitialize")
	procRoUninitialize         = combase.NewProc("RoUninitialize")
	procRoGetActivationFactory = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString    = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString    = combase.NewProc("WindowsDeleteString")
)

// IAsyncInfo, from windows.foundation.idl.
var iidAsyncInfo = windows.GUID{Data1: 0x00000036, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}

// HRESULTError is a failed COM / Windows Runtime call.
type HRESULTError uint32

func (e HRESULTError) Error() string { return fmt.Sprintf("Windows error 0x%08X", uint32(e)) }

func hresult(r uintptr) error {
	if int32(uint32(r)) < 0 {
		return HRESULTError(uint32(r))
	}
	return nil
}

// com is a COM interface pointer.
type com struct{ p unsafe.Pointer }

// call invokes vtable method i with the object as the first argument.
func (o com) call(i int, args ...uintptr) error {
	vtbl := *(*unsafe.Pointer)(o.p)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(i)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(o.p)}, args...)...)
	return hresult(r)
}

func (o com) release() {
	if o.p != nil {
		_ = o.call(2)
	}
}

func (o com) query(iid *windows.GUID) (com, error) {
	var p unsafe.Pointer
	err := o.call(0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&p)))
	return com{p}, err
}

// inMTA runs fn on a fresh OS thread initialized for the Windows Runtime.
// The thread is discarded afterwards, so no apartment state leaks.
func inMTA(fn func() error) error {
	done := make(chan error, 1)
	go func() {
		// Never unlocked: the thread exits with this goroutine.
		runtime.LockOSThread()
		r, _, _ := procRoInitialize.Call(1) // RO_INIT_MULTITHREADED
		if err := hresult(r); err != nil {
			done <- fmt.Errorf("RoInitialize: %w", err)
			return
		}
		defer procRoUninitialize.Call()
		done <- fn()
	}()
	return <-done
}

func newHString(s string) (unsafe.Pointer, error) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return nil, err
	}
	var h unsafe.Pointer
	r, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	return h, hresult(r)
}

func deleteHString(h unsafe.Pointer) {
	if h != nil {
		procWindowsDeleteString.Call(uintptr(h))
	}
}

// activationFactory returns the statics interface iid of a runtime class.
func activationFactory(class string, iid *windows.GUID) (com, error) {
	h, err := newHString(class)
	if err != nil {
		return com{}, err
	}
	defer deleteHString(h)
	var p unsafe.Pointer
	r, _, _ := procRoGetActivationFactory.Call(uintptr(h), uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&p)))
	if err := hresult(r); err != nil {
		return com{}, fmt.Errorf("%s: %w", class, err)
	}
	return com{p}, nil
}

// await waits for a Windows Runtime IAsyncOperation to finish. On success,
// call GetResults (vtable slot 8) on the operation.
func await(op com, timeout time.Duration) error {
	info, err := op.query(&iidAsyncInfo)
	if err != nil {
		return err
	}
	defer info.release()
	deadline := time.Now().Add(timeout)
	for {
		var status int32
		if err := info.call(7, uintptr(unsafe.Pointer(&status))); err != nil {
			return err
		}
		switch status {
		case 1: // Completed
			return nil
		case 2: // Canceled
			return errors.New("the operation was canceled")
		case 3: // Error
			var code int32
			_ = info.call(8, uintptr(unsafe.Pointer(&code)))
			return HRESULTError(uint32(code))
		}
		if time.Now().After(deadline) {
			_ = info.call(9) // Cancel
			return errors.New("Windows didn't answer in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
