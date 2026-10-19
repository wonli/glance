package windowmove

import (
	"runtime"
	"sync"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

var observed sync.Map
var observedPaint sync.Map

func observeMessages(hwnd, msg, wparam, lparam, id, ref uintptr) uintptr {
	if msg == wmPaint {
		if v, ok := observedPaint.Load(hwnd); ok {
			*v.(*int)++
		}
	}
	if msg == wmWindowPosChanged {
		if v, ok := observed.Load(hwnd); ok {
			*v.(*int)++
		}
	}
	result, _, _ := defSubclass.Call(hwnd, msg, wparam, lparam)
	return result
}

// This exercises a real HWND and subclass chain, without Gio or a GPU.
func TestNativeMoveBridge(t *testing.T) {
	t.Setenv("GLANCE_WINDOWS_MOVE_REDRAW", "1")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	create := user32.NewProc("CreateWindowExW")
	destroy := user32.NewProc("DestroyWindow")
	send := user32.NewProc("SendMessageW")
	class, _ := windows.UTF16PtrFromString("STATIC")
	hwnd, _, err := create.Call(0, uintptr(unsafe.Pointer(class)), 0, 0x00cf0000, 0, 0, 640, 480, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatalf("CreateWindowEx: %v", err)
	}
	destroyed := false
	defer func() {
		if !destroyed {
			destroy.Call(hwnd)
		}
	}()
	count := 0
	paints := 0
	observedPaint.Store(hwnd, &paints)
	defer observedPaint.Delete(hwnd)
	observed.Store(hwnd, &count)
	defer observed.Delete(hwnd)
	observer := windows.NewCallback(observeMessages)
	if ok, _, err := setSubclass.Call(hwnd, observer, 123, 0); ok == 0 {
		t.Fatalf("observer: %v", err)
	}
	defer removeSubclass.Call(hwnd, observer, 123)
	if err := Attach(hwnd); err != nil {
		t.Fatal(err)
	}
	if err := Attach(hwnd); err != nil {
		t.Fatal("duplicate attach:", err)
	}
	nativeThreadErr := make(chan error, 1)
	go func() { nativeThreadErr <- Attach(hwnd) }()
	if err := <-nativeThreadErr; err == nil {
		t.Fatal("accepted cross-thread attach")
	}
	position := windowPos{hwnd: hwnd, flags: 0x0001 | 0x0004}
	sendPosition := func() { send.Call(hwnd, wmWindowPosChanged, 0, uintptr(unsafe.Pointer(&position))) }
	sendPosition()
	if count != 1 {
		t.Fatalf("outside gesture: %d", count)
	}
	send.Call(hwnd, wmEnterSizeMove, 0, 0)
	sendPosition()
	if count != 1 {
		t.Fatalf("pure move forwarded: %d", count)
	}
	send.Call(hwnd, wmPaint, 0, 0)
	if paints != 0 {
		t.Fatal("paint during pure move reached renderer")
	}
	send.Call(hwnd, wmSizing, 0, 0)
	send.Call(hwnd, wmPaint, 0, 0)
	if paints != 1 {
		t.Fatal("resize paint was swallowed")
	}
	for _, flags := range []uint32{0x0004, 0x0001, 0x0001 | 0x0004 | 0x0020, 0x0001 | 0x0004 | 0x0080} {
		position.flags = flags
		before := count
		sendPosition()
		if count != before+1 {
			t.Fatalf("flags %#x swallowed", flags)
		}
	}
	send.Call(hwnd, wmExitSizeMove, 0, 0)
	send.Call(hwnd, wmPaint, 0, 0)
	if paints != 2 {
		t.Fatal("paint after gesture was swallowed")
	}
	position.flags = 0x0001 | 0x0004
	before := count
	sendPosition()
	if count != before+1 {
		t.Fatal("move after exit swallowed")
	}
	destroy.Call(hwnd)
	destroyed = true
	if _, ok := bridges.Load(hwnd); ok {
		t.Fatal("bridge retained after native destroy")
	}
}
