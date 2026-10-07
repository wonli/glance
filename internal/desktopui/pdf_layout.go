package desktopui

import (
	"fmt"
	"gioui.org/font"
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
	{id: "open", label: "打开", group: "file", icon: "open"},
	{id: "close", label: "关闭", group: "file", icon: "close"},
	{id: "save", label: "保存", group: "file", icon: "save"},
	{id: "save-as", label: "另存为", group: "file", icon: "save-as"},

	{id: "thumbnails", label: "缩略图", group: "view", icon: "thumbnails"},
	{id: "search", label: "搜索", group: "view", icon: "search"},

	{id: "previous", label: "上一页", group: "page", icon: "previous"},
	{id: "page-indicator", label: "页码", group: "page", icon: ""},
	{id: "next", label: "下一页", group: "page", icon: "next"},

	{id: "zoom-out", label: "缩小", group: "zoom", icon: "zoom-out"},
	{id: "zoom-indicator", label: "缩放", group: "zoom", icon: ""},
	{id: "zoom-in", label: "放大", group: "zoom", icon: "zoom-in"},
	{id: "fit-width", label: "适合宽度", group: "zoom", icon: "fit-width"},
	{id: "fit-page", label: "适合页面", group: "zoom", icon: "fit-page"},
	{id: "rotate", label: "旋转", group: "zoom", icon: "rotate"},

	{id: "highlight", label: "高亮", group: "annotate", icon: "highlight"},
	{id: "note", label: "添加批注", group: "annotate", icon: "note"},
	{id: "ink", label: "自由绘制", group: "annotate", icon: "ink"},

	{id: "undo", label: "撤销", group: "history", icon: "undo"},
	{id: "redo", label: "重做", group: "history", icon: "redo"},
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
					if action.id == "page-indicator" {
						return layoutToolbarActionItem(gtx, action, i, pdfToolbarActions, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(82), gtx.Dp(34)))
							paint.FillShape(gtx.Ops, color.NRGBA{R: 221, G: 226, B: 231, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(5)).Op(gtx.Ops))
							paint.FillShape(gtx.Ops, color.NRGBA{R: 247, G: 248, B: 250, A: 255}, clip.UniformRRect(image.Rect(1, 1, gtx.Constraints.Max.X-1, gtx.Constraints.Max.Y-1), gtx.Dp(4)).Op(gtx.Ops))
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										l := material.Label(a.theme, 12, fmt.Sprintf("%d", v.page+1))
										l.Color = color.NRGBA{R: 52, G: 64, B: 75, A: 255}
										l.Font.Weight = font.SemiBold
										return l.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											l := material.Label(a.theme, 12, fmt.Sprintf("/ %d", max(1, v.info.Pages)))
											l.Color = color.NRGBA{R: 122, G: 131, B: 142, A: 255}
											return l.Layout(gtx)
										})
									}),
								)
							})
						})
					}
					if action.id == "zoom-indicator" {
						return layoutToolbarActionItem(gtx, action, i, pdfToolbarActions, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(64), gtx.Dp(34)))
							paint.FillShape(gtx.Ops, color.NRGBA{R: 221, G: 226, B: 231, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(5)).Op(gtx.Ops))
							paint.FillShape(gtx.Ops, color.NRGBA{R: 247, G: 248, B: 250, A: 255}, clip.UniformRRect(image.Rect(1, 1, gtx.Constraints.Max.X-1, gtx.Constraints.Max.Y-1), gtx.Dp(4)).Op(gtx.Ops))
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								l := material.Label(a.theme, 11, fmt.Sprintf("%.0f%%", v.zoom*100))
								l.Color = color.NRGBA{R: 78, G: 88, B: 99, A: 255}
								l.Font.Weight = font.SemiBold
								return l.Layout(gtx)
							})
						})
					}
					enabled := !v.pending && !a.isSaving()
					if action.id == "undo" {
						enabled = enabled && v.info.CanUndo
					}
					if action.id == "redo" {
						enabled = enabled && v.info.CanRedo
					}
					iconCol := toolbarIconColor(enabled)
					if action.id == "thumbnails" && v.thumbnails {
						iconCol = color.NRGBA{R: 43, G: 108, B: 176, A: 255}
					}
					icon, ok := svgIconTintAtSize("pdf", action.icon, 192, iconCol)
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
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(a.layoutPDFModeSwitch),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								enabled := !v.pending && !a.isSaving()
								icon, _ := svgIconTintAtSize("pdf", "print", 192, toolbarIconColor(enabled))
								gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(48), gtx.Dp(48)))
								return toolbarIconButton(gtx, a.theme, v.button("print"), icon, 18, "打印", "打印", enabled, false, contentSurface)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								enabled := !v.pending && !a.isSaving()
								icon, _ := svgIconTintAtSize("pdf", "download", 192, toolbarIconColor(enabled))
								gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(48), gtx.Dp(48)))
								return toolbarIconButton(gtx, a.theme, v.button("download"), icon, 18, "下载", "下载", enabled, false, contentSurface)
							})
						}),
					)
				})
			}),
		)
	})
}

func (a *App) layoutPDFModeSwitch(gtx layout.Context) layout.Dimensions {
	v := a.ensurePDFView()
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(202), gtx.Dp(34)))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 241, G: 243, B: 245, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(6)).Op(gtx.Ops))
	return layout.UniformInset(3).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var children []layout.FlexChild
		modes := []struct{ id, label string }{
			{"read", "阅读"},
			{"annotate", "批注"},
			{"signature", "签名"},
		}
		for _, item := range modes {
			item := item
			active := v.mode == item.id
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(28)))
				return v.button(item.id).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if active {
						shadowRect := image.Rect(0, gtx.Dp(1), gtx.Constraints.Max.X, gtx.Constraints.Max.Y+gtx.Dp(1))
						paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 20}, clip.UniformRRect(shadowRect, gtx.Dp(4)).Op(gtx.Ops))
						paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(4)).Op(gtx.Ops))
					}
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						l := material.Label(a.theme, 11, item.label)
						if active {
							l.Color = color.NRGBA{R: 43, G: 108, B: 176, A: 255}
							l.Font.Weight = font.SemiBold
						} else {
							l.Color = color.NRGBA{R: 104, G: 113, B: 123, A: 255}
						}
						return l.Layout(gtx)
					})
				})
			}))
		}
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx, children...)
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
	scale := float64(gtx.Metric.PxPerDp) * 4 / 3 * v.zoom
	if v.fit != "" {
		scale = float64(max(1, width)) / refW
		if v.fit == "page" {
			scale = math.Min(scale, float64(max(1, height))/refH)
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
