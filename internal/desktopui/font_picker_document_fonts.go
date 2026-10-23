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
		addStyle := func(style document.WordTextStyle) {
			for _, name := range []string{style.FontFamily, style.FontFamilyASCII, style.FontFamilyHighANSI, style.FontFamilyEastAsia, style.FontFamilyComplexScript} {
				add(name)
			}
		}
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
		unlock := lockSessionAdapterRead(state)
		data := state.word.WordLayoutData()
		unlock()
		addStyle(data.Defaults.Text)
		for _, style := range data.Styles {
			addStyle(style.Text)
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
			unlock := lockSessionAdapterRead(state)
			values, err := reader.FontFamilies()
			unlock()
			if err != nil {
				return nil, err
			}
			for _, n := range values {
				add(n)
			}
		}
	}
	if state.pptxDoc != nil {
		snapshot := state.pptxDoc.Snapshot()
		addPresentationFonts := func(elements []document.PresentationElement) {
			for _, element := range elements {
				for _, paragraph := range element.Paragraphs {
					for _, run := range paragraph.Runs {
						add(run.Font)
					}
				}
			}
		}
		for _, slide := range snapshot.Slides {
			addPresentationFonts(slide.Elements)
			addPresentationFonts(slide.InheritedElements)
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
		return ""
	}
	if run, ok := selectedWordFormattingRun(state, state.selectedWordRunID); ok && run.Style.FontFamily != "" {
		return wordRunDisplayFamily(run)
	}
	return ""
}

func (a *App) localizedSelectedWordFontFamilyName(state *documentSessionState) string {
	name := selectedWordFontFamilyName(state)
	if name == "" {
		return a.textID("font.picker.title")
	}
	return name
}
