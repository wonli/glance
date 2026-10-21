package desktopui

import (
	"encoding/json"
	"errors"
	"io"

	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"glance/internal/format/pdfdoc"
	"glance/internal/i18n"
	"glance/internal/workspace"
)

var errPDFWorkspaceRestoreRequired = errors.New("PDF workspace restore must run asynchronously")

type pdfWorkspaceRestoreResult struct {
	tabID      string
	generation uint64
	record     workspace.Session
	doc        *pdfdoc.Document
	err        error
}

func (a *App) invalidatePDFWorkspaceRestore() {
	if a == nil {
		return
	}
	a.workspacePDFRestoreGeneration++
	if a.pendingPDFRestoreSaveOnReady {
		a.pendingPDFRestoreSaveOnReady = false
		a.pendingCloseTabID = ""
	}
	a.pendingPDFRestoreTabID = ""
}

// startWorkspacePDFRestore streams a stored PDF snapshot or opens its source
// file outside the Gio frame loop. The result is installed on the UI thread.
func (a *App) startWorkspacePDFRestore(record workspace.Session) bool {
	if a == nil || a.collection == nil || record.Tab.ID == "" || record.Tab.DocumentID == "" {
		return false
	}
	if a.pendingPDFRestoreTabID == record.Tab.ID {
		return true
	}
	a.collection.ReleasePayload(record.Tab.ID)
	record.Payload = nil
	record.ViewState = append([]byte(nil), record.ViewState...)
	a.workspacePDFRestoreGeneration++
	generation := a.workspacePDFRestoreGeneration
	a.pendingPDFRestoreTabID = record.Tab.ID
	openingLabel := record.Tab.Title
	if record.Tab.SourcePath != "" {
		openingLabel = record.Tab.SourcePath
	}
	if !record.SnapshotPresent && record.Tab.SourcePath != "" {
		a.setStatusMessage(i18n.Message{ID: "workspace.status.restore_from_source", Args: i18n.Args{"Path": openingLabel}}, feedbackInfo, notificationSuppressed)
	} else {
		a.setStatusMessage(i18n.Message{ID: "document.opening", Args: i18n.Args{"Path": openingLabel}}, feedbackInfo, notificationSuppressed)
	}
	runtime := a.ensureDesktopRuntime()
	if runtime.isPrimaryWindow(a.windowID) {
		if err := a.collection.Activate(record.Tab.ID); err != nil {
			a.pendingPDFRestoreTabID = ""
			a.setStatusErrorMessage(workspaceRestoreTabMessage(record.Tab.Title), err, notificationToast)
			return false
		}
	}
	collection := a.collection
	if !a.launchManagedBackground(func() {
		var doc *pdfdoc.Document
		var err error
		switch {
		case record.SnapshotPresent:
			doc, err = pdfdoc.OpenSnapshotStream(func(dst io.Writer) error {
				_, streamErr := collection.StreamPayload(record.Tab.ID, dst)
				return streamErr
			})
		case record.Tab.SourcePath != "":
			doc, err = pdfdoc.Open(record.Tab.SourcePath)
		default:
			doc, err = pdfdoc.NewBlank()
		}
		if err == nil {
			doc.SetSourcePath(record.Tab.SourcePath)
			doc.SetDirty(record.Dirty)
		}
		result := pdfWorkspaceRestoreResult{tabID: record.Tab.ID, generation: generation, record: record, doc: doc, err: err}
		select {
		case a.workspacePDFRestoreResults <- result:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
			if doc != nil {
				_ = doc.Close()
			}
		}
	}) {
		a.pendingPDFRestoreTabID = ""
		return false
	}
	return true
}

func workspaceRestoreTabMessage(title string) i18n.Message {
	if title == "" {
		title = "PDF"
	}
	return i18n.Message{ID: "workspace.error.restore_tab", Args: i18n.Args{"Title": title}}
}

func (a *App) processPDFWorkspaceRestoreResults() {
	if a == nil || a.workspacePDFRestoreResults == nil {
		return
	}
	for range max(1, cap(a.workspacePDFRestoreResults)) {
		select {
		case result := <-a.workspacePDFRestoreResults:
			a.applyPDFWorkspaceRestoreResult(result)
		default:
			return
		}
	}
}

func (a *App) discardPDFWorkspaceRestoreResults() {
	if a == nil || a.workspacePDFRestoreResults == nil {
		return
	}
	for {
		select {
		case result := <-a.workspacePDFRestoreResults:
			if result.doc != nil {
				_ = result.doc.Close()
			}
		default:
			return
		}
	}
}

func (a *App) applyPDFWorkspaceRestoreResult(result pdfWorkspaceRestoreResult) {
	closeResult := func() {
		if result.doc != nil {
			_ = result.doc.Close()
		}
	}
	if a == nil || result.tabID == "" || result.generation != a.workspacePDFRestoreGeneration || result.tabID != a.pendingPDFRestoreTabID {
		closeResult()
		return
	}
	if result.tabID != a.workspaceSelectedTabID() {
		closeResult()
		return
	}
	if a.collection == nil {
		a.pendingPDFRestoreTabID = ""
		closeResult()
		return
	}
	current, exists := a.collection.GetMetadata(result.tabID)
	if !exists || current.Tab.DocumentID != result.record.Tab.DocumentID {
		a.pendingPDFRestoreTabID = ""
		if a.pendingCloseTabID == result.tabID {
			a.pendingCloseTabID = ""
			a.pendingPDFRestoreSaveOnReady = false
		}
		closeResult()
		return
	}
	a.pendingPDFRestoreTabID = ""
	if result.err != nil {
		if a.pendingCloseTabID == result.tabID {
			a.pendingCloseTabID = ""
			a.pendingPDFRestoreSaveOnReady = false
		}
		closeResult()
		a.setStatusErrorMessage(workspaceRestoreTabMessage(result.record.Tab.Title), result.err, notificationToast)
		return
	}
	if result.doc == nil {
		a.setStatusErrorMessage(workspaceRestoreTabMessage(result.record.Tab.Title), errors.New("PDF restore produced no document"), notificationToast)
		return
	}
	record := result.record
	record.Tab = current.Tab
	record.Format = current.Format
	record.Dirty = current.Dirty
	record.ViewState = append([]byte(nil), current.ViewState...)
	result.doc.SetSourcePath(record.Tab.SourcePath)
	result.doc.SetDirty(record.Dirty)
	if state := a.sessions[result.tabID]; state != nil && !state.closed.Load() {
		closeResult()
		a.activateSession(result.tabID, state)
		return
	}
	runtime := a.ensureDesktopRuntime()
	candidate := newDocumentSessionState(record.Tab.SourcePath)
	candidate.documentID = record.Tab.DocumentID
	candidate.pdfDoc = result.doc
	candidate.pdfSnapshotPresent = record.SnapshotPresent
	if record.SnapshotPresent {
		candidate.pdfPersistedRevision = result.doc.Info().Revision
	}
	shared := runtime.registerSession(candidate.sharedDocumentSession)
	var state *documentSessionState
	if shared == candidate.sharedDocumentSession {
		state = candidate
	} else {
		closeUnregisteredSessionAdapters(candidate.sharedDocumentSession, shared)
		state = newDocumentSessionView(shared, a.windowID)
	}
	state.documentID = record.Tab.DocumentID
	state.windowID = a.windowID
	state.sharedDocumentSession.attachView(a.windowID, state)
	restorePDFWorkspaceViewState(state, record.ViewState)
	a.sessions[result.tabID] = state
	saveAfterRestore := a.pendingCloseTabID == result.tabID && a.pendingPDFRestoreSaveOnReady
	if saveAfterRestore {
		a.pendingPDFRestoreSaveOnReady = false
	}
	if a.statusRef.ID == "document.opening" || a.statusRef.ID == "workspace.status.restore_from_source" {
		a.clearStatus()
	}
	a.activateSession(result.tabID, state)
	if saveAfterRestore {
		if a.path == "" {
			a.startFileCommand(commandSaveAs, "pdf")
		} else {
			a.startSave()
		}
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func restorePDFWorkspaceViewState(state *documentSessionState, raw []byte) {
	if state == nil || state.pdfDoc == nil {
		return
	}
	view := &pdfViewState{
		info: state.pdfDoc.Info(), selected: -1, selectStart: -1, selectEnd: -1,
		zoom: 1, fit: "page", mode: "read", thumbnails: true,
		buttons: map[string]*widget.Clickable{}, results: make(chan pdfPageResult, 1),
		cache: map[int]pdfPageResult{}, thumbCache: map[int]paint.ImageOp{}, thumbErrors: map[int]string{},
	}
	view.toolbar.Axis = layout.Horizontal
	view.thumbList.Axis = layout.Vertical
	view.canvas.Axis = layout.Vertical
	view.horizontal.Axis = layout.Horizontal
	view.notes.Axis = layout.Vertical
	view.search.SingleLine = true
	var saved sessionViewSnapshot
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &saved)
	}
	if saved.PDFPageNumber > 0 {
		lastPage := max(0, view.info.Pages-1)
		view.page = min(lastPage, max(0, saved.PDFPageNumber-1))
		view.canvas.Position.First = view.page
		view.thumbList.Position.First = view.page
	}
	if saved.PDFZoomPercent >= 25 && saved.PDFZoomPercent <= 400 {
		view.zoom = float64(saved.PDFZoomPercent) / 100
	}
	switch saved.PDFFitMode {
	case "page", "width":
		view.fit = saved.PDFFitMode
	case "custom":
		view.fit = ""
	}
	switch saved.PDFMode {
	case "read", "annotate", "signature":
		view.mode = saved.PDFMode
	}
	view.tool = saved.PDFTool
	if saved.PDFThumbnails != nil {
		view.thumbnails = *saved.PDFThumbnails
	}
	view.showSearch = saved.PDFSearchVisible
	state.pdfView = view
}
