package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/markdowndoc"
	"glance/internal/format/pdfdoc"
	"glance/internal/format/pptx"
	"glance/internal/format/xlsx"
	"glance/internal/i18n"
)

type homeSection uint8

const (
	homeSectionRecent homeSection = iota
	homeSectionFavorites
	homeSectionAIAssistant
	homeSectionSettings
)

type homeFileMetadata struct {
	modified    time.Time
	size        int64
	unavailable bool
}

type homeMetadataResult struct {
	generation     uint64
	entries        map[string]homeFileMetadata
	canonicalPaths map[string]string
}

type homeAction struct {
	id        string
	label     string
	labelRef  i18n.Message
	extension string
	icon      string
	iconColor color.NRGBA
	titleSize unit.Sp
	ai        bool
}

var homeActions = []homeAction{
	{id: "docx", label: "AI Docs", extension: ".docx", icon: "file-text", iconColor: color.NRGBA{R: 52, G: 120, B: 209, A: 255}, titleSize: 16, ai: true},
	{id: "xlsx", label: "AI Sheets", extension: ".xlsx", icon: "file-spreadsheet", iconColor: color.NRGBA{R: 79, G: 166, B: 107, A: 255}, titleSize: 16, ai: true},
	{id: "pptx", label: "AI Slides", extension: ".pptx", icon: "presentation", iconColor: color.NRGBA{R: 226, G: 59, B: 39, A: 255}, titleSize: 16, ai: true},
	{id: "md", label: "AI Markdown", extension: ".md", icon: "file-code", iconColor: color.NRGBA{R: 133, G: 88, B: 232, A: 255}, titleSize: 13, ai: true},
	{id: "json", label: "JSON", extension: ".json", icon: "braces", iconColor: color.NRGBA{R: 214, G: 154, B: 45, A: 255}, titleSize: 16},
	{id: "pdf", label: "AI PDF", extension: ".pdf", icon: "file", iconColor: color.NRGBA{R: 240, G: 68, B: 69, A: 255}, titleSize: 16, ai: true},
	{id: "open", label: "打开本地文件", labelRef: i18n.Message{ID: "home.action.open_file"}, extension: ".docx / .xlsx", icon: "folder-open", iconColor: color.NRGBA{R: 123, G: 134, B: 146, A: 255}, titleSize: 13},
}

const (
	homeFilterAll           = "all"
	homeFilterDocuments     = "documents"
	homeFilterSpreadsheets  = "spreadsheets"
	homeFilterPresentations = "presentations"
	homeFilterPDF           = "pdf"
	homeFilterMarkdown      = "markdown"
	homeFilterJSON          = "json"
)

type homeFilter struct {
	id      string
	message i18n.Message
}

var homeFilters = []homeFilter{
	{id: homeFilterAll, message: i18n.Message{ID: "home.filter.all"}},
	{id: homeFilterDocuments, message: i18n.Message{ID: "home.filter.documents"}},
	{id: homeFilterSpreadsheets, message: i18n.Message{ID: "home.filter.spreadsheets"}},
	{id: homeFilterPresentations, message: i18n.Message{ID: "home.filter.presentations"}},
	{id: homeFilterPDF, message: i18n.Message{ID: "home.filter.pdf"}},
	{id: homeFilterMarkdown, message: i18n.Message{ID: "home.filter.markdown"}},
	{id: homeFilterJSON, message: i18n.Message{ID: "home.filter.json"}},
}

const (
	homeTextOpticalCenterTopInset unit.Dp = 6
)

var (
	homeCanvas         = color.NRGBA{R: 251, G: 251, B: 252, A: 255}
	homeSidebarFill    = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	homeSurface        = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	homeBorder         = color.NRGBA{R: 225, G: 229, B: 234, A: 255}
	homeSidebarBorder  = color.NRGBA{R: 231, G: 233, B: 236, A: 255}
	homeRowBorder      = color.NRGBA{R: 238, G: 240, B: 242, A: 255}
	homeText           = color.NRGBA{R: 52, G: 59, B: 69, A: 255}
	homeMuted          = color.NRGBA{R: 138, G: 146, B: 157, A: 255}
	settingsBlack      = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	homeSelectedFill   = color.NRGBA{R: 241, G: 242, B: 244, A: 255}
	homeHoverFill      = color.NRGBA{R: 247, G: 248, B: 249, A: 255}
	homeDropTargetFill = color.NRGBA{R: 229, G: 240, B: 253, A: 255}
	homeFavoriteGold   = color.NRGBA{R: 176, G: 138, B: 55, A: 255}
)

func (a *App) refreshHomeMetadata() {
	a.ensureDesktopRuntime()
	if a.collection == nil {
		return
	}
	recents := a.collection.Recents()
	a.homeMetadataGeneration++
	generation := a.homeMetadataGeneration
	a.launchManagedBackground(func() {
		entries := make(map[string]homeFileMetadata, len(recents))
		canonicalPaths := make(map[string]string, len(recents))
		for _, item := range recents {
			canonicalPaths[cleanHomeDocumentPath(item.Path)] = canonicalDocumentPath(item.Path)
			info, err := os.Stat(item.Path)
			if err != nil {
				entries[item.Path] = homeFileMetadata{unavailable: true}
				continue
			}
			entries[item.Path] = homeFileMetadata{modified: info.ModTime(), size: info.Size()}
		}
		select {
		case a.homeMetadataResults <- homeMetadataResult{generation: generation, entries: entries, canonicalPaths: canonicalPaths}:
		case <-a.shutdown:
		}
	})
}

func (a *App) layoutRecent(gtx layout.Context) layout.Dimensions {
	a.reconcileHomeFolderSelection()
	if a.homeSection == homeSectionSettings {
		a.processSettingsNavClicks(gtx)
	} else {
		a.processHomeNavClicks(gtx)
	}
	if a.homeSection == homeSectionSettings {
		a.dismissHomeRecentMenu()
		dims := a.layoutSettingsWorkspace(gtx)
		a.layoutHomeFolderMenuDismissInput(gtx)
		return dims
	}
	if a.homeSection == homeSectionAIAssistant {
		a.dismissHomeRecentMenu()
		dims := a.layoutAIAssistantWorkspace(gtx)
		a.layoutHomeFolderMenuDismissInput(gtx)
		return dims
	}
	dims := layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return fill(gtx, homeCanvas) },
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					width := gtx.Dp(unit.Dp(250))
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
					return a.layoutHomeSidebar(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					a.homeContentRootOrigin = image.Pt(gtx.Dp(unit.Dp(250)), 0)
					return a.layoutHomeContent(gtx)
				}),
			)
		},
	)
	a.layoutHomeDocumentDragOverlay(gtx)
	a.layoutHomeRecentMenuDismissInput(gtx)
	a.layoutHomeRecentFloatingMenu(gtx)
	a.layoutHomeFolderMenuDismissInput(gtx)
	return dims
}

func (a *App) layoutSettingsWorkspace(gtx layout.Context) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return fill(gtx, homeCanvas) },
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					width := gtx.Dp(unit.Dp(250))
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
					return a.layoutSettingsSidebar(gtx)
				}),
				layout.Flexed(1, a.layoutSettingsMain),
			)
		},
	)
}

func (a *App) layoutSettingsSidebar(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, homeSidebarFill, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, homeSidebarBorder, clip.Rect{Min: image.Pt(gtx.Constraints.Max.X-1, 0), Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(28), Left: unit.Dp(18), Right: unit.Dp(18), Bottom: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						height := gtx.Dp(unit.Dp(30))
						gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return homeIcon(gtx, "settings-2", 20, color.NRGBA{R: 63, G: 71, B: 82, A: 255})
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: unit.Dp(12), Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return homeTextLabel(gtx, a.theme, a.text(i18n.Message{ID: "settings.sidebar.title"}), 15, homeText, true)
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return a.homeNavButton("settings-back").Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									width, height := gtx.Dp(unit.Dp(30)), gtx.Dp(unit.Dp(30))
									gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
									return homeRoundedPanel(gtx, homeHoverFill, homeHoverFill, unit.Dp(5), func(gtx layout.Context) layout.Dimensions {
										return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return homeIcon(gtx, "arrow-left", 18, settingsBlack)
										})
									})
								})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(22), Bottom: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							height := gtx.Dp(unit.Dp(1))
							gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(gtx.Constraints.Max.X, height), image.Pt(gtx.Constraints.Max.X, height)
							return homeRoundedPanel(gtx, homeSidebarBorder, homeSidebarBorder, unit.Dp(0), func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{Size: gtx.Constraints.Max}
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutSettingsNavItem(gtx, "basic", a.text(i18n.Message{ID: "settings.nav.basic"}), "settings-2")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutSettingsNavItem(gtx, "providers", a.text(i18n.Message{ID: "settings.nav.providers"}), "bot")
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutSettingsNavItem(gtx, "mcp", a.text(i18n.Message{ID: "settings.nav.mcp"}), "plug")
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
				)
			})
		},
	)
}

func (a *App) layoutSettingsNavItem(gtx layout.Context, id, label, icon string) layout.Dimensions {
	button := a.settingsPageTabButton(id)
	selected := a.agentUI.settingsPageTab == id
	height := gtx.Dp(unit.Dp(44))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bg := color.NRGBA{A: 0}
				if selected {
					bg = homeSelectedFill
				} else if button.Hovered() {
					bg = homeHoverFill
				}
				shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(7)))
				paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							height := gtx.Dp(unit.Dp(44))
							gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeIcon(gtx, icon, 20, color.NRGBA{R: 63, G: 71, B: 82, A: 255})
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								height := gtx.Dp(unit.Dp(44))
								gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return homeTextLabel(gtx, a.theme, label, 15, homeText, selected)
									})
								})
							})
						}),
					)
				})
			},
		)
	})
}

func (a *App) layoutSettingsMain(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(76), Left: unit.Dp(60), Right: unit.Dp(48), Bottom: unit.Dp(36)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if a.agentUI.settingsPageTab == "mcp" {
			return a.layoutAgentSettingsPage(gtx)
		}
		return a.homeContentList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return a.layoutAgentSettingsPage(gtx)
		})
	})
}

func (a *App) layoutHomeSidebar(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, homeSidebarFill, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, homeSidebarBorder, clip.Rect{Min: image.Pt(gtx.Constraints.Max.X-1, 0), Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(28), Left: unit.Dp(18), Right: unit.Dp(18), Bottom: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				var precedingHeight int
				trackedRigid := func(child layout.Widget) layout.FlexChild {
					return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						dims := child(gtx)
						precedingHeight += dims.Size.Y
						return dims
					})
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					trackedRigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							height := gtx.Dp(unit.Dp(42))
							gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
							return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return homeTextLabel(gtx, a.theme, "Glance", 20, color.NRGBA{R: 18, G: 22, B: 28, A: 255}, true)
									}),
								)
							})
						})
					}),
					trackedRigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutHomeNavRow(gtx, "recent", "clock-3", a.text(i18n.Message{ID: "home.nav.recent"}), len(a.homeVisibleRecents(false)), a.homeSection == homeSectionRecent)
						})
					}),
					trackedRigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutHomeNavRow(gtx, "favorites", "star-outline", a.text(i18n.Message{ID: "home.nav.favorites"}), len(a.homeFavorites), a.homeSection == homeSectionFavorites)
						})
					}),
					trackedRigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutHomeNavRow(gtx, "ai-assistant", "sparkles", a.text(i18n.Message{ID: "home.nav.assistant"}), -1, a.homeSection == homeSectionAIAssistant)
						})
					}),
					trackedRigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutHomeNavRow(gtx, "agent-settings", "settings-2", a.text(i18n.Message{ID: "home.nav.settings"}), -1, a.homeSection == homeSectionSettings)
					}),
					trackedRigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(18), Bottom: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							paint.FillShape(gtx.Ops, homeSidebarBorder, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}.Op())
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}
						})
					}),
					trackedRigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							height := gtx.Dp(unit.Dp(30))
							gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
							return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.Y = 0
										return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return homeTextLabel(gtx, a.theme, a.text(i18n.Message{ID: "home.folder.title"}), 14, color.NRGBA{R: 125, G: 135, B: 146, A: 255}, true)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return homeSmallIconButton(gtx, &a.homeNewFolderButton, "folder-open", 17, homeMuted, a.startHomeFolderDirectoryPicker)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Left: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return homeSmallIconButton(gtx, &a.homeFolderAddButton, "plus", 17, homeMuted, func() { a.showCreateHomeFolderDialog(a.homeSelectedFolderID) })
										})
									}),
								)
							})
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						a.homeFolderTreeRootOrigin = image.Pt(gtx.Dp(unit.Dp(18)), gtx.Dp(unit.Dp(28))+precedingHeight)
						return a.layoutHomeFolderTree(gtx)
					}),
				)
			})
		},
	)
}

func (a *App) homeNavButton(id string) *widget.Clickable {
	if a.homeNavButtons == nil {
		a.homeNavButtons = make(map[string]*widget.Clickable)
	}
	button := a.homeNavButtons[id]
	if button == nil {
		button = new(widget.Clickable)
		a.homeNavButtons[id] = button
	}
	return button
}

func (a *App) processHomeNavClicks(gtx layout.Context) {
	for _, navigation := range []struct {
		id      string
		section homeSection
	}{
		{id: "recent", section: homeSectionRecent},
		{id: "favorites", section: homeSectionFavorites},
	} {
		if a.homeNavButton(navigation.id).Clicked(gtx) {
			a.selectHomeFolder("")
			a.homeSection = navigation.section
		}
	}
	if a.homeNavButton("ai-assistant").Clicked(gtx) {
		if a.homeSection == homeSectionAIAssistant {
			draft := a.agentUI.chatInput.Text()
			if len(a.agentUI.chatLines) > 0 {
				a.createAgentConversation()
				if draft != "" {
					a.agentUI.chatInput.SetText(draft)
				}
			}
		} else {
			a.resetHomeContentScroll()
			a.selectHomeFolder("")
			a.ensureGlobalAIAssistant()
			a.homeSection = homeSectionAIAssistant
		}
	}
	if a.homeNavButton("agent-settings").Clicked(gtx) {
		a.openAgentSettingsPage()
	}
}

func (a *App) resetHomeContentScroll() {
	a.homeContentList.Position.First = 0
	a.homeContentList.Position.Offset = 0
	a.homeContentList.Position.OffsetLast = 0
}

func (a *App) layoutHomeNavRow(gtx layout.Context, id, glyph, label string, count int, selected bool) layout.Dimensions {
	button := a.homeNavButton(id)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(42))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bg := color.NRGBA{A: 0}
				if selected {
					bg = homeSelectedFill
				} else if button.Hovered() {
					bg = homeHoverFill
				}
				shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(10)))
				paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					children := []layout.FlexChild{
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							iconColor := color.NRGBA{R: 63, G: 71, B: 82, A: 255}
							return homeIcon(gtx, glyph, 20, iconColor)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							return layout.Inset{Left: unit.Dp(12), Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, label, 15, homeText, selected)
							})
						}),
					}
					if count >= 0 {
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							countText := fmt.Sprintf("%d", count)
							gtx.Constraints.Min.Y = 0
							return layout.Inset{Left: unit.Dp(12), Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, countText, 13, homeMuted, false)
							})
						}))
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
				})
			},
		)
	})
}

func (a *App) layoutHomeContent(gtx layout.Context) layout.Dimensions {
	if a.homeRecentMenuPath != "" {
		a.homeRecentMenuAnchorBounds = image.Rectangle{}
	}
	if a.homeSection == homeSectionSettings {
		return layout.Inset{Top: unit.Dp(64), Left: unit.Dp(64), Right: unit.Dp(64), Bottom: unit.Dp(64)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return a.homeContentList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
				return a.layoutAgentSettingsPage(gtx)
			})
		})
	}
	if a.homeSection == homeSectionRecent && a.homeSelectedFolderID != "" {
		a.homeContentList.Axis = layout.Vertical
		return layout.Inset{Top: unit.Dp(68), Left: unit.Dp(50)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			a.homeContentViewportHeight = gtx.Constraints.Max.Y
			return layout.Stack{Alignment: layout.NE}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					return material.List(a.theme, &a.homeContentList).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
						a.homeContentRootOrigin = image.Pt(a.homeContentRootOrigin.X+gtx.Dp(unit.Dp(50)), gtx.Dp(unit.Dp(68))-a.homeContentList.Position.Offset)
						return layout.Inset{Right: unit.Dp(50)}.Layout(gtx, a.layoutHomeFolderContentPage)
					})
				}),
				layout.Stacked(a.layoutHomeContentScrollbar),
			)
		})
	}
	return layout.Inset{Top: unit.Dp(68), Left: unit.Dp(50), Right: unit.Dp(50)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return a.homeContentList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			a.homeContentRootOrigin = image.Pt(a.homeContentRootOrigin.X+gtx.Dp(unit.Dp(50)), gtx.Dp(unit.Dp(68))-a.homeContentList.Position.Offset)
			return a.layoutHomeContentPage(gtx)
		})
	})
}

func homeContentScrollbarRange(position layout.Position, viewportHeight int) (float32, float32, bool) {
	if position.Length <= viewportHeight || position.Length <= 0 || viewportHeight <= 0 {
		return 0, 1, false
	}
	offset := max(0, position.Offset)
	start := float32(offset) / float32(position.Length)
	end := float32(offset+viewportHeight) / float32(position.Length)
	if start > 1 {
		start = 1
	}
	if end > 1 {
		end = 1
	}
	if end < start {
		end = start
	}
	return start, end, end-start < 1
}

func (a *App) layoutHomeContentScrollbar(gtx layout.Context) layout.Dimensions {
	start, end, scrollable := homeContentScrollbarRange(a.homeContentList.Position, a.homeContentViewportHeight)
	if !scrollable {
		return layout.Dimensions{}
	}
	style := material.Scrollbar(a.theme, &a.homeContentList.Scrollbar)
	dimensions := style.Layout(gtx, layout.Vertical, start, end)
	if delta := a.homeContentList.ScrollDistance(); delta != 0 {
		a.homeContentList.List.ScrollBy(delta)
	}
	return dimensions
}

func (a *App) layoutHomeContentPage(gtx layout.Context) layout.Dimensions {
	if a.homeSection == homeSectionRecent && a.homeSelectedFolderID != "" {
		return a.layoutHomeFolderContentPage(gtx)
	}
	var precedingHeight int
	trackedRigid := func(child layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := child(gtx)
			precedingHeight += dims.Size.Y
			return dims
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		trackedRigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutHomeGreeting(gtx)
			})
		}),
		trackedRigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(28)}.Layout(gtx, a.layoutHomeActions)
		}),
		trackedRigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(28)}.Layout(gtx, a.layoutHomeFilters)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			tableTopInset := gtx.Dp(unit.Dp(12))
			a.homeRecentTableRootOrigin = a.homeContentRootOrigin.Add(image.Pt(0, precedingHeight+tableTopInset))
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := a.layoutHomeRecentTable(gtx)
				precedingHeight += dims.Size.Y + tableTopInset
				return dims
			})
		}),
	)
}

func (a *App) layoutHomeGreeting(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(78))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	gtx.Execute(op.InvalidateCmd{At: nextHomeGreetingChange(gtx.Now)})
	style := material.H4(a.theme, a.text(homeGreetingMessageForTime(gtx.Now))+"\n"+a.text(i18n.Message{ID: "home.greeting.prompt"}))
	style.TextSize = a.tokens().Typography.Display
	style.LineHeightScale = 1.15
	style.MaxLines = 2
	style.Color = color.NRGBA{R: 35, G: 40, B: 48, A: 255}
	style.Font.Weight = font.Bold
	return style.Layout(gtx)
}

func nextHomeGreetingChange(now time.Time) time.Time {
	for _, hour := range [...]int{5, 12, 14, 18} {
		change := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
		if change.After(now) {
			return change
		}
	}
	tomorrow := now.AddDate(0, 0, 1)
	return time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, now.Location())
}

func homeGreetingMessageForTime(now time.Time) i18n.Message {
	switch hour := now.Hour(); {
	case hour < 5:
		return i18n.Message{ID: "home.greeting.early"}
	case hour < 12:
		return i18n.Message{ID: "home.greeting.morning"}
	case hour < 14:
		return i18n.Message{ID: "home.greeting.noon"}
	case hour < 18:
		return i18n.Message{ID: "home.greeting.afternoon"}
	default:
		return i18n.Message{ID: "home.greeting.evening"}
	}
}

func (a *App) layoutHomeActions(gtx layout.Context) layout.Dimensions {
	const actionGap = unit.Dp(14)
	const narrowCardMinWidth = unit.Dp(120)
	width := gtx.Constraints.Max.X
	columns := (width + gtx.Dp(actionGap)) / (gtx.Dp(narrowCardMinWidth) + gtx.Dp(actionGap))
	if columns < 1 {
		columns = 1
	}
	if columns > len(homeActions) {
		columns = len(homeActions)
	}
	rows := (len(homeActions) + columns - 1) / columns
	height := gtx.Dp(unit.Dp(132))*rows + gtx.Dp(actionGap)*(rows-1)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	rowChildren := make([]layout.FlexChild, 0, rows)
	for row := 0; row < rows; row++ {
		start, end := homeActionRowRange(row, columns, len(homeActions))
		bottom := unit.Dp(0)
		if row < rows-1 {
			bottom = actionGap
		}
		rowChildren = append(rowChildren, a.homeActionRow(start, end, columns, bottom))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rowChildren...)
}

func homeActionRowRange(row, columns, total int) (start, end int) {
	start = row * columns
	end = start + columns
	if end > total {
		end = total
	}
	return start, end
}

func (a *App) homeActionRow(start, end, columns int, bottom unit.Dp) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: bottom}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gap := gtx.Dp(unit.Dp(14))
			cardWidth := (gtx.Constraints.Max.X - gap*(columns-1)) / columns
			children := make([]layout.FlexChild, 0, end-start+1)
			for index := start; index < end; index++ {
				item := homeActions[index]
				lastInRow := index == end-1
				outerWidth := cardWidth
				if !lastInRow {
					outerWidth += gap
				}
				children = append(children, a.homeActionCardChild(item, cardWidth, outerWidth, !lastInRow))
			}
			if end-start < columns {
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{}
				}))
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
		})
	})
}

func (a *App) homeActionCardChild(item homeAction, cardWidth, outerWidth int, withGap bool) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = outerWidth, outerWidth
		right := unit.Dp(0)
		if withGap {
			right = unit.Dp(14)
		}
		return layout.Inset{Right: right}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = cardWidth, cardWidth
			return a.layoutHomeActionCard(gtx, item)
		})
	})
}

func (a *App) layoutHomeActionCard(gtx layout.Context, item homeAction) layout.Dimensions {
	item.label = a.messageText(item.labelRef, item.label)
	button := a.homeActionButtons[item.id]
	if button == nil {
		button = new(widget.Clickable)
		a.homeActionButtons[item.id] = button
	}
	if button.Clicked(gtx) {
		a.handleHomeAction(item.id)
	}
	return homeActionCard(gtx, a.theme, button, item)
}

func (a *App) handleHomeAction(id string) {
	switch id {
	case "docx":
		word, err := docx.NewBlank()
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "home.create.error.failed"}, err, notificationToast)
			return
		}
		_ = a.installDocument(loadedDocument{word: word})
	case "json":
		_ = a.installDocument(loadedDocument{jsonDoc: jsondoc.NewBlank()})
	case "xlsx":
		book := xlsx.NewBlank()
		_ = a.installDocument(loadedDocument{book: book, sheets: book.SheetNames()})
	case "open":
		a.startOpenFilePicker()
	case "pptx":
		doc, err := pptx.NewBlank()
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "home.create.error.failed"}, err, notificationToast)
			return
		}
		_ = a.installDocument(loadedDocument{pptxDoc: doc})
	case "md":
		_ = a.installDocument(loadedDocument{markdownDoc: markdowndoc.NewBlank()})
	case "html":
		a.setStatusMessage(i18n.Message{ID: "home.create.error.not_available"}, feedbackWarning, notificationToast)
	case "pdf":
		doc, err := pdfdoc.NewBlank()
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "home.create.error.failed"}, err, notificationToast)
			return
		}
		_ = a.installDocument(loadedDocument{pdfDoc: doc})
	}
}
