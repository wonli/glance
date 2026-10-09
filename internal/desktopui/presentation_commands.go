package desktopui

func (a *App) handlePresentationAddSlide(v *presentationViewState) {
	if a.pptxDoc == nil {
		return
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return
	}
	if err := a.pptxDoc.AddSlide(); err != nil {
		a.setStatus("新建幻灯片失败: " + err.Error())
		return
	}
	v.snapshot = a.pptxDoc.Snapshot()
	v.activeSlide = v.snapshot.SlideCount - 1
	v.selectedShape = 0
	if v.activeRibbonTab == "picture" {
		v.activeRibbonTab = "home"
	}
	v.editingShape = 0
	v.shapeDraftText = ""
	if cur := v.currentSlide(); cur != nil {
		v.notesEditor.SetText(cur.Notes)
	}
	a.setStatus("已新建幻灯片")
}

func (a *App) handlePresentationDeleteSlide(v *presentationViewState) {
	if a.pptxDoc == nil {
		return
	}
	if v.snapshot.SlideCount <= 1 {
		a.setStatus("无法删除演示文稿中的最后一页幻灯片")
		return
	}
	if err := a.pptxDoc.DeleteSlide(v.activeSlide); err != nil {
		a.setStatus("删除幻灯片失败: " + err.Error())
		return
	}
	v.snapshot = a.pptxDoc.Snapshot()
	if v.activeSlide >= v.snapshot.SlideCount {
		v.activeSlide = v.snapshot.SlideCount - 1
	}
	v.selectedShape = 0
	if v.activeRibbonTab == "picture" {
		v.activeRibbonTab = "home"
	}
	v.editingShape = 0
	v.shapeDraftText = ""
	if cur := v.currentSlide(); cur != nil {
		v.notesEditor.SetText(cur.Notes)
	}
	a.setStatus("已删除幻灯片")
}

func (a *App) handlePresentationAddTextBox(v *presentationViewState) {
	if a.pptxDoc == nil {
		return
	}
	a.setStatus("点击幻灯片画布上的文本框可直接编辑内容")
}
