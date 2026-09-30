package docx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"
)

type xmlReplacement struct {
	start int
	end   int
	value []byte
}

type rawXMLAttribute struct {
	name      string
	prefix    string
	local     string
	valueFrom int
	valueTo   int
	quote     byte
}

type wordPropertyUpdate struct {
	attribute string
	value     string
	remove    bool
}

func (d *Document) ReplaceTextRun(runID, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	return d.replaceTextRunLocked(runID, value)
}

func (d *Document) replaceTextRunLocked(runID, value string) error {
	target, ok := d.wordTextTargets[runID]
	if !ok || target.text == nil || target.run == nil {
		return fmt.Errorf("Word text run %q is not editable", runID)
	}
	if target.text.text == value {
		return nil
	}
	current := d.parts[d.documentIdx].data
	patched, err := replaceWordTextNode(current, target.text, value)
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
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

func (d *Document) SetParagraphAlignment(paragraphID string, alignment document.WordAlignment) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	paragraph, ok := d.wordParagraphTargets[paragraphID]
	if !ok || paragraph == nil {
		return fmt.Errorf("Word paragraph %q is not editable", paragraphID)
	}
	value, err := wordAlignmentValue(alignment)
	if err != nil {
		return err
	}
	data := d.parts[d.documentIdx].data
	pPr := firstChild(paragraph, "pPr")
	var patched []byte
	if pPr == nil {
		prefix := elementPrefix(paragraph.rawName)
		markup := wrapWordElement(prefix, "pPr", makeWordPropertyMarkup(prefix, "jc", "val", value))
		patched, err = insertElementContent(data, paragraph, markup)
	} else if jc := firstChild(pPr, "jc"); jc != nil {
		patched, err = patchElementAttribute(data, jc, "val", value)
	} else {
		prefix := elementPrefix(pPr.rawName)
		markup := makeWordPropertyMarkup(prefix, "jc", "val", value)
		patched, err = insertElementContent(data, pPr, markup)
	}
	if err != nil {
		return err
	}
	return d.applyWordDocumentXMLLocked(patched)
}

func (d *Document) applyWordDocumentXMLLocked(patched []byte) error {
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return errors.New("DOCX document part is unavailable")
	}
	current := d.parts[d.documentIdx].data
	if bytes.Equal(current, patched) {
		return nil
	}
	before := d.captureWordHistoryLocked("word/document.xml")
	d.parts[d.documentIdx].data = patched
	if err := d.reparse(); err != nil {
		d.parts[d.documentIdx].data = current
		if restoreErr := d.reparse(); restoreErr != nil {
			return fmt.Errorf("Word edit failed (%v) and previous content could not be restored (%v)", err, restoreErr)
		}
		return fmt.Errorf("reparse DOCX after Word edit: %w", err)
	}
	d.recordWordHistoryLocked(before)
	return nil
}

func replaceWordTextNode(data []byte, node *wordXMLNode, value string) ([]byte, error) {
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(value)); err != nil {
		return nil, err
	}
	rawOpen := append([]byte(nil), data[node.start:node.openEnd]...)
	if needsWordSpacePreserve(value) && !rawAttributeEquals(rawOpen, "xml", "space", "preserve") {
		var err error
		rawOpen, err = setRawXMLAttribute(rawOpen, "xml", "space", "preserve")
		if err != nil {
			return nil, err
		}
	}
	if node.selfClosing {
		open, err := expandSelfClosingTag(rawOpen)
		if err != nil {
			return nil, err
		}
		replacement := make([]byte, 0, len(open)+escaped.Len()+len(node.rawName)+3)
		replacement = append(replacement, open...)
		replacement = append(replacement, escaped.Bytes()...)
		replacement = append(replacement, []byte("</"+node.rawName+">")...)
		return applyXMLReplacements(data, []xmlReplacement{{start: node.start, end: node.end, value: replacement}})
	}
	changes := []xmlReplacement{{start: node.contentStart, end: node.contentEnd, value: escaped.Bytes()}}
	if !bytes.Equal(rawOpen, data[node.start:node.openEnd]) {
		changes = append(changes, xmlReplacement{start: node.start, end: node.openEnd, value: rawOpen})
	}
	return applyXMLReplacements(data, changes)
}

func needsWordSpacePreserve(value string) bool {
	return value != "" && strings.TrimSpace(value) != value
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
	return updates, order, nil
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
		return insertElementContent(data, run, wrapWordProperties(prefix, markup))
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

func wrapWordProperties(prefix string, children []byte) []byte {
	return wrapWordElement(prefix, "rPr", children)
}

func wrapWordElement(prefix, local string, children []byte) []byte {
	name := qualifiedWordName(prefix, local)
	result := make([]byte, 0, len(name)*2+5+len(children))
	result = append(result, '<')
	result = append(result, name...)
	result = append(result, '>')
	result = append(result, children...)
	result = append(result, []byte("</"+name+">")...)
	return result
}

func makeWordPropertyMarkup(prefix, element, attribute, value string) []byte {
	name := qualifiedWordName(prefix, element)
	result := make([]byte, 0, len(name)*2+len(attribute)+len(value)+8)
	result = append(result, '<')
	result = append(result, name...)
	if attribute != "" {
		result = append(result, ' ')
		result = append(result, qualifiedWordName(prefix, attribute)...)
		result = append(result, "=\""...)
		result = append(result, escapeXMLAttribute(value)...)
		result = append(result, '"')
	}
	result = append(result, "/>"...)
	return result
}

func qualifiedWordName(prefix, local string) string {
	if prefix == "" {
		return local
	}
	return prefix + ":" + local
}

func elementPrefix(name string) string {
	if index := strings.IndexByte(name, ':'); index >= 0 {
		return name[:index]
	}
	return "w"
}

func patchElementAttribute(data []byte, node *wordXMLNode, attribute, value string) ([]byte, error) {
	opening := data[node.start:node.openEnd]
	patched, err := setRawXMLAttribute(opening, elementPrefix(node.rawName), attribute, value)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(opening, patched) {
		return data, nil
	}
	return applyXMLReplacements(data, []xmlReplacement{{start: node.start, end: node.openEnd, value: patched}})
}

func insertElementContent(data []byte, node *wordXMLNode, value []byte) ([]byte, error) {
	if len(value) == 0 {
		return data, nil
	}
	if node.selfClosing {
		opening, err := expandSelfClosingTag(data[node.start:node.openEnd])
		if err != nil {
			return nil, err
		}
		replacement := append([]byte(nil), opening...)
		replacement = append(replacement, value...)
		replacement = append(replacement, []byte("</"+node.rawName+">")...)
		return applyXMLReplacements(data, []xmlReplacement{{start: node.start, end: node.end, value: replacement}})
	}
	return applyXMLReplacements(data, []xmlReplacement{{start: node.contentEnd, end: node.contentEnd, value: value}})
}

func expandSelfClosingTag(rawOpen []byte) ([]byte, error) {
	end := bytes.LastIndexByte(rawOpen, '>')
	if end < 0 {
		return nil, errors.New("invalid XML start tag")
	}
	slash := end - 1
	for slash >= 0 && (rawOpen[slash] == ' ' || rawOpen[slash] == '\t' || rawOpen[slash] == '\r' || rawOpen[slash] == '\n') {
		slash--
	}
	if slash < 0 || rawOpen[slash] != '/' {
		return nil, errors.New("XML element is not self-closing")
	}
	result := make([]byte, 0, len(rawOpen)-1)
	result = append(result, rawOpen[:slash]...)
	result = append(result, rawOpen[slash+1:]...)
	return result, nil
}

func setRawXMLAttribute(rawOpen []byte, prefix, local, value string) ([]byte, error) {
	attributes, nameEnd, tagEnd, err := scanRawXMLAttributes(rawOpen)
	if err != nil {
		return nil, err
	}
	escaped := escapeXMLAttribute(value)
	for _, attribute := range attributes {
		if attribute.prefix != prefix || attribute.local != local {
			continue
		}
		result := make([]byte, 0, len(rawOpen)-attribute.valueTo+attribute.valueFrom+len(escaped))
		result = append(result, rawOpen[:attribute.valueFrom]...)
		result = append(result, escaped...)
		result = append(result, rawOpen[attribute.valueTo:]...)
		return result, nil
	}
	name := qualifiedWordName(prefix, local)
	addition := append([]byte(" "+name+`="`), escaped...)
	addition = append(addition, '"')
	insertAt := tagEnd
	for insertAt > nameEnd && (rawOpen[insertAt-1] == ' ' || rawOpen[insertAt-1] == '\t' || rawOpen[insertAt-1] == '\r' || rawOpen[insertAt-1] == '\n') {
		insertAt--
	}
	if insertAt > nameEnd && rawOpen[insertAt-1] == '/' {
		insertAt--
	}
	result := make([]byte, 0, len(rawOpen)+len(addition))
	result = append(result, rawOpen[:insertAt]...)
	result = append(result, addition...)
	result = append(result, rawOpen[insertAt:]...)
	return result, nil
}

func rawAttributeEquals(rawOpen []byte, prefix, local, value string) bool {
	attributes, _, _, err := scanRawXMLAttributes(rawOpen)
	if err != nil {
		return false
	}
	for _, attribute := range attributes {
		if attribute.prefix == prefix && attribute.local == local {
			return string(rawOpen[attribute.valueFrom:attribute.valueTo]) == value
		}
	}
	return false
}

func scanRawXMLAttributes(rawOpen []byte) ([]rawXMLAttribute, int, int, error) {
	end := bytes.LastIndexByte(rawOpen, '>')
	if end < 0 || len(rawOpen) == 0 || rawOpen[0] != '<' {
		return nil, 0, 0, errors.New("invalid XML start tag")
	}
	nameEnd := 1
	for nameEnd < end && !xmlSpace(rawOpen[nameEnd]) && rawOpen[nameEnd] != '/' {
		nameEnd++
	}
	attributes := make([]rawXMLAttribute, 0)
	for index := nameEnd; index < end; {
		for index < end && (xmlSpace(rawOpen[index]) || rawOpen[index] == '/') {
			index++
		}
		if index >= end {
			break
		}
		nameStart := index
		for index < end && !xmlSpace(rawOpen[index]) && rawOpen[index] != '=' && rawOpen[index] != '/' {
			index++
		}
		if nameStart == index {
			return nil, 0, 0, errors.New("invalid XML attribute name")
		}
		name := string(rawOpen[nameStart:index])
		for index < end && xmlSpace(rawOpen[index]) {
			index++
		}
		if index >= end || rawOpen[index] != '=' {
			return nil, 0, 0, errors.New("invalid XML attribute assignment")
		}
		index++
		for index < end && xmlSpace(rawOpen[index]) {
			index++
		}
		if index >= end || (rawOpen[index] != '\'' && rawOpen[index] != '"') {
			return nil, 0, 0, errors.New("invalid XML attribute quote")
		}
		quote := rawOpen[index]
		index++
		valueFrom := index
		for index < end && rawOpen[index] != quote {
			index++
		}
		if index >= end {
			return nil, 0, 0, errors.New("unterminated XML attribute")
		}
		valueTo := index
		index++
		prefix, local := splitQualifiedName(name)
		attributes = append(attributes, rawXMLAttribute{name: name, prefix: prefix, local: local, valueFrom: valueFrom, valueTo: valueTo, quote: quote})
	}
	return attributes, nameEnd, end, nil
}

func splitQualifiedName(name string) (string, string) {
	if index := strings.IndexByte(name, ':'); index >= 0 {
		return name[:index], name[index+1:]
	}
	return "", name
}

func xmlSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func escapeXMLAttribute(value string) []byte {
	return []byte(strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(value))
}

func applyXMLReplacements(data []byte, replacements []xmlReplacement) ([]byte, error) {
	if len(replacements) == 0 {
		return data, nil
	}
	sortXMLReplacements(replacements)
	result := append([]byte(nil), data...)
	previousStart := len(data) + 1
	for _, replacement := range replacements {
		if replacement.start < 0 || replacement.end < replacement.start || replacement.end > len(data) || replacement.end > previousStart {
			return nil, errors.New("overlapping or out-of-range DOCX XML patch")
		}
		patched := make([]byte, 0, len(result)-(replacement.end-replacement.start)+len(replacement.value))
		patched = append(patched, result[:replacement.start]...)
		patched = append(patched, replacement.value...)
		patched = append(patched, result[replacement.end:]...)
		result = patched
		previousStart = replacement.start
	}
	return result, nil
}

func sortXMLReplacements(replacements []xmlReplacement) {
	for i := 1; i < len(replacements); i++ {
		for j := i; j > 0 && replacements[j].start > replacements[j-1].start; j-- {
			replacements[j], replacements[j-1] = replacements[j-1], replacements[j]
		}
	}
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
