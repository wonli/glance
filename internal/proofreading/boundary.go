package proofreading

import (
	"errors"
	"unicode/utf8"
)

// BoundaryReturnableRunes permits correcting an initially inferred boundary.
// Only explicit typing and reviewed pages stop a backwards adjustment.
func (m *Model) BoundaryReturnableRunes(page PageID) int {
	i := m.index(page)
	if i < 0 || i+1 >= len(m.s.Pages) || m.s.Pages[i].Reviewed || m.s.Pages[i+1].Reviewed {
		return 0
	}
	available := 0
	for k := len(m.s.Runs) - 1; k >= 0; k-- {
		r := m.s.Runs[k]
		if r.Page != page {
			continue
		}
		if r.Typed {
			break
		}
		available += utf8.RuneCountInString(r.Text)
	}
	return available
}

// AdjustBoundary moves the end of the active page continuously. Previously
// inferred or manually adjusted boundaries are editable; reviewed pages are not.
func (m *Model) AdjustBoundary(page PageID, count int) error {
	i := m.index(page)
	if i < 0 || m.s.Pages[i].Reviewed {
		return errors.New("page cannot be adjusted")
	}
	if count == 0 {
		return nil
	}
	start, end := m.bounds(page)
	left, right := end, end
	if count < 0 {
		left = end - min(-count, m.BoundaryReturnableRunes(page))
		left = max(start, left)
	} else {
		last := i
		if i+1 < len(m.s.Pages) && !m.s.Pages[i+1].Reviewed {
			last = i + 1
		}
		available := 0
		for _, r := range m.s.Runs {
			j := m.index(r.Page)
			if j > last {
				break
			}
			if j > i {
				available += utf8.RuneCountInString(r.Text)
			}
		}
		right = end + min(count, available)
	}
	if left == right {
		return nil
	}
	return m.transaction(func() error {
		a, b := m.cut(left), m.cut(right)
		m.s.Pages[i].Manual = true
		m.s.Pages[i].Locked = false
		for k := a; k < b; k++ {
			r := &m.s.Runs[k]
			if count > 0 {
				m.s.Pages[m.index(r.Page)].Manual = true
				r.Origin = r.Page
				r.Page = page
			} else {
				r.Page = m.s.Pages[i+1].ID
				r.Origin = 0
				m.s.Pages[i+1].Manual = true
			}
			r.Protected = r.Typed
		}
		return nil
	})
}
