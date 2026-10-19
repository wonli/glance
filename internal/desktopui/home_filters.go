package desktopui

import (
	"path/filepath"

	"gioui.org/layout"
	"gioui.org/unit"

	"glance/internal/i18n"
)

func (a *App) layoutHomeFilters(gtx layout.Context) layout.Dimensions {
	width := gtx.Dp(unit.Dp(1090))
	if width > gtx.Constraints.Max.X {
		width = gtx.Constraints.Max.X
	}
	if width < gtx.Dp(unit.Dp(760)) {
		return a.layoutHomeFiltersCompact(gtx, width)
	}
	scale := float32(width) / float32(gtx.Dp(unit.Dp(1090)))
	if scale <= 0 {
		scale = 1
	}
	height := gtx.Dp(unit.Dp(44 * scale))
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	countText := a.homeRecentCountText()
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeFilterGroup(gtx, a, a.homeFilterButtons, &a.homeTypeFilter, scale, func(filter string) {
				a.homeTypeFilter = filter
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return layout.Inset{Left: unit.Dp(18 * scale)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeSearchField(gtx, a.theme, &a.homeSearch, a.homeSearchPlaceholder(), 240*scale, 44*scale)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.homeSortButton.Clicked(gtx) {
				a.homeSortOpenedDesc = !a.homeSortOpenedDesc
			}
			return layout.Inset{Left: unit.Dp(18 * scale)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeSortButton(gtx, &a.homeSortButton, 20*scale, 44*scale)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return layout.Inset{Left: unit.Dp(18 * scale), Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeRecentCountStyle(a.theme, countText, scale).Layout(gtx)
			})
		}),
	)
}

func (a *App) layoutHomeFiltersCompact(gtx layout.Context, width int) layout.Dimensions {
	scale := float32(width) / float32(gtx.Dp(unit.Dp(650)))
	if scale > 1 {
		scale = 1
	}
	if scale <= 0 {
		scale = 1
	}
	rowHeightDp := 44 * scale
	rowHeight := gtx.Dp(unit.Dp(rowHeightDp))
	rowGap := gtx.Dp(unit.Dp(8))
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = rowHeight*2+rowGap, rowHeight*2+rowGap
	countText := a.homeRecentCountText()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeFilterGroup(gtx, a, a.homeFilterButtons, &a.homeTypeFilter, scale, func(filter string) {
						a.homeTypeFilter = filter
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return homeSearchField(gtx, a.theme, &a.homeSearch, a.homeSearchPlaceholder(), 0, rowHeightDp)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(10 * scale)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if a.homeSortButton.Clicked(gtx) {
								a.homeSortOpenedDesc = !a.homeSortOpenedDesc
							}
							return homeSortButton(gtx, &a.homeSortButton, 20*scale, rowHeightDp)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						return layout.Inset{Left: unit.Dp(10 * scale), Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeRecentCountStyle(a.theme, countText, scale).Layout(gtx)
						})
					}),
				)
			})
		}),
	)
}

func (a *App) homeRecentCountText() string {
	label := a.text(i18n.Message{ID: "home.nav.recent"})
	if a.homeSection == homeSectionRecent && a.homeSelectedFolderID != "" {
		label = filepath.Base(a.homeFolderDisplayPath(a.homeSelectedFolderID))
	} else if a.homeSection == homeSectionFavorites {
		label = a.text(i18n.Message{ID: "home.nav.favorites"})
	}
	return a.text(i18n.Message{ID: "home.recent.count", Args: i18n.Args{
		"Label": label,
		"Count": len(a.homeDisplayedRecents()),
	}})
}
