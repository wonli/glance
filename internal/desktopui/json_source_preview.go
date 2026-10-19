package desktopui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const jsonSourceEditLimit = 512 * 1024
const jsonValueEditLimit = 64 * 1024

type jsonSourceSegment struct{ start, end, line int }

// Segment in the parse worker. Even one minified line is bounded, so shaping a
// visible row never sends a whole large file to Gio's editor or text shaper.
func jsonSourceSegments(source string) []jsonSourceSegment {
	return jsonSourceSegmentsWithCancel(source, nil)
}

func jsonSourceSegmentsWithCancel(source string, done <-chan struct{}) []jsonSourceSegment {
	if len(source) <= jsonSourceEditLimit {
		return nil
	}
	segments := make([]jsonSourceSegment, 0, len(source)/1024+1)
	line := 1
	for start := 0; start < len(source); {
		if done != nil && len(segments)&127 == 0 {
			select {
			case <-done:
				return nil
			default:
			}
		}
		end := min(start+1024, len(source))
		for end > start && end < len(source) && !utf8.RuneStart(source[end]) {
			end--
		}
		if end == start {
			end = min(start+1024, len(source))
		}
		if newline := strings.IndexByte(source[start:end], '\n'); newline >= 0 {
			segments = append(segments, jsonSourceSegment{start: start, end: start + newline, line: line})
			start += newline + 1
			line++
		} else {
			segments = append(segments, jsonSourceSegment{start: start, end: end, line: line})
			start = end
		}
	}
	return segments
}

func jsonTextPreview(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	end := limit
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "…"
}

func (a *App) jsonDraftText() string {
	if a.jsonView != nil && !a.jsonView.sourceLoaded {
		return a.jsonDoc.Text()
	}
	return a.jsonEditor.Text()
}

func (a *App) layoutJSONSourcePreview(gtx layout.Context, source string, segments []jsonSourceSegment, list *widget.List) layout.Dimensions {
	if len(segments) == 0 {
		return jsonLabel(gtx, a, a.textID("json.source_preview.preparing"), 13, mutedText, false, false)
	}
	return list.Layout(gtx, len(segments), func(gtx layout.Context, index int) layout.Dimensions {
		segment := segments[index]
		return layout.Inset{Top: 3, Bottom: 3, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return jsonLabel(gtx, a, strconv.Itoa(segment.line), 10, homeMuted, false, true)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					// Gio retains strings in its layout cache. Detach the visible
					// segment so a cached row cannot pin the whole source file.
					style := material.Body2(a.theme, strings.Clone(source[segment.start:segment.end]))
					style.TextSize = 13
					style.Font.Typeface = jsonMonoFace
					gtx.Constraints.Min.Y = 0
					return style.Layout(gtx)
				}),
			)
		})
	})
}
