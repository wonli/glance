package application

import (
	"context"
	"fmt"
	"glance/internal/document"
	"glance/internal/proofreading"
	"glance/internal/wordlayout"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// ReflowProofreading shapes outside Gio and exposes source-aligned line boundaries.
func ReflowProofreading(ctx context.Context, s proofreading.Snapshot, geometry []ProofreadingPageGeometry, style ProofreadingTextStyle, shaper wordlayout.TextShaper, initial bool) (ProofreadingLayoutSnapshot, error) {
	return ReflowProofreadingLines(ctx, s, geometry, style, func(ctx context.Context, value string, width float64, style ProofreadingTextStyle) ([]ProofreadingLine, error) {
		return proofreadingShapeLines(ctx, value, width, style, shaper)
	}, initial)
}

// ProofreadingLineShaper supplies source-aligned line geometry from the active
// text renderer. The application model remains independent of that renderer.
type ProofreadingLineShaper func(context.Context, string, float64, ProofreadingTextStyle) ([]ProofreadingLine, error)

func ReflowProofreadingLines(ctx context.Context, s proofreading.Snapshot, geometry []ProofreadingPageGeometry, style ProofreadingTextStyle, shape ProofreadingLineShaper, initial bool) (ProofreadingLayoutSnapshot, error) {
	result := ProofreadingLayoutSnapshot{Revision: s.Revision, StyleRevision: style.Revision, Pages: map[proofreading.PageID]ProofreadingPageLayout{}, Ends: map[proofreading.PageID]int{}, Initial: initial}
	builders := map[proofreading.PageID]*strings.Builder{}
	for _, p := range s.Pages {
		builders[p.ID] = &strings.Builder{}
	}
	for _, r := range s.Runs {
		builders[r.Page].WriteString(r.Text)
	}
	texts := map[proofreading.PageID]string{}
	for id, b := range builders {
		texts[id] = b.String()
	}
	globalStart := 0
	for i := 0; i < len(s.Pages); {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		first := i
		independent := s.Pages[i].Independent()
		if independent {
			i++
		} else {
			for i < len(s.Pages) && !s.Pages[i].Independent() {
				i++
			}
		}
		var combinedBuilder strings.Builder
		for j := first; j < i; j++ {
			combinedBuilder.WriteString(texts[s.Pages[j].ID])
		}
		combined := combinedBuilder.String()
		combinedRunes := []rune(combined)
		// Locked pages expose subsequent eligible content to line-based adjustment.
		var previewBuilder strings.Builder
		previewBuilder.WriteString(combined)
		if s.Pages[first].Locked || s.Pages[first].Manual {
			for j := i; j < len(s.Pages) && !s.Pages[j].Reviewed && (s.Pages[first].Manual || !s.Pages[j].Independent()); j++ {
				previewBuilder.WriteString(texts[s.Pages[j].ID])
			}
		}
		previewRunes := []rune(previewBuilder.String())
		consumed := 0
		for j := first; j < i; j++ {
			if j >= len(geometry) {
				return result, fmt.Errorf("missing proofreading geometry")
			}
			remaining := combinedRunes[consumed:]
			if independent {
				remaining = previewRunes
			}
			window := min(len(remaining), 4096)
			if independent || j == i-1 {
				window = min(len(remaining), len(combinedRunes)+4096)
			}
			var lines []ProofreadingLine
			var err error
			text := ""
			for {
				text = string(remaining[:window])
				lines, err = shape(ctx, text, geometry[j].Width, style)
				if err != nil || window == len(remaining) || len(lines) > 0 && lines[len(lines)-1].Bottom > geometry[j].Height*2 {
					break
				}
				window = min(len(remaining), max(1, window*2))
			}
			if err != nil {
				return result, err
			}
			fit := 0
			for _, line := range lines {
				if line.Bottom <= geometry[j].Height+0.01 {
					fit = line.End
				} else {
					break
				}
			}
			if fit == 0 && len(lines) > 0 {
				fit = lines[0].End
			}
			if independent || j == i-1 {
				fit = len(combinedRunes) - consumed
			}
			result.Ends[s.Pages[j].ID] = globalStart + consumed + fit
			height := 0.0
			if len(lines) > 0 {
				height = lines[len(lines)-1].Bottom
			}
			result.Pages[s.Pages[j].ID] = ProofreadingPageLayout{Text: text, Lines: lines, Height: height}
			consumed += fit
		}
		globalStart += utf8.RuneCountInString(combined)
	}
	return result, nil
}
func proofreadingShapeLines(ctx context.Context, value string, width float64, style ProofreadingTextStyle, shaper wordlayout.TextShaper) ([]ProofreadingLine, error) {
	pieces := strings.Split(value, "\n")
	blocks := make([]document.WordBlock, 0, len(pieces))
	starts := map[string]int{}
	offset := 0
	for i, text := range pieces {
		id := fmt.Sprintf("proof-%d", i)
		runID := id + "-text"
		starts[runID] = offset
		p := document.WordParagraph{ID: id, Layout: document.WordParagraphLayout{LineSpacingMultiple: 1.28, Set: document.WordParagraphLineSpacingSet}, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: runID, Text: text, Style: document.WordTextStyle{FontSize: style.Size, FontFamily: style.Family}, StyleSet: document.WordTextFontSizeSet | document.WordTextFontFamilySet}}}}
		blocks = append(blocks, document.WordBlock{ID: id, Kind: document.WordParagraphBlock, Paragraph: &p})
		offset += utf8.RuneCountInString(text) + 1
	}
	input := wordlayout.Input{Blocks: blocks, DefaultPageSetup: document.WordPageSetup{WidthPT: math.Max(1, width), HeightPT: 1e9}}
	scene, err := wordlayout.NewEngine(shaper).Layout(ctx, input)
	if err != nil {
		return nil, err
	}
	for _, diagnostic := range scene.Diagnostics {
		if diagnostic.Severity == wordlayout.DiagnosticError && diagnostic.Code != "paragraph-line-overflow" {
			return nil, fmt.Errorf("proofreading layout: %s", diagnostic.Detail)
		}
	}
	type caret struct {
		offset int
		y      float64
	}
	carets := make([]caret, 0, len(scene.Locations))
	for source, pos := range scene.Locations {
		carets = append(carets, caret{starts[source.RunID] + source.RuneOffset, pos.YPT})
	}
	sort.Slice(carets, func(i, j int) bool {
		if carets[i].offset != carets[j].offset {
			return carets[i].offset < carets[j].offset
		}
		return carets[i].y < carets[j].y
	})
	lines := []ProofreadingLine{}
	total := utf8.RuneCountInString(value)
	for _, c := range carets {
		if len(lines) == 0 || math.Abs(lines[len(lines)-1].Top-c.y) > 0.1 {
			if len(lines) > 0 {
				lines[len(lines)-1].End = c.offset
			}
			lines = append(lines, ProofreadingLine{Start: c.offset, End: total, Top: c.y})
		}
	}
	// Normalize baseline positions to top coordinates. Empty paragraphs are real lines.
	if len(lines) > 0 {
		base := lines[0].Top
		for i := range lines {
			lines[i].Top -= base
			if i+1 < len(lines) {
				lines[i].Bottom = lines[i+1].Top - base
			} else {
				lines[i].Bottom = lines[i].Top + style.Size*1.28
			}
		}
	}
	return lines, nil
}
