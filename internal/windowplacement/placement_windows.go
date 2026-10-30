package windowplacement

import (
	"fmt"
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

var user32 = windows.NewLazySystemDLL("user32.dll")
var getWindowRect = user32.NewProc("GetWindowRect")
var monitorFromWindow = user32.NewProc("MonitorFromWindow")
var getMonitorInfo = user32.NewProc("GetMonitorInfoW")
var setWindowPos = user32.NewProc("SetWindowPos")

type rect struct{ left, top, right, bottom int32 }
type monitorInfo struct {
	size          uint32
	monitor, work rect
	flags         uint32
}

// FitInitialWindow must run on the HWND's owning thread, after Gio has applied
// its initial window options. Coordinates and sizes are physical screen pixels.
func FitInitialWindow(hwnd uintptr) error {
	var bounds rect
	if ok, _, err := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&bounds))); ok == 0 {
		return fmt.Errorf("GetWindowRect: %w", err)
	}
	monitor, _, err := monitorFromWindow.Call(hwnd, 2) // MONITOR_DEFAULTTONEAREST
	if monitor == 0 {
		return fmt.Errorf("MonitorFromWindow: %w", err)
	}
	info := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, err := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return fmt.Errorf("GetMonitorInfo: %w", err)
	}
	window := image.Rect(int(bounds.left), int(bounds.top), int(bounds.right), int(bounds.bottom))
	work := image.Rect(int(info.work.left), int(info.work.top), int(info.work.right), int(info.work.bottom))
	if window.Empty() || work.Empty() {
		return fmt.Errorf("invalid window or monitor work area")
	}
	fitted := fitToWorkArea(window, work)
	if fitted == window {
		return nil
	}
	const flags = 0x0004 | 0x0010 // SWP_NOZORDER | SWP_NOACTIVATE
	if ok, _, err := setWindowPos.Call(hwnd, 0, uintptr(int32(fitted.Min.X)), uintptr(int32(fitted.Min.Y)), uintptr(fitted.Dx()), uintptr(fitted.Dy()), flags); ok == 0 {
		return fmt.Errorf("SetWindowPos: %w", err)
	}
	return nil
}
