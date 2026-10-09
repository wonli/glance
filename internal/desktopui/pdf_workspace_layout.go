package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

func (a *App) layoutPDFWorkspace(gtx layout.Context) layout.Dimensions {
	v := a.ensurePDFView()
	wide := gtx.Constraints.Max.X >= gtx.Dp(1000)
	return layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !v.thumbnails {
				return layout.Dimensions{}
			}
			w := gtx.Dp(220)
			if !wide {
				w = gtx.Dp(140)
			}
			gtx.Constraints.Min.X = w
			gtx.Constraints.Max.X = w
			return a.layoutPDFThumbnails(gtx)
		}),
		layout.Flexed(1, a.layoutPDFCanvas),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !wide && v.mode == "read" && !v.showSearch {
				return layout.Dimensions{}
			}
			w := gtx.Dp(300)
			if !wide {
				w = gtx.Dp(220)
			}
			gtx.Constraints.Min.X = w
			gtx.Constraints.Max.X = w
			return a.layoutPDFNotes(gtx)
		}),
	)
}

func (a *App) layoutPDFThumbnails(gtx layout.Context) layout.Dimensions {
	v := a.ensurePDFView()
	paint.FillShape(gtx.Ops, pageBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.Label(a.theme, 12, fmt.Sprintf("页面               %d 页", v.info.Pages)).Layout(gtx)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return v.thumbList.Layout(gtx, v.info.Pages, func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Left: 12, Right: 12, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return v.button("page-"+strconv.Itoa(i)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if i == v.page {
							paint.FillShape(gtx.Ops, selectedFill, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(154))}.Op())
						}
						return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints = layout.Exact(image.Pt(min(gtx.Constraints.Max.X, gtx.Dp(110)), gtx.Dp(124)))
									if img := v.thumbCache[i]; img != nil {
										return widget.Image{Src: paint.NewImageOp(img), Fit: widget.Contain}.Layout(gtx)
									}
									paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
									return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return material.Label(a.theme, 11, fmt.Sprintf("第 %d 页", i+1)).Layout(gtx)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return material.Label(a.theme, 10, strconv.Itoa(i+1)).Layout(gtx)
									})
								}),
							)
						})
					})
				})
			})
		}),
	)
}

func (a *App) layoutPDFCanvas(gtx layout.Context) layout.Dimensions {
	v := a.ensurePDFView()
	paint.FillShape(gtx.Ops, color.NRGBA{R: 231, G: 236, B: 240, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
	totalPages := max(1, v.info.Pages)
	if v.canvas.Position.First >= 0 && v.canvas.Position.First < totalPages && v.page != v.canvas.Position.First {
		v.page = v.canvas.Position.First
		if r, ok := v.cache[v.page]; ok {
			v.scene = r.page
		}
	}
	refW, refH := 595.0, 842.0
	if r, ok := v.cache[v.page]; ok && r.page.Width > 0 {
		refW = r.page.Width
		refH = r.page.Height
	} else {
		for _, cached := range v.cache {
			if cached.page.Width > 0 {
				refW = cached.page.Width
				refH = cached.page.Height
				break
			}
		}
	}
	width, height := gtx.Constraints.Max.X-gtx.Dp(48), gtx.Constraints.Max.Y-gtx.Dp(48)
	baseScale := float64(gtx.Metric.PxPerDp) * 4 / 3
	scale := baseScale * v.zoom
	if v.fit != "" {
		scale = float64(max(1, width)) / refW
		if v.fit == "page" {
			scale = math.Min(scale, float64(max(1, height))/refH)
		}
		if baseScale > 0 {
			v.zoom = scale / baseScale
		}
	}
	if v.scale != scale {
		gtx.Execute(op.InvalidateCmd{})
	}
	v.scale = scale
	viewport := gtx.Constraints.Max
	contentWidth := max(viewport.X, int(refW*scale)+gtx.Dp(48))
	return v.horizontal.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(contentWidth, viewport.Y))
		return v.canvas.Layout(gtx, totalPages, func(gtx layout.Context, pageIdx int) layout.Dimensions {
			pageW, pageH := refW, refH
			r, ok := v.cache[pageIdx]
			if ok && r.page.Width > 0 {
				pageW = r.page.Width
				pageH = r.page.Height
			}
			size := image.Pt(max(1, int(pageW*scale)), max(1, int(pageH*scale)))
			return layout.Inset{Top: 12, Bottom: 12, Left: 24, Right: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(size)
					shadowOffset := gtx.Dp(2)
					shadowRect := image.Rect(-shadowOffset, -shadowOffset, size.X+shadowOffset, size.Y+shadowOffset)
					paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 20}, clip.UniformRRect(shadowRect, gtx.Dp(3)).Op(gtx.Ops))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: size}.Op())

					area := clip.Rect{Max: size}.Push(gtx.Ops)
					defer area.Pop()

					if pageIdx == v.page {
						event.Op(gtx.Ops, &v.tag)
					}

					if ok && r.pixels != nil {
						widget.Image{Src: paint.NewImageOp(r.pixels), Fit: widget.Fill}.Layout(gtx)
					} else {
						a.requestPDFPage(pageIdx)
						layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							msg := fmt.Sprintf("正在读取第 %d 页…", pageIdx+1)
							if r.err != nil {
								msg = r.err.Error()
							}
							return material.Body2(a.theme, msg).Layout(gtx)
						})
					}

					if pageIdx == v.page {
						start, end := v.selectionRange()
						if start >= 0 && end >= 0 && len(r.page.Text) > 0 {
							start = max(0, min(start, len(r.page.Text)-1))
							end = max(0, min(end, len(r.page.Text)-1))
							for i := start; i <= end; i++ {
								b := r.page.Text[i].Bounds
								rect := image.Rect(int(b.X*scale), int(b.Y*scale), int((b.X+b.W)*scale), int((b.Y+b.H)*scale))
								paint.FillShape(gtx.Ops, color.NRGBA{R: 43, G: 108, B: 176, A: 65}, clip.Rect(rect).Op())
							}
						}
						if len(v.ink) > 1 {
							var path clip.Path
							path.Begin(gtx.Ops)
							for i, p := range v.ink {
								pt := pdfPoint(p.X*scale, p.Y*scale)
								if i == 0 {
									path.MoveTo(pt)
								} else {
									path.LineTo(pt)
								}
							}
							paint.FillShape(gtx.Ops, color.NRGBA{R: 43, G: 108, B: 176, A: 255}, clip.Stroke{Path: path.End(), Width: float32(1.5 * scale)}.Op())
						}
					}

					return layout.Dimensions{Size: size}
				})
			})
		})
	})
}

func (a *App) layoutPDFNotes(gtx layout.Context) layout.Dimensions {
	v := a.ensurePDFView()
	paint.FillShape(gtx.Ops, pageBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.UniformInset(18).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return material.Label(a.theme, 13, "批注").Layout(gtx) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !v.showSearch {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: 12, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return material.Editor(a.theme, &v.search, "搜索 PDF").Layout(gtx)
					}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Button(a.theme, v.button("find"), "查找").Layout(gtx)
					}))
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				selText := v.selectedText()
				if selText == "" && v.selected >= 0 && v.selected < len(v.scene.Text) {
					selText = v.scene.Text[v.selected].Text
				}
				if selText == "" {
					return layout.Inset{Top: 12, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return material.Label(a.theme, 11, "点击文字或拖选 · 编辑模式可修改文字").Layout(gtx)
					})
				}
				return layout.Inset{Top: 12, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							l := material.Label(a.theme, 11, selText)
							l.MaxLines = 6
							return l.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(a.theme, v.button("copy"), "复制")
										btn.Inset = layout.Inset{Top: 6, Bottom: 6, Left: 14, Right: 14}
										return btn.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Dimensions{Size: image.Pt(gtx.Dp(8), 0)}
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(a.theme, v.button("replace"), "编辑")
										btn.Inset = layout.Inset{Top: 6, Bottom: 6, Left: 14, Right: 14}
										return btn.Layout(gtx)
									}),
								)
							})
						}),
					)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Label(a.theme, 11, fmt.Sprintf("当前页批注 · %d", len(v.scene.Annotations))).Layout(gtx)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return v.notes.Layout(gtx, len(v.scene.Annotations), func(gtx layout.Context, i int) layout.Dimensions {
					ann := v.scene.Annotations[i]
					return layout.Inset{Bottom: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return pdfCard(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return material.Label(a.theme, 10, fmt.Sprintf("第 %d 页 · %s", v.page+1, ann.Kind)).Layout(gtx)
								}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: 9, Bottom: 9}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return material.Label(a.theme, 11, ann.Text).Layout(gtx) })
								}), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return material.Label(a.theme, 9, ann.Author).Layout(gtx) }))
							})
						})
					})
				})
			}),
		)
	})
}

func (a *App) layoutPDFStatus(gtx layout.Context) layout.Dimensions {
	v := a.ensurePDFView()
	gtx = jsonFixedHeight(gtx, 34)
	return jsonPanel(gtx, pageBackground, false, false, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, jsonBorderColor, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))}.Op())
		return layout.Inset{Top: 4, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					return material.Label(a.theme, 11, fmt.Sprintf("%d / %d 页 · %.1f MB", v.page+1, v.info.Pages, float64(v.info.Bytes)/(1024*1024))).Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						l := material.Label(a.theme, 11, v.message)
						l.MaxLines = 1
						return l.Layout(gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					state := "已保存"
					if v.info.Dirty {
						state = "未保存"
					}
					return material.Label(a.theme, 11, fmt.Sprintf("%s   %.0f%%   PDF", state, v.currentZoom(gtx.Metric)*100)).Layout(gtx)
				}),
			)
		})
	})
}

// Keep pointer coordinates in page points; display scaling never enters saved geometry.
func pdfHit(p document.PDFPage, x, y float64) int {
	for i, t := range p.Text {
		b := t.Bounds
		if x >= b.X && x <= b.X+b.W && y >= b.Y && y <= b.Y+b.H {
			return i
		}
	}
	return -1
}

func pdfHitOrNearest(p document.PDFPage, x, y float64) int {
	if hit := pdfHit(p, x, y); hit >= 0 {
		return hit
	}
	if len(p.Text) == 0 {
		return -1
	}
	bestIdx := -1
	bestDist := math.MaxFloat64
	for i, t := range p.Text {
		b := t.Bounds
		cx := math.Max(b.X, math.Min(x, b.X+b.W))
		cy := math.Max(b.Y, math.Min(y, b.Y+b.H))
		dx := x - cx
		dy := y - cy
		dist := dx*dx + dy*dy
		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}
	return bestIdx
}

func pdfCard(gtx layout.Context, content layout.Widget) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	recording := op.Record(gtx.Ops)
	dims := content(gtx)
	call := recording.Stop()
	paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: dims.Size}.Op())
	call.Add(gtx.Ops)
	return dims
}
