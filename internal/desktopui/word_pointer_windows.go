//go:build windows

package desktopui

import (
	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/input"
	"golang.org/x/sys/windows"
	"unsafe"
)

var wordPointerUser32 = windows.NewLazySystemDLL("user32.dll")
var wordPointerGetCursorPos = wordPointerUser32.NewProc("GetCursorPos")
var wordPointerScreenToClient = wordPointerUser32.NewProc("ScreenToClient")
var wordPointerGetWindowThread = wordPointerUser32.NewProc("GetWindowThreadProcessId")

var wordPointerGetAsyncKeyState = wordPointerUser32.NewProc("GetAsyncKeyState")

type wordNativePoint struct{ X, Y int32 }

func (a *App) handleWordPointerViewEvent(event any) {
	if view, ok := event.(app.Win32ViewEvent); ok {
		if view.HWND != 0 && a.wordPointer.hwnd != view.HWND && a.window != nil {
			a.window.Run(func() { attachNativeMousePointer(view.HWND) })
		}
		a.wordPointer.hwnd = view.HWND
		a.wordPointer.sample = wordPointerSample{}
	}
}

// Read the completed frame's public semantic geometry before NewContext resets
// its operations. Native client pixels and that viewport share the window origin.
// A size/DPI/session change falls back to Gio until a matching frame exists.
func (a *App) prepareWordPointerFrame(frame app.FrameEvent) {
	state := a.documentSessionState
	capture := &a.wordPointer
	capture.sample = wordPointerSample{}
	if capture.hwnd == 0 || state == nil || (state.word == nil && a.book == nil) || (state.word != nil && state.wordViewMode == wordViewWebLayout) ||
		capture.previousSession != state || capture.previousSize != frame.Size || capture.previousMetric != frame.Metric || a.modal.Visible {
		return
	}
	if a.window == nil {
		return
	}
	// Gio sets per-monitor DPI awareness only on the native window thread.
	// Cursor APIs on a pooled Go thread may return virtualized coordinates.
	var point wordNativePoint
	var valid bool
	// Key state is not a coordinate API; avoid scheduling native work on
	// ordinary document frames without a selection gesture.
	if state.wordPageSelectionDrag == nil && !a.selectionDragging {
		down, _, _ := wordPointerGetAsyncKeyState.Call(0x01)
		if down&0x8000 == 0 {
			return
		}
	}
	var nativeThread, windowThread uint32
	a.window.Run(func() {
		nativeThread = windows.GetCurrentThreadId()
		owner, _, _ := wordPointerGetWindowThread.Call(capture.hwnd, 0)
		windowThread = uint32(owner)
		if nativeThread != windowThread || windowThread == 0 {
			return
		}
		if ok, _, _ := wordPointerGetCursorPos.Call(uintptr(unsafe.Pointer(&point))); ok == 0 {
			return
		}
		if ok, _, _ := wordPointerScreenToClient.Call(capture.hwnd, uintptr(unsafe.Pointer(&point))); ok == 0 {
			return
		}
		valid = true
	})
	if !valid {
		return
	}
	var router input.Router
	router.Frame(&a.ops)
	bounds, ok := wordPointerViewportBounds(router.AppendSemantics(nil))
	if !ok {
		return
	}
	capture.sample = wordPointerSample{valid: true, client: f32.Pt(float32(point.X), float32(point.Y)), origin: bounds.Min, nativeThread: nativeThread, windowThread: windowThread}
}
