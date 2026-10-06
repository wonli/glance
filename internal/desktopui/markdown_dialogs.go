package desktopui

import (
	"glance/internal/format/markdowndoc"
)

func (a *App) showMarkdownInsertDialog(kind string) {
	v := a.ensureMarkdownView()
	v.dialogEpoch++
	epoch := v.dialogEpoch
	if markdownEditorComposing(&v.editor) {
		a.setStatus("请先完成输入法选字")
		return
	}
	state := a.documentSessionState
	source := v.text()
	start, end := v.editor.Selection()
	if start > end {
		start, end = end, start
	}
	selected := string([]rune(source)[start:end])
	labels, hints, initial := []string{"文字", "目标地址"}, []string{"显示文字", "https://… 或相对路径"}, []string{selected, ""}
	title := "插入链接"
	actions := []modalDialogActionSpec{{ID: "cancel", Label: "取消"}, {ID: "insert", Label: "插入"}}
	if kind == "image" {
		title = "插入图片"
		labels = []string{"替代文字", "图片路径"}
		hints[1] = "本地绝对路径或相对路径"
		actions = []modalDialogActionSpec{{ID: "cancel", Label: "取消"}, {ID: "browse", Label: "选择图片"}, {ID: "insert", Label: "插入"}}
	} else if kind == "math" {
		title = "插入公式"
		labels = []string{"公式"}
		hints = []string{`例如 \frac{a}{b} 或 x^2`}
		initial = []string{selected}
		actions = []modalDialogActionSpec{{ID: "cancel", Label: "取消"}, {ID: "inline", Label: "行内公式"}, {ID: "block", Label: "独立公式"}}
	}
	a.modal.ShowForm(title, "", labels, hints, initial, func(id string) {
		if id == "browse" {
			if v.imagePickPending {
				return
			}
			v.imagePickPending = true
			choose := a.wordImageFilePicker
			if choose == nil {
				choose = chooseOfficeImageFile
			}
			window, shutdown := a.window, a.shutdown
			go func() {
				path, err := choose()
				result := markdownImagePickResult{path: path, err: err, epoch: epoch}
				select {
				case v.imagePicks <- result:
					if window != nil {
						window.Invalidate()
					}
				case <-shutdown:
				}
			}()
			return
		}
		if id == "cancel" {
			a.modal.Hide()
			return
		}
		if a.documentSessionState != state || v.text() != source {
			a.modal.Hide()
			a.setStatus("文档内容已改变，插入已取消")
			return
		}
		args := markdowndoc.EditArgs{Label: a.modal.FormInputs[0].Text(), Display: id == "block"}
		if kind == "math" {
			if _, err := markdowndoc.RenderMath(args.Label, args.Display); err != nil {
				a.setStatus(err.Error())
				return
			}
			v.editor.SetCaret(start, end)
			out, err := markdowndoc.ApplyEdit(source, "math", start, end, args)
			if err != nil {
				a.setStatus(err.Error())
				return
			} // Formula text replaces the retained selection before wrapping.
			_ = out
			v.editor.Insert(args.Label)
			v.editor.SetCaret(start, start+len([]rune(args.Label)))
			args.Label = ""
		} else {
			args.Destination = a.modal.FormInputs[1].Text()
			if args.Destination == "" {
				a.setStatus("请输入目标地址")
				return
			}
			v.editor.SetCaret(start, end)
		}
		a.modal.Hide()
		if err := a.applyMarkdownEdit(kind, args); err != nil {
			a.setStatus(err.Error())
		}
	}, actions...)
}

func (a *App) pollMarkdownImagePicker() {
	v := a.ensureMarkdownView()
	select {
	case result := <-v.imagePicks:
		v.imagePickPending = false
		if !a.modal.Visible || result.epoch != v.dialogEpoch || a.modal.FormInputCount != 2 {
			return
		}
		if result.err != nil {
			a.setStatus(result.err.Error())
			return
		}
		if result.path != "" {
			a.modal.FormInputs[1].SetText(result.path)
		}
	default:
	}
}
