package desktopui

import (
	"path/filepath"
	"strings"
	"time"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
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
			&state.mergeModeButton, &state.splitModeButton,
			&state.mergeDraft.addButton, &state.mergeDraft.clearButton, &state.mergeDraft.outputButton, &state.mergeDraft.mergeButton,
			&state.splitDraft.clearButton, &state.splitDraft.selectButton, &state.splitDraft.outputButton, &state.splitDraft.rangesButton, &state.splitDraft.byPageButton, &state.splitDraft.splitButton,
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
				a.cancelPDFMergeSplitDrag(gtx, buttons)
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
	mergeModeClicked := state.mergeModeButton.Clicked(gtx)
	splitModeClicked := state.splitModeButton.Clicked(gtx)
	rangesModeClicked := state.splitDraft.rangesButton.Clicked(gtx)
	byPageModeClicked := state.splitDraft.byPageButton.Clicked(gtx)
	mergeAddClicked := state.mergeDraft.addButton.Clicked(gtx)
	mergeClearClicked := state.mergeDraft.clearButton.Clicked(gtx)
	splitSelectClicked := state.splitDraft.selectButton.Clicked(gtx)
	mergeFolderClicked := state.mergeDraft.outputButton.Clicked(gtx)
	mergeRunClicked := state.mergeDraft.mergeButton.Clicked(gtx)
	splitClearClicked := state.splitDraft.clearButton.Clicked(gtx)
	splitFolderClicked := state.splitDraft.outputButton.Clicked(gtx)
	splitRunClicked := state.splitDraft.splitButton.Clicked(gtx)
	removeClicked := make(map[uint64]bool, len(state.mergeDraft.fileButtons))
	for identity, buttons := range state.mergeDraft.fileButtons {
		removeClicked[identity] = buttons.remove.Clicked(gtx)
		buttons.grab.Clicked(gtx)
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
	if state.mode == pdfMergeSplitModeSplit && splitClearClicked {
		draft := &state.splitDraft
		draft.source = nil
		draft.rangeInput.SetText("")
		draft.outputFolder = ""
		draft.prefix.SetText("")
		draft.prefixDefault, draft.prefixLastSeen = "", ""
		draft.prefixCustomized = false
		a.updatePDFMergeSplitRangePreview()
		state.clearOperationResult()
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
			buttons.dragStartIndex = rowIndex
			buttons.dragTargetIndex = rowIndex
			buttons.dragPressVisualOffsetY, _ = pdfMergeSplitReorderOffsetAt(buttons, gtx.Now)
			buttons.dragOffsetY = buttons.dragPressVisualOffsetY
			buttons.dragDropStartedAt = time.Time{}
			buttons.dragStarted = false
		case pointer.Drag:
			step := float32(gtx.Dp(unit.Dp(80)))
			delta := pdfMergeSplitDragPointerDelta(event.Position.Y, buttons, step)
			threshold := float32(gtx.Dp(unit.Dp(3)))
			if delta*delta > threshold*threshold {
				if !buttons.dragStarted {
					buttons.dragLiftStartedAt = gtx.Now
					buttons.reorderOffsetY = 0
					buttons.reorderAnimationStart = time.Time{}
				}
				buttons.dragStarted = true
			}
			if buttons.dragStarted {
				target := clampPDFMergeSplitFileIndex(buttons.dragStartIndex+pdfMergeSplitDragSteps(delta, step), len(a.pdfMergeSplitTool.mergeDraft.files))
				if target != buttons.dragTargetIndex {
					a.updatePDFMergeSplitDragPreview(gtx, buttons, target, step)
				}
				buttons.dragOffsetY = buttons.dragPressVisualOffsetY + delta - float32(buttons.dragTargetIndex-buttons.dragStartIndex)*step
				gtx.Execute(op.InvalidateCmd{})
			}
		case pointer.Release:
			if buttons.dragStarted {
				step := float32(gtx.Dp(unit.Dp(80)))
				delta := pdfMergeSplitDragPointerDelta(event.Position.Y, buttons, step)
				target := clampPDFMergeSplitFileIndex(buttons.dragStartIndex+pdfMergeSplitDragSteps(delta, step), len(a.pdfMergeSplitTool.mergeDraft.files))
				if target != buttons.dragTargetIndex {
					a.updatePDFMergeSplitDragPreview(gtx, buttons, target, step)
				}
				buttons.dragOffsetY = buttons.dragPressVisualOffsetY + delta - float32(buttons.dragTargetIndex-buttons.dragStartIndex)*step
				from := a.pdfMergeSplitFileIndex(buttons.dragIdentity)
				a.movePDFMergeSplitFile(from, buttons.dragTargetIndex)
				buttons.reorderOffsetY = buttons.dragOffsetY
				buttons.reorderAnimationStart = gtx.Now
				buttons.dragStarted = false
				buttons.dragDropStartedAt = gtx.Now
				buttons.dragOffsetY = 0
				buttons.dragIdentity = 0
				gtx.Execute(op.InvalidateCmd{})
			} else {
				resumePDFMergeSplitReorderAnimation(buttons, buttons.dragPressVisualOffsetY, gtx.Now)
				buttons.dragIdentity = 0
			}
		case pointer.Cancel:
			a.cancelPDFMergeSplitDrag(gtx, buttons)
		}
	}
}

func pdfMergeSplitDragPointerDelta(positionY float32, buttons *pdfMergeSplitFileButtons, step float32) float32 {
	visualOffset := float32(buttons.dragTargetIndex-buttons.dragStartIndex)*step + buttons.dragOffsetY
	return positionY - buttons.dragPressY + visualOffset - buttons.dragPressVisualOffsetY
}

func pdfMergeSplitDragSteps(delta, step float32) int {
	if step <= 0 {
		return 0
	}
	if delta < 0 {
		return -int((-delta + step/2) / step)
	}
	return int((delta + step/2) / step)
}

func clampPDFMergeSplitFileIndex(index, count int) int {
	if count <= 0 {
		return 0
	}
	return max(0, min(count-1, index))
}

func pdfMergeSplitMoveFileSlice(files []pdfMergeSplitFile, from, to int) []pdfMergeSplitFile {
	moved := append([]pdfMergeSplitFile(nil), files...)
	if from < 0 || from >= len(moved) || to < 0 || to >= len(moved) || from == to {
		return moved
	}
	file := moved[from]
	if from < to {
		copy(moved[from:to], moved[from+1:to+1])
	} else {
		copy(moved[to+1:from+1], moved[to:from])
	}
	moved[to] = file
	return moved
}

func (a *App) updatePDFMergeSplitDragPreview(gtx layout.Context, buttons *pdfMergeSplitFileButtons, target int, step float32) {
	draft := &a.pdfMergeSplitTool.mergeDraft
	target = clampPDFMergeSplitFileIndex(target, len(draft.files))
	if buttons == nil || target == buttons.dragTargetIndex || len(draft.files) == 0 {
		return
	}
	currentOrder := pdfMergeSplitMoveFileSlice(draft.files, buttons.dragStartIndex, buttons.dragTargetIndex)
	nextOrder := pdfMergeSplitMoveFileSlice(draft.files, buttons.dragStartIndex, target)
	currentIndex := make(map[uint64]int, len(currentOrder))
	for index, file := range currentOrder {
		currentIndex[file.identity] = index
	}
	for nextIndex, file := range nextOrder {
		if file.identity == buttons.dragIdentity {
			continue
		}
		fileButtons := draft.fileButtons[file.identity]
		if fileButtons == nil {
			continue
		}
		oldIndex, found := currentIndex[file.identity]
		if !found {
			continue
		}
		currentOffset, _ := pdfMergeSplitReorderOffsetAt(fileButtons, gtx.Now)
		fromOffset := float32(oldIndex-nextIndex)*step + currentOffset
		fileButtons.reorderOffsetY = fromOffset
		if fromOffset == 0 {
			fileButtons.reorderAnimationStart = time.Time{}
		} else {
			fileButtons.reorderAnimationStart = gtx.Now
		}
	}
	buttons.dragTargetIndex = target
}

func (a *App) cancelPDFMergeSplitDrag(gtx layout.Context, buttons *pdfMergeSplitFileButtons) {
	if buttons == nil {
		return
	}
	if buttons.dragStarted {
		step := float32(gtx.Dp(unit.Dp(80)))
		oldTarget := buttons.dragTargetIndex
		returnOffset := float32(oldTarget-buttons.dragStartIndex)*step + buttons.dragOffsetY
		a.updatePDFMergeSplitDragPreview(gtx, buttons, buttons.dragStartIndex, step)
		buttons.reorderOffsetY = returnOffset
		buttons.reorderAnimationStart = gtx.Now
		buttons.dragStarted = false
		buttons.dragDropStartedAt = gtx.Now
		buttons.dragOffsetY = 0
		gtx.Execute(op.InvalidateCmd{})
	} else if buttons.dragIdentity != 0 {
		resumePDFMergeSplitReorderAnimation(buttons, buttons.dragPressVisualOffsetY, gtx.Now)
	}
	buttons.dragIdentity = 0
}

func resumePDFMergeSplitReorderAnimation(buttons *pdfMergeSplitFileButtons, offset float32, now time.Time) {
	buttons.reorderOffsetY = offset
	if offset == 0 {
		buttons.reorderAnimationStart = time.Time{}
	} else {
		buttons.reorderAnimationStart = now
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
