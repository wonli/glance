package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"glance/internal/application"
	"glance/internal/document"
	"io"
	"math"
	"strconv"
	"strings"
)

func (a *App) startPDFCommand(c document.PDFCommand) bool {
	v := a.ensurePDFView()
	if v.pending || a.isSaving() {
		return false
	}
	state := a.documentSessionState
	if state == nil || state.pdfDoc == nil {
		return false
	}
	runtime := a.ensureDesktopRuntime()
	if runtime.applicationRuntimeErr != nil || runtime.applicationRuntime == nil {
		a.setStatus("PDF Agent 能力目录不可用")
		return false
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return false
	}
	if c.Revision != 0 && c.Revision != token.revision {
		state.finishOperation(token)
		a.setStatus("PDF 页面已更新，请重新选择后再操作")
		return false
	}
	capabilityID, arguments, err := pdfCommandCapability(c)
	if err != nil {
		state.finishOperation(token)
		a.setStatus("PDF 操作无效：" + err.Error())
		return false
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		state.finishOperation(token)
		return false
	}
	v.pending = true
	v.lastCommand = c
	window := a.window
	shutdown := a.shutdown
	done := v.workerSignal()
	commandContext, cancel := context.WithCancel(context.Background())
	a.launchManagedBackground(func() {
		defer cancel()
		a.launchManagedBackground(func() {
			select {
			case <-shutdown:
				cancel()
			case <-commandContext.Done():
			}
		})
		invocation, e := runtime.applicationRuntime.InvokeCapability(commandContext, application.CapabilityRequest{
			RequestID:  fmt.Sprintf("pdf-ribbon-%s-%d", token.documentID, token.generation),
			DocumentID: token.documentID, CapabilityID: capabilityID, ExpectedRevision: token.revision,
			Operator: application.Operator{ID: fmt.Sprintf("window-%d", a.windowID), Kind: application.OperatorHuman},
			View: application.ViewContext{
				WindowID: a.windowID, DocumentID: token.documentID, Revision: token.revision,
				Selection: &application.SelectionContext{TargetType: "pdf_page", TargetID: strconv.Itoa(c.Page + 1)},
			},
			Arguments: encoded,
		})
		changed := false
		if e == nil {
			var result struct {
				Changed bool `json:"changed"`
			}
			if json.Unmarshal(invocation.Data, &result) != nil {
				e = application.ErrInvalidCommandResult
			} else {
				changed = result.Changed
			}
		}
		state.finishOperation(token)
		if changed && state.ownerRuntime != nil {
			state.ownerRuntime.invalidateSessionViews(state.documentID)
		}
		result := pdfPageResult{mutation: changed, err: e, info: state.pdfDoc.Info()}
		if e == nil && !changed {
			result.notice = "PDF 操作没有产生更改"
		}
		select {
		case v.results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-done:
		case <-shutdown:
		}
	})
	return true
}

func pdfCommandCapability(command document.PDFCommand) (string, any, error) {
	switch command.Kind {
	case "rotate":
		return "pdf.page.rotate", map[string]any{"page_number": command.Page + 1}, nil
	case "replace":
		return "pdf.text.replace", map[string]any{"page_number": command.Page + 1, "run": command.Run, "text": command.Text}, nil
	case "note":
		return "pdf.annotation.note", map[string]any{"page_number": command.Page + 1, "x": command.X, "y": command.Y, "text": command.Text}, nil
	case "highlight":
		return "pdf.annotation.highlight", map[string]any{
			"page_number": command.Page + 1,
			"bounds":      map[string]any{"x": command.X, "y": command.Y, "w": command.W, "h": command.H},
			"text":        command.Text,
		}, nil
	case "ink":
		return "pdf.annotation.ink", map[string]any{"page_number": command.Page + 1, "points": command.Points}, nil
	case "undo", "redo":
		return "pdf.history.apply", map[string]any{"operation": command.Kind}, nil
	default:
		return "", nil, fmt.Errorf("unsupported PDF command %q", command.Kind)
	}
}
func (a *App) pdfTextDialog(kind string) {
	v := a.ensurePDFView()
	p := v.scene
	initial := ""
	run := -1
	x, y := 40.0, 40.0
	if v.selected >= 0 && v.selected < len(p.Text) {
		t := p.Text[v.selected]
		run = t.Run
		x, y = t.Bounds.X, t.Bounds.Y
		if kind == "replace" {
			if !t.Editable {
				a.setStatus("此文字暂不支持原位编辑")
				return
			}
			initial = t.Text
		}
	}
	if kind == "replace" && run < 0 {
		return
	}

	a.showPDFDraft(document.PDFCommand{Kind: kind, Page: p.Index, Revision: p.Revision, Run: run, Text: initial, X: x, Y: y}, "确认后更新当前页面；取消不修改文档。")
}
func (a *App) showPDFDraft(command document.PDFCommand, message string) {
	title := "添加批注"
	if command.Kind == "replace" {
		title = "编辑 PDF 文字"
	}
	state := a.documentSessionState
	a.modal.ShowInput(title, message, command.Text, "输入内容", func(action string) {
		if action != "confirm" {
			a.modal.Hide()
			return
		}
		if a.documentSessionState != state {
			return
		}
		command.Text = a.modal.Input.Text()
		if a.startPDFCommand(command) {
			a.modal.Hide()
		} else {
			a.modal.Message = "文档正在处理，请稍后重试；输入已保留。"
		}
	}, modalDialogActionSpec{ID: "cancel", Label: "取消"}, modalDialogActionSpec{ID: "confirm", Label: "应用", Style: modalDialogActionPrimary})
}

func (a *App) handlePDFAction(gtx layout.Context, id string) {
	v := a.ensurePDFView()
	if strings.HasPrefix(id, "page-") {
		n, e := strconv.Atoi(strings.TrimPrefix(id, "page-"))
		if e == nil {
			a.runPDFViewAction("set_page", map[string]any{"operation": "set_page", "page_number": n + 1})
		}
		return
	}
	switch id {
	case "open":
		a.startOpenFilePicker()
	case "close":
		a.closeWorkspaceTab(a.activeTabID)
	case "thumbnails":
		a.runPDFViewAction("set_thumbnails", map[string]any{"operation": "set_thumbnails", "enabled": !v.thumbnails})
	case "search":
		a.runPDFViewAction("search", map[string]any{"operation": "set_search_visible", "enabled": !v.showSearch})
	case "find":
		a.startPDFSearch()
	case "previous":
		a.runPDFViewAction("previous", map[string]any{"operation": "previous"})
	case "next":
		a.runPDFViewAction("next", map[string]any{"operation": "next"})
	case "zoom-out":
		zoom := int(math.Round(min(4, max(.25, v.currentZoom(gtx.Metric)-.1)) * 100))
		a.runPDFViewAction("zoom-out", map[string]any{"operation": "set_zoom", "zoom_percent": zoom})
	case "zoom-in":
		zoom := int(math.Round(min(4, v.currentZoom(gtx.Metric)+.1) * 100))
		a.runPDFViewAction("zoom-in", map[string]any{"operation": "set_zoom", "zoom_percent": zoom})
	case "zoom-indicator", "zoom-100", "actual-size":
		a.runPDFViewAction("reset_zoom", map[string]any{"operation": "reset_zoom"})
	case "fit-width":
		a.runPDFViewAction("fit_width", map[string]any{"operation": "fit_width"})
	case "fit-page":
		a.runPDFViewAction("fit_page", map[string]any{"operation": "fit_page"})
	case "read", "edit", "annotate", "signature":
		a.runPDFViewAction("set_mode", map[string]any{"operation": "set_mode", "mode": id})
	case "ink":
		a.runPDFViewAction("ink", map[string]any{"operation": "set_tool", "tool": "ink"})
	case "highlight":
		start, end := v.selectionRange()
		if start < 0 || end < 0 || len(v.scene.Text) == 0 {
			a.runPDFViewAction("highlight", map[string]any{"operation": "set_tool", "tool": "highlight"})
			return
		}
		start = max(0, min(start, len(v.scene.Text)-1))
		end = max(0, min(end, len(v.scene.Text)-1))
		first := v.scene.Text[start].Bounds
		left, top, right, bottom := first.X, first.Y, first.X+first.W, first.Y+first.H
		for i := start + 1; i <= end; i++ {
			bounds := v.scene.Text[i].Bounds
			left = min(left, bounds.X)
			top = min(top, bounds.Y)
			right = max(right, bounds.X+bounds.W)
			bottom = max(bottom, bounds.Y+bounds.H)
		}
		a.startPDFCommand(document.PDFCommand{
			Kind: "highlight", Page: v.page, Revision: v.scene.Revision,
			X: left, Y: top, W: right - left, H: bottom - top, Text: v.selectedText(),
		})
	case "note":
		a.pdfTextDialog("note")
	case "replace":
		a.pdfTextDialog("replace")
	case "copy":
		text := v.selectedText()
		if text == "" && v.selected >= 0 && v.selected < len(v.scene.Text) {
			text = v.scene.Text[v.selected].Text
		}
		if text != "" {
			gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
		}
	case "rotate", "undo", "redo":
		a.startPDFCommand(document.PDFCommand{Kind: id, Page: v.page, Revision: v.info.Revision})
	case "save":
		if sessionPath(a.documentSessionState) == "" {
			a.startFileCommand(commandSaveAs, "pdf")
		} else {
			a.startSave()
		}
	case "save-as", "download":
		a.startFileCommand(commandSaveAs, "pdf")
	case "print":
		a.printCurrentPDF()
	}
}
func (a *App) startPDFSearch() {
	v := a.ensurePDFView()
	query := strings.TrimSpace(v.search.Text())
	state := a.documentSessionState
	if v.pending || query == "" || state == nil || state.pdfDoc == nil {
		return
	}
	v.pending = true
	d := a.pdfDoc
	window := a.window
	shutdown := a.shutdown
	documentID := state.documentID
	revision := state.wordRevisionAtomic.Load()
	startPage := v.page + 1
	a.launchManagedBackground(func() {
		r := pdfPageResult{info: d.Info()}
		ranges := [][2]int{{startPage, r.info.Pages}, {1, startPage - 1}}
		runtime := a.ensureDesktopRuntime()
		if runtime.applicationRuntimeErr != nil || runtime.applicationRuntime == nil {
			r.err = application.ErrRuntimeClosed
		}
		for _, pageRange := range ranges {
			for first := pageRange[0]; r.err == nil && first <= pageRange[1] && len(r.search) == 0; {
				pageLimit := min(100, pageRange[1]-first+1)
				arguments, err := json.Marshal(map[string]any{
					"query": query, "start_page": first, "page_limit": pageLimit, "max_results": 1,
				})
				if err != nil {
					r.err = err
					break
				}
				invocation, err := runtime.applicationRuntime.InvokeCapability(context.Background(), application.CapabilityRequest{
					RequestID:  fmt.Sprintf("pdf-search-%s-%d-%d", documentID, revision, first),
					DocumentID: documentID, CapabilityID: "pdf.document.search", ExpectedRevision: revision,
					Operator:  application.Operator{ID: fmt.Sprintf("window-%d", a.windowID), Kind: application.OperatorHuman},
					Arguments: arguments,
				})
				if err != nil {
					r.err = err
					break
				}
				var result struct {
					TotalPages int `json:"total_pages"`
					Matches    []struct {
						PageNumber int `json:"page_number"`
					} `json:"matches"`
					PageContentTruncated bool `json:"page_content_truncated"`
				}
				if err := json.Unmarshal(invocation.Data, &result); err != nil {
					r.err = application.ErrInvalidCommandResult
					break
				}
				if len(result.Matches) > 0 {
					r.search = []int{result.Matches[0].PageNumber - 1}
					break
				}
				if result.PageContentTruncated {
					r.notice = "搜索范围内页面内容达到读取上限，结果可能不完整"
					break
				}
				if result.TotalPages < 1 {
					r.err = application.ErrInvalidCommandResult
					break
				}
				first += pageLimit
			}
			if r.err != nil || len(r.search) > 0 || r.notice != "" {
				break
			}
		}
		if len(r.search) == 0 && r.err == nil {
			if r.notice == "" {
				r.err = fmt.Errorf("未找到 %q", query)
				r.notice = "搜索完成"
			}
		}
		r.info = d.Info()
		select {
		case v.results <- r:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
}
func (a *App) printCurrentPDF() {
	v := a.ensurePDFView()
	if v.pending {
		return
	}
	state := a.documentSessionState
	if state == nil || state.pdfDoc == nil || state.closed.Load() {
		v.message = "PDF 文档会话不可用"
		return
	}
	if a.nativePrinter == nil || !a.nativePrinter.Available() {
		v.message = "当前平台不支持原生打印"
		return
	}
	runtime := a.ensureDesktopRuntime()
	if runtime.applicationRuntimeErr != nil || runtime.applicationRuntime == nil || !a.applicationSessionRegistered(state.documentID) {
		v.message = "PDF 应用运行时不可用，无法通过共享打印能力执行"
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		v.message = "PDF 文档正在处理其他操作，请稍后重试"
		return
	}
	v.pending = true
	done := v.workerSignal()
	window := a.window
	shutdown := a.shutdown
	ctx, cancel := context.WithCancel(context.Background())
	a.launchManagedBackground(func() {
		defer cancel()
		a.launchManagedBackground(func() {
			select {
			case <-shutdown:
				cancel()
			case <-ctx.Done():
			}
		})
		arguments, _ := json.Marshal(map[string]any{"copies": 1})
		invocation, err := runtime.applicationRuntime.InvokeCapability(ctx, application.CapabilityRequest{
			RequestID:  fmt.Sprintf("pdf-print-%s-%d", token.documentID, token.generation),
			DocumentID: token.documentID, CapabilityID: "pdf.document.print", ExpectedRevision: token.revision,
			Operator:  application.Operator{ID: fmt.Sprintf("window-%d", a.windowID), Kind: application.OperatorHuman},
			View:      application.ViewContext{WindowID: a.windowID, DocumentID: token.documentID, Revision: token.revision},
			Arguments: arguments,
		})
		result := pdfPageResult{info: state.pdfDoc.Info(), err: err}
		if err == nil {
			var printed struct {
				Status          string `json:"status"`
				PrintedRevision uint64 `json:"printed_revision"`
			}
			if decodeErr := json.Unmarshal(invocation.Data, &printed); decodeErr != nil || printed.Status != "sent_to_system_print_queue" || printed.PrintedRevision != token.revision {
				result.err = application.ErrInvalidCommandResult
			} else {
				result.notice = fmt.Sprintf("已将 PDF 修订版本 r%d 发送到系统打印服务", printed.PrintedRevision)
			}
		}
		state.finishOperation(token)
		select {
		case v.results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-done:
		case <-shutdown:
		}
	})
}
