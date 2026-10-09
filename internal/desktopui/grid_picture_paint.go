package desktopui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

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
