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
	session  *sharedDocumentSession
}

func (s *cellEditState) selectCell(address, value string) {
	s.setEditing(false)
	s.Address, s.Original, s.Draft = address, value, value
}
func (s *cellEditState) begin() bool {
	if s.Address == "" {
		return false
	}
	s.Draft = s.Original
	s.setEditing(true)
	return true
}
func (s *cellEditState) setDraft(value string) { s.Draft = value }
func (s *cellEditState) commit(apply func(address, value string) error) error {
	if s.Address == "" {
		return errNoSelectedCell
	}
	if s.Draft == s.Original {
		s.setEditing(false)
		return nil
	}
	if err := apply(s.Address, s.Draft); err != nil {
		return err
	}
	s.Original = s.Draft
	s.setEditing(false)
	return nil
}
func (s *cellEditState) cancel() {
	s.Draft = s.Original
	s.setEditing(false)
}

func (s *cellEditState) setEditing(editing bool) {
	if s.Editing == editing {
		return
	}
	s.Editing = editing
	if s.session == nil {
		return
	}
	if editing {
		s.session.spreadsheetDraftViews.Add(1)
	} else {
		s.session.spreadsheetDraftViews.Add(-1)
	}
}

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
