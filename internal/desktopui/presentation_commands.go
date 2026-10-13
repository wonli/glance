package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/application"
	"glance/internal/document"
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
		a.setStatus("新建幻灯片失败: " + err.Error())
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
	a.setStatus("已新建幻灯片")
}

type presentationCommandResult struct {
	state                                *documentSessionState
	view                                 *presentationViewState
	generation                           uint64
	accessibility                        bool
	accessibilityText                    string
	animationPane                        bool
	animationPaneText                    string
	proofread                            bool
	proofreadText                        string
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
		a.setStatus("演示文稿正在处理其他操作")
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
				a.setStatus("设置演示文稿主题失败：" + result.err.Error())
			case result.presentationGroup:
				a.setStatus("组合演示文稿对象失败：" + result.err.Error())
			case result.presentationLayoutList:
				a.setStatus("读取演示文稿版式失败：" + result.err.Error())
			case result.presentationLayoutSet:
				a.setStatus("设置幻灯片版式失败：" + result.err.Error())
			case result.animationPane:
				a.setStatus("读取动画窗格失败：" + result.err.Error())
			case result.transition:
				a.setStatus("设置切换效果失败：" + result.err.Error())
			case result.presentationAnimation:
				a.setStatus("设置元素动画失败：" + result.err.Error())
			case result.presentationAnimationDuration:
				a.setStatus("设置动画时长失败：" + result.err.Error())
			case result.background:
				a.setStatus("设置幻灯片背景失败：" + result.err.Error())
			case result.slideSize:
				a.setStatus("设置幻灯片大小失败：" + result.err.Error())
			case result.elementAlignment:
				a.setStatus("排列对象失败：" + result.err.Error())
			case result.proofread:
				a.setStatus("演示文稿文本检查失败：" + result.err.Error())
			case result.presentationCommentList:
				a.setStatus("读取演示文稿批注失败：" + result.err.Error())
			case result.presentationCommentMutation != "":
				a.setStatus("操作演示文稿批注失败：" + result.err.Error())
			case result.clipboardCopy:
				a.setStatus("复制文本失败：" + result.err.Error())
			case result.clipboardPaste:
				a.setStatus("粘贴文本失败：" + result.err.Error())
			case result.accessibility:
				a.setStatus("辅助功能检查失败：" + result.err.Error())
			case result.deleted:
				a.setStatus("删除幻灯片失败：" + result.err.Error())
			case result.createdTextBox:
				a.setStatus("新建文本框失败：" + result.err.Error())
			case result.createdShape:
				a.setStatus("新建形状失败：" + result.err.Error())
			case result.createdPicture:
				a.setStatus("插入图片失败：" + result.err.Error())
			case result.createdTable:
				a.setStatus("插入表格失败：" + result.err.Error())
			case result.createdChart:
				a.setStatus("插入图表失败：" + result.err.Error())
			case result.createdAudio:
				a.setStatus("插入音频失败：" + result.err.Error())
			case result.createdVideo:
				a.setStatus("插入视频失败：" + result.err.Error())
			default:
				a.setStatus("新建幻灯片失败：" + result.err.Error())
			}
			return
		}
		if result.accessibility {
			a.modal.Show("辅助功能检查", result.accessibilityText, func(action string) {
				a.modal.Hide()
			}, modalDialogActionSpec{ID: "close", Label: "关闭"})
			a.setStatus("已完成演示文稿辅助功能检查")
			return
		}
		if result.state.pptxDoc == nil {
			switch {
			case result.presentationTheme:
				a.setStatus("设置演示文稿主题失败：演示文稿会话已关闭")
			case result.presentationGroup:
				a.setStatus("组合演示文稿对象失败：演示文稿会话已关闭")
			case result.presentationLayoutList:
				a.setStatus("读取演示文稿版式失败：演示文稿会话已关闭")
			case result.presentationLayoutSet:
				a.setStatus("设置幻灯片版式失败：演示文稿会话已关闭")
			case result.animationPane:
				a.setStatus("读取动画窗格失败：演示文稿会话已关闭")
			case result.transition:
				a.setStatus("设置切换效果失败：演示文稿会话已关闭")
			case result.presentationAnimation:
				a.setStatus("设置元素动画失败：演示文稿会话已关闭")
			case result.presentationAnimationDuration:
				a.setStatus("设置动画时长失败：演示文稿会话已关闭")
			case result.background:
				a.setStatus("设置幻灯片背景失败：演示文稿会话已关闭")
			case result.slideSize:
				a.setStatus("设置幻灯片大小失败：演示文稿会话已关闭")
			case result.elementAlignment:
				a.setStatus("排列对象失败：演示文稿会话已关闭")
			case result.proofread:
				a.setStatus("演示文稿文本检查失败：演示文稿会话已关闭")
			case result.presentationCommentList:
				a.setStatus("读取演示文稿批注失败：演示文稿会话已关闭")
			case result.presentationCommentMutation != "":
				a.setStatus("操作演示文稿批注失败：演示文稿会话已关闭")
			case result.clipboardCopy:
				a.setStatus("复制文本失败：演示文稿会话已关闭")
			case result.clipboardPaste:
				a.setStatus("粘贴文本失败：演示文稿会话已关闭")
			case result.deleted:
				a.setStatus("删除幻灯片失败：演示文稿会话已关闭")
			case result.createdTextBox:
				a.setStatus("新建文本框失败：演示文稿会话已关闭")
			case result.createdShape:
				a.setStatus("新建形状失败：演示文稿会话已关闭")
			case result.createdPicture:
				a.setStatus("插入图片失败：演示文稿会话已关闭")
			case result.createdTable:
				a.setStatus("插入表格失败：演示文稿会话已关闭")
			case result.createdChart:
				a.setStatus("插入图表失败：演示文稿会话已关闭")
			case result.createdAudio:
				a.setStatus("插入音频失败：演示文稿会话已关闭")
			case result.createdVideo:
				a.setStatus("插入视频失败：演示文稿会话已关闭")
			default:
				a.setStatus("新建幻灯片失败：演示文稿会话已关闭")
			}
			return
		}
		if result.animationPane {
			a.modal.Show("动画窗格", result.animationPaneText, func(action string) {
				a.modal.Hide()
			}, modalDialogActionSpec{ID: "close", Label: "关闭"})
			a.setStatus("已读取当前幻灯片动画")
			return
		}
		if result.proofread {
			a.modal.Show("拼写检查", result.proofreadText, func(action string) {
				a.modal.Hide()
			}, modalDialogActionSpec{ID: "close", Label: "关闭"})
			a.setStatus("已完成演示文稿文本检查")
			return
		}
		if result.presentationCommentList {
			if len(result.presentationComments) == 0 {
				a.modal.Show("删除批注", "当前幻灯片没有批注。", func(action string) { a.modal.Hide() }, modalDialogActionSpec{ID: "close", Label: "关闭"})
				a.setStatus("当前幻灯片没有批注")
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
					a.setStatus("已添加批注：" + result.presentationCommentID)
				} else {
					a.setStatus("批注已存在：" + result.presentationCommentID)
				}
			} else {
				if result.presentationCommentChanged {
					a.setStatus("已删除批注：" + result.presentationCommentID)
				} else {
					a.setStatus("批注无需更改：" + result.presentationCommentID)
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
			a.setStatus("已复制所选元素文本")
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
				a.setStatus("已粘贴到原选中的文本元素")
			} else {
				a.setStatus("文本内容相同，无需粘贴")
			}
			return
		}
		if result.presentationTheme {
			v.snapshot = result.state.pptxDoc.Snapshot()
			a.persistActiveSession()
			if result.presentationThemeChanged {
				if result.presentationThemeOperation == "palette" {
					a.setStatus("已应用配色方案：" + result.presentationThemeID)
				} else {
					a.setStatus("已应用主题：" + result.presentationThemeID)
				}
			} else {
				a.setStatus("主题无需更改")
			}
			return
		}
		if result.presentationGroup {
			v.snapshot = result.state.pptxDoc.Snapshot()
			a.persistActiveSession()
			if result.presentationGroupChanged {
				a.setStatus(fmt.Sprintf("已组合 %d 个对象（组合 ID %d）", len(result.presentationGroupElementIDs), result.presentationGroupID))
			} else {
				a.setStatus("对象已组合，无需再次更改")
			}
			return
		}
		v.snapshot = result.state.pptxDoc.Snapshot()
		if result.presentationLayoutSet {
			a.persistActiveSession()
			if result.presentationLayoutChanged {
				a.setStatus(fmt.Sprintf("已将幻灯片 %d 设置为版式：%s", result.presentationLayoutSlideID, result.presentationLayoutOption.Name))
			} else {
				a.setStatus("幻灯片已使用该版式，无需更改")
			}
			return
		}
		if result.background {
			a.persistActiveSession()
			if result.backgroundChanged {
				a.setStatus("已更新幻灯片背景颜色")
			} else {
				a.setStatus("幻灯片背景无需更改")
			}
			return
		}
		if result.slideSize {
			a.persistActiveSession()
			if result.slideSizeChanged {
				a.setStatus(fmt.Sprintf("已设置画布尺寸为 %.2f × %.2f point；对象位置和尺寸保持不变", result.slideWidthPT, result.slideHeightPT))
			} else {
				a.setStatus("幻灯片大小无需更改")
			}
			return
		}
		if result.elementAlignment {
			a.persistActiveSession()
			if result.alignmentChanged {
				a.setStatus("已将所选对象" + result.alignmentAction)
			} else {
				a.setStatus("对象已符合" + result.alignmentAction + "，无需更改")
			}
			return
		}
		if result.transition {
			a.persistActiveSession()
			if result.transitionChanged {
				a.setStatus(fmt.Sprintf("已更新 %d 张幻灯片的切换效果", result.transitionAffected))
			} else {
				a.setStatus("切换效果无需更改")
			}
			return
		}
		if result.presentationAnimation {
			a.persistActiveSession()
			if result.presentationAnimationChanged {
				a.setStatus("已添加动画：" + result.presentationAnimationEffect)
			} else {
				a.setStatus("动画无需更改")
			}
			return
		}
		if result.presentationAnimationDuration {
			a.persistActiveSession()
			if result.presentationAnimationDurationChanged {
				a.setStatus(fmt.Sprintf("已将所选动画时长设为 %d 毫秒", result.presentationAnimationDurationMS))
			} else {
				a.setStatus("动画时长无需更改")
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
					a.setStatus("新建形状失败：返回的幻灯片未出现在当前视图中")
				} else if result.createdPicture {
					a.setStatus("插入图片失败：返回的幻灯片未出现在当前视图中")
				} else if result.createdTable {
					a.setStatus("插入表格失败：返回的幻灯片未出现在当前视图中")
				} else if result.createdChart {
					a.setStatus("插入图表失败：返回的幻灯片未出现在当前视图中")
				} else if result.createdAudio {
					a.setStatus("插入音频失败：返回的幻灯片未出现在当前视图中")
				} else if result.createdVideo {
					a.setStatus("插入视频失败：返回的幻灯片未出现在当前视图中")
				} else {
					a.setStatus("新建文本框失败：返回的幻灯片未出现在当前视图中")
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
			a.setStatus("已删除幻灯片")
		case result.createdTextBox:
			a.setStatus("已新建文本框；双击文本框可编辑内容")
		case result.createdShape:
			a.setStatus("已新建矩形；可调整其大小和样式")
		case result.createdPicture:
			a.setStatus("已插入图片；可调整大小和样式")
		case result.createdTable:
			a.setStatus("已插入表格；Agent 可继续填入和整理单元格数据")
		case result.createdChart:
			a.setStatus("已插入图表模板；Agent 可按实际数据新建正式图表")
		case result.createdAudio:
			a.setStatus("已嵌入音频文件")
		case result.createdVideo:
			a.setStatus("已嵌入视频文件")
		default:
			a.setStatus(fmt.Sprintf("已新建第 %d 张幻灯片", v.activeSlide+1))
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
		a.setStatus("演示文稿正在处理其他操作")
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

func formatPresentationAccessibilityReview(report presentationAccessibilityReviewResult) string {
	text := fmt.Sprintf("检查了 %d/%d 张幻灯片。\n此规则检查关注可读取文本和图片替代文本，不判断图片语义、颜色对比度或内容质量。", report.CheckedSlides, report.TotalSlides)
	if report.Complete {
		text += "\n检查完整。"
	} else {
		text += "\n检查结果不完整。可通过 document.inspect 使用 start/limit 继续分页检查。"
	}
	if len(report.Issues) == 0 {
		if report.ElementLimitReached {
			text += "\n在当前扫描到的元素中未发现可读取文本或图片替代文本问题。"
		} else {
			text += "\n未发现缺少可读取文本或图片替代文本的问题。"
		}
		return text
	}
	text += fmt.Sprintf("\n发现 %d 项需要复核：", len(report.Issues))
	for index, issue := range report.Issues {
		if index == 30 {
			text += fmt.Sprintf("\n另有 %d 项未显示。", len(report.Issues)-index)
			break
		}
		text += fmt.Sprintf("\n· 第 %d 张幻灯片", issue.SlideNumber)
		if issue.ElementID != 0 {
			text += fmt.Sprintf("，元素 %d", issue.ElementID)
		}
		text += "：" + issue.Message
	}
	return text
}

func (a *App) handlePresentationDeleteSlide(v *presentationViewState) {
	if a.pptxDoc == nil {
		return
	}
	if v.snapshot.SlideCount <= 1 {
		a.setStatus("无法删除演示文稿中的最后一页幻灯片")
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
		a.setStatus("删除幻灯片失败: " + err.Error())
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
	a.setStatus("已删除幻灯片")
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
		a.setStatus("演示文稿正在处理其他操作")
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
	a.setStatus("请点击幻灯片上的位置放置文本框")
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
		a.setStatus("新建文本框失败：" + err.Error())
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
	a.setStatus("已新建文本框；双击文本框可编辑内容")
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
		a.setStatus("演示文稿正在处理其他操作")
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
	a.setStatus("请点击幻灯片上的位置放置矩形")
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
		a.setStatus("新建形状失败：" + err.Error())
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
	a.setStatus("已新建矩形；可调整其大小和样式")
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
		a.setStatus("演示文稿正在处理其他操作")
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
		a.setStatus("插入图片失败：" + err.Error())
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
	a.setStatus("已插入图片；可调整大小和样式")
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
		a.setStatus("演示文稿正在处理其他操作")
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
