package desktopui

import (
	"fmt"
	"log"
	"runtime"

	"gioui.org/app"
	"gioui.org/unit"
)

type windowLauncher func(documentID string, snapshot windowViewSnapshot) error

func supportsMultipleDesktopWindows() bool {
	switch runtime.GOOS {
	case "darwin", "windows", "linux":
		return true
	default:
		return false
	}
}

func configureDesktopWindow(window *app.Window) {
	window.Option(
		app.Title("desktop"),
		app.Decorated(false),
		app.Size(unit.Dp(1280), unit.Dp(820)),
		app.MinSize(unit.Dp(900), unit.Dp(600)),
	)
}

func (r *desktopRuntime) openDocumentWindow(documentID string, snapshot windowViewSnapshot, source ...*App) error {
	if r == nil {
		return fmt.Errorf("桌面窗口运行时不可用")
	}
	if !supportsMultipleDesktopWindows() {
		return fmt.Errorf("当前平台不支持多个桌面窗口")
	}
	if _, ok := r.session(documentID); !ok {
		return fmt.Errorf("文档会话已关闭，请重新打开文档")
	}
	r.mu.Lock()
	launcher := r.windowLauncher
	collection := r.collection
	r.mu.Unlock()
	if launcher != nil {
		return launcher(documentID, snapshot)
	}
	if collection == nil {
		return fmt.Errorf("工作区尚未初始化，无法创建文档窗口")
	}
	if _, ok := collection.Get(documentID); !ok {
		return fmt.Errorf("工作区标签已关闭，请重新打开文档")
	}
	window := new(app.Window)
	configureDesktopWindow(window)
	var sourceApp *App
	if len(source) > 0 {
		sourceApp = source[0]
	}
	go r.runWindowFrom(0, window, documentID, snapshot, sourceApp)
	return nil
}

func (r *desktopRuntime) runWindow(windowID uint64, window *app.Window, initialDocumentID string, snapshot windowViewSnapshot) {
	r.runWindowFrom(windowID, window, initialDocumentID, snapshot, nil)
}

func (r *desktopRuntime) runWindowFrom(windowID uint64, window *app.Window, initialDocumentID string, snapshot windowViewSnapshot, source *App) {
	a := newAppWithRuntimeAndWindowID(window, "", r, windowID)
	a.windowOpenSource = source
	if initialDocumentID != "" {
		if shared, ok := r.session(initialDocumentID); ok {
			view := newDocumentViewFromSnapshot(shared, a.windowID, snapshot)
			view.documentID = initialDocumentID
			a.sessions[initialDocumentID] = view
			a.activateSession(initialDocumentID, view)
		} else {
			a.setStatus("文档会话已关闭，请重新打开文档")
		}
	}
	r.runAppWindow(a)
}

func (r *desktopRuntime) runAppWindow(a *App) {
	if a == nil || a.window == nil {
		return
	}
	var unregisterView func()
	if applicationRuntime := a.ensureDesktopRuntime().applicationRuntime; applicationRuntime != nil {
		unregisterView = applicationRuntime.RegisterViewIntentHandler(a.windowID, a)
	}
	if unregisterView != nil {
		defer unregisterView()
	}
	for {
		e := a.window.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			a.ensureDesktopRuntime().markWindowClosing(a.windowID)
			if e.Err != nil {
				log.Printf("desktop window destroyed: %v", e.Err)
				if source := a.windowOpenSource; source != nil {
					select {
					case source.windowOpenResults <- e.Err:
					default:
					}
					if source.window != nil {
						source.window.Invalidate()
					}
				}
			}
			a.shutdownOnce.Do(func() { close(a.shutdown) })
			a.closeAgentSubscription()
			a.stopAndWaitManagedBackground()
			a.closeWorkspace()
			return
		case app.ConfigEvent:
			a.applyWindowConfig(e.Config)
			a.window.Invalidate()
		case app.FrameEvent:
			a.processViewIntentRequests()
			gtx := app.NewContext(&a.ops, e)
			a.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}
