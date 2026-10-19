package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/i18n"
)

type wordCommentAddArguments struct {
	RunId       string `json:"run_id"`
	StartRunId  string `json:"start_run_id,omitempty"`
	StartOffset *int   `json:"start_offset,omitempty"`
	EndRunId    string `json:"end_run_id,omitempty"`
	EndOffset   *int   `json:"end_offset,omitempty"`
	Text        string `json:"text"`
}

func (a *App) showWordNewCommentDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	args, selectedText, hasSelection, ok := wordCommentTargetArguments(state)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.comment.error.select_editable_text"}, feedbackWarning, notificationToast)
		return
	}
	expectedRevision := state.wordRevisionAtomic.Load()
	if _, ok := any(state.word).(document.WordCommentEditor); !ok {
		a.setStatusMessage(i18n.Message{ID: "word.comment.error.unsupported"}, feedbackWarning, notificationToast)
		return
	}

	body := i18n.Message{ID: "word.comment.add.body"}
	if hasSelection {
		body = i18n.Message{ID: "word.comment.add.body.selection", Args: i18n.Args{"Selection": wordCommentSelectionPreview(selectedText)}}
	}
	a.modal.ShowInputMessage(
		"word.comment.add",
		i18n.Message{ID: "word.comment.add.title"}, body,
		"",
		i18n.Message{ID: "word.comment.add.hint"},
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			commentText := strings.TrimSpace(a.modal.Input.Text())
			if commentText == "" {
				a.setStatusMessage(i18n.Message{ID: "word.comment.error.empty"}, feedbackError, notificationToast)
				return
			}
			args.Text = commentText
			if !a.runWordApplicationCommandExpectedRevision("添加批注", "word.comment.add", args, expectedRevision) {
				a.setStatusMessage(i18n.Message{ID: "word.comment.error.stale_add"}, feedbackWarning, notificationToast)
			}
		},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.comment.action.add"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func wordCommentTargetArguments(state *documentSessionState) (wordCommentAddArguments, string, bool, bool) {
	if state == nil {
		return wordCommentAddArguments{}, "", false, false
	}
	blocks := wordBlocksForState(state)
	if start, end, active := wordPageOrderedSelection(state.wordPageTextSelection); active {
		selectedText, hasText := wordPageSelectedText(state)
		startRun, startOK := selectedWordRun(blocks, start.runID)
		endRun, endOK := selectedWordRun(blocks, end.runID)
		_, startParagraph, startParagraphOK := findEditableWordRunInState(state, start.runID)
		_, endParagraph, endParagraphOK := findEditableWordRunInState(state, end.runID)
		if hasText && startOK && endOK && startParagraphOK && endParagraphOK && startParagraph.ID == endParagraph.ID && startRun.Editable && endRun.Editable && wordBodyRunEditable(blocks, start.runID) && wordBodyRunEditable(blocks, end.runID) {
			startOffset, endOffset := start.runeOffset, end.runeOffset
			return wordCommentAddArguments{
				RunId: start.runID, StartRunId: start.runID, StartOffset: &startOffset,
				EndRunId: end.runID, EndOffset: &endOffset,
			}, selectedText, true, true
		}
		return wordCommentAddArguments{}, "", false, false
	}
	if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
		start, end := editor.Selection()
		if end < start {
			start, end = end, start
		}
		if end > start {
			run, exists := selectedWordRun(blocks, state.selectedWordRunID)
			if !exists || !run.Editable || !wordBodyRunEditable(blocks, run.ID) || start < 0 || end > len([]rune(run.Text)) {
				return wordCommentAddArguments{}, "", false, false
			}
			selectedText := string([]rune(run.Text)[start:end])
			return wordCommentAddArguments{
				RunId: run.ID, StartRunId: run.ID, StartOffset: &start,
				EndRunId: run.ID, EndOffset: &end,
			}, selectedText, true, selectedText != ""
		}
	}
	run, hasRun := selectedWordRun(blocks, state.selectedWordRunID)
	if !hasRun || !run.Editable || !wordBodyRunEditable(blocks, run.ID) {
		return wordCommentAddArguments{}, "", false, false
	}
	return wordCommentAddArguments{RunId: run.ID}, "", false, true
}

func wordCommentSelectionPreview(selectedText string) string {
	selectedText = strings.Join(strings.Fields(selectedText), " ")
	if len([]rune(selectedText)) > 80 {
		selectedText = string([]rune(selectedText)[:77]) + "…"
	}
	return selectedText
}

type wordCommentsListArguments struct {
	Start int `json:"start"`
	Limit int `json:"limit"`
}

func (a *App) showWordDeleteCommentDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if !a.applicationSessionRegistered(state.documentID) {
		a.setStatusMessage(i18n.Message{ID: "word.comment.error.runtime_unavailable"}, feedbackWarning, notificationToast)
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.operation_busy"}, feedbackWarning, notificationToast)
		return
	}
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommandWithContext(context.Background(), token, "word.comments.list", "word_document", token.documentID, "", wordCommentsListArguments{Start: 0, Limit: 100})
		comments := make([]document.WordComment, 0)
		total := 0
		if err == nil {
			var payload struct {
				Total    int `json:"total"`
				Comments []struct {
					ID     string `json:"id"`
					Author string `json:"author"`
					Text   string `json:"text"`
					RunID  string `json:"run_id"`
				} `json:"comments"`
			}
			if json.Unmarshal(result.Data, &payload) != nil {
				err = application.ErrInvalidCommandResult
			} else {
				total = payload.Total
				for _, item := range payload.Comments {
					comments = append(comments, document.WordComment{ID: item.ID, Author: item.Author, Text: item.Text, RunID: item.RunID})
				}
			}
		}
		revision := uint64(0)
		if err == nil {
			revision = result.Revision
		}
		a.publishWordCommand(wordCommandResult{
			state: state, operationToken: token, revision: revision, generation: generation,
			operation: "读取 Word 批注", applicationCommand: true, wordCommentList: true,
			wordComments: comments, wordCommentTotal: total, err: err,
		})
	})
	a.setStatusMessage(i18n.Message{ID: "word.comment.status.loading"}, feedbackInfo, notificationSuppressed)
}

type wordCommentDeleteArguments struct {
	CommentId string `json:"comment_id"`
}

func (a *App) showWordDeleteCommentPrompt(comments []document.WordComment, total int, expectedRevision uint64) {
	var listing strings.Builder
	visible := len(comments)
	if visible > 30 {
		visible = 30
	}
	for _, comment := range comments[:visible] {
		text := strings.Join(strings.Fields(comment.Text), " ")
		text = truncateWordCommentPreview(text, 96)
		fmt.Fprintf(&listing, "\n%s · %s: %s", comment.ID, comment.Author, text)
	}
	if total > visible {
		listing.WriteString("\n")
		listing.WriteString(a.text(i18n.Message{ID: "word.comment.delete.more", Args: i18n.Args{"Visible": visible, "Total": total}}))
	}
	initial := ""
	if total == 1 && len(comments) == 1 {
		initial = comments[0].ID
	}
	a.modal.ShowInputMessage(
		"word.comment.delete",
		i18n.Message{ID: "word.comment.delete.title"},
		i18n.Message{ID: "word.comment.delete.body", Args: i18n.Args{"CommentList": listing.String()}},
		initial,
		i18n.Message{ID: "word.comment.delete.hint"},
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			commentID := strings.TrimSpace(a.modal.Input.Text())
			found := false
			for _, comment := range comments {
				if comment.ID == commentID {
					found = true
					break
				}
			}
			if !found {
				a.setStatusMessage(i18n.Message{ID: "word.comment.error.not_found"}, feedbackWarning, notificationToast)
				return
			}
			if !a.runWordApplicationCommandExpectedRevision("删除批注", "word.comment.delete", wordCommentDeleteArguments{CommentId: commentID}, expectedRevision) {
				a.setStatusMessage(i18n.Message{ID: "word.comment.error.stale_delete"}, feedbackWarning, notificationToast)
			}
		},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.delete"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func truncateWordCommentPreview(value string, maxRunes int) string {
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, "�")
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "…"
}
