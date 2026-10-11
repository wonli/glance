package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"runtime/debug"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/jsondoc"
)

type jsonTreeRow struct {
	node    int
	closing bool
}

type jsonTreeResult struct {
	baselineSegments []jsonSourceSegment
	segments         []jsonSourceSegment
	text             string
	tree             *document.JSONTree
	err              error
}

func (v *jsonWorkspaceView) isCollapsed(id int) bool {
	if state, ok := v.collapsed[id]; ok {
		return state
	}
	if v.tree == nil || id < 0 || id >= len(v.tree.Nodes) {
		return false
	}
	if v.expandedLevel < 0 {
		return false
	}
	return v.tree.Nodes[id].Depth >= v.expandedLevel
}

func (v *jsonWorkspaceView) accept(result jsonTreeResult) {
	v.pending = false
	if result.text != v.requested {
		return
	}
	pointer := ""
	collapsed := make(map[string]bool)
	if v.tree != nil {
		if v.selected >= 0 && v.selected < len(v.tree.Nodes) {
			pointer = v.tree.NodePointer(v.selected)
		}
		if len(v.collapsed) < 1000 {
			for id, closed := range v.collapsed {
				if id >= 0 && id < len(v.tree.Nodes) {
					collapsed[v.tree.NodePointer(id)] = closed
				}
			}
		}
	}
	v.tree, v.err, v.parsed = result.tree, result.err, result.text
	v.sourceSegments = result.segments
	if result.baselineSegments != nil {
		v.baselineSegments = result.baselineSegments
	}
	v.collapsed = make(map[int]bool)
	v.rowButtons, v.toggleButtons, v.outlineButtons = make(map[int]*widget.Clickable), make(map[int]*widget.Clickable), make(map[int]*widget.Clickable)
	selected := 0
	if v.tree != nil {
		if pointer != "" {
			selected = v.tree.FindNodeByPointer(pointer)
		}
		for p, closed := range collapsed {
			target := v.tree.FindNodeByPointer(p)
			if target >= 0 && target < len(v.tree.Nodes) {
				v.collapsed[target] = closed
			}
		}
		v.selectNode(selected)
	}
	v.rebuildRows()
}

func (v *jsonWorkspaceView) selectNode(id int) {
	if v.tree == nil || id < 0 || id >= len(v.tree.Nodes) {
		return
	}
	v.selected = id
	v.valueError = ""
	n := v.tree.Nodes[id]
	value := n.Raw
	if n.Kind == "string" {
		value = n.Display
	}
	v.value.ReadOnly = n.Container() || len(value) > jsonValueEditLimit
	if n.Container() {
		value = fmt.Sprintf("{ %d keys }", v.tree.ChildCount(id))
		if n.Kind == "array" {
			value = fmt.Sprintf("[ %d items ]", v.tree.ChildCount(id))
		}
	} else if len(value) > jsonValueEditLimit {
		value = jsonTextPreview(value, 4096)
	}
	v.value.SetText(value)
}

func (v *jsonWorkspaceView) rebuildRows() {
	v.rows, v.outline = nil, nil
	if v.tree == nil {
		return
	}
	query := strings.ToLower(strings.TrimSpace(v.search.Text()))
	var matches []bool
	if query != "" {
		matches = make([]bool, len(v.tree.Nodes))
		for id, n := range v.tree.Nodes {
			if strings.Contains(strings.ToLower(n.Key), query) || (!n.Container() && strings.Contains(strings.ToLower(n.Display), query)) {
				for at := id; at >= 0 && !matches[at]; at = v.tree.Nodes[at].Parent {
					matches[at] = true
				}
			}
		}
	}
	var walk func(int)
	walk = func(id int) {
		n := v.tree.Nodes[id]
		if query != "" && !matches[id] {
			return
		}
		v.rows = append(v.rows, jsonTreeRow{node: id})
		if n.Container() {
			if !v.isCollapsed(id) || query != "" {
				for child := n.FirstChild; child >= 0; child = v.tree.Nodes[child].NextSibling {
					walk(child)
				}
				if v.tree.ChildCount(id) > 0 {
					v.rows = append(v.rows, jsonTreeRow{node: id, closing: true})
				}
			}
		}
	}
	walk(0)
	maxOutline := 1000
	for id, n := range v.tree.Nodes {
		if n.Container() && (query == "" || matches[id]) {
			if len(v.tree.Nodes) > 10000 && n.Depth > 2 {
				continue
			}
			v.outline = append(v.outline, id)
			if len(v.outline) >= maxOutline {
				break
			}
		}
	}
}

func (a *App) requestJSONTree() {
	v := a.ensureJSONView()
	text := a.jsonDoc.Text()
	v.requested = text
	if v.pending || (v.parsed == text && (v.tree != nil || v.err != nil)) {
		return
	}
	v.pending = true
	if len(text) > jsonSourceEditLimit {
		a.documentSessionState.jsonLarge.Store(true)
	}
	results, window, baseline, done := v.results, a.window, v.baseline, v.done
	a.launchManagedBackground(func() {
		defer releaseClosedJSONWorkerMemory(done, len(text) > jsonSourceEditLimit)
		tree, err := jsondoc.ParseTree(text)
		segments := jsonSourceSegments(text)
		baselineSegments := segments
		if baseline != text {
			baselineSegments = jsonSourceSegments(baseline)
		}
		if len(text) > 512*1024 {
			debug.FreeOSMemory()
		}
		select {
		case <-done:
			return
		default:
		}
		select {
		case results <- jsonTreeResult{text: text, tree: tree, err: err, segments: segments, baselineSegments: baselineSegments}:
		case <-done:
			return
		}
		if window != nil {
			window.Invalidate()
		}
	})
}

func (a *App) layoutJSONTree(gtx layout.Context) layout.Dimensions {
	v := a.ensureJSONView()
	return jsonPanel(gtx, contentSurface, true, false, func(gtx layout.Context) layout.Dimensions {
		if v.tree == nil || len(v.rows) == 0 {
			text := "正在解析 JSON…"
			if v.err != nil {
				text = "JSON 暂时无法解析，请切换源码修正\n" + v.err.Error()
			} else if v.tree != nil {
				text = "没有匹配的键或值"
			}
			return layout.UniformInset(24).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Body2(a.theme, text)
				style.TextSize = 13
				style.Color = mutedText
				return style.Layout(gtx)
			})
		}
		return v.treeList.Layout(gtx, len(v.rows), func(gtx layout.Context, index int) layout.Dimensions {
			row := v.rows[index]
			n := v.tree.Nodes[row.node]
			gtx = jsonFixedHeight(gtx, 38)
			button := jsonViewButton(v.rowButtons, row.node)
			if row.closing {
				return a.layoutJSONTreeRow(gtx, index, row, n)
			}
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return a.layoutJSONTreeRow(gtx, index, row, n) })
		})
	})
}

func (a *App) layoutJSONTreeRow(gtx layout.Context, index int, row jsonTreeRow, n document.JSONNode) layout.Dimensions {
	v := a.ensureJSONView()
	bg := contentSurface
	if index%2 == 1 {
		bg = color.NRGBA{R: 252, G: 252, B: 253, A: 255}
	}
	if v.selected == row.node && !row.closing {
		bg = color.NRGBA{R: 234, G: 242, B: 251, A: 255}
	}
	return jsonPanel(gtx, bg, false, false, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(14 + min(n.Depth, 12)*22), Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(15), gtx.Dp(24)))
					if !n.Container() || row.closing || v.tree.ChildCount(row.node) == 0 {
						return layout.Dimensions{Size: gtx.Constraints.Max}
					}
					button := jsonViewButton(v.toggleButtons, row.node)
					return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						icon := "chevron-down"
						if v.isCollapsed(row.node) && v.search.Text() == "" {
							icon = "chevron-right"
						}
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return jsonIcon(gtx, icon, 15, mutedText) })
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return jsonLabel(gtx, a, fmt.Sprintf("%02d", index+1), 10, color.NRGBA{R: 176, G: 182, B: 189, A: 255}, false, true)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if row.closing {
						mark := "}"
						if n.Kind == "array" {
							mark = "]"
						}
						return jsonLabel(gtx, a, mark, 13, jsonKeyColor, false, true)
					}
					key := fmt.Sprintf("%q", jsonTextPreview(n.Key, 1024)) + " : "
					if n.Parent >= 0 && v.tree.Nodes[n.Parent].Kind == "array" {
						key = jsonTextPreview(n.Key, 1024) + " : "
					}
					value := jsonTextPreview(n.Raw, 4096)
					if n.Container() {
						if n.Kind == "object" {
							value = "{"
						} else {
							value = "["
						}
						if (v.isCollapsed(row.node) && v.search.Text() == "") || v.tree.ChildCount(row.node) == 0 {
							if n.Kind == "object" {
								value = fmt.Sprintf("{ %d keys }", v.tree.ChildCount(row.node))
							} else {
								value = fmt.Sprintf("[ %d items ]", v.tree.ChildCount(row.node))
							}
						}
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return jsonLabel(gtx, a, key, 13, jsonKeyColor, v.selected == row.node, true)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return jsonLabel(gtx, a, value, 13, jsonNodeColor(n), false, true)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if v.selected != row.node || row.closing || gtx.Constraints.Max.X < gtx.Dp(60) {
						return layout.Dimensions{}
					}
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.jsonCopyButton(gtx, "row-copy-value", "copy") }),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return a.jsonCopyButton(gtx, "row-copy-path", "route") })
						}),
					)
				}),
			)
		})
	})
}

func jsonNodeColor(n document.JSONNode) color.NRGBA {
	switch n.Kind {
	case "string":
		return jsonStringColor
	case "number":
		return jsonBlue
	case "boolean":
		return jsonKeyColor
	case "null":
		return homeMuted
	default:
		return homeText
	}
}

func (a *App) jsonCopyButton(gtx layout.Context, id, icon string) layout.Dimensions {
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(26), gtx.Dp(26)))
	button := jsonToolbarButton(a.documentSessionState, a, id)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, contentSurface, jsonBorderColor, 4, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return jsonIcon(gtx, icon, 14, jsonBlue) })
		})
	})
}
