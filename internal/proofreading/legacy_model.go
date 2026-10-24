package proofreading

import (
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"
)

// legacyModel is used only while converting obsolete locked/reflow operations.
// Live content and history are owned by Model's per-box immutable roots.
type legacyModel struct {
	s          Snapshot
	undo, redo []Snapshot
	pageIndex  map[PageID]int
}

func clone(s Snapshot) Snapshot {
	s.Runs = append([]Run(nil), s.Runs...)
	s.Pages = append([]Page(nil), s.Pages...)
	return s
}
func (m *legacyModel) index(id PageID) int {
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
func (m *legacyModel) transaction(f func() error) error {
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
func (m *legacyModel) Validate() error {
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
func (m *legacyModel) cut(offset int) int {
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

func (m *legacyModel) bounds(page PageID) (start, end int) {
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
func (m *legacyModel) AssignInitial(ends []int) error {
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
