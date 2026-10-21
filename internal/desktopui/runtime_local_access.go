package desktopui

import (
	"context"

	"gioui.org/widget"

	"glance/internal/localaccess"
)

type localAccessPickerRequest struct {
	id       uint64
	context  context.Context
	request  localaccess.SelectionRequest
	response chan localAccessPickerResult
}

type localAccessPickerResult struct {
	request localAccessPickerRequest
	path    string
	err     error
}

type localAccessConsentRequest struct {
	id       uint64
	context  context.Context
	request  localaccess.ConsentRequest
	response chan bool
}

type agentLocalAccessConsent struct {
	id       uint64
	context  context.Context
	request  localaccess.ConsentRequest
	response chan bool
	reject   widget.Clickable
	allow    widget.Clickable
	scrim    widget.Clickable
}

type agentLocalAccessUIState struct {
	pickerRequests  chan localAccessPickerRequest
	pickerResults   chan localAccessPickerResult
	consentRequests chan localAccessConsentRequest
	cancelRequests  chan uint64
	pickerQueue     []localAccessPickerRequest
	consentQueue    []localAccessConsentRequest
	activeConsent   *agentLocalAccessConsent
	pickerRunning   bool
	pickerID        uint64
	cancelledIDs    map[uint64]struct{}
	pickerRunner    func(context.Context, filePickerLabels) (string, error)
}

func newAgentLocalAccessUIState() agentLocalAccessUIState {
	return agentLocalAccessUIState{
		pickerRequests:  make(chan localAccessPickerRequest, 16),
		pickerResults:   make(chan localAccessPickerResult, 8),
		consentRequests: make(chan localAccessConsentRequest, 16),
		cancelRequests:  make(chan uint64, 32),
		cancelledIDs:    make(map[uint64]struct{}),
	}
}

func (r *desktopRuntime) SelectDirectory(ctx context.Context, request localaccess.SelectionRequest) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	app, err := r.localAccessPromptWindow(request.WindowID)
	if err != nil {
		return "", err
	}
	lock := r.localAccessPickerGate()
	select {
	case lock <- struct{}{}:
		defer func() { <-lock }()
	case <-ctx.Done():
		return "", ctx.Err()
	case <-app.shutdown:
		return "", localaccess.ErrInteractiveAuthorizationUnavailable
	}
	id := r.localAccessSeq.Add(1)
	response := make(chan localAccessPickerResult, 1)
	selection := localAccessPickerRequest{id: id, context: ctx, request: request, response: response}
	select {
	case app.agentUI.localAccess.pickerRequests <- selection:
		if app.window != nil {
			app.window.Invalidate()
		}
	case <-ctx.Done():
		return "", ctx.Err()
	case <-app.shutdown:
		return "", localaccess.ErrInteractiveAuthorizationUnavailable
	}
	select {
	case result := <-response:
		return result.path, result.err
	case <-ctx.Done():
		r.cancelLocalAccessPrompt(app, id)
		return "", ctx.Err()
	case <-app.shutdown:
		r.cancelLocalAccessPrompt(app, id)
		return "", localaccess.ErrInteractiveAuthorizationUnavailable
	}
}

func (r *desktopRuntime) localAccessPickerGate() chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.localAccessPickerLock == nil {
		r.localAccessPickerLock = make(chan struct{}, 1)
	}
	return r.localAccessPickerLock
}

func (r *desktopRuntime) ConfirmDirectory(ctx context.Context, request localaccess.ConsentRequest) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	app, err := r.localAccessPromptWindow(request.WindowID)
	if err != nil {
		return false, err
	}
	id := r.localAccessSeq.Add(1)
	response := make(chan bool, 1)
	consent := localAccessConsentRequest{id: id, context: ctx, request: request, response: response}
	select {
	case app.agentUI.localAccess.consentRequests <- consent:
		if app.window != nil {
			app.window.Invalidate()
		}
	case <-ctx.Done():
		return false, ctx.Err()
	case <-app.shutdown:
		return false, localaccess.ErrInteractiveAuthorizationUnavailable
	}
	select {
	case allowed := <-response:
		return allowed, nil
	case <-ctx.Done():
		r.cancelLocalAccessPrompt(app, id)
		return false, ctx.Err()
	case <-app.shutdown:
		r.cancelLocalAccessPrompt(app, id)
		return false, localaccess.ErrInteractiveAuthorizationUnavailable
	}
}

func (r *desktopRuntime) localAccessPromptWindow(windowID uint64) (*App, error) {
	if r == nil || windowID == 0 {
		return nil, localaccess.ErrInteractiveAuthorizationUnavailable
	}
	r.mu.Lock()
	app := r.windows[windowID]
	closing := r.closingWindows[windowID]
	r.mu.Unlock()
	if app == nil || closing || app.agentUI.localAccess.pickerRequests == nil || app.agentUI.localAccess.consentRequests == nil || isAppShuttingDown(app.shutdown) {
		return nil, localaccess.ErrInteractiveAuthorizationUnavailable
	}
	return app, nil
}

func (r *desktopRuntime) cancelLocalAccessPrompt(app *App, id uint64) {
	if app == nil || id == 0 {
		return
	}
	select {
	case app.agentUI.localAccess.cancelRequests <- id:
	default:
	}
	if app.window != nil {
		app.window.Invalidate()
	}
}

func (a *App) processAgentLocalAccessUI() {
	if a == nil {
		return
	}
	state := &a.agentUI.localAccess
	for {
		select {
		case id := <-state.cancelRequests:
			a.cancelAgentLocalAccessRequest(id)
		default:
			goto cancellationsDrained
		}
	}

cancellationsDrained:
	for {
		select {
		case result := <-state.pickerResults:
			if state.pickerRunning && state.pickerID == result.request.id {
				state.pickerRunning = false
				state.pickerID = 0
			}
			if _, cancelled := state.cancelledIDs[result.request.id]; cancelled {
				delete(state.cancelledIDs, result.request.id)
			} else {
				result.request.response <- result
			}
		default:
			goto pickerResultsDrained
		}
	}

pickerResultsDrained:
	for {
		select {
		case request := <-state.pickerRequests:
			if _, cancelled := state.cancelledIDs[request.id]; cancelled {
				delete(state.cancelledIDs, request.id)
				continue
			}
			state.pickerQueue = append(state.pickerQueue, request)
		default:
			goto pickerRequestsDrained
		}
	}

pickerRequestsDrained:
	for {
		select {
		case request := <-state.consentRequests:
			if _, cancelled := state.cancelledIDs[request.id]; cancelled {
				delete(state.cancelledIDs, request.id)
				request.response <- false
				continue
			}
			state.consentQueue = append(state.consentQueue, request)
		default:
			goto consentRequestsDrained
		}
	}

consentRequestsDrained:
	if !state.pickerRunning && len(state.pickerQueue) > 0 {
		a.startNextAgentLocalAccessPicker()
	}
	if state.activeConsent == nil && len(state.consentQueue) > 0 {
		request := state.consentQueue[0]
		state.consentQueue = state.consentQueue[1:]
		if request.context.Err() != nil {
			request.response <- false
		} else {
			state.activeConsent = &agentLocalAccessConsent{id: request.id, context: request.context, request: request.request, response: request.response}
		}
	}
	if consent := state.activeConsent; consent != nil && consent.context.Err() != nil {
		a.resolveAgentLocalAccessConsent(consent, false)
	}
}

func (a *App) startNextAgentLocalAccessPicker() {
	state := &a.agentUI.localAccess
	request := state.pickerQueue[0]
	state.pickerQueue = state.pickerQueue[1:]
	if request.context.Err() != nil {
		request.response <- localAccessPickerResult{request: request, err: request.context.Err()}
		return
	}
	state.pickerRunning = true
	state.pickerID = request.id
	labels := a.filePickerLabels()
	labels.OpenFolder = a.textID("agent.local_access.picker.title")
	runner := state.pickerRunner
	started := a.launchManagedBackground(func() {
		var path string
		var err error
		if runner != nil {
			path, err = runner(request.context, labels)
		} else {
			path, err = chooseOfficeFolderContext(request.context, labels)
		}
		result := localAccessPickerResult{request: request, path: path, err: err}
		select {
		case state.pickerResults <- result:
		case <-a.shutdown:
			return
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	})
	if !started {
		state.pickerRunning = false
		state.pickerID = 0
		request.response <- localAccessPickerResult{request: request, err: localaccess.ErrInteractiveAuthorizationUnavailable}
	}
}

func (a *App) cancelAgentLocalAccessRequest(id uint64) {
	if id == 0 {
		return
	}
	state := &a.agentUI.localAccess
	found := false
	state.pickerQueue, found = removePickerRequest(state.pickerQueue, id)
	if state.pickerRunning && state.pickerID == id {
		state.cancelledIDs[id] = struct{}{}
		found = true
	}
	var removed bool
	state.consentQueue, removed = removeConsentRequest(state.consentQueue, id)
	found = found || removed
	if state.activeConsent != nil && state.activeConsent.id == id {
		state.activeConsent.response <- false
		state.activeConsent = nil
		found = true
	}
	if !found {
		state.cancelledIDs[id] = struct{}{}
	}
}

func removePickerRequest(requests []localAccessPickerRequest, id uint64) ([]localAccessPickerRequest, bool) {
	for index, request := range requests {
		if request.id == id {
			return append(requests[:index], requests[index+1:]...), true
		}
	}
	return requests, false
}

func removeConsentRequest(requests []localAccessConsentRequest, id uint64) ([]localAccessConsentRequest, bool) {
	for index, request := range requests {
		if request.id == id {
			request.response <- false
			return append(requests[:index], requests[index+1:]...), true
		}
	}
	return requests, false
}

func (a *App) activeAgentLocalAccessConsent() *agentLocalAccessConsent {
	if a == nil {
		return nil
	}
	return a.agentUI.localAccess.activeConsent
}

func (a *App) resolveAgentLocalAccessConsent(consent *agentLocalAccessConsent, allowed bool) {
	if a == nil || consent == nil || a.agentUI.localAccess.activeConsent != consent {
		return
	}
	consent.response <- allowed
	a.agentUI.localAccess.activeConsent = nil
	if a.window != nil {
		a.window.Invalidate()
	}
}

var _ localaccess.Prompt = (*desktopRuntime)(nil)
