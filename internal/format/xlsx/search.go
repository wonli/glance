package xlsx

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

var _ document.SpreadsheetSearcher = (*Workbook)(nil)

func (w *Workbook) SearchSheet(sheet, query string) ([]document.CellMatch, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	if !contains(w.file.GetSheetList(), sheet) {
		return nil, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	if query == "" {
		return nil, nil
	}
	pattern := "(?i)" + regexp.QuoteMeta(query)
	refs, err := w.file.SearchSheet(sheet, pattern, true)
	if err != nil {
		return nil, err
	}
	matches := make([]document.CellMatch, 0, len(refs))
	for _, ref := range refs {
		col, row, err := excelize.CellNameToCoordinates(ref)
		if err != nil {
			continue
		}
		val, _ := w.file.GetCellValue(sheet, ref)
		matches = append(matches, document.CellMatch{
			Row:    row,
			Column: col,
			Value:  val,
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Row != matches[j].Row {
			return matches[i].Row < matches[j].Row
		}
		return matches[i].Column < matches[j].Column
	})
	return matches, nil
}
