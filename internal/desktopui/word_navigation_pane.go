package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

const wordNavigationPaneWidthDP = 300
const wordNavigationResultLimit = 500

var (
	wordNavigationPaneBackground = color.NRGBA{R: 247, G: 248, B: 250, A: 255}
	wordNavigationPaneDivider    = color.NRGBA{R: 216, G: 220, B: 226, A: 255}
	wordNavigationRowSelected    = color.NRGBA{R: 224, G: 235, B: 248, A: 255}
)

type wordNavigationEntryKind uint8

const (
	wordNavigationGroup wordNavigationEntryKind = iota
	wordNavigationHeading
	wordNavigationResult
)

type wordNavigationEntry struct {
	kind        wordNavigationEntryKind
	key         string
	label       string
	level       int
	blockIndex  int
	paragraphID string
	runID       string
	anchorRunID string
	start       int
	end         int
}

type wordNavigationParagraphRef struct {
	paragraph  *document.WordParagraph
	blockIndex int
}

type wordNavigationEntryCache struct {
	valid        bool
	revision     uint64
	draftEpoch   uint64
	query        string
	entries      []wordNavigationEntry
	headingCount int
	resultCount  int
}

func (a *App) toggleWordNavigationPane() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	state.wordNavigationVisible = !state.wordNavigationVisible
	if state.wordNavigationVisible {
		a.status = "已打开导航窗格"
	} else {
		a.status = "已关闭导航窗格"
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) layoutWordBodyArea(gtx layout.Context, state *documentSessionState, blocks []document.WordBlock) layout.Dimensions {
	state.wordNavigationList.List.Axis = layout.Vertical
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !state.wordNavigationVisible || state.wordReadingMode {
				return layout.Dimensions{}
			}
			return a.layoutWordNavigationPane(gtx, state, blocks)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if state.wordRulerHidden || state.wordReadingMode || state.wordViewMode == wordViewWebLayout {
						return layout.Dimensions{}
					}
					return a.layoutWordHorizontalRuler(gtx, state)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if state.wordViewMode == wordViewPrintLayout {
						return a.layoutWordPageView(gtx, state)
					}
					return a.layoutWordCanvas(gtx, state, blocks)
				}),
			)
		}),
	)
}

func (a *App) layoutWordNavigationPane(gtx layout.Context, state *documentSessionState, blocks []document.WordBlock) layout.Dimensions {
	width := gtx.Dp(unit.Dp(wordNavigationPaneWidthDP))
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
	state.wordNavigationSearchEditor.SingleLine = true
	state.wordNavigationSearchEditor.ReadOnly = false
	if state.wordNavigationCloseButton.Clicked(gtx) {
		state.wordNavigationVisible = false
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	query := strings.TrimSpace(state.wordNavigationSearchEditor.Text())
	if query != state.wordNavigationSearchQuery {
		state.wordNavigationSearchQuery = query
		state.wordNavigationButtons = make(map[string]*widget.Clickable)
		state.wordNavigationList.Position.First = 0
		state.wordNavigationList.Position.Offset = 0
	}
	entries, headingCount, resultCount := wordNavigationCachedEntries(state, blocks, query)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, wordNavigationPaneBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, wordNavigationPaneDivider, clip.Rect{Min: image.Pt(gtx.Constraints.Max.X-gtx.Dp(unit.Dp(1)), 0), Max: gtx.Constraints.Max}.Op())
			bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			semantic.DescriptionOp("Word 导航窗格").Add(gtx.Ops)
			bounds.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutWordNavigationHeader(gtx, state)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutWordNavigationSearch(gtx, state)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					height := gtx.Dp(unit.Dp(28))
					gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
					return layout.Inset{Left: unit.Dp(14), Right: unit.Dp(12), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						value := fmt.Sprintf("%d 个标题 · %d 个搜索结果", headingCount, resultCount)
						bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
						semantic.DescriptionOp("Word 导航统计 " + value).Add(gtx.Ops)
						bounds.Pop()
						label := material.Caption(a.theme, value)
						label.Color = wordStatusText
						return label.Layout(gtx)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return state.wordNavigationList.Layout(gtx, len(entries), func(gtx layout.Context, index int) layout.Dimensions {
						return a.layoutWordNavigationEntry(gtx, state, blocks, entries[index])
					})
				}),
			)
		},
	)
}

func wordNavigationCachedEntries(state *documentSessionState, blocks []document.WordBlock, query string) ([]wordNavigationEntry, int, int) {
	if state != nil {
		cache := state.wordNavigationEntryCache
		draftEpoch := state.wordDraftEpoch.Load()
		if cache.valid && cache.revision == state.wordRevisionAtomic.Load() && cache.draftEpoch == draftEpoch && cache.query == query {
			return cache.entries, cache.headingCount, cache.resultCount
		}
	}
	drafts := wordNavigationDrafts(state)
	styles := []document.WordStyle(nil)
	if state != nil && state.word != nil {
		if data, ok := wordLayoutDataForState(state); ok {
			styles = data.Styles
		}
	}
	entries, headingCount, resultCount := wordNavigationEntries(blocks, styles, query, drafts)
	if state != nil {
		state.wordNavigationEntryCache = wordNavigationEntryCache{
			valid: true, revision: state.wordRevisionAtomic.Load(), draftEpoch: state.wordDraftEpoch.Load(), query: query,
			entries: entries, headingCount: headingCount, resultCount: resultCount,
		}
	}
	return entries, headingCount, resultCount
}

func (a *App) layoutWordNavigationHeader(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(44))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Inset{Top: unit.Dp(10), Left: unit.Dp(14), Right: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				label := material.Body1(a.theme, "导航")
				label.Color = wordStatusText
				bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
				semantic.DescriptionOp("Word 导航标题栏").Add(gtx.Ops)
				bounds.Pop()
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return state.wordNavigationCloseButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					size := gtx.Dp(unit.Dp(28))
					gtx.Constraints.Min = image.Pt(size, size)
					gtx.Constraints.Max = gtx.Constraints.Min
					semantic.Button.Add(gtx.Ops)
					semantic.DescriptionOp("关闭导航窗格").Add(gtx.Ops)
					if state.wordNavigationCloseButton.Hovered() {
						paint.FillShape(gtx.Ops, wordNavigationRowSelected, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
					}
					label := material.Body1(a.theme, "×")
					label.Color = wordStatusText
					return layout.Center.Layout(gtx, label.Layout)
				})
			}),
		)
	})
}

func (a *App) layoutWordNavigationSearch(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(44))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
				bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
				semantic.Editor.Add(gtx.Ops)
				semantic.DescriptionOp("Word 导航搜索").Add(gtx.Ops)
				bounds.Pop()
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(5), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Editor(a.theme, &state.wordNavigationSearchEditor, "搜索文档")
					style.TextSize = unit.Sp(12)
					style.Color = wordStatusText
					return style.Layout(gtx)
				})
			},
		)
	})
}

func (a *App) layoutWordNavigationEntry(gtx layout.Context, state *documentSessionState, blocks []document.WordBlock, entry wordNavigationEntry) layout.Dimensions {
	if entry.kind == wordNavigationGroup {
		return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(5), Left: unit.Dp(14), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Caption(a.theme, entry.label)
			label.Color = wordStatusText
			bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			semantic.DescriptionOp("Word 导航分组 " + entry.label).Add(gtx.Ops)
			bounds.Pop()
			return label.Layout(gtx)
		})
	}
	button := wordNavigationButton(state, entry.key)
	if button.Clicked(gtx) {
		a.navigateToWordNavigationEntry(state, blocks, entry)
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if button.Hovered() {
			paint.FillShape(gtx.Ops, wordNavigationRowSelected, clip.Rect{Max: gtx.Constraints.Max}.Op())
		}
		left := 14 + max(0, entry.level-1)*14
		return layout.Inset{Left: unit.Dp(left), Right: unit.Dp(10), Top: unit.Dp(5), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Body1(a.theme, entry.label)
			label.TextSize = unit.Sp(12)
			label.Color = wordStatusText
			label.MaxLines = 2
			bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			if entry.kind == wordNavigationHeading {
				semantic.DescriptionOp("Word 导航标题 " + entry.label).Add(gtx.Ops)
			} else {
				semantic.DescriptionOp("Word 导航结果 " + entry.label).Add(gtx.Ops)
			}
			semantic.Button.Add(gtx.Ops)
			bounds.Pop()
			return label.Layout(gtx)
		})
	})
}

func wordNavigationButton(state *documentSessionState, key string) *widget.Clickable {
	if state.wordNavigationButtons == nil {
		state.wordNavigationButtons = make(map[string]*widget.Clickable)
	}
	button := state.wordNavigationButtons[key]
	if button == nil {
		button = new(widget.Clickable)
		state.wordNavigationButtons[key] = button
	}
	return button
}

func wordNavigationDrafts(state *documentSessionState) map[string]string {
	if state == nil {
		return nil
	}
	return state.wordDraftSnapshot()
}

func wordNavigationEntries(blocks []document.WordBlock, styles []document.WordStyle, query string, drafts map[string]string) ([]wordNavigationEntry, int, int) {
	paragraphs := wordNavigationParagraphRefs(blocks)
	styleParents := wordNavigationStyleParents(styles)
	entries := []wordNavigationEntry{{kind: wordNavigationGroup, key: "group:headings", label: "文档标题"}}
	headingCount := 0
	for _, ref := range paragraphs {
		paragraph := ref.paragraph
		level := wordNavigationHeadingLevelFromParents(paragraph.Style, styleParents)
		if level == 0 {
			continue
		}
		label := strings.TrimSpace(wordNavigationParagraphText(*paragraph, drafts))
		if label == "" {
			continue
		}
		runID := wordNavigationFirstEditableRun(*paragraph)
		entries = append(entries, wordNavigationEntry{
			kind: wordNavigationHeading, key: "heading:" + paragraph.ID, label: wordNavigationShortLabel(label), level: level,
			blockIndex: ref.blockIndex, paragraphID: paragraph.ID, runID: runID, anchorRunID: wordNavigationFirstTextRun(*paragraph),
		})
		headingCount++
	}
	if headingCount == 0 {
		entries = append(entries, wordNavigationEntry{kind: wordNavigationGroup, key: "empty:headings", label: "未发现标题"})
	}
	resultCount := 0
	if query != "" {
		resultGroupIndex := len(entries)
		entries = append(entries, wordNavigationEntry{kind: wordNavigationGroup, key: "group:results", label: "搜索结果"})
		needle := lowerWordRunes([]rune(query))
		for _, ref := range paragraphs {
			paragraph := ref.paragraph
			for _, inline := range paragraph.Inlines {
				if inline.Text == nil || !inline.Text.Editable {
					continue
				}
				run := inline.Text
				value := run.Text
				if draft, ok := drafts[run.ID]; ok {
					value = draft
				}
				runes := []rune(value)
				lowered := lowerWordRunes(runes)
				for start := 0; start+len(needle) <= len(runes); {
					found := findWordRunes(lowered, needle, start)
					if found < 0 {
						break
					}
					end := found + len(needle)
					resultCount++
					if resultCount <= wordNavigationResultLimit {
						entries = append(entries, wordNavigationEntry{
							kind: wordNavigationResult, key: fmt.Sprintf("result:%s:%s:%d", paragraph.ID, run.ID, found),
							label: wordNavigationMatchSnippet(runes, found, end), blockIndex: ref.blockIndex,
							paragraphID: paragraph.ID, runID: run.ID, start: found, end: end,
						})
					}
					start = end
				}
			}
		}
		if resultCount == 0 {
			entries = append(entries, wordNavigationEntry{kind: wordNavigationGroup, key: "empty:results", label: "没有搜索结果"})
		} else if resultCount > wordNavigationResultLimit {
			entries[resultGroupIndex].label = fmt.Sprintf("搜索结果（显示前 %d 个，共 %d 个）", wordNavigationResultLimit, resultCount)
		}
	}
	return entries, headingCount, resultCount
}

func wordNavigationParagraphRefs(blocks []document.WordBlock) []wordNavigationParagraphRef {
	var paragraphs []wordNavigationParagraphRef
	for blockIndex := range blocks {
		block := &blocks[blockIndex]
		if block.Paragraph != nil {
			paragraphs = append(paragraphs, wordNavigationParagraphRef{paragraph: block.Paragraph, blockIndex: blockIndex})
		}
		if block.Table == nil {
			continue
		}
		for rowIndex := range block.Table.Rows {
			for cellIndex := range block.Table.Rows[rowIndex].Cells {
				cell := &block.Table.Rows[rowIndex].Cells[cellIndex]
				for paragraphIndex := range cell.Paragraphs {
					paragraphs = append(paragraphs, wordNavigationParagraphRef{paragraph: &cell.Paragraphs[paragraphIndex], blockIndex: blockIndex})
				}
			}
		}
	}
	return paragraphs
}

func wordNavigationHeadingLevel(styleID string, styles []document.WordStyle) int {
	return wordNavigationHeadingLevelFromParents(styleID, wordNavigationStyleParents(styles))
}

func wordNavigationStyleParents(styles []document.WordStyle) map[string]string {
	styleByID := make(map[string]string, len(styles))
	for _, style := range styles {
		styleByID[wordNavigationNormalizeStyleID(style.ID)] = style.BasedOn
	}
	return styleByID
}

func wordNavigationHeadingLevelFromParents(styleID string, styleByID map[string]string) int {
	seen := make(map[string]struct{}, len(styleByID)+1)
	for styleID != "" {
		normalized := wordNavigationNormalizeStyleID(styleID)
		if level := wordNavigationBuiltinHeadingLevel(normalized); level > 0 {
			return level
		}
		if _, duplicate := seen[normalized]; duplicate {
			return 0
		}
		seen[normalized] = struct{}{}
		basedOn, ok := styleByID[normalized]
		if !ok {
			return 0
		}
		styleID = basedOn
	}
	return 0
}

func wordNavigationNormalizeStyleID(styleID string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(styleID)))
}

func wordNavigationBuiltinHeadingLevel(styleID string) int {
	if !strings.HasPrefix(styleID, "heading") {
		return 0
	}
	level, err := strconv.Atoi(strings.TrimPrefix(styleID, "heading"))
	if err != nil || level < 1 || level > 9 {
		return 0
	}
	return level
}

func wordNavigationParagraphText(paragraph document.WordParagraph, drafts map[string]string) string {
	var text strings.Builder
	for _, inline := range paragraph.Inlines {
		if inline.Text == nil {
			continue
		}
		value := inline.Text.Text
		if draft, ok := drafts[inline.Text.ID]; ok {
			value = draft
		}
		text.WriteString(value)
	}
	return text.String()
}

func wordNavigationFirstEditableRun(paragraph document.WordParagraph) string {
	for _, inline := range paragraph.Inlines {
		if inline.Text != nil && inline.Text.Editable {
			return inline.Text.ID
		}
	}
	return ""
}

func wordNavigationFirstTextRun(paragraph document.WordParagraph) string {
	for _, inline := range paragraph.Inlines {
		if inline.Text != nil {
			return inline.Text.ID
		}
	}
	return ""
}

func wordNavigationShortLabel(value string) string {
	runes := []rune(value)
	if len(runes) <= 64 {
		return value
	}
	return string(runes[:61]) + "…"
}

func wordNavigationMatchSnippet(runes []rune, start, end int) string {
	left, right := max(0, start-26), min(len(runes), end+26)
	value := string(runes[left:right])
	if left > 0 {
		value = "…" + value
	}
	if right < len(runes) {
		value += "…"
	}
	return value
}

func (a *App) navigateToWordNavigationEntry(state *documentSessionState, blocks []document.WordBlock, entry wordNavigationEntry) {
	if entry.runID != "" {
		a.focusWordTextMatch(state, blocks, wordTextMatch{ParagraphID: entry.paragraphID, RunID: entry.runID, Start: entry.start, End: entry.end})
		a.navigateWordPageToRun(state, entry.runID, entry.start)
		return
	}
	state.selectedWordRunID = ""
	state.selectedWordParagraphID = entry.paragraphID
	state.selectedWordImageID = ""
	state.wordListState.Position.First = entry.blockIndex
	state.wordListState.Position.Offset = 0
	state.wordListState.Position.OffsetLast = 0
	state.wordListState.Position.BeforeEnd = false
	if entry.anchorRunID != "" {
		a.navigateWordPageToRun(state, entry.anchorRunID, 0)
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
