package pptx

import (
	"fmt"
	"strings"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

// UpdateSlideText updates the text of a specific shape on a slide.
func (d *Document) UpdateSlideText(slideIndex int, shapeID int64, newText string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	oldText, err := slideShapeText(d.pres, slideIndex, shapeID)
	if err != nil {
		return err
	}
	if oldText == newText {
		return nil
	}
	changed, err := updateShapeText(d.pres, slideIndex, shapeID, newText)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	d.dirty = true
	d.revision++
	return nil
}

func slideShapeText(pres *gopptx.Presentation, slideIndex int, shapeID int64) (string, error) {
	slides, err := pres.Slides()
	if err != nil || slideIndex < 0 || slideIndex >= len(slides) {
		return "", fmt.Errorf("幻灯片序号无效: %d", slideIndex)
	}
	shapes, err := slides[slideIndex].Shapes()
	if err != nil {
		return "", fmt.Errorf("读取幻灯片形状失败: %w", err)
	}
	for _, shape := range shapes {
		if int64(shape.ID()) != shapeID {
			continue
		}
		autoShape, ok := shape.(*gopptx.AutoShape)
		if !ok {
			return "", fmt.Errorf("形状不支持文本编辑")
		}
		frame, err := autoShape.TextFrame()
		if err != nil || frame == nil {
			return "", fmt.Errorf("形状不包含文本框")
		}
		paragraphs, err := frame.Paragraphs()
		if err != nil {
			return "", fmt.Errorf("读取文本段落失败: %w", err)
		}
		lines := make([]string, 0, len(paragraphs))
		for _, paragraph := range paragraphs {
			line, err := paragraph.Text()
			if err != nil {
				return "", fmt.Errorf("读取文本失败: %w", err)
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n"), nil
	}
	return "", fmt.Errorf("未找到形状 ID %d", shapeID)
}

func updateShapeText(pres *gopptx.Presentation, slideIndex int, shapeID int64, newText string) (bool, error) {
	slides, err := pres.Slides()
	if err != nil || slideIndex < 0 || slideIndex >= len(slides) {
		return false, fmt.Errorf("幻灯片序号无效: %d", slideIndex)
	}
	shapes, err := slides[slideIndex].Shapes()
	if err != nil {
		return false, fmt.Errorf("读取幻灯片形状失败: %w", err)
	}
	for _, shape := range shapes {
		if int64(shape.ID()) != shapeID {
			continue
		}
		autoShape, ok := shape.(*gopptx.AutoShape)
		if !ok {
			return false, fmt.Errorf("形状不支持文本编辑")
		}
		frame, err := autoShape.TextFrame()
		if err != nil || frame == nil {
			return false, fmt.Errorf("形状不包含文本框")
		}
		paragraphs, err := frame.Paragraphs()
		if err != nil {
			return false, fmt.Errorf("读取文本段落失败: %w", err)
		}
		if len(paragraphs) != 1 || strings.ContainsRune(newText, '\n') {
			return false, fmt.Errorf("当前只支持单段落文本框；不能增删或更改段落换行")
		}
		paragraph := paragraphs[0]
		oldLine, err := paragraph.Text()
		if err != nil {
			return false, fmt.Errorf("读取文本段落失败: %w", err)
		}
		runs, err := paragraph.Runs()
		if err != nil {
			return false, fmt.Errorf("读取文本格式失败: %w", err)
		}
		var runText strings.Builder
		for _, run := range runs {
			value, err := run.Text()
			if err != nil {
				return false, fmt.Errorf("读取文本格式失败: %w", err)
			}
			runText.WriteString(value)
		}
		if runText.String() != oldLine {
			return false, fmt.Errorf("此文本段含字段或特殊换行，暂不支持安全编辑")
		}
		if newText == oldLine {
			return false, nil
		}
		if oldLine == "" {
			if len(runs) > 0 {
				err = runs[0].SetText(newText)
			} else if newText != "" {
				_, err = paragraph.AddRun(newText, gopptx.FontStyle{})
			}
		} else if newText == "" {
			// Keep the existing styled runs empty instead of deleting them.
			// A later edit can then inherit the paragraph's original run style.
			for _, run := range runs {
				if err = run.SetText(""); err != nil {
					break
				}
			}
		} else {
			var result gopptx.ReplaceResult
			result, err = paragraph.ReplaceText(oldLine, newText)
			if err == nil && (result.Matches != 1 || result.Replaced != 1 || result.Skipped != 0) {
				err = fmt.Errorf("文本包含暂不支持安全替换的字段、链接或格式结构")
			}
		}
		if err != nil {
			return false, fmt.Errorf("设置文本失败: %w", err)
		}
		return true, nil
	}
	return false, fmt.Errorf("未找到形状 ID %d", shapeID)
}

// UpdateSlideNotes updates the speaker notes for a slide.
func (d *Document) UpdateSlideNotes(slideIndex int, notes string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	slides, err := d.pres.Slides()
	if err != nil || slideIndex < 0 || slideIndex >= len(slides) {
		return fmt.Errorf("幻灯片序号无效: %d", slideIndex)
	}

	if err := slides[slideIndex].SetSpeakerNotes(notes); err != nil {
		return fmt.Errorf("更新备注失败: %w", err)
	}
	d.dirty = true
	d.revision++
	return nil
}
