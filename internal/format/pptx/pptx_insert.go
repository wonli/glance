package pptx

import (
	"fmt"
	"strings"

	"glance/internal/document"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

// AddTextBoxByID creates a single-paragraph text box on the explicitly
// identified slide. Coordinates and dimensions are expressed in points.
func (d *Document) AddTextBoxByID(slideID uint32, bounds document.PresentationRect, text string) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return 0, fmt.Errorf("演示文稿已关闭")
	}
	if err := validatePresentationTransform(bounds, 0); err != nil {
		return 0, err
	}
	if len([]rune(text)) > 32767 || strings.ContainsAny(text, "\r\n") {
		return 0, fmt.Errorf("新建文本框只支持最多 32767 个字符的单段落文本")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return 0, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for index, slide := range slides {
		if uint32(slide.ID()) != slideID {
			continue
		}
		shape, err := slide.AddTextBox(gopptx.TextBoxSpec{
			X: pointToEMU(bounds.X), Y: pointToEMU(bounds.Y),
			Width: pointToEMU(bounds.W), Height: pointToEMU(bounds.H), Text: text,
		})
		if err != nil {
			return 0, fmt.Errorf("新建文本框失败: %w", err)
		}
		d.ensureSlideProjectionCacheCount(len(slides))
		d.activeSlide = index
		d.dirty = true
		d.revision++
		return int64(shape.ID()), nil
	}
	return 0, fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

// AddAutoShapeByID creates a supported preset shape on the explicitly
// identified slide. Coordinates and dimensions are expressed in points.
func (d *Document) AddAutoShapeByID(slideID uint32, bounds document.PresentationRect, geometry, text string) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return 0, fmt.Errorf("演示文稿已关闭")
	}
	if err := validatePresentationTransform(bounds, 0); err != nil {
		return 0, err
	}
	switch geometry {
	case "rect", "roundRect", "ellipse":
	default:
		return 0, fmt.Errorf("暂不支持此自选图形: %q", geometry)
	}
	if len([]rune(text)) > 32767 || strings.ContainsAny(text, "\r\n") {
		return 0, fmt.Errorf("自选图形文本只支持最多 32767 个字符的单段落文本")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return 0, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for index, slide := range slides {
		if uint32(slide.ID()) != slideID {
			continue
		}
		shape, err := slide.AddAutoShape(gopptx.AutoShapeSpec{
			X: pointToEMU(bounds.X), Y: pointToEMU(bounds.Y),
			Width: pointToEMU(bounds.W), Height: pointToEMU(bounds.H),
			Geometry: geometry, Text: text,
		})
		if err != nil {
			return 0, fmt.Errorf("新建自选图形失败: %w", err)
		}
		d.ensureSlideProjectionCacheCount(len(slides))
		fillColor, borderColor, borderWidth := "#DCE7F7", "#4263EB", 1.5
		d.slideStyleChanges[index][uint32(shape.ID())] = document.PresentationElementStylePatch{
			FillColorHex: &fillColor, BorderColor: &borderColor, BorderWidthPT: &borderWidth,
		}
		d.activeSlide = index
		d.dirty = true
		d.revision++
		return int64(shape.ID()), nil
	}
	return 0, fmt.Errorf("未找到幻灯片 ID %d", slideID)
}
