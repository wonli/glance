package desktopui

import (
	"glance/internal/proofreading"
	"strings"
)

// A native editor shapes only a bounded visible portion of a page. The model
// retains the entire page, including text beyond the viewport.
type proofreadingEditorWindow struct {
	Initialized            bool
	Start, End             int
	Top                    float64
	StartAnchor, EndAnchor proofreading.Anchor
}
type proofreadingEditorCaret struct {
	Start, End proofreading.Anchor
	Valid      bool
}

func (a *App) proofreadingVisibleEditorWindow(index int, text string) proofreadingEditorWindow {
	s := a.proofreading
	content := []rune(text)
	w := proofreadingEditorWindow{End: min(len(content), 2048)}
	if !s.modelInitialAssigned {
		return proofreadingEditorWindow{}
	}
	if index < len(s.xEditorWindows) && s.xEditorWindows[index].Initialized {
		w = s.xEditorWindows[index]
		w.Start = max(0, min(len(content), w.Start))
		w.End = max(w.Start, min(len(content), w.End))
	}
	id := proofreading.PageID(index + 1)
	shaped, ok := s.controller.Layout.Pages[id]
	ready := ok && strings.HasPrefix(shaped.Text, text) && s.controller.Layout.StyleRevision == s.xTextSizeStep
	if ready {
		scale := s.xPageRenderScales[index]
		if scale <= 0 {
			scale = 1
		}
		height := s.xPageViewportHeights[index]
		if height <= 0 {
			_, marginY := proofreadingTextMargins(s.pageFrames[index].Width, s.pageFrames[index].Height)
			height = s.pageFrames[index].Height - 2*marginY
		}
		contentHeight := proofreadingPageContentHeight(shaped, text)
		top := s.xPageScrollOffsets[index]
		top = clampProofreadingScrollTop(top, height, contentHeight)
		if s.xRevealCaret[index] && s.xEditorCarets[index].Valid {
			if caret, err := proofreading.OffsetAt(s.controller.Model.PageSnapshot(id), id, s.xEditorCarets[index].Start); err == nil {
				for _, line := range shaped.Lines {
					if caret >= line.Start && caret <= line.End {
						if line.Top < top {
							top = line.Top
						}
						if line.Bottom > top+height {
							top = max(0, line.Bottom-height)
						}
						break
					}
				}
			}
			top = clampProofreadingScrollTop(top, height, contentHeight)
			s.xPageScrollOffsets[index] = top
			s.xRevealCaret[index] = false
		}
		s.xPageScrollOffsets[index] = top
		start, end := 0, len(content)
		first := true
		w.Top = 0
		for _, line := range shaped.Lines {
			if line.Start > len(content) {
				break
			}
			if line.Bottom <= max(0, top-12*proofreadingXTextScale(s.xTextSizeStep)*proofreadingXEditorLineHeightScale) {
				continue
			}
			if first {
				start = line.Start
				w.Top = line.Top
				first = false
			}
			if line.Top > top+height*2 {
				end = line.Start
				break
			}
		}
		w.Start = max(0, min(len(content), start))
		w.End = max(w.Start, min(len(content), end))
	}
	// A pending/failed layout never permits an unbounded native editor.
	if w.End-w.Start > 4096 {
		w.End = w.Start + 4096
	}
	return w
}
