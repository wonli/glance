package xlsx

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"glance/internal/fileutil"
)

func (w *Workbook) Save() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if w.path == "" {
		return errors.New("workbook has no save path")
	}
	if err := w.saveTo(w.path); err != nil {
		return err
	}
	if err := w.rebaseHistoryLocked(); err != nil {
		w.dirty = true
		return err
	}
	return nil
}

// SaveAs writes an independent copy without changing the workbook's source path.
func (w *Workbook) SaveAs(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if err := w.saveTo(path); err != nil {
		return err
	}
	return nil
}

// SaveAsDocument writes and adopts a new source path for Save operations that follow.
func (w *Workbook) SaveAsDocument(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	if err := w.saveTo(path); err != nil {
		return err
	}
	w.path = path
	if err := w.rebaseHistoryLocked(); err != nil {
		w.dirty = true
		return err
	}
	return nil
}

// SaveAsWithCommitGuard writes a copy and checks guard after the temporary
// package has been fully written but immediately before it replaces path.
// A successful guard may return a release function that is called after the
// atomic replacement attempt completes.
func (w *Workbook) SaveAsWithCommitGuard(path string, guard func() (func(), error)) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return err
	}
	return w.saveToWithGuard(path, guard)
}

func (w *Workbook) saveTo(path string) error {
	return w.saveToWithGuard(path, nil)
}

func (w *Workbook) saveToWithGuard(path string, guard func() (func(), error)) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".glance-*.xlsx")
	if err != nil {
		return fmt.Errorf("create temporary workbook: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)
	if err := w.file.SaveAs(tmpPath); err != nil {
		return fmt.Errorf("write workbook: %w", err)
	}
	written, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	if err := written.Sync(); err != nil {
		_ = written.Close()
		return err
	}
	if err := written.Close(); err != nil {
		return err
	}
	var release func()
	if guard != nil {
		release, err = guard()
		if err != nil {
			return err
		}
	}
	if release != nil {
		defer release()
	}
	if err := fileutil.ReplaceFile(tmpPath, path); err != nil {
		return fmt.Errorf("replace workbook: %w", err)
	}
	return nil
}
