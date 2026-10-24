package desktopui

import (
	"fmt"
	"gioui.org/app"
)

// The proofreading module has its own window and tab collection. It shares
// local persistence with the application, never its home route or open tabs.
func (r *desktopRuntime) openProofreadingModuleWindow(request proofreadingWindowRequest) error {
	window, requests, start, err := r.enqueueProofreadingModuleRequest(request)
	if err != nil {
		return err
	}
	if start {
		configureDesktopWindow(window)
		go r.runProofreadingModuleWindow(window, requests)
	} else {
		// Queue the raise on the destination's live event loop. Perform on a
		// native window whose loop has ended can block the PDF window forever.
		window.Invalidate()
	}
	return nil
}

// Enqueue and retirement share the runtime lock: no request can be delivered
// to an old queue after that window begins closing.
func (r *desktopRuntime) enqueueProofreadingModuleRequest(request proofreadingWindowRequest) (*app.Window, chan proofreadingWindowRequest, bool, error) {
	if request.sourceSnapshot == nil {
		return nil, nil, false, fmt.Errorf("无法读取当前 PDF 快照")
	}
	r.mu.Lock()
	if r.shuttingDown || r.quitRequested {
		r.mu.Unlock()
		request.sourceSnapshot.Close()
		return nil, nil, false, fmt.Errorf("应用正在退出，无法打开校对模块")
	}
	window, requests := r.proofreadingModuleWindow, r.proofreadingModuleRequests
	start := window == nil
	if start {
		window = new(app.Window)
		requests = make(chan proofreadingWindowRequest, 16)
		r.proofreadingModuleWindow = window
		r.proofreadingModuleRequests = requests
	}
	select {
	case requests <- request:
		r.mu.Unlock()
		return window, requests, start, nil
	default:
		r.mu.Unlock()
		request.sourceSnapshot.Close()
		return nil, nil, false, fmt.Errorf("校对模块正在打开任务，请稍后重试")
	}
}

func (r *desktopRuntime) runProofreadingModuleWindow(window *app.Window, requests chan proofreadingWindowRequest) {
	a := newAppWithRuntimeAndWindowID(nil, "", r, 0)
	a.window = window
	a.proofreadingModule = true
	a.proofreadingModuleRequests = requests
	a.attachWindow(0, a)
	a.startFontService()
	r.runAppWindow(a)
	r.finishProofreadingModuleWindow(window, requests)
}

// A previous window may finish saving after its replacement has opened.
// Release only that previous window's queue and preserve the replacement.
func (r *desktopRuntime) finishProofreadingModuleWindow(window *app.Window, requests chan proofreadingWindowRequest) {
	r.mu.Lock()
	if r.proofreadingModuleWindow == window {
		r.proofreadingModuleWindow = nil
		r.proofreadingModuleRequests = nil
	}
	r.mu.Unlock()
	for {
		select {
		case request := <-requests:
			if request.sourceSnapshot != nil {
				request.sourceSnapshot.Close()
			}
		default:
			return
		}
	}
}
