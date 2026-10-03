package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"glance/internal/document"
	"glance/internal/fileutil"
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
		{"word/document.xml", `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t></w:t></w:r></w:p><w:sectPr/></w:body></w:document>`},
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

func (d *Document) Save() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.path == "" {
		return errors.New("document has no save path")
	}
	if err := d.saveAs(d.path, nil); err != nil {
		return err
	}
	d.savedHistoryState = d.historyState
	d.dirty = false
	return nil
}

func (d *Document) SaveAs(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	return d.saveAs(path, nil)
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
	return d.saveAs(path, guard)
}

// SaveAsDocument writes and adopts a new source path for subsequent Save calls.
func (d *Document) SaveAsDocument(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if err := d.saveAs(path, nil); err != nil {
		return err
	}
	d.path, d.dirty = path, false
	d.savedHistoryState = d.historyState
	return nil
}

func (d *Document) saveAs(path string, guard func() (func(), error)) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".glance-*.docx")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	data, err := d.packageBytes()
	if err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
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
		return fmt.Errorf("replace DOCX file: %w", err)
	}
	return nil
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

func (d *Document) reparse() error {
	data := d.parts[d.documentIdx].data
	decoder := xml.NewDecoder(bytes.NewReader(data))
	d.blocks = nil
	d.spans = nil
	inBody := false
	bodyDepth := 0
	depth := 0
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch value := tok.(type) {
		case xml.StartElement:
			depth++
			if !inBody && value.Name.Local == "body" {
				inBody = true
				bodyDepth = depth
				continue
			}
			if inBody && depth == bodyDepth+1 {
				if value.Name.Local == "p" {
					block, spans, err := readParagraph(decoder, value, data)
					if err != nil {
						return err
					}
					d.blocks = append(d.blocks, block)
					d.spans = append(d.spans, spans)
					depth = bodyDepth
				} else if value.Name.Local == "tbl" {
					block, err := readTable(decoder, value)
					if err != nil {
						return err
					}
					d.blocks = append(d.blocks, block)
					d.spans = append(d.spans, nil)
					depth = bodyDepth
				} else {
					if err := skipElement(decoder, value); err != nil {
						return err
					}
					depth = bodyDepth
				}
			}
		case xml.EndElement:
			if inBody && depth == bodyDepth && value.Name.Local == "body" {
				inBody = false
			}
			depth--
		}
	}
	if inBody {
		return errors.New("unterminated Word body")
	}
	projection, err := projectWordDocument(d.parts, d.documentIdx)
	if err != nil {
		return err
	}
	d.wordBlocks = projection.blocks
	d.wordLayoutData = projection.layout
	d.wordImages = projection.imageParts
	d.wordImageTargets = projection.imageTargets
	d.wordInlineTargets = projection.inlineTargets
	d.wordParagraphInlines = projection.paragraphInlines
	d.wordTextTargets = projection.textTargets
	d.wordParagraphTargets = projection.paragraphTargets
	return nil
}

func readParagraph(decoder *xml.Decoder, root xml.StartElement, data []byte) (Block, []textSpan, error) {
	block := Block{Kind: ParagraphBlock, Editable: true}
	spans := []textSpan{}
	unsupported := false
	depth := 1
	activeStart := -1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return Block{}, nil, err
		}
		switch value := tok.(type) {
		case xml.StartElement:
			depth++
			if value.Name.Local == "t" {
				activeStart = int(decoder.InputOffset())
			}
			switch value.Name.Local {
			case "tab", "br", "drawing", "pict", "object", "fldChar", "instrText":
				unsupported = true
			}
			if value.Name.Local == "pStyle" {
				for _, attr := range value.Attr {
					if attr.Name.Local == "val" {
						block.Style = attr.Value
					}
				}
			}
		case xml.CharData:
			if activeStart >= 0 {
				block.Text += string(value)
			}
		case xml.EndElement:
			if value.Name.Local == "t" && activeStart >= 0 {
				endOffset := int(decoder.InputOffset())
				if endOffset == activeStart {
					spans = append(spans, textSpan{start: activeStart, end: activeStart})
					activeStart = -1
					depth--
					continue
				}
				relative := bytes.LastIndex(data[activeStart:endOffset], []byte("</"))
				if relative < 0 {
					return Block{}, nil, errors.New("cannot locate end of Word text run")
				}
				spans = append(spans, textSpan{start: activeStart, end: activeStart + relative})
				activeStart = -1
			}
			depth--
		}
	}
	block.Editable = len(spans) == 1 && !unsupported
	return block, spans, nil
}

func readTable(decoder *xml.Decoder, root xml.StartElement) (Block, error) {
	block := Block{Kind: TableBlock}
	depth := 1
	rowOpen, cellOpen := false, false
	textOpen := false
	var row []string
	var cell strings.Builder
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return Block{}, err
		}
		switch value := tok.(type) {
		case xml.StartElement:
			depth++
			switch value.Name.Local {
			case "t":
				textOpen = true
			case "tr":
				rowOpen, row = true, nil
			case "tc":
				cellOpen = true
				cell.Reset()
			}
		case xml.CharData:
			if cellOpen && textOpen {
				cell.Write(value)
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "t":
				textOpen = false
			case "tc":
				if cellOpen {
					row = append(row, cell.String())
					cellOpen = false
				}
			case "tr":
				if rowOpen {
					block.Rows = append(block.Rows, row)
					rowOpen = false
				}
			}
			depth--
		}
	}
	return block, nil
}

func skipElement(decoder *xml.Decoder, root xml.StartElement) error {
	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}
