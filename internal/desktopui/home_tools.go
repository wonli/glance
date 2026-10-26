package desktopui

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/i18n"
)

const (
	homeToolFilterAll        = "all"
	homeToolFilterDocuments  = "documents-pdf"
	homeToolFilterImages     = "images-recognition"
	homeToolFilterManagement = "file-management"
)

type homeToolFilter struct {
	id      string
	message i18n.Message
}

var homeToolFilters = []homeToolFilter{
	{id: homeToolFilterAll, message: i18n.Message{ID: "home.tools.filter.all"}},
	{id: homeToolFilterDocuments, message: i18n.Message{ID: "home.tools.filter.documents"}},
	{id: homeToolFilterImages, message: i18n.Message{ID: "home.tools.filter.images"}},
	{id: homeToolFilterManagement, message: i18n.Message{ID: "home.tools.filter.management"}},
}

type homeToolEntry struct {
	id          string
	category    string
	title       i18n.Message
	description i18n.Message
}

var homeToolCatalog = []homeToolEntry{
	{
		id:          "workbench",
		category:    homeToolFilterDocuments,
		title:       i18n.Message{ID: "home.tools.workbench.title"},
		description: i18n.Message{ID: "home.tools.workbench.description"},
	},
}

var (
	homeToolsAccent    = color.NRGBA{R: 48, G: 119, B: 231, A: 255}
	homeToolsIconFill  = color.NRGBA{R: 235, G: 243, B: 255, A: 255}
	homeToolsTextTitle = color.NRGBA{R: 35, G: 40, B: 48, A: 255}
)

func (a *App) layoutHomeToolsPage(gtx layout.Context) layout.Dimensions {
	a.processHomeToolsFilterClicks(gtx)
	children := []layout.FlexChild{
		layout.Rigid(a.layoutHomeToolsHeader),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutHomeToolsToolbar(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(32)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				entries := a.filteredHomeToolEntries()
				if len(entries) == 0 {
					return homeTextLabel(gtx, a.theme, a.textID("home.tools.empty"), 14, homeMuted, false)
				}
				return layoutHomeToolsEntries(gtx, a, entries)
			})
		}),
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (a *App) layoutHomeToolsHeader(gtx layout.Context) layout.Dimensions {
	searchWidth := float32(300)
	if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(760)) {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.layoutHomeToolsHeading),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return homeSearchField(gtx, a.theme, &a.homeToolsSearch, a.textID("home.tools.search.placeholder"), 0, 44)
				})
			}),
		)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, a.layoutHomeToolsHeading),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(32)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeSearchField(gtx, a.theme, &a.homeToolsSearch, a.textID("home.tools.search.placeholder"), searchWidth, 44)
			})
		}),
	)
}

func (a *App) layoutHomeToolsHeading(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, a.textID("home.tools.title"), 31, homeToolsTextTitle, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Body2(a.theme, a.textID("home.tools.subtitle"))
				style.TextSize = 15
				style.Color = homeMuted
				style.MaxLines = 2
				return style.Layout(gtx)
			})
		}),
	)
}

func (a *App) layoutHomeToolsToolbar(gtx layout.Context) layout.Dimensions {
	width := gtx.Constraints.Max.X
	if width >= gtx.Dp(unit.Dp(560)) {
		height := gtx.Dp(unit.Dp(44))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutHomeToolsFilterRow(gtx, homeToolFilterAll, homeToolFilterDocuments, homeToolFilterImages, homeToolFilterManagement)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
		)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutHomeToolsFilterRow(gtx, homeToolFilterAll, homeToolFilterDocuments)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutHomeToolsFilterRow(gtx, homeToolFilterImages, homeToolFilterManagement)
			})
		}),
	)
}

func (a *App) layoutHomeToolsFilterRow(gtx layout.Context, filterIDs ...string) layout.Dimensions {
	if a.homeToolsFilterButtons == nil {
		a.homeToolsFilterButtons = make(map[string]*widget.Clickable)
	}
	children := make([]layout.FlexChild, 0, len(filterIDs))
	locale := a.localeSnapshot().Resolved
	for index, filterID := range filterIDs {
		filterID := filterID
		button := a.homeToolsFilterButtons[filterID]
		if button == nil {
			button = new(widget.Clickable)
			a.homeToolsFilterButtons[filterID] = button
		}
		gap := unit.Dp(4)
		if index == len(filterIDs)-1 {
			gap = 0
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: gap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeToolsFilterChip(gtx, a.theme, button, filterID, a.text(homeToolFilterMessage(filterID)), a.homeToolsCategory == filterID, locale)
			})
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (a *App) processHomeToolsFilterClicks(gtx layout.Context) {
	if a.homeToolsFilterButtons == nil {
		a.homeToolsFilterButtons = make(map[string]*widget.Clickable)
	}
	for _, filter := range homeToolFilters {
		button := a.homeToolsFilterButtons[filter.id]
		if button == nil {
			button = new(widget.Clickable)
			a.homeToolsFilterButtons[filter.id] = button
		}
		if button.Clicked(gtx) {
			a.homeToolsCategory = filter.id
		}
	}
}

func homeToolsFilterChip(gtx layout.Context, theme *material.Theme, button *widget.Clickable, filterID, label string, selected bool, locale i18n.Locale) layout.Dimensions {
	width := gtx.Dp(homeToolsFilterWidth(filterID, locale))
	height := gtx.Dp(unit.Dp(38))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bg := color.NRGBA{A: 0}
				if selected {
					bg = homeSelectedFill
				} else if button.Hovered() {
					bg = homeHoverFill
				}
				shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(8)))
				paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				style := material.Body2(theme, label)
				style.TextSize = defaultShellTokens.Typography.Nav
				style.Color = homeMuted
				style.Font.Weight = font.Normal
				if selected {
					style.Color = homeToolsTextTitle
					style.Font.Weight = font.SemiBold
				}
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: shellOpticalTextTopInset(opticalHomeFilterText)}.Layout(gtx, style.Layout)
				})
			},
		)
	})
}

func homeToolsFilterWidth(filterID string, locale i18n.Locale) unit.Dp {
	if locale == i18n.LocaleChinese {
		switch filterID {
		case homeToolFilterAll, homeToolFilterManagement:
			return 88
		case homeToolFilterDocuments, homeToolFilterImages:
			return 120
		}
	}
	switch filterID {
	case homeToolFilterAll:
		return 82
	case homeToolFilterDocuments:
		return 132
	case homeToolFilterImages:
		return 112
	case homeToolFilterManagement:
		return 128
	default:
		return 0
	}
}

func homeToolFilterMessage(filterID string) i18n.Message {
	for _, filter := range homeToolFilters {
		if filter.id == filterID {
			return filter.message
		}
	}
	return i18n.Message{ID: "common.translation_missing", Args: i18n.Args{"ID": "home.tools.filter." + filterID}}
}

func (a *App) filteredHomeToolEntries() []homeToolEntry {
	query := strings.ToLower(strings.TrimSpace(a.homeToolsSearch.Text()))
	filtered := make([]homeToolEntry, 0, len(homeToolCatalog))
	for _, entry := range homeToolCatalog {
		if a.homeToolsCategory != "" && a.homeToolsCategory != homeToolFilterAll && entry.category != a.homeToolsCategory {
			continue
		}
		if query != "" {
			title := strings.ToLower(a.text(entry.title))
			description := strings.ToLower(a.text(entry.description))
			if !strings.Contains(title, query) && !strings.Contains(description, query) {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func layoutHomeToolsEntries(gtx layout.Context, app *App, entries []homeToolEntry) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(entries))
	for _, entry := range entries {
		entry := entry
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			switch entry.id {
			case "workbench":
				return app.layoutHomeToolsWorkbenchCard(gtx)
			default:
				return layout.Dimensions{}
			}
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (a *App) layoutHomeToolsWorkbenchCard(gtx layout.Context) layout.Dimensions {
	width := min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(720)))
	if width <= 0 {
		return layout.Dimensions{}
	}
	size := image.Pt(width, gtx.Dp(unit.Dp(200)))
	gtx.Constraints = layout.Exact(size)
	button := a.homeNavButton("tools-workbench")
	if button.Clicked(gtx) {
		if err := a.ensureDesktopRuntime().openProofreadingModuleWindow(proofreadingWindowRequest{workbenchOnly: true}); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "home.tools.workbench.open_failed"}, err, notificationToast)
		}
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(14), func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(20), Bottom: unit.Dp(20), Left: unit.Dp(22), Right: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(layoutHomeToolsWorkbenchIcon),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return homeIcon(gtx, "chevron-right", unit.Dp(18), homeMuted)
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(13)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeTextLabel(gtx, a.theme, a.textID("home.tools.workbench.title"), 20, homeToolsTextTitle, true)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							style := material.Body2(a.theme, a.textID("home.tools.workbench.description"))
							style.TextSize = 14
							style.Color = homeMuted
							style.MaxLines = 2
							return style.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(15)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeTextLabel(gtx, a.theme, a.textID("home.tools.workbench.action"), 13, homeToolsAccent, true)
						})
					}),
				)
			})
		})
	})
}

func layoutHomeToolsWorkbenchIcon(gtx layout.Context) layout.Dimensions {
	size := gtx.Dp(unit.Dp(38))
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	shape := clip.UniformRRect(image.Rect(0, 0, size, size), gtx.Dp(unit.Dp(8)))
	paint.FillShape(gtx.Ops, homeToolsIconFill, shape.Op(gtx.Ops))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeIcon(gtx, "file-text", unit.Dp(21), homeToolsAccent)
	})
}
