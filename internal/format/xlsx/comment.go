package xlsx

import (
	"fmt"
	"strings"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

var _ document.SpreadsheetCommentEditor = (*Workbook)(nil)

// GetComments returns all cell comments in the given worksheet.
func (w *Workbook) GetComments(sheet string) ([]document.SpreadsheetComment, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	if sheet == "" {
		sheet = w.file.GetSheetName(0)
	}

	comments, err := w.file.GetComments(sheet)
	if err != nil {
		return nil, fmt.Errorf("get comments for sheet %q: %w", sheet, err)
	}

	var out []document.SpreadsheetComment
	for _, c := range comments {
		text := c.Text
		if text == "" && len(c.Paragraph) > 0 {
			var sb strings.Builder
			for _, p := range c.Paragraph {
				sb.WriteString(p.Text)
			}
			text = sb.String()
		}
		out = append(out, document.SpreadsheetComment{
			Author: c.Author,
			Cell:   c.Cell,
			Text:   text,
		})
	}
	return out, nil
}

// AddComment adds or replaces a comment on the specified cell.
func (w *Workbook) AddComment(sheet string, comment document.SpreadsheetComment) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if sheet == "" {
		sheet = w.file.GetSheetName(0)
	}
	if comment.Cell == "" {
		return fmt.Errorf("cell coordinate cannot be empty")
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}

	// Remove previous comment on cell if any (excelize rejects adding duplicate comments)
	_ = w.file.DeleteComment(sheet, comment.Cell)

	author := comment.Author
	if author == "" {
		author = "Author"
	}

	opts := excelize.Comment{
		Cell:   comment.Cell,
		Author: author,
		Text:   comment.Text,
		Paragraph: []excelize.RichTextRun{
			{Text: comment.Text},
		},
		Width:  160,
		Height: 60,
	}

	if err := w.file.AddComment(sheet, opts); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return fmt.Errorf("add comment on %s: %w", comment.Cell, err)
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}

// DeleteComment removes the comment from the specified cell.
func (w *Workbook) DeleteComment(sheet string, cell string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if sheet == "" {
		sheet = w.file.GetSheetName(0)
	}
	if cell == "" {
		return fmt.Errorf("cell coordinate cannot be empty")
	}

	beforeSnapshot, err := w.snapshotLocked()
	if err != nil {
		return err
	}

	if err := w.file.DeleteComment(sheet, cell); err != nil {
		_ = w.restoreSnapshotLocked(beforeSnapshot)
		return fmt.Errorf("delete comment on %s: %w", cell, err)
	}

	if err := w.recordHistoryLocked(beforeSnapshot); err != nil {
		return err
	}
	w.dirty = true
	return nil
}
