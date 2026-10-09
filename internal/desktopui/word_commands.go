package desktopui

import "glance/internal/document"

func (a *App) runWordCommand(operation string, edit func(document.WordTextEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordTextEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	}()
}

func (a *App) runWordListCommand(operation string, edit func(document.WordParagraphListEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordParagraphListEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	}()
}

func (a *App) runWordImageCommand(operation, imageID string, edit func(document.WordImageEditor) error, clearSelection bool) bool {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return false
	}
	editor, ok := any(state.word).(document.WordImageEditor)
	if !ok {
		return false
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return false
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, clearImageSelection: clearSelection && err == nil, imageID: imageID, blocks: blocks})
	}()
	return true
}

func (a *App) rotateSelectedWordImage(delta float64) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	image := findWordImage(wordBlocksForState(state), state.selectedWordImageID)
	if image == nil {
		return
	}
	imageID := image.ID
	a.runWordImageRotationCommand(imageID, image.RotationDegrees+delta)
}

type wordCommandResult struct {
	state                 *documentSessionState
	operationToken        sessionOperationToken
	revision              uint64
	blocks                []document.WordBlock
	selectInsertedImage   bool
	generation            uint64
	operation             string
	err                   error
	clearImageSelection   bool
	imageID               string
	undo                  bool
	history               bool
	applied               bool
	textDraft             bool
	paragraphSplit        bool
	paragraphJoin         bool
	runID                 string
	runText               string
	splitSourceRunID      string
	splitPrefix           string
	splitParagraphID      string
	splitRunID            string
	splitRunText          string
	joinParagraphID       string
	joinRunID             string
	joinCaret             int
	insertedParagraphID   string
	bodyParagraphInsert   bool
	wordRangeReplacement  bool
	wordSymbolInsertion   bool
	pageBreak             bool
	replaceRunID          string
	replaceParagraphID    string
	replaceStart          int
	replaceText           string
	replaceQuery          string
	insertedSymbol        string
	pageBreakSourcePrefix string
	pageBreakParagraphID  string
	pageBreakCaretRunID   string
}

func (a *App) publishWordCommand(result wordCommandResult) {
	if result.state != nil && result.operationToken.generation == 0 {
		result.operationToken, _ = result.state.currentOperationToken()
	}
	if result.state != nil && result.operationToken.generation != 0 {
		if result.err == nil && (!result.history || result.applied) {
			if result.blocks != nil {
				if result.history {
					result.state.setWordBlocksAndTextState(result.blocks, false)
				} else {
					result.state.setWordBlocks(result.blocks)
				}
			}
			result.revision = result.operationToken.revision + 1
			if !a.ensureDesktopRuntime().publishSessionRevision(result.state.documentID, max(result.revision, result.state.wordRevisionAtomic.Load())) {
				result.state.advanceRevision(result.revision)
			}
		}
		if result.revision == 0 {
			result.revision = result.state.wordRevisionAtomic.Load()
		}
		result.state.finishOperation(result.operationToken)
	}
	if a.wordCommandResults == nil {
		return
	}
	select {
	case a.wordCommandResults <- result:
		if a.window != nil {
			a.window.Invalidate()
		}
	case <-a.shutdown:
	}
}

func (a *App) applyWordHistory(undo bool) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	history, ok := any(state.word).(document.WordHistory)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		var applied bool
		blocks, err := runWordMutation(state, func() error {
			var operationErr error
			if undo {
				applied, operationErr = history.Undo()
			} else {
				applied, operationErr = history.Redo()
			}
			return operationErr
		})
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "编辑", err: err, history: true, applied: applied, undo: undo, blocks: blocks})
	}()
}
