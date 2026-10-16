package docx

import (
	"bytes"
	"fmt"
	"glance/internal/document"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type wordPropertyUpdate struct {
	attribute string
	value     string
	remove    bool
}

func (d *Document) SetTextRunStyle(runID string, patch document.WordTextFormatPatch) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	target, ok := d.wordTextTargets[runID]
	if !ok || target.run == nil {
		return fmt.Errorf("Word text run %q is not editable", runID)
	}
	currentStyle := parseWordTextStyle(firstChild(target.run, "rPr"))
	if patch.Bold != nil && *patch.Bold == currentStyle.Bold {
		patch.Bold = nil
	}
	if patch.Italic != nil && *patch.Italic == currentStyle.Italic {
		patch.Italic = nil
	}
	if patch.Underline != nil && *patch.Underline == currentStyle.Underline {
		patch.Underline = nil
	}
	if patch.FontColor != nil {
		color, err := normalizeWordColor(*patch.FontColor)
		if err != nil {
			return err
		}
		if color == currentStyle.FontColor {
			patch.FontColor = nil
		} else {
			patch.FontColor = &color
		}
	}
	if patch.Highlight != nil {
		highlight, err := normalizeWordHighlight(*patch.Highlight)
		if err != nil {
			return err
		}
		if highlight == currentStyle.Highlight {
			patch.Highlight = nil
		} else {
			patch.Highlight = &highlight
		}
	}
	updates, order, err := wordStyleUpdates(patch)
	if err != nil {
		return err
	}
	if len(order) == 0 {
		return nil
	}
	data := d.parts[d.documentIdx].data
	patched, err := patchRunProperties(data, target.run, updates, order)
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
}

func (d *Document) SetTextRunFontSize(runID string, points float64) error {
	if math.IsNaN(points) || math.IsInf(points, 0) || points < 1 || points > 1638 {
		return fmt.Errorf("Word font size must be between 1 and 1638 points")
	}
	halfPoints := int64(math.Round(points * 2))
	if halfPoints < 2 || halfPoints > 3276 {
		return fmt.Errorf("Word font size is outside the supported half-point range")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	target, ok := d.wordTextTargets[runID]
	if !ok || target.run == nil {
		return fmt.Errorf("Word text run %q is not editable", runID)
	}
	if current := parseWordTextStyle(firstChild(target.run, "rPr")).FontSize; current == float64(halfPoints)/2 {
		return nil
	}
	updates := map[string]wordPropertyUpdate{
		"sz": {attribute: "val", value: strconv.FormatInt(halfPoints, 10)},
	}
	patched, err := patchRunProperties(d.parts[d.documentIdx].data, target.run, updates, []string{"sz"})
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
}

func (d *Document) SetTextRunFontFamily(runID, family string) error {
	family = strings.TrimSpace(family)
	if family == "" || utf8.RuneCountInString(family) > 128 || strings.IndexFunc(family, unicode.IsControl) >= 0 {
		return fmt.Errorf("Word font family must contain 1–128 printable characters")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	target, ok := d.wordTextTargets[runID]
	if !ok || target.run == nil {
		return fmt.Errorf("Word text run %q is not editable", runID)
	}
	properties := firstChild(target.run, "rPr")
	if current := parseWordTextStyle(properties); current.FontFamily == family &&
		(current.FontFamilyASCII == "" || current.FontFamilyASCII == family) &&
		(current.FontFamilyHighANSI == "" || current.FontFamilyHighANSI == family) &&
		(current.FontFamilyEastAsia == "" || current.FontFamilyEastAsia == family) &&
		(current.FontFamilyComplexScript == "" || current.FontFamilyComplexScript == family) {
		return nil
	}

	data := d.parts[d.documentIdx].data
	prefix := elementPrefix(target.run.rawName)
	fonts := (*wordXMLNode)(nil)
	if properties != nil {
		fonts = firstChild(properties, "rFonts")
	}
	var patched []byte
	var err error
	if fonts == nil {
		markup := makeWordPropertyMarkup(prefix, "rFonts", "ascii", family)
		for _, attribute := range []string{"hAnsi", "eastAsia", "cs"} {
			markup, err = setRawXMLAttribute(markup, prefix, attribute, family)
			if err != nil {
				return err
			}
		}
		if properties == nil {
			patched, err = insertElementContentAtStart(data, target.run, wrapWordProperties(prefix, markup))
		} else {
			patched, err = insertElementContent(data, properties, markup)
		}
	} else {
		opening := data[fonts.start:fonts.openEnd]
		for _, attribute := range []string{"ascii", "hAnsi", "eastAsia", "cs"} {
			opening, err = setRawXMLAttribute(opening, elementPrefix(fonts.rawName), attribute, family)
			if err != nil {
				return err
			}
		}
		for _, attribute := range []string{"asciiTheme", "hAnsiTheme", "eastAsiaTheme", "cstheme"} {
			opening, err = removeRawXMLAttribute(opening, elementPrefix(fonts.rawName), attribute)
			if err != nil {
				return err
			}
		}
		patched, err = applyXMLReplacements(data, []xmlReplacement{{start: fonts.start, end: fonts.openEnd, value: opening}})
	}
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
}

func (d *Document) SetParagraphAlignment(paragraphID string, alignment document.WordAlignment) error {
	return d.SetParagraphAlignments([]string{paragraphID}, alignment)
}

func (d *Document) SetParagraphAlignments(paragraphIDs []string, alignment document.WordAlignment) error {
	if len(paragraphIDs) == 0 {
		return fmt.Errorf("at least one Word paragraph must be selected")
	}
	value, err := wordAlignmentValue(alignment)
	if err != nil {
		return err
	}
	type target struct {
		id        string
		paragraph *wordXMLNode
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(paragraphIDs))
	targets := make([]target, 0, len(paragraphIDs))
	for _, id := range paragraphIDs {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		paragraph := d.wordParagraphTargets[id]
		if paragraph == nil {
			return fmt.Errorf("Word paragraph %q is not editable", id)
		}
		targets = append(targets, target{id: id, paragraph: paragraph})
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].paragraph.start > targets[j].paragraph.start
	})
	data := d.parts[d.documentIdx].data
	patched := data
	for _, target := range targets {
		paragraph := target.paragraph
		pPr := firstChild(paragraph, "pPr")
		jc := firstChild(pPr, "jc")
		if jc != nil && attrValue(jc, "val") == value {
			continue
		}
		switch {
		case jc != nil:
			opening := patched[jc.start:jc.openEnd]
			opening, err = setRawXMLAttribute(opening, elementPrefix(jc.rawName), "val", value)
			if err == nil {
				patched, err = applyXMLReplacements(patched, []xmlReplacement{{start: jc.start, end: jc.openEnd, value: opening}})
			}
		case pPr != nil:
			markup := makeWordPropertyMarkup(rawWordPrefix(pPr.rawName), "jc", "val", value)
			patched, err = insertElementContent(patched, pPr, markup)
		default:
			prefix := rawWordPrefix(paragraph.rawName)
			markup := wrapWordElement(prefix, "pPr", makeWordPropertyMarkup(prefix, "jc", "val", value))
			patched, err = insertElementContentAtStart(patched, paragraph, markup)
		}
		if err != nil {
			return fmt.Errorf("set Word paragraph %q alignment: %w", target.id, err)
		}
	}
	if bytes.Equal(data, patched) {
		return nil
	}
	return d.applyWordDocumentXMLLocked(patched)
}

func (d *Document) SetParagraphIndents(paragraphID string, leftEMU, rightEMU int64) error {
	leftTwips, err := wordIndentEMUToTwips(leftEMU)
	if err != nil {
		return fmt.Errorf("left paragraph indent: %w", err)
	}
	rightTwips, err := wordIndentEMUToTwips(rightEMU)
	if err != nil {
		return fmt.Errorf("right paragraph indent: %w", err)
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
	var indent *wordXMLNode
	if properties != nil {
		indent = firstChild(properties, "ind")
	}
	leftName := wordIndentAttributeName(indent, "left", "start")
	rightName := wordIndentAttributeName(indent, "right", "end")
	var currentLeftEMU, currentRightEMU int64
	if indent != nil {
		currentLeftEMU = parseWordIndentEMU(attrValue(indent, leftName))
		currentRightEMU = parseWordIndentEMU(attrValue(indent, rightName))
	}
	if currentLeftEMU == leftTwips*635 && currentRightEMU == rightTwips*635 {
		return nil
	}
	leftValue, rightValue := strconv.FormatInt(leftTwips, 10), strconv.FormatInt(rightTwips, 10)
	prefix := elementPrefix(paragraph.rawName)
	data := d.parts[d.documentIdx].data
	var patched []byte
	if properties == nil {
		property := makeWordPropertyMarkup(prefix, "ind", leftName, leftValue)
		property, err = setRawXMLAttribute(property, prefix, rightName, rightValue)
		if err != nil {
			return err
		}
		patched, err = insertElementContentAtStart(data, paragraph, wrapWordElement(prefix, "pPr", property))
	} else if indent == nil {
		property := makeWordPropertyMarkup(prefix, "ind", leftName, leftValue)
		property, err = setRawXMLAttribute(property, prefix, rightName, rightValue)
		if err != nil {
			return err
		}
		patched, err = insertElementContent(data, properties, property)
	} else {
		opening := data[indent.start:indent.openEnd]
		patchedOpening, patchErr := setRawXMLAttribute(opening, prefix, leftName, leftValue)
		if patchErr != nil {
			return patchErr
		}
		patchedOpening, patchErr = setRawXMLAttribute(patchedOpening, prefix, rightName, rightValue)
		if patchErr != nil {
			return patchErr
		}
		patched, err = applyXMLReplacements(data, []xmlReplacement{{start: indent.start, end: indent.openEnd, value: patchedOpening}})
	}
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
}

func wordIndentAttributeName(node *wordXMLNode, physical, logical string) string {
	if node != nil {
		for _, attribute := range node.attrs {
			if attribute.Name.Local == physical {
				return physical
			}
		}
		for _, attribute := range node.attrs {
			if attribute.Name.Local == logical {
				return logical
			}
		}
	}
	return physical
}

func wordIndentEMUToTwips(value int64) (int64, error) {
	const maxTwips = int64(1<<31 - 1)
	const maxEMU = maxTwips * 635
	if value > maxEMU || value < -maxEMU {
		return 0, fmt.Errorf("value exceeds the supported DOCX range")
	}
	if value < 0 {
		return -((-value + 317) / 635), nil
	}
	return (value + 317) / 635, nil
}

func wordStyleUpdates(patch document.WordTextFormatPatch) (map[string]wordPropertyUpdate, []string, error) {
	updates := make(map[string]wordPropertyUpdate)
	order := make([]string, 0, 4)
	add := func(name string, update wordPropertyUpdate) {
		updates[name] = update
		order = append(order, name)
	}
	if patch.Bold != nil {
		value := "0"
		if *patch.Bold {
			value = "1"
		}
		add("b", wordPropertyUpdate{attribute: "val", value: value})
	}
	if patch.Italic != nil {
		value := "0"
		if *patch.Italic {
			value = "1"
		}
		add("i", wordPropertyUpdate{attribute: "val", value: value})
	}
	if patch.Underline != nil {
		value := "none"
		if *patch.Underline {
			value = "single"
		}
		add("u", wordPropertyUpdate{attribute: "val", value: value})
	}
	if patch.FontColor != nil {
		color, err := normalizeWordColor(*patch.FontColor)
		if err != nil {
			return nil, nil, err
		}
		add("color", wordPropertyUpdate{attribute: "val", value: color, remove: color == ""})
	}
	if patch.Highlight != nil {
		highlight, err := normalizeWordHighlight(*patch.Highlight)
		if err != nil {
			return nil, nil, err
		}
		add("highlight", wordPropertyUpdate{attribute: "val", value: highlight, remove: highlight == ""})
	}
	return updates, order, nil
}

func normalizeWordHighlight(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") {
		return "", nil
	}
	for _, supported := range []string{
		"black", "blue", "cyan", "darkBlue", "darkCyan", "darkGray", "darkGreen", "darkMagenta", "darkRed", "darkYellow", "green", "magenta", "red", "white", "yellow", "lightGray",
	} {
		if strings.EqualFold(value, supported) {
			return supported, nil
		}
	}
	return "", fmt.Errorf("unsupported Word text highlight color %q", value)
}

func normalizeWordColor(value string) (string, error) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "#"))
	if value == "" {
		return "", nil
	}
	if len(value) != 6 {
		return "", fmt.Errorf("Word font color must be a six-digit RGB value")
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F') {
			return "", fmt.Errorf("Word font color must be a six-digit RGB value")
		}
	}
	return strings.ToUpper(value), nil
}

func patchRunProperties(data []byte, run *wordXMLNode, updates map[string]wordPropertyUpdate, order []string) ([]byte, error) {
	properties := firstChild(run, "rPr")
	if properties == nil {
		var markup []byte
		prefix := elementPrefix(run.rawName)
		for _, name := range order {
			update := updates[name]
			if update.remove {
				continue
			}
			markup = append(markup, makeWordPropertyMarkup(prefix, name, update.attribute, update.value)...)
		}
		if len(markup) == 0 {
			return data, nil
		}
		return insertElementContentAtStart(data, run, wrapWordProperties(prefix, markup))
	}

	var replacements []xmlReplacement
	var missing []byte
	prefix := elementPrefix(properties.rawName)
	for _, name := range order {
		update := updates[name]
		child := firstChild(properties, name)
		if child == nil {
			if !update.remove {
				missing = append(missing, makeWordPropertyMarkup(prefix, name, update.attribute, update.value)...)
			}
			continue
		}
		if update.remove {
			replacements = append(replacements, xmlReplacement{start: child.start, end: child.end})
			continue
		}
		opening := data[child.start:child.openEnd]
		patchedOpening, err := setRawXMLAttribute(opening, elementPrefix(child.rawName), update.attribute, update.value)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(opening, patchedOpening) {
			replacements = append(replacements, xmlReplacement{start: child.start, end: child.openEnd, value: patchedOpening})
		}
	}
	if len(missing) != 0 {
		if properties.selfClosing {
			opening, err := expandSelfClosingTag(data[properties.start:properties.openEnd])
			if err != nil {
				return nil, err
			}
			replacement := append([]byte(nil), opening...)
			replacement = append(replacement, missing...)
			replacement = append(replacement, []byte("</"+properties.rawName+">")...)
			replacements = append(replacements, xmlReplacement{start: properties.start, end: properties.end, value: replacement})
		} else {
			replacements = append(replacements, xmlReplacement{start: properties.contentEnd, end: properties.contentEnd, value: missing})
		}
	}
	return applyXMLReplacements(data, replacements)
}

func wordAlignmentValue(alignment document.WordAlignment) (string, error) {
	switch alignment {
	case document.WordAlignLeft:
		return "left", nil
	case document.WordAlignCenter:
		return "center", nil
	case document.WordAlignRight:
		return "right", nil
	case document.WordAlignJustified:
		return "both", nil
	default:
		return "", fmt.Errorf("unsupported Word paragraph alignment %q", alignment)
	}
}
