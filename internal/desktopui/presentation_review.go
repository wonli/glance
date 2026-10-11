package desktopui

import (
	"encoding/json"
	"fmt"
	"strings"

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
		a.setStatus("演示文稿正在处理其他操作")
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

func formatPresentationAnimationPane(data presentationAnimationPaneData) string {
	var body strings.Builder
	fmt.Fprintf(&body, "幻灯片 ID：%d\n读取修订：%d\n", data.SlideID, data.Revision)
	if !data.HasTiming || len(data.Animations) == 0 {
		body.WriteString("此幻灯片没有可读取的动画效果。")
	} else {
		body.WriteString("\n动画效果：\n")
		limit := len(data.Animations)
		if limit > 100 {
			limit = 100
		}
		for _, animation := range data.Animations[:limit] {
			fmt.Fprintf(&body, "%d. %s", animation.Index, animation.Kind)
			if animation.TargetElementID != 0 {
				fmt.Fprintf(&body, " · 元素 %d", animation.TargetElementID)
			}
			if animation.PresetClass != "" || animation.PresetID != "" {
				fmt.Fprintf(&body, " · 预设 %s/%s", animation.PresetClass, animation.PresetID)
			}
			if animation.Effect != "" {
				fmt.Fprintf(&body, " · 效果 %s", animation.Effect)
			}
			if animation.Transition != "" {
				fmt.Fprintf(&body, " · 方向 %s", animation.Transition)
			}
			if animation.Duration != "" {
				fmt.Fprintf(&body, " · 时长 %s", animation.Duration)
			}
			body.WriteByte('\n')
		}
		if limit < len(data.Animations) {
			fmt.Fprintf(&body, "……界面显示前 %d 项；MCP 查询可读取最多 %d 项。\n", limit, len(data.Animations))
		}
	}
	if data.Truncated {
		body.WriteString("\n结果不完整：动画时序达到解析上限。")
	}
	for _, diagnostic := range data.Diagnostics {
		fmt.Fprintf(&body, "\n诊断：%s", diagnostic)
	}
	return body.String()
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
		a.setStatus("演示文稿正在处理其他操作")
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

func formatPresentationProofreadData(report presentationProofreadData) string {
	if report.TotalSlides == 0 {
		return "演示文稿没有幻灯片。此检查只关注重复中文标点及中文括号/弯引号数量，不含词典拼写、语法或语义检查。"
	}
	message := fmt.Sprintf("检查了 %d/%d 张幻灯片。\n此检查只关注重复中文标点及中文括号/弯引号数量，不含词典拼写、语法或语义检查。", report.ScannedSlides, report.TotalSlides)
	if report.Complete && !report.ElementsTruncated {
		message += "\n检查完整。"
	} else {
		message += "\n检查不完整；MCP 可通过 start/limit 继续分页。"
	}
	if len(report.Findings) == 0 {
		message += "\n未发现这两类问题。"
		return message
	}
	if report.FindingsTruncated {
		message += fmt.Sprintf("\n发现超过 %d 项，以下显示前 30 项：", presentationReviewMaxFindings)
	} else {
		message += fmt.Sprintf("\n发现 %d 项需要复核：", len(report.Findings))
	}
	for index, finding := range report.Findings {
		if index == 30 {
			message += fmt.Sprintf("\n另有 %d 项未显示。", len(report.Findings)-index)
			break
		}
		message += fmt.Sprintf("\n· 第 %d 张幻灯片，元素 %d：%s", finding.SlideNumber, finding.ElementID, finding.Message)
	}
	return message
}
