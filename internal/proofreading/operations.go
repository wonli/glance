package proofreading

import (
	"errors"
	"unicode/utf8"
)

func (m *Model) ReplaceAtRevision(page PageID, text string, revision uint64) error {
	if revision != m.revision {
		return errors.New("stale proofreading edit")
	}
	return m.ReplacePage(page, text)
}
func (m *Model) ReplacePage(page PageID, text string) error {
	i := m.index(page)
	if i < 0 || !utf8.ValidString(text) {
		return errors.New("invalid page edit")
	}
	before := m.boxes[i]
	old := []rune(pieceString(before.root))
	next := []rune(text)
	if string(old) == text {
		return nil
	}
	prefix, suffix := 0, 0
	for prefix < len(old) && prefix < len(next) && old[prefix] == next[prefix] {
		prefix++
	}
	for suffix < len(old)-prefix && suffix < len(next)-prefix && old[len(old)-1-suffix] == next[len(next)-1-suffix] {
		suffix++
	}
	left, _ := pieceSplit(before.root, prefix)
	_, right := pieceSplit(before.root, len(old)-suffix)
	inserted := string(next[prefix : len(next)-suffix])
	middle := pieceFromRun(Run{First: m.next, Text: inserted, Typed: true, Protected: before.page.Independent()})
	after := before
	after.root = pieceConcat(pieceConcat(left, middle), right)
	m.commit([]boxChange{{i, before, after}}, m.next+CharacterID(len(next)-prefix-suffix))
	return nil
}
func (m *Model) Lock(page PageID, locked bool) error {
	i := m.index(page)
	if i < 0 || m.boxes[i].page.Reviewed {
		return errors.New("page cannot be locked")
	}
	var changes []boxChange
	for j, b := range m.boxes {
		after := b
		if j == i {
			after.page.Locked = locked
			mode := protectionFalse
			if locked {
				mode = protectionTrue
			}
			after.root = pieceFlagsIfChanged(b.root, 0, mode)
		} else if locked {
			after.page.Locked = false
		}
		if after != b {
			changes = append(changes, boxChange{j, b, after})
		}
	}
	m.commit(changes, m.next)
	return nil
}
func (m *Model) Review(page PageID, reviewed bool) error {
	i := m.index(page)
	if i < 0 {
		return errors.New("unknown page")
	}
	b := m.boxes[i]
	after := b
	after.page.Reviewed = reviewed
	if reviewed {
		after.page.Locked = false
	}
	if after != b {
		m.commit([]boxChange{{i, b, after}}, m.next)
	}
	return nil
}
func (m *Model) Split(page PageID, at int, up bool) error {
	i := m.index(page)
	j := i + 1
	if up {
		j = i - 1
	}
	if i < 0 || j < 0 || j >= len(m.boxes) || m.boxes[i].page.Reviewed || m.boxes[j].page.Reviewed || m.boxes[j].page.Locked {
		return errors.New("pagination blocked")
	}
	b, d := m.boxes[i], m.boxes[j]
	if at < 0 || at > pieceLen(b.root) {
		return errors.New("invalid caret")
	}
	left, right := pieceSplit(b.root, at)
	after, donor := b, d
	if up {
		after.root = right
		donor.root = pieceConcat(d.root, pieceFlags(left, 0, protectionFalse))
	} else {
		after.root = left
		donor.root = pieceConcat(pieceFlags(right, 0, protectionFalse), d.root)
	}
	mode := protectionFalse
	if b.page.Independent() {
		mode = protectionTrue
	}
	after.root = pieceFlagsIfChanged(after.root, 0, mode)
	m.commit([]boxChange{{i, b, after}, {j, d, donor}}, m.next)
	return nil
}

// Deprecated locked/reflow paths retain their old semantics via a validated
// conversion. Current direct editing and boundary adjustment never use this.
func (m *Model) Adjust(page PageID, count int) error {
	return m.legacy(func(l *legacyModel) error { return l.Adjust(page, count) }, true)
}
func (m *Model) ReturnableRunes(page PageID) int {
	l := &legacyModel{s: m.Snapshot()}
	return l.ReturnableRunes(page)
}
func (m *Model) Reflow(ends map[PageID]int) error {
	return m.legacy(func(l *legacyModel) error { return l.Reflow(ends) }, true)
}
