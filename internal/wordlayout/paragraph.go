package wordlayout

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"glance/internal/document"
)

type paragraphAtomKind uint8

const (
	atomText paragraphAtomKind = iota + 1
	atomImage
	atomPageBreak
	atomLineBreak
	atomTab
)

type paragraphAtom struct {
	kind            paragraphAtomKind
	sourceIndex     int
	sourceID        string
	runID           string
	text            string
	runeStart       int
	runeEnd         int
	glyphRuneOffset int
	style           document.WordTextStyle
	editable        bool
	glyphs          []ShapedGlyph
	outlines        [][]GlyphPathCommand
	advance         float64
	ascent          float64
	descent         float64
	gap             float64
	imageID         string
	imageRotation   float64
	imageWidth      float64
	imageHeight     float64
	synthetic       bool
	breakBefore     bool
	breakAfter      bool
}

type paragraphLine struct {
	atoms           []paragraphAtom
	width           float64
	ascent          float64
	descent         float64
	gap             float64
	height          float64
	pageBreakBefore bool
	pageBreakAfter  bool
	nextAtomIndex   int
	rangeXPT        float64
	rangeWidthPT    float64
	hasRange        bool
}

type preparedParagraph struct {
	sourceID         string
	borderSourceID   string
	uiBeforePT       float64
	uiAfterPT        float64
	style            resolvedParagraphStyle
	defaultTabStopPT float64
	gridLinePitchPT  float64
	atoms            []paragraphAtom
	maxWidth         float64
	floatingImages   []preparedFloatingImage
	lines            []paragraphLine
}

type preparedFloatingImage struct {
	id       string
	width    float64
	height   float64
	rotation float64
	geometry document.WordImageFloating
}

const unavailableWordImageLabel = "[图片无法显示]"

func shapeUnavailableWordImage(ctx context.Context, shaper TextShaper, pathCache map[glyphPathCacheKey][]GlyphPathCommand, imageID, sourceID string, style document.WordTextStyle) ([]paragraphAtom, error) {
	if imageID == "" {
		imageID = sourceID
	}
	return shapeTextAtoms(ctx, shaper, pathCache, imageID+".image-fallback", unavailableWordImageLabel, 0, style, true, sourceID)
}

func (p preparedParagraph) totalHeight() float64 {
	height := p.uiBeforePT + p.uiAfterPT + p.style.paragraph.BeforePT + p.style.paragraph.AfterPT + paragraphBottomBorderExtent(p.style.paragraph)
	for _, line := range p.lines {
		height += line.height
	}
	return height
}

func paragraphBottomBorderExtent(paragraph document.WordParagraphLayout) float64 {
	border := paragraph.BottomBorder
	if border.Style == "none" || border.Style == "nil" || border.SizePT <= 0 {
		return 0
	}
	return border.SpacePT + border.SizePT
}

func prepareParagraph(ctx context.Context, shaper TextShaper, styles *styleResolver, paragraph document.WordParagraph, sourceID string, availableWidth float64, inCell, editorInsets bool) (preparedParagraph, error) {
	if paragraph.PreviewWarning != "" {
		styles.cursor.diagnostic(DiagnosticWarning, sourceID, "content-preview-fallback", paragraph.PreviewWarning)
	}
	prepared := preparedParagraph{sourceID: sourceID, borderSourceID: sourceID, style: styles.paragraphStyle(paragraph, sourceID), defaultTabStopPT: styles.defaultTabStopPT}
	if sectionIndex := styles.cursor.sectionIndex; sectionIndex >= 0 && sectionIndex < len(styles.sections) && (!inCell || styles.adjustLineHeightInTable) {
		grid := styles.sections[sectionIndex].Grid
		if grid.Type == "lines" {
			prepared.gridLinePitchPT = grid.LinePitchPT
		}
	}
	if err := resolveCharacterIndentUnits(ctx, shaper, styles.cursor, sourceID, prepared.style.text, &prepared.style.paragraph); err != nil {
		return preparedParagraph{}, err
	}
	if editorInsets {
		prepared.uiBeforePT, prepared.uiAfterPT = 3.75, 4.5
		if inCell {
			prepared.uiAfterPT = 2.25
		}
	}
	leftIndent := emuToPoints(prepared.style.paragraph.LeftIndentEMU)
	rightIndent := emuToPoints(prepared.style.paragraph.RightIndentEMU)
	firstOffset := emuToPoints(prepared.style.paragraph.FirstLineIndentEMU - prepared.style.paragraph.HangingIndentEMU)
	imageWidth := availableWidth - leftIndent - rightIndent - math.Max(0, firstOffset)
	if imageWidth <= 0 {
		imageWidth = 1
	}
	atoms := make([]paragraphAtom, 0, len(paragraph.Inlines)+1)
	if paragraph.ListKind == document.WordListUnsupported {
		styles.cursor.diagnostic(DiagnosticWarning, sourceID, "unsupported-list", unsupportedListDetail)
	} else if paragraph.ListKind != document.WordListNone && paragraph.ListMarker != "" {
		marker := paragraph.ListMarker + "\t"
		markerAtoms, err := shapeTextAtoms(ctx, shaper, styles.cursor.glyphPaths, paragraph.ID+".list-marker", marker, 0, prepared.style.text, true, sourceID)
		if err != nil {
			return preparedParagraph{}, err
		}
		atoms = append(atoms, markerAtoms...)
	}
	fieldCodePresent := false
	for _, inline := range paragraph.Inlines {
		if err := ctx.Err(); err != nil {
			return preparedParagraph{}, err
		}
		switch inline.Kind {
		case document.WordTextInline:
			if inline.Text == nil {
				styles.cursor.diagnostic(DiagnosticError, sourceID, "invalid-text-inline", "A text inline has no text run data.")
				continue
			}
			run := inline.Text
			runStyle := prepared.style.text
			styles.applyCharacterStyle(&runStyle, run.CharacterStyle, run.ID)
			mergeTextStyle(&runStyle, run.Style, run.StyleSet)
			if runStyle.FontSize <= 0 {
				runStyle.FontSize = DefaultFontSizePT
			}
			textAtoms, err := shapeTextAtoms(ctx, shaper, styles.cursor.glyphPaths, run.ID, run.Text, 0, runStyle, false, sourceID)
			if err != nil {
				styles.cursor.diagnostic(DiagnosticError, run.ID, "text-shaping", err.Error())
				continue
			}
			for atomIndex := range textAtoms {
				textAtoms[atomIndex].editable = run.Editable
			}
			atoms = append(atoms, textAtoms...)
		case document.WordImageInline:
			if inline.Image == nil {
				styles.cursor.diagnostic(DiagnosticWarning, sourceID, "invalid-image-inline", "An image inline has no image data; a local placeholder is shown.")
				placeholder, err := shapeUnavailableWordImage(ctx, shaper, styles.cursor.glyphPaths, "", sourceID, prepared.style.text)
				if err != nil {
					return preparedParagraph{}, err
				}
				atoms = append(atoms, placeholder...)
				continue
			}
			image := inline.Image
			imageSourceID := image.ID
			if imageSourceID == "" {
				imageSourceID = sourceID
			}
			if image.InlineFallback && image.Floating == nil {
				styles.cursor.diagnostic(DiagnosticWarning, imageSourceID, "floating-image-inline-fallback", "A floating image is shown in the text flow because exact Word text wrapping is unavailable.")
			}
			if !image.Resolved {
				styles.cursor.diagnostic(DiagnosticWarning, imageSourceID, "unresolved-image", "The inline image bytes are unavailable; a local placeholder is shown.")
				placeholder, err := shapeUnavailableWordImage(ctx, shaper, styles.cursor.glyphPaths, imageSourceID, sourceID, prepared.style.text)
				if err != nil {
					return preparedParagraph{}, err
				}
				atoms = append(atoms, placeholder...)
				continue
			}
			width := emuToPoints(image.WidthEMU)
			height := emuToPoints(image.HeightEMU)
			if width <= 0 || height <= 0 || !finite(width) || !finite(height) {
				styles.cursor.diagnostic(DiagnosticWarning, imageSourceID, "invalid-image-size", "The inline image has missing or invalid dimensions; a local placeholder is shown.")
				placeholder, err := shapeUnavailableWordImage(ctx, shaper, styles.cursor.glyphPaths, imageSourceID, sourceID, prepared.style.text)
				if err != nil {
					return preparedParagraph{}, err
				}
				atoms = append(atoms, placeholder...)
				continue
			}
			if image.Floating != nil {
				switch {
				case inCell:
					styles.cursor.diagnostic(DiagnosticWarning, imageSourceID, "floating-image-table-fallback", "A floating image in a table cell is shown inline because table text flow has no polygon exclusion layout.")
				case !validWordImageFloating(image):
					styles.cursor.diagnostic(DiagnosticWarning, imageSourceID, "floating-image-geometry-fallback", "Floating-image geometry is outside the supported tight-wrap subset; the image is shown inline.")
				case len(atoms) > 0:
					styles.cursor.diagnostic(DiagnosticWarning, imageSourceID, "floating-image-inline-order-fallback", "A floating image after other paragraph content is shown inline to preserve source order.")
				default:
					prepared.floatingImages = append(prepared.floatingImages, preparedFloatingImage{
						id: image.ID, width: width, height: height, rotation: image.RotationDegrees,
						geometry: *image.Floating,
					})
					continue
				}
			}
			if width > imageWidth && imageWidth > 0 {
				ratio := imageWidth / width
				width *= ratio
				height *= ratio
				styles.cursor.diagnostic(DiagnosticWarning, imageSourceID, "image-scaled-to-page", "The inline image was scaled down to fit the printable text width.")
			}
			maxHeight := styles.cursor.content.HeightPT
			if maxHeight > 0 {
				reserved := prepared.style.paragraph.BeforePT + prepared.style.paragraph.AfterPT + prepared.uiBeforePT + prepared.uiAfterPT + 4
				if available := maxHeight - reserved; available > 20 {
					maxHeight = available
				}
			}
			if height > maxHeight && maxHeight > 0 {
				ratio := maxHeight / height
				width *= ratio
				height *= ratio
				styles.cursor.diagnostic(DiagnosticWarning, imageSourceID, "image-scaled-to-page", "The inline image was scaled down to fit the printable page height.")
			}
			atoms = append(atoms, paragraphAtom{kind: atomImage, sourceID: imageSourceID, imageID: image.ID, imageRotation: image.RotationDegrees, imageWidth: width, imageHeight: height, advance: width, ascent: height, breakBefore: true, breakAfter: true})
		case document.WordPageBreakInline:
			if inline.PageBreak == nil {
				styles.cursor.diagnostic(DiagnosticError, sourceID, "invalid-page-break", "A page break inline has no source identity.")
				continue
			}
			atoms = append(atoms, paragraphAtom{kind: atomPageBreak, sourceID: inline.PageBreak.ID})
		case document.WordTabInline:
			if inline.Tab == nil || inline.Tab.ID == "" {
				styles.cursor.diagnostic(DiagnosticError, sourceID, "invalid-tab-inline", "A tab inline has no source identity.")
				continue
			}
			atoms = append(atoms, paragraphAtom{kind: atomTab, sourceID: inline.Tab.ID, breakBefore: true, breakAfter: true})
			for _, stop := range prepared.style.paragraph.TabStops {
				if stop.Leader != "" && stop.Leader != "none" {
					styles.cursor.diagnostic(DiagnosticWarning, inline.Tab.ID, "tab-leader-omitted", "The tab leader is omitted from page layout; text and tab alignment are preserved.")
					break
				}
			}
		case document.WordFieldCodeInline:
			// Field delimiters and instructions do not print; the cached field
			// result is projected as adjacent read-only text.
			fieldCodePresent = true
			continue
		case document.WordUnsupportedInline:
			source := sourceID
			detail := "This paragraph contains unsupported inline content."
			if inline.Unsupported != nil {
				if inline.Unsupported.ID != "" {
					source = inline.Unsupported.ID
				}
				if inline.Unsupported.Description != "" {
					detail = inline.Unsupported.Description
				}
				if preview := inline.Unsupported.ImagePreview; preview != nil {
					width := emuToPoints(preview.WidthEMU)
					height := emuToPoints(preview.HeightEMU)
					if width > 0 && height > 0 && finite(width) && finite(height) {
						styles.cursor.diagnostic(DiagnosticWarning, source, "missing-image-media", detail+"; a placeholder is shown.")
						if width > imageWidth && imageWidth > 0 {
							ratio := imageWidth / width
							width *= ratio
							height *= ratio
						}
						maxHeight := styles.cursor.content.HeightPT
						if maxHeight > 0 {
							reserved := prepared.style.paragraph.BeforePT + prepared.style.paragraph.AfterPT + prepared.uiBeforePT + prepared.uiAfterPT + 4
							if available := maxHeight - reserved; available > 20 {
								maxHeight = available
							}
						}
						if height > maxHeight && maxHeight > 0 {
							ratio := maxHeight / height
							width *= ratio
							height *= ratio
						}
						atoms = append(atoms, paragraphAtom{kind: atomImage, sourceID: source, imageID: source, imageWidth: width, imageHeight: height, advance: width, ascent: height, breakBefore: true, breakAfter: true})
					} else {
						styles.cursor.diagnostic(DiagnosticWarning, source, "missing-image-media", detail+"; it has no usable extent and is omitted from the preview.")
					}
					continue
				}
			}
			styles.cursor.diagnostic(DiagnosticError, source, "unsupported-inline", detail)
		default:
			styles.cursor.diagnostic(DiagnosticError, sourceID, "unknown-inline-kind", fmt.Sprintf("Unsupported Word inline kind %d.", inline.Kind))
		}
	}
	if fieldCodePresent {
		styles.cursor.diagnostic(DiagnosticWarning, sourceID, "cached-field-result", "Field instructions are retained but not recalculated; the stored field result is shown.")
	}
	if len(atoms) == 0 {
		// Empty paragraph marks obey the same line-spacing rule as text.
		prepared.lines = []paragraphLine{paragraphMarkLine(prepared.style.text)}
		prepared.lines[0].height = lineHeight(prepared.lines[0], prepared.style.paragraph, prepared.gridLinePitchPT)
		prepared.atoms = atoms
		return prepared, nil
	}
	maxWidth := availableWidth - leftIndent - rightIndent
	if maxWidth <= 0 || maxWidth-firstOffset <= 0 {
		styles.cursor.diagnostic(DiagnosticError, sourceID, "invalid-paragraph-width", "Paragraph indents leave no printable line width.")
		maxWidth = 1
	}
	prepared.atoms = atoms
	prepared.maxWidth = maxWidth
	for index := range prepared.atoms {
		prepared.atoms[index].sourceIndex = index
	}
	prepared.lines = wrapParagraphAtoms(atoms, maxWidth, prepared.style.paragraph, styles.defaultTabStopPT, prepared.gridLinePitchPT)
	if len(prepared.lines) == 0 {
		prepared.lines = []paragraphLine{paragraphMarkLine(prepared.style.text)}
		prepared.lines[0].height = lineHeight(prepared.lines[0], prepared.style.paragraph, prepared.gridLinePitchPT)
		for _, atom := range atoms {
			if atom.kind == atomPageBreak {
				prepared.lines[0].pageBreakAfter = true
				break
			}
		}
	}
	return prepared, nil
}

func shapeTextAtoms(ctx context.Context, shaper TextShaper, pathCache map[glyphPathCacheKey][]GlyphPathCommand, runID, text string, runeOffset int, style document.WordTextStyle, synthetic bool, sourceID string) ([]paragraphAtom, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runes := []rune(text)
	// Shape visible segments independently. Line delimiters have no font glyph
	// and must force a line even when the remaining printable width is ample.
	for _, r := range runes {
		if r != '\n' && r != '\r' {
			continue
		}
		var atoms []paragraphAtom
		start := 0
		for i := 0; i < len(runes); i++ {
			if runes[i] != '\n' && runes[i] != '\r' {
				continue
			}
			segment, err := shapeTextAtoms(ctx, shaper, pathCache, runID, string(runes[start:i]), runeOffset+start, style, synthetic, sourceID)
			if err != nil {
				return nil, err
			}
			atoms = append(atoms, segment...)
			atoms = append(atoms, paragraphAtom{kind: atomLineBreak})
			if runes[i] == '\r' && i+1 < len(runes) && runes[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
		segment, err := shapeTextAtoms(ctx, shaper, pathCache, runID, string(runes[start:]), runeOffset+start, style, synthetic, sourceID)
		if err != nil {
			return nil, err
		}
		return append(atoms, segment...), nil
	}
	for _, r := range runes {
		if isRTL(r) {
			return nil, fmt.Errorf("right-to-left text shaping is not supported in this Word layout slice")
		}
	}
	shaped, err := shaper.Shape(text, style)
	if err != nil {
		return nil, err
	}
	if err := ValidateShapedText(text, shaped); err != nil {
		return nil, err
	}
	if len(runes) == 0 {
		mark := paragraphMarkLine(style)
		return []paragraphAtom{{kind: atomText, sourceID: sourceID, runID: runID, style: style, runeStart: runeOffset, runeEnd: runeOffset, ascent: mark.ascent, descent: mark.descent, gap: mark.gap}}, nil
	}

	type cluster struct {
		start  int
		end    int
		glyphs []ShapedGlyph
	}
	clustersByRange := make(map[[2]int]*cluster)
	clusters := make([]*cluster, 0, len(shaped.Glyphs))
	for _, glyph := range shaped.Glyphs {
		key := [2]int{glyph.RuneStart, glyph.RuneEnd}
		current := clustersByRange[key]
		if current == nil {
			current = &cluster{start: glyph.RuneStart, end: glyph.RuneEnd}
			clustersByRange[key] = current
			clusters = append(clusters, current)
		}
		current.glyphs = append(current.glyphs, glyph)
	}
	sort.SliceStable(clusters, func(i, j int) bool {
		if clusters[i].start == clusters[j].start {
			return clusters[i].end < clusters[j].end
		}
		return clusters[i].start < clusters[j].start
	})
	atoms := make([]paragraphAtom, 0, len(clusters))
	for _, current := range clusters {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		advance := 0.0
		ascent, descent := shaped.AscentPT, shaped.DescentPT
		outlines := make([][]GlyphPathCommand, len(current.glyphs))
		for _, glyph := range current.glyphs {
			advance += glyph.AdvanceXPT
		}
		clusterText := string(runes[current.start:current.end])
		blankCluster := true
		for _, r := range clusterText {
			if !unicode.IsSpace(r) && !unicode.Is(unicode.Cf, r) {
				blankCluster = false
				break
			}
		}
		if !blankCluster {
			outliner, ok := shaper.(GlyphOutlineSource)
			if !ok {
				return nil, fmt.Errorf("text shaper cannot resolve vector outlines for visible glyphs")
			}
			for index, glyph := range current.glyphs {
				slant := 0.0
				if style.Italic {
					nativeItalic := false
					if aspectSource, ok := shaper.(FontAspectSource); ok {
						nativeItalic = aspectSource.IsItalicFont(glyph.FontKey)
					} else {
						lower := strings.ToLower(glyph.FontKey)
						nativeItalic = strings.Contains(lower, "italic") || strings.Contains(lower, "oblique")
					}
					if !nativeItalic {
						slant = 0.21255656 // tan(12 deg) oblique shear
					}
				}
				key := glyphPathCacheKey{
					fontKey:  glyph.FontKey,
					glyphID:  glyph.GlyphID,
					fontSize: math.Float64bits(style.FontSize),
					slant:    math.Float64bits(slant),
				}
				path, cached := pathCache[key]
				if !cached {
					outline, err := outliner.GlyphOutline(glyph.FontKey, glyph.GlyphID)
					if err != nil {
						return nil, err
					}
					path = scaleGlyphOutlineWithSlant(outline, style.FontSize, slant)
					pathCache[key] = path
				}
				outlines[index] = path
			}
		}
		first, last := runes[current.start], runes[current.end-1]
		atoms = append(atoms, paragraphAtom{
			kind: atomText, sourceID: sourceID, runID: runID, text: clusterText,
			runeStart: runeOffset + current.start, runeEnd: runeOffset + current.end,
			glyphRuneOffset: runeOffset,
			style:           style, glyphs: append([]ShapedGlyph(nil), current.glyphs...), outlines: outlines, advance: advance,
			ascent: ascent, descent: descent, gap: shaped.LineGapPT, synthetic: synthetic,
			breakBefore: isCJK(first), breakAfter: unicode.IsSpace(last),
		})
	}
	return atoms, nil
}

func emuToPoints(value int64) float64 { return float64(value) / emuPerPoint }

func isCJK(r rune) bool {
	return (r >= 0x2e80 && r <= 0x9fff) || (r >= 0xf900 && r <= 0xfaff) || (r >= 0x20000 && r <= 0x3134f)
}

func isRTL(r rune) bool {
	return (r >= 0x0590 && r <= 0x08ff) || (r >= 0xfb1d && r <= 0xfdff) || (r >= 0xfe70 && r <= 0xfeff)
}

// Paragraph marks have no visible glyph. Preserve their nominal font size
// instead of letting a substituted space glyph introduce extra font leading.
func paragraphMarkLine(style document.WordTextStyle) paragraphLine {
	size := style.FontSize
	if size <= 0 {
		size = DefaultFontSizePT
	}
	return paragraphLine{ascent: size * .8, descent: size * .2}
}
