package docx

import (
	"bytes"
	"fmt"
	"reflect"
)

const wordHistoryLimit = 100

type wordHistoryPart struct {
	value  part
	exists bool
}

type wordHistoryChange struct {
	before wordHistoryPart
	after  wordHistoryPart
}

type wordHistorySnapshot struct {
	parts map[string]wordHistoryPart
	order []string
	force bool
}

type wordHistoryEntry struct {
	changes                 map[string]wordHistoryChange
	beforeOrder, afterOrder []string
	beforeState, afterState uint64
}

func (d *Document) captureWordHistoryLocked(names ...string) wordHistorySnapshot {
	snapshot := wordHistorySnapshot{parts: make(map[string]wordHistoryPart, len(names)), order: packagePartNames(d.parts)}
	for _, name := range names {
		if _, exists := snapshot.parts[name]; exists {
			continue
		}
		value, exists := findPackagePart(d.parts, name)
		snapshot.parts[name] = wordHistoryPart{value: clonePackagePart(value), exists: exists}
	}
	return snapshot
}

func (d *Document) recordWordHistoryLocked(before wordHistorySnapshot) bool {
	changes := make(map[string]wordHistoryChange)
	for name, old := range before.parts {
		current, exists := findPackagePart(d.parts, name)
		newValue := wordHistoryPart{value: clonePackagePart(current), exists: exists}
		if sameHistoryPart(old, newValue) {
			continue
		}
		changes[name] = wordHistoryChange{before: old, after: newValue}
	}
	afterOrder := packagePartNames(d.parts)
	if len(changes) == 0 && equalStrings(before.order, afterOrder) && !before.force {
		return false
	}
	entry := wordHistoryEntry{changes: changes, beforeOrder: before.order, afterOrder: afterOrder, beforeState: d.historyState, afterState: d.nextHistoryState}
	d.nextHistoryState++
	d.historyState = entry.afterState
	d.undo = append(d.undo, entry)
	if len(d.undo) > wordHistoryLimit {
		d.undo = append([]wordHistoryEntry(nil), d.undo[len(d.undo)-wordHistoryLimit:]...)
	}
	d.redo = nil
	d.dirty = d.historyState != d.savedHistoryState
	return true
}

func (d *Document) Undo() (bool, error) { return d.applyWordHistory(true) }

func (d *Document) Redo() (bool, error) { return d.applyWordHistory(false) }

func (d *Document) CanUndo() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.closed && len(d.undo) != 0
}

func (d *Document) CanRedo() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.closed && len(d.redo) != 0
}

func (d *Document) applyWordHistory(undo bool) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return false, err
	}
	from, to := &d.undo, &d.redo
	if !undo {
		from, to = &d.redo, &d.undo
	}
	if len(*from) == 0 {
		return false, nil
	}
	entry := (*from)[len(*from)-1]
	if err := d.restoreWordHistoryLocked(entry, undo); err != nil {
		return false, err
	}
	*from = (*from)[:len(*from)-1]
	*to = append(*to, entry)
	if undo {
		d.historyState = entry.beforeState
	} else {
		d.historyState = entry.afterState
	}
	d.dirty = d.historyState != d.savedHistoryState
	return true, nil
}

func (d *Document) restoreWordHistoryLocked(entry wordHistoryEntry, undo bool) error {
	targetOrder := entry.beforeOrder
	if !undo {
		targetOrder = entry.afterOrder
	}
	current := make(map[string]part, len(d.parts)+len(entry.changes))
	for _, value := range d.parts {
		current[value.header.Name] = value
	}
	for name, change := range entry.changes {
		target := change.before
		if !undo {
			target = change.after
		}
		if target.exists {
			current[name] = clonePackagePart(target.value)
		} else {
			delete(current, name)
		}
	}
	parts := make([]part, 0, len(targetOrder))
	for _, name := range targetOrder {
		if value, ok := current[name]; ok {
			parts = append(parts, value)
			delete(current, name)
		}
	}
	if len(current) != 0 {
		return fmt.Errorf("history target is missing %d ordered DOCX package parts", len(current))
	}
	previousParts, previousDocumentIdx := d.parts, d.documentIdx
	d.parts = parts
	d.documentIdx = packagePartIndex(parts, "word/document.xml")
	if err := d.reparse(); err != nil {
		d.parts, d.documentIdx = previousParts, previousDocumentIdx
		if restoreErr := d.reparse(); restoreErr != nil {
			return fmt.Errorf("restore history failed (%v) and previous projection failed (%v)", err, restoreErr)
		}
		return fmt.Errorf("reparse DOCX history state: %w", err)
	}
	return nil
}

func packagePartNames(parts []part) []string {
	names := make([]string, len(parts))
	for i, value := range parts {
		names[i] = value.header.Name
	}
	return names
}

func packagePartIndex(parts []part, name string) int {
	for i, value := range parts {
		if value.header.Name == name {
			return i
		}
	}
	return -1
}

func findPackagePart(parts []part, name string) (part, bool) {
	index := packagePartIndex(parts, name)
	if index < 0 {
		return part{}, false
	}
	return parts[index], true
}

func clonePackagePart(value part) part {
	value.data = append([]byte(nil), value.data...)
	value.header.Extra = append([]byte(nil), value.header.Extra...)
	return value
}

func sameHistoryPart(a, b wordHistoryPart) bool {
	return a.exists == b.exists && (!a.exists || (reflect.DeepEqual(a.value.header, b.value.header) && bytes.Equal(a.value.data, b.value.data)))
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
