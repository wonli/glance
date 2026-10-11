package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

type presentationStyleOption struct {
	id    string
	label string
	patch document.PresentationElementStylePatch
}

func presentationSelectedStyleElement(v *presentationViewState) (int, document.PresentationElement, bool) {
	if v == nil || v.selectedShape == 0 {
		return 0, document.PresentationElement{}, false
	}
	slide := v.currentSlide()
	if slide == nil {
		return 0, document.PresentationElement{}, false
	}
	for _, elem := range slide.Elements {
		if elem.ID == v.selectedShape {
			return v.activeSlide, elem, true
		}
	}
	return 0, document.PresentationElement{}, false
}

func presentationSelectedRunStyle(v *presentationViewState, actionID string) bool {
	_, elem, ok := presentationSelectedStyleElement(v)
	if !ok {
		return false
	}
	var first *bool
	for _, paragraph := range elem.Paragraphs {
		for _, run := range paragraph.Runs {
			value := run.Bold
			switch actionID {
			case "home_italic":
				value = run.Italic
			case "home_underline":
				value = run.Underline
			}
			if first == nil {
				copy := value
				first = &copy
			} else if *first != value {
				return false
			}
		}
	}
	return first != nil && *first
}

func (a *App) handlePresentationStyleAction(v *presentationViewState, actionID string) bool {
	if v == nil {
		return false
	}
	switch actionID {
	case "home_font":
		v.stylePopover = togglePresentationStylePopover(v.stylePopover, "font")
	case "home_font_size":
		v.stylePopover = togglePresentationStylePopover(v.stylePopover, "font_size")
	case "home_font_color":
		v.stylePopover = togglePresentationStylePopover(v.stylePopover, "text_color")
	case "home_fill", "shape_fill":
		v.stylePopover = togglePresentationStylePopover(v.stylePopover, "fill")
	case "shape_outline", "picture_outline":
		v.stylePopover = togglePresentationStylePopover(v.stylePopover, "outline")
	case "shape_line_width", "picture_line_width":
		v.stylePopover = togglePresentationStylePopover(v.stylePopover, "line_width")
	case "home_line_spacing":
		v.stylePopover = togglePresentationStylePopover(v.stylePopover, "line_spacing")
	case "home_space_before":
		v.stylePopover = togglePresentationStylePopover(v.stylePopover, "space_before")
	case "home_space_after":
		v.stylePopover = togglePresentationStylePopover(v.stylePopover, "space_after")
	case "home_bold":
		return a.applyPresentationStylePatch(v, document.PresentationElementStylePatch{Bold: styleBool(!presentationSelectedRunStyle(v, actionID))})
	case "home_italic":
		return a.applyPresentationStylePatch(v, document.PresentationElementStylePatch{Italic: styleBool(!presentationSelectedRunStyle(v, actionID))})
	case "home_underline":
		return a.applyPresentationStylePatch(v, document.PresentationElementStylePatch{Underline: styleBool(!presentationSelectedRunStyle(v, actionID))})
	case "home_bullets":
		_, elem, ok := presentationSelectedStyleElement(v)
		if !ok || !elem.CanEditTextStyle || len(elem.Paragraphs) != 1 {
			return false
		}
		return a.applyPresentationStylePatch(v, document.PresentationElementStylePatch{Bullet: styleBool(!elem.Paragraphs[0].Bullet)})
	case "home_align_left":
		return a.applyPresentationStylePatch(v, document.PresentationElementStylePatch{ParagraphAlignment: styleString("left")})
	case "home_align_center":
		return a.applyPresentationStylePatch(v, document.PresentationElementStylePatch{ParagraphAlignment: styleString("center")})
	case "home_align_right":
		return a.applyPresentationStylePatch(v, document.PresentationElementStylePatch{ParagraphAlignment: styleString("right")})
	case "home_align_justify":
		return a.applyPresentationStylePatch(v, document.PresentationElementStylePatch{ParagraphAlignment: styleString("justify")})
	case "shape_no_fill":
		return a.applyPresentationStylePatch(v, document.PresentationElementStylePatch{FillColorHex: styleString("")})
	default:
		return false
	}
	if a.window != nil {
		a.window.Invalidate()
	}
	if v.stylePopover != "" {
		v.stylePopoverShapeID = v.selectedShape
	} else {
		v.stylePopoverShapeID = 0
	}
	return true
}

func togglePresentationStylePopover(current, target string) string {
	if current == target {
		return ""
	}
	return target
}

func (a *App) applyPresentationStylePatch(v *presentationViewState, patch document.PresentationElementStylePatch) bool {
	if a == nil || a.pptxDoc == nil || v == nil {
		return false
	}
	_, elem, ok := presentationSelectedStyleElement(v)
	if !ok {
		return false
	}
	slide := v.currentSlide()
	if slide == nil || slide.ID == 0 {
		return false
	}
	var err error
	state := a.documentSessionState
	if state != nil && state.pptxDoc == a.pptxDoc && a.applicationSessionRegistered(state.documentID) {
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			a.setStatus("演示文稿正在处理其他操作")
			return false
		}
		_, err = a.executeRibbonApplicationCommand(token, "presentation.element.style.set", "presentation_element", strconv.FormatInt(elem.ID, 10), "", presentationStyleArguments(slide.ID, elem.ID, patch))
		state.finishOperation(token)
	} else if state == nil || state.pptxDoc != a.pptxDoc {
		err = a.pptxDoc.UpdateSlideElementStyleByID(slide.ID, elem.ID, patch)
	} else {
		before := a.pptxDoc.Snapshot().Revision
		unlock := lockSessionAdapterWrite(state)
		err = a.pptxDoc.UpdateSlideElementStyleByID(slide.ID, elem.ID, patch)
		unlock()
		if err == nil && a.pptxDoc.Snapshot().Revision != before {
			a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load()+1)
		}
	}
	if err != nil {
		a.setStatus("样式修改失败：" + err.Error())
		return false
	}
	v.snapshot = a.pptxDoc.Snapshot()
	if v.editingShape == elem.ID {
		v.shapeDraftText = v.shapeEditor.Text()
	}
	a.setStatus("对象样式已修改；点击保存写入文件")
	if a.window != nil {
		a.window.Invalidate()
	}
	return true
}

func presentationStyleArguments(slideID uint32, elementID int64, patch document.PresentationElementStylePatch) map[string]any {
	arguments := map[string]any{"slide_id": slideID, "element_id": elementID}
	if patch.FontFamily != nil {
		arguments["font_family"] = *patch.FontFamily
	}
	if patch.FontSizePT != nil {
		arguments["font_size_pt"] = *patch.FontSizePT
	}
	if patch.Bold != nil {
		arguments["bold"] = *patch.Bold
	}
	if patch.Italic != nil {
		arguments["italic"] = *patch.Italic
	}
	if patch.Underline != nil {
		arguments["underline"] = *patch.Underline
	}
	if patch.TextColorHex != nil {
		arguments["text_color_hex"] = *patch.TextColorHex
	}
	if patch.ParagraphAlignment != nil {
		arguments["paragraph_alignment"] = *patch.ParagraphAlignment
	}
	if patch.Bullet != nil {
		arguments["bullet"] = *patch.Bullet
	}
	if patch.LineSpacing != nil {
		arguments["line_spacing"] = patch.LineSpacing
	}
	if patch.SpaceBefore != nil {
		arguments["space_before"] = patch.SpaceBefore
	}
	if patch.SpaceAfter != nil {
		arguments["space_after"] = patch.SpaceAfter
	}
	if patch.FillColorHex != nil {
		arguments["fill_color_hex"] = *patch.FillColorHex
	}
	if patch.BorderColor != nil {
		arguments["border_color"] = *patch.BorderColor
	}
	if patch.BorderWidthPT != nil {
		arguments["border_width_pt"] = *patch.BorderWidthPT
	}
	return arguments
}

func (a *App) presentationStyleOptions(v *presentationViewState) []presentationStyleOption {
	options := a.presentationStylePresetOptions(v)
	if len(options) > 0 {
		options = append(options, presentationStyleOption{id: "custom", label: "自定义…"})
	}
	return options
}

func (a *App) presentationStylePresetOptions(v *presentationViewState) []presentationStyleOption {
	colorOptions := func(prefix string, fill bool) []presentationStyleOption {
		values := []struct{ id, label, hex string }{
			{"black", "黑色", "#1F2937"}, {"blue", "蓝色", "#2563EB"}, {"red", "红色", "#DC2626"},
			{"green", "绿色", "#15803D"}, {"orange", "橙色", "#EA580C"}, {"purple", "紫色", "#7C3AED"},
		}
		options := make([]presentationStyleOption, 0, len(values)+1)
		for _, value := range values {
			colorValue := value.hex
			patch := document.PresentationElementStylePatch{TextColorHex: styleString(colorValue)}
			if fill {
				patch = document.PresentationElementStylePatch{FillColorHex: styleString(colorValue)}
			}
			if prefix == "outline" {
				patch = document.PresentationElementStylePatch{BorderColor: styleString(colorValue)}
			}
			options = append(options, presentationStyleOption{id: prefix + "_" + value.id, label: value.label, patch: patch})
		}
		if fill {
			options = append(options, presentationStyleOption{id: prefix + "_none", label: "无填充", patch: document.PresentationElementStylePatch{FillColorHex: styleString("")}})
		} else if prefix == "outline" {
			options = append(options, presentationStyleOption{id: prefix + "_none", label: "无轮廓", patch: document.PresentationElementStylePatch{BorderColor: styleString("")}})
		}
		return options
	}
	spacingOption := func(name, label, kind string, value float64) presentationStyleOption {
		spacing := &document.PresentationSpacing{Kind: kind, Value: value}
		patch := document.PresentationElementStylePatch{}
		switch name {
		case "line_spacing":
			patch.LineSpacing = spacing
		case "space_before":
			patch.SpaceBefore = spacing
		case "space_after":
			patch.SpaceAfter = spacing
		}
		return presentationStyleOption{id: fmt.Sprintf("%s_%.2f", name, value), label: label, patch: patch}
	}
	switch v.stylePopover {
	case "font":
		return []presentationStyleOption{
			{id: "font_aptos", label: "Aptos", patch: document.PresentationElementStylePatch{FontFamily: styleString("Aptos")}},
			{id: "font_arial", label: "Arial", patch: document.PresentationElementStylePatch{FontFamily: styleString("Arial")}},
			{id: "font_dengxian", label: "等线", patch: document.PresentationElementStylePatch{FontFamily: styleString("等线")}},
		}
	case "font_size":
		var options []presentationStyleOption
		for _, size := range []float64{12, 14, 18, 24, 28, 32, 36, 44} {
			options = append(options, presentationStyleOption{id: fmt.Sprintf("font_size_%.0f", size), label: fmt.Sprintf("%.0f pt", size), patch: document.PresentationElementStylePatch{FontSizePT: styleFloat(size)}})
		}
		return options
	case "text_color":
		return colorOptions("text_color", false)
	case "fill":
		return colorOptions("fill", true)
	case "outline":
		return colorOptions("outline", false)
	case "line_width":
		var options []presentationStyleOption
		for _, width := range []float64{0, 0.75, 1, 1.5, 2, 3} {
			options = append(options, presentationStyleOption{id: fmt.Sprintf("line_width_%.2f", width), label: fmt.Sprintf("%.2f pt", width), patch: document.PresentationElementStylePatch{BorderWidthPT: styleFloat(width)}})
		}
		return options
	case "line_spacing":
		return []presentationStyleOption{
			spacingOption("line_spacing", "1.0×", "pct", 1), spacingOption("line_spacing", "1.18×", "pct", 1.18),
			spacingOption("line_spacing", "1.5×", "pct", 1.5), spacingOption("line_spacing", "2.0×", "pct", 2),
			spacingOption("line_spacing", "18 pt", "pts", 18), spacingOption("line_spacing", "24 pt", "pts", 24),
			spacingOption("line_spacing", "30 pt", "pts", 30),
		}
	case "space_before", "space_after":
		options := make([]presentationStyleOption, 0, 5)
		for _, points := range []float64{0, 3, 6, 12, 18} {
			options = append(options, spacingOption(v.stylePopover, fmt.Sprintf("%.0f pt", points), "pts", points))
		}
		return options
	default:
		return nil
	}
}

func (a *App) layoutPresentationStylePopover(gtx layout.Context, v *presentationViewState) layout.Dimensions {
	if v == nil || v.stylePopover == "" {
		return layout.Dimensions{}
	}
	height := gtx.Dp(unit.Dp(48))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	paint.FillShape(gtx.Ops, color.NRGBA{R: 250, G: 251, B: 253, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
	paint.FillShape(gtx.Ops, pptxBorderColor, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, 1)}.Op())
	options := a.presentationStyleOptions(v)
	return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(5), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return v.styleOptionList.Layout(gtx, len(options), func(gtx layout.Context, index int) layout.Dimensions {
			option := options[index]
			button := v.styleOptionBtn(option.id)
			style := material.Button(a.theme, button, option.label)
			style.Background = color.NRGBA{R: 240, G: 242, B: 246, A: 255}
			style.Color = pptxTextDark
			if _, elem, ok := presentationSelectedStyleElement(v); ok && presentationStyleOptionMatches(elem, option.patch) {
				style.Background = toolbarActiveBackground
			}
			if button.Clicked(gtx) {
				if option.id == "custom" {
					a.showPresentationCustomStyleDialog(v, v.stylePopover)
					v.stylePopover = ""
				} else if a.applyPresentationStylePatch(v, option.patch) {
					v.stylePopover = ""
				}
			}
			return layout.Inset{Left: unit.Dp(3), Right: unit.Dp(3)}.Layout(gtx, style.Layout)
		})
	})
}

func presentationStyleOptionMatches(elem document.PresentationElement, p document.PresentationElementStylePatch) bool {
	hasRunStyle := p.FontFamily != nil || p.FontSizePT != nil || p.Bold != nil || p.Italic != nil || p.Underline != nil || p.TextColorHex != nil
	hasParagraphStyle := p.ParagraphAlignment != nil || p.Bullet != nil || p.LineSpacing != nil || p.SpaceBefore != nil || p.SpaceAfter != nil
	if !hasRunStyle && !hasParagraphStyle && p.FillColorHex == nil && p.BorderColor == nil && p.BorderWidthPT == nil {
		return false
	}
	runCount := 0
	for _, paragraph := range elem.Paragraphs {
		align := paragraph.Align
		if align == "" {
			align = "left"
		}
		if p.ParagraphAlignment != nil && align != *p.ParagraphAlignment {
			return false
		}
		if p.Bullet != nil && paragraph.Bullet != *p.Bullet {
			return false
		}
		for _, pair := range []struct {
			current, wanted *document.PresentationSpacing
			fallback        float64
		}{
			{paragraph.LineSpacing, p.LineSpacing, float64(presentationTextLineHeightScale)},
			{paragraph.SpaceBefore, p.SpaceBefore, 0}, {paragraph.SpaceAfter, p.SpaceAfter, 0},
		} {
			if pair.wanted == nil {
				continue
			}
			if pair.current == nil {
				if math.Abs(pair.wanted.Value-pair.fallback) > 1e-6 {
					return false
				}
			} else if pair.current.Kind != pair.wanted.Kind || math.Abs(pair.current.Value-pair.wanted.Value) > 1e-6 {
				return false
			}
		}
		for _, run := range paragraph.Runs {
			runCount++
			if p.FontFamily != nil && run.Font != *p.FontFamily ||
				p.FontSizePT != nil && math.Abs(run.SizePT-*p.FontSizePT) > 1e-6 ||
				p.Bold != nil && run.Bold != *p.Bold || p.Italic != nil && run.Italic != *p.Italic ||
				p.Underline != nil && run.Underline != *p.Underline || p.TextColorHex != nil && !strings.EqualFold(run.ColorHex, *p.TextColorHex) {
				return false
			}
		}
	}
	if hasRunStyle && runCount == 0 || hasParagraphStyle && len(elem.Paragraphs) == 0 {
		return false
	}
	if p.FillColorHex != nil && (elem.FillGradient != nil || !strings.EqualFold(elem.FillColorHex, *p.FillColorHex)) {
		return false
	}
	if p.BorderColor != nil && !strings.EqualFold(elem.BorderColor, *p.BorderColor) {
		return false
	}
	if p.BorderWidthPT != nil && math.Abs(elem.BorderWidth-*p.BorderWidthPT) > 1e-6 {
		return false
	}
	return true
}

func (a *App) showPresentationCustomStyleDialog(v *presentationViewState, kind string) {
	slide, elem, ok := presentationSelectedStyleElement(v)
	if !ok || a.pptxDoc == nil {
		return
	}
	doc, state := a.pptxDoc, a.documentSessionState
	labels := map[string]string{"font": "字体", "font_size": "字号（1–400 pt）", "text_color": "文字颜色（#RRGGBB）", "fill": "填充颜色（#RRGGBB）", "outline": "轮廓颜色（#RRGGBB）", "line_width": "轮廓宽度（0–20 pt）", "line_spacing": "行距（如 1.5×、150% 或 24 pt）", "space_before": "段前距（0–200 pt）", "space_after": "段后距（0–200 pt）"}
	label, supported := labels[kind]
	if !supported {
		return
	}
	a.modal.ShowInput("自定义"+label, label, "", label, func(action string) {
		draft := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" || a.pptxDoc != doc || a.documentSessionState != state || a.pptxView != v || v.activeSlide != slide || v.selectedShape != elem.ID {
			return
		}
		patch, err := presentationCustomStylePatch(kind, draft)
		if err != nil {
			a.setStatus("样式输入无效：" + err.Error())
			return
		}
		a.applyPresentationStylePatch(v, patch)
	}, modalDialogActionSpec{ID: "confirm", Label: "应用"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func presentationCustomStylePatch(kind, draft string) (document.PresentationElementStylePatch, error) {
	p := document.PresentationElementStylePatch{}
	value := strings.TrimSpace(draft)
	if kind == "font" {
		p.FontFamily = styleString(value)
		return p, nil
	}
	if kind == "text_color" || kind == "fill" || kind == "outline" {
		if !strings.HasPrefix(value, "#") {
			value = "#" + value
		}
		value = strings.ToUpper(value)
		switch kind {
		case "text_color":
			p.TextColorHex = &value
		case "fill":
			p.FillColorHex = &value
		case "outline":
			p.BorderColor = &value
		}
		return p, nil
	}
	unitKind := "pts"
	value = strings.ToLower(value)
	if kind == "line_spacing" {
		unitKind = "pct"
		if strings.HasSuffix(value, "pt") {
			unitKind = "pts"
		}
	}
	percent := strings.HasSuffix(value, "%")
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(value, "pt"), "%"), "x"), "×"))
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return p, fmt.Errorf("请输入有效数字")
	}
	if percent {
		number /= 100
	}
	switch kind {
	case "font_size":
		p.FontSizePT = &number
	case "line_width":
		p.BorderWidthPT = &number
	case "line_spacing":
		p.LineSpacing = &document.PresentationSpacing{Kind: unitKind, Value: number}
	case "space_before":
		p.SpaceBefore = &document.PresentationSpacing{Kind: "pts", Value: number}
	case "space_after":
		p.SpaceAfter = &document.PresentationSpacing{Kind: "pts", Value: number}
	default:
		return p, fmt.Errorf("不支持的样式选项")
	}
	return p, nil
}

func (v *presentationViewState) styleOptionBtn(id string) *widget.Clickable {
	if btn, ok := v.styleOptionBtns[id]; ok {
		return btn
	}
	btn := &widget.Clickable{}
	v.styleOptionBtns[id] = btn
	return btn
}

func styleString(value string) *string  { return &value }
func styleFloat(value float64) *float64 { return &value }
func styleBool(value bool) *bool        { return &value }
