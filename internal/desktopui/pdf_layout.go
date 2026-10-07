package desktopui

import (
	"fmt"
	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/document"
	"image"
	"image/color"
	"math"
	"strconv"
)

var pdfToolbarActions = []ribbonAction{
	{id: "thumbnails", label: "缩略图", group: "view", icon: "panel-left"}, {id: "search", label: "搜索", group: "view", icon: "find"},
	{id: "previous", label: "上一页", group: "page", icon: "undo-2"}, {id: "next", label: "下一页", group: "page", icon: "redo-2"},
	{id: "zoom-out", label: "缩小", group: "zoom", icon: "view-zoom"}, {id: "zoom-in", label: "放大", group: "zoom", icon: "zoom-in"}, {id: "fit-width", label: "适合宽度", group: "zoom", icon: "view-multiple-pages"}, {id: "fit-page", label: "适合页面", group: "zoom", icon: "view-single-page"}, {id: "rotate", label: "旋转", group: "zoom", icon: "rotate-cw"},
	{id: "highlight", label: "高亮", group: "annotate", icon: "text-highlight"}, {id: "note", label: "添加批注", group: "annotate", icon: "review-new-comment"}, {id: "ink", label: "自由绘制", group: "annotate", icon: "italic"},
	{id: "undo", label: "撤销", group: "history", icon: "undo-2"}, {id: "redo", label: "重做", group: "history", icon: "redo-2"},
	{id: "save", label: "保存", group: "file", icon: "save"}, {id: "print", label: "打印", group: "file", icon: "print"}, {id: "download", label: "另存为", group: "file", icon: "save-as"},
}

func (a *App) layoutPDFDocument(gtx layout.Context) layout.Dimensions {
	a.handlePDFEvents(gtx)
	v := a.ensurePDFView()
	if r, ok := v.cache[v.page]; ok {
		v.scene = r.page
	} else {
		v.scene = document.PDFPage{}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(a.layoutPDFRibbon), layout.Flexed(1, a.layoutPDFWorkspace), layout.Rigid(a.layoutPDFStatus))
}
func (a *App) layoutPDFRibbon(gtx layout.Context) layout.Dimensions {
	v := a.ensurePDFView()
	gtx = jsonFixedHeight(gtx, 54)
	return jsonPanel(gtx, contentSurface, false, true, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return v.toolbar.Layout(gtx, len(pdfToolbarActions), func(gtx layout.Context, i int) layout.Dimensions {
					action := pdfToolbarActions[i]
					enabled := !v.pending && !a.isSaving()
					if action.id == "undo" {
						enabled = enabled && v.info.CanUndo
					}
					if action.id == "redo" {
						enabled = enabled && v.info.CanRedo
					}
					icon, ok := svgIconTintAtSize("pdf", action.id, 192, toolbarIconColor(enabled))
					if !ok {
						icon, _ = markdownToolbarIcon(action, enabled)
					}
					return layoutToolbarActionItem(gtx, action, i, pdfToolbarActions, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(48), gtx.Dp(48)))
						return toolbarIconButton(gtx, a.theme, v.button(action.id), icon, 18, action.label, action.label, enabled, false, contentSurface)
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 8, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					var children []layout.FlexChild
					for _, item := range []struct{ id, label string }{{"read", "阅读"}, {"edit", "编辑"}, {"annotate", "批注"}, {"signature", "签名"}} {
						item := item
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(48), gtx.Dp(30)))
							return v.button(item.id).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								if v.mode == item.id {
									paint.FillShape(gtx.Ops, selectedFill, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(4)).Op(gtx.Ops))
								}
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return material.Label(a.theme, 11, item.label).Layout(gtx) })
							})
						}))
					}
					return layout.Flex{}.Layout(gtx, children...)
				})
			}),
		)
	})
}
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
	r, ok := v.cache[v.page]
	if !ok || r.pixels == nil {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			msg := "正在读取页面…"
			if r.err != nil {
				msg = r.err.Error()
			}
			return material.Body2(a.theme, msg).Layout(gtx)
		})
	}
	v.scene = r.page
	width, height := gtx.Constraints.Max.X-gtx.Dp(48), gtx.Constraints.Max.Y-gtx.Dp(48)
	scale := float64(gtx.Metric.PxPerDp) * 4 / 3 * v.zoom
	if v.fit != "" {
		scale = float64(max(1, width)) / r.page.Width
		if v.fit == "page" {
			scale = math.Min(scale, float64(max(1, height))/r.page.Height)
		}
	}
	if v.scale != scale {
		gtx.Execute(op.InvalidateCmd{})
	}
	v.scale = scale
	viewport := gtx.Constraints.Max
	return v.horizontal.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(max(viewport.X, int(r.page.Width*scale)+gtx.Dp(48)), viewport.Y))
		return v.canvas.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return layout.UniformInset(24).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.N.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(max(1, int(r.page.Width*scale)), max(1, int(r.page.Height*scale)))
					gtx.Constraints = layout.Exact(size)
					area := clip.Rect{Max: size}.Push(gtx.Ops)
					defer area.Pop()
					event.Op(gtx.Ops, &v.tag)
					widget.Image{Src: paint.NewImageOp(r.pixels), Fit: widget.Fill}.Layout(gtx)
					if v.selected >= 0 && v.selected < len(r.page.Text) {
						b := r.page.Text[v.selected].Bounds
						rect := image.Rect(int(b.X*scale), int(b.Y*scale), int((b.X+b.W)*scale), int((b.Y+b.H)*scale))
						paint.FillShape(gtx.Ops, color.NRGBA{R: 43, G: 108, B: 176, A: 65}, clip.Rect(rect).Op())
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
				if v.selected < 0 || v.selected >= len(v.scene.Text) {
					return layout.Inset{Top: 12, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return material.Label(a.theme, 11, "点击文字选择 · 编辑模式可修改文字").Layout(gtx)
					})
				}
				return layout.Inset{Top: 12, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							l := material.Label(a.theme, 11, v.scene.Text[v.selected].Text)
							l.MaxLines = 5
							return l.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Button(a.theme, v.button("copy"), "复制").Layout(gtx)
							}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Button(a.theme, v.button("replace"), "编辑").Layout(gtx)
							}))
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
	return jsonPanel(gtx, pageBackground, true, false, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Label(a.theme, 11, fmt.Sprintf("%d / %d 页 · %.1f MB", v.page+1, v.info.Pages, float64(v.info.Bytes)/(1024*1024))).Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						l := material.Label(a.theme, 11, v.message)
						l.MaxLines = 1
						return l.Layout(gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					state := "已保存"
					if v.info.Dirty {
						state = "未保存"
					}
					return material.Label(a.theme, 11, fmt.Sprintf("%s   %.0f%%   PDF", state, v.scale/(float64(gtx.Metric.PxPerDp)*4/3)*100)).Layout(gtx)
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

func pdfCard(gtx layout.Context, content layout.Widget) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	recording := op.Record(gtx.Ops)
	dims := content(gtx)
	call := recording.Stop()
	paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: dims.Size}.Op())
	call.Add(gtx.Ops)
	return dims
}
