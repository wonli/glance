package pptx

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	gopptx "github.com/F31/go-pptx/v2/pptx"
	"glance/internal/document"
)

// UpdateSlideElementStyle records a slide-local style override. The override
// is projected immediately; serialization is handled by the PPTX adapter.
func (d *Document) UpdateSlideElementStyle(slideIndex int, shapeID int64, patch document.PresentationElementStylePatch) error {
	if presentationStylePatchEmpty(patch) {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.updateSlideElementStyleLocked(slideIndex, shapeID, patch)
}

// UpdateSlideElementStyleByID resolves a stable slide ID and applies the patch
// under the same document lock, so callers never need a UI's active slide.
func (d *Document) UpdateSlideElementStyleByID(slideID uint32, shapeID int64, patch document.PresentationElementStylePatch) error {
	if presentationStylePatchEmpty(patch) {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for index, slide := range slides {
		if uint32(slide.ID()) == slideID {
			return d.updateSlideElementStyleLocked(index, shapeID, patch)
		}
	}
	return fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

func (d *Document) updateSlideElementStyleLocked(slideIndex int, shapeID int64, patch document.PresentationElementStylePatch) error {
	if d.pres == nil {
		return fmt.Errorf("演示文稿已关闭")
	}

	if err := validatePresentationStylePatch(patch); err != nil {
		return err
	}
	if shapeID <= 0 || uint64(shapeID) > uint64(^uint32(0)) {
		return fmt.Errorf("形状 ID 无效: %d", shapeID)
	}
	slides, err := d.pres.Slides()
	if err != nil || slideIndex < 0 || slideIndex >= len(slides) {
		return fmt.Errorf("幻灯片序号无效: %d", slideIndex)
	}
	shapes, err := slides[slideIndex].Shapes()
	if err != nil {
		return fmt.Errorf("读取幻灯片形状失败: %w", err)
	}
	var selected gopptx.Shape
	count := 0
	for _, shape := range shapes {
		if int64(shape.ID()) == shapeID {
			selected = shape
			count++
		}
	}
	if count != 1 {
		if count == 0 {
			return fmt.Errorf("未找到幻灯片 %d 上的形状 ID %d", slideIndex, shapeID)
		}
		return fmt.Errorf("幻灯片 %d 上的形状 ID %d 不唯一", slideIndex, shapeID)
	}
	switch selected.(type) {
	case *gopptx.AutoShape, *gopptx.PictureShape:
	default:
		return fmt.Errorf("不支持修改幻灯片 %d 上形状 ID %d 的样式", slideIndex, shapeID)
	}
	if _, picture := selected.(*gopptx.PictureShape); picture && presentationStylePatchHasTextOrFill(patch) {
		return fmt.Errorf("图片仅支持修改轮廓样式")
	}
	if _, textShape := selected.(*gopptx.AutoShape); !textShape && presentationStylePatchHasText(patch) {
		return fmt.Errorf("目标对象不支持文字样式")
	}
	if presentationStylePatchHasText(patch) {
		projected := d.projectShape(selected, slideIndex)
		if len(projected.Paragraphs) == 0 {
			return fmt.Errorf("目标对象没有可编辑的文字段落")
		}
		autoShape, ok := selected.(*gopptx.AutoShape)
		if !ok {
			return fmt.Errorf("目标对象不支持文字样式")
		}
		if err := validateStyleTextTarget(autoShape); err != nil {
			return err
		}
	}
	// An explicit none can suppress an inherited/theme fill or line that is
	// absent from the projection; retain that intentional XML override.
	explicitNone := patch.FillColorHex != nil && *patch.FillColorHex == "" || patch.BorderColor != nil && *patch.BorderColor == ""
	if !explicitNone && validateProjectedPresentationStyle(d.projectShape(selected, slideIndex), patch) == nil {
		return nil
	}

	d.ensureSlideProjectionCacheCount(len(slides))
	id := uint32(shapeID)
	merged := mergePresentationStylePatch(d.slideStyleChanges[slideIndex][id], patch)
	if presentationStylePatchesEqual(d.slideStyleChanges[slideIndex][id], merged) {
		return nil
	}
	d.slideStyleChanges[slideIndex][id] = merged
	d.dirty = true
	d.revision++
	return nil
}

func validateStyleTextTarget(shape *gopptx.AutoShape) error {
	frame, err := shape.TextFrame()
	if err != nil || frame == nil {
		return fmt.Errorf("形状不包含可编辑文本框")
	}
	paragraphs, err := frame.Paragraphs()
	if err != nil {
		return fmt.Errorf("读取文本段落失败: %w", err)
	}
	if len(paragraphs) != 1 {
		return fmt.Errorf("当前仅支持单段落文本框样式")
	}
	paragraphText, err := paragraphs[0].Text()
	if err != nil {
		return fmt.Errorf("读取文本段落失败: %w", err)
	}
	runs, err := paragraphs[0].Runs()
	if err != nil {
		return fmt.Errorf("读取文本格式失败: %w", err)
	}
	var runText strings.Builder
	for _, run := range runs {
		text, err := run.Text()
		if err != nil {
			return fmt.Errorf("读取文本格式失败: %w", err)
		}
		runText.WriteString(text)
	}
	if runText.String() != paragraphText {
		return fmt.Errorf("此文本段含字段或特殊换行，暂不支持安全修改样式")
	}
	return nil
}

func presentationStylePatchEmpty(p document.PresentationElementStylePatch) bool {
	return p.FontFamily == nil && p.FontSizePT == nil && p.Bold == nil && p.Italic == nil && p.Underline == nil &&
		p.TextColorHex == nil && p.ParagraphAlignment == nil && p.Bullet == nil && p.LineSpacing == nil && p.SpaceBefore == nil &&
		p.SpaceAfter == nil && p.FillColorHex == nil && p.BorderColor == nil && p.BorderWidthPT == nil
}

func presentationStylePatchHasText(p document.PresentationElementStylePatch) bool {
	return p.FontFamily != nil || p.FontSizePT != nil || p.Bold != nil || p.Italic != nil || p.Underline != nil ||
		p.TextColorHex != nil || p.ParagraphAlignment != nil || p.Bullet != nil || p.LineSpacing != nil || p.SpaceBefore != nil || p.SpaceAfter != nil
}

func presentationStylePatchHasTextOrFill(p document.PresentationElementStylePatch) bool {
	return presentationStylePatchHasText(p) || p.FillColorHex != nil
}

func validatePresentationStylePatch(p document.PresentationElementStylePatch) error {
	if p.FontFamily != nil {
		if strings.TrimSpace(*p.FontFamily) == "" || len([]rune(*p.FontFamily)) > 128 || strings.IndexFunc(*p.FontFamily, unicode.IsControl) >= 0 {
			return fmt.Errorf("字体名称无效")
		}
	}
	if p.FontSizePT != nil && (!finite(*p.FontSizePT) || *p.FontSizePT < 1 || *p.FontSizePT > 400) {
		return fmt.Errorf("字体大小必须在 1 到 400 磅之间")
	}
	if p.TextColorHex != nil && !validPresentationHexColor(*p.TextColorHex, false) {
		return fmt.Errorf("文字颜色必须为 #RRGGBB")
	}
	if p.ParagraphAlignment != nil {
		switch *p.ParagraphAlignment {
		case "left", "center", "right", "justify":
		default:
			return fmt.Errorf("段落对齐方式无效: %q", *p.ParagraphAlignment)
		}
	}
	if p.LineSpacing != nil {
		if err := validatePresentationSpacing(*p.LineSpacing, true); err != nil {
			return fmt.Errorf("行距无效: %w", err)
		}
	}
	for name, spacing := range map[string]*document.PresentationSpacing{"段前距": p.SpaceBefore, "段后距": p.SpaceAfter} {
		if spacing != nil {
			if err := validatePresentationSpacing(*spacing, false); err != nil {
				return fmt.Errorf("%s无效: %w", name, err)
			}
		}
	}
	if p.FillColorHex != nil && !validPresentationHexColor(*p.FillColorHex, true) {
		return fmt.Errorf("填充颜色必须为 #RRGGBB 或空值")
	}
	if p.BorderColor != nil && !validPresentationHexColor(*p.BorderColor, true) {
		return fmt.Errorf("轮廓颜色必须为 #RRGGBB 或空值")
	}
	if p.BorderWidthPT != nil && (!finite(*p.BorderWidthPT) || *p.BorderWidthPT < 0 || *p.BorderWidthPT > 20) {
		return fmt.Errorf("轮廓宽度必须在 0 到 20 磅之间")
	}
	return nil
}

func validatePresentationSpacing(s document.PresentationSpacing, line bool) error {
	if !finite(s.Value) {
		return fmt.Errorf("数值无效")
	}
	switch s.Kind {
	case "pct":
		min := 0.0
		if line {
			min = 0.5
		}
		if s.Value < min || s.Value > 4 {
			return fmt.Errorf("百分比值超出范围")
		}
	case "pts":
		min := 0.0
		if line {
			min = 1
		}
		if s.Value < min || s.Value > 200 {
			return fmt.Errorf("磅值超出范围")
		}
	default:
		return fmt.Errorf("不支持的单位 %q", s.Kind)
	}
	return nil
}

func validPresentationHexColor(value string, allowEmpty bool) bool {
	if allowEmpty && value == "" {
		return true
	}
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, char := range value[1:] {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func mergePresentationStylePatch(base, update document.PresentationElementStylePatch) document.PresentationElementStylePatch {
	merged := clonePresentationStylePatch(base)
	if update.FontFamily != nil {
		merged.FontFamily = cloneString(update.FontFamily)
	}
	if update.FontSizePT != nil {
		merged.FontSizePT = cloneFloat(update.FontSizePT)
	}
	if update.Bold != nil {
		merged.Bold = cloneBool(update.Bold)
	}
	if update.Italic != nil {
		merged.Italic = cloneBool(update.Italic)
	}
	if update.Underline != nil {
		merged.Underline = cloneBool(update.Underline)
	}
	if update.TextColorHex != nil {
		merged.TextColorHex = cloneString(update.TextColorHex)
	}
	if update.ParagraphAlignment != nil {
		merged.ParagraphAlignment = cloneString(update.ParagraphAlignment)
	}
	if update.Bullet != nil {
		merged.Bullet = cloneBool(update.Bullet)
	}
	if update.LineSpacing != nil {
		merged.LineSpacing = cloneSpacing(update.LineSpacing)
	}
	if update.SpaceBefore != nil {
		merged.SpaceBefore = cloneSpacing(update.SpaceBefore)
	}
	if update.SpaceAfter != nil {
		merged.SpaceAfter = cloneSpacing(update.SpaceAfter)
	}
	if update.FillColorHex != nil {
		merged.FillColorHex = cloneString(update.FillColorHex)
	}
	if update.BorderColor != nil {
		merged.BorderColor = cloneString(update.BorderColor)
	}
	if update.BorderWidthPT != nil {
		merged.BorderWidthPT = cloneFloat(update.BorderWidthPT)
	}
	return merged
}

func clonePresentationStylePatch(p document.PresentationElementStylePatch) document.PresentationElementStylePatch {
	return document.PresentationElementStylePatch{
		FontFamily:         cloneString(p.FontFamily),
		FontSizePT:         cloneFloat(p.FontSizePT),
		Bold:               cloneBool(p.Bold),
		Italic:             cloneBool(p.Italic),
		Underline:          cloneBool(p.Underline),
		TextColorHex:       cloneString(p.TextColorHex),
		ParagraphAlignment: cloneString(p.ParagraphAlignment),
		Bullet:             cloneBool(p.Bullet),
		LineSpacing:        cloneSpacing(p.LineSpacing),
		SpaceBefore:        cloneSpacing(p.SpaceBefore),
		SpaceAfter:         cloneSpacing(p.SpaceAfter),
		FillColorHex:       cloneString(p.FillColorHex),
		BorderColor:        cloneString(p.BorderColor),
		BorderWidthPT:      cloneFloat(p.BorderWidthPT),
	}
}

func presentationStylePatchesEqual(a, b document.PresentationElementStylePatch) bool {
	return equalStringPtr(a.FontFamily, b.FontFamily) && equalFloatPtr(a.FontSizePT, b.FontSizePT) &&
		equalBoolPtr(a.Bold, b.Bold) && equalBoolPtr(a.Italic, b.Italic) && equalBoolPtr(a.Underline, b.Underline) &&
		equalStringPtr(a.TextColorHex, b.TextColorHex) && equalStringPtr(a.ParagraphAlignment, b.ParagraphAlignment) && equalBoolPtr(a.Bullet, b.Bullet) &&
		equalSpacingPtr(a.LineSpacing, b.LineSpacing) && equalSpacingPtr(a.SpaceBefore, b.SpaceBefore) &&
		equalSpacingPtr(a.SpaceAfter, b.SpaceAfter) && equalStringPtr(a.FillColorHex, b.FillColorHex) &&
		equalStringPtr(a.BorderColor, b.BorderColor) && equalFloatPtr(a.BorderWidthPT, b.BorderWidthPT)
}

func applyPresentationElementStyle(elem *document.PresentationElement, p document.PresentationElementStylePatch) {
	for paraIndex := range elem.Paragraphs {
		para := &elem.Paragraphs[paraIndex]
		if p.ParagraphAlignment != nil {
			para.Align = *p.ParagraphAlignment
		}
		if p.LineSpacing != nil {
			para.LineSpacing = cloneSpacing(p.LineSpacing)
		}
		if p.SpaceBefore != nil {
			para.SpaceBefore = cloneSpacing(p.SpaceBefore)
		}
		if p.SpaceAfter != nil {
			para.SpaceAfter = cloneSpacing(p.SpaceAfter)
		}
		if p.Bullet != nil {
			para.Bullet = *p.Bullet
		}
		for runIndex := range para.Runs {
			run := &para.Runs[runIndex]
			if p.FontFamily != nil {
				run.Font = *p.FontFamily
			}
			if p.FontSizePT != nil {
				run.SizePT = *p.FontSizePT
			}
			if p.Bold != nil {
				run.Bold = *p.Bold
			}
			if p.Italic != nil {
				run.Italic = *p.Italic
			}
			if p.Underline != nil {
				run.Underline = *p.Underline
			}
			if p.TextColorHex != nil {
				run.ColorHex = *p.TextColorHex
			}
		}
	}
	if p.FillColorHex != nil {
		elem.FillColorHex = *p.FillColorHex
		elem.FillGradient = nil
	}
	if p.BorderColor != nil {
		elem.BorderColor = *p.BorderColor
	}
	if p.BorderWidthPT != nil {
		elem.BorderWidth = *p.BorderWidthPT
	}
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneSpacing(value *document.PresentationSpacing) *document.PresentationSpacing {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func equalStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func equalFloatPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func equalBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func equalSpacingPtr(a, b *document.PresentationSpacing) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
