// Package jsondoc provides a minimal text editing adapter for JSON documents.
package jsondoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"glance/internal/fileutil"
)

type snapshotMetadata struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
}

type Document struct {
	mu     sync.Mutex
	path   string
	text   string
	dirty  bool
	closed bool
}

func NewBlank() *Document { return &Document{text: "{}\n", dirty: true} }

func Open(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &Document{path: path, text: string(data)}, nil
}

// OpenSnapshot restores possibly in-progress JSON editor text, including invalid intermediate text.
func OpenSnapshot(payload []byte) (*Document, error) {
	if bytes.HasPrefix(payload, []byte(`{"kind":"glance-json-v1"`)) {
		var meta snapshotMetadata
		if err := json.Unmarshal(payload, &meta); err == nil && meta.Source != "" {
			doc, err := Open(meta.Source)
			if err == nil {
				return doc, nil
			}
		}
	}
	if !json.Valid(payload) {
		// Snapshot envelopes are intentionally the exact UTF-8 editor contents, so malformed user text is recoverable.
		if !validUTF8(payload) {
			return nil, errors.New("JSON snapshot is not valid UTF-8")
		}
	}
	return &Document{text: string(payload), dirty: true}, nil
}

func (d *Document) Text() string              { d.mu.Lock(); defer d.mu.Unlock(); return d.text }
func (d *Document) Dirty() bool               { d.mu.Lock(); defer d.mu.Unlock(); return d.dirty }
func (d *Document) SetSourcePath(path string) { d.mu.Lock(); defer d.mu.Unlock(); d.path = path }
func (d *Document) SetDirty(dirty bool)       { d.mu.Lock(); defer d.mu.Unlock(); d.dirty = dirty }

func (d *Document) SetText(value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("document is closed")
	}
	if value != d.text {
		d.text = value
		d.dirty = true
	}
	return nil
}

func (d *Document) Snapshot() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, errors.New("document is closed")
	}
	if d.path != "" && len(d.text) > 2*1024*1024 && !d.dirty {
		meta := snapshotMetadata{
			Kind:   "glance-json-v1",
			Source: d.path,
		}
		return json.Marshal(meta)
	}
	return []byte(d.text), nil
}

func (d *Document) Save() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("document is closed")
	}
	if d.path == "" {
		return errors.New("document has no save path")
	}
	return d.saveAsLocked(d.path)
}

func (d *Document) SaveAs(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("document is closed")
	}
	return d.saveAsLocked(path)
}

func (d *Document) saveAsLocked(path string) error {
	if !json.Valid([]byte(d.text)) {
		return errors.New("JSON is invalid; correct it before saving")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".glance-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(d.text); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := fileutil.ReplaceFile(tmpPath, path); err != nil {
		return fmt.Errorf("replace JSON file: %w", err)
	}
	d.path = path
	d.dirty = false
	return nil
}

func (d *Document) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	d.text = ""
	return nil
}
