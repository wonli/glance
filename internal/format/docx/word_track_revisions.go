package docx

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"
)

var _ document.WordTrackRevisionsEditor = (*Document)(nil)

// GetTrackRevisions checks whether revision tracking is enabled in word/settings.xml.
func (d *Document) GetTrackRevisions() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return false, err
	}
	settingsPart, exists := findPackagePart(d.parts, wordSettingsPath)
	if !exists {
		return false, nil
	}
	root, err := parseWordXML(settingsPart.data)
	if err != nil {
		return false, fmt.Errorf("parse DOCX settings: %w", err)
	}
	track := firstChild(root, "trackRevisions")
	if track == nil {
		return false, nil
	}
	val := attrValue(track, "val")
	if val == "" {
		return true, nil
	}
	return val == "1" || strings.EqualFold(val, "true") || strings.EqualFold(val, "on"), nil
}

// SetTrackRevisions enables or disables revision tracking in word/settings.xml.
func (d *Document) SetTrackRevisions(enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}

	settingsPart, exists := findPackagePart(d.parts, wordSettingsPath)
	if !exists {
		if !enabled {
			return nil
		}
		initialData := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
			`<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
			`<w:trackRevisions/>` +
			`</w:settings>`)
		newPart := &part{
			header: zip.FileHeader{Name: wordSettingsPath, Method: zip.Deflate},
			data:   initialData,
		}
		changes := map[string]*part{wordSettingsPath: newPart}

		relPart, err := ensureWordSettingsRelationship(d.parts)
		if err != nil {
			return err
		}
		if relPart != nil {
			changes[relPart.header.Name] = relPart
		}

		ctPart, err := ensureWordSettingsContentType(d.parts)
		if err != nil {
			return err
		}
		if ctPart != nil {
			changes[ctPart.header.Name] = ctPart
		}

		return d.commitWordPackageChange(changes)
	}

	root, err := parseWordXML(settingsPart.data)
	if err != nil {
		return fmt.Errorf("parse DOCX settings: %w", err)
	}
	prefix := elementPrefix(root.rawName)
	if prefix == "" {
		prefix = "w"
	}
	track := firstChild(root, "trackRevisions")

	var updated []byte
	if enabled {
		if track != nil {
			val := attrValue(track, "val")
			if val == "" || val == "1" || strings.EqualFold(val, "true") || strings.EqualFold(val, "on") {
				return nil
			}
			tag := fmt.Sprintf(`<%s:trackRevisions/>`, prefix)
			updated, err = applyXMLReplacements(settingsPart.data, []xmlReplacement{{start: track.start, end: track.end, value: []byte(tag)}})
			if err != nil {
				return err
			}
		} else {
			tag := fmt.Sprintf(`<%s:trackRevisions/>`, prefix)
			updated, err = insertElementContent(settingsPart.data, root, []byte(tag))
			if err != nil {
				return err
			}
		}
	} else {
		if track == nil {
			return nil
		}
		updated, err = applyXMLReplacements(settingsPart.data, []xmlReplacement{{start: track.start, end: track.end, value: nil}})
		if err != nil {
			return err
		}
	}

	changes := map[string]*part{wordSettingsPath: d.partWithData(wordSettingsPath, updated)}
	relPart, err := ensureWordSettingsRelationship(d.parts)
	if err != nil {
		return err
	}
	if relPart != nil {
		changes[relPart.header.Name] = relPart
	}

	ctPart, err := ensureWordSettingsContentType(d.parts)
	if err != nil {
		return err
	}
	if ctPart != nil {
		changes[ctPart.header.Name] = ctPart
	}

	return d.commitWordPackageChange(changes)
}

// AcceptAllRevisions accepts all tracked revisions in the document.
// Inserted text is preserved as regular text runs, and deleted content is permanently removed.
func (d *Document) AcceptAllRevisions() (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return 0, err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return 0, errors.New("document part missing")
	}

	docData := d.parts[d.documentIdx].data
	root, err := parseWordXML(docData)
	if err != nil {
		return 0, fmt.Errorf("parse word/document.xml: %w", err)
	}

	var insNodes []*wordXMLNode
	var delNodes []*wordXMLNode
	collectRevisionNodes(root, &insNodes, &delNodes)

	totalCount := len(insNodes) + len(delNodes)
	if totalCount == 0 {
		return 0, nil
	}

	var replacements []xmlReplacement
	for _, ins := range insNodes {
		if ins.selfClosing {
			replacements = append(replacements, xmlReplacement{start: ins.start, end: ins.end, value: nil})
		} else {
			// Remove opening <w:ins...> and closing </w:ins>
			replacements = append(replacements, xmlReplacement{start: ins.start, end: ins.contentStart, value: nil})
			replacements = append(replacements, xmlReplacement{start: ins.contentEnd, end: ins.end, value: nil})
		}
	}

	for _, del := range delNodes {
		// Completely remove deleted blocks
		replacements = append(replacements, xmlReplacement{start: del.start, end: del.end, value: nil})
	}

	updated, err := applyXMLReplacements(docData, replacements)
	if err != nil {
		return 0, fmt.Errorf("apply accepted revisions: %w", err)
	}

	changes := map[string]*part{"word/document.xml": d.partWithData("word/document.xml", updated)}
	if err := d.commitWordPackageChange(changes); err != nil {
		return 0, err
	}
	return totalCount, nil
}

// RejectAllRevisions rejects all tracked revisions in the document.
// Inserted content is removed, and deleted content is restored as regular text.
func (d *Document) RejectAllRevisions() (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return 0, err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return 0, errors.New("document part missing")
	}

	docData := d.parts[d.documentIdx].data
	root, err := parseWordXML(docData)
	if err != nil {
		return 0, fmt.Errorf("parse word/document.xml: %w", err)
	}

	var insNodes []*wordXMLNode
	var delNodes []*wordXMLNode
	collectRevisionNodes(root, &insNodes, &delNodes)

	totalCount := len(insNodes) + len(delNodes)
	if totalCount == 0 {
		return 0, nil
	}

	var replacements []xmlReplacement
	for _, ins := range insNodes {
		// Remove inserted content entirely
		replacements = append(replacements, xmlReplacement{start: ins.start, end: ins.end, value: nil})
	}

	for _, del := range delNodes {
		if del.selfClosing {
			replacements = append(replacements, xmlReplacement{start: del.start, end: del.end, value: nil})
		} else {
			// Restore deleted content by converting <w:delText> to <w:t>
			content := docData[del.contentStart:del.contentEnd]
			restored := bytes.ReplaceAll(content, []byte("<w:delText"), []byte("<w:t"))
			restored = bytes.ReplaceAll(restored, []byte("</w:delText>"), []byte("</w:t>"))
			restored = bytes.ReplaceAll(restored, []byte("<delText"), []byte("<t"))
			restored = bytes.ReplaceAll(restored, []byte("</delText>"), []byte("</t>"))
			replacements = append(replacements, xmlReplacement{start: del.start, end: del.end, value: restored})
		}
	}

	updated, err := applyXMLReplacements(docData, replacements)
	if err != nil {
		return 0, fmt.Errorf("apply rejected revisions: %w", err)
	}

	changes := map[string]*part{"word/document.xml": d.partWithData("word/document.xml", updated)}
	if err := d.commitWordPackageChange(changes); err != nil {
		return 0, err
	}
	return totalCount, nil
}

func collectRevisionNodes(node *wordXMLNode, insNodes, delNodes *[]*wordXMLNode) {
	if node == nil {
		return
	}
	if node.name.Local == "ins" {
		*insNodes = append(*insNodes, node)
		return
	}
	if node.name.Local == "del" {
		*delNodes = append(*delNodes, node)
		return
	}
	for _, child := range node.children {
		collectRevisionNodes(child, insNodes, delNodes)
	}
}
