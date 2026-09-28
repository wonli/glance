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
	mu          sync.Mutex
	path        string
	parts       []part
	documentIdx int
	blocks      []Block
	spans       [][]textSpan
	dirty       bool
	closed      bool
}

func Open(path string) (*Document, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open docx %q: %w", path, err)
	}
	defer zr.Close()
	doc := &Document{path: path, documentIdx: -1}
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

// ReplaceParagraph edits a paragraph only when all its text belongs to one
// Word run. This preserves run formatting; multi-run editing needs a richer UI.
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
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(value)); err != nil {
		return err
	}
	xmlData := d.parts[d.documentIdx].data
	preserveSpace := len(value) > 0 && strings.TrimSpace(value) != value
	insertAt := span.start
	if preserveSpace {
		tagStart := bytes.LastIndex(xmlData[:span.start], []byte("<"))
		if tagStart < 0 {
			return errors.New("cannot locate Word text element")
		}
		if !bytes.Contains(xmlData[tagStart:span.start], []byte("xml:space=")) {
			insertAt = span.start - 1
		}
	}
	patched := make([]byte, 0, len(xmlData)-span.end+span.start+escaped.Len()+24)
	patched = append(patched, xmlData[:insertAt]...)
	if preserveSpace && insertAt < span.start {
		patched = append(patched, []byte(` xml:space="preserve"`)...)
		patched = append(patched, xmlData[insertAt:span.start]...)
	}
	patched = append(patched, escaped.Bytes()...)
	patched = append(patched, xmlData[span.end:]...)
	original := d.parts[d.documentIdx].data
	d.parts[d.documentIdx].data = patched
	if err := d.reparse(); err != nil {
		d.parts[d.documentIdx].data = original
		_ = d.reparse()
		return err
	}
	d.dirty = true
	return nil
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
	if err := d.saveAs(d.path); err != nil {
		return err
	}
	d.dirty = false
	return nil
}

func (d *Document) SaveAs(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	return d.saveAs(path)
}

func (d *Document) saveAs(path string) error {
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
	zw := zip.NewWriter(file)
	for _, item := range d.parts {
		header := item.header
		writer, err := zw.CreateHeader(&header)
		if err != nil {
			_ = zw.Close()
			_ = file.Close()
			return err
		}
		if _, err := writer.Write(item.data); err != nil {
			_ = zw.Close()
			_ = file.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
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
