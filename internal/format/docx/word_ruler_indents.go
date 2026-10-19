package docx

import (
	"bytes"
	"fmt"
	"strconv"
)

// SetParagraphRulerIndents applies a ruler gesture as one document mutation.
// firstOffsetEMU is relative to the paragraph left indent; nil preserves its
// existing first-line/hanging representation, including character units.
func (d *Document) SetParagraphRulerIndents(id string, leftEMU, rightEMU int64, firstOffsetEMU *int64) error {
	left, err := wordIndentEMUToTwips(leftEMU)
	if err != nil {
		return err
	}
	right, err := wordIndentEMUToTwips(rightEMU)
	if err != nil {
		return err
	}
	var first int64
	if firstOffsetEMU != nil {
		first, err = wordIndentEMUToTwips(*firstOffsetEMU)
		if err != nil {
			return err
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	p := d.wordParagraphTargets[id]
	if p == nil {
		return fmt.Errorf("Word paragraph %q is not editable", id)
	}
	props := firstChild(p, "pPr")
	var ind *wordXMLNode
	if props != nil {
		ind = firstChild(props, "ind")
	}
	prefix := elementPrefix(p.rawName)
	data := d.parts[d.documentIdx].data
	opening := makeWordPropertyMarkup(prefix, "ind", "left", strconv.FormatInt(left, 10))
	if ind != nil {
		opening = append([]byte(nil), data[ind.start:ind.openEnd]...)
		prefix = elementPrefix(ind.rawName)
	}
	attrs := map[string]string{wordIndentAttributeName(ind, "left", "start"): strconv.FormatInt(left, 10), wordIndentAttributeName(ind, "right", "end"): strconv.FormatInt(right, 10)}
	remove := []string{"leftChars", "rightChars", "startChars", "endChars"}
	if firstOffsetEMU != nil {
		remove = append(remove, "firstLine", "first-line", "hanging", "firstLineChars", "hangingChars")
	}
	for _, name := range remove {
		opening, err = removeRawXMLAttribute(opening, prefix, name)
		if err != nil {
			return err
		}
	}
	if firstOffsetEMU != nil {
		if first < 0 {
			attrs["hanging"] = strconv.FormatInt(-first, 10)
		} else {
			attrs["firstLine"] = strconv.FormatInt(first, 10)
		}
	}
	// Stable order also makes repeated no-op edits byte-identical.
	for _, name := range []string{"left", "start", "right", "end", "firstLine", "hanging"} {
		if value, ok := attrs[name]; ok {
			opening, err = setRawXMLAttribute(opening, prefix, name, value)
			if err != nil {
				return err
			}
		}
	}
	var patched []byte
	switch {
	case ind != nil:
		patched, err = applyXMLReplacements(data, []xmlReplacement{{start: ind.start, end: ind.openEnd, value: opening}})
	case props != nil:
		patched, err = insertElementContent(data, props, opening)
	default:
		patched, err = insertElementContentAtStart(data, p, wrapWordElement(prefix, "pPr", opening))
	}
	if err != nil {
		return err
	}
	if bytes.Equal(data, patched) {
		return nil
	}
	return d.applyWordDocumentXMLLocked(patched)
}
