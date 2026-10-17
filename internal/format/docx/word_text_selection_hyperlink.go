package docx

import (
	"bytes"
	"errors"
	"fmt"
	"glance/internal/document"
	"sort"
	"strings"
)

type wordHyperlinkSelectionTarget struct {
	target wordTextTarget
	range_ wordSelectedTextRange
}

// InsertHyperlinkForTextSelection links only the selected text range. It keeps
// each selected run's existing formatting when the display text is unchanged,
// and preserves all unselected text when the display text is edited.
func (d *Document) InsertHyperlinkForTextSelection(startRunID string, startOffset int, endRunID string, endOffset int, displayText, targetURL string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return errors.New("DOCX document part is unavailable")
	}
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return errors.New("超链接地址不能为空")
	}
	if strings.TrimSpace(displayText) == "" {
		return errors.New("超链接显示文本不能为空")
	}
	start, ok := d.wordTextTargets[startRunID]
	if !ok || start.text == nil || start.run == nil || start.paragraph == nil {
		return fmt.Errorf("Word text run %q is not editable", startRunID)
	}
	end, ok := d.wordTextTargets[endRunID]
	if !ok || end.text == nil || end.run == nil || end.paragraph == nil {
		return fmt.Errorf("Word text run %q is not editable", endRunID)
	}
	if startOffset < 0 || startOffset > len([]rune(start.text.text)) || endOffset < 0 || endOffset > len([]rune(end.text.text)) || start.text.start > end.text.start || (start.text == end.text && startOffset >= endOffset) {
		return errors.New("Word hyperlink selection endpoints are outside their runs or not ordered")
	}
	if start.paragraph != end.paragraph {
		return errors.New("Word hyperlink selection cannot cross paragraphs")
	}
	if start.run.parent != start.paragraph || end.run.parent != end.paragraph {
		return errors.New("Word hyperlink selection endpoints must belong to direct paragraph runs")
	}

	data := d.parts[d.documentIdx].data
	root, err := parseWordXML(data)
	if err != nil {
		return fmt.Errorf("parse DOCX before hyperlink insertion: %w", err)
	}
	if err := validateWordSelectionTextOnly(data, start.paragraph, start.text.start, end.text.end); err != nil {
		return fmt.Errorf("Word hyperlink selection is not safe to edit: %w", err)
	}

	selected := make([]wordHyperlinkSelectionTarget, 0)
	for _, target := range d.wordTextTargets {
		if target.text == nil || target.run == nil || target.paragraph != start.paragraph || target.text.start < start.text.start || target.text.start > end.text.start {
			continue
		}
		runes := []rune(target.text.text)
		from, to := 0, len(runes)
		if target.text == start.text {
			from = startOffset
		}
		if target.text == end.text {
			to = endOffset
		}
		if from < 0 || to > len(runes) || from >= to {
			continue
		}
		selected = append(selected, wordHyperlinkSelectionTarget{target: target, range_: wordSelectedTextRange{start: from, end: to}})
	}
	if len(selected) == 0 {
		return errors.New("Word hyperlink selection contains no editable text")
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].target.text.start < selected[j].target.text.start })
	selectedText := selectedWordText(selected)
	rangesByRun := make(map[*wordXMLNode]map[*wordXMLNode]wordSelectedTextRange)
	runs := make([]*wordXMLNode, 0)
	for _, item := range selected {
		if rangesByRun[item.target.run] == nil {
			rangesByRun[item.target.run] = make(map[*wordXMLNode]wordSelectedTextRange)
			runs = append(runs, item.target.run)
		}
		rangesByRun[item.target.run][item.target.text] = item.range_
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].start < runs[j].start })
	for i := 1; i < len(runs); i++ {
		if !onlyXMLWhitespace(data[runs[i-1].end:runs[i].start]) {
			return errors.New("Word hyperlink selection crosses unsupported inline content")
		}
	}

	underline := true
	linkColor := "0563C1"
	updates, order, err := wordStyleUpdates(document.WordTextFormatPatch{Underline: &underline, FontColor: &linkColor})
	if err != nil {
		return err
	}
	var prefix, linkedRuns, suffix bytes.Buffer
	for _, run := range runs {
		fragments, err := splitWordRunForSelection(data, run, rangesByRun[run], start.text.start, end.text.end)
		if err != nil {
			return fmt.Errorf("split selected Word run for hyperlink: %w", err)
		}
		if fragments.prefix.Len() > 0 {
			prefix.Write(wrapWordRunFragment(data, run, fragments.props, fragments.prefix.Bytes()))
		}
		selectedRun := wrapWordRunFragment(data, run, fragments.props, fragments.selected.Bytes())
		selectedRun, err = patchedWordRunFragment(selectedRun, updates, order)
		if err != nil {
			return fmt.Errorf("apply standard hyperlink formatting: %w", err)
		}
		linkedRuns.Write(selectedRun)
		if fragments.suffix.Len() > 0 {
			suffix.Write(wrapWordRunFragment(data, run, fragments.props, fragments.suffix.Bytes()))
		}
	}
	if displayText != selectedText {
		first := selected[0]
		content, err := wordTextNodeXML(data, first.target.text, displayText)
		if err != nil {
			return fmt.Errorf("encode hyperlink display text: %w", err)
		}
		selectedRun := wrapWordRunFragment(data, runs[0], runProperties(data, runs[0]), content)
		selectedRun, err = patchedWordRunFragment(selectedRun, updates, order)
		if err != nil {
			return fmt.Errorf("format hyperlink display text: %w", err)
		}
		linkedRuns.Reset()
		linkedRuns.Write(selectedRun)
	}

	prefixName := elementPrefix(root.rawName)
	if prefixName == "" {
		prefixName = "w"
	}
	isInternal := strings.HasPrefix(targetURL, "#")
	relID := ""
	var hyperlink bytes.Buffer
	if isInternal {
		anchor := strings.TrimPrefix(targetURL, "#")
		fmt.Fprintf(&hyperlink, `<%s:hyperlink %s:anchor="%s">`, prefixName, prefixName, escapeXMLString(anchor))
	} else {
		relID = d.nextWordRelationshipID()
		fmt.Fprintf(&hyperlink, `<%s:hyperlink xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:id="%s">`, prefixName, escapeXMLString(relID))
	}
	hyperlink.Write(linkedRuns.Bytes())
	fmt.Fprintf(&hyperlink, `</%s:hyperlink>`, prefixName)

	var replacement bytes.Buffer
	replacement.Write(prefix.Bytes())
	replacement.Write(hyperlink.Bytes())
	replacement.Write(suffix.Bytes())
	patched, err := applyXMLReplacements(data, []xmlReplacement{{start: runs[0].start, end: runs[len(runs)-1].end, value: replacement.Bytes()}})
	if err != nil {
		return fmt.Errorf("insert Word hyperlink XML: %w", err)
	}
	parts := append([]part(nil), d.parts...)
	parts[d.documentIdx].data = patched
	if _, err := projectWordDocument(parts, d.documentIdx); err != nil {
		return fmt.Errorf("validate Word hyperlink edit: %w", err)
	}
	if isInternal {
		return d.applyWordDocumentXMLLocked(patched)
	}
	relPart, err := d.addWordHyperlinkRelationship(relID, targetURL)
	if err != nil {
		return fmt.Errorf("create external hyperlink relationship: %w", err)
	}
	changes := map[string]*part{
		"word/document.xml":            d.partWithData("word/document.xml", patched),
		"word/_rels/document.xml.rels": relPart,
	}
	return d.commitWordPackageChange(changes)
}

func runProperties(data []byte, run *wordXMLNode) [][]byte {
	for _, child := range run.children {
		if child.name.Space == wordprocessingMLNamespace && child.name.Local == "rPr" {
			return [][]byte{append([]byte(nil), data[child.start:child.end]...)}
		}
	}
	return nil
}

func selectedWordText(selected []wordHyperlinkSelectionTarget) string {
	var text strings.Builder
	for _, item := range selected {
		runes := []rune(item.target.text.text)
		text.WriteString(string(runes[item.range_.start:item.range_.end]))
	}
	return text.String()
}

var _ document.WordTextSelectionHyperlinkEditor = (*Document)(nil)
