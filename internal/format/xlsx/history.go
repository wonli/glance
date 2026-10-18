package xlsx

import (
	"bytes"

	"github.com/xuri/excelize/v2"
)

const workbookHistoryLimit = 100

type workbookHistoryEntry struct {
	before, after           []byte
	beforeState, afterState uint64
}

func (w *Workbook) snapshotLocked() ([]byte, error) {
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	buf, err := w.file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), buf.Bytes()...), nil
}

func (w *Workbook) recordHistoryLocked(before []byte) error {
	after, err := w.snapshotLocked()
	if err != nil {
		return err
	}
	return w.recordHistoryWithAfterLocked(before, after)
}

func (w *Workbook) recordHistoryWithAfterLocked(before, after []byte) error {
	if bytes.Equal(before, after) {
		return nil
	}
	entry := workbookHistoryEntry{before: before, after: after, beforeState: w.historyState, afterState: w.nextHistoryState}
	w.nextHistoryState++
	w.historyState = entry.afterState
	w.undo = append(w.undo, entry)
	if len(w.undo) > workbookHistoryLimit {
		w.undo = append([]workbookHistoryEntry(nil), w.undo[len(w.undo)-workbookHistoryLimit:]...)
	}
	w.redo = nil
	w.pictureIndexes = nil
	return nil
}

func (w *Workbook) Undo() (bool, error) { return w.applyHistory(true) }
func (w *Workbook) Redo() (bool, error) { return w.applyHistory(false) }

func (w *Workbook) applyHistory(undo bool) (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return false, err
	}
	from, to := &w.undo, &w.redo
	if !undo {
		from, to = &w.redo, &w.undo
	}
	if len(*from) == 0 {
		return false, nil
	}
	entry := (*from)[len(*from)-1]
	target := entry.before
	if !undo {
		target = entry.after
	}
	restored, err := excelize.OpenReader(bytes.NewReader(target))
	if err != nil {
		return false, err
	}
	(*from) = (*from)[:len(*from)-1]
	*to = append(*to, entry)
	previous := w.file
	w.file = restored
	w.pictureIndexes = nil
	w.invalidateStyleCacheLocked()
	if previous != nil {
		_ = previous.Close()
	}
	if undo {
		w.historyState = entry.beforeState
	} else {
		w.historyState = entry.afterState
	}
	w.dirty = w.historyState != w.savedHistoryState
	return true, nil
}

func (w *Workbook) RebaseHistory() {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.rebaseHistoryLocked()
}

func (w *Workbook) rebaseHistoryLocked() error {
	w.savedHistoryState = w.historyState
	w.dirty = false
	return nil
}
