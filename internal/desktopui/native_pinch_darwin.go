//go:build darwin && !ios && cgo

package desktopui

/*
#cgo LDFLAGS: -framework AppKit
#include <stdint.h>
void *glance_installPinch(uintptr_t view, uintptr_t key);
void glance_removePinch(void *monitor);
*/
import "C"

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"gioui.org/app"
	"gioui.org/f32"
)

var nativePinchRegistry sync.Map
var nativePinchSequence atomic.Uint64

type nativePinchMonitor struct {
	key     uintptr
	monitor unsafe.Pointer
}

func (a *App) handleNativePinchViewEvent(event any) {
	view, ok := event.(app.AppKitViewEvent)
	if !ok {
		return
	}
	a.closeNativePinch()
	if view.View == 0 {
		return
	}
	state := newNativePinchState(a.window.Invalidate)
	if !a.launchManagedBackground(func() { state.runInvalidations(a.shutdown) }) {
		return
	}
	key := uintptr(nativePinchSequence.Add(1))
	nativePinchRegistry.Store(key, state)
	a.nativePinch = state
	a.nativePinchMonitor = nativePinchMonitor{key: key, monitor: C.glance_installPinch(C.uintptr_t(view.View), C.uintptr_t(key))}
}

func (a *App) closeNativePinch() {
	monitor := a.nativePinchMonitor
	if monitor.key != 0 {
		nativePinchRegistry.Delete(monitor.key)
		if a.nativePinch != nil {
			a.nativePinch.stop()
		}
		C.glance_removePinch(monitor.monitor)
	}
	a.nativePinchMonitor = nativePinchMonitor{}
	a.nativePinch = nil
	a.pinchGestureSession = nil
}

//export glance_onPinch
func glance_onPinch(key C.uintptr_t, x, y, magnification C.double, begin, end C.int) {
	if value, ok := nativePinchRegistry.Load(uintptr(key)); ok {
		value.(*nativePinchState).enqueue(nativePinchEvent{Position: f32.Pt(float32(x), float32(y)), Magnification: float64(magnification), Begin: begin != 0, End: end != 0})
	}
}
