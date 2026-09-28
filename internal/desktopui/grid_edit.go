package desktopui

import (
	"errors"
	"strconv"
	"strings"
)

var errNoSelectedCell = errors.New("no spreadsheet cell is selected")

type cellEditState struct {
	Address  string
	Original string
	Draft    string
	Editing  bool
}

func (s *cellEditState) selectCell(address, value string) {
	s.Address, s.Original, s.Draft, s.Editing = address, value, value, false
}
func (s *cellEditState) begin() bool {
	if s.Address == "" {
		return false
	}
	s.Draft, s.Editing = s.Original, true
	return true
}
func (s *cellEditState) setDraft(value string) { s.Draft = value }
func (s *cellEditState) commit(apply func(address, value string) error) error {
	if s.Address == "" {
		return errNoSelectedCell
	}
	if s.Draft == s.Original {
		s.Editing = false
		return nil
	}
	if err := apply(s.Address, s.Draft); err != nil {
		return err
	}
	s.Original, s.Editing = s.Draft, false
	return nil
}
func (s *cellEditState) cancel() { s.Draft = s.Original; s.Editing = false }

func parseCellAddress(address string) (row, column int, ok bool) {
	a := strings.ToUpper(strings.TrimSpace(address))
	i := 0
	for i < len(a) && a[i] >= 'A' && a[i] <= 'Z' {
		column = column*26 + int(a[i]-'A'+1)
		i++
	}
	if i == 0 || i == len(a) {
		return 0, 0, false
	}
	r, err := strconv.Atoi(a[i:])
	if err != nil || r < 1 || column < 1 {
		return 0, 0, false
	}
	return r, column, true
}
