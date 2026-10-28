package desktopui

import (
	"path/filepath"
	"strings"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"glance/internal/application"
)

func (a *App) processPDFMergeSplitToolEvents(gtx layout.Context) {
	state := &a.pdfMergeSplitTool
	if !a.pdfMergeSplitToolPage {
		return
	}
	if state.busy {
		cancelClicked := consumePDFMergeSplitClick(&state.cancelButton, gtx)
		for _, button := range []*widget.Clickable{
			&state.backButton, &state.mergeModeButton, &state.splitModeButton,
			&state.mergeDraft.addButton, &state.mergeDraft.clearButton, &state.mergeDraft.outputButton, &state.mergeDraft.mergeButton,
			&state.splitDraft.selectButton, &state.splitDraft.outputButton, &state.splitDraft.rangesButton, &state.splitDraft.byPageButton, &state.splitDraft.splitButton,
		} {
			consumePDFMergeSplitClick(button, gtx)
		}
		for _, editor := range []*widget.Editor{&state.mergeDraft.outputName, &state.splitDraft.prefix, &state.splitDraft.rangeInput} {
			for _, ok := editor.Update(gtx); ok; _, ok = editor.Update(gtx) {
			}
		}
		for identity, buttons := range state.mergeDraft.fileButtons {
			consumePDFMergeSplitClick(&buttons.remove, gtx)
			if buttons.dragIdentity == identity {
				buttons.dragStarted = false
			}
			for _, ok := buttons.drag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; _, ok = buttons.drag.Update(gtx.Metric, gtx.Source, gesture.Both) {
			}
		}
		if cancelClicked && state.cancel != nil {
			state.cancel()
		}
		return
	}
	editors := []*widget.Editor{&state.mergeDraft.outputName}
	if state.mode == pdfMergeSplitModeSplit {
		editors = []*widget.Editor{&state.splitDraft.prefix, &state.splitDraft.rangeInput}
	}
	for _, editor := range editors {
		for _, ok := editor.Update(gtx); ok; _, ok = editor.Update(gtx) {
		}
	}
	if state.mode == pdfMergeSplitModeSplit {
		a.updatePDFMergeSplitRangePreview()
	}
	backClicked := state.backButton.Clicked(gtx)
	mergeModeClicked := state.mergeModeButton.Clicked(gtx)
	splitModeClicked := state.splitModeButton.Clicked(gtx)
	rangesModeClicked := state.splitDraft.rangesButton.Clicked(gtx)
	byPageModeClicked := state.splitDraft.byPageButton.Clicked(gtx)
	mergeAddClicked := state.mergeDraft.addButton.Clicked(gtx)
	mergeClearClicked := state.mergeDraft.clearButton.Clicked(gtx)
	splitSelectClicked := state.splitDraft.selectButton.Clicked(gtx)
	mergeFolderClicked := state.mergeDraft.outputButton.Clicked(gtx)
	mergeRunClicked := state.mergeDraft.mergeButton.Clicked(gtx)
	splitFolderClicked := state.splitDraft.outputButton.Clicked(gtx)
	splitRunClicked := state.splitDraft.splitButton.Clicked(gtx)
	removeClicked := make(map[uint64]bool, len(state.mergeDraft.fileButtons))
	for identity, buttons := range state.mergeDraft.fileButtons {
		removeClicked[identity] = buttons.remove.Clicked(gtx)
		buttons.grab.Clicked(gtx)
	}
	if backClicked {
		a.pdfMergeSplitToolPage = false
		a.cancelPDFMergeSplitJobForExit()
		state.clearOperationResult()
		return
	}
	if mergeModeClicked {
		state.selectMode(pdfMergeSplitModeMerge)
		state.clearOperationResult()
		return
	}
	if splitModeClicked {
		state.selectMode(pdfMergeSplitModeSplit)
		state.clearOperationResult()
		return
	}
	if state.mode == pdfMergeSplitModeSplit && rangesModeClicked {
		state.splitDraft.rangeMode = pdfMergeSplitSplitModeRanges
		state.clearOperationResult()
		a.updatePDFMergeSplitRangePreview()
		return
	}
	if state.mode == pdfMergeSplitModeSplit && byPageModeClicked {
		state.splitDraft.rangeMode = pdfMergeSplitSplitModeByPage
		state.clearOperationResult()
		a.updatePDFMergeSplitRangePreview()
		return
	}
	for index := range state.mergeDraft.files {
		file := state.mergeDraft.files[index]
		buttons := state.mergeDraft.fileButtons[file.identity]
		if buttons == nil {
			continue
		}
		if state.mode == pdfMergeSplitModeMerge && removeClicked[file.identity] {
			a.removePDFMergeSplitFile(file.identity)
			return
		}
		if state.mode == pdfMergeSplitModeMerge {
			a.processPDFMergeSplitDrag(gtx, index, buttons)
		}
	}
	if state.mode == pdfMergeSplitModeMerge && mergeClearClicked {
		state.mergeDraft.files = nil
		state.mergeDraft.fileButtons = make(map[uint64]*pdfMergeSplitFileButtons)
		a.recalculatePDFMergeSplitMergeSummary()
		state.clearOperationResult()
		return
	}
	if state.mode == pdfMergeSplitModeMerge && mergeAddClicked {
		state.clearOperationResult()
		a.startPDFMergeSplitFilePicker(pdfMergeSplitJobMergeFile)
		return
	}
	if state.mode == pdfMergeSplitModeSplit && splitSelectClicked {
		state.clearOperationResult()
		a.startPDFMergeSplitFilePicker(pdfMergeSplitJobSplitSource)
		return
	}
	if state.mode == pdfMergeSplitModeMerge {
		if mergeFolderClicked {
			a.startPDFMergeSplitFolderPicker(pdfMergeSplitJobMergeFolder, false)
			return
		}
		if mergeRunClicked {
			a.startPDFMergeOperation()
			return
		}
	} else {
		if splitFolderClicked {
			a.startPDFMergeSplitFolderPicker(pdfMergeSplitJobSplitFolder, false)
			return
		}
		if splitRunClicked {
			a.startPDFSplitOperation()
			return
		}
	}
}

func consumePDFMergeSplitClick(button *widget.Clickable, gtx layout.Context) bool {
	clicked := false
	for button.Clicked(gtx) {
		clicked = true
	}
	return clicked
}

func (a *App) processPDFMergeSplitDrag(gtx layout.Context, rowIndex int, buttons *pdfMergeSplitFileButtons) {
	if buttons == nil {
		return
	}
	for event, ok := buttons.drag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; event, ok = buttons.drag.Update(gtx.Metric, gtx.Source, gesture.Both) {
		switch event.Kind {
		case pointer.Press:
			buttons.dragPressY = event.Position.Y
			buttons.dragIdentity = a.pdfMergeSplitTool.mergeDraft.files[rowIndex].identity
			buttons.dragStarted = false
		case pointer.Drag:
			delta := event.Position.Y - buttons.dragPressY
			threshold := float32(gtx.Dp(unit.Dp(3)))
			if delta*delta > threshold*threshold {
				buttons.dragStarted = true
			}
		case pointer.Release:
			if buttons.dragStarted {
				from := a.pdfMergeSplitFileIndex(buttons.dragIdentity)
				delta := event.Position.Y - buttons.dragPressY
				rowHeight := float32(gtx.Dp(unit.Dp(54)))
				if delta < 0 {
					steps := -int((-delta + rowHeight/2) / rowHeight)
					a.movePDFMergeSplitFile(from, from+steps)
				} else {
					steps := int((delta + rowHeight/2) / rowHeight)
					a.movePDFMergeSplitFile(from, from+steps)
				}
			}
			buttons.dragStarted = false
		case pointer.Cancel:
			buttons.dragStarted = false
		}
	}
}

func (a *App) pdfMergeSplitFileIndex(identity uint64) int {
	for index, file := range a.pdfMergeSplitTool.mergeDraft.files {
		if file.identity == identity {
			return index
		}
	}
	return -1
}

func (a *App) appendPDFMergeSplitFile(info application.PDFUtilityFileInfo, merge bool) {
	state := &a.pdfMergeSplitTool
	file := pdfMergeSplitFile{path: info.Path, name: filepath.Base(info.Path), pageCount: info.PageCount, sizeBytes: info.SizeBytes}
	if merge {
		draft := &state.mergeDraft
		draft.nextIdentity++
		file.identity = draft.nextIdentity
		draft.files = append(draft.files, file)
		draft.fileButtons[file.identity] = &pdfMergeSplitFileButtons{}
		a.recalculatePDFMergeSplitMergeSummary()
		return
	}
	a.setPDFMergeSplitSource(info)
}

func (a *App) setPDFMergeSplitSource(info application.PDFUtilityFileInfo) {
	draft := &a.pdfMergeSplitTool.splitDraft
	file := pdfMergeSplitFile{path: info.Path, name: filepath.Base(info.Path), pageCount: info.PageCount, sizeBytes: info.SizeBytes}
	draft.source = &file
	base := strings.TrimSuffix(file.name, filepath.Ext(file.name))
	current := draft.prefix.Text()
	if !draft.prefixCustomized || current == draft.prefixDefault {
		draft.prefix.SetText(base)
		draft.prefixDefault = base
		draft.prefixLastSeen = base
		draft.prefixCustomized = false
	}
	a.updatePDFMergeSplitRangePreview()
}

func (a *App) updatePDFMergeSplitRangePreview() {
	draft := &a.pdfMergeSplitTool.splitDraft
	currentPrefix := draft.prefix.Text()
	if currentPrefix != draft.prefixLastSeen {
		draft.prefixCustomized = currentPrefix != draft.prefixDefault
		draft.prefixLastSeen = currentPrefix
	}
	draft.rangeCount, draft.rangeError = 0, ""
	if draft.source == nil {
		return
	}
	if draft.rangeMode == pdfMergeSplitSplitModeByPage {
		draft.rangeCount = draft.source.pageCount
		return
	}
	ranges, err := application.ParsePDFUtilityPageRanges(draft.rangeInput.Text(), draft.source.pageCount)
	if err != nil {
		draft.rangeError = a.textID("pdf_merge_split.validation.invalid_ranges")
		return
	}
	draft.rangeCount = len(ranges)
}

func (a *App) recalculatePDFMergeSplitMergeSummary() {
	draft := &a.pdfMergeSplitTool.mergeDraft
	draft.totalPageCount, draft.totalSizeBytes = 0, 0
	for _, file := range draft.files {
		draft.totalPageCount += file.pageCount
		draft.totalSizeBytes += file.sizeBytes
	}
}

func (a *App) removePDFMergeSplitFile(identity uint64) {
	draft := &a.pdfMergeSplitTool.mergeDraft
	for index := range draft.files {
		if draft.files[index].identity == identity {
			draft.files = append(draft.files[:index], draft.files[index+1:]...)
			delete(draft.fileButtons, identity)
			a.recalculatePDFMergeSplitMergeSummary()
			a.pdfMergeSplitTool.clearOperationResult()
			return
		}
	}
}

func (a *App) movePDFMergeSplitFile(from, to int) bool {
	draft := &a.pdfMergeSplitTool.mergeDraft
	if from < 0 || from >= len(draft.files) || to < 0 || to >= len(draft.files) || from == to {
		return false
	}
	file := draft.files[from]
	if from < to {
		copy(draft.files[from:to], draft.files[from+1:to+1])
	} else {
		copy(draft.files[to+1:from+1], draft.files[to:from])
	}
	draft.files[to] = file
	return true
}
