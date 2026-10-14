//go:build darwin

package desktopui

/*
#cgo darwin LDFLAGS: -framework AppKit
#include <stdlib.h>
#include "system_tray_darwin.h"
*/
import "C"

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type systemTray struct {
	queue   systemTrayActionQueue
	ready   chan struct{}
	setup   chan error
	removed chan struct{}
	id      uint64
}

var (
	nativeTrayID      atomic.Uint64
	nativeTrayMu      sync.RWMutex
	nativeTrayEntries = make(map[uint64]*systemTray)
)

func trayHostAvailable() error { return nil }

func trayStartupDeferredUntilWindowFrame() bool { return true }

func (a *App) hideMainWindowToTray() bool {
	if a == nil || a.window == nil || a.windowID == 0 {
		return false
	}
	runtime := a.ensureDesktopRuntime()
	runtime.mu.Lock()
	canHide := runtime.trayReady && !runtime.shuttingDown && !runtime.quitRequested &&
		runtime.primaryWindowID == a.windowID && runtime.windows[a.windowID] == a
	runtime.mu.Unlock()
	if !canHide {
		return false
	}
	return C.glance_tray_hide_active_window() != 0
}

func newSystemTray(actions chan trayAction) (*systemTray, error) {
	if actions == nil {
		return nil, errors.New("tray action channel is unavailable")
	}
	return &systemTray{
		queue:   systemTrayActionQueue{actions: actions},
		ready:   make(chan struct{}),
		setup:   make(chan error, 1),
		removed: make(chan struct{}, 1),
	}, nil
}

func (t *systemTray) run(ctx context.Context) error {
	if t == nil {
		return errors.New("system tray is unavailable")
	}
	t.id = nativeTrayID.Add(1)
	nativeTrayMu.Lock()
	nativeTrayEntries[t.id] = t
	nativeTrayMu.Unlock()

	icon22PNG := embeddedMacTrayIcon22
	icon44PNG := embeddedMacTrayIcon44
	icon22 := C.CBytes(icon22PNG)
	icon44 := C.CBytes(icon44PNG)
	if C.glance_tray_create(
		C.uintptr_t(t.id), icon22, C.size_t(len(icon22PNG)), icon44, C.size_t(len(icon44PNG)),
	) == 0 {
		C.free(icon22)
		C.free(icon44)
		t.unregister()
		return errors.New("schedule macOS status item creation")
	}
	C.free(icon22)
	C.free(icon44)

	select {
	case err := <-t.setup:
		if err != nil {
			t.unregister()
			return err
		}
		close(t.ready)
	case <-ctx.Done():
		if C.glance_tray_remove(C.uintptr_t(t.id)) == 0 {
			t.unregister()
			return errors.New("schedule macOS status item removal")
		}
		select {
		case <-t.removed:
		case <-time.After(5 * time.Second):
		}
		t.unregister()
		return nil
	}

	<-ctx.Done()
	if C.glance_tray_remove(C.uintptr_t(t.id)) == 0 {
		t.unregister()
		return errors.New("schedule macOS status item removal")
	}
	select {
	case <-t.removed:
	case <-time.After(5 * time.Second):
		t.unregister()
		return errors.New("timed out removing the macOS system tray icon")
	}
	t.unregister()
	return nil
}

func (t *systemTray) unregister() {
	nativeTrayMu.Lock()
	delete(nativeTrayEntries, t.id)
	nativeTrayMu.Unlock()
}

func nativeTrayForCallback(id uint64) *systemTray {
	nativeTrayMu.RLock()
	tray := nativeTrayEntries[id]
	nativeTrayMu.RUnlock()
	return tray
}

//export goGlanceTraySetupFinished
func goGlanceTraySetupFinished(id C.uintptr_t, success C.int) {
	tray := nativeTrayForCallback(uint64(id))
	if tray == nil {
		return
	}
	var err error
	if success == 0 {
		err = errors.New("macOS did not create the Glance status item")
	}
	select {
	case tray.setup <- err:
	default:
	}
}

//export goGlanceTrayAction
func goGlanceTrayAction(id C.uintptr_t, action C.int) {
	if tray := nativeTrayForCallback(uint64(id)); tray != nil {
		tray.queue.send(trayAction(action))
	}
}

//export goGlanceTrayRemoved
func goGlanceTrayRemoved(id C.uintptr_t) {
	if tray := nativeTrayForCallback(uint64(id)); tray != nil {
		select {
		case tray.removed <- struct{}{}:
		default:
		}
	}
}
