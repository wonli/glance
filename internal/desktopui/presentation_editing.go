package desktopui

import (
	"fmt"

	"glance/internal/document"
)

// flushPresentationTextDraft commits the active canvas editor before a save or
// when the user switches to another shape. The editor text stays transient
// while typing so file-format work does not run on every Gio frame.
func (a *App) flushPresentationTextDraft() error {
	if a.pptxDoc == nil || a.pptxView == nil || a.pptxView.editingShape == 0 {
		return nil
	}
	v := a.pptxView
	text := v.shapeEditor.Text()
	var current string
	found := false
	if slide := v.currentSlide(); slide != nil {
		for _, elem := range slide.Elements {
			if elem.ID == v.editingShape && elem.Kind == document.ElementKindText {
				current = elem.PlainText()
				found = true
				break
			}
		}
	}
	if !found {
		return fmt.Errorf("找不到当前编辑的幻灯片文本框")
	}
	if current == text {
		v.shapeDraftText = text
		return nil
	}
	if err := a.pptxDoc.UpdateSlideText(v.activeSlide, v.editingShape, text); err != nil {
		a.setStatus("幻灯片文字更新失败：" + err.Error())
		return err
	}
	v.snapshot = a.pptxDoc.Snapshot()
	v.shapeDraftText = text
	a.setStatus("幻灯片文字已修改；点击保存写入文件")
	return nil
}

func presentationTextDraftDirty(v *presentationViewState) bool {
	if v == nil || v.editingShape == 0 {
		return false
	}
	if slide := v.currentSlide(); slide != nil {
		for _, elem := range slide.Elements {
			if elem.ID == v.editingShape && elem.Kind == document.ElementKindText {
				return v.shapeEditor.Text() != elem.PlainText()
			}
		}
	}
	return false
}
