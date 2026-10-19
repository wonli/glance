package desktopui

import (
	"glance/internal/i18n"
	"image"
	"image/color"
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
	kind           wordNavigationEntryKind
	key            string
	label          string
	labelMessageID string
	labelArgs      i18n.Args
	level          int
	blockIndex     int
	paragraphID    string
	runID          string
	anchorRunID    string
	start          int
	end            int
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
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.opened_the_navigation_pane_6b4392"}, feedbackSuccess, notificationSuppressed)
	} else {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.closed_the_navigation_pane_b17469"}, feedbackSuccess, notificationSuppressed)
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
					return a.layoutWordZoomableBody(gtx, state, blocks, func(gtx layout.Context) layout.Dimensions {
						if state.wordViewMode == wordViewWebLayout {
							return a.layoutWordCanvas(gtx, state, blocks)
						}
						if state.wordLayoutResult == nil && state.wordLayoutError == "" && len(blocks) > 0 {
							return a.layoutWordCanvas(gtx, state, blocks)
						}
						return a.layoutWordPageView(gtx, state)
					})
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
			semantic.DescriptionOp(a.textID("word.accessibility.navigation_pane")).Add(gtx.Ops)
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
						locale := a.localeSnapshot().Resolved
						value := a.textArgs("word.navigation.stats", i18n.Args{"Headings": i18n.FormatNumber(locale, float64(headingCount)), "Results": i18n.FormatNumber(locale, float64(resultCount))})
						bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
						semantic.DescriptionOp(a.textArgs("word.accessibility.navigation_stats", i18n.Args{"Value": value})).Add(gtx.Ops)
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

func (a *App) layoutWordNavigationHeader(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(44))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Inset{Top: unit.Dp(10), Left: unit.Dp(14), Right: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				label := material.Body1(a.theme, a.textID("word.navigation.title"))
				label.Color = wordStatusText
				bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
				semantic.DescriptionOp(a.textID("word.accessibility.navigation_header")).Add(gtx.Ops)
				bounds.Pop()
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return state.wordNavigationCloseButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					size := gtx.Dp(unit.Dp(28))
					gtx.Constraints.Min = image.Pt(size, size)
					gtx.Constraints.Max = gtx.Constraints.Min
					semantic.Button.Add(gtx.Ops)
					semantic.DescriptionOp(a.textID("word.accessibility.close_navigation")).Add(gtx.Ops)
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
				semantic.DescriptionOp(a.textID("word.accessibility.navigation_search")).Add(gtx.Ops)
				bounds.Pop()
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(5), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Editor(a.theme, &state.wordNavigationSearchEditor, a.textID("word.navigation.search.placeholder"))
					style.TextSize = unit.Sp(12)
					style.Color = wordStatusText
					return style.Layout(gtx)
				})
			},
		)
	})
}

func (a *App) layoutWordNavigationEntry(gtx layout.Context, state *documentSessionState, blocks []document.WordBlock, entry wordNavigationEntry) layout.Dimensions {
	labelText := entry.label
	if entry.labelMessageID != "" {
		labelText = a.textArgs(entry.labelMessageID, entry.labelArgs)
	}
	if entry.kind == wordNavigationGroup {
		return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(5), Left: unit.Dp(14), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Caption(a.theme, labelText)
			label.Color = wordStatusText
			bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			semantic.DescriptionOp(a.textArgs("word.accessibility.navigation_group", i18n.Args{"Label": labelText})).Add(gtx.Ops)
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
			label := material.Body1(a.theme, labelText)
			label.TextSize = unit.Sp(12)
			label.Color = wordStatusText
			label.MaxLines = 2
			bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			if entry.kind == wordNavigationHeading {
				semantic.DescriptionOp(a.textArgs("word.accessibility.navigation_heading", i18n.Args{"Label": labelText})).Add(gtx.Ops)
			} else {
				semantic.DescriptionOp(a.textArgs("word.accessibility.navigation_result", i18n.Args{"Label": labelText})).Add(gtx.Ops)
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
