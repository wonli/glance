package desktopui

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/application"
	"glance/internal/i18n"
)

type wordEditorReviewAsyncResult struct {
	state      *documentSessionState
	generation uint64
	request    uint64
	revision   uint64
	data       []byte
	err        error
}

func (a *App) showWordEditorDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.operation_busy"}, feedbackWarning, notificationToast)
		return
	}
	a.wordEditorReviewRequest++
	request := a.wordEditorReviewRequest
	generation := a.documentGeneration
	window, shutdown, results := a.window, a.shutdown, a.wordEditorReviewResults
	a.modal.ShowMessage(dialogIDWordEditorReview, i18n.Message{ID: "word.review.editor.title"}, i18n.Message{ID: "word.review.editor.scanning"}, func(string) {
		if a.wordEditorReviewRequest == request {
			a.wordEditorReviewRequest++
		}
		a.modal.Hide()
		if window != nil {
			window.Invalidate()
		}
	}, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
	a.launchManagedBackground(func() {
		command, err := a.executeRibbonApplicationCommand(token, "word.review.editor.analyze", "word_document", token.documentID, "", map[string]int{"start_block": 0, "max_blocks": 1000})
		revision := token.revision
		if err == nil {
			revision = command.Revision
		}
		state.finishOperation(token)
		result := wordEditorReviewAsyncResult{
			state: state, generation: generation, request: request,
			revision: revision, data: command.Data, err: err,
		}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
}

func (a *App) applyWordEditorReviewResult(result wordEditorReviewAsyncResult) {
	if result.request != a.wordEditorReviewRequest || !a.modal.Visible || a.modal.DialogID != dialogIDWordEditorReview {
		return
	}
	if result.state == nil || result.state.closed.Load() || result.state != a.documentSessionState || result.generation != a.documentGeneration {
		a.modal.SetMessageRef(i18n.Message{ID: "word.review.editor.error.switched"})
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.revision != result.state.wordRevisionAtomic.Load() {
		a.modal.SetMessageRef(i18n.Message{ID: "word.review.editor.error.stale"})
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.err != nil {
		a.modal.SetMessageRef(i18n.Message{ID: "word.review.editor.error.failed", Args: i18n.Args{"Details": result.err.Error()}})
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	report, err := a.wordEditorReviewMessage(result.data)
	if err != nil {
		a.modal.SetMessageRef(i18n.Message{ID: "word.review.editor.error.read", Args: i18n.Args{"Details": err.Error()}})
	} else {
		a.modal.SetMessageText(report)
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) wordEditorReviewMessage(data []byte) (string, error) {
	var result struct {
		Score                       int `json:"heuristic_score"`
		ScannedBlocks               int `json:"scanned_blocks"`
		TotalBlocks                 int `json:"total_blocks"`
		CharactersWithWhitespace    int `json:"characters_with_whitespace"`
		CharactersWithoutWhitespace int `json:"characters_without_whitespace"`
		Paragraphs                  int `json:"paragraphs"`
		Tables                      int `json:"tables"`
		Findings                    []struct {
			Code        string `json:"code"`
			BlockID     string `json:"block_id"`
			BlockIndex  int    `json:"block_index"`
			ParagraphID string `json:"paragraph_id"`
			RunID       string `json:"run_id"`
			Detail      string `json:"detail"`
		} `json:"findings"`
		FindingsTruncated bool `json:"findings_truncated"`
		HasMore           bool `json:"has_more"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	estMin := result.CharactersWithoutWhitespace / 350
	if estMin < 1 {
		estMin = 1
	}
	report := a.text(i18n.Message{ID: "word.review.editor.report.title", Args: i18n.Args{"Score": result.Score}}) + "\n\n" +
		a.text(i18n.Message{ID: "word.review.editor.report.overview"}) + "\n" +
		a.text(i18n.Message{ID: "word.review.editor.report.scanned", Args: i18n.Args{"Scanned": result.ScannedBlocks, "Total": result.TotalBlocks}}) + "\n" +
		a.text(i18n.Message{ID: "word.review.editor.report.characters", Args: i18n.Args{"Count": result.CharactersWithWhitespace, "WithoutSpaces": result.CharactersWithoutWhitespace}}) + "\n" +
		a.text(i18n.Message{ID: "word.review.editor.report.paragraphs", Args: i18n.Args{"Count": result.Paragraphs}}) + "\n" +
		a.text(i18n.Message{ID: "word.review.editor.report.tables", Args: i18n.Args{"Count": result.Tables}}) + "\n" +
		a.text(i18n.Message{ID: "word.review.editor.report.reading_time", Args: i18n.Args{"Minutes": estMin}}) + "\n\n"
	if len(result.Findings) > 0 {
		report += a.text(i18n.Message{ID: "word.review.editor.report.findings"}) + "\n"
		for i, finding := range result.Findings {
			if i >= 3 {
				report += a.text(i18n.Message{ID: "word.review.editor.report.more_findings"}) + "\n"
				break
			}
			report += a.text(i18n.Message{ID: "word.review.editor.report.finding", Args: i18n.Args{"Index": finding.BlockIndex + 1, "BlockID": finding.BlockID, "ParagraphID": finding.ParagraphID, "RunID": finding.RunID, "Detail": finding.Detail}}) + "\n"
		}
	} else {
		report += a.text(i18n.Message{ID: "word.review.editor.report.no_findings"})
	}
	if result.FindingsTruncated {
		report += "\n" + a.text(i18n.Message{ID: "word.review.editor.report.truncated"})
	}
	if result.HasMore {
		report += "\n" + a.text(i18n.Message{ID: "word.review.editor.report.has_more"})
	}
	report += "\n\n" + a.text(i18n.Message{ID: "word.review.editor.report.limitation"})
	return report, nil
}

type wordTextRunReplaceArguments struct {
	RunId string `json:"run_id"`
	Value string `json:"value"`
}

func (a *App) showWordTranslateDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	blocks := wordBlocksForState(state)
	var activeRunID, originalText string
	if run, ok := selectedWordRun(blocks, state.selectedWordRunID); ok && run.Editable {
		activeRunID = run.ID
		originalText = run.Text
	} else {
		for _, b := range blocks {
			if b.Paragraph != nil {
				for _, in := range b.Paragraph.Inlines {
					if in.Text != nil && in.Text.Editable {
						activeRunID = in.Text.ID
						originalText = in.Text.Text
						break
					}
				}
			}
			if activeRunID != "" {
				break
			}
		}
	}
	if activeRunID == "" {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.no_document_text_is_available_b791d7"}, feedbackWarning, notificationToast)
		return
	}

	previewText := originalText
	if previewText == "" {
		previewText = a.text(i18n.Message{ID: "word.translate.empty_paragraph"})
	} else {
		runes := []rune(previewText)
		if len(runes) > 25 {
			previewText = string(runes[:25]) + "..."
		}
	}

	a.modal.ShowInputMessage("word.translate", i18n.Message{ID: "word.translate.dialog.title"}, i18n.Message{ID: "word.translate.dialog.body", Args: i18n.Args{"Preview": previewText}},
		originalText,
		i18n.Message{ID: "word.translate.dialog.hint"},
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			translated := strings.TrimSpace(a.modal.Input.Text())
			if translated == "" {
				return
			}
			if !a.runWordApplicationCommand("翻译替换", "word.text_run.replace", wordTextRunReplaceArguments{RunId: activeRunID, Value: translated}) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_start_the_word_0fa0c8"}, feedbackWarning, notificationToast)
			}
		},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.translate.action.replace"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) showWordCompareDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	a.wordCompareRequest++
	request := a.wordCompareRequest
	runtime := a.ensureDesktopRuntime()
	if runtime.applicationRuntimeErr != nil || runtime.applicationRuntime == nil {
		a.modal.ShowMessage(dialogIDWordCompare, i18n.Message{ID: "word.compare.dialog.title"}, i18n.Message{ID: "word.compare.error.unavailable"}, func(string) {
			if a.wordCompareRequest == request {
				a.wordCompareRequest++
			}
			a.modal.Hide()
		}, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
		if runtime.applicationRuntimeErr != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "word.compare.error.unavailable"}, runtime.applicationRuntimeErr, notificationToast)
		}
		return
	}
	documents, err := runtime.applicationRuntime.ListDocuments(context.Background())
	if err != nil {
		a.modal.ShowMessage(dialogIDWordCompare, i18n.Message{ID: "word.compare.dialog.title"}, i18n.Message{ID: "word.compare.error.list_documents"}, func(string) {
			if a.wordCompareRequest == request {
				a.wordCompareRequest++
			}
			a.modal.Hide()
		}, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
		a.setStatusErrorMessage(i18n.Message{ID: "word.compare.error.list_documents"}, err, notificationToast)
		return
	}
	otherDocuments := make([]application.DocumentInfo, 0, len(documents))
	labels := make(map[string]string, len(documents))
	for _, info := range documents {
		if info.DocumentID == state.documentID || info.Format != application.FormatDocx {
			continue
		}
		otherDocuments = append(otherDocuments, info)
		actionID := "compare:" + info.DocumentID
		label := a.wordCompareDocumentLabel(info)
		labels[actionID] = label
	}
	if len(otherDocuments) == 0 {
		a.modal.ShowMessage(dialogIDWordCompare, i18n.Message{ID: "word.compare.dialog.title"}, i18n.Message{ID: "word.compare.dialog.body.no_other_documents"}, func(string) {
			if a.wordCompareRequest == request {
				a.wordCompareRequest++
			}
			a.modal.Hide()
		}, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
		return
	}
	currentDocument := a.wordCompareDocumentLabel(application.DocumentInfo{
		DocumentID: state.documentID, SourcePath: sessionPath(state), Dirty: sessionDirty(state),
	})
	a.wordCompareDocumentButtons = make(map[string]*widget.Clickable, len(otherDocuments))
	for _, info := range otherDocuments {
		a.wordCompareDocumentButtons["compare:"+info.DocumentID] = new(widget.Clickable)
	}

	a.modal.ShowCustomMessage(dialogIDWordCompare, i18n.Message{ID: "word.compare.dialog.title"}, i18n.Message{ID: "word.compare.dialog.body.choose_document", Args: i18n.Args{"Current": currentDocument}}, 520,
		func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordCompareDocumentChoices(gtx, otherDocuments, labels)
		},
		func(action string) {
			if action == "close" {
				if a.wordCompareRequest == request {
					a.wordCompareRequest++
				}
				a.modal.Hide()
				return
			}
			for _, info := range otherDocuments {
				if action == "compare:"+info.DocumentID {
					a.startWordDocumentCompare(state, info, request)
					return
				}
			}
		}, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
}

func (a *App) layoutWordCompareDocumentChoices(gtx layout.Context, documents []application.DocumentInfo, labels map[string]string) layout.Dimensions {
	a.wordCompareDocumentList.List.Axis = layout.Vertical
	maxHeight := gtx.Dp(unit.Dp(280))
	if gtx.Constraints.Max.Y < maxHeight {
		maxHeight = gtx.Constraints.Max.Y
	}
	gtx.Constraints.Max.Y = maxHeight
	return material.List(a.theme, &a.wordCompareDocumentList).Layout(gtx, len(documents), func(gtx layout.Context, index int) layout.Dimensions {
		info := documents[index]
		actionID := "compare:" + info.DocumentID
		button := a.wordCompareDocumentButtons[actionID]
		if button == nil {
			button = new(widget.Clickable)
			a.wordCompareDocumentButtons[actionID] = button
		}
		style := material.Button(a.theme, button, labels[actionID])
		if button.Clicked(gtx) && a.modal.OnAction != nil {
			a.modal.OnAction(actionID)
		}
		height := gtx.Dp(unit.Dp(48))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return style.Layout(gtx)
	})
}

func (a *App) wordCompareDocumentLabel(info application.DocumentInfo) string {
	name := filepath.Base(strings.TrimSpace(info.SourcePath))
	if name == "." || name == string(filepath.Separator) {
		name = ""
	}
	if name == "" {
		id := info.DocumentID
		if len(id) > 8 {
			id = id[:8]
		}
		name = a.text(i18n.Message{ID: "word.compare.document.untitled", Args: i18n.Args{"ID": id}})
	}
	if info.Dirty {
		name += a.text(i18n.Message{ID: "word.compare.document.unsaved"})
	}
	return name
}

type documentCompareArguments struct {
	LeftDocumentId        string `json:"left_document_id"`
	RightDocumentId       string `json:"right_document_id"`
	LeftExpectedRevision  uint64 `json:"left_expected_revision"`
	RightExpectedRevision uint64 `json:"right_expected_revision"`
	Start                 int    `json:"start"`
	Limit                 int    `json:"limit"`
}

func (a *App) startWordDocumentCompare(state *documentSessionState, other application.DocumentInfo, pickerRequest uint64) {
	if state == nil || state != a.documentSessionState || pickerRequest != a.wordCompareRequest {
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.modal.Hide()
		a.setStatusMessage(i18n.Message{ID: "word.status.operation_busy"}, feedbackWarning, notificationToast)
		return
	}
	a.wordCompareRequest++
	request := a.wordCompareRequest
	generation := a.documentGeneration
	window, shutdown, results := a.window, a.shutdown, a.wordCompareResults
	a.modal.ShowMessage(dialogIDWordCompare, i18n.Message{ID: "word.compare.dialog.title"}, i18n.Message{ID: "word.compare.dialog.body.running"}, func(string) {
		if a.wordCompareRequest == request {
			a.wordCompareRequest++
		}
		a.modal.Hide()
		if window != nil {
			window.Invalidate()
		}
	}, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
	a.launchManagedBackground(func() {
		command, err := a.executeRibbonApplicationCommand(token, "document.compare", "two_word_documents", state.documentID, "", documentCompareArguments{LeftDocumentId: state.documentID, RightDocumentId: other.DocumentID, LeftExpectedRevision: token.revision, RightExpectedRevision: other.Revision, Start: 0, Limit: 1000})
		state.finishOperation(token)
		result := wordCompareAsyncResult{
			state: state, generation: generation, request: request,
			leftDocumentID: state.documentID, rightDocumentID: other.DocumentID,
			leftRevision: token.revision, rightRevision: other.Revision,
			rightLabel: a.wordCompareDocumentLabel(other), data: command.Data, err: err,
		}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
}

type wordCompareAsyncResult struct {
	state           *documentSessionState
	generation      uint64
	request         uint64
	leftDocumentID  string
	rightDocumentID string
	leftRevision    uint64
	rightRevision   uint64
	rightLabel      string
	data            []byte
	err             error
}

func (a *App) applyWordCompareResult(result wordCompareAsyncResult) {
	if result.request != a.wordCompareRequest || !a.modal.Visible || a.modal.DialogID != dialogIDWordCompare {
		return
	}
	if result.state == nil || result.state.closed.Load() || result.state != a.documentSessionState || result.generation != a.documentGeneration {
		a.modal.SetMessageRef(i18n.Message{ID: "word.compare.result.stale_current_document"})
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.state.documentID != result.leftDocumentID || result.state.wordRevisionAtomic.Load() != result.leftRevision {
		a.modal.SetMessageRef(i18n.Message{ID: "word.compare.result.stale_document_revision"})
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.err != nil {
		a.modal.SetMessageRef(i18n.Message{ID: "word.compare.result.failed"})
		a.setStatusErrorMessage(i18n.Message{ID: "word.compare.result.failed"}, result.err, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	runtime := a.ensureDesktopRuntime()
	if runtime.applicationRuntime == nil {
		a.modal.SetMessageRef(i18n.Message{ID: "word.compare.result.runtime_closed"})
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	rightInfo, err := runtime.applicationRuntime.GetDocument(context.Background(), result.rightDocumentID)
	if err != nil || rightInfo.Revision != result.rightRevision {
		a.modal.SetMessageRef(i18n.Message{ID: "word.compare.result.stale_compared_document"})
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	message, err := wordDocumentCompareMessage(result.data, result.rightLabel)
	if err != nil {
		a.modal.SetMessageRef(i18n.Message{ID: "word.compare.error.result"})
		a.setStatusErrorMessage(i18n.Message{ID: "word.compare.error.result"}, err, notificationToast)
	} else {
		a.modal.Message = ""
		a.modal.MessageRef = message
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func wordDocumentCompareMessage(data []byte, rightLabel string) (i18n.Message, error) {
	var result struct {
		LeftRevision       uint64   `json:"left_revision"`
		RightRevision      uint64   `json:"right_revision"`
		ChangedPaths       []string `json:"changed_paths"`
		HasMoreDifferences bool     `json:"has_more_differences"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return i18n.Message{}, err
	}
	var paths strings.Builder
	limit := min(12, len(result.ChangedPaths))
	for i := 0; i < limit; i++ {
		if i > 0 {
			paths.WriteByte('\n')
		}
		paths.WriteString("• ")
		paths.WriteString(result.ChangedPaths[i])
	}
	return i18n.Message{ID: "word.compare.result.report", Args: i18n.Args{
		"RightDocument": rightLabel, "LeftRevision": result.LeftRevision, "RightRevision": result.RightRevision,
		"Changes": len(result.ChangedPaths), "Paths": paths.String(), "Remaining": len(result.ChangedPaths) - limit, "HasMore": result.HasMoreDifferences,
	}}, nil
}
