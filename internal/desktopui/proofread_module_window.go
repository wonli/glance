package desktopui

import (
	"fmt"
	"gioui.org/app"
	"gioui.org/io/system"
)

// The proofreading module has its own window and tab collection. It shares
// local persistence with the application, never its home route or open tabs.
func (r *desktopRuntime) openProofreadingModuleWindow(request proofreadingWindowRequest) error {
	if request.sourceSnapshot == nil {
		return fmt.Errorf("无法读取当前 PDF 快照")
	}
	r.mu.Lock()
	window := r.proofreadingModuleWindow
	requests := r.proofreadingModuleRequests
	start := window == nil
	if start {
		window = new(app.Window)
		requests = make(chan proofreadingWindowRequest, 16)
		r.proofreadingModuleWindow = window
		r.proofreadingModuleRequests = requests
	}
	r.mu.Unlock()
	select {
	case requests <- request:
	default:
		request.sourceSnapshot.Close()
		return fmt.Errorf("校对模块正在打开任务，请稍后重试")
	}
	if start {
		configureDesktopWindow(window)
		go r.runProofreadingModuleWindow(window, requests)
	} else {
		window.Perform(system.ActionRaise)
		window.Invalidate()
	}
	return nil
}

func (r *desktopRuntime) runProofreadingModuleWindow(window *app.Window, requests chan proofreadingWindowRequest) {
	a := newAppWithRuntimeAndWindowID(nil, "", r, 0)
	a.window = window
	a.proofreadingModule = true
	a.proofreadingModuleRequests = requests
	a.attachWindow(0, a)
	a.startFontService()
	r.runAppWindow(a)
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
