package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/application"
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
		a.setStatus("Word 文档正在处理其他操作")
		return
	}
	a.wordEditorReviewRequest++
	request := a.wordEditorReviewRequest
	generation := a.documentGeneration
	window, shutdown, results := a.window, a.shutdown, a.wordEditorReviewResults
	a.modal.Show("文档编辑器", "正在扫描 Word 文档…", func(string) {
		if a.wordEditorReviewRequest == request {
			a.wordEditorReviewRequest++
		}
		a.modal.Hide()
		if window != nil {
			window.Invalidate()
		}
	}, modalDialogActionSpec{ID: "close", Label: "关闭"})
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
	if result.request != a.wordEditorReviewRequest || !a.modal.Visible || a.modal.Title != "文档编辑器" {
		return
	}
	if result.state == nil || result.state.closed.Load() || result.state != a.documentSessionState || result.generation != a.documentGeneration {
		a.modal.Message = "当前文档已切换，审阅结果已丢弃。"
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.revision != result.state.wordRevisionAtomic.Load() {
		a.modal.Message = "文档在扫描期间发生变化，审阅结果已过期。请重新审阅。"
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.err != nil {
		a.modal.Message = "审阅失败：" + result.err.Error()
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	report, err := wordEditorReviewMessage(result.data)
	if err != nil {
		a.modal.Message = "无法读取审阅结果：" + err.Error()
	} else {
		a.modal.Message = report
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func wordEditorReviewMessage(data []byte) (string, error) {
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
	report := fmt.Sprintf("【文档编辑器启发式评分：%d / 100】\n\n"+
		"统计概览：\n· 本次扫描正文块：%d / %d\n· 总字符数：%d（不含空白：%d）\n· 段落总数：%d\n· 表格总数：%d\n· 预估阅读耗时：约 %d 分钟\n\n",
		result.Score, result.ScannedBlocks, result.TotalBlocks, result.CharactersWithWhitespace,
		result.CharactersWithoutWhitespace, result.Paragraphs, result.Tables, estMin)
	if len(result.Findings) > 0 {
		report += "启发式检查：\n"
		for i, finding := range result.Findings {
			if i >= 3 {
				report += "· 更多发现可通过 MCP 分页读取。\n"
				break
			}
			report += fmt.Sprintf("· 正文块 %d（%s；段落 %s，文本运行 %s）：%s\n", finding.BlockIndex+1, finding.BlockID, finding.ParagraphID, finding.RunID, finding.Detail)
		}
	} else {
		report += "未发现重复中文标点或括号/引号数量不平衡。"
	}
	if result.FindingsTruncated {
		report += "\n部分发现已省略，请通过 MCP 分页检查。"
	}
	if result.HasMore {
		report += "\n文档超出本次扫描的 1000 个正文块；可通过 MCP 工具继续分页。"
	}
	report += "\n\n此轻量检查不执行词典拼写、语法或语义分析。"
	return report, nil
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
		a.setStatus("未找到可翻译的正文内容，请先选择包含文本的段落")
		return
	}

	previewText := originalText
	if previewText == "" {
		previewText = "（空段落）"
	} else {
		runes := []rune(previewText)
		if len(runes) > 25 {
			previewText = string(runes[:25]) + "..."
		}
	}

	a.modal.ShowInput(
		"翻译",
		fmt.Sprintf("原文内容：%s\n请输入翻译译文（确认后将替换选中文本）：", previewText),
		originalText,
		"输入翻译后的内容",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			translated := strings.TrimSpace(a.modal.Input.Text())
			if translated == "" {
				return
			}
			if !a.runWordApplicationCommand("翻译替换", "word.text_run.replace", map[string]any{
				"run_id": activeRunID, "value": translated,
			}) {
				a.setStatus("无法启动 Word 翻译替换操作")
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "替换入正文"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
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
		message := "文档比较服务当前不可用。"
		if runtime.applicationRuntimeErr != nil {
			message += "\n" + runtime.applicationRuntimeErr.Error()
		}
		a.modal.Show("文档比较", message, func(string) {
			if a.wordCompareRequest == request {
				a.wordCompareRequest++
			}
			a.modal.Hide()
		}, modalDialogActionSpec{ID: "close", Label: "关闭"})
		return
	}
	documents, err := runtime.applicationRuntime.ListDocuments(context.Background())
	if err != nil {
		a.modal.Show("文档比较", "无法列出可比较的实时文档：\n"+err.Error(), func(string) {
			if a.wordCompareRequest == request {
				a.wordCompareRequest++
			}
			a.modal.Hide()
		}, modalDialogActionSpec{ID: "close", Label: "关闭"})
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
		label := wordCompareDocumentLabel(info)
		labels[actionID] = label
	}
	if len(otherDocuments) == 0 {
		a.modal.Show("文档比较", "当前没有其他已打开的 Word 文档。\n请先打开或创建另一份 Word 文档，再比较两个版本。", func(string) {
			if a.wordCompareRequest == request {
				a.wordCompareRequest++
			}
			a.modal.Hide()
		}, modalDialogActionSpec{ID: "close", Label: "关闭"})
		return
	}
	message := fmt.Sprintf("文档版本比较\n当前文档：%s\n\n选择另一份已打开的 Word 文档。比较范围为双方正文开头最多 1000 个正文块。", wordCompareDocumentLabel(application.DocumentInfo{
		DocumentID: state.documentID, SourcePath: sessionPath(state), Dirty: sessionDirty(state),
	}))
	a.wordCompareDocumentButtons = make(map[string]*widget.Clickable, len(otherDocuments))
	for _, info := range otherDocuments {
		a.wordCompareDocumentButtons["compare:"+info.DocumentID] = new(widget.Clickable)
	}

	a.modal.ShowCustom("文档比较", message, 520,
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
		}, modalDialogActionSpec{ID: "close", Label: "关闭"})
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

func wordCompareDocumentLabel(info application.DocumentInfo) string {
	name := filepath.Base(strings.TrimSpace(info.SourcePath))
	if name == "." || name == string(filepath.Separator) {
		name = ""
	}
	if name == "" {
		id := info.DocumentID
		if len(id) > 8 {
			id = id[:8]
		}
		name = "未命名文档 " + id
	}
	if info.Dirty {
		name += "（未保存修改）"
	}
	return name
}

func (a *App) startWordDocumentCompare(state *documentSessionState, other application.DocumentInfo, pickerRequest uint64) {
	if state == nil || state != a.documentSessionState || pickerRequest != a.wordCompareRequest {
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.modal.Hide()
		a.setStatus("Word 文档正在处理其他操作")
		return
	}
	a.wordCompareRequest++
	request := a.wordCompareRequest
	generation := a.documentGeneration
	window, shutdown, results := a.window, a.shutdown, a.wordCompareResults
	a.modal.Show("文档比较", "正在比较两份 Word 文档…", func(string) {
		if a.wordCompareRequest == request {
			a.wordCompareRequest++
		}
		a.modal.Hide()
		if window != nil {
			window.Invalidate()
		}
	}, modalDialogActionSpec{ID: "close", Label: "关闭"})
	a.launchManagedBackground(func() {
		command, err := a.executeRibbonApplicationCommand(token, "document.compare", "two_word_documents", state.documentID, "", map[string]any{
			"left_document_id": state.documentID, "right_document_id": other.DocumentID,
			"left_expected_revision": token.revision, "right_expected_revision": other.Revision,
			"start": 0, "limit": 1000,
		})
		state.finishOperation(token)
		result := wordCompareAsyncResult{
			state: state, generation: generation, request: request,
			leftDocumentID: state.documentID, rightDocumentID: other.DocumentID,
			leftRevision: token.revision, rightRevision: other.Revision,
			rightLabel: wordCompareDocumentLabel(other), data: command.Data, err: err,
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
	if result.request != a.wordCompareRequest || !a.modal.Visible || a.modal.Title != "文档比较" {
		return
	}
	if result.state == nil || result.state.closed.Load() || result.state != a.documentSessionState || result.generation != a.documentGeneration {
		a.modal.Message = "当前文档已切换，比较结果已丢弃。"
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.state.documentID != result.leftDocumentID || result.state.wordRevisionAtomic.Load() != result.leftRevision {
		a.modal.Message = "比较期间当前文档发生变化，结果已过期。请重新比较。"
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	if result.err != nil {
		a.modal.Message = "比较失败：" + result.err.Error()
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	runtime := a.ensureDesktopRuntime()
	if runtime.applicationRuntime == nil {
		a.modal.Message = "比较失败：文档运行时已关闭。"
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	rightInfo, err := runtime.applicationRuntime.GetDocument(context.Background(), result.rightDocumentID)
	if err != nil || rightInfo.Revision != result.rightRevision {
		a.modal.Message = "比较期间所选文档已关闭或发生变化，结果已过期。请重新比较。"
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	message, err := wordDocumentCompareMessage(result.data, result.rightLabel)
	if err != nil {
		a.modal.Message = "无法读取比较结果：" + err.Error()
	} else {
		a.modal.Message = message
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func wordDocumentCompareMessage(data []byte, rightLabel string) (string, error) {
	var result struct {
		LeftRevision       uint64   `json:"left_revision"`
		RightRevision      uint64   `json:"right_revision"`
		ChangedPaths       []string `json:"changed_paths"`
		HasMoreDifferences bool     `json:"has_more_differences"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	message := fmt.Sprintf("比较完成\n对比文档：%s\n修订版本：当前文档 r%d，对比文档 r%d\n比较范围：双方正文开头最多 1000 个正文块\n\n",
		rightLabel, result.LeftRevision, result.RightRevision)
	if len(result.ChangedPaths) == 0 {
		message += "在本次读取的正文范围内未发现差异。"
	} else {
		message += fmt.Sprintf("发现 %d 处变化路径：\n", len(result.ChangedPaths))
		for i, path := range result.ChangedPaths {
			if i >= 12 {
				message += fmt.Sprintf("· 其余 %d 项请通过 MCP 工具读取。\n", len(result.ChangedPaths)-i)
				break
			}
			message += "· " + path + "\n"
		}
	}
	if result.HasMoreDifferences {
		message += "\n变化路径已达到结果上限；可缩小比较范围后继续检查。"
	}
	return message, nil
}
