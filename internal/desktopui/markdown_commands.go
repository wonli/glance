package desktopui

import (
	"encoding/json"
	"fmt"
	"gioui.org/io/key"
	"gioui.org/layout"
	"glance/internal/application"
	"glance/internal/format/markdowndoc"
	"glance/internal/i18n"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func (a *App) flushMarkdownDraft() {
	if a == nil || a.documentSessionState == nil || a.markdownDoc == nil || a.markdownView == nil {
		return
	}
	v := a.markdownView
	a.observeMarkdownDraft()
	if markdownEditorComposing(&v.editor) {
		return
	}
	source := v.text()
	if source == v.sourceSynced {
		return
	}
	token, ok := a.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	unlock := lockSessionAdapterWrite(a.documentSessionState)
	err := a.markdownDoc.SetText(source)
	if err == nil {
		v.sourceSynced = source
		setMarkdownDraftState(a.documentSessionState, v, false, false)
	}
	unlock()
	a.completeSessionOperation(a.documentSessionState, token, err, err == nil)
	if err == nil {
		a.persistActiveSession()
	} else {
		a.setStatusErrorMessage(i18n.Message{ID: "markdown.error.operation_failed"}, err, notificationToast)
	}
}
func (a *App) applyMarkdownEdit(action string, args markdowndoc.EditArgs) error {
	v := a.ensureMarkdownView()
	if markdownEditorComposing(&v.editor) {
		return fmt.Errorf("请先完成输入法选字")
	}
	start, end := v.editor.Selection()
	if a.documentSessionState != nil && a.applicationSessionRegistered(a.documentSessionState.documentID) {
		return a.startMarkdownApplicationEdit(v, action, args, start, end)
	}
	out, err := markdowndoc.ApplyEdit(v.text(), action, start, end, args)
	if err != nil {
		return err
	}
	v.editor.SetCaret(0, len([]rune(v.text())))
	v.editor.Insert(out.Text)
	v.editor.SetCaret(out.Start, out.End)
	a.observeMarkdownDraft()
	v.focus = true
	a.flushMarkdownDraft()
	return nil
}

type startMarkdownApplicationEditPayload struct {
	Action      string `json:"action"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
	Label       string `json:"label"`
	Destination string `json:"destination"`
	Display     bool   `json:"display"`
}

func (a *App) startMarkdownApplicationEdit(v *markdownViewState, action string, args markdowndoc.EditArgs, start, end int) error {
	if v.editPending {
		return fmt.Errorf("Markdown 正在处理上一项操作")
	}
	state := a.documentSessionState
	if state == nil || state.markdownDoc == nil {
		return application.ErrDocumentNotFound
	}
	source := v.text()
	if markdownEditorComposing(&v.editor) {
		return fmt.Errorf("请先完成输入法选字")
	}
	a.flushMarkdownDraft()
	if state.markdownDoc.Text() != source {
		return fmt.Errorf("Markdown 内容尚未同步，操作已取消")
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("Markdown 文档正在处理其他操作")
	}
	v.editPending = true
	window, shutdown, generation := a.window, a.shutdown, a.documentGeneration
	arguments := startMarkdownApplicationEditPayload{Action: action, Start: start, End: end, Label: args.Label, Destination: args.Destination, Display: args.Display}
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "markdown.text.edit", "markdown_document", token.documentID, "", arguments)
		state.finishOperation(token)
		editResult := markdownEditResult{state: state, view: v, source: source, action: action, generation: generation, err: err}
		if err == nil {
			editResult.data = append([]byte(nil), result.Data...)
		}
		select {
		case v.editResults <- editResult:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return nil
}

func (a *App) startMarkdownViewApplicationCommand(v *markdownViewState, mode string) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.markdownDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_markdown_document_is_busy_with_another_3c81c0"}, feedbackInfo, notificationSuppressed)
		return true
	}
	v.editPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		_, err := a.executeRibbonApplicationCommand(token, "markdown.view.set", "attached_window", fmt.Sprint(a.windowID), "", map[string]string{"mode": mode})
		state.finishOperation(token)
		result := markdownEditResult{state: state, view: v, action: "view", generation: generation, err: err}
		select {
		case v.editResults <- result:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

func (a *App) pollMarkdownEditResult(v *markdownViewState) {
	select {
	case result := <-v.editResults:
		v.editPending = false
		if result.action == "view" {
			if result.err != nil {
				if result.state == a.documentSessionState && result.generation == a.documentGeneration {
					a.setStatusErrorMessage(i18n.Message{ID: "markdown.error.operation_failed"}, result.err, notificationToast)
				}
				return
			}
			if result.state == a.documentSessionState && result.generation == a.documentGeneration {
				a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.markdown_view_changed_8d90cd"}, feedbackInfo, notificationSuppressed)
			}
			return
		}
		if result.err != nil {
			if result.state == a.documentSessionState && result.generation == a.documentGeneration {
				a.setStatusErrorMessage(i18n.Message{ID: "markdown.error.operation_failed"}, result.err, notificationToast)
			}
			return
		}
		if result.state == nil || result.state.markdownDoc == nil || result.view != v {
			return
		}
		var output struct {
			Changed   bool `json:"changed"`
			Selection struct {
				Start int `json:"start"`
				End   int `json:"end"`
			} `json:"selection"`
		}
		if err := json.Unmarshal(result.data, &output); err != nil {
			if result.state == a.documentSessionState && result.generation == a.documentGeneration {
				a.setStatusErrorMessage(i18n.Message{ID: "markdown.error.operation_failed"}, err, notificationToast)
			}
			return
		}
		current := result.state.markdownDoc.Text()
		if v.text() == result.source {
			v.editor.SetText(current)
			v.sourceSynced, v.observedDraft = current, current
			setMarkdownDraftState(result.state, v, false, false)
			v.cacheValid = false
			start, end := clampEditorSelection(output.Selection.Start, output.Selection.End, current)
			v.editor.SetCaret(start, end)
			v.focus = true
		}
		if output.Changed && result.state == a.documentSessionState && result.generation == a.documentGeneration {
			a.persistActiveSession()
			a.setStatusMessage(i18n.Message{ID: "markdown.status.applied"}, feedbackSuccess, notificationToast)
		}
	default:
	}
}
func (a *App) handleMarkdownAction(gtx layout.Context, id string) {
	v := a.ensureMarkdownView()
	switch id {
	case "mode-edit", "mode-preview", "mode-split":
		if v.editPending {
			return
		}
		if markdownEditorComposing(&v.editor) {
			return
		}
		a.flushMarkdownDraft()
		mode := map[string]string{"mode-edit": "edit", "mode-preview": "preview", "mode-split": "split"}[id]
		if a.startMarkdownViewApplicationCommand(v, mode) {
			return
		}
		v.mode = map[string]int{"edit": 0, "preview": 1, "split": 2}[mode]
		if v.mode == 0 {
			a.closeMarkdownCharts(v)
		}
		v.focus = v.mode != 1
		a.persistActiveSession()
	case "outline":
		v.collapsed = !v.collapsed
		a.persistActiveSession()
	case "open":
		a.startOpenFilePicker()
	case "save":
		a.flushMarkdownDraft()
		if markdownEditorComposing(&v.editor) {
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.finish_chinese_input_composition_first_b4284e"}, feedbackWarning, notificationSuppressed)
			return
		}
		if a.path == "" {
			a.startSaveAsPicker("md")
		} else {
			a.startSave()
		}
	case "save-as":
		a.flushMarkdownDraft()
		if !markdownEditorComposing(&v.editor) {
			a.startSaveAsPicker("md")
		}
	case "reveal-pdf":
		if v.lastPDFPath != "" {
			a.openMarkdownLink((&url.URL{Scheme: "file", Path: filepath.Dir(v.lastPDFPath)}).String())
		}
	case "export-pdf":
		a.requestMarkdownPDFExport()
	case "link", "image", "math":
		if id == "math" {
			if formula, ok := a.markdownFormulaAtSelection(); ok {
				a.showMarkdownFormulaEdit(formula)
				break
			}
		}
		a.showMarkdownInsertDialog(id)
	default:
		if strings.HasPrefix(id, "heading-") {
			for _, h := range v.snapshot.Headings {
				if id == fmt.Sprintf("heading-%d", h.Offset) {
					v.editor.SetCaret(h.Offset, h.Offset)
					v.preview.ScrollTo(h.Block)
					v.focus = v.mode != 1
					break
				}
			}
		} else if offset, ok := v.tasks[id]; ok {
			if err := a.toggleMarkdownTask(offset); err != nil {
				a.setStatusErrorMessage(i18n.Message{ID: "markdown.error.operation_failed"}, err, notificationToast)
			}
		} else if formula, ok := v.formulas[id]; ok {
			a.showMarkdownFormulaEdit(formula)
		} else if dest, ok := v.links[id]; ok {
			a.openMarkdownLink(dest)
		} else if err := a.applyMarkdownEdit(id, markdowndoc.EditArgs{}); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "markdown.error.operation_failed"}, err, notificationToast)
		}
	}
	if v.focus && v.mode != 1 {
		gtx.Execute(key.FocusCmd{Tag: &v.editor})
		v.focus = false
	}
}
func (a *App) openMarkdownLink(destination string) {
	if strings.HasPrefix(destination, "#") {
		needle := strings.TrimPrefix(destination, "#")
		for i, b := range a.markdownView.snapshot.Blocks {
			if b.Anchor == needle {
				a.markdownView.preview.ScrollTo(i)
				return
			}
		}
		for _, h := range a.markdownView.snapshot.Headings {
			if strings.ReplaceAll(strings.ToLower(h.Title), " ", "-") == needle {
				a.markdownView.preview.ScrollTo(h.Block)
				return
			}
		}
		return
	}
	target, local, err := markdownLinkTarget(destination, a.path)
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "markdown.error.operation_failed"}, err, notificationToast)
		return
	}
	if local && (strings.EqualFold(filepath.Ext(target), ".md") || strings.EqualFold(filepath.Ext(target), ".markdown")) {
		a.startOpen(target)
		return
	}
	a.launchManagedBackground(func() {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", "--", target)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
		default:
			cmd = exec.Command("xdg-open", target)
		}
		_ = cmd.Run()
	})
}

func markdownLinkTarget(destination, sourcePath string) (string, bool, error) {
	u, err := url.Parse(destination)
	if err != nil {
		return "", false, err
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return u.String(), false, nil
	case "file":
		if u.Host != "" && u.Host != "localhost" {
			return "", false, fmt.Errorf("不支持远程文件链接")
		}
	case "":
	default:
		return "", false, fmt.Errorf("不支持此链接协议：%s", u.Scheme)
	}
	target := u.Path
	if target == "" {
		return "", false, fmt.Errorf("链接路径为空")
	}
	if !filepath.IsAbs(target) {
		if sourcePath == "" {
			return "", false, fmt.Errorf("请先保存文档以解析相对链接")
		}
		target = filepath.Join(filepath.Dir(sourcePath), target)
	}
	absolute, err := filepath.Abs(target)
	return absolute, true, err
}
