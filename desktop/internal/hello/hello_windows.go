package hello

import (
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	combase                    = windows.NewLazySystemDLL("combase.dll")
	procRoInitialize           = combase.NewProc("RoInitialize")
	procRoGetActivationFactory = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString    = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString    = combase.NewProc("WindowsDeleteString")
	user32                     = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	procFindWindowW            = user32.NewProc("FindWindowW")
	procGetWindowThreadProcId  = user32.NewProc("GetWindowThreadProcessId")
)

const className = "Windows.Security.Credentials.UI.UserConsentVerifier"

var (
	// IUserConsentVerifierStatics
	iidStatics = windows.GUID{Data1: 0xAF4F3F91, Data2: 0x564C, Data3: 0x4DDC, Data4: [8]byte{0xB8, 0xB5, 0x97, 0x34, 0x47, 0x62, 0x7C, 0x65}}
	// IUserConsentVerifierInterop: shows the prompt attached to our window.
	iidInterop = windows.GUID{Data1: 0x39E050C3, Data2: 0x4E74, Data3: 0x441A, Data4: [8]byte{0x8D, 0xC0, 0xB8, 0x11, 0x04, 0xDF, 0x94, 0x9C}}
	// IAsyncOperation<UserConsentVerificationResult>
	iidAsyncResult = windows.GUID{Data1: 0xfd596ffd, Data2: 0x2318, Data3: 0x558f, Data4: [8]byte{0x9d, 0xbe, 0xd2, 0x1d, 0xf4, 0x37, 0x64, 0xa5}}
	// IAsyncInfo
	iidAsyncInfo = windows.GUID{Data1: 0x00000036, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

// Available reports whether Hello is set up and usable on this computer.
func Available() bool {
	var ok bool
	run(func() error {
		f, err := factory(&iidStatics)
		if err != nil {
			return err
		}
		defer release(f)
		var op unsafe.Pointer
		if hr := vcall(f, 6, uintptr(unsafe.Pointer(&op))); failed(hr) { // CheckAvailabilityAsync
			return syscall.Errno(hr)
		}
		defer release(op)
		v, err := await(op, 10*time.Second)
		ok = err == nil && v == 0 // UserConsentVerifierAvailability.Available
		return nil
	})
	return ok
}

// Verify shows the Hello prompt over the app window titled windowTitle.
func Verify(windowTitle, message string) error {
	return run(func() error {
		f, err := factory(&iidInterop)
		if err != nil {
			return ErrUnavailable
		}
		defer release(f)
		msg, err := hstring(message)
		if err != nil {
			return err
		}
		defer procWindowsDeleteString.Call(msg)
		var op unsafe.Pointer
		hr := vcall(f, 6, appWindow(windowTitle), msg, uintptr(unsafe.Pointer(&iidAsyncResult)), uintptr(unsafe.Pointer(&op)))
		if failed(hr) {
			return ErrUnavailable
		}
		defer release(op)
		v, err := await(op, 5*time.Minute)
		if err != nil {
			return err
		}
		switch v {
		case 0: // Verified
			return nil
		case 6: // Canceled
			return ErrCanceled
		case 1, 2, 3: // DeviceNotPresent, NotConfiguredForUser, DisabledByPolicy
			return ErrUnavailable
		default:
			return ErrFailed
		}
	})
}

// run executes f on a locked OS thread with the Windows Runtime initialised.
func run(f func() error) error {
	errc := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		procRoInitialize.Call(1) // RO_INIT_MULTITHREADED; S_FALSE / changed mode are fine
		errc <- f()
	}()
	return <-errc
}

func factory(iid *windows.GUID) (unsafe.Pointer, error) {
	name, err := hstring(className)
	if err != nil {
		return nil, err
	}
	defer procWindowsDeleteString.Call(name)
	var f unsafe.Pointer
	hr, _, _ := procRoGetActivationFactory.Call(name, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&f)))
	if failed(hr) {
		return nil, syscall.Errno(hr)
	}
	return f, nil
}

func hstring(s string) (uintptr, error) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	var h uintptr
	hr, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	if failed(hr) {
		return 0, syscall.Errno(hr)
	}
	return h, nil
}

// await polls an IAsyncOperation<enum> and returns its result.
func await(op unsafe.Pointer, timeout time.Duration) (int32, error) {
	var info unsafe.Pointer
	if hr := vcall(op, 0, uintptr(unsafe.Pointer(&iidAsyncInfo)), uintptr(unsafe.Pointer(&info))); failed(hr) {
		return 0, syscall.Errno(hr)
	}
	defer release(info)
	deadline := time.Now().Add(timeout)
	for {
		var status int32
		vcall(info, 7, uintptr(unsafe.Pointer(&status))) // get_Status
		switch status {
		case 1: // Completed
			var v int32
			if hr := vcall(op, 8, uintptr(unsafe.Pointer(&v))); failed(hr) { // GetResults
				return 0, syscall.Errno(hr)
			}
			return v, nil
		case 2:
			return 0, ErrCanceled
		case 3:
			return 0, ErrFailed
		}
		if time.Now().After(deadline) {
			vcall(info, 9) // Cancel
			return 0, ErrCanceled
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// appWindow finds the app's top-level window to parent the prompt.
func appWindow(title string) uintptr {
	pid := uint32(os.Getpid())
	owned := func(h uintptr) bool {
		var p uint32
		procGetWindowThreadProcId.Call(h, uintptr(unsafe.Pointer(&p)))
		return h != 0 && p == pid
	}
	if h, _, _ := procGetForegroundWindow.Call(); owned(h) {
		return h
	}
	t, _ := windows.UTF16PtrFromString(title)
	if h, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(t))); owned(h) {
		return h
	}
	return 0
}

// vcall invokes method index of a COM object's vtable.
func vcall(obj unsafe.Pointer, index int, args ...uintptr) uintptr {
	vtbl := *(**[64]uintptr)(obj)
	r, _, _ := syscall.SyscallN(vtbl[index], append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

func release(obj unsafe.Pointer) {
	if obj != nil {
		vcall(obj, 2)
	}
}

func failed(hr uintptr) bool { return int32(hr) < 0 }
