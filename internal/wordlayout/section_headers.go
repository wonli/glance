package wordlayout

import (
	"context"
	"fmt"
	"math"
	"strings"

	"glance/internal/document"
)

func appendSectionHeaders(ctx context.Context, shaper TextShaper, layout document.WordLayoutData, cursor *layoutCursor, result *Result) error {
	for pageIndex := range result.Pages {
		if err := ctx.Err(); err != nil {
			return err
		}
		page := &result.Pages[pageIndex]
		if page.SectionIndex < 0 || page.SectionIndex >= len(layout.Sections) {
			continue
		}
		section := layout.Sections[page.SectionIndex]
		if section.HeaderText == "" {
			continue
		}
		style := document.WordTextStyle{FontSize: 10}
		lines := strings.Split(section.HeaderText, "\n")
		lineHeight := style.FontSize * 1.25
		baseline := section.HeaderDistancePT + style.FontSize*0.8
		lastHeaderBaseline := page.Content.YPT - 2 - float64(max(0, len(lines)-1))*lineHeight
		if baseline > lastHeaderBaseline {
			baseline = math.Max(style.FontSize, lastHeaderBaseline)
		}
		for lineIndex, line := range lines {
			if line == "" {
				continue
			}
			sourceID := fmt.Sprintf("header-s%d-p%d", page.SectionIndex, lineIndex)
			atoms, err := shapeTextAtoms(ctx, shaper, cursor.glyphPaths, sourceID, line, 0, style, true, sourceID)
			if err != nil {
				cursor.diagnostic(DiagnosticWarning, sourceID, "header-text-unavailable", "The section header could not be shaped for preview: "+err.Error())
				baseline += lineHeight
				continue
			}
			x := page.Content.XPT
			maxX := page.Clip.WidthPT - page.PageSetup.MarginRightPT
			for _, atom := range atoms {
				if atom.kind != atomText {
					continue
				}
				if x > page.Content.XPT && x+atom.advance > maxX {
					baseline += lineHeight
					x = page.Content.XPT
				}
				page.Text = append(page.Text, textFragmentAt(atom, x, baseline))
				x += atom.advance
			}
			baseline += lineHeight
		}
	}
	return nil
}
