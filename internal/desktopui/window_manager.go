package desktopui

import (
	"fmt"
	"glance/internal/diagnostics"
	"glance/internal/i18n"
	"log"
	"runtime"

	"gioui.org/app"
	"gioui.org/io/system"
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
	if _, ok := collection.GetMetadata(documentID); !ok {
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
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_document_session_is_closed_reopen_the_8de5b7"}, feedbackInfo, notificationSuppressed)
		}
	}
	r.runAppWindow(a)
}

func (r *desktopRuntime) runAppWindow(a *App) {
	if a == nil || a.window == nil {
		return
	}
	defer a.closeNativePinch()
	var startupFontErr error
	if err := a.prepareInitialFontTheme(); err != nil {
		startupFontErr = err
		log.Printf("prepare process font theme: %v", err)
		runtime := a.ensureDesktopRuntime()
		runtime.mu.Lock()
		reportStartupError := runtime.startupErrorReporter
		runtime.mu.Unlock()
		if reportStartupError != nil {
			reportStartupError(err)
		}
		if source := a.windowOpenSource; source != nil {
			select {
			case source.windowOpenResults <- err:
			default:
			}
			if source.window != nil {
				source.window.Invalidate()
			}
		}
		// The UI shaper intentionally has no OS-font fallback. Do not show a
		// normal window with missing text when the shared catalog is unavailable.
		a.window.Perform(system.ActionClose)
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
		if startupFontErr != nil {
			if _, destroyed := e.(app.DestroyEvent); !destroyed {
				continue
			}
		}
		switch e := e.(type) {
		case app.DestroyEvent:
			runtime := a.ensureDesktopRuntime()
			runtime.markWindowClosing(a.windowID)
			if e.Err != nil {
				log.Printf("desktop window destroyed: %v", e.Err)
				if !a.renderedFrame && startupFontErr == nil {
					runtime.mu.Lock()
					reportStartupError := runtime.startupErrorReporter
					runtime.mu.Unlock()
					if reportStartupError != nil {
						reportStartupError(e.Err)
					}
				}
				if source := a.windowOpenSource; source != nil && startupFontErr == nil {
					select {
					case source.windowOpenResults <- e.Err:
					default:
					}
					if source.window != nil {
						source.window.Invalidate()
					}
				}
			}
			if a.proofreading != nil && a.proofreading.modelLayoutCancel != nil {
				a.proofreading.modelLayoutCancel()
			}
			if a.proofreading != nil {
				if a.proofreading.ocrScan != nil {
					a.proofreading.ocrScan.cancelTask()
				}
				a.queueProofreadingDraftPersistence()
			}
			a.cancelPDFMergeSplitJobForShutdown()
			a.closeAllProofreadingTabs()
			a.shutdownOnce.Do(func() { close(a.shutdown) })
			a.closeAgentSubscription()
			a.stopAndWaitManagedBackground()
			a.closePendingProofreadingPDFEntry()
			a.closeProofreadingDraft()
			a.closeWorkspace()
			return
		case app.ConfigEvent:
			diagnostics.WindowPhase("config", func() {
				a.applyWindowConfig(e.Config)
				a.window.Invalidate()
			})
		case app.FrameEvent:
			diagnostics.WindowPhase("event", func() {
				select {
				case err := <-a.trayStartupResults:
					if err != nil {
						a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
					}
				default:
				}
				a.processViewIntentRequests()
				a.processNativePinch(e)
			})
			a.prepareWordPointerFrame(e)
			gtx := app.NewContext(&a.ops, e)
			diagnostics.WindowPhase("layout", func() { a.layout(gtx) })
			diagnostics.WindowPhase("frame-submit", func() { e.Frame(gtx.Ops) })
			a.renderedFrame = true
			a.finishWordPointerFrame(e)
			pinchTarget := a
			if tab := a.proofreadingTab(a.selectedProofreadingTabID); tab != nil {
				pinchTarget = tab.editor
			}
			pinchTarget.pinchFrameSession = pinchTarget.documentSessionState
			pinchTarget.pinchFrameSize = e.Size
			pinchTarget.pinchFrameMetric = e.Metric
			if trayStartupDeferredUntilWindowFrame() {
				runtime := a.ensureDesktopRuntime()
				if !a.trayStartupScheduled && !runtime.trayStartupWasAttempted() {
					a.trayStartupScheduled = true
					go func() {
						err := runtime.startSystemTray()
						if err != nil {
							log.Printf("start system tray: %v", err)
						}
						select {
						case a.trayStartupResults <- err:
						default:
						}
						if a.window != nil {
							a.window.Invalidate()
						}
					}()
				}
			}
		default:
			handleWindowMoveViewEvent(a.window, e)
			a.handleNativePinchViewEvent(e)
			a.handleWordPointerViewEvent(e)
		}
	}
}
