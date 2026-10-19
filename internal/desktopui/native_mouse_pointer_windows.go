//go:build windows

package desktopui

import (
	"log"
	"unsafe"

	"golang.org/x/sys/windows"
)

var mousePointerComctl = windows.NewLazySystemDLL("comctl32.dll")
var mousePointerSetSubclass = mousePointerComctl.NewProc("SetWindowSubclass")
var mousePointerRemoveSubclass = mousePointerComctl.NewProc("RemoveWindowSubclass")
var mousePointerDefSubclass = mousePointerComctl.NewProc("DefSubclassProc")
var mousePointerGetInfo = wordPointerUser32.NewProc("GetPointerInfo")
var mousePointerCallback uintptr

func init() { mousePointerCallback = windows.NewCallback(nativeMousePointerProc) }

const mousePointerSubclassID = 0x474c5054

type nativeMousePointerInfo struct {
	pointerType, pointerID, frameID, flags uint32
	sourceDevice, target                   uintptr
	pixel, himetric, rawPixel, rawHimetric wordNativePoint
	time, history                          uint32
	inputData                              int32
	keyStates                              uint32
	performance                            uint64
	buttonChange                           uint32
}

// Install on the HWND thread. This public Win32 subclass sits before Gio and
// fixes captured mouse movement for its native Editors as well as custom grids.
// No private Gio state or maintained fork is needed.
func attachNativeMousePointer(hwnd uintptr) {
	if err := mousePointerSetSubclass.Find(); err != nil {
		log.Printf("mouse pointer bridge: %v", err)
		return
	}
	if ok, _, err := mousePointerSetSubclass.Call(hwnd, mousePointerCallback, mousePointerSubclassID, 0); ok == 0 {
		log.Printf("mouse pointer bridge: %v", err)
	}
}

func nativeMousePointerProc(hwnd, msg, wparam, lparam, id, ref uintptr) uintptr {
	if msg == 0x0245 { // WM_POINTERUPDATE
		var info nativeMousePointerInfo
		if ok, _, _ := mousePointerGetInfo.Call(wparam&0xffff, uintptr(unsafe.Pointer(&info))); ok != 0 && info.pointerType == 4 && info.flags&0x10 != 0 { // PT_MOUSE, FIRSTBUTTON
			// This callback already runs in the HWND's per-monitor DPI context.
			// WM_POINTERUPDATE requires screen coordinates; Gio converts them to client.
			var point wordNativePoint
			if ok, _, _ := wordPointerGetCursorPos.Call(uintptr(unsafe.Pointer(&point))); ok != 0 {
				lparam = nativeMousePointerPosition(point.X, point.Y)
			}
		}
	}
	if msg == 0x0082 {
		mousePointerRemoveSubclass.Call(hwnd, mousePointerCallback, id)
	} // WM_NCDESTROY
	result, _, _ := mousePointerDefSubclass.Call(hwnd, msg, wparam, lparam)
	return result
}
