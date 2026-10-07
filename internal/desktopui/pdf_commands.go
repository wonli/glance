package desktopui

import (
	"context"
	"fmt"
	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"glance/internal/document"
	"glance/internal/printservice"
	"io"
	"strconv"
	"strings"
)

func (a *App) startPDFCommand(c document.PDFCommand) bool {
	v := a.ensurePDFView()
	if v.pending || a.isSaving() {
		return false
	}
	state := a.documentSessionState
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return false
	}
	v.pending = true
	v.lastCommand = c
	d := a.pdfDoc
	window := a.window
	shutdown := a.shutdown
	go func() {
		unlock := lockSessionAdapterWrite(state)
		var e error
		switch c.Kind {
		case "undo":
			d.Undo()
		case "redo":
			d.Redo()
		default:
			e = d.Apply(c)
		}
		info := d.Info()
		unlock()
		state.finishOperation(token)
		if state.ownerRuntime != nil {
			state.ownerRuntime.invalidateSessionViews(state.documentID)
		}
		select {
		case v.results <- pdfPageResult{mutation: true, err: e, info: info}:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
	return true
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
			v.page = n
			v.canvas.Position.First = n
			v.canvas.Position.Offset = 0
			v.clearSelection()
		}
		return
	}
	switch id {
	case "open":
		a.startOpenFilePicker()
	case "close":
		a.closeWorkspaceTab(a.activeTabID)
	case "thumbnails":
		v.thumbnails = !v.thumbnails
	case "search":
		v.showSearch = !v.showSearch
	case "find":
		a.startPDFSearch()
	case "previous":
		v.page = max(0, v.page-1)
		v.canvas.Position.First = v.page
		v.canvas.Position.Offset = 0
		v.clearSelection()
	case "next":
		v.page = min(v.info.Pages-1, v.page+1)
		v.canvas.Position.First = v.page
		v.canvas.Position.Offset = 0
		v.clearSelection()
	case "zoom-out":
		v.zoom = max(.25, v.zoom-.1)
		v.fit = ""
	case "zoom-in":
		v.zoom = min(4, v.zoom+.1)
		v.fit = ""
	case "fit-width":
		v.fit = "width"
	case "fit-page":
		v.fit = "page"
	case "read", "edit", "annotate", "signature":
		v.mode = id
		v.tool = ""
		if id == "signature" {
			v.tool = "ink"
			v.message = "在页面上手写签名（非数字证书签名）"
		}
	case "ink":
		v.mode = "annotate"
		v.tool = "ink"
		v.message = "拖动鼠标绘制，松开后提交；撤销可恢复"
	case "highlight":
		start, end := v.selectionRange()
		if start < 0 || end < 0 || len(v.scene.Text) == 0 {
			v.tool = "highlight"
			v.message = "点击或拖拽要高亮的文字"
			return
		}
		start = max(0, min(start, len(v.scene.Text)-1))
		end = max(0, min(end, len(v.scene.Text)-1))
		for i := start; i <= end; i++ {
			b := v.scene.Text[i].Bounds
			a.startPDFCommand(document.PDFCommand{Kind: "highlight", Page: v.page, Revision: v.scene.Revision, X: b.X, Y: b.Y, W: b.W, H: b.H})
		}
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
	if v.pending || query == "" {
		return
	}
	v.pending = true
	d := a.pdfDoc
	window := a.window
	shutdown := a.shutdown
	start := v.page
	go func() {
		r := pdfPageResult{info: d.Info()}
		for offset := 0; offset < r.info.Pages; offset++ {
			n := (start + offset) % r.info.Pages
			p, e := d.Page(n)
			if e != nil {
				r.err = e
				break
			}
			for _, t := range p.Text {
				if strings.Contains(strings.ToLower(t.Text), strings.ToLower(query)) {
					r.search = []int{n}
					break
				}
			}
			if len(r.search) > 0 {
				break
			}
		}
		if len(r.search) == 0 && r.err == nil {
			r.err = fmt.Errorf("未找到 %q", query)
			r.notice = "搜索完成"
		}
		select {
		case v.results <- r:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}
func (a *App) printCurrentPDF() {
	v := a.ensurePDFView()
	if v.pending {
		return
	}
	printer := a.wordPrinter
	if printer == nil || !printer.Available() {
		v.message = "当前平台不支持原生打印"
		return
	}
	v.pending = true
	d := a.pdfDoc
	window := a.window
	shutdown := a.shutdown
	go func() {
		b, e := d.Snapshot()
		if e == nil {
			e = printer.Print(context.Background(), b, printservice.Options{Copies: 1})
		}
		r := pdfPageResult{notice: "已提交打印", info: d.Info(), err: e}
		select {
		case v.results <- r:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}
