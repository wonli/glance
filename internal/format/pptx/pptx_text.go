package pptx

import (
	"errors"
	"fmt"
	"strings"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

var ErrSpeakerNotesConflict = errors.New("演讲者备注已被其他操作修改")

// UpdateSlideText updates the text of a specific shape on a slide.
func (d *Document) UpdateSlideText(slideIndex int, shapeID int64, newText string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.updateSlideTextLocked(slideIndex, shapeID, newText)
}

// UpdateSlideTextByID updates an element using the stable slide ID returned by
// the neutral presentation snapshot.
func (d *Document) UpdateSlideTextByID(slideID uint32, shapeID int64, newText string) error {
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
			return d.updateSlideTextLocked(index, shapeID, newText)
		}
	}
	return fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

func (d *Document) updateSlideTextLocked(slideIndex int, shapeID int64, newText string) error {
	if d.pres == nil {
		return fmt.Errorf("演示文稿已关闭")
	}

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
	return d.updateSlideNotesLocked(slideIndex, notes)
}

// UpdateSlideNotesByID resolves a stable slide ID before replacing its notes.
func (d *Document) UpdateSlideNotesByID(slideID uint32, notes string) error {
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
			return d.updateSlideNotesLocked(index, notes)
		}
	}
	return fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

// UpdateSlideNotesByIDIfCurrent writes notes only if the current text matches
// expectedNotes. It gives concurrent desktop and Agent edits compare-and-set
// semantics instead of silently replacing one another.
func (d *Document) UpdateSlideNotesByIDIfCurrent(slideID uint32, expectedNotes, notes string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return false, fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return false, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for index, slide := range slides {
		if uint32(slide.ID()) != slideID {
			continue
		}
		current, err := presentationSlideNotesText(slide)
		if err != nil {
			return false, err
		}
		if current != expectedNotes {
			return false, ErrSpeakerNotesConflict
		}
		if current == notes {
			return false, nil
		}
		if err := d.updateSlideNotesLocked(index, notes); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

func (d *Document) updateSlideNotesLocked(slideIndex int, notes string) error {
	if d.pres == nil {
		return fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil || slideIndex < 0 || slideIndex >= len(slides) {
		return fmt.Errorf("幻灯片序号无效: %d", slideIndex)
	}
	current, err := presentationSlideNotesText(slides[slideIndex])
	if err != nil {
		return err
	}
	if current == notes {
		return nil
	}

	if err := slides[slideIndex].SetSpeakerNotes(notes); err != nil {
		return fmt.Errorf("更新备注失败: %w", err)
	}
	d.dirty = true
	d.revision++
	return nil
}

func presentationSlideNotesText(slide *gopptx.Slide) (string, error) {
	frame, err := slide.SpeakerNotes()
	if err != nil {
		if errors.Is(err, gopptx.ErrNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("读取备注失败: %w", err)
	}
	paragraphs, err := frame.Paragraphs()
	if err != nil {
		return "", fmt.Errorf("读取备注失败: %w", err)
	}
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		line, err := paragraph.Text()
		if err != nil {
			return "", fmt.Errorf("读取备注失败: %w", err)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}
