package windowmove

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	comctl32        = windows.NewLazySystemDLL("comctl32.dll")
	setSubclass     = comctl32.NewProc("SetWindowSubclass")
	removeSubclass  = comctl32.NewProc("RemoveWindowSubclass")
	defSubclass     = comctl32.NewProc("DefSubclassProc")
	getClientRect   = user32.NewProc("GetClientRect")
	getDPI          = user32.NewProc("GetDpiForWindow")
	isZoomed        = user32.NewProc("IsZoomed")
	isIconic        = user32.NewProc("IsIconic")
	invalidateRect  = user32.NewProc("InvalidateRect")
	validateRect    = user32.NewProc("ValidateRect")
	getWindowThread = user32.NewProc("GetWindowThreadProcessId")
	moveCallback    uintptr
	bridges         sync.Map // HWND -> *bridge; no Go pointers stored by Windows.
)

func init() { moveCallback = windows.NewCallback(subclassProc) }

const subclassID = 0x474c4d56
const (
	wmWindowPosChanged = 0x0047
	wmNCDestroy        = 0x0082
	wmEnterSizeMove    = 0x0231
	wmExitSizeMove     = 0x0232
	wmPaint            = 0x000f
	wmSizing           = 0x0214
)

type bridge struct {
	dragging bool
	resizing bool
	last     geometry
}

// windowPos matches Win32 WINDOWPOS on both 32-bit and 64-bit Windows.
type windowPos struct {
	hwnd, insertAfter uintptr
	x, y, cx, cy      int32
	flags             uint32
}

type rect struct{ left, top, right, bottom int32 }

// Attach must run on the HWND's owning thread (through app.Window.Run).
// The subclass removes itself at WM_NCDESTROY. Do not call Window.Run for an
// invalid ViewEvent: Gio may already be shutting down its native event loop.
func Attach(hwnd uintptr) error {
	if hwnd == 0 || !Enabled() {
		return nil
	}
	for _, proc := range []*windows.LazyProc{setSubclass, removeSubclass, defSubclass, getClientRect, getDPI, isZoomed, isIconic, invalidateRect, validateRect, getWindowThread} {
		if err := proc.Find(); err != nil {
			return fmt.Errorf("window move bridge: %w", err)
		}
	}
	thread, _, _ := getWindowThread.Call(hwnd, 0)
	if thread == 0 || uint32(thread) != windows.GetCurrentThreadId() {
		return fmt.Errorf("window move bridge: not on window thread")
	}
	if _, ok := bridges.Load(hwnd); ok {
		return nil
	}
	b := &bridge{last: readGeometry(hwnd)}
	bridges.Store(hwnd, b)
	ok, _, err := setSubclass.Call(hwnd, moveCallback, subclassID, 0)
	if ok == 0 {
		bridges.Delete(hwnd)
		return fmt.Errorf("window move bridge: SetWindowSubclass: %v", err)
	}
	return nil
}

func readGeometry(hwnd uintptr) geometry {
	var r rect
	ok, _, _ := getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	dpi, _, _ := getDPI.Call(hwnd)
	zoomed, _, _ := isZoomed.Call(hwnd)
	iconic, _, _ := isIconic.Call(hwnd)
	return geometry{width: r.right - r.left, height: r.bottom - r.top, dpi: uint32(dpi), normal: zoomed == 0 && iconic == 0, valid: ok != 0 && dpi != 0}
}

func subclassProc(hwnd, msg, wparam, lparam, id, ref uintptr) uintptr {
	if value, ok := bridges.Load(hwnd); ok {
		b := value.(*bridge) // All fields are owned by this HWND's native thread.
		switch msg {
		case wmEnterSizeMove:
			b.last = readGeometry(hwnd)
			b.dragging = true
			b.resizing = false
		case wmSizing:
			b.resizing = true
		case wmPaint:
			if deferMovePaint(b.dragging, b.resizing, b.last, readGeometry(hwnd)) {
				// A stationary pointer lets the modal move loop dispatch WM_PAINT.
				// Validate the region so it does not spin; EXIT always invalidates
				// again to display content updates deferred during this gesture.
				if ok, _, _ := validateRect.Call(hwnd, 0); ok != 0 {
					return 0
				}
			}
		case wmExitSizeMove:
			b.dragging = false
			// Queue a final paint without reentering Gio from a native callback.
			invalidateRect.Call(hwnd, 0, 0)
		case wmWindowPosChanged:
			next := readGeometry(hwnd)
			previous := b.last
			if previous != next {
				b.resizing = true // Preserve paints after size, DPI or state changes.
			}
			b.last = next
			if lparam != 0 && skipPositionRedraw((*windowPos)(unsafe.Pointer(lparam)).flags, b.dragging, previous, next) {
				// This message is a pure move inside a native gesture. Gio's update()
				// would synchronously render and Present on the window message thread.
				// Keep the current DWM surface; paint/invalidation messages still pass.
				return 0
			}
		case wmNCDestroy:
			b.dragging = false
			removeSubclass.Call(hwnd, moveCallback, id)
			bridges.Delete(hwnd)
		}
	}
	result, _, _ := defSubclass.Call(hwnd, msg, wparam, lparam)
	return result
}
