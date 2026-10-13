package desktopui

import (
	"context"
	"encoding/json"
	"fmt"

	"glance/internal/application"
	"glance/internal/document"
)

type wordViewSelection struct {
	Scope string `json:"scope"`
}

type wordViewArguments struct {
	Selection         *wordViewSelection `json:"selection,omitempty"`
	Enabled           *bool              `json:"enabled,omitempty"`
	Mode              string             `json:"mode,omitempty"`
	MultiplePages     *bool              `json:"multiple_pages,omitempty"`
	NavigationVisible *bool              `json:"navigation_visible,omitempty"`
	ReadingMode       *bool              `json:"reading_mode,omitempty"`
	RulerVisible      *bool              `json:"ruler_visible,omitempty"`
	GridlinesVisible  *bool              `json:"gridlines_visible,omitempty"`
}

func commandArgumentPointer[T any](value T) *T { return &value }

func (a *App) applicationSessionRegistered(documentID string) bool {
	runtime := a.ensureDesktopRuntime()
	if runtime.applicationRuntimeErr != nil || runtime.applicationRuntime == nil || documentID == "" {
		return false
	}
	_, err := runtime.applicationRuntime.GetDocument(context.Background(), documentID)
	return err == nil
}

func (a *App) executeRibbonApplicationCommand(token sessionOperationToken, commandID, targetType, targetID, sheet string, arguments any) (application.CommandResult, error) {
	return a.executeRibbonApplicationCommandWithContext(context.Background(), token, commandID, targetType, targetID, sheet, arguments)
}

func (a *App) executeRibbonApplicationCommandWithContext(ctx context.Context, token sessionOperationToken, commandID, targetType, targetID, sheet string, arguments any) (application.CommandResult, error) {
	runtime := a.ensureDesktopRuntime()
	if runtime.applicationRuntimeErr != nil {
		return application.CommandResult{}, runtime.applicationRuntimeErr
	}
	if runtime.applicationRuntime == nil {
		return application.CommandResult{}, application.ErrRuntimeClosed
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return application.CommandResult{}, err
	}
	invocation, err := runtime.applicationRuntime.InvokeCapability(ctx, application.CapabilityRequest{
		RequestID:  fmt.Sprintf("ribbon-%d-%d", a.windowID, a.applicationRequestSeq.Add(1)),
		DocumentID: token.documentID, CapabilityID: commandID, ExpectedRevision: token.revision,
		Operator: application.Operator{ID: fmt.Sprintf("window-%d", a.windowID), Kind: application.OperatorHuman},
		View: application.ViewContext{
			WindowID: a.windowID, DocumentID: token.documentID, Revision: token.revision,
			Selection: &application.SelectionContext{TargetType: targetType, TargetID: targetID, Sheet: sheet},
		},
		Arguments: encoded,
	})
	if err != nil {
		return application.CommandResult{}, err
	}
	return application.CommandResult{
		RequestID: invocation.RequestID, DocumentID: invocation.DocumentID,
		Revision: invocation.Revision, Data: invocation.Data,
	}, nil
}

func (a *App) runWordViewRibbonAction(actionID string) bool {
	state := a.documentSessionState
	if state == nil || state.word == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	commandID := "word.view.set"
	arguments := wordViewArguments{}
	switch actionID {
	case "select":
		arguments.Selection = &wordViewSelection{Scope: "document"}
	case "autosave":
		commandID = "word.autosave.set"
		arguments.Enabled = commandArgumentPointer(!state.wordAutoSave)
	case "view-print-layout":
		mode := "print"
		if state.wordViewMode == wordViewPrintLayout {
			mode = "continuous"
		}
		arguments.Mode = mode
	case "view-web-layout":
		mode := "web"
		if state.wordViewMode == wordViewWebLayout {
			mode = "continuous"
		}
		arguments.Mode = mode
	case "view-navigation":
		arguments.NavigationVisible = commandArgumentPointer(!state.wordNavigationVisible)
	case "view-single-page":
		arguments.Mode, arguments.MultiplePages = "print", commandArgumentPointer(false)
	case "view-multiple-pages":
		arguments.Mode, arguments.MultiplePages = "print", commandArgumentPointer(true)
	case "view-reading":
		arguments.ReadingMode = commandArgumentPointer(!state.wordReadingMode)
	case "view-ruler":
		arguments.RulerVisible = commandArgumentPointer(state.wordRulerHidden)
	case "view-gridlines":
		arguments.GridlinesVisible = commandArgumentPointer(!state.wordGridlinesVisible)
	case "view-new-window":
		commandID = "word.view.new_window"
	case "view-side-by-side":
		commandID = "word.view.side_by_side"
	case "view-zoom":
		commandID = "word.zoom.dialog"
	default:
		return false
	}
	generation := a.documentGeneration
	operation := "切换 Word 视图"
	if actionID == "select" {
		operation = "选择 Word 正文"
	} else if actionID == "autosave" {
		operation = "设置 Word 自动保存"
	} else if actionID == "view-zoom" {
		operation = "打开 Word 缩放设置"
	}
	var token sessionOperationToken
	trackOperation := true
	if actionID == "view-new-window" {
		// Opening another view does not read or mutate the format adapter. Keep
		// this available while a shared document operation is in progress.
		trackOperation = false
		token = sessionOperationToken{documentID: state.documentID, revision: state.wordRevisionAtomic.Load()}
	} else {
		var ok bool
		token, ok = state.beginOperation(sessionOperationRead)
		if !ok {
			return true
		}
	}
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, commandID, "word_document", token.documentID, "", arguments)
		revision := uint64(0)
		if err == nil {
			revision = result.Revision
		}
		a.publishWordCommand(wordCommandResult{
			state: state, operationToken: token, generation: generation, operation: operation,
			applicationCommand: true, viewOnly: true, skipOperationTracking: !trackOperation, revision: revision, err: err,
		})
	})
	return true
}

func (a *App) runWordApplicationCommand(operation, commandID string, arguments any) bool {
	return a.runWordApplicationCommandAtRevision(operation, commandID, arguments, nil)
}

func (a *App) runWordApplicationCommandExpectedRevision(operation, commandID string, arguments any, expectedRevision uint64) bool {
	return a.runWordApplicationCommandAtRevision(operation, commandID, arguments, &expectedRevision)
}

func (a *App) runWordApplicationCommandAtRevision(operation, commandID string, arguments any, expectedRevision *uint64) bool {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return false
	}
	if expectedRevision != nil && token.revision != *expectedRevision {
		state.finishOperation(token)
		return false
	}
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, commandID, "word_document", token.documentID, "", arguments)
		revision := uint64(0)
		var blocks []document.WordBlock
		confirmationPending := false
		var trackRevisions *bool
		if err == nil {
			revision = result.Revision
			if commandID == "word.text_box.insert" || commandID == "word.table.insert" || commandID == "word.equation.insert" || commandID == "word.paragraph.alignment.set" || commandID == "word.image.move" || commandID == "word.image.wrap.set" {
				unlock := lockSessionAdapterRead(state)
				blocks = cloneWordBlockSnapshot(state.word.WordBlocks())
				unlock()
			}
			var state struct {
				Status        string `json:"status"`
				RevisionCount *int   `json:"revision_count"`
				Enabled       *bool  `json:"enabled"`
			}
			if json.Unmarshal(result.Data, &state) == nil {
				confirmationPending = state.Status == "confirmation_required"
				if commandID == "word.review.revisions.apply" && state.RevisionCount != nil {
					operation = fmt.Sprintf("%s（处理 %d 处）", operation, *state.RevisionCount)
				}
				if commandID == "word.review.track.set" && state.Enabled != nil {
					enabled := *state.Enabled
					trackRevisions = &enabled
				}
			}
		}
		a.publishWordCommand(wordCommandResult{
			state: state, operationToken: token, generation: generation, operation: operation,
			applicationCommand: true, revision: revision, blocks: blocks, confirmationPending: confirmationPending, wordTrackRevisions: trackRevisions, err: err,
		})
	})
	return true
}
