package proofreading

import (
	"encoding/json"
	"errors"
)

func Restore(s Snapshot) (*Model, error) {
	l := &legacyModel{s: s}
	if err := l.Validate(); err != nil {
		return nil, err
	}
	m := &Model{next: s.Next, revision: s.Revision, pageIndex: make(map[PageID]int, len(s.Pages))}
	for i, p := range s.Pages {
		m.pageIndex[p.ID] = i
		m.boxes = append(m.boxes, box{page: p})
	}
	for _, r := range s.Runs {
		i := m.index(r.Page)
		m.boxes[i].root = pieceConcat(m.boxes[i].root, pieceFromRun(r))
	}
	return m, nil
}
func Encode(s Snapshot) ([]byte, error) {
	if _, err := Restore(s); err != nil {
		return nil, err
	}
	return json.Marshal(s)
}
func Decode(data []byte) (Snapshot, error) {
	if len(data) > 128<<20 {
		return Snapshot{}, errors.New("proofreading copy exceeds size limit")
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	_, err := Restore(s)
	return s, err
}

// ReflowDerived is part of the preceding content operation, not a separate undo step.
func (m *Model) ReflowDerived(ends map[PageID]int) error {
	count := len(m.undo)
	redo := m.redo
	if err := m.Reflow(ends); err != nil {
		return err
	}
	if len(m.undo) == count {
		return nil
	}
	derived := m.undo[len(m.undo)-1]
	m.undo[len(m.undo)-1] = modelEdit{}
	m.undo = m.undo[:count]
	if count > 0 {
		previous := m.undo[count-1]
		changes := append([]boxChange(nil), previous.changes...)
		positions := make(map[int]int, len(changes))
		for i, c := range changes {
			positions[c.index] = i
		}
		for _, c := range derived.changes {
			if at, ok := positions[c.index]; ok {
				changes[at].after = c.after
			} else {
				positions[c.index] = len(changes)
				changes = append(changes, c)
			}
		}
		previous.changes = changes
		previous.afterNext = derived.afterNext
		m.undo[count-1] = previous
	}
	m.redo = redo
	return nil
}
