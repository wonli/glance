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

	data := d.parts[d.documentIdx].data
	rep, changed, err := paragraphSpacingReplacement(data, paragraph, beforeTwips, afterTwips)
	if err != nil {
		return fmt.Errorf("Word paragraph %q: %w", paragraphID, err)
	}
	if !changed {
		return nil
	}
	patched, err := applyXMLReplacements(data, []xmlReplacement{rep})
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
}

func (d *Document) SetAllParagraphsSpacing(beforeTwips, afterTwips int64) error {
	if beforeTwips < 0 || afterTwips < 0 {
		return fmt.Errorf("Word paragraph spacing must be non-negative twips")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	data := d.parts[d.documentIdx].data
	var replacements []xmlReplacement
	for _, paragraph := range d.wordParagraphTargets {
		if paragraph == nil {
			continue
		}
		rep, changed, err := paragraphSpacingReplacement(data, paragraph, &beforeTwips, &afterTwips)
		if err != nil {
			return err
		}
		if changed {
			replacements = append(replacements, rep)
		}
	}
	if len(replacements) == 0 {
		return nil
	}
	patched, err := applyXMLReplacements(data, replacements)
	if err != nil {
		return fmt.Errorf("apply all paragraphs spacing: %w", err)
	}
	return d.applyWordDocumentXMLLocked(patched)
}

func paragraphSpacingReplacement(data []byte, paragraph *wordXMLNode, beforeTwips, afterTwips *int64) (xmlReplacement, bool, error) {
	properties := firstChild(paragraph, "pPr")
	var spacing *wordXMLNode
	if properties != nil {
		for _, child := range properties.children {
			if child.name.Local != "spacing" {
				continue
			}
			if spacing != nil {
				return xmlReplacement{}, false, fmt.Errorf("duplicate spacing properties")
			}
			spacing = child
		}
	}
	if spacing != nil && len(spacing.children) != 0 {
		return xmlReplacement{}, false, fmt.Errorf("unsupported nested spacing content")
	}

	beforeUnchanged := beforeTwips == nil || wordSpacingAttributeEquals(spacing, "before", *beforeTwips) && !wordSpacingHasRelativeValues(spacing, conflictingWordParagraphBeforeSpacingAttributes)
	afterUnchanged := afterTwips == nil || wordSpacingAttributeEquals(spacing, "after", *afterTwips) && !wordSpacingHasRelativeValues(spacing, conflictingWordParagraphAfterSpacingAttributes)
	if beforeUnchanged && afterUnchanged {
		return xmlReplacement{}, false, nil
	}

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
				return xmlReplacement{}, false, err
			}
			for _, related := range value.related {
				patchedOpening, err = removeRawXMLAttribute(patchedOpening, prefix, related)
				if err != nil {
					return xmlReplacement{}, false, err
				}
			}
		}
		return xmlReplacement{start: spacing.start, end: spacing.openEnd, value: patchedOpening}, true, nil
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
				return xmlReplacement{}, false, err
			}
		}
	}
	if properties == nil {
		markup := wrapWordElement(prefix, "pPr", spacingMarkup)
		return xmlReplacement{start: paragraph.openEnd, end: paragraph.openEnd, value: markup}, true, nil
	}
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
	return xmlReplacement{start: insertAt, end: insertAt, value: spacingMarkup}, true, nil
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
