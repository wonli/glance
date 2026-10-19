package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"
	"strings"

	"glance/internal/format/jsondoc"
)

type jsonToolResult struct {
	id, source, text   string
	data               json.RawMessage
	err                error
	applicationCommand bool
	operationToken     sessionOperationToken
	generation         uint64
}

// Transform an immutable snapshot off the window loop. A newer source draft
// cancels installation, without reserving a session mutation while parsing.
func (a *App) startJSONTool(id string) {
	v := a.ensureJSONView()
	if v.toolPending {
		return
	}
	source := a.jsonDraftText()
	v.toolPending = true
	if a.startJSONApplicationTool(v, id, source) {
		return
	}
	results, window, done := v.toolResults, a.window, v.done
	a.launchManagedBackground(func() {
		defer releaseClosedJSONWorkerMemory(done, len(source) > jsonSourceEditLimit)
		result := jsonToolResult{id: id, source: source}
		switch id {
		case "format":
			result.text, result.err = jsondoc.Format(source)
		case "minify":
			result.text, result.err = jsondoc.Minify(source)
		case "sort-keys":
			result.text, result.err = jsondoc.SortKeys(source)
		case "validate":
			result.err = jsondoc.Validate(source)
		}
		select {
		case <-done:
			return
		default:
		}
		select {
		case results <- result:
		case <-done:
			return
		}
		if window != nil {
			window.Invalidate()
		}
	})
}

type startJSONApplicationToolPayload struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type startJSONApplicationToolSortKeysPayload struct {
	Operation string `json:"operation"`
}

type startJSONApplicationToolPayloadOperation struct {
	Operation string `json:"operation"`
}

type startJSONApplicationToolArguments struct{}

func (a *App) startJSONApplicationTool(v *jsonWorkspaceView, id, source string) bool {
	state := a.documentSessionState
	if state == nil || state.jsonDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	capabilityID := "json.text.transform"
	var arguments any = startJSONApplicationToolArguments{}
	kind := sessionOperationMutation
	switch id {
	case "format":
		arguments = startJSONApplicationToolPayloadOperation{Operation: "format"}
	case "minify":
		arguments = startJSONApplicationToolPayloadOperation{Operation: "minify"}
	case "sort-keys":
		arguments = startJSONApplicationToolSortKeysPayload{Operation: "sort_keys"}
	case "validate":
		capabilityID = "json.document.validate"
		kind = sessionOperationRead
	case "search":
		query := strings.TrimSpace(v.search.Text())
		if query == "" {
			return false
		}
		capabilityID = "json.document.search"
		arguments = startJSONApplicationToolPayload{Query: query, Limit: 100}
		kind = sessionOperationRead
	case "copy":
		capabilityID = "json.document.read_text"
		kind = sessionOperationRead
	default:
		return false
	}
	token, ok := state.beginOperation(kind)
	if !ok {
		v.toolPending = false
		return true
	}
	generation := a.documentGeneration
	results, window := v.toolResults, a.window
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, capabilityID, "json_document", token.documentID, "", arguments)
		state.finishOperation(token)
		toolResult := jsonToolResult{
			id: id, source: source, applicationCommand: true,
			operationToken: token, generation: generation, err: err,
		}
		if err == nil {
			toolResult.data = append(json.RawMessage(nil), result.Data...)
		}
		select {
		case results <- toolResult:
		case <-v.done:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

type startJSONViewApplicationToolPayload struct {
	Operation string `json:"operation"`
	Level     *int   `json:"level,omitempty"`
}

func (a *App) startJSONViewApplicationTool(v *jsonWorkspaceView, operation string, level int) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.jsonDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		v.toolMessage = i18n.Message{ID: "json.status.busy"}
		return true
	}
	arguments := startJSONViewApplicationToolPayload{Operation: operation}
	if operation == "expand_level" {
		arguments.Level = &level
	}
	v.toolPending = true
	generation := a.documentGeneration
	results, window := v.toolResults, a.window
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "json.view.set", "attached_window", fmt.Sprint(a.windowID), "", arguments)
		state.finishOperation(token)
		toolResult := jsonToolResult{
			id: "view", source: state.jsonDoc.Text(), applicationCommand: true,
			operationToken: token, generation: generation, err: err,
		}
		if err == nil {
			toolResult.data = append(json.RawMessage(nil), result.Data...)
		}
		select {
		case results <- toolResult:
		case <-v.done:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

func (a *App) applyJSONToolResult(result jsonToolResult) {
	v := a.ensureJSONView()
	v.toolPending = false
	if result.applicationCommand {
		a.applyJSONApplicationToolResult(v, result)
		return
	}
	if a.jsonDraftText() != result.source || a.jsonDoc.Text() != result.source {
		return
	}
	if result.err != nil {
		message := i18n.Message{ID: "json.error.operation_failed"}
		a.setStatusErrorMessage(message, result.err, notificationToast)
		v.toolMessage = message
		return
	}
	if result.id == "validate" {
		a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_json_is_valid_593b83"}, feedbackSuccess, notificationSuppressed)
		v.toolMessage = i18n.Message{ID: "desktop.status.inline.the_json_is_valid_593b83"}
		return
	}
	if a.applyJSONTextMutationIfSource(result.text, jsonSuccessMessage(result.id), result.source) {
		v.toolMessage = jsonSuccessMessage(result.id)
	} else {
		v.toolMessage = a.statusRef
	}
}

func (a *App) applyJSONApplicationToolResult(v *jsonWorkspaceView, result jsonToolResult) {
	state := result.operationTokenState(a)
	if state != nil {
		state.finishOperation(result.operationToken)
	}
	if result.err != nil {
		message := i18n.Message{ID: "json.error.operation_failed"}
		a.setStatusErrorMessage(message, result.err, notificationToast)
		v.toolMessage = message
		return
	}
	if state == nil || state.jsonDoc == nil {
		return
	}
	if result.id == "view" {
		var view struct {
			Changed bool `json:"changed"`
			Level   int  `json:"level"`
		}
		if err := json.Unmarshal(result.data, &view); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "json.error.operation_failed"}, err, notificationToast)
		} else if view.Changed {
			switch view.Level {
			case -1:
				a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.expanded_the_entire_json_tree_3624e6"}, feedbackInfo, notificationSuppressed)
			case 0:
				a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.collapsed_the_entire_json_tree_d27372"}, feedbackInfo, notificationSuppressed)
			default:
				a.setStatusMessage(i18n.Message{ID: "json.status.tree.expanded_to_level", Args: i18n.Args{"Level": view.Level}}, feedbackInfo, notificationSuppressed)
			}
		} else {
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_json_tree_view_is_unchanged_e20b7b"}, feedbackInfo, notificationSuppressed)
		}
		v.toolMessage = a.statusRef
		return
	}
	if result.id == "validate" {
		var validation struct {
			Valid      bool   `json:"valid"`
			Diagnostic string `json:"diagnostic"`
		}
		if err := json.Unmarshal(result.data, &validation); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "json.error.operation_failed"}, err, notificationToast)
		} else if validation.Valid {
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_json_is_valid_593b83"}, feedbackSuccess, notificationSuppressed)
		} else {
			a.setStatusDetailMessage(i18n.Message{ID: "json.error.operation_failed"}, feedbackError, notificationToast, validation.Diagnostic)
		}
		v.toolMessage = a.statusRef
		return
	}
	if result.id == "search" {
		if state.jsonDoc.Text() != result.source || a.jsonDraftText() != result.source {
			return
		}
		var search struct {
			Matches []struct {
				Pointer string `json:"pointer"`
			} `json:"matches"`
			Truncated bool `json:"truncated"`
		}
		if err := json.Unmarshal(result.data, &search); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "json.error.operation_failed"}, err, notificationToast)
		} else if search.Truncated {
			a.setStatusMessage(i18n.Message{ID: "json.status.search.truncated", Args: i18n.Args{"Count": len(search.Matches)}}, feedbackInfo, notificationSuppressed)
		} else {
			a.setStatusMessage(i18n.Message{ID: "json.status.search.found", Args: i18n.Args{"Count": len(search.Matches)}}, feedbackInfo, notificationSuppressed)
		}
		v.toolMessage = a.statusRef
		return
	}
	if result.id == "copy" {
		if state.jsonDoc.Text() != result.source || a.jsonDraftText() != result.source {
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_json_document_changed_while_it_was_bei_752e16"}, feedbackInfo, notificationToast)
			v.toolMessage = a.statusRef
			return
		}
		var copyResult struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(result.data, &copyResult); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "json.error.operation_failed"}, err, notificationToast)
		} else {
			v.clipboardWriteText = copyResult.Text
			v.clipboardWritePending = true
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.json_copied_14f177"}, feedbackSuccess, notificationSuppressed)
		}
		v.toolMessage = a.statusRef
		return
	}
	var output struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(result.data, &output); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "json.error.operation_failed"}, err, notificationToast)
		v.toolMessage = a.statusRef
		return
	}
	text := state.jsonDoc.Text()
	if text != result.source && a.jsonDraftText() != result.source {
		// Keep a newer editor draft visible; the adapter mutation remains
		// committed and can be reconciled after the draft is resolved.
		v.sourceSynced = text
		return
	}
	if len(text) <= jsonSourceEditLimit {
		a.jsonEditor.SetText(text)
		v.sourceLoaded = true
	} else if v.sourceLoaded {
		a.jsonEditor.SetText("")
		v.sourceLoaded = false
	}
	v.sourceSynced = text
	v.requested = text
	if output.Changed {
		a.persistActiveSession()
		a.setStatusMessage(jsonSuccessMessage(result.id), feedbackSuccess, notificationToast)
	} else {
		a.setStatusMessage(i18n.Message{ID: "json.status.operation_unchanged"}, feedbackInfo, notificationSuppressed)
	}
	v.toolMessage = a.statusRef
}

func jsonSuccessMessage(id string) i18n.Message {
	switch id {
	case "minify":
		return i18n.Message{ID: "json.status.minified"}
	case "sort-keys":
		return i18n.Message{ID: "json.status.keys_sorted"}
	case "copy":
		return i18n.Message{ID: "json.status.copied"}
	default:
		return i18n.Message{ID: "json.status.formatted"}
	}
}

func (result jsonToolResult) operationTokenState(a *App) *documentSessionState {
	if result.generation != a.documentGeneration || a.documentSessionState == nil || a.documentSessionState.documentID != result.operationToken.documentID {
		return nil
	}
	return a.documentSessionState
}

func (a *App) pollJSONToolResults() {
	v := a.ensureJSONView()
	select {
	case result := <-v.toolResults:
		a.applyJSONToolResult(result)
	default:
	}
}
