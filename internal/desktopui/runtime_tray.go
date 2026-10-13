package desktopui

import (
	"context"
	"errors"
	"log"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
)

const trayActionQueueSize = 16

func (r *desktopRuntime) startSystemTray() error {
	if r == nil {
		return errors.New("desktop runtime is unavailable")
	}
	r.mu.Lock()
	if r.shuttingDown || r.quitRequested {
		r.mu.Unlock()
		return errors.New("desktop runtime is shutting down")
	}
	if r.trayCancel != nil {
		r.mu.Unlock()
		return nil
	}
	actions := make(chan trayAction, trayActionQueueSize)
	r.trayActions = actions
	r.mu.Unlock()

	tray, err := newSystemTray(actions)
	if err != nil {
		r.mu.Lock()
		r.trayActions = nil
		r.mu.Unlock()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.mu.Lock()
	if r.shuttingDown || r.quitRequested {
		r.mu.Unlock()
		cancel()
		return errors.New("desktop runtime is shutting down")
	}
	r.trayCancel = cancel
	r.trayDone = done
	r.mu.Unlock()

	go func() {
		runErr := tray.run(ctx)
		r.mu.Lock()
		r.trayError = runErr
		r.trayReady = false
		close(done)
		r.mu.Unlock()
	}()
	go r.watchSystemTray(done)

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-tray.ready:
		r.mu.Lock()
		if !r.shuttingDown && !r.quitRequested && !r.isTrayStopped(done) {
			r.trayReady = true
			r.mu.Unlock()
			go r.dispatchTrayActions(actions, done)
			return nil
		}
		r.mu.Unlock()
		return errors.New("system tray stopped during initialization")
	case <-done:
		r.mu.Lock()
		err := r.trayError
		r.mu.Unlock()
		if err == nil {
			err = errors.New("system tray stopped during initialization")
		}
		return err
	case <-timer.C:
		cancel()
		return errors.New("timed out starting the system tray")
	}
}

func (r *desktopRuntime) recordResumeActiveTab(tabID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.trayReady && !r.quitRequested && !r.shuttingDown {
		r.resumeActiveTabID = tabID
	}
	r.mu.Unlock()
}

func (r *desktopRuntime) takeResumeActiveTab() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	tabID := r.resumeActiveTabID
	r.resumeActiveTabID = ""
	r.mu.Unlock()
	return tabID
}

func (r *desktopRuntime) isTrayStopped(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func (r *desktopRuntime) dispatchTrayActions(actions <-chan trayAction, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case action := <-actions:
			switch action {
			case trayActionShow:
				r.showMainWindow()
			case trayActionQuit:
				r.requestQuit()
				return
			}
		}
	}
}

func (r *desktopRuntime) watchSystemTray(done <-chan struct{}) {
	<-done
	r.mu.Lock()
	err := r.trayError
	creationDone := r.mainWindowCreationDone
	r.mu.Unlock()
	if err != nil {
		log.Printf("system tray stopped: %v", err)
	}
	if creationDone != nil {
		<-creationDone
	}
	r.mu.Lock()
	shouldShutdown := !r.shuttingDown && !r.quitRequested && !r.trayReady && len(r.windows) == 0 && !r.mainWindowCreating
	r.mu.Unlock()
	if shouldShutdown {
		r.shutdown()
	}
}

func (r *desktopRuntime) shouldKeepAlive() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	keepAlive := r.trayReady && !r.quitRequested && !r.shuttingDown
	r.mu.Unlock()
	return keepAlive
}

func (r *desktopRuntime) showMainWindow() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.shuttingDown || r.quitRequested || !r.trayReady {
		r.mu.Unlock()
		return
	}
	primary := r.windows[r.primaryWindowID]
	if primary != nil && !r.closingWindows[r.primaryWindowID] {
		window := primary.window
		r.mu.Unlock()
		if window != nil {
			window.Perform(system.ActionRaise)
			window.Invalidate()
		}
		return
	}
	if primary != nil {
		r.pendingMainWindowShow = true
		r.mu.Unlock()
		return
	}
	if r.mainWindowCreating {
		r.mu.Unlock()
		return
	}
	r.mainWindowCreating = true
	creationDone := make(chan struct{})
	r.mainWindowCreationDone = creationDone
	r.mu.Unlock()

	window := new(app.Window)
	configureDesktopWindow(window)
	a := newAppWithRuntime(window, r)

	r.mu.Lock()
	r.mainWindowCreating = false
	r.mainWindowCreationDone = nil
	close(creationDone)
	closeNow := r.shuttingDown || r.quitRequested
	r.mu.Unlock()
	if closeNow {
		window.Perform(system.ActionClose)
	}
	go r.runAppWindow(a)
}

func (r *desktopRuntime) showPendingMainWindow() {
	if r == nil {
		return
	}
	r.mu.Lock()
	show := r.pendingMainWindowShow && r.trayReady && !r.quitRequested && !r.shuttingDown
	r.pendingMainWindowShow = false
	r.mu.Unlock()
	if show {
		r.showMainWindow()
	}
}

func (r *desktopRuntime) requestQuit() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.shuttingDown || r.quitRequested {
		r.mu.Unlock()
		return
	}
	r.quitRequested = true
	r.trayReady = false
	cancelTray := r.trayCancel
	creationDone := r.mainWindowCreationDone
	r.mu.Unlock()
	if cancelTray != nil {
		cancelTray()
	}
	if creationDone != nil {
		<-creationDone
	}
	r.mu.Lock()
	windows := make([]*App, 0, len(r.windows))
	for _, windowApp := range r.windows {
		if windowApp != nil {
			windows = append(windows, windowApp)
		}
	}
	r.mu.Unlock()
	for _, windowApp := range windows {
		if windowApp.window != nil {
			windowApp.window.Perform(system.ActionClose)
		}
	}
	if len(windows) == 0 {
		r.shutdown()
	}
}
