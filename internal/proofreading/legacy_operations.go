package proofreading

import (
	"errors"
	"sort"
	"unicode/utf8"
)

func sortIntervals(v [][2]CharacterID) {
	sort.Slice(v, func(i, j int) bool { return v[i][0] < v[j][0] })
}
func (m *legacyModel) Adjust(page PageID, count int) error {
	i := m.index(page)
	if i < 0 || m.s.Pages[i].Reviewed {
		return errors.New("page is not being adjusted")
	}
	if count == 0 {
		return nil
	}
	before := clone(m.s)
	return m.transaction(func() error {
		moved := false
		if !m.s.Pages[i].Independent() {
			m.s.Pages[i].Manual = true
			for k := range m.s.Runs {
				if m.s.Runs[k].Page == page {
					m.s.Runs[k].Protected = true
					m.s.Runs[k].Origin = 0
				}
			}
		}
		_, end := m.bounds(page)
		if count > 0 {
			limit := end
			last := i
			for j := i + 1; j < len(m.s.Pages); j++ {
				if m.s.Pages[j].Independent() {
					break
				}
				last = j
			}
			for _, r := range m.s.Runs {
				j := m.index(r.Page)
				if j > last {
					break
				}
				if j > i {
					limit += utf8.RuneCountInString(r.Text)
				}
			}
			stop := min(end+count, limit)
			moved = stop > end
			a := m.cut(end)
			b := m.cut(stop)
			for k := a; k < b; k++ {
				r := &m.s.Runs[k]
				r.Origin = r.Page
				r.Page = page
				r.Protected = false
			}
		} else {
			start, _ := m.bounds(page)
			available, upper := m.adjustmentReturnCapacity(i)
			stop := max(start, end-min(-count, available))
			moved = stop < end
			a := m.cut(stop)
			b := m.cut(end)
			for k := a; k < b; k++ {
				r := &m.s.Runs[k]
				r.Page = m.s.Pages[min(m.index(r.Origin), upper)].ID
				r.Origin = 0
				r.Protected = false
			}
		}
		if !moved {
			m.s = before
		}
		return nil
	})
}

// ReturnableRunes reports the contiguous acquired suffix eligible to return.
// Controllers use this limit to avoid splitting a grapheme at a protected edge.
func (m *legacyModel) ReturnableRunes(page PageID) int {
	i := m.index(page)
	if i < 0 || m.s.Pages[i].Reviewed {
		return 0
	}
	available, _ := m.adjustmentReturnCapacity(i)
	return available
}

func (m *legacyModel) adjustmentReturnCapacity(i int) (int, int) {
	page := m.s.Pages[i].ID
	// Following ordinary pages may have reflowed since acquisition. Clamp
	// historical destinations to the first remaining following page so a
	// returned suffix always precedes that page's existing text.
	upper := len(m.s.Pages) - 1
	for _, r := range m.s.Runs {
		if j := m.index(r.Page); j > i {
			upper = j
			break
		}
	}
	available := 0
	for k := len(m.s.Runs) - 1; k >= 0; k-- {
		r := m.s.Runs[k]
		if r.Page != page {
			continue
		}
		if r.Protected || r.Origin == 0 || m.s.Pages[m.index(r.Origin)].Independent() {
			break
		}
		blocked := false
		for j := i + 1; j <= min(m.index(r.Origin), upper); j++ {
			if m.s.Pages[j].Independent() {
				blocked = true
				break
			}
		}
		if blocked {
			break
		}
		available += utf8.RuneCountInString(r.Text)
	}
	return available, upper
}

// Reflow applies layout's page-end offsets only inside ordinary contiguous page groups.
func (m *legacyModel) Reflow(ends map[PageID]int) error {
	return m.transaction(func() error {
		for i := 0; i < len(m.s.Pages); {
			if m.s.Pages[i].Independent() {
				i++
				continue
			}
			first := i
			for i < len(m.s.Pages) && !m.s.Pages[i].Independent() {
				i++
			}
			start, _ := m.bounds(m.s.Pages[first].ID)
			_, limit := m.bounds(m.s.Pages[i-1].ID)
			for j := first; j < i; j++ {
				end := ends[m.s.Pages[j].ID]
				if j == i-1 {
					end = limit
				}
				if end < start || end > limit {
					return errors.New("invalid reflow boundary")
				}
				a := m.cut(start)
				b := m.cut(end)
				for k := a; k < b; k++ {
					m.s.Runs[k].Page = m.s.Pages[j].ID
				}
				start = end
			}
		}
		return nil
	})
}
