package xlsx

import (
	"errors"
	"fmt"
	"io"

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
	var opts []fileutil.WriteOption
	if guard != nil {
		opts = append(opts, fileutil.WithCommitGuard(guard))
	}
	if err := fileutil.AtomicWrite(path, func(writer io.Writer) error {
		return w.file.Write(writer)
	}, opts...); err != nil {
		return fmt.Errorf("replace workbook: %w", err)
	}
	return nil
}
