package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/i18n"
)

func (a *App) handlePresentationAddSlide(v *presentationViewState) {
	if a.pptxDoc == nil {
		return
	}
	if err := a.flushPresentationNotesDraft(v); err != nil {
		return
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return
	}
	if a.startPresentationAddSlide(v) {
		return
	}
	if err := a.pptxDoc.AddSlide(); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
		return
	}
	v.snapshot = a.pptxDoc.Snapshot()
	v.activeSlide = v.snapshot.SlideCount - 1
	clearPresentationSelection(v)
	if v.activeRibbonTab == "picture" {
		v.activeRibbonTab = "home"
	}
	v.editingShape = 0
	v.shapeDraftText = ""
	if cur := v.currentSlide(); cur != nil {
		a.resetPresentationNotesEditor(v, cur)
	}
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.new_slide_created_050935"}, feedbackSuccess, notificationToast)
}

type presentationCommandResult struct {
	state                                *documentSessionState
	view                                 *presentationViewState
	generation                           uint64
	accessibility                        bool
	accessibilityText                    i18n.Message
	animationPane                        bool
	animationPaneText                    i18n.Message
	proofread                            bool
	proofreadText                        i18n.Message
	presentationCommentList              bool
	presentationCommentMutation          string
	presentationCommentChanged           bool
	presentationCommentID                string
	presentationComments                 []document.PresentationComment
	presentationCommentTotal             int
	revision                             uint64
	slideCount                           int
	deleted                              bool
	deletedSlideIndex                    int
	createdTextBox                       bool
	createdShape                         bool
	createdPicture                       bool
	createdTable                         bool
	createdChart                         bool
	createdAudio                         bool
	createdVideo                         bool
	createdSlideID                       uint32
	createdElementID                     int64
	clipboardCopy                        bool
	clipboardPaste                       bool
	clipboardText                        string
	clipboardElementID                   int64
	clipboardChanged                     bool
	transition                           bool
	transitionChanged                    bool
	transitionAffected                   int
	presentationAnimation                bool
	presentationAnimationEffect          string
	presentationAnimationChanged         bool
	presentationAnimationDuration        bool
	presentationAnimationDurationChanged bool
	presentationAnimationDurationMS      int
	background                           bool
	backgroundChanged                    bool
	presentationTheme                    bool
	presentationThemeOperation           string
	presentationThemeID                  string
	presentationThemeChanged             bool
	presentationGroup                    bool
	presentationGroupID                  uint32
	presentationGroupElementIDs          []uint32
	presentationGroupChanged             bool
	presentationLayoutList               bool
	presentationLayoutSet                bool
	presentationLayoutSlideID            uint32
	presentationLayoutOptions            document.PresentationLayoutSummary
	presentationLayoutOption             document.PresentationLayoutOption
	presentationLayoutChanged            bool
	slideSize                            bool
	slideSizeChanged                     bool
	slideWidthPT                         float64
	slideHeightPT                        float64
	elementAlignment                     bool
	alignmentAction                      string
	alignmentChanged                     bool
	err                                  error
}
type presentationSlideAddArguments struct {
}

func (a *App) startPresentationAddSlide(v *presentationViewState) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if v.commandPending {
		return true
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	v.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.slide.add", "presentation_document", token.documentID, "", presentationSlideAddArguments{})
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: v, generation: generation, err: err}
		if err == nil {
			var data struct {
				SlideCount int `json:"slide_count"`
			}
			if json.Unmarshal(result.Data, &data) != nil || data.SlideCount < 1 {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.slideCount = data.SlideCount
			}
		}
		select {
		case v.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

func (a *App) pollPresentationCommandResult(v *presentationViewState) {
	if v == nil {
		return
	}
	select {
	case result := <-v.commandResults:
		v.commandPending = false
		if result.state != a.documentSessionState || result.view != v || result.generation != a.documentGeneration {
			return
		}
		if result.err != nil {
			switch {
			case result.presentationTheme:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.presentationGroup:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.presentationLayoutList:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.presentationLayoutSet:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.animationPane:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.transition:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.presentationAnimation:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.presentationAnimationDuration:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.background:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.slideSize:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.elementAlignment:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.proofread:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.presentationCommentList:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.presentationCommentMutation != "":
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.clipboardCopy:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.clipboardPaste:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.accessibility:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.deleted:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.createdTextBox:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.createdShape:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.createdPicture:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.createdTable:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.createdChart:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.createdAudio:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			case result.createdVideo:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			default:
				a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, result.err, notificationToast)
			}
			return
		}
		if result.accessibility {
			a.modal.ShowMessage("presentation.accessibility", i18n.Message{ID: "presentation.accessibility.title"}, result.accessibilityText, func(action string) {
				a.modal.Hide()
			}, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.presentation_accessibility_check_completed_3e162c"}, feedbackSuccess, notificationToast)
			return
		}
		if result.state.pptxDoc == nil {
			switch {
			case result.presentationTheme:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_set_the_presentation_theme_becau_a66f97"}, feedbackError, notificationToast)
			case result.presentationGroup:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_group_presentation_objects_becau_e085fa"}, feedbackError, notificationToast)
			case result.presentationLayoutList:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_read_presentation_layouts_becaus_261b52"}, feedbackError, notificationToast)
			case result.presentationLayoutSet:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_set_the_slide_layout_because_the_4a9473"}, feedbackError, notificationToast)
			case result.animationPane:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_read_the_animation_pane_because_452159"}, feedbackError, notificationToast)
			case result.transition:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_set_the_transition_because_the_p_390882"}, feedbackError, notificationToast)
			case result.presentationAnimation:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_set_the_element_animation_becaus_8f4f1b"}, feedbackError, notificationToast)
			case result.presentationAnimationDuration:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_set_the_animation_duration_becau_f2ad63"}, feedbackError, notificationToast)
			case result.background:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_set_the_slide_background_because_0ca4f0"}, feedbackError, notificationToast)
			case result.slideSize:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_set_the_slide_size_because_the_p_21ddfa"}, feedbackError, notificationToast)
			case result.elementAlignment:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_arrange_objects_because_the_pres_947cad"}, feedbackError, notificationToast)
			case result.proofread:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.presentation_text_review_failed_because_th_a83624"}, feedbackError, notificationToast)
			case result.presentationCommentList:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_read_presentation_comments_becau_6fc2e8"}, feedbackError, notificationToast)
			case result.presentationCommentMutation != "":
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_update_presentation_comments_bec_90921d"}, feedbackError, notificationToast)
			case result.clipboardCopy:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_copy_text_because_the_presentati_8af4c9"}, feedbackError, notificationToast)
			case result.clipboardPaste:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_paste_text_because_the_presentat_66801e"}, feedbackError, notificationToast)
			case result.deleted:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_delete_the_slide_because_the_pre_f2d819"}, feedbackError, notificationToast)
			case result.createdTextBox:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_create_the_text_box_because_the_d67b9f"}, feedbackError, notificationToast)
			case result.createdShape:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_create_the_shape_because_the_pre_838280"}, feedbackError, notificationToast)
			case result.createdPicture:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_insert_the_picture_because_the_p_31edac"}, feedbackError, notificationToast)
			case result.createdTable:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_insert_the_table_because_the_pre_a0535a"}, feedbackError, notificationToast)
			case result.createdChart:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_insert_the_chart_because_the_pre_d13bf6"}, feedbackError, notificationToast)
			case result.createdAudio:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_insert_the_audio_because_the_pre_649aec"}, feedbackError, notificationToast)
			case result.createdVideo:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_insert_the_video_because_the_pre_96eeec"}, feedbackError, notificationToast)
			default:
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_create_a_slide_because_the_prese_2de729"}, feedbackError, notificationToast)
			}
			return
		}
		if result.animationPane {
			a.modal.ShowMessage("presentation.animation_pane", i18n.Message{ID: "presentation.animation_pane.title"}, result.animationPaneText, func(action string) {
				a.modal.Hide()
			}, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.read_animations_for_the_current_slide_8d45d2"}, feedbackSuccess, notificationToast)
			return
		}
		if result.proofread {
			a.modal.ShowMessage("presentation.proofread", i18n.Message{ID: "presentation.proofread.title"}, result.proofreadText, func(action string) {
				a.modal.Hide()
			}, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.presentation_text_review_completed_0428e3"}, feedbackSuccess, notificationToast)
			return
		}
		if result.presentationCommentList {
			if len(result.presentationComments) == 0 {
				a.modal.ShowMessage("presentation.comments.none", i18n.Message{ID: "presentation.comments.delete.title"}, i18n.Message{ID: "presentation.comments.none"}, func(action string) { a.modal.Hide() }, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_current_slide_has_no_comments_6b1fcc"}, feedbackInfo, notificationSuppressed)
				return
			}
			a.showPresentationDeleteCommentPrompt(v, result.presentationComments, result.presentationCommentTotal, result.revision)
			return
		}
		if result.presentationCommentMutation != "" {
			v.snapshot = result.state.pptxDoc.Snapshot()
			a.persistActiveSession()
			if result.presentationCommentMutation == "add" {
				if result.presentationCommentChanged {
					a.setStatusMessage(i18n.Message{ID: "presentation.comment.status.added", Args: i18n.Args{"ID": result.presentationCommentID}}, feedbackSuccess, notificationToast)
				} else {
					a.setStatusMessage(i18n.Message{ID: "presentation.comment.status.exists", Args: i18n.Args{"ID": result.presentationCommentID}}, feedbackInfo, notificationSuppressed)
				}
			} else {
				if result.presentationCommentChanged {
					a.setStatusMessage(i18n.Message{ID: "presentation.comment.status.deleted", Args: i18n.Args{"ID": result.presentationCommentID}}, feedbackSuccess, notificationToast)
				} else {
					a.setStatusMessage(i18n.Message{ID: "presentation.comment.status.unchanged", Args: i18n.Args{"ID": result.presentationCommentID}}, feedbackInfo, notificationSuppressed)
				}
			}
			return
		}
		if result.presentationLayoutList {
			a.showPresentationLayoutChoices(v, result.presentationLayoutSlideID, result.presentationLayoutOptions)
			return
		}
		if result.clipboardCopy {
			v.clipboardWriteText = result.clipboardText
			v.clipboardWritePending = true
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.copied_the_selected_element_text_329f7f"}, feedbackSuccess, notificationToast)
			return
		}
		if result.clipboardPaste {
			v.snapshot = result.state.pptxDoc.Snapshot()
			if v.editingShape == result.clipboardElementID {
				v.shapeEditor.SetText(result.clipboardText)
				v.shapeDraftText = result.clipboardText
				result.state.setPresentationDraftDirty(a.windowID, presentationTextDraft, false)
			}
			a.persistActiveSession()
			if result.clipboardChanged {
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.pasted_into_the_originally_selected_text_e_3336e4"}, feedbackSuccess, notificationToast)
			} else {
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_text_is_unchanged_nothing_was_pasted_5dd9a0"}, feedbackInfo, notificationToast)
			}
			return
		}
		if result.presentationTheme {
			v.snapshot = result.state.pptxDoc.Snapshot()
			a.persistActiveSession()
			if result.presentationThemeChanged {
				if result.presentationThemeOperation == "palette" {
					a.setStatusMessage(i18n.Message{ID: "presentation.theme.status.palette_applied", Args: i18n.Args{"Theme": result.presentationThemeID}}, feedbackSuccess, notificationToast)
				} else {
					a.setStatusMessage(i18n.Message{ID: "presentation.theme.status.applied", Args: i18n.Args{"Theme": result.presentationThemeID}}, feedbackSuccess, notificationToast)
				}
			} else {
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_theme_is_already_applied_3ff948"}, feedbackInfo, notificationSuppressed)
			}
			return
		}
		if result.presentationGroup {
			v.snapshot = result.state.pptxDoc.Snapshot()
			a.persistActiveSession()
			if result.presentationGroupChanged {
				a.setStatusMessage(i18n.Message{ID: "presentation.group.status.created", Args: i18n.Args{"Count": len(result.presentationGroupElementIDs), "ID": result.presentationGroupID}}, feedbackSuccess, notificationToast)
			} else {
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_objects_are_already_grouped_41c6d0"}, feedbackInfo, notificationToast)
			}
			return
		}
		v.snapshot = result.state.pptxDoc.Snapshot()
		if result.presentationLayoutSet {
			a.persistActiveSession()
			if result.presentationLayoutChanged {
				a.setStatusMessage(i18n.Message{ID: "presentation.layout.status.applied", Args: i18n.Args{"Slide": result.presentationLayoutSlideID, "Layout": result.presentationLayoutOption.Name}}, feedbackSuccess, notificationToast)
			} else {
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_slide_already_uses_this_layout_056d34"}, feedbackInfo, notificationSuppressed)
			}
			return
		}
		if result.background {
			a.persistActiveSession()
			if result.backgroundChanged {
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.slide_background_color_updated_e7288b"}, feedbackSuccess, notificationToast)
			} else {
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_slide_background_is_already_set_5ed59e"}, feedbackInfo, notificationSuppressed)
			}
			return
		}
		if result.slideSize {
			a.persistActiveSession()
			if result.slideSizeChanged {
				a.setStatusMessage(i18n.Message{ID: "presentation.slide_size.status.applied", Args: i18n.Args{"Width": fmt.Sprintf("%.2f", result.slideWidthPT), "Height": fmt.Sprintf("%.2f", result.slideHeightPT)}}, feedbackSuccess, notificationToast)
			} else {
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_slide_size_is_already_set_4e741a"}, feedbackInfo, notificationSuppressed)
			}
			return
		}
		if result.elementAlignment {
			a.persistActiveSession()
			if result.alignmentChanged {
				alignment := a.presentationAlignmentLabel(result.alignmentAction)
				a.setStatusMessage(i18n.Message{ID: "presentation.alignment.status.applied", Args: i18n.Args{"Alignment": alignment}}, feedbackSuccess, notificationToast)
			} else {
				alignment := a.presentationAlignmentLabel(result.alignmentAction)
				a.setStatusMessage(i18n.Message{ID: "presentation.alignment.status.unchanged", Args: i18n.Args{"Alignment": alignment}}, feedbackInfo, notificationSuppressed)
			}
			return
		}
		if result.transition {
			a.persistActiveSession()
			if result.transitionChanged {
				a.setStatusMessage(i18n.Message{ID: "presentation.transition.status.updated", Args: i18n.Args{"Count": result.transitionAffected}}, feedbackSuccess, notificationToast)
			} else {
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_transition_is_already_applied_48c2bb"}, feedbackInfo, notificationSuppressed)
			}
			return
		}
		if result.presentationAnimation {
			a.persistActiveSession()
			if result.presentationAnimationChanged {
				a.setStatusMessage(i18n.Message{ID: "presentation.animation.status.added", Args: i18n.Args{"Effect": result.presentationAnimationEffect}}, feedbackSuccess, notificationToast)
			} else {
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_animation_is_already_applied_734906"}, feedbackInfo, notificationSuppressed)
			}
			return
		}
		if result.presentationAnimationDuration {
			a.persistActiveSession()
			if result.presentationAnimationDurationChanged {
				a.setStatusMessage(i18n.Message{ID: "presentation.animation.status.duration", Args: i18n.Args{"Duration": result.presentationAnimationDurationMS}}, feedbackSuccess, notificationToast)
			} else {
				a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_animation_duration_is_already_set_136d40"}, feedbackInfo, notificationSuppressed)
			}
			return
		}
		if result.createdTextBox || result.createdShape || result.createdPicture || result.createdTable || result.createdChart || result.createdAudio || result.createdVideo {
			foundSlide := false
			for index, slide := range v.snapshot.Slides {
				if slide.ID == result.createdSlideID {
					v.activeSlide = index
					foundSlide = true
					break
				}
			}
			if !foundSlide {
				if result.createdShape {
					a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_create_the_shape_because_the_ret_137678"}, feedbackError, notificationToast)
				} else if result.createdPicture {
					a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_insert_the_picture_because_the_r_c68c3e"}, feedbackError, notificationToast)
				} else if result.createdTable {
					a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_insert_the_table_because_the_ret_8e9f79"}, feedbackError, notificationToast)
				} else if result.createdChart {
					a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_insert_the_chart_because_the_ret_f87bad"}, feedbackError, notificationToast)
				} else if result.createdAudio {
					a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_insert_the_audio_because_the_ret_aaf493"}, feedbackError, notificationToast)
				} else if result.createdVideo {
					a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_insert_the_video_because_the_ret_764a60"}, feedbackError, notificationToast)
				} else {
					a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_create_the_text_box_because_the_08c2be"}, feedbackError, notificationToast)
				}
				return
			}
		} else if result.deleted {
			v.activeSlide = min(result.deletedSlideIndex, v.snapshot.SlideCount-1)
		} else {
			v.activeSlide = min(result.slideCount-1, v.snapshot.SlideCount-1)
		}
		clearPresentationSelection(v)
		if result.createdTextBox || result.createdShape || result.createdPicture || result.createdTable || result.createdChart {
			setPresentationSingleSelection(v, result.createdElementID)
		}
		if v.activeRibbonTab == "picture" {
			v.activeRibbonTab = "home"
		}
		v.editingShape = 0
		v.shapeDraftText = ""
		if cur := v.currentSlide(); cur != nil {
			a.resetPresentationNotesEditor(v, cur)
		}
		a.persistActiveSession()
		switch {
		case result.deleted:
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.slide_deleted_5c31c3"}, feedbackSuccess, notificationToast)
		case result.createdTextBox:
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.text_box_created_double_click_it_to_edit_i_a9171f"}, feedbackSuccess, notificationToast)
		case result.createdShape:
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.rectangle_created_you_can_adjust_its_size_8cabe7"}, feedbackSuccess, notificationToast)
		case result.createdPicture:
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.picture_inserted_you_can_adjust_its_size_a_0c87cc"}, feedbackSuccess, notificationToast)
		case result.createdTable:
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.table_inserted_the_agent_can_fill_in_and_o_7ef07d"}, feedbackSuccess, notificationToast)
		case result.createdChart:
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.chart_template_inserted_the_agent_can_crea_9c9e0a"}, feedbackSuccess, notificationToast)
		case result.createdAudio:
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.audio_embedded_fa4d22"}, feedbackSuccess, notificationToast)
		case result.createdVideo:
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.video_embedded_85bb99"}, feedbackSuccess, notificationToast)
		default:
			a.setStatusMessage(i18n.Message{ID: "presentation.slide.status.created", Args: i18n.Args{"Number": v.activeSlide + 1}}, feedbackSuccess, notificationToast)
		}
	default:
	}
}

type documentInspectArguments struct {
	Start    int `json:"start"`
	Limit    int `json:"limit"`
	MaxBytes int `json:"max_bytes"`
}

func (a *App) startPresentationAccessibilityReview(v *presentationViewState) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if v.commandPending {
		return true
	}
	if err := a.flushPresentationNotesDraft(v); err != nil {
		return true
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return true
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	v.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		const pageSize, maxSlides = 100, 1000
		out := presentationCommandResult{state: state, view: v, generation: generation, accessibility: true}
		var report presentationAccessibilityReviewResult
		for start := 0; start < maxSlides; {
			result, err := a.executeRibbonApplicationCommand(token, "document.inspect", "presentation_document", token.documentID, "", documentInspectArguments{Start: start, Limit: pageSize, MaxBytes: application.MaxQueryResultBytes})
			if err != nil {
				out.err = err
				break
			}
			var envelope struct {
				Content struct {
					SlideCount    int               `json:"slide_count"`
					Start         int               `json:"start"`
					Slides        []json.RawMessage `json:"slides"`
					Accessibility struct {
						CheckedSlides int  `json:"checked_slides"`
						TotalSlides   int  `json:"total_slides"`
						Complete      bool `json:"complete"`
						Issues        []struct {
							Code        string `json:"code"`
							Severity    string `json:"severity"`
							Message     string `json:"message"`
							SlideNumber int    `json:"slide_number"`
							ElementID   int64  `json:"element_id"`
						} `json:"issues"`
					} `json:"accessibility"`
				} `json:"content"`
				Diagnostics []application.Diagnostic `json:"diagnostics"`
			}
			if err := json.Unmarshal(result.Data, &envelope); err != nil || envelope.Content.SlideCount < 1 || envelope.Content.Accessibility.TotalSlides != envelope.Content.SlideCount {
				out.err = application.ErrInvalidCommandResult
				break
			}
			report.TotalSlides = envelope.Content.SlideCount
			report.CheckedSlides += envelope.Content.Accessibility.CheckedSlides
			report.Issues = append(report.Issues, envelope.Content.Accessibility.Issues...)
			for _, diagnostic := range envelope.Diagnostics {
				if diagnostic.Code == "elements_truncated" {
					report.Complete = false
					report.ElementLimitReached = true
				}
			}
			start = envelope.Content.Start + len(envelope.Content.Slides)
			if start >= report.TotalSlides {
				report.Complete = !report.ElementLimitReached
				break
			}
			if len(envelope.Content.Slides) == 0 {
				out.err = application.ErrInvalidCommandResult
				break
			}
		}
		if out.err == nil {
			out.accessibilityText = formatPresentationAccessibilityReview(report)
		}
		state.finishOperation(token)
		select {
		case v.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

type presentationAccessibilityReviewResult struct {
	CheckedSlides       int
	TotalSlides         int
	Complete            bool
	ElementLimitReached bool
	Issues              []struct {
		Code        string `json:"code"`
		Severity    string `json:"severity"`
		Message     string `json:"message"`
		SlideNumber int    `json:"slide_number"`
		ElementID   int64  `json:"element_id"`
	}
}

func formatPresentationAccessibilityReview(report presentationAccessibilityReviewResult) i18n.Message {
	issues := report.Issues
	if len(issues) > 30 {
		issues = issues[:30]
	}
	return i18n.Message{ID: "presentation.accessibility.report", Args: i18n.Args{
		"CheckedSlides": report.CheckedSlides, "TotalSlides": report.TotalSlides, "Complete": report.Complete,
		"ElementLimitReached": report.ElementLimitReached, "IssueCount": len(report.Issues),
		"HiddenIssues": max(0, len(report.Issues)-len(issues)), "Issues": issues,
	}}
}

func (a *App) handlePresentationDeleteSlide(v *presentationViewState) {
	if a.pptxDoc == nil {
		return
	}
	if v.snapshot.SlideCount <= 1 {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_last_slide_in_a_presentation_cannot_be_aa2057"}, feedbackWarning, notificationToast)
		return
	}
	if err := a.flushPresentationNotesDraft(v); err != nil {
		return
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return
	}
	if a.startPresentationDeleteSlide(v) {
		return
	}
	current := v.currentSlide()
	if current == nil {
		return
	}
	deletedIndex := v.activeSlide
	if err := a.pptxDoc.DeleteSlideByID(current.ID); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
		return
	}
	v.snapshot = a.pptxDoc.Snapshot()
	v.activeSlide = min(deletedIndex, v.snapshot.SlideCount-1)
	clearPresentationSelection(v)
	if v.activeRibbonTab == "picture" {
		v.activeRibbonTab = "home"
	}
	v.editingShape = 0
	v.shapeDraftText = ""
	if cur := v.currentSlide(); cur != nil {
		a.resetPresentationNotesEditor(v, cur)
	}
	a.persistActiveSession()
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.slide_deleted_5c31c3"}, feedbackSuccess, notificationToast)
}

type presentationSlideDeleteArguments struct {
	SlideId uint32 `json:"slide_id"`
}

func (a *App) startPresentationDeleteSlide(v *presentationViewState) bool {
	if v == nil {
		return false
	}
	state := a.documentSessionState
	current := v.currentSlide()
	if current == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if v.commandPending {
		return true
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	deletedSlideID := current.ID
	deletedSlideIndex := v.activeSlide
	v.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.slide.delete", "presentation_slide", fmt.Sprint(deletedSlideID), "", presentationSlideDeleteArguments{SlideId: deletedSlideID})
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: v, generation: generation, deleted: true, deletedSlideIndex: deletedSlideIndex, err: err}
		if err == nil {
			var data struct {
				DeletedSlideID uint32 `json:"deleted_slide_id"`
				SlideCount     int    `json:"slide_count"`
			}
			if json.Unmarshal(result.Data, &data) != nil || data.DeletedSlideID != deletedSlideID || data.SlideCount < 1 {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.slideCount = data.SlideCount
			}
		}
		select {
		case v.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

func (a *App) handlePresentationAddTextBox(v *presentationViewState) {
	if a.pptxDoc == nil {
		return
	}
	if err := a.flushPresentationNotesDraft(v); err != nil {
		return
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return
	}
	v.insertShape = false
	v.insertPicturePending = false
	v.insertPictureData = nil
	v.insertPictureMediaType = ""
	v.insertPictureWidth, v.insertPictureHeight = 0, 0
	v.insertTextBox = true
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.click_a_position_on_the_slide_to_place_the_5ddd6a"}, feedbackWarning, notificationSuppressed)
}

func (a *App) addPresentationTextBoxAt(v *presentationViewState, slideID uint32, bounds document.PresentationRect, text string) {
	if a.startPresentationCreateTextBox(v, slideID, bounds, text) {
		return
	}
	if a.pptxDoc == nil {
		return
	}
	elementID, err := a.pptxDoc.AddTextBoxByID(slideID, bounds, text)
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
		return
	}
	v.snapshot = a.pptxDoc.Snapshot()
	for index, slide := range v.snapshot.Slides {
		if slide.ID == slideID {
			v.activeSlide = index
			break
		}
	}
	setPresentationSingleSelection(v, elementID)
	if cur := v.currentSlide(); cur != nil {
		a.resetPresentationNotesEditor(v, cur)
	}
	a.persistActiveSession()
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.text_box_created_double_click_it_to_edit_i_a9171f"}, feedbackSuccess, notificationToast)
}

type startPresentationCreateTextBoxPayload struct {
	SlideID uint32  `json:"slide_id"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Width   float64 `json:"width"`
	Height  float64 `json:"height"`
	Text    string  `json:"text"`
}

func (a *App) startPresentationCreateTextBox(v *presentationViewState, slideID uint32, bounds document.PresentationRect, text string) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if v.commandPending {
		return true
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	v.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		arguments := startPresentationCreateTextBoxPayload{SlideID: slideID, X: bounds.X, Y: bounds.Y, Width: bounds.W, Height: bounds.H, Text: text}
		result, err := a.executeRibbonApplicationCommand(token, "presentation.element.textbox.create", "presentation_slide", fmt.Sprint(slideID), "", arguments)
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: v, generation: generation, createdTextBox: true, createdSlideID: slideID, err: err}
		if err == nil {
			var data struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
			}
			if json.Unmarshal(result.Data, &data) != nil || data.SlideID != slideID || data.ElementID <= 0 {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.createdElementID = data.ElementID
			}
		}
		select {
		case v.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

func (a *App) handlePresentationAddShape(v *presentationViewState) {
	if a.pptxDoc == nil {
		return
	}
	if err := a.flushPresentationNotesDraft(v); err != nil {
		return
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return
	}
	v.insertTextBox = false
	v.insertPicturePending = false
	v.insertPictureData = nil
	v.insertPictureMediaType = ""
	v.insertPictureWidth, v.insertPictureHeight = 0, 0
	v.insertShape = true
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.click_a_position_on_the_slide_to_place_the_6760ff"}, feedbackWarning, notificationSuppressed)
}

func (a *App) addPresentationShapeAt(v *presentationViewState, slideID uint32, bounds document.PresentationRect, geometry, text string) {
	if a.startPresentationCreateShape(v, slideID, bounds, geometry, text) {
		return
	}
	if a.pptxDoc == nil || v == nil {
		return
	}
	elementID, err := a.pptxDoc.AddAutoShapeByID(slideID, bounds, geometry, text)
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
		return
	}
	v.snapshot = a.pptxDoc.Snapshot()
	for index, slide := range v.snapshot.Slides {
		if slide.ID == slideID {
			v.activeSlide = index
			break
		}
	}
	setPresentationSingleSelection(v, elementID)
	if cur := v.currentSlide(); cur != nil {
		a.resetPresentationNotesEditor(v, cur)
	}
	a.persistActiveSession()
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.rectangle_created_you_can_adjust_its_size_8cabe7"}, feedbackSuccess, notificationToast)
}

type startPresentationCreateShapePayload struct {
	SlideID  uint32  `json:"slide_id"`
	Geometry string  `json:"geometry"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
	Text     string  `json:"text"`
}

func (a *App) startPresentationCreateShape(v *presentationViewState, slideID uint32, bounds document.PresentationRect, geometry, text string) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if v.commandPending {
		return true
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	v.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		arguments := startPresentationCreateShapePayload{SlideID: slideID, Geometry: geometry, X: bounds.X, Y: bounds.Y, Width: bounds.W, Height: bounds.H, Text: text}
		result, err := a.executeRibbonApplicationCommand(token, "presentation.element.shape.create", "presentation_slide", fmt.Sprint(slideID), "", arguments)
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: v, generation: generation, createdShape: true, createdSlideID: slideID, err: err}
		if err == nil {
			var data struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				Geometry  string `json:"geometry"`
			}
			if json.Unmarshal(result.Data, &data) != nil || data.SlideID != slideID || data.ElementID <= 0 || data.Geometry != geometry {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.createdElementID = data.ElementID
			}
		}
		select {
		case v.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

func (a *App) addPresentationPictureAt(v *presentationViewState, slideID uint32, bounds document.PresentationRect, fit, mediaType string, data []byte) {
	if a.startPresentationCreatePicture(v, slideID, bounds, fit, mediaType, data) {
		return
	}
	if a.pptxDoc == nil || v == nil {
		return
	}
	elementID, _, err := a.pptxDoc.AddSlidePictureByID(slideID, bounds, mediaType, data, fit, "", false)
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
		return
	}
	v.snapshot = a.pptxDoc.Snapshot()
	for index, slide := range v.snapshot.Slides {
		if slide.ID == slideID {
			v.activeSlide = index
			break
		}
	}
	setPresentationSingleSelection(v, elementID)
	if cur := v.currentSlide(); cur != nil {
		a.resetPresentationNotesEditor(v, cur)
	}
	a.persistActiveSession()
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.picture_inserted_you_can_adjust_its_size_a_0c87cc"}, feedbackSuccess, notificationToast)
}

type startPresentationCreatePicturePayload struct {
	SlideID     uint32  `json:"slide_id"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
	Fit         string  `json:"fit"`
	MediaType   string  `json:"media_type"`
	ImageBase64 []byte  `json:"image_base64"`
}

func (a *App) startPresentationCreatePicture(v *presentationViewState, slideID uint32, bounds document.PresentationRect, fit, mediaType string, data []byte) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if v.commandPending {
		return true
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	v.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		arguments := startPresentationCreatePicturePayload{SlideID: slideID, X: bounds.X, Y: bounds.Y, Width: bounds.W, Height: bounds.H, Fit: fit, MediaType: mediaType, ImageBase64: data}
		result, err := a.executeRibbonApplicationCommand(token, "presentation.element.picture.create", "presentation_slide", fmt.Sprint(slideID), "", arguments)
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: v, generation: generation, createdPicture: true, createdSlideID: slideID, err: err}
		if err == nil {
			var payload struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				MediaType string `json:"media_type"`
			}
			if json.Unmarshal(result.Data, &payload) != nil || payload.SlideID != slideID || payload.ElementID <= 0 || payload.MediaType != mediaType {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.createdElementID = payload.ElementID
			}
		}
		select {
		case v.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

func (a *App) presentationAlignmentLabel(actionID string) string {
	return a.text(i18n.Message{ID: "presentation.alignment." + actionID})
}
