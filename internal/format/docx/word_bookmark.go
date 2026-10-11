package docx

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"glance/internal/document"
)

var _ document.WordBookmarkEditor = (*Document)(nil)

// InsertBookmark inserts a named bookmark surrounding the target run or at the target paragraph.
func (d *Document) InsertBookmark(runID, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return errors.New("DOCX document part is unavailable")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = "Bookmark1"
	}
	// OOXML bookmark names must not contain whitespace; replace spaces with underscores.
	name = strings.ReplaceAll(name, " ", "_")

	data := d.parts[d.documentIdx].data
	root, err := parseWordXML(data)
	if err != nil {
		return fmt.Errorf("parse DOCX document before bookmark insertion: %w", err)
	}

	prefix := elementPrefix(root.rawName)
	if prefix == "" {
		prefix = "w"
	}

	// Find target run or fallback to first available run
	var targetRun *wordXMLNode
	if runID != "" {
		target, ok := d.wordTextTargets[runID]
		if !ok || target.run == nil {
			return fmt.Errorf("bookmark target text run %q not found", runID)
		}
		targetRun = target.run
	} else {
		for _, t := range d.wordTextTargets {
			if t.run != nil {
				targetRun = t.run
				break
			}
		}
	}

	var startOffset, endOffset int
	if targetRun != nil {
		startOffset = targetRun.start
		endOffset = targetRun.end
	} else {
		body := firstChild(root, "body")
		if body == nil {
			return errors.New("DOCX document has no body element")
		}
		startOffset = body.contentStart
		endOffset = body.contentStart
	}

	// Find maximum bookmark ID
	maxID := -1
	var findMaxID func(node *wordXMLNode)
	findMaxID = func(node *wordXMLNode) {
		if node == nil {
			return
		}
		if node.name.Local == "bookmarkStart" || node.name.Local == "bookmarkEnd" {
			if idVal := attrValue(node, "id"); idVal != "" {
				if id, err := strconv.Atoi(idVal); err == nil && id > maxID {
					maxID = id
				}
			}
		}
		for _, ch := range node.children {
			findMaxID(ch)
		}
	}
	findMaxID(root)

	newID := maxID + 1
	startTag := fmt.Sprintf("<%s:bookmarkStart %s:id=\"%d\" %s:name=\"%s\"/>", prefix, prefix, newID, prefix, escapeXMLString(name))
	endTag := fmt.Sprintf("<%s:bookmarkEnd %s:id=\"%d\"/>", prefix, prefix, newID)

	var replacements []xmlReplacement
	if startOffset == endOffset {
		replacements = []xmlReplacement{
			{start: startOffset, end: startOffset, value: []byte(startTag + endTag)},
		}
	} else {
		replacements = []xmlReplacement{
			{start: startOffset, end: startOffset, value: []byte(startTag)},
			{start: endOffset, end: endOffset, value: []byte(endTag)},
		}
	}

	patched, err := applyXMLReplacements(data, replacements)
	if err != nil {
		return fmt.Errorf("apply bookmark XML replacements: %w", err)
	}

	return d.applyWordDocumentXMLLocked(patched)
}
