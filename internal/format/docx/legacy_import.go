package docx

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"glance/internal/document"
)

// LegacyImportLayout carries only independently recovered Word binary metadata.
// The converted main story must match before any paragraphs are relocated.
type LegacyImportLayout struct {
	Page                                 document.WordPageSetup
	Grid                                 document.WordDocumentGrid
	HeaderDistancePT, FooterDistancePT   float64
	HeaderDistanceSet, FooterDistanceSet bool
	Paragraphs                           []LegacyImportParagraph
	Stories                              []LegacyImportStory
}
type LegacyImportParagraph struct {
	Text   string
	Layout document.WordParagraphLayout
	Fonts  document.WordTextStyle
}
type LegacyImportStory struct{ Kind, Type, Text string }

// RestoreLegacyImportLayout repairs a private conversion package, never the DOC
// source. Unknown package parts and unrelated XML remain intact. A mismatch is
// a no-op: a text-only converter is not trusted to identify arbitrary content.
func (d *Document) RestoreLegacyImportLayout(layout LegacyImportLayout) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return false, err
	}
	data := d.parts[d.documentIdx].data
	root, err := parseWordXML(data)
	if err != nil {
		return false, err
	}
	body := firstChild(root, "body")
	if body == nil {
		return false, errors.New("legacy conversion has no body")
	}
	var paragraphs []*wordXMLNode
	for _, node := range body.children {
		if node.name.Local == "p" {
			paragraphs = append(paragraphs, node)
		} else if node.name.Local != "sectPr" {
			return false, nil
		}
	}
	if len(layout.Paragraphs) == 0 || len(paragraphs) < len(layout.Paragraphs) {
		return false, nil
	}
	sections := findWordSectionNodes(body)
	if len(sections) != 1 {
		return false, nil
	}
	for i, source := range layout.Paragraphs {
		for _, r := range source.Text {
			if r < 32 && r != '\t' && r != '\v' && r != '\n' && r != '\f' {
				return false, nil
			}
		}
		if legacyConvertedParagraphText(paragraphs[i]) != strings.ReplaceAll(source.Text, "\v", "\n") {
			return false, nil
		}
	}
	prefix := elementPrefix(root.rawName)
	if prefix == "" {
		prefix = "w"
	}
	changes := map[string]*part{}
	relName := "word/_rels/document.xml.rels"
	rel, ok := findPackagePart(d.parts, relName)
	if !ok {
		rel = part{header: zip.FileHeader{Name: relName, Method: zip.Deflate}, data: []byte(`<Relationships xmlns="` + wordPackageRelationshipsNS + `"/>`)}
	}
	ct, ok := findPackagePart(d.parts, "[Content_Types].xml")
	if !ok {
		return false, errors.New("legacy conversion has no content types")
	}
	var refs strings.Builder
	for index, story := range layout.Stories {
		for _, r := range story.Text {
			if r < 32 && r != 13 && r != 19 && r != 20 && r != 21 && r != '\t' && r != '\v' && r != '\n' && r != '\f' {
				return false, nil
			}
		}
		if story.Kind != "header" && story.Kind != "footer" {
			return false, errors.New("invalid recovered document story")
		}
		if story.Type != "default" && story.Type != "even" && story.Type != "first" {
			return false, errors.New("invalid recovered story variant")
		}
		path := fmt.Sprintf("word/legacy-%s-%d.xml", story.Kind, index+1)
		id := fmt.Sprintf("rIdLegacyStory%d", index+1)
		relRoot, err := parseWordXML(rel.data)
		if err != nil {
			return false, err
		}
		for _, existing := range relRoot.children {
			if attrValue(existing, "Id") == id {
				return false, errors.New("recovered story relationship collides with conversion output")
			}
		}
		if _, exists := findPackagePart(d.parts, path); exists {
			return false, errors.New("recovered story part collides with conversion output")
		}
		relType, contentType, tag := wordHeaderRelType, wordHeaderContentType, "headerReference"
		if story.Kind == "footer" {
			relType, contentType, tag = wordFooterRelType, wordFooterContentType, "footerReference"
		}
		newRel, err := ensureDocumentRelPart(&rel, id, relType, strings.TrimPrefix(path, "word/"))
		if err != nil {
			return false, err
		}
		rel = *newRel
		newCt, err := ensureContentTypePart(&ct, "/"+path, contentType)
		if err != nil {
			return false, err
		}
		ct = *newCt
		changes[path] = &part{header: zip.FileHeader{Name: path, Method: zip.Deflate}, data: legacyStoryXML(story)}
		fmt.Fprintf(&refs, `<%s:%s %s:type="%s" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:id="%s"/>`, prefix, tag, prefix, story.Type, id)
	}
	sectionXML, err := legacySectionXML(data, sections[0], prefix, layout, refs.String())
	if err != nil {
		return false, err
	}
	replacements := []xmlReplacement{{start: sections[0].start, end: sections[0].end, value: sectionXML}}
	for i, source := range layout.Paragraphs {
		fontReplacements, err := legacyParagraphFontReplacements(data, paragraphs[i], prefix, source.Fonts)
		if err != nil {
			return false, err
		}
		replacements = append(replacements, fontReplacements...)
		if source.Layout.Set == 0 {
			continue
		}
		p := paragraphs[i]
		ppr := firstChild(p, "pPr")
		markup := legacyParagraphPropertiesXML(data, ppr, prefix, source.Layout)
		if ppr == nil {
			replacements = append(replacements, xmlReplacement{start: p.contentStart, end: p.contentStart, value: markup})
		} else {
			replacements = append(replacements, xmlReplacement{start: ppr.start, end: ppr.end, value: markup})
		}
	}
	for _, p := range paragraphs[len(layout.Paragraphs):] {
		replacements = append(replacements, xmlReplacement{start: p.start, end: p.end})
	}
	patched, err := applyXMLReplacements(data, replacements)
	if err != nil {
		return false, err
	}
	changes["word/document.xml"] = d.partWithData("word/document.xml", patched)
	if len(layout.Stories) > 0 {
		changes[relName] = &rel
		changes["[Content_Types].xml"] = &ct
	}
	if err := d.commitWordPackageChange(changes); err != nil {
		return false, err
	}
	return true, nil
}

func legacyConvertedParagraphText(node *wordXMLNode) string {
	var out strings.Builder
	var visit func(*wordXMLNode)
	visit = func(n *wordXMLNode) {
		switch n.name.Local {
		case "t":
			out.WriteString(n.text)
		case "tab":
			out.WriteByte('\t')
		case "cr":
			out.WriteByte('\n')
		case "br":
			if attrValue(n, "type") == "page" {
				out.WriteByte('\f')
			} else {
				out.WriteByte('\n')
			}
		}
		for _, c := range n.children {
			visit(c)
		}
	}
	visit(node)
	return out.String()
}

func legacySectionXML(data []byte, node *wordXMLNode, prefix string, layout LegacyImportLayout, refs string) ([]byte, error) {
	var out bytes.Buffer
	fmt.Fprintf(&out, "<%s:sectPr>", prefix)
	out.WriteString(refs)
	page := firstChild(node, "pgSz")
	pageXML := []byte(fmt.Sprintf(`<%s:pgSz/>`, prefix))
	if page != nil {
		pageXML = append([]byte(nil), data[page.start:page.end]...)
	}
	margin := firstChild(node, "pgMar")
	marginXML := []byte(fmt.Sprintf(`<%s:pgMar/>`, prefix))
	if margin != nil {
		marginXML = append([]byte(nil), data[margin.start:margin.end]...)
	}
	patch := func(raw []byte, name string, pt float64) ([]byte, error) {
		root, err := parseWordXML(raw)
		if err != nil {
			return nil, err
		}
		return patchElementAttribute(raw, root, name, strconv.Itoa(int(math.Round(pt*20))))
	}
	var err error
	if layout.Page.Set&document.WordPageOrientationSet != 0 {
		n, err := parseWordXML(pageXML)
		if err != nil {
			return nil, err
		}
		pageXML, err = patchElementAttribute(pageXML, n, "orient", layout.Page.Orientation)
		if err != nil {
			return nil, err
		}
	}
	for _, v := range []struct {
		name  string
		mask  document.WordPageSetupMask
		value float64
	}{{"w", document.WordPageWidthSet, layout.Page.WidthPT}, {"h", document.WordPageHeightSet, layout.Page.HeightPT}} {
		if layout.Page.Set&v.mask != 0 {
			pageXML, err = patch(pageXML, v.name, v.value)
			if err != nil {
				return nil, err
			}
		}
	}
	for _, v := range []struct {
		name  string
		mask  document.WordPageSetupMask
		value float64
	}{{"left", document.WordPageMarginLeftSet, layout.Page.MarginLeftPT}, {"right", document.WordPageMarginRightSet, layout.Page.MarginRightPT}, {"top", document.WordPageMarginTopSet, layout.Page.MarginTopPT}, {"bottom", document.WordPageMarginBottomSet, layout.Page.MarginBottomPT}} {
		if layout.Page.Set&v.mask != 0 {
			marginXML, err = patch(marginXML, v.name, v.value)
			if err != nil {
				return nil, err
			}
		}
	}
	for _, v := range []struct {
		name  string
		value float64
		set   bool
	}{{"header", layout.HeaderDistancePT, layout.HeaderDistanceSet}, {"footer", layout.FooterDistancePT, layout.FooterDistanceSet}} {
		if !v.set {
			continue
		}
		marginXML, err = patch(marginXML, v.name, v.value)
		if err != nil {
			return nil, err
		}
	}
	out.Write(pageXML)
	out.Write(marginXML)
	for _, child := range node.children {
		switch child.name.Local {
		case "pgSz", "pgMar", "docGrid", "headerReference", "footerReference":
			continue
		}
		out.Write(data[child.start:child.end])
	}
	if layout.Grid.Type != "" {
		typ := layout.Grid.Type
		if typ == "linesandchars" {
			typ = "linesAndChars"
		}
		if typ == "snaptochars" {
			typ = "snapToChars"
		}
		fmt.Fprintf(&out, `<%s:docGrid %s:type="%s" %s:linePitch="%d"/>`, prefix, prefix, escapeXMLString(typ), prefix, int(math.Round(layout.Grid.LinePitchPT*20)))
	}
	fmt.Fprintf(&out, "</%s:sectPr>", prefix)
	return orderLegacyPropertiesXML(out.Bytes(), wordSectionPropertyOrder)
}

func legacyParagraphPropertiesXML(data []byte, node *wordXMLNode, prefix string, layout document.WordParagraphLayout) []byte {
	var out bytes.Buffer
	fmt.Fprintf(&out, "<%s:pPr>", prefix)
	if node != nil {
		for _, child := range node.children {
			if child.name.Local == "spacing" || child.name.Local == "pageBreakBefore" && layout.Set&document.WordParagraphPageBreakBeforeSet != 0 || child.name.Local == "snapToGrid" && layout.Set&document.WordParagraphSnapToGridSet != 0 {
				continue
			}
			out.Write(data[child.start:child.end])
		}
	}
	if layout.Set&document.WordParagraphPageBreakBeforeSet != 0 {
		fmt.Fprintf(&out, `<%s:pageBreakBefore %s:val="%t"/>`, prefix, prefix, layout.PageBreakBefore)
	}
	if layout.Set&document.WordParagraphSnapToGridSet != 0 {
		fmt.Fprintf(&out, `<%s:snapToGrid %s:val="%t"/>`, prefix, prefix, layout.SnapToGrid)
	}
	attrs := map[string]string{}
	if node != nil {
		if old := firstChild(node, "spacing"); old != nil {
			for _, a := range old.attrs {
				attrs[a.Name.Local] = a.Value
			}
		}
	}
	if layout.Set&document.WordParagraphLineSpacingSet != 0 {
		rule := layout.LineSpacingRule
		value := layout.LineSpacingPT * 20
		if rule == "auto" {
			value = layout.LineSpacingMultiple * 240
		}
		if rule == "atleast" {
			rule = "atLeast"
		}
		attrs["line"], attrs["lineRule"] = strconv.Itoa(int(math.Round(value))), rule
	}
	if layout.Set&document.WordParagraphBeforeSet != 0 {
		attrs["before"] = strconv.Itoa(int(math.Round(layout.BeforePT * 20)))
	}
	if layout.Set&document.WordParagraphAfterSet != 0 {
		attrs["after"] = strconv.Itoa(int(math.Round(layout.AfterPT * 20)))
	}
	if len(attrs) > 0 {
		fmt.Fprintf(&out, "<%s:spacing", prefix)
		for _, name := range []string{"before", "after", "beforeLines", "afterLines", "beforeAutospacing", "afterAutospacing", "line", "lineRule"} {
			if value, ok := attrs[name]; ok {
				fmt.Fprintf(&out, ` %s:%s="%s"`, prefix, name, escapeXMLString(value))
			}
		}
		out.WriteString("/>")
	}
	fmt.Fprintf(&out, "</%s:pPr>", prefix)
	ordered, err := orderLegacyPropertiesXML(out.Bytes(), wordParagraphPropertyOrder)
	if err != nil {
		return out.Bytes()
	}
	return ordered
}

// Field delimiters remain field delimiters. Their instruction text is never
// reintroduced into the document body as a visible "PAGE" label.
func legacyStoryXML(story LegacyImportStory) []byte {
	root := "hdr"
	if story.Kind == "footer" {
		root = "ftr"
	}
	var out strings.Builder
	fmt.Fprintf(&out, `<?xml version="1.0" encoding="UTF-8"?><w:%s xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p>`, root)
	var text strings.Builder
	var fieldInstructions []bool
	flush := func() {
		if text.Len() == 0 {
			return
		}
		tag := "t"
		if len(fieldInstructions) > 0 && fieldInstructions[len(fieldInstructions)-1] {
			tag = "instrText"
		}
		fmt.Fprintf(&out, `<w:r><w:%s xml:space="preserve">%s</w:%s></w:r>`, tag, escapeXMLString(text.String()), tag)
		text.Reset()
	}
	for _, r := range story.Text {
		switch r {
		case 13:
			flush()
			out.WriteString("</w:p><w:p>")
		case 19, 20, 21:
			flush()
			typ := "begin"
			if r == 20 {
				typ = "separate"
			}
			if r == 21 {
				typ = "end"
			}
			fmt.Fprintf(&out, `<w:r><w:fldChar w:fldCharType="%s"/></w:r>`, typ)
			switch r {
			case 19:
				fieldInstructions = append(fieldInstructions, true)
			case 20:
				if len(fieldInstructions) > 0 {
					fieldInstructions[len(fieldInstructions)-1] = false
				}
			case 21:
				if len(fieldInstructions) > 0 {
					fieldInstructions = fieldInstructions[:len(fieldInstructions)-1]
				}
			}
		case '\t':
			flush()
			out.WriteString("<w:r><w:tab/></w:r>")
		case '\v', '\n':
			flush()
			out.WriteString("<w:r><w:br/></w:r>")
		default:
			if r >= 32 {
				text.WriteRune(r)
			}
		}
	}
	flush()
	fmt.Fprintf(&out, "</w:p></w:%s>", root)
	return []byte(out.String())
}

func orderLegacyPropertiesXML(data []byte, order []string) ([]byte, error) {
	root, err := parseWordXML(data)
	if err != nil {
		return nil, err
	}
	ranks := map[string]int{}
	for i, name := range order {
		ranks[name] = i
	}
	children := append([]*wordXMLNode(nil), root.children...)
	rank := func(n *wordXMLNode) int {
		if v, ok := ranks[n.name.Local]; ok {
			return v
		}
		return len(order)
	}
	sort.SliceStable(children, func(i, j int) bool { return rank(children[i]) < rank(children[j]) })
	var out bytes.Buffer
	out.Write(data[:root.openEnd])
	for _, child := range children {
		out.Write(data[child.start:child.end])
	}
	out.Write(data[root.contentEnd:])
	return out.Bytes(), nil
}
