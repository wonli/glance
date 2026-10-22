package docx

import (
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.WordTableSelectionEditor = (*Document)(nil)

func (d *Document) DeleteWordTableSelection(tableID string, cellIDs []string) (document.WordTableSelectionEditResult, error) {
	if strings.TrimSpace(tableID) == "" {
		return document.WordTableSelectionNoChange, fmt.Errorf("Word table ID is empty")
	}
	if len(cellIDs) == 0 {
		return document.WordTableSelectionNoChange, fmt.Errorf("Word table cell selection is empty")
	}
	requested := make(map[string]struct{}, len(cellIDs))
	for _, cellID := range cellIDs {
		if strings.TrimSpace(cellID) == "" {
			return document.WordTableSelectionNoChange, fmt.Errorf("Word table cell ID is empty")
		}
		if _, exists := requested[cellID]; exists {
			return document.WordTableSelectionNoChange, fmt.Errorf("duplicate Word table cell ID %q", cellID)
		}
		requested[cellID] = struct{}{}
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return document.WordTableSelectionNoChange, err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return document.WordTableSelectionNoChange, fmt.Errorf("DOCX document part is unavailable")
	}
	if !documentHasWordTable(d.wordBlocks, tableID) {
		return document.WordTableSelectionNoChange, fmt.Errorf("Word table %q is not available", tableID)
	}

	data := d.parts[d.documentIdx].data
	root, err := parseWordXML(data)
	if err != nil {
		return document.WordTableSelectionNoChange, fmt.Errorf("parse DOCX document before table edit: %w", err)
	}
	table, readOnly, err := findProjectedWordTable(root, tableID)
	if err != nil {
		return document.WordTableSelectionNoChange, err
	}
	if readOnly {
		return document.WordTableSelectionNoChange, fmt.Errorf("Word table %q is inside read-only content", tableID)
	}

	cells := make(map[string]*wordXMLNode, len(requested))
	tableEmpty := true
	rowIndex := 0
	for _, row := range table.children {
		if row.name.Local != "tr" {
			continue
		}
		cellIndex := 0
		for _, cell := range row.children {
			if cell.name.Local != "tc" {
				continue
			}
			cellID := fmt.Sprintf("%s.r%d.c%d", tableID, rowIndex, cellIndex)
			if wordTableCellHasContent(cell) {
				tableEmpty = false
			}
			if _, selected := requested[cellID]; selected {
				cells[cellID] = cell
			}
			cellIndex++
		}
		rowIndex++
	}
	if len(cells) != len(requested) {
		for cellID := range requested {
			if _, found := cells[cellID]; !found {
				return document.WordTableSelectionNoChange, fmt.Errorf("Word table cell %q is not part of table %q", cellID, tableID)
			}
		}
	}

	if tableEmpty {
		patched, err := applyXMLReplacements(data, []xmlReplacement{{start: table.start, end: table.end}})
		if err != nil {
			return document.WordTableSelectionNoChange, fmt.Errorf("prepare DOCX table deletion: %w", err)
		}
		if err := d.applyWordDocumentXMLLocked(patched); err != nil {
			return document.WordTableSelectionNoChange, err
		}
		return document.WordTableSelectionTableDeleted, nil
	}

	changes := make([]xmlReplacement, 0, len(cells))
	for _, cellID := range cellIDs {
		cell := cells[cellID]
		if !wordTableCellHasContent(cell) {
			continue
		}
		content, err := emptyWordTableCellContents(data, cell)
		if err != nil {
			return document.WordTableSelectionNoChange, fmt.Errorf("clear Word table cell %q: %w", cellID, err)
		}
		changes = append(changes, xmlReplacement{start: cell.contentStart, end: cell.contentEnd, value: content})
	}
	if len(changes) == 0 {
		return document.WordTableSelectionNoChange, nil
	}
	patched, err := applyXMLReplacements(data, changes)
	if err != nil {
		return document.WordTableSelectionNoChange, fmt.Errorf("prepare DOCX table cell clearing: %w", err)
	}
	if err := d.applyWordDocumentXMLLocked(patched); err != nil {
		return document.WordTableSelectionNoChange, err
	}
	return document.WordTableSelectionCellsCleared, nil
}

func documentHasWordTable(blocks []document.WordBlock, tableID string) bool {
	for _, block := range blocks {
		if block.ID == tableID && block.Kind == document.WordTableBlock && block.Table != nil {
			return true
		}
	}
	return false
}

// findProjectedWordTable follows the same top-level block IDs as the neutral
// Word projection. Tables projected from content controls are read-only.
func findProjectedWordTable(root *wordXMLNode, tableID string) (*wordXMLNode, bool, error) {
	body := firstChild(root, "body")
	if body == nil {
		return nil, false, fmt.Errorf("DOCX document has no body element")
	}
	contentBlock := 0
	for _, child := range body.children {
		blockID := fmt.Sprintf("b%d", contentBlock)
		switch child.name.Local {
		case "p", "tbl":
			if child.name.Local == "tbl" && blockID == tableID {
				return child, false, nil
			}
			contentBlock++
		case "sdt":
			content := firstChild(child, "sdtContent")
			if content == nil {
				contentBlock++
				continue
			}
			for contentIndex, contentChild := range content.children {
				if isWordRangeMarker(contentChild.name.Local) {
					continue
				}
				contentBlockID := fmt.Sprintf("%s.c%d", blockID, contentIndex)
				if contentChild.name.Local == "tbl" && contentBlockID == tableID {
					return contentChild, true, nil
				}
				contentBlock++
			}
		case "sectPr", "bookmarkStart", "bookmarkEnd", "proofErr", "commentRangeStart", "commentRangeEnd", "permStart", "permEnd":
			// These nodes do not advance the projected body block index.
		default:
			contentBlock++
		}
	}
	return nil, false, fmt.Errorf("Word table %q is not available in the DOCX body", tableID)
}

func emptyWordTableCellContents(data []byte, cell *wordXMLNode) ([]byte, error) {
	var content []byte
	hasParagraph := false
	for _, child := range cell.children {
		switch child.name.Local {
		case "tcPr":
			content = append(content, data[child.start:child.end]...)
		case "p":
			emptyParagraph, err := emptyWordParagraph(data, child)
			if err != nil {
				return nil, err
			}
			content = append(content, emptyParagraph...)
			hasParagraph = true
		}
	}
	if !hasParagraph {
		prefix := elementPrefix(cell.rawName)
		content = append(content, []byte("<"+prefix+":p/>")...)
	}
	return content, nil
}

func emptyWordParagraph(data []byte, paragraph *wordXMLNode) ([]byte, error) {
	opening := append([]byte(nil), data[paragraph.start:paragraph.openEnd]...)
	if paragraph.selfClosing {
		var err error
		opening, err = expandSelfClosingTag(opening)
		if err != nil {
			return nil, err
		}
	}
	content := append([]byte(nil), opening...)
	if properties := firstChild(paragraph, "pPr"); properties != nil {
		content = append(content, data[properties.start:properties.end]...)
	}
	content = append(content, []byte("</"+paragraph.rawName+">")...)
	return content, nil
}
