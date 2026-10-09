package docx

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"glance/internal/fileutil"
)

func (d *Document) packageBytes() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, item := range d.parts {
		header := item.header
		writer, err := zw.CreateHeader(&header)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := writer.Write(item.data); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (d *Document) Save() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.path == "" {
		return errors.New("document has no save path")
	}
	if _, err := d.saveAsSnapshot(d.path, nil); err != nil {
		return err
	}
	d.savedHistoryState = d.historyState
	d.dirty = false
	return nil
}

// SaveWithSnapshot atomically saves the active package and returns the exact
// serialized bytes written to the document path.
func (d *Document) SaveWithSnapshot() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return nil, err
	}
	if d.path == "" {
		return nil, errors.New("document has no save path")
	}
	snapshot, err := d.saveAsSnapshot(d.path, nil)
	if err != nil {
		return nil, err
	}
	d.savedHistoryState = d.historyState
	d.dirty = false
	return snapshot, nil
}

func (d *Document) SaveAs(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	_, err := d.saveAsSnapshot(path, nil)
	return err
}

// SaveAsWithCommitGuard writes a copy and checks guard after the temporary
// package has been fully written but immediately before it replaces path.
// A successful guard may return a release function that is called after the
// atomic replacement attempt completes.
func (d *Document) SaveAsWithCommitGuard(path string, guard func() (func(), error)) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	_, err := d.saveAsSnapshot(path, guard)
	return err
}

// SaveAsDocument writes and adopts a new source path for subsequent Save calls.
func (d *Document) SaveAsDocument(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if _, err := d.saveAsSnapshot(path, nil); err != nil {
		return err
	}
	d.path, d.dirty = path, false
	d.savedHistoryState = d.historyState
	return nil
}

// SaveAsDocumentWithSnapshot adopts path only after successfully writing the
// package, and returns the exact bytes written there.
func (d *Document) SaveAsDocumentWithSnapshot(path string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return nil, err
	}
	snapshot, err := d.saveAsSnapshot(path, nil)
	if err != nil {
		return nil, err
	}
	d.path, d.dirty = path, false
	d.savedHistoryState = d.historyState
	return snapshot, nil
}

func (d *Document) saveAsSnapshot(path string, guard func() (func(), error)) ([]byte, error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".glance-*.docx")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	defer os.Remove(tmpPath)
	file, err := os.Create(tmpPath)
	if err != nil {
		return nil, err
	}
	data, err := d.packageBytes()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	var release func()
	if guard != nil {
		release, err = guard()
		if err != nil {
			return nil, err
		}
	}
	if release != nil {
		defer release()
	}
	if err := fileutil.ReplaceFile(tmpPath, path); err != nil {
		return nil, fmt.Errorf("replace DOCX file: %w", err)
	}
	return data, nil
}
