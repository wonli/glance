package desktopui

import (
	"glance/internal/document"
	"glance/internal/wordfont"
	"sort"
)

func documentFontNames(state *documentSessionState) ([]string, error) {
	if state == nil {
		return nil, nil
	}
	names := map[string]bool{}
	add := func(name string) {
		for _, n := range wordfont.SplitFamilies(name) {
			if n != "" {
				names[n] = true
			}
		}
	}
	if state.word != nil {
		paragraph := func(p document.WordParagraph) {
			for _, inline := range p.Inlines {
				if inline.Text == nil {
					continue
				}
				s := inline.Text.Style
				for _, n := range []string{s.FontFamily, s.FontFamilyASCII, s.FontFamilyHighANSI, s.FontFamilyEastAsia, s.FontFamilyComplexScript} {
					add(n)
				}
			}
		}
		for _, b := range wordBlocksForState(state) {
			if b.Paragraph != nil {
				paragraph(*b.Paragraph)
			}
			if b.Table != nil {
				for _, row := range b.Table.Rows {
					for _, cell := range row.Cells {
						for _, p := range cell.Paragraphs {
							paragraph(p)
						}
					}
				}
			}
		}
	}
	if state.book != nil {
		if reader, ok := state.book.(interface{ FontFamilies() ([]string, error) }); ok {
			values, err := reader.FontFamilies()
			if err != nil {
				return nil, err
			}
			for _, n := range values {
				add(n)
			}
		}
	}
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

// Read only the selected run; avoid cloning the document on every Ribbon frame.
func selectedWordFontFamilyName(state *documentSessionState) string {
	if state == nil {
		return "字体"
	}
	state.wordBlocksMu.RLock()
	defer state.wordBlocksMu.RUnlock()
	if run, ok := selectedWordRun(state.wordBlocks, state.selectedWordRunID); ok && run.Style.FontFamily != "" {
		return run.Style.FontFamily
	}
	return "字体"
}
