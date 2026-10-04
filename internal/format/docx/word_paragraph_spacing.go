package docx

import (
	"fmt"
	"strconv"

	"glance/internal/document"
)

var conflictingWordParagraphBeforeSpacingAttributes = []string{
	"beforeLines",
	"beforeAutospacing",
}

var conflictingWordParagraphAfterSpacingAttributes = []string{
	"afterLines",
	"afterAutospacing",
}

func (d *Document) SetParagraphSpacing(paragraphID string, beforeTwips, afterTwips *int64) error {
	if beforeTwips == nil && afterTwips == nil {
		return fmt.Errorf("at least one Word paragraph spacing value must be provided")
	}
	if beforeTwips != nil && *beforeTwips < 0 || afterTwips != nil && *afterTwips < 0 {
		return fmt.Errorf("Word paragraph spacing must be non-negative twips")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	paragraph, ok := d.wordParagraphTargets[paragraphID]
	if !ok || paragraph == nil {
		return fmt.Errorf("Word paragraph %q is not editable", paragraphID)
	}

	properties := firstChild(paragraph, "pPr")
	var spacing *wordXMLNode
	if properties != nil {
		for _, child := range properties.children {
			if child.name.Local != "spacing" {
				continue
			}
			if spacing != nil {
				return fmt.Errorf("Word paragraph %q has duplicate spacing properties", paragraphID)
			}
			spacing = child
		}
	}
	if spacing != nil && len(spacing.children) != 0 {
		return fmt.Errorf("Word paragraph %q has unsupported nested spacing content", paragraphID)
	}

	beforeUnchanged := beforeTwips == nil || wordSpacingAttributeEquals(spacing, "before", *beforeTwips) && !wordSpacingHasRelativeValues(spacing, conflictingWordParagraphBeforeSpacingAttributes)
	afterUnchanged := afterTwips == nil || wordSpacingAttributeEquals(spacing, "after", *afterTwips) && !wordSpacingHasRelativeValues(spacing, conflictingWordParagraphAfterSpacingAttributes)
	if beforeUnchanged && afterUnchanged {
		return nil
	}

	data := d.parts[d.documentIdx].data
	if spacing != nil {
		prefix := elementPrefix(spacing.rawName)
		opening := data[spacing.start:spacing.openEnd]
		patchedOpening := opening
		var err error
		for _, value := range []struct {
			attribute string
			value     *int64
			related   []string
		}{{"before", beforeTwips, conflictingWordParagraphBeforeSpacingAttributes}, {"after", afterTwips, conflictingWordParagraphAfterSpacingAttributes}} {
			if value.value == nil {
				continue
			}
			patchedOpening, err = setRawXMLAttribute(patchedOpening, prefix, value.attribute, strconv.FormatInt(*value.value, 10))
			if err != nil {
				return err
			}
			for _, related := range value.related {
				patchedOpening, err = removeRawXMLAttribute(patchedOpening, prefix, related)
				if err != nil {
					return err
				}
			}
		}
		patched, err := applyXMLReplacements(data, []xmlReplacement{{start: spacing.start, end: spacing.openEnd, value: patchedOpening}})
		if err != nil {
			return err
		}
		return d.applyWordDocumentXMLLocked(patched)
	}

	prefix := elementPrefix(paragraph.rawName)
	var spacingMarkup []byte
	if beforeTwips != nil {
		spacingMarkup = makeWordPropertyMarkup(prefix, "spacing", "before", strconv.FormatInt(*beforeTwips, 10))
	}
	if afterTwips != nil {
		if spacingMarkup == nil {
			spacingMarkup = makeWordPropertyMarkup(prefix, "spacing", "after", strconv.FormatInt(*afterTwips, 10))
		} else {
			var err error
			spacingMarkup, err = setRawXMLAttribute(spacingMarkup, prefix, "after", strconv.FormatInt(*afterTwips, 10))
			if err != nil {
				return err
			}
		}
	}
	if properties == nil {
		patched, err := insertWordParagraphProperties(data, paragraph, prefix, spacingMarkup)
		if err != nil {
			return err
		}
		return d.applyWordDocumentXMLLocked(patched)
	}
	patched, err := insertWordParagraphSpacing(data, properties, spacingMarkup)
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
}

func wordSpacingAttributeEquals(node *wordXMLNode, name string, want int64) bool {
	if node == nil {
		return false
	}
	got, err := strconv.ParseInt(attrValue(node, name), 10, 64)
	return err == nil && got == want
}

func wordSpacingHasRelativeValues(node *wordXMLNode, attributes []string) bool {
	if node == nil {
		return false
	}
	for _, attribute := range attributes {
		for _, existing := range node.attrs {
			if existing.Name.Local == attribute {
				return true
			}
		}
	}
	return false
}

func insertWordParagraphProperties(data []byte, paragraph *wordXMLNode, prefix string, child []byte) ([]byte, error) {
	markup := wrapWordElement(prefix, "pPr", child)
	return applyXMLReplacements(data, []xmlReplacement{{start: paragraph.openEnd, end: paragraph.openEnd, value: markup}})
}

func insertWordParagraphSpacing(data []byte, properties *wordXMLNode, markup []byte) ([]byte, error) {
	ranks := make(map[string]int, len(wordParagraphPropertyOrder))
	for index, name := range wordParagraphPropertyOrder {
		ranks[name] = index
	}
	spacingRank := ranks["spacing"]
	insertAt := properties.contentEnd
	for _, child := range properties.children {
		if rank, known := ranks[child.name.Local]; known && rank > spacingRank {
			insertAt = child.start
			break
		}
	}
	return applyXMLReplacements(data, []xmlReplacement{{start: insertAt, end: insertAt, value: markup}})
}

var _ document.WordParagraphSpacingEditor = (*Document)(nil)
