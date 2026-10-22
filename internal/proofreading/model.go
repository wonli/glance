// Package proofreading owns editable copy content independently of file formats and UI.
package proofreading

import (
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"
)

type CharacterID uint64
type PageID uint64
type Run struct {
	First     CharacterID `json:"first"`
	Text      string      `json:"text"`
	Page      PageID      `json:"page"`
	Protected bool        `json:"protected,omitempty"`
	Typed     bool        `json:"typed,omitempty"`
	Origin    PageID      `json:"origin,omitempty"`
}
type Page struct {
	ID     PageID `json:"id"`
	Locked bool   `json:"locked,omitempty"`
	// Manual preserves an explicitly adjusted boundary without a user lock mode.
	Manual   bool `json:"manual,omitempty"`
	Reviewed bool `json:"reviewed,omitempty"`
}

func (p Page) Independent() bool { return p.Locked || p.Manual || p.Reviewed }

type Snapshot struct {
	Version  int         `json:"version"`
	Revision uint64      `json:"revision"`
	Next     CharacterID `json:"next"`
	Runs     []Run       `json:"runs"`
	Pages    []Page      `json:"pages"`
}
type Model struct {
	s          Snapshot
	undo, redo []Snapshot
	pageIndex  map[PageID]int
}

func New(text string, pages []PageID) (*Model, error) {
	if len(pages) == 0 || !utf8.ValidString(text) {
		return nil, errors.New("invalid proofreading input")
	}
	m := &Model{s: Snapshot{Version: 2, Next: CharacterID(utf8.RuneCountInString(text)) + 1}}
	for _, id := range pages {
		m.s.Pages = append(m.s.Pages, Page{ID: id})
	}
	if text != "" {
		m.s.Runs = []Run{{First: 1, Text: text, Page: pages[0]}}
	}
	return m, m.Validate()
}
func clone(s Snapshot) Snapshot {
	s.Runs = append([]Run(nil), s.Runs...)
	s.Pages = append([]Page(nil), s.Pages...)
	return s
}
func (m *Model) Snapshot() Snapshot { return clone(m.s) }
func (m *Model) Revision() uint64   { return m.s.Revision }
func (m *Model) Text(page PageID) string {
	b := []byte{}
	for _, r := range m.s.Runs {
		if r.Page == page {
			b = append(b, r.Text...)
		}
	}
	return string(b)
}
func (m *Model) IDs(page PageID) []CharacterID {
	var ids []CharacterID
	for _, r := range m.s.Runs {
		if r.Page == page {
			for i := range utf8.RuneCountInString(r.Text) {
				ids = append(ids, r.First+CharacterID(i))
			}
		}
	}
	return ids
}
func (m *Model) index(id PageID) int {
	if m.pageIndex == nil {
		m.pageIndex = make(map[PageID]int, len(m.s.Pages))
		for i, p := range m.s.Pages {
			m.pageIndex[p.ID] = i
		}
	}
	if i, ok := m.pageIndex[id]; ok {
		return i
	}
	return -1
}
func (m *Model) transaction(f func() error) error {
	before := clone(m.s)
	if err := f(); err != nil {
		m.s = before
		return err
	}
	if err := m.Validate(); err != nil {
		m.s = before
		return err
	}
	if slices.Equal(before.Runs, m.s.Runs) && slices.Equal(before.Pages, m.s.Pages) {
		return nil
	}
	m.undo = append(m.undo, before)
	m.redo = nil
	m.s.Revision++
	return nil
}
func (m *Model) Undo() bool {
	if len(m.undo) == 0 {
		return false
	}
	m.redo = append(m.redo, clone(m.s))
	r := m.s.Revision + 1
	m.s = m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	m.s.Revision = r
	return true
}
func (m *Model) Redo() bool {
	if len(m.redo) == 0 {
		return false
	}
	m.undo = append(m.undo, clone(m.s))
	r := m.s.Revision + 1
	m.s = m.redo[len(m.redo)-1]
	m.redo = m.redo[:len(m.redo)-1]
	m.s.Revision = r
	return true
}
func (m *Model) CanUndo() bool { return len(m.undo) > 0 }
func (m *Model) CanRedo() bool { return len(m.redo) > 0 }
func (m *Model) Validate() error {
	if m.s.Version != 2 || m.s.Next == 0 || len(m.s.Pages) == 0 {
		return errors.New("invalid proofreading version/identity")
	}
	seen := map[PageID]bool{}
	for _, p := range m.s.Pages {
		if p.ID == 0 || seen[p.ID] {
			return errors.New("duplicate page")
		}
		seen[p.ID] = true
	}
	intervals := make([][2]CharacterID, 0, len(m.s.Runs))
	last := -1
	for _, r := range m.s.Runs {
		n := utf8.RuneCountInString(r.Text)
		i := m.index(r.Page)
		if n == 0 || !utf8.ValidString(r.Text) || r.First == 0 || r.First+CharacterID(n) < r.First || r.First+CharacterID(n) > m.s.Next || i < last || i < 0 {
			return fmt.Errorf("invalid character run for page %d", r.Page)
		}
		if r.Origin != 0 && !seen[r.Origin] {
			return errors.New("invalid origin page")
		}
		last = i
		intervals = append(intervals, [2]CharacterID{r.First, r.First + CharacterID(n)})
	}
	// IDs may be out of numerical order after insertion; uniqueness is independent of text order.
	sortIntervals(intervals)
	for i := 1; i < len(intervals); i++ {
		if intervals[i][0] < intervals[i-1][1] {
			return errors.New("duplicate character identity")
		}
	}
	return nil
}
func (m *Model) cut(offset int) int {
	pos := 0
	for i, r := range m.s.Runs {
		n := utf8.RuneCountInString(r.Text)
		if offset == pos {
			return i
		}
		if offset > pos && offset < pos+n {
			rs := []rune(r.Text)
			left, right := r, r
			left.Text = string(rs[:offset-pos])
			right.Text = string(rs[offset-pos:])
			right.First += CharacterID(offset - pos)
			m.s.Runs = append(m.s.Runs, nilRun)
			copy(m.s.Runs[i+2:], m.s.Runs[i+1:])
			m.s.Runs[i] = left
			m.s.Runs[i+1] = right
			return i + 1
		}
		pos += n
	}
	return len(m.s.Runs)
}

var nilRun Run

func (m *Model) bounds(page PageID) (start, end int) {
	idx := m.index(page)
	for _, r := range m.s.Runs {
		i := m.index(r.Page)
		if i > idx {
			break
		}
		n := utf8.RuneCountInString(r.Text)
		if i < idx {
			start += n
			end += n
		} else if i == idx {
			end += n
		}
	}
	return
}
func (m *Model) AssignInitial(ends []int) error {
	if len(ends) != len(m.s.Pages) || len(m.undo) > 0 {
		return errors.New("invalid initial assignment")
	}
	before := clone(m.s)
	total := 0
	for _, r := range m.s.Runs {
		total += utf8.RuneCountInString(r.Text)
	}
	start := 0
	for i, p := range m.s.Pages {
		end := ends[i]
		if i == len(ends)-1 {
			end = total
		}
		if end < start || end > total {
			m.s = before
			return errors.New("invalid initial boundary")
		}
		a := m.cut(start)
		b := m.cut(end)
		for j := a; j < b; j++ {
			m.s.Runs[j].Page = p.ID
		}
		start = end
	}
	if err := m.Validate(); err != nil {
		m.s = before
		return err
	}
	return nil
}
