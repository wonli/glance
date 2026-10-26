package desktopui

import (
	"fmt"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/workspace"
	"image"
	"image/color"
	"runtime"
)

var proofreadingDesignBlue = color.NRGBA{R: 55, G: 105, B: 142, A: 255}

func (a *App) proofreadingAction(g layout.Context, id, label string, action func()) layout.Dimensions {
	g.Constraints.Min = image.Point{}
	if id == "new" {
		g.Constraints = layout.Exact(image.Pt(g.Dp(139), g.Dp(38)))
	}
	if id == "save-identity" {
		g.Constraints = layout.Exact(image.Pt(g.Dp(172), g.Dp(42)))
	}
	w := a.ensureProofreadingWorkbench()
	b := w.button(id)
	if b.Clicked(g) && !w.busy {
		action()
		g.Execute(op.InvalidateCmd{})
	}
	if id == "new" {
		size := g.Constraints.Max
		return b.Layout(g, func(g layout.Context) layout.Dimensions {
			paint.FillShape(g.Ops, proofreadingDesignBlue, clip.UniformRRect(image.Rectangle{Max: size}, g.Dp(6)).Op(g.Ops))
			layout.Center.Layout(g, func(g layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(g, layout.Rigid(func(g layout.Context) layout.Dimensions {
					return layoutProofreadingIcon(g, "home", "plus", 16, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
				}), layout.Rigid(func(g layout.Context) layout.Dimensions {
					return layout.Inset{Left: 7}.Layout(g, func(g layout.Context) layout.Dimensions {
						return a.proofreadingText(g, a.textID(label), 13, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, true)
					})
				}))
			})
			return layout.Dimensions{Size: size}
		})
	}
	s := material.Button(a.theme, b, a.textID(label))
	s.TextSize = 12
	s.Background = proofreadingDesignBlue
	return s.Layout(g)
}
func (a *App) proofreadingText(g layout.Context, text string, size unit.Sp, ink color.NRGBA, bold bool) layout.Dimensions {
	g.Constraints.Min.Y = 0
	l := material.Label(a.theme, size, text)
	l.Color = ink
	if bold {
		l.Font.Weight = font.SemiBold
	}
	return l.Layout(g)
}
func proofreadingAt(g layout.Context, x, y, w, h unit.Dp, draw layout.Widget) {
	offset := op.Offset(image.Pt(g.Dp(x), g.Dp(y))).Push(g.Ops)
	defer offset.Pop()
	g.Constraints = layout.Exact(image.Pt(g.Dp(w), g.Dp(h)))
	draw(g)
}
func proofreadingCard(g layout.Context, height unit.Dp, draw layout.Widget) layout.Dimensions {
	size := image.Pt(g.Constraints.Max.X, g.Dp(height))
	g.Constraints = layout.Exact(size)
	paint.FillShape(g.Ops, contentSurface, clip.UniformRRect(image.Rectangle{Max: size}, g.Dp(8)).Op(g.Ops))
	proofreadRoundedBorder(g, size, 8, color.NRGBA{R: 225, G: 230, B: 233, A: 255}, max(1, g.Dp(1)))
	draw(g)
	return layout.Dimensions{Size: size}
}
func (a *App) layoutProofreadingWorkbench(g layout.Context) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	if !w.loaded && !w.busy {
		a.loadProofreadingWorkbench()
	}
	for _, e := range []*widget.Editor{&w.name, &w.email, &w.remote, &w.authUser, &w.token, &w.ssh} {
		for {
			_, ok := e.Update(g)
			if !ok {
				break
			}
		}
	}
	if a.clientTitlebar && a.proofreading != nil {
		a.processProofreadingWindowControlClicks(g)
	}
	paint.FillShape(g.Ops, color.NRGBA{R: 248, G: 250, B: 251, A: 255}, clip.Rect{Max: g.Constraints.Max}.Op())
	return layout.Flex{Axis: layout.Vertical}.Layout(g,
		layout.Rigid(a.layoutProofreadingWorkbenchHeader),
		layout.Flexed(1, func(g layout.Context) layout.Dimensions {
			bodyWidth := min(g.Constraints.Max.X, g.Dp(988))
			side := max(g.Dp(24), (g.Constraints.Max.X-bodyWidth)/2)
			if g.Constraints.Max.X < g.Dp(1036) {
				bodyWidth = max(1, g.Constraints.Max.X-2*side)
			}
			shift := op.Offset(image.Pt(side, 0)).Push(g.Ops)
			defer shift.Pop()
			g.Constraints.Min.X = bodyWidth
			g.Constraints.Max.X = bodyWidth
			g.Constraints.Min.Y = 0
			if w.settings {
				return w.list.Layout(g, 1, func(g layout.Context, _ int) layout.Dimensions {
					return layout.Inset{Top: 61, Bottom: 40}.Layout(g, a.layoutProofreadingIdentity)
				})
			}
			return layout.Inset{Top: 61}.Layout(g, func(g layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(g,
					layout.Rigid(func(g layout.Context) layout.Dimensions {
						size := image.Pt(g.Constraints.Max.X, g.Dp(104))
						g.Constraints = layout.Exact(size)
						width := unit.Dp(float32(size.X) / g.Metric.PxPerDp)
						proofreadingAt(g, 0, 0, width-160, 40, func(g layout.Context) layout.Dimensions {
							return a.proofreadingText(g, a.textID("proofread.workbench.list"), 27, proofreadInk, true)
						})
						proofreadingAt(g, 0, 49, width, 22, func(g layout.Context) layout.Dimensions {
							return a.proofreadingText(g, a.textID("proofread.workbench.subtitle"), 13, proofreadMuted, false)
						})
						proofreadingAt(g, width-139, 3, 139, 38, func(g layout.Context) layout.Dimensions {
							return a.proofreadingAction(g, "new", "proofread.workbench.new", a.newProofreadingProject)
						})
						return layout.Dimensions{Size: size}
					}),
					layout.Rigid(func(g layout.Context) layout.Dimensions {
						if w.message == "" {
							return layout.Dimensions{}
						}
						return layout.Inset{Bottom: 12}.Layout(g, material.Body2(a.theme, w.message).Layout)
					}),
					layout.Flexed(1, func(g layout.Context) layout.Dimensions {
						if len(w.projects) == 0 {
							return material.Body2(a.theme, a.textID("proofread.workbench.empty")).Layout(g)
						}
						return w.list.Layout(g, len(w.projects), func(g layout.Context, i int) layout.Dimensions {
							return a.layoutProofreadingProjectRow(g, w.projects[i])
						})
					}),
				)
			})
		}),
	)
}
func (a *App) layoutProofreadingWorkbenchHeader(g layout.Context) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	size := image.Pt(g.Constraints.Max.X, g.Dp(64))
	g.Constraints = layout.Exact(size)
	paint.FillShape(g.Ops, contentSurface, clip.Rect{Max: size}.Op())
	paint.FillShape(g.Ops, proofreadChromeLine, clip.Rect{Min: image.Pt(0, size.Y-max(1, g.Dp(1))), Max: size}.Op())
	return layout.Inset{Left: 32, Right: 32}.Layout(g, func(g layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(g,
			layout.Rigid(func(g layout.Context) layout.Dimensions {
				if !a.clientTitlebar || a.proofreading == nil || a.proofreadingTabHost != nil {
					return layout.Dimensions{}
				}
				if runtime.GOOS == "darwin" {
					return a.layoutMacWindowControls(g)
				}
				return a.layoutStandardWindowControls(g)
			}),
			layout.Rigid(func(g layout.Context) layout.Dimensions {
				return layoutProofreadingIcon(g, "ribbon", "files", 20, color.NRGBA{R: 66, G: 109, B: 145, A: 255})
			}),
			layout.Rigid(func(g layout.Context) layout.Dimensions {
				return layout.Inset{Left: 10}.Layout(g, func(g layout.Context) layout.Dimensions {
					return a.proofreadingText(g, a.textID("proofread.workspace.name"), 16, proofreadInk, true)
				})
			}),
			layout.Flexed(1, func(g layout.Context) layout.Dimensions {
				return layoutWindowMoveArea(g, func(g layout.Context) layout.Dimensions { return layout.Dimensions{Size: g.Constraints.Max} })
			}),
			layout.Rigid(func(g layout.Context) layout.Dimensions {
				if w.settings {
					return layout.Dimensions{}
				}
				return a.proofreadingOutlinedAction(g, "settings", "proofread.workbench.settings", func() {
					w.settings = true
					w.historyOpen = false
					w.authMode = ""
					w.message = ""
					w.list.Position = layout.Position{}
				})
			}),
		)
	})
}
func (a *App) proofreadingOutlinedAction(g layout.Context, id, label string, action func()) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	b := w.button(id)
	if b.Clicked(g) && !w.busy {
		action()
		g.Execute(op.InvalidateCmd{})
	}
	width := unit.Dp(100)
	height := unit.Dp(36)
	if id == "add-ssh" || id == "add-token" {
		width = 134
		height = 34
	}
	size := image.Pt(g.Dp(width), g.Dp(height))
	g.Constraints = layout.Exact(size)
	return b.Layout(g, func(g layout.Context) layout.Dimensions {
		proofreadRoundedBorder(g, size, 6, color.NRGBA{R: 214, G: 225, B: 232, A: 255}, max(1, g.Dp(1)))
		return layout.Center.Layout(g, func(g layout.Context) layout.Dimensions {
			if id == "settings" {
				return layout.Flex{Alignment: layout.Middle}.Layout(g, layout.Rigid(func(g layout.Context) layout.Dimensions {
					return layoutProofreadingIcon(g, "home", "settings-2", 14, proofreadingDesignBlue)
				}), layout.Rigid(func(g layout.Context) layout.Dimensions {
					return layout.Inset{Left: 8}.Layout(g, func(g layout.Context) layout.Dimensions {
						return a.proofreadingText(g, a.textID(label), 12, proofreadingDesignBlue, false)
					})
				}))
			}
			return a.proofreadingText(g, a.textID(label), 12, proofreadingDesignBlue, false)
		})
	})
}
func (a *App) layoutProofreadingProjectRow(g layout.Context, p workspace.ProofreadingProject) layout.Dimensions {
	size := image.Pt(g.Constraints.Max.X, g.Dp(125))
	g.Constraints = layout.Exact(size)
	paint.FillShape(g.Ops, contentSurface, clip.Rect{Max: size}.Op())
	paint.FillShape(g.Ops, color.NRGBA{R: 237, G: 240, B: 242, A: 255}, clip.Rect{Min: image.Pt(0, size.Y-max(1, g.Dp(1))), Max: size}.Op())
	width := unit.Dp(float32(size.X) / g.Metric.PxPerDp)
	proofreadingAt(g, 27, 32, width-260, 24, func(g layout.Context) layout.Dimensions {
		return a.proofreadingText(g, p.Title, 14, proofreadInk, true)
	})
	proofreadingAt(g, 27, 64, width-260, 22, func(g layout.Context) layout.Dimensions {
		return a.proofreadingText(g, fmt.Sprintf(a.textID("proofread.workbench.progress_short"), p.CurrentPage+1, p.ReviewedCount, p.PageCount), 12, proofreadMuted, false)
	})
	proofreadingAt(g, width-185, 32, 158, 20, func(g layout.Context) layout.Dimensions {
		l := material.Label(a.theme, 11, p.UpdatedAt.Local().Format("2006-01-02 15:04"))
		l.Color = proofreadMuted
		return layout.E.Layout(g, l.Layout)
	})
	proofreadingAt(g, width-240, 61, 213, 26, func(g layout.Context) layout.Dimensions {
		w := a.ensureProofreadingWorkbench()
		bDelete := w.button("delete-" + p.ID)
		if bDelete.Clicked(g) && !w.busy {
			a.confirmDeleteProofreadingProject(p)
		}
		bOpen := w.button("open-" + p.ID)
		if bOpen.Clicked(g) && !w.busy {
			a.resumeProofreadingProject(p)
		}
		return layout.E.Layout(g, func(g layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(g,
				layout.Rigid(func(g layout.Context) layout.Dimensions {
					return bDelete.Layout(g, func(g layout.Context) layout.Dimensions {
						return layout.Inset{Left: 6, Right: 6, Top: 4, Bottom: 4}.Layout(g, func(g layout.Context) layout.Dimensions {
							return a.proofreadingText(g, a.textID("common.action.delete"), 12, color.NRGBA{R: 204, G: 68, B: 68, A: 255}, false)
						})
					})
				}),
				layout.Rigid(func(g layout.Context) layout.Dimensions {
					return layout.Inset{Left: 12}.Layout(g, func(g layout.Context) layout.Dimensions {
						return bOpen.Layout(g, func(g layout.Context) layout.Dimensions {
							return layout.Inset{Left: 4, Right: 4, Top: 4, Bottom: 4}.Layout(g, func(g layout.Context) layout.Dimensions {
								return a.proofreadingText(g, a.textID("proofread.workbench.continue"), 12, proofreadingDesignBlue, true)
							})
						})
					})
				}),
			)
		})
	})
	return layout.Dimensions{Size: size}
}
func (a *App) proofreadingField(gtx layout.Context, editor *widget.Editor, label string) layout.Dimensions {
	return layout.Inset{Bottom: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(material.Label(a.theme, 12, a.textID(label)).Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(42))
					gtx.Constraints = layout.Exact(size)
					settingsFieldFocus(gtx, editor)
					paint.FillShape(gtx.Ops, contentSurface, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(6)).Op(gtx.Ops))
					proofreadRoundedBorder(gtx, size, 6, color.NRGBA{R: 220, G: 227, B: 232, A: 255}, max(1, gtx.Dp(1)))
					return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							e := material.Editor(a.theme, editor, "")
							e.TextSize = 12
							return e.Layout(gtx)
						}))
					})
				})
			}),
		)
	})
}

func (a *App) layoutProofreadingIdentity(g layout.Context) layout.Dimensions {
	w := a.ensureProofreadingWorkbench()
	gap := func(h unit.Dp) layout.FlexChild {
		return layout.Rigid(func(g layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(0, g.Dp(h))} })
	}
	children := []layout.FlexChild{
		layout.Rigid(func(g layout.Context) layout.Dimensions {
			b := w.button("back-list")
			if b.Clicked(g) && !w.busy {
				w.settings = false
				w.historyOpen = false
				w.authMode = ""
				w.message = ""
				w.list.Position = layout.Position{}
				g.Execute(op.InvalidateCmd{})
			}
			g.Constraints.Min.Y = 0
			return b.Layout(g, func(g layout.Context) layout.Dimensions {
				return a.proofreadingText(g, a.textID("proofread.workbench.back_list"), 12, proofreadingDesignBlue, false)
			})
		}),
		gap(30),
		layout.Rigid(func(g layout.Context) layout.Dimensions {
			g.Constraints.Max.Y = g.Dp(40)
			return a.proofreadingText(g, a.textID("proofread.workbench.settings"), 27, proofreadInk, true)
		}),
		gap(18),
		layout.Rigid(func(g layout.Context) layout.Dimensions {
			return proofreadingCard(g, 178, func(g layout.Context) layout.Dimensions {
				width := unit.Dp(float32(g.Constraints.Max.X) / g.Metric.PxPerDp)
				half := (width - 104) / 2
				proofreadingAt(g, 27, 24, width-54, 24, func(g layout.Context) layout.Dimensions {
					return a.proofreadingText(g, a.textID("proofread.workbench.identity"), 16, proofreadInk, true)
				})
				proofreadingAt(g, 27, 52, width-54, 20, func(g layout.Context) layout.Dimensions {
					return a.proofreadingText(g, a.textID("proofread.workbench.identity_help"), 12, proofreadMuted, false)
				})
				proofreadingAt(g, 27, 86, half, 70, func(g layout.Context) layout.Dimensions {
					return a.proofreadingField(g, &w.name, "proofread.workbench.name")
				})
				proofreadingAt(g, width/2+25, 86, half, 70, func(g layout.Context) layout.Dimensions {
					return a.proofreadingField(g, &w.email, "proofread.workbench.email")
				})
				return layout.Dimensions{}
			})
		}),
		gap(26),
		layout.Rigid(func(g layout.Context) layout.Dimensions {
			g.Constraints.Max.X = g.Dp(172)
			g.Constraints.Max.Y = g.Dp(42)
			return a.proofreadingAction(g, "save-identity", "proofread.workbench.save_identity", a.saveProofreadingIdentity)
		}),
		gap(48),
		layout.Rigid(func(g layout.Context) layout.Dimensions {
			return proofreadingCard(g, 269, func(g layout.Context) layout.Dimensions {
				width := unit.Dp(float32(g.Constraints.Max.X) / g.Metric.PxPerDp)
				proofreadingAt(g, 27, 24, width-54, 24, func(g layout.Context) layout.Dimensions {
					return a.proofreadingText(g, a.textID("proofread.remote.auth"), 16, proofreadInk, true)
				})
				proofreadingAt(g, 27, 53, width-54, 23, func(g layout.Context) layout.Dimensions {
					return a.proofreadingText(g, a.textID("proofread.remote.auth_help"), 12, proofreadMuted, false)
				})
				for _, y := range []unit.Dp{88, 154} {
					proofreadingAt(g, 27, y, width-54, 1, func(g layout.Context) layout.Dimensions { return fill(g, color.NRGBA{R: 237, G: 240, B: 242, A: 255}) })
				}
				proofreadingAt(g, 28, 109, 19, 19, func(g layout.Context) layout.Dimensions {
					return layoutProofreadingIcon(g, "home", "key-round", 19, proofreadingDesignBlue)
				})
				proofreadingAt(g, 62, 104, width-250, 24, func(g layout.Context) layout.Dimensions {
					return a.proofreadingText(g, a.textID("proofread.remote.ssh_title"), 13, proofreadInk, true)
				})
				proofreadingAt(g, 62, 129, width-250, 20, func(g layout.Context) layout.Dimensions {
					return a.proofreadingText(g, a.textID("proofread.remote.ssh_help"), 11, proofreadMuted, false)
				})
				proofreadingAt(g, width-161, 105, 134, 34, func(g layout.Context) layout.Dimensions {
					return a.proofreadingOutlinedAction(g, "add-ssh", "proofread.remote.add_key", func() { w.authMode = "ssh" })
				})
				proofreadingAt(g, 28, 174, 19, 19, func(g layout.Context) layout.Dimensions {
					return layoutProofreadingIcon(g, "home", "shield-check", 19, proofreadingDesignBlue)
				})
				proofreadingAt(g, 62, 169, width-250, 24, func(g layout.Context) layout.Dimensions {
					return a.proofreadingText(g, a.textID("proofread.remote.token_title"), 13, proofreadInk, true)
				})
				proofreadingAt(g, 62, 194, width-250, 20, func(g layout.Context) layout.Dimensions {
					return a.proofreadingText(g, a.textID("proofread.remote.token_help"), 11, proofreadMuted, false)
				})
				proofreadingAt(g, width-161, 170, 134, 34, func(g layout.Context) layout.Dimensions {
					return a.proofreadingOutlinedAction(g, "add-token", "proofread.remote.add_token", func() { w.authMode = "https" })
				})
				return layout.Dimensions{}
			})
		}),
	}
	if w.authMode == "ssh" {
		children = append(children, gap(24), layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.proofreadingField(g, &w.ssh, "proofread.remote.ssh")
		}), layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.proofreadingAction(g, "save-ssh", "proofread.remote.save_ssh", a.saveProofreadingSSH)
		}), gap(12), layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.proofreadingAction(g, "generate-ssh", "proofread.remote.generate_ssh", a.generateProofreadingSSH)
		}))
	}
	if w.authMode == "https" {
		children = append(children, gap(24), layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.proofreadingField(g, &w.authUser, "proofread.remote.username")
		}), layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.proofreadingField(g, &w.token, "proofread.remote.token")
		}), layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.proofreadingAction(g, "save-token", "proofread.remote.save_token", a.saveProofreadingToken)
		}))
	}
	if w.historyProject.ID != "" {
		children = append(children, gap(24), layout.Rigid(material.H6(a.theme, w.historyProject.Title).Layout), layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.proofreadingField(g, &w.remote, "proofread.remote.url")
		}), layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.proofreadingAction(g, "save-remote", "proofread.remote.save", a.saveProofreadingRemote)
		}), gap(12), layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.proofreadingAction(g, "scan-host", "proofread.remote.scan_host", a.scanProofreadingHost)
		}), gap(12), layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.proofreadingAction(g, "trust-host", "proofread.remote.trust_host", a.trustProofreadingHost)
		}), gap(12), layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.proofreadingAction(g, "push", "proofread.remote.push", a.pushProofreadingRemote)
		}))
	}
	if w.cancel != nil {
		children = append(children, layout.Rigid(func(g layout.Context) layout.Dimensions {
			b := w.button("cancel-job")
			if b.Clicked(g) {
				w.cancel()
			}
			return material.Button(a.theme, b, a.textID("proofread.remote.cancel")).Layout(g)
		}))
	}
	if w.message != "" {
		children = append(children, gap(16), layout.Rigid(material.Body2(a.theme, w.message).Layout))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(g, children...)
}
