package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"
	"strings"

	"glance/internal/application"
	"glance/internal/document"
)

type presentationCommentAddArguments struct {
	SlideId uint32  `json:"slide_id"`
	XPt     float64 `json:"x_pt"`
	YPt     float64 `json:"y_pt"`
	Text    string  `json:"text"`
}

func (a *App) showPresentationNewCommentDialog(view *presentationViewState) {
	state := a.documentSessionState
	if view == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_application_command_runtime_is_unavail_cb81d9"}, feedbackWarning, notificationToast)
		return
	}
	if err := a.flushPresentationNotesDraft(view); err != nil {
		return
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return
	}
	slide := view.currentSlide()
	if slide == nil {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.select_a_slide_first_ddb108"}, feedbackWarning, notificationSuppressed)
		return
	}
	xPT, yPT := 24.0, 24.0
	for _, element := range slide.Elements {
		if element.ID == view.selectedShape {
			xPT = max(0.0, min(element.Bounds.X+element.Bounds.W, view.snapshot.WidthPT))
			yPT = max(0.0, min(element.Bounds.Y, view.snapshot.HeightPT))
			break
		}
	}
	expectedRevision := state.wordRevisionAtomic.Load()
	a.modal.ShowInputMessage(
		"presentation.comments.add",
		i18n.Message{ID: "presentation.comments.add.title"},
		i18n.Message{ID: "presentation.comments.add.body", Args: i18n.Args{"Slide": view.activeSlide + 1, "X": fmt.Sprintf("%.1f", xPT), "Y": fmt.Sprintf("%.1f", yPT)}},
		"",
		i18n.Message{ID: "presentation.comments.add.hint"},
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			text := strings.TrimSpace(a.modal.Input.Text())
			if text == "" {
				a.setStatusMessage(i18n.Message{ID: "word.comment.error.empty"}, feedbackError, notificationToast)
				return
			}
			a.startPresentationCommentMutation(view, "presentation.comment.add", presentationCommentAddArguments{SlideId: slide.ID, XPt: xPT, YPt: yPT, Text: text}, expectedRevision, "add", "")
		},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "presentation.comments.add.action"}},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

type presentationCommentsListArguments struct {
	SlideId uint32 `json:"slide_id"`
	Start   int    `json:"start"`
	Limit   int    `json:"limit"`
}

func (a *App) startPresentationDeleteComment(view *presentationViewState) bool {
	state := a.documentSessionState
	if view == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if view.commandPending {
		return true
	}
	slide := view.currentSlide()
	if slide == nil {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.select_a_slide_first_ddb108"}, feedbackWarning, notificationSuppressed)
		return true
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		out := presentationCommandResult{state: state, view: view, generation: generation, presentationCommentList: true}
		result, err := a.executeRibbonApplicationCommand(token, "presentation.comments.list", "presentation_document", token.documentID, "", presentationCommentsListArguments{SlideId: slide.ID, Start: 0, Limit: 100})
		if err != nil {
			out.err = err
		} else {
			out.revision = result.Revision
			var payload struct {
				Total    int `json:"total"`
				Comments []struct {
					ID          string  `json:"comment_id"`
					SlideID     uint32  `json:"slide_id"`
					SlideNumber int     `json:"slide_number"`
					Author      string  `json:"author"`
					Text        string  `json:"text"`
					XPT         float64 `json:"x_pt"`
					YPT         float64 `json:"y_pt"`
				} `json:"comments"`
			}
			if json.Unmarshal(result.Data, &payload) != nil || payload.Total < 0 || len(payload.Comments) > 100 {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.presentationCommentTotal = payload.Total
				for _, item := range payload.Comments {
					out.presentationComments = append(out.presentationComments, document.PresentationComment{
						ID: item.ID, SlideID: item.SlideID, SlideNumber: item.SlideNumber,
						Author: item.Author, Text: item.Text, XPT: item.XPT, YPT: item.YPT,
					})
				}
			}
		}
		state.finishOperation(token)
		select {
		case view.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.reading_comments_on_the_current_slide_534fab"}, feedbackInfo, notificationSuppressed)
	return true
}

type presentationCommentDeleteArguments struct {
	SlideId   uint32 `json:"slide_id"`
	CommentId string `json:"comment_id"`
}

func (a *App) showPresentationDeleteCommentPrompt(view *presentationViewState, comments []document.PresentationComment, total int, expectedRevision uint64) {
	var listing strings.Builder
	visible := min(len(comments), 30)
	for _, comment := range comments[:visible] {
		text := strings.Join(strings.Fields(comment.Text), " ")
		text = truncatePresentationCommentPreview(text, 96)
		fmt.Fprintf(&listing, "\n%s · %s：%s", comment.ID, comment.Author, text)
	}
	if total > visible {
		fmt.Fprintf(&listing, "\n当前列出 %d/%d 条；可通过 MCP presentation.comments.list 分页查询。", visible, total)
	}
	initial := ""
	if total == 1 && len(comments) == 1 {
		initial = comments[0].ID
	}
	a.modal.ShowInputMessage(
		"presentation.comments.delete",
		i18n.Message{ID: "presentation.comments.delete.title"},
		i18n.Message{ID: "presentation.comments.delete.body", Args: i18n.Args{"List": listing.String()}},
		initial,
		i18n.Message{ID: "presentation.comments.delete.hint"},
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			commentID := strings.TrimSpace(a.modal.Input.Text())
			var selected *document.PresentationComment
			for index := range comments {
				if comments[index].ID == commentID {
					selected = &comments[index]
					break
				}
			}
			if selected == nil {
				a.setStatusMessage(i18n.Message{ID: "word.comment.error.not_found"}, feedbackInfo, notificationToast)
				return
			}
			a.startPresentationCommentMutation(view, "presentation.comment.delete", presentationCommentDeleteArguments{SlideId: selected.SlideID, CommentId: selected.ID}, expectedRevision, "delete", selected.ID)
		},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.delete"}},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) startPresentationCommentMutation(view *presentationViewState, commandID string, arguments any, expectedRevision uint64, action, commentID string) {
	state := a.documentSessionState
	if view == nil || state == nil || state.pptxDoc == nil || state.formatRunning.Load() || a.isSaving() || !a.applicationSessionRegistered(state.documentID) {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.comments_are_unavailable_for_the_current_p_850c72"}, feedbackWarning, notificationToast)
		return
	}
	if view.commandPending {
		return
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return
	}
	if token.revision != expectedRevision {
		state.finishOperation(token)
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_document_changed_select_the_comment_op_6dd648"}, feedbackInfo, notificationToast)
		return
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, commandID, "presentation_document", token.documentID, "", arguments)
		out := presentationCommandResult{
			state: state, view: view, generation: generation, presentationCommentMutation: action,
			presentationCommentID: commentID, err: err,
		}
		if err == nil {
			out.revision = result.Revision
			var payload struct {
				Changed   bool   `json:"changed"`
				CommentID string `json:"comment_id"`
			}
			if json.Unmarshal(result.Data, &payload) != nil || payload.CommentID == "" || commentID != "" && payload.CommentID != commentID {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.presentationCommentID = payload.CommentID
				out.presentationCommentChanged = payload.Changed
			}
			if out.err == nil {
				a.ensureDesktopRuntime().publishSessionRevision(state.documentID, result.Revision)
			}
		}
		state.finishOperation(token)
		select {
		case view.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.updating_presentation_comments_3ff948"}, feedbackInfo, notificationSuppressed)
}

func truncatePresentationCommentPreview(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "…"
}
