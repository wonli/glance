package docx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

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
