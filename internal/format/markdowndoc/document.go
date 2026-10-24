// Package markdowndoc preserves Markdown source independently of derived previews.
package markdowndoc

import (
	"errors"
	"fmt"
	"sync"
	"unicode/utf8"

	"glance/internal/fileutil"
)

type Document struct {
	mu     sync.Mutex
	path   string
	text   string
	dirty  bool
	closed bool
}

func NewBlank() *Document { return &Document{dirty: true} }

func Open(path string) (*Document, error) {
	data, err := fileutil.ReadExact(path)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, errors.New("Markdown is not valid UTF-8")
	}
	return &Document{path: path, text: string(data)}, nil
}

// OpenSnapshot restores exact Markdown source, including incomplete syntax.
func OpenSnapshot(payload []byte) (*Document, error) {
	if !utf8.Valid(payload) {
		return nil, errors.New("Markdown snapshot is not valid UTF-8")
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
	if err := fileutil.AtomicWriteString(path, d.text); err != nil {
		return fmt.Errorf("replace Markdown file: %w", err)
	}
	d.path = path
	d.dirty = false
	return nil
}

func (d *Document) Close() error { d.mu.Lock(); defer d.mu.Unlock(); d.closed = true; return nil }
