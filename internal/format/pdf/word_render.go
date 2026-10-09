package pdf

import (
	"errors"
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/SalvioniDigitalSolutions/gopdf"

	"glance/internal/wordlayout"
)

func blockingLayoutError(result wordlayout.Result) error {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == wordlayout.DiagnosticError {
			return fmt.Errorf("Word layout blocks PDF export: %s", diagnostic.Detail)
		}
	}
	return errors.New("Word layout is not safe to export")
}

func wordResultHasText(result wordlayout.Result) bool {
	for _, page := range result.Pages {
		for _, fragment := range page.Text {
			if fragment.Text != "" {
				return true
			}
		}
	}
	return false
}

func wordResultHasImages(result wordlayout.Result) bool {
	for _, page := range result.Pages {
		if len(page.Images) > 0 {
			return true
		}
	}
	return false
}

func validatePage(scene wordlayout.PageScene) error {
	if !finite(scene.Clip.WidthPT) || !finite(scene.Clip.HeightPT) || scene.Clip.WidthPT <= 0 || scene.Clip.HeightPT <= 0 {
		return errors.New("invalid page dimensions")
	}
	if !finite(scene.Clip.XPT) || !finite(scene.Clip.YPT) || scene.Clip.XPT != 0 || scene.Clip.YPT != 0 {
		return errors.New("page clip must start at the page origin")
	}
	return nil
}

func validateRect(rect wordlayout.Rect) error {
	if !finite(rect.XPT) || !finite(rect.YPT) || !finite(rect.WidthPT) || !finite(rect.HeightPT) || rect.WidthPT < 0 || rect.HeightPT < 0 {
		return errors.New("invalid point geometry")
	}
	return nil
}

func validateParagraphBorder(border wordlayout.ParagraphBorderPlacement) error {
	if err := validateRect(border.Bounds); err != nil {
		return err
	}
	if border.Style != "single" || border.Bounds.WidthPT <= 0 || border.Bounds.HeightPT <= 0 {
		return errors.New("unsupported or empty paragraph border")
	}
	return nil
}

func validateFragment(fragment wordlayout.TextFragment) error {
	if !finite(fragment.XPT) || !finite(fragment.BaselineYPT) || !finite(fragment.Style.FontSize) || fragment.Style.FontSize < 0 {
		return errors.New("invalid text geometry")
	}
	if fragment.Text != "" && len(fragment.Glyphs) == 0 {
		return errors.New("text has no positioned glyphs")
	}
	if fragment.Text != "" && (fragment.RuneStart < 0 || fragment.RuneEnd < fragment.RuneStart || fragment.RuneEnd-fragment.RuneStart != utf8.RuneCountInString(fragment.Text)) {
		return errors.New("text rune range does not match fragment text")
	}
	for _, glyph := range fragment.Glyphs {
		if glyph.FontKey == "" || !finite(glyph.XPT) || !finite(glyph.YPT) || !finite(glyph.AdvanceXPT) || glyph.AdvanceXPT < 0 {
			return errors.New("invalid positioned glyph")
		}
		hasPath := false
		for _, command := range glyph.Outline {
			if command.Op < wordlayout.GlyphPathMoveTo || command.Op > wordlayout.GlyphPathCurveTo {
				return errors.New("unknown glyph path command")
			}
			if command.Op == wordlayout.GlyphPathMoveTo {
				hasPath = true
			} else if !hasPath {
				return errors.New("glyph path segment has no initial move")
			}
			count := 1
			if command.Op == wordlayout.GlyphPathCurveTo {
				count = 3
			}
			for index := 0; index < count; index++ {
				if !finite(command.Points[index].XPT) || !finite(command.Points[index].YPT) {
					return errors.New("invalid glyph outline point")
				}
			}
		}
	}
	return nil
}

func drawTextFragment(page *gopdf.Page, fragment wordlayout.TextFragment) {
	pen := 0.0
	for _, glyph := range fragment.Glyphs {
		pen += glyph.AdvanceXPT
	}
	if highlight, ok := highlightColor(fragment.Style.Highlight); ok && pen > 0 {
		fontSize := effectiveFontSize(fragment.Style.FontSize)
		page.SetFillColor(highlight)
		page.Rect(fragment.XPT, fragment.BaselineYPT-fontSize*0.88, pen, fontSize*1.18, gopdf.Fill)
	}
	for _, glyph := range fragment.Glyphs {
		if len(glyph.Outline) == 0 {
			continue
		}
		page.SetFillColor(textColor(fragment.Style.FontColor))
		var hasPath bool
		for _, command := range glyph.Outline {
			switch command.Op {
			case wordlayout.GlyphPathMoveTo:
				if hasPath {
					page.ClosePath()
				}
				point := command.Points[0]
				page.MoveTo(glyph.XPT+point.XPT, glyph.YPT+point.YPT)
				hasPath = true
			case wordlayout.GlyphPathLineTo:
				point := command.Points[0]
				page.LineTo(glyph.XPT+point.XPT, glyph.YPT+point.YPT)
			case wordlayout.GlyphPathCurveTo:
				points := command.Points
				page.CurveTo(
					glyph.XPT+points[0].XPT, glyph.YPT+points[0].YPT,
					glyph.XPT+points[1].XPT, glyph.YPT+points[1].YPT,
					glyph.XPT+points[2].XPT, glyph.YPT+points[2].YPT,
				)
			}
		}
		if hasPath {
			page.ClosePath()
			page.DrawPath(gopdf.Fill)
		}
	}
	if fragment.Style.Underline && pen > 0 {
		fontSize := effectiveFontSize(fragment.Style.FontSize)
		page.SetFillColor(textColor(fragment.Style.FontColor))
		page.Rect(fragment.XPT, fragment.BaselineYPT+math.Max(0.5, fontSize*0.08), pen, 0.6, gopdf.Fill)
	}
	if fragment.Style.Strike && pen > 0 {
		fontSize := effectiveFontSize(fragment.Style.FontSize)
		page.SetFillColor(textColor(fragment.Style.FontColor))
		page.Rect(fragment.XPT, fragment.BaselineYPT-fontSize*0.32, pen, math.Max(0.6, fontSize*0.04), gopdf.Fill)
	}
}

func drawCellBorder(page *gopdf.Page, cell wordlayout.TableCellPlacement) {
	const thickness = 0.5
	ink := gopdf.RGB(136, 143, 151)
	if cell.FillColor != "" {
		page.SetFillColor(textColor(cell.FillColor))
		page.Rect(cell.Bounds.XPT, cell.Bounds.YPT, cell.Bounds.WidthPT, cell.Bounds.HeightPT, gopdf.Fill)
	}
	page.SetFillColor(ink)
	if cell.DrawTopBorder {
		page.Rect(cell.Bounds.XPT, cell.Bounds.YPT, cell.Bounds.WidthPT, thickness, gopdf.Fill)
	}
	if cell.DrawBottomBorder {
		page.Rect(cell.Bounds.XPT, cell.Bounds.YPT+cell.Bounds.HeightPT-thickness, cell.Bounds.WidthPT, thickness, gopdf.Fill)
	}
	if cell.DrawLeftBorder {
		page.Rect(cell.Bounds.XPT, cell.Bounds.YPT, thickness, cell.Bounds.HeightPT, gopdf.Fill)
	}
	if cell.DrawRightBorder {
		page.Rect(cell.Bounds.XPT+cell.Bounds.WidthPT-thickness, cell.Bounds.YPT, thickness, cell.Bounds.HeightPT, gopdf.Fill)
	}
}

func fragmentTextChunks(fragment wordlayout.TextFragment) ([]textChunk, error) {
	if fragment.Text == "" {
		return nil, nil
	}
	runes := []rune(fragment.Text)
	type glyphCluster struct {
		start int
		end   int
		key   string
	}
	byRange := make(map[[2]int]string)
	for _, glyph := range fragment.Glyphs {
		start := glyph.RuneStart - fragment.RuneStart
		end := glyph.RuneEnd - fragment.RuneStart
		if start < 0 || end <= start || end > len(runes) {
			return nil, fmt.Errorf("glyph rune range [%d,%d) does not match fragment [%d,%d)", glyph.RuneStart, glyph.RuneEnd, fragment.RuneStart, fragment.RuneEnd)
		}
		rangeKey := [2]int{start, end}
		if existing := byRange[rangeKey]; existing != "" && existing != glyph.FontKey {
			return nil, errors.New("one Unicode cluster uses multiple PDF text fonts")
		}
		byRange[rangeKey] = glyph.FontKey
	}
	clusters := make([]glyphCluster, 0, len(byRange))
	for key, fontKey := range byRange {
		clusters = append(clusters, glyphCluster{start: key[0], end: key[1], key: fontKey})
	}
	for i := 1; i < len(clusters); i++ {
		for j := i; j > 0 && clusters[j].start < clusters[j-1].start; j-- {
			clusters[j], clusters[j-1] = clusters[j-1], clusters[j]
		}
	}
	chunks := make([]textChunk, 0, len(clusters))
	covered := 0
	for _, cluster := range clusters {
		if cluster.start != covered {
			return nil, errors.New("glyph clusters do not cover fragment text exactly once")
		}
		text := string(runes[cluster.start:cluster.end])
		if len(chunks) > 0 && chunks[len(chunks)-1].fontKey == cluster.key {
			chunks[len(chunks)-1].text += text
		} else {
			chunks = append(chunks, textChunk{fontKey: cluster.key, text: text})
		}
		covered = cluster.end
	}
	if covered != len(runes) {
		return nil, errors.New("glyph clusters do not cover fragment text")
	}
	return chunks, nil
}
