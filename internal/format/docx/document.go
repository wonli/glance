package docx

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"glance/internal/document"
)

type BlockKind string

const (
	ParagraphBlock BlockKind = "paragraph"
	TableBlock     BlockKind = "table"
)

type Block struct {
	Kind     BlockKind
	Text     string
	Rows     [][]string
	Editable bool
	Style    string
}

type part struct {
	header zip.FileHeader
	data   []byte
}

type textSpan struct{ start, end int }

type Document struct {
	mu                   sync.Mutex
	path                 string
	parts                []part
	documentIdx          int
	blocks               []Block
	spans                [][]textSpan
	wordBlocks           []document.WordBlock
	wordLayoutData       document.WordLayoutData
	wordImages           map[string]int
	wordImageTargets     map[string]wordImageTarget
	wordInlineTargets    map[string]wordInlineTarget
	wordParagraphInlines map[string][]string
	wordTextTargets      map[string]wordTextTarget
	wordParagraphTargets map[string]*wordXMLNode
	undo                 []wordHistoryEntry
	redo                 []wordHistoryEntry
	historyState         uint64
	savedHistoryState    uint64
	nextHistoryState     uint64
	dirty                bool
	closed               bool
}

func Open(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("open docx %q: %w", path, err)
	}
	doc, err := OpenSnapshot(data)
	if err != nil {
		return nil, fmt.Errorf("open docx %q: %w", path, err)
	}
	doc.path = path
	return doc, nil
}

// OpenSnapshot restores a self-contained DOCX package from durable session state.
func OpenSnapshot(data []byte) (*Document, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("read DOCX snapshot: %w", err)
	}
	doc := &Document{documentIdx: -1}
	doc.historyState, doc.savedHistoryState, doc.nextHistoryState = 1, 1, 2
	for _, file := range zr.File {
		r, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		doc.parts = append(doc.parts, part{header: file.FileHeader, data: data})
		if file.Name == "word/document.xml" {
			doc.documentIdx = len(doc.parts) - 1
		}
	}
	if doc.documentIdx < 0 {
		return nil, errors.New("DOCX package is missing word/document.xml")
	}
	if err := doc.reparse(); err != nil {
		return nil, fmt.Errorf("parse DOCX body: %w", err)
	}
	return doc, nil
}

// NewBlank creates a minimal editable Word draft.
func NewBlank() (*Document, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entries := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/document.xml", `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t></w:t></w:r></w:p><w:sectPr><w:pgSz w:w="11906" w:h="16838" w:orient="portrait"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`},
	}
	for _, entry := range entries {
		w, err := zw.Create(entry.name)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(w, entry.body); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	doc, err := OpenSnapshot(buf.Bytes())
	if err == nil {
		doc.dirty = true
		doc.savedHistoryState = 0
	}
	return doc, err
}

// Snapshot serializes current package parts without changing dirty state.
func (d *Document) Snapshot() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return nil, err
	}
	return d.packageBytes()
}

// RestoreSnapshot atomically applies a complete DOCX package as one history edit.
// Parsing is completed before current document state is touched. Failed parsing
// or projection leaves package data, UI projections, dirty state, and history intact.
func (d *Document) RestoreSnapshot(snapshot []byte) error {
	target, err := OpenSnapshot(snapshot)
	if err != nil {
		return err
	}
	defer target.Close()

	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}

	names := append(packagePartNames(d.parts), packagePartNames(target.parts)...)
	before := d.captureWordHistoryLocked(names...)
	before.force = true

	previous := d.snapshotProjectionLocked()
	previousParts, previousDocumentIdx := d.parts, d.documentIdx
	d.parts = clonePackageParts(target.parts)
	d.documentIdx = packagePartIndex(d.parts, "word/document.xml")
	if err := d.reparse(); err != nil {
		d.parts, d.documentIdx = previousParts, previousDocumentIdx
		d.restoreProjectionLocked(previous)
		return fmt.Errorf("restore DOCX snapshot: %w", err)
	}
	d.recordWordHistoryLocked(before)
	return nil
}

type wordProjectionState struct {
	blocks               []Block
	spans                [][]textSpan
	wordBlocks           []document.WordBlock
	wordLayoutData       document.WordLayoutData
	wordImages           map[string]int
	wordImageTargets     map[string]wordImageTarget
	wordInlineTargets    map[string]wordInlineTarget
	wordParagraphInlines map[string][]string
	wordTextTargets      map[string]wordTextTarget
	wordParagraphTargets map[string]*wordXMLNode
}

func (d *Document) snapshotProjectionLocked() wordProjectionState {
	return wordProjectionState{
		blocks: d.blocks, spans: d.spans, wordBlocks: d.wordBlocks,
		wordLayoutData: d.wordLayoutData, wordImages: d.wordImages,
		wordImageTargets: d.wordImageTargets, wordInlineTargets: d.wordInlineTargets,
		wordParagraphInlines: d.wordParagraphInlines, wordTextTargets: d.wordTextTargets,
		wordParagraphTargets: d.wordParagraphTargets,
	}
}

func (d *Document) restoreProjectionLocked(state wordProjectionState) {
	d.blocks, d.spans, d.wordBlocks = state.blocks, state.spans, state.wordBlocks
	d.wordLayoutData = state.wordLayoutData
	d.wordImages, d.wordImageTargets = state.wordImages, state.wordImageTargets
	d.wordInlineTargets, d.wordParagraphInlines = state.wordInlineTargets, state.wordParagraphInlines
	d.wordTextTargets, d.wordParagraphTargets = state.wordTextTargets, state.wordParagraphTargets
}

func clonePackageParts(parts []part) []part {
	cloned := make([]part, len(parts))
	for i, item := range parts {
		cloned[i] = clonePackagePart(item)
	}
	return cloned
}

func (d *Document) SetSourcePath(path string) { d.mu.Lock(); defer d.mu.Unlock(); d.path = path }
func (d *Document) SetDirty(dirty bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if dirty {
		if !d.dirty && d.historyState == d.savedHistoryState {
			d.historyState = d.nextHistoryState
			d.nextHistoryState++
		}
		d.dirty = true
		return
	}
	d.savedHistoryState = d.historyState
	d.dirty = false
}

func (d *Document) Blocks() []Block {
	d.mu.Lock()
	defer d.mu.Unlock()
	blocks := make([]Block, len(d.blocks))
	for i, block := range d.blocks {
		blocks[i] = block
		if block.Rows != nil {
			blocks[i].Rows = make([][]string, len(block.Rows))
			for j := range block.Rows {
				blocks[i].Rows[j] = append([]string(nil), block.Rows[j]...)
			}
		}
	}
	return blocks
}

// ReplaceParagraph preserves the legacy paragraph-index API for existing
// callers. New UI code should edit a selected run through WordTextEditor.
func (d *Document) ReplaceParagraph(index int, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if index < 0 || index >= len(d.blocks) || d.blocks[index].Kind != ParagraphBlock {
		return fmt.Errorf("paragraph index %d is out of range", index)
	}
	if !d.blocks[index].Editable || len(d.spans[index]) != 1 {
		return errors.New("paragraph has formatting or inline content that is not supported for editing")
	}
	span := d.spans[index][0]
	for runID, target := range d.wordTextTargets {
		if target.text != nil && target.text.contentStart == span.start && target.text.contentEnd == span.end {
			return d.replaceTextRunLocked(runID, value)
		}
	}
	return errors.New("paragraph text could not be resolved to an editable Word run")
}

func (d *Document) Dirty() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dirty
}

func (d *Document) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	d.parts = nil
	d.blocks = nil
	d.spans = nil
	d.wordBlocks = nil
	d.wordImages = nil
	d.wordImageTargets = nil
	d.wordInlineTargets = nil
	d.wordParagraphInlines = nil
	d.wordTextTargets = nil
	d.wordParagraphTargets = nil
	d.undo, d.redo = nil, nil
	return nil
}

func (d *Document) checkOpen() error {
	if d.closed {
		return errors.New("document is closed")
	}
	return nil
}
