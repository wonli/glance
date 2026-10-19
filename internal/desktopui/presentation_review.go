package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"

	"glance/internal/application"
)

const (
	presentationReviewPageSize    = 100
	presentationReviewMaxSlides   = 1000
	presentationReviewMaxFindings = 500
)

type presentationProofreadData struct {
	Start             int  `json:"start"`
	ScannedSlides     int  `json:"scanned_slides"`
	TotalSlides       int  `json:"total_slides"`
	Complete          bool `json:"complete"`
	ElementsTruncated bool `json:"elements_truncated"`
	HasMore           bool `json:"has_more"`
	FindingsTruncated bool `json:"findings_truncated"`
	Findings          []struct {
		Code        string `json:"code"`
		Message     string `json:"message"`
		SlideID     uint32 `json:"slide_id"`
		SlideNumber int    `json:"slide_number"`
		ElementID   int64  `json:"element_id"`
	} `json:"findings"`
}

type presentationAnimationPaneData struct {
	SlideID    uint32 `json:"slide_id"`
	Revision   uint64 `json:"revision"`
	HasTiming  bool   `json:"has_timing"`
	Truncated  bool   `json:"truncated"`
	Animations []struct {
		Index           int    `json:"index"`
		Kind            string `json:"kind"`
		TargetElementID uint32 `json:"target_element_id"`
		PresetID        string `json:"preset_id"`
		PresetClass     string `json:"preset_class"`
		Transition      string `json:"transition"`
		Effect          string `json:"effect"`
		Duration        string `json:"duration"`
	} `json:"animations"`
	Diagnostics []string `json:"diagnostics"`
}

func (a *App) startPresentationAnimationPane(view *presentationViewState) bool {
	state := a.documentSessionState
	if view == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if view.commandPending {
		return true
	}
	if view.activeSlide < 0 || view.activeSlide >= len(view.snapshot.Slides) {
		return false
	}
	slideID := view.snapshot.Slides[view.activeSlide].ID
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		out := presentationCommandResult{state: state, view: view, generation: generation, animationPane: true}
		command, err := a.executeRibbonApplicationCommand(token, "presentation.animations.list", "presentation_document", token.documentID, "", map[string]uint32{
			"slide_id": slideID,
		})
		if err != nil {
			out.err = err
		} else {
			var data presentationAnimationPaneData
			if err := json.Unmarshal(command.Data, &data); err != nil || data.SlideID != slideID || len(data.Animations) > 500 || len(data.Diagnostics) > 8 {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.animationPaneText = formatPresentationAnimationPane(data)
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
	return true
}

func formatPresentationAnimationPane(data presentationAnimationPaneData) i18n.Message {
	animations := data.Animations
	if len(animations) > 100 {
		animations = animations[:100]
	}
	return i18n.Message{ID: "presentation.animation_pane.report", Args: i18n.Args{
		"SlideID": data.SlideID, "Revision": data.Revision, "HasTiming": data.HasTiming,
		"AnimationCount": len(data.Animations), "Animations": animations, "Truncated": data.Truncated,
		"Diagnostics": data.Diagnostics,
	}}
}

func (a *App) startPresentationProofread(view *presentationViewState) bool {
	state := a.documentSessionState
	if view == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if view.commandPending {
		return true
	}
	if err := a.flushPresentationNotesDraft(view); err != nil {
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
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		out := presentationCommandResult{state: state, view: view, generation: generation, proofread: true}
		var report presentationProofreadData
		havePage := false
		for start := 0; ; {
			command, err := a.executeRibbonApplicationCommand(token, "presentation.text.proofread", "presentation_document", token.documentID, "", map[string]int{
				"start": start, "limit": presentationReviewPageSize,
			})
			if err != nil {
				out.err = err
				break
			}
			var page presentationProofreadData
			if err := json.Unmarshal(command.Data, &page); err != nil || page.Start != start || page.ScannedSlides < 0 || page.ScannedSlides > presentationReviewPageSize || len(page.Findings) > presentationReviewMaxFindings || page.TotalSlides < 0 || start+page.ScannedSlides > page.TotalSlides || (page.HasMore && page.ScannedSlides == 0) {
				out.err = application.ErrInvalidCommandResult
				break
			}
			if !havePage {
				report.Start = 0
				report.TotalSlides = page.TotalSlides
				report.Complete = true
				havePage = true
			} else if report.TotalSlides != page.TotalSlides {
				out.err = fmt.Errorf("演示文稿在检查期间发生变化，请重新检查")
				break
			}
			report.ScannedSlides += page.ScannedSlides
			report.ElementsTruncated = report.ElementsTruncated || page.ElementsTruncated
			report.FindingsTruncated = report.FindingsTruncated || page.FindingsTruncated
			for _, finding := range page.Findings {
				if len(report.Findings) >= presentationReviewMaxFindings {
					report.FindingsTruncated = true
					break
				}
				report.Findings = append(report.Findings, finding)
			}
			if !page.HasMore || page.ElementsTruncated {
				report.HasMore = page.HasMore
				break
			}
			start += page.ScannedSlides
			if start >= presentationReviewMaxSlides {
				report.Complete = false
				report.HasMore = start < page.TotalSlides
				break
			}
		}
		if out.err == nil {
			report.Complete = !report.HasMore && !report.ElementsTruncated
			out.proofreadText = formatPresentationProofreadData(report)
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
	return true
}

func formatPresentationProofreadData(report presentationProofreadData) i18n.Message {
	if report.TotalSlides == 0 {
		return i18n.Message{ID: "presentation.proofread.report.no_slides"}
	}
	findings := report.Findings
	if len(findings) > 30 {
		findings = findings[:30]
	}
	return i18n.Message{ID: "presentation.proofread.report", Args: i18n.Args{
		"ScannedSlides": report.ScannedSlides, "TotalSlides": report.TotalSlides,
		"Complete": report.Complete && !report.ElementsTruncated, "FindingCount": len(report.Findings),
		"FindingsTruncated": report.FindingsTruncated, "Findings": findings,
		"HiddenFindings": max(0, len(report.Findings)-len(findings)),
	}}
}
