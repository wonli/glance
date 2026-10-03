package desktopui

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/document"
)

const maxSpreadsheetImagePreviewBytes = 128 << 20

type gridPictureMetadataResult struct {
	state      *documentSessionState
	generation uint64
	sheet      string
	images     []document.SpreadsheetImage
	err        error
}

type gridPicturePreviewResult struct {
	state      *documentSessionState
	generation uint64
	sheet      string
	id         string
	version    uint64
	preview    image.Image
	err        error
}

type gridPictureCommandResult struct {
	state      *documentSessionState
	token      sessionOperationToken
	revision   uint64
	generation uint64
	sheet      string
	operation  string
	images     []document.SpreadsheetImage
	selectedID string
	mutated    bool
	err        error
}

type gridPictureCommandKind uint8

const (
	gridPictureCommandMove gridPictureCommandKind = iota + 1
	gridPictureCommandResize
	gridPictureCommandInsert
	gridPictureCommandReplace
	gridPictureCommandDelete
)

type gridPictureCommandRequest struct {
	kind           gridPictureCommandKind
	operation      string
	imageID        string
	placement      document.SpreadsheetImagePlacement
	widthPx        int
	target         *gridPicturePlacementTarget
	preview        *gridPictureGeometry
	data           []byte
	mediaType      string
	findFreeAnchor bool
}

func (a *App) requestGridPictureMetadata(state *documentSessionState, reader document.SpreadsheetImageReader, sheet string) {
	if state == nil || reader == nil || sheet == "" || state.closed.Load() || a.gridPictureMetaResults == nil {
		return
	}
	if state.gridPicturesSheet == sheet || (state.gridPicturesLoading && state.gridPicturesRequestedSheet == sheet) {
		return
	}
	if state.gridPicturesSheet != sheet {
		state.gridPictures = nil
		state.gridPicturesSheet = ""
		setSelectedGridPicture(state, "")
		state.gridPictureGesture.cancel()
		invalidateGridPicturePreviews(state)
	}
	state.gridPicturesRequestedSheet = sheet
	state.gridPicturesLoading = true
	state.gridPicturesError = ""
	generation := a.documentGeneration
	results := a.gridPictureMetaResults
	shutdown, window := a.shutdown, a.window
	go func() {
		unlock := lockSessionAdapterRead(state)
		images, err := reader.SpreadsheetImages(sheet)
		unlock()
		result := gridPictureMetadataResult{state: state, generation: generation, sheet: sheet, images: images, err: err}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func invalidateGridPicturePreviews(state *documentSessionState) {
	if state == nil {
		return
	}
	if state.gridPictureCache != nil {
		state.gridPictureCache.Clear()
	}
	if state.gridPictureVersions == nil {
		state.gridPictureVersions = make(map[string]uint64)
	}
	for id := range state.gridPicturePending {
		state.gridPictureVersions[id]++
	}
	state.gridPicturePending = make(map[string]uint64)
	state.gridPictureErrors = make(map[string]string)
}

func (a *App) invalidateGridPictures(state *documentSessionState) {
	if state == nil {
		return
	}
	state.gridPictures = nil
	state.gridPicturesSheet = ""
	state.gridPicturesRequestedSheet = ""
	state.gridPicturesLoading = false
	state.gridPicturesError = ""
	state.gridPictureGesture.cancel()
	invalidateGridPicturePreviews(state)
}

func (a *App) pollGridPictureResults() {
	for {
		select {
		case result := <-a.gridPictureMetaResults:
			a.applyGridPictureMetadata(result)
		default:
			goto previews
		}
	}

previews:
	for {
		select {
		case result := <-a.gridPictureResults:
			a.applyGridPicturePreview(result)
		default:
			goto commands
		}
	}

commands:
	for {
		select {
		case result := <-a.gridPictureCommands:
			a.applyGridPictureCommand(result)
		default:
			return
		}
	}
}

func (a *App) applyGridPictureMetadata(result gridPictureMetadataResult) {
	state := result.state
	if state == nil || state.closed.Load() {
		return
	}
	requested := state.gridPicturesRequestedSheet == result.sheet
	if result.generation != a.documentGeneration || state != a.documentSessionState || result.sheet != a.sheet || !requested {
		if requested {
			state.gridPicturesLoading = false
			state.gridPicturesRequestedSheet = ""
		}
		return
	}
	state.gridPicturesLoading = false
	state.gridPicturesRequestedSheet = ""
	state.gridPictureGesture.cancel()
	state.gridPictureCache.Clear()
	state.gridPicturePending = make(map[string]uint64)
	state.gridPictureErrors = make(map[string]string)
	if result.err != nil {
		state.gridPictures = nil
		state.gridPicturesSheet = result.sheet
		state.gridPicturesError = result.err.Error()
		if a.window != nil {
			a.status = "读取工作表图片失败：" + result.err.Error()
		}
	} else {
		state.gridPictures = append([]document.SpreadsheetImage(nil), result.images...)
		state.gridPicturesSheet = result.sheet
		state.gridPicturesError = ""
		if state.selectedGridPictureID != "" && !containsGridPicture(state.gridPictures, state.selectedGridPictureID) {
			setSelectedGridPicture(state, "")
		}
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) requestGridPicturePreview(state *documentSessionState, picture document.SpreadsheetImage) {
	if state == nil || state.closed.Load() || picture.ID == "" || state.gridPicturesSheet == "" {
		return
	}
	if _, ok := state.gridPictureCache.Get(picture.ID); ok || state.gridPictureErrors[picture.ID] != "" {
		return
	}
	if _, pending := state.gridPicturePending[picture.ID]; pending {
		return
	}
	reader, ok := any(state.book).(document.SpreadsheetImageReader)
	if !ok {
		state.gridPictureErrors[picture.ID] = "此工作簿不支持读取图片"
		return
	}
	switch picture.MediaType {
	case "image/png", "image/jpeg", "image/gif":
	default:
		state.gridPictureErrors[picture.ID] = picture.ReadOnlyReason
		if state.gridPictureErrors[picture.ID] == "" {
			state.gridPictureErrors[picture.ID] = "此图片格式暂不支持预览"
		}
		return
	}
	if state.gridPicturePending == nil {
		state.gridPicturePending = make(map[string]uint64)
	}
	if state.gridPictureVersions == nil {
		state.gridPictureVersions = make(map[string]uint64)
	}
	state.gridPictureVersions[picture.ID]++
	version := state.gridPictureVersions[picture.ID]
	state.gridPicturePending[picture.ID] = version
	results := a.gridPictureResults
	if results == nil {
		delete(state.gridPicturePending, picture.ID)
		return
	}
	generation, sheet, window := a.documentGeneration, state.gridPicturesSheet, a.window
	shutdown := a.shutdown
	go func() {
		unlock := lockSessionAdapterRead(state)
		data, err := reader.SpreadsheetImageBytes(picture.ID)
		unlock()
		var preview image.Image
		if err == nil {
			if len(data) > maxSpreadsheetImagePreviewBytes {
				err = fmt.Errorf("图片文件超过 %d MiB", maxSpreadsheetImagePreviewBytes>>20)
			} else {
				preview, err = decodeSpreadsheetPicturePreview(data)
			}
		}
		result := gridPicturePreviewResult{state: state, generation: generation, sheet: sheet, id: picture.ID, version: version, preview: preview, err: err}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func decodeSpreadsheetPicturePreview(data []byte) (image.Image, error) {
	if len(data) == 0 || len(data) > maxSpreadsheetImagePreviewBytes {
		return nil, fmt.Errorf("图片文件大小无效")
	}
	return decodeWordPreview(data)
}

func (a *App) applyGridPicturePreview(result gridPicturePreviewResult) {
	state := result.state
	if state == nil || state.closed.Load() {
		return
	}
	if version, pending := state.gridPicturePending[result.id]; pending && version == result.version {
		delete(state.gridPicturePending, result.id)
	}
	if result.generation != a.documentGeneration || state != a.documentSessionState || result.sheet != a.sheet || state.gridPicturesSheet != result.sheet || state.gridPictureVersions[result.id] != result.version || !containsGridPicture(state.gridPictures, result.id) {
		return
	}
	if result.err != nil {
		state.gridPictureErrors[result.id] = result.err.Error()
		if a.window != nil {
			a.status = "图片预览失败：" + result.err.Error()
		}
	} else if !state.gridPictureCache.Put(result.id, result.preview) {
		state.gridPictureErrors[result.id] = "图片无法放入工作簿预览缓存"
	} else {
		delete(state.gridPictureErrors, result.id)
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func containsGridPicture(pictures []document.SpreadsheetImage, id string) bool {
	for _, picture := range pictures {
		if picture.ID == id {
			return true
		}
	}
	return false
}

func activeGridPictures(state *documentSessionState, sheet string) []document.SpreadsheetImage {
	if state == nil || sheet == "" || state.gridPicturesSheet != sheet {
		return nil
	}
	return state.gridPictures
}

func setSelectedGridPicture(state *documentSessionState, imageID string) {
	if state == nil {
		return
	}
	state.selectedGridPictureID = imageID
	if imageID == "" {
		if state.toolbarTab == toolbarTabPicture {
			state.toolbarTab = toolbarTabHome
		}
		return
	}
	if state.toolbarTab != toolbarTabPicture {
		state.toolbarActionList.List.Position = layout.Position{}
	}
	state.toolbarTab = toolbarTabPicture
}

func gridPictureByID(pictures []document.SpreadsheetImage, id string) (document.SpreadsheetImage, bool) {
	for _, picture := range pictures {
		if picture.ID == id {
			return picture, true
		}
	}
	return document.SpreadsheetImage{}, false
}

func (a *App) startGridPictureMoveResize(state *documentSessionState, picture document.SpreadsheetImage, mode gridPictureGestureMode, target gridPicturePlacementTarget, preview gridPictureGeometry) {
	kind, operation := gridPictureCommandMove, "移动图片"
	if mode == gridPictureGestureResize {
		kind, operation = gridPictureCommandResize, "调整图片尺寸"
	}
	a.startGridPictureCommand(state, gridPictureCommandRequest{kind: kind, operation: operation, imageID: picture.ID, widthPx: target.WidthPx, target: &target, preview: &preview})
}

func (a *App) startGridPictureCommand(state *documentSessionState, request gridPictureCommandRequest) {
	if state == nil || state.formatRunning.Load() || a.isSaving() {
		if a.window != nil {
			a.status = "正在处理其他操作，请稍后重试"
			a.window.Invalidate()
		}
		return
	}
	editor, ok := any(state.book).(document.SpreadsheetImageEditor)
	if !ok {
		if a.window != nil {
			a.status = "当前工作簿不支持编辑图片"
			a.window.Invalidate()
		}
		return
	}
	if request.operation == "" {
		request.operation = pictureCommandOperationName(request.kind)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		if a.window != nil {
			a.status = "正在处理其他操作，请稍后重试"
			a.window.Invalidate()
		}
		return
	}
	state.gridPictureCommitID = request.imageID
	state.gridPictureCommitPreview = gridPictureGeometry{}
	state.gridPictureCommitActive = false
	if request.preview != nil {
		state.gridPictureCommitPreview = *request.preview
		state.gridPictureCommitActive = true
	}
	generation, sheet := a.documentGeneration, state.sheet
	results, shutdown, window := a.gridPictureCommands, a.shutdown, a.window
	go func() {
		result := gridPictureCommandResult{state: state, token: token, generation: generation, sheet: sheet, operation: request.operation}
		unlock := lockSessionAdapterWrite(state)
		switch request.kind {
		case gridPictureCommandMove:
			if request.target == nil {
				result.err = fmt.Errorf("missing picture placement")
			} else {
				target := request.target
				result.err = editor.MoveSpreadsheetImage(request.imageID, target.Row, target.Column, target.OffsetX, target.OffsetY)
			}
		case gridPictureCommandResize:
			result.err = editor.ResizeSpreadsheetImage(request.imageID, request.widthPx)
		case gridPictureCommandInsert:
			placement := request.placement
			if request.findFreeAnchor {
				reader, supported := any(state.book).(document.SpreadsheetImageReader)
				if !supported {
					result.err = fmt.Errorf("workbook cannot find an open picture anchor")
				} else {
					var existing []document.SpreadsheetImage
					existing, result.err = reader.SpreadsheetImages(sheet)
					if result.err == nil {
						placement.Row, placement.Column = nearbyFreeGridPictureAnchor(existing, placement.Row, placement.Column)
					}
				}
			}
			if result.err != nil {
				break
			}
			var imageID string
			imageID, result.err = editor.InsertSpreadsheetImage(placement, request.data)
			result.selectedID = imageID
			request.placement = placement
		case gridPictureCommandReplace:
			result.err = editor.ReplaceSpreadsheetImage(request.imageID, request.mediaType, request.data)
			result.selectedID = request.imageID
		case gridPictureCommandDelete:
			result.err = editor.DeleteSpreadsheetImage(request.imageID)
		default:
			result.err = fmt.Errorf("unknown picture operation")
		}
		if result.err == nil {
			result.mutated = true
			reader, supported := any(state.book).(document.SpreadsheetImageReader)
			if !supported {
				result.err = fmt.Errorf("workbook cannot refresh picture metadata")
			} else {
				result.images, result.err = reader.SpreadsheetImages(sheet)
			}
			if result.err == nil {
				switch request.kind {
				case gridPictureCommandInsert:
					if !containsGridPicture(result.images, result.selectedID) {
						result.selectedID = pictureIDAtCell(result.images, request.placement.Row, request.placement.Column)
					}
				case gridPictureCommandMove:
					if request.target != nil {
						result.selectedID = pictureIDAtCell(result.images, request.target.Row, request.target.Column)
					}
				case gridPictureCommandResize, gridPictureCommandReplace:
					if !containsGridPicture(result.images, result.selectedID) {
						result.selectedID = request.imageID
					}
				case gridPictureCommandDelete:
					result.selectedID = ""
				}
			}
		}
		unlock()
		if result.mutated {
			a.ensureDesktopRuntime().publishSessionRevision(token.documentID, token.revision+1)
		}
		state.finishOperation(token)
		result.revision = state.wordRevisionAtomic.Load()
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func pictureCommandOperationName(kind gridPictureCommandKind) string {
	switch kind {
	case gridPictureCommandMove:
		return "移动图片"
	case gridPictureCommandResize:
		return "调整图片尺寸"
	case gridPictureCommandInsert:
		return "插入图片"
	case gridPictureCommandReplace:
		return "替换图片"
	case gridPictureCommandDelete:
		return "删除图片"
	default:
		return "处理图片"
	}
}

func pictureIDAtCell(pictures []document.SpreadsheetImage, row, column int) string {
	for _, picture := range pictures {
		if picture.Row == row && picture.Column == column {
			return picture.ID
		}
	}
	return ""
}

func (a *App) applyGridPictureCommand(result gridPictureCommandResult) {
	state := result.state
	if state == nil || state.closed.Load() {
		return
	}
	state.gridPictureCommitID = ""
	state.gridPictureCommitPreview = gridPictureGeometry{}
	state.gridPictureCommitActive = false
	state.gridPictureGesture.cancel()
	if result.token.generation != 0 && (!state.acceptsResult(result.token) || state.wordRevisionAtomic.Load() != result.revision) {
		if result.mutated {
			a.invalidateGridPictures(state)
		}
		return
	}
	if result.generation != a.documentGeneration || state != a.documentSessionState || result.sheet != a.sheet {
		if result.mutated {
			a.invalidateGridPictures(state)
		}
		return
	}
	if result.err != nil {
		if result.mutated {
			a.invalidateGridPictures(state)
		}
		a.status = result.operation + "失败：" + result.err.Error()
	} else {
		state.gridPictures = append([]document.SpreadsheetImage(nil), result.images...)
		state.gridPicturesSheet = result.sheet
		state.gridPicturesRequestedSheet = ""
		state.gridPicturesLoading = false
		state.gridPicturesError = ""
		setSelectedGridPicture(state, result.selectedID)
		invalidateGridPicturePreviews(state)
		a.status = "已" + result.operation
		a.persistActiveSession()
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func paintGridPictures(gtx layout.Context, a *App, v *gridViewport) {
	state := a.documentSessionState
	if state == nil || state.gridPicturesSheet != state.sheet {
		return
	}
	rowHeader := gtx.Dp(unit.Dp(gridRowHeader))
	columnHeader := gtx.Dp(unit.Dp(gridColumnHeader))
	viewport := image.Rect(rowHeader, columnHeader, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
	for _, picture := range state.gridPictures {
		geometry := gridPictureGeometryForState(state, v, picture)
		bounds := gridPictureScreenRect(v, geometry, rowHeader, columnHeader)
		visible := gridPictureClipRect(bounds, viewport)
		if visible.Empty() {
			continue
		}
		preview, cached := state.gridPictureCache.Get(picture.ID)
		if !cached && state.gridPictureErrors[picture.ID] == "" {
			a.requestGridPicturePreview(state, picture)
		}
		paintOneGridPicture(gtx, a.theme, bounds, visible, preview, gridPicturePlaceholderMessage(state, picture))
	}
}

func paintOneGridPicture(gtx layout.Context, theme *material.Theme, bounds, viewport image.Rectangle, preview image.Image, placeholder string) {
	if bounds.Empty() || viewport.Empty() {
		return
	}
	viewportClip := clip.Rect(viewport).Push(gtx.Ops)
	defer viewportClip.Pop()
	transform := op.Offset(bounds.Min).Push(gtx.Ops)
	defer transform.Pop()
	size := bounds.Size()
	if preview == nil {
		background := color.NRGBA{R: 247, G: 249, B: 252, A: 255}
		paint.FillShape(gtx.Ops, background, clip.Rect{Max: size}.Op())
		paint.FillShape(gtx.Ops, color.NRGBA{R: 170, G: 184, B: 199, A: 255}, clip.Stroke{Path: clip.Rect(image.Rectangle{Max: size}).Path(), Width: 1}.Op())
		gtx.Constraints = layout.Constraints{Min: size, Max: size}
		label := material.Caption(theme, placeholder)
		label.Color = color.NRGBA{R: 84, G: 101, B: 119, A: 255}
		layout.Inset{Left: unit.Dp(5), Right: unit.Dp(5), Top: unit.Dp(3), Bottom: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
				layout.Stacked(label.Layout),
			)
		})
		return
	}
	gtx.Constraints = layout.Constraints{Min: size, Max: size}
	imageWidth := max(1, preview.Bounds().Dx())
	imageScale := float32(size.X) / float32(imageWidth) / max32(1, gtx.Metric.PxPerDp)
	widget.Image{Src: paint.NewImageOp(preview), Fit: widget.Contain, Position: layout.Center, Scale: imageScale}.Layout(gtx)
}

func gridPicturePlaceholderMessage(state *documentSessionState, picture document.SpreadsheetImage) string {
	if state != nil && state.gridPictureErrors[picture.ID] != "" {
		return "无法预览图片：" + state.gridPictureErrors[picture.ID]
	}
	if picture.ReadOnlyReason != "" && picture.MediaType == "" {
		return "图片无法预览：" + picture.ReadOnlyReason
	}
	return "正在载入图片…"
}

func paintGridPictureSelection(gtx layout.Context, a *App, v *gridViewport) {
	state := a.documentSessionState
	if state == nil || state.gridPicturesSheet != state.sheet || state.selectedGridPictureID == "" {
		return
	}
	var selected *document.SpreadsheetImage
	for index := range state.gridPictures {
		if state.gridPictures[index].ID == state.selectedGridPictureID {
			selected = &state.gridPictures[index]
			break
		}
	}
	if selected == nil {
		return
	}
	rowHeader := gtx.Dp(unit.Dp(gridRowHeader))
	columnHeader := gtx.Dp(unit.Dp(gridColumnHeader))
	viewport := image.Rect(rowHeader, columnHeader, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
	geometry := gridPictureGeometryForState(state, v, *selected)
	bounds := gridPictureClipRect(gridPictureScreenRect(v, geometry, rowHeader, columnHeader), viewport)
	if bounds.Empty() {
		return
	}
	line := max(1, gtx.Dp(unit.Dp(1)))
	blue := color.NRGBA{R: 38, G: 119, B: 211, A: 255}
	for _, edge := range selectionBorderRects(bounds, line) {
		edge = edge.Intersect(viewport)
		if !edge.Empty() {
			paint.FillShape(gtx.Ops, blue, clip.Rect(edge).Op())
		}
	}
	if selected.Editable {
		handleSize := max(6, gtx.Dp(unit.Dp(8)))
		handle := gridPictureClipRect(gridPictureHandleRect(bounds, handleSize), viewport)
		if !handle.Empty() {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect(handle).Op())
			inner := handle.Inset(max(1, line))
			if !inner.Empty() {
				paint.FillShape(gtx.Ops, blue, clip.Rect(inner).Op())
			}
		}
	}
}

func gridPictureResizeHandleSize(gtx layout.Context) int { return max(6, gtx.Dp(unit.Dp(8))) }

func gridPictureGeometryForState(state *documentSessionState, v *gridViewport, picture document.SpreadsheetImage) gridPictureGeometry {
	if state != nil {
		if state.gridPictureGesture.Mode != gridPictureGestureNone && state.gridPictureGesture.ImageID == picture.ID {
			return state.gridPictureGesture.Preview
		}
		if state.gridPictureCommitActive && state.gridPictureCommitID == picture.ID {
			return state.gridPictureCommitPreview
		}
	}
	return gridPictureContentGeometry(v, picture)
}
