package desktopui

import (
	"fmt"
	"gioui.org/io/key"
	"gioui.org/layout"
	"glance/internal/format/markdowndoc"
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
	unlock()
	a.completeSessionOperation(a.documentSessionState, token, err, err == nil)
	if err == nil {
		v.sourceSynced = source
		a.persistActiveSession()
	} else {
		a.setStatus(err.Error())
	}
}
func (a *App) applyMarkdownEdit(action string, args markdowndoc.EditArgs) error {
	v := a.ensureMarkdownView()
	if markdownEditorComposing(&v.editor) {
		return fmt.Errorf("请先完成输入法选字")
	}
	start, end := v.editor.Selection()
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
func (a *App) handleMarkdownAction(gtx layout.Context, id string) {
	v := a.ensureMarkdownView()
	switch id {
	case "mode-edit", "mode-preview", "mode-split":
		if markdownEditorComposing(&v.editor) {
			return
		}
		a.flushMarkdownDraft()
		v.mode = map[string]int{"mode-edit": 0, "mode-preview": 1, "mode-split": 2}[id]
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
			a.setStatus("请先完成输入法选字")
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
				a.setStatus(err.Error())
			}
		} else if formula, ok := v.formulas[id]; ok {
			a.showMarkdownFormulaEdit(formula)
		} else if dest, ok := v.links[id]; ok {
			a.openMarkdownLink(dest)
		} else if err := a.applyMarkdownEdit(id, markdowndoc.EditArgs{}); err != nil {
			a.setStatus(err.Error())
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
		a.setStatus(err.Error())
		return
	}
	if local && (strings.EqualFold(filepath.Ext(target), ".md") || strings.EqualFold(filepath.Ext(target), ".markdown")) {
		a.startOpen(target)
		return
	}
	go func() {
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
	}()
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
