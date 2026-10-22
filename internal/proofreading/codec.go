package proofreading

import (
	"encoding/json"
	"errors"
)

func Restore(s Snapshot) (*Model, error) {
	m := &Model{s: clone(s)}
	if err := m.Validate(); err != nil {
		return nil, err
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
	undo, redo := m.undo, m.redo
	if err := m.Reflow(ends); err != nil {
		return err
	}
	m.undo, m.redo = undo, redo
	return nil
}
