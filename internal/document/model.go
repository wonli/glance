package document

import "errors"

var ErrReadOnly = errors.New("document is read-only")

type Cell struct {
	Value   string
	StyleID int
}

type Spreadsheet interface {
	SheetNames() []string
	Range(sheet string, maxRows, maxColumns int) ([][]Cell, error)
	SetCellText(sheet, cell, value string) error
	Save() error
	Dirty() bool
	Editable() bool
	Close() error
}
