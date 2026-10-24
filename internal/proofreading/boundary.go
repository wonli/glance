package proofreading

import (
	"errors"
)

// BoundaryReturnableRunes permits correcting an inferred boundary, stopping at
// locally typed content or a reviewed neighbor.
func (m *Model) BoundaryReturnableRunes(page PageID) int {
	i := m.index(page)
	if i < 0 || i+1 >= len(m.boxes) || m.boxes[i].page.Reviewed || m.boxes[i+1].page.Reviewed {
		return 0
	}
	return pieceReturnable(m.boxes[i].root)
}
func pieceReturnable(n *pieceTree) int {
	if n == nil {
		return 0
	}
	return n.returnable
}

// AdjustBoundary transfers piece references between adjacent fixed boxes. When
// the donor empties, later roots shift left without visiting their characters.
func (m *Model) AdjustBoundary(page PageID, count int) error {
	m.boundaryAffected = nil
	i := m.index(page)
	if i < 0 || m.boxes[i].page.Reviewed {
		return errors.New("page cannot be adjusted")
	}
	if count == 0 || i+1 >= len(m.boxes) || m.boxes[i+1].page.Reviewed {
		return nil
	}
	forward := count > 0
	b, d := m.boxes[i], m.boxes[i+1]
	after, donor := b, d
	if count > 0 {
		count = min(count, pieceLen(d.root))
		if count == 0 {
			return nil
		}
		moved, remain := pieceSplit(d.root, count)
		after.root = pieceConcat(b.root, pieceFlags(moved, d.page.ID, protectionTyped))
		donor.root = remain
	} else {
		count = min(-count, m.BoundaryReturnableRunes(page))
		if count == 0 {
			return nil
		}
		remain, moved := pieceSplit(b.root, pieceLen(b.root)-count)
		after.root = remain
		donor.root = pieceConcat(pieceFlags(moved, 0, protectionTyped), d.root)
	}
	after.page.Manual = true
	after.page.Locked = false
	donor.page.Manual = true
	changes := []boxChange{{i, b, after}}
	if forward && donor.root == nil {
		last := i + 1
		for last+1 < len(m.boxes) && !m.boxes[last+1].page.Reviewed {
			last++
		}
		for j := i + 1; j <= last; j++ {
			next := m.boxes[j]
			next.page.Manual = true
			if j < last {
				next.root = pieceFlags(m.boxes[j+1].root, 0, protectionKeep)
			} else {
				next.root = nil
			}
			changes = append(changes, boxChange{j, m.boxes[j], next})
		}
	} else {
		changes = append(changes, boxChange{i + 1, d, donor})
	}
	m.commit(changes, m.next)
	for _, c := range changes {
		m.boundaryAffected = append(m.boundaryAffected, c.after.page.ID)
	}
	return nil
}

// BoundaryAffectedPages reports pages touched by the latest boundary intent.
func (m *Model) BoundaryAffectedPages() []PageID { return append([]PageID(nil), m.boundaryAffected...) }

// PagePrefix returns bounded lookahead without flattening the rest of the book.
func (m *Model) PagePrefix(page PageID, limit int) (string, bool) {
	i := m.index(page)
	if i < 0 {
		return "", false
	}
	root := m.boxes[i].root
	truncated := pieceLen(root) > limit
	if truncated {
		root, _ = pieceSplit(root, limit)
	}
	return pieceString(root), truncated
}
