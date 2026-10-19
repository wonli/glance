package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/i18n"
	"glance/internal/workspace"
	"glance/ui"
)

func (a *App) layoutHomeRecentTable(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(330))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	items := a.homeDisplayedRecents()
	a.homeRecentListRootOrigin = a.homeRecentTableRootOrigin.Add(image.Pt(0, gtx.Dp(unit.Dp(48))))
	return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(14), func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutHomeTableHeader(gtx) }),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(items) == 0 {
					message := i18n.Message{ID: "home.recent.empty"}
					if a.homeSection == homeSectionRecent && a.homeSelectedFolderID != "" {
						message = i18n.Message{ID: "home.recent.empty.folder"}
						if strings.TrimSpace(a.homeSearch.Text()) != "" || a.homeTypeFilter != homeFilterAll {
							message = i18n.Message{ID: "home.recent.empty.search"}
						}
					} else if a.homeSection == homeSectionFavorites {
						message = i18n.Message{ID: "home.recent.empty.favorites"}
					} else if strings.TrimSpace(a.homeSearch.Text()) != "" || a.homeTypeFilter != homeFilterAll {
						message = i18n.Message{ID: "home.recent.empty.search"}
					}
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, a.text(message), 13, homeMuted, false)
					})
				}
				return material.List(a.theme, &a.recentList).Layout(gtx, len(items), func(gtx layout.Context, index int) layout.Dimensions {
					return a.layoutHomeRecentRow(gtx, items[index], index)
				})
			}),
		)
	})
}

func (a *App) layoutHomeTableHeader(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(48))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, homeSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, homeRowBorder, clip.Rect{Min: image.Pt(0, gtx.Constraints.Max.Y-1), Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(22), Right: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeTableColumns(gtx, a, a.theme, true, nil, a.homeSortLabel())
			})
		},
	)
}

func (a *App) layoutHomeRecentRow(gtx layout.Context, item workspace.Recent, index int) layout.Dimensions {
	rowOrigin := a.homeRecentRowOrigin(index, gtx)
	a.updateHomeDocumentDrag(gtx, item.Path, rowOrigin)
	button := a.homeOpenButtons[item.Path]
	if button == nil {
		button = new(widget.Clickable)
		a.homeOpenButtons[item.Path] = button
	}
	if button.Clicked(gtx) {
		if meta, ok := a.homeMetadata[item.Path]; !ok || !meta.unavailable {
			a.startOpen(item.Path)
		}
	}
	star := a.homeFavoriteButtons[item.Path]
	if star == nil {
		star = new(widget.Clickable)
		a.homeFavoriteButtons[item.Path] = star
	}
	if star.Clicked(gtx) {
		a.toggleHomeFavorite(item.Path)
	}
	more := a.homeMoreButtons[item.Path]
	if more == nil {
		more = new(widget.Clickable)
		a.homeMoreButtons[item.Path] = more
	}
	if more.Clicked(gtx) {
		a.toggleHomeRecentMenu(item.Path, gtx)
	}
	if a.homeRecentMenuPath == item.Path {
		a.homeRecentMenuAnchorBounds = homeRecentMoreButtonBounds(rowOrigin, gtx.Constraints.Max.X, gtx)
	}

	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(56)), gtx.Dp(unit.Dp(56))
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bg := homeSurface
			if button.Hovered() {
				bg = homeHoverFill
			}
			paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, homeRowBorder, clip.Rect{Min: image.Pt(0, gtx.Constraints.Max.Y-1), Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(22), Right: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeTableColumns(gtx, a, a.theme, false, &homeRowData{app: a, item: item, open: button, favorite: star, more: more}, "")
			})
		},
	)
}

type homeRowData struct {
	app      *App
	item     workspace.Recent
	open     *widget.Clickable
	favorite *widget.Clickable
	more     *widget.Clickable
}

type homeTableColumnMetrics struct {
	nameWidth, locationWidth, modifiedWidth, sizeWidth, actionWidth int
	scale                                                           float32
	compact                                                         bool
}

func homeTableColumnMetricsForWidth(gtx layout.Context, contentWidth int) homeTableColumnMetrics {
	compact := contentWidth < gtx.Dp(unit.Dp(720))
	baseWidth := gtx.Dp(unit.Dp(812))
	if compact {
		baseWidth = gtx.Dp(unit.Dp(702))
	}
	scale := float32(contentWidth) / float32(baseWidth)
	if scale > 1 {
		scale = 1
	}
	column := func(width unit.Dp) int { return gtx.Dp(unit.Dp(float32(width) * scale)) }
	metrics := homeTableColumnMetrics{
		nameWidth:     column(500),
		locationWidth: column(110),
		modifiedWidth: column(90),
		sizeWidth:     column(60),
		actionWidth:   column(52),
		scale:         scale,
		compact:       compact,
	}
	if compact {
		metrics.locationWidth = 0
	}
	usedWidth := metrics.nameWidth + metrics.locationWidth + metrics.modifiedWidth + metrics.sizeWidth + metrics.actionWidth
	if overflow := usedWidth - contentWidth; overflow > 0 {
		metrics.nameWidth = max(0, metrics.nameWidth-overflow)
	}
	return metrics
}

func homeTableColumns(gtx layout.Context, app *App, theme *material.Theme, header bool, row *homeRowData, sortLabel string) layout.Dimensions {
	metrics := homeTableColumnMetricsForWidth(gtx, gtx.Constraints.Max.X)
	nameWidth, locationWidth, modifiedWidth, sizeWidth, actionWidth := metrics.nameWidth, metrics.locationWidth, metrics.modifiedWidth, metrics.sizeWidth, metrics.actionWidth
	scale := metrics.scale
	textStyle := func(text string, strong bool) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			fg := color.NRGBA{R: 107, G: 116, B: 128, A: 255}
			weight := font.Normal
			if header || strong {
				weight = font.SemiBold
			}
			if !header {
				fg = color.NRGBA{R: 146, G: 154, B: 164, A: 255}
			}
			if strong {
				fg = color.NRGBA{R: 45, G: 52, B: 61, A: 255}
			}
			label := homeLabelStyle(theme, text, unit.Sp(14*scale), fg, weight)
			return (ui.OpticalPlacement{TopInset: shellOpticalTextTopInset(opticalHomeFileListText)}).Layout(gtx, label.Layout)
		}
	}
	dataColumns := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = nameWidth, nameWidth
			if header {
				return textStyle(homeTableColumnLabel(app, "name"), false)(gtx)
			}
			meta := row.app.homeMetadata[row.item.Path]
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeFileBadge(gtx, extensionForPath(row.item.Path), homeFileColor(row.item.Path), 34)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(14), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return (ui.OpticalPlacement{TopInset: shellOpticalTextTopInset(opticalHomeFileListText)}).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							fg := color.NRGBA{R: 45, G: 52, B: 61, A: 255}
							if meta.unavailable {
								fg = homeMuted
							}
							return homeTextLabel(gtx, row.app.theme, filepath.Base(row.item.Path), unit.Sp(15*scale), fg, true)
						})
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = locationWidth, locationWidth
			if header {
				return textStyle(homeTableColumnLabel(app, "location"), false)(gtx)
			}
			return textStyle(filepath.Base(filepath.Dir(row.item.Path)), false)(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = modifiedWidth, modifiedWidth
			if header {
				return textStyle(sortLabel, false)(gtx)
			}
			opened := row.item.OpenedAt
			if opened.IsZero() && row.app != nil && row.app.homeMetadata != nil {
				opened = row.app.homeMetadata[row.item.Path].modified
			}
			if opened.IsZero() {
				return textStyle("—", false)(gtx)
			}
			return textStyle(row.app.formatHomeModified(opened), false)(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = sizeWidth, sizeWidth
			if header {
				return textStyle(homeTableColumnLabel(app, "size"), false)(gtx)
			}
			meta := row.app.homeMetadata[row.item.Path]
			if meta.unavailable {
				return textStyle("—", false)(gtx)
			}
			return textStyle(formatHomeFileSize(meta.size), false)(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
	}
	layoutDataColumns := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, dataColumns...)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if !header && row != nil && row.open != nil {
				return row.open.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					state := row.app.homeDocumentDragState(row.item.Path)
					area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
					state.drag.Add(gtx.Ops)
					area.Pop()
					return layoutDataColumns(gtx)
				})
			}
			return layoutDataColumns(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = actionWidth, actionWidth
			if header {
				return layout.Dimensions{Size: image.Pt(actionWidth, 0)}
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: unit.Dp(16 * scale)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeIconButton(gtx, row.favorite, "star", unit.Dp(18*scale), color.NRGBA{R: 136, G: 145, B: 156, A: 255}, row.app.homeFavorites[row.item.Path])
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeIconButton(gtx, row.more, "ellipsis", unit.Dp(18*scale), color.NRGBA{R: 136, G: 145, B: 156, A: 255}, false)
				}),
			)
		}),
	)
}

func (a *App) homeVisibleRecents(applyFilter bool) []workspace.Recent {
	var recents []workspace.Recent
	if a != nil && a.desktopRuntime != nil && a.collection != nil {
		recents = a.collection.Recents()
	}
	filtered := a.filterHomeRecents(recents, applyFilter, nil)
	if len(filtered) > 5 {
		filtered = filtered[:5]
	}
	return filtered
}

func (a *App) homeDisplayedRecents() []workspace.Recent {
	if a != nil && a.homeSection == homeSectionRecent && a.homeSelectedFolderID != "" {
		return a.homeFolderVisibleRecents(a.homeSelectedFolderID)
	}
	return a.homeVisibleRecents(true)
}

func (a *App) filterHomeRecents(recents []workspace.Recent, applyFilter bool, include func(workspace.Recent) bool) []workspace.Recent {
	filtered := make([]workspace.Recent, 0, len(recents))
	query := ""
	if a != nil {
		query = strings.ToLower(strings.TrimSpace(a.homeSearch.Text()))
	}
	for _, item := range recents {
		if include != nil && !include(item) {
			continue
		}
		if !applyFilter {
			filtered = append(filtered, item)
			continue
		}
		if a != nil && a.homeSection == homeSectionFavorites && !a.homeFavorites[item.Path] {
			continue
		}
		if a != nil && a.homeTypeFilter != homeFilterAll && !homeTypeMatches(item.Path, a.homeTypeFilter) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(filepath.Base(item.Path)), query) && !strings.Contains(strings.ToLower(filepath.Dir(item.Path)), query) {
			continue
		}
		filtered = append(filtered, item)
	}
	if applyFilter {
		sort.SliceStable(filtered, func(i, j int) bool {
			left := filtered[i].OpenedAt
			if left.IsZero() && a != nil && a.homeMetadata != nil {
				left = a.homeMetadata[filtered[i].Path].modified
			}
			right := filtered[j].OpenedAt
			if right.IsZero() && a != nil && a.homeMetadata != nil {
				right = a.homeMetadata[filtered[j].Path].modified
			}
			if left.Equal(right) {
				return filtered[i].Path < filtered[j].Path
			}
			if a != nil && a.homeSortOpenedDesc {
				return left.After(right)
			}
			return left.Before(right)
		})
	}
	return filtered
}

func (a *App) homeSortLabel() string {
	if a != nil && a.homeSortOpenedDesc {
		return a.text(i18n.Message{ID: "home.recent.sort.opened_descending"})
	}
	return a.text(i18n.Message{ID: "home.recent.sort.opened_ascending"})
}

func homeTableColumnLabel(app *App, column string) string {
	ids := map[string]string{
		"name":     "home.table.name",
		"location": "home.table.location",
		"size":     "home.table.size",
	}
	id, ok := ids[column]
	if !ok {
		id = "common.translation_missing"
	}
	if app == nil {
		return id
	}
	message := i18n.Message{ID: id}
	if !ok {
		message.Args = i18n.Args{"ID": "home.table." + column}
	}
	return app.text(message)
}

func (a *App) toggleHomeFavorite(path string) {
	a.ensureDesktopRuntime()
	if a.homeFavorites == nil {
		a.homeFavorites = make(map[string]bool)
	}
	favorite := !a.homeFavorites[path]
	if favorite {
		a.homeFavorites[path] = true
	} else {
		delete(a.homeFavorites, path)
	}
	if a.workspaceStore == nil {
		return
	}
	job := workspaceWriteJob{favoritePath: path, favoriteValue: favorite, saveFavorite: true}
	if a.workspaceWriteJobs != nil {
		a.workspaceWriteJobs <- job
		return
	}
	if err := a.workspaceStore.SaveFavorite(path, favorite); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "home.error.save_favorites"}, err, notificationToast)
	}
}

func (a *App) loadHomeFavorites() {
	a.ensureDesktopRuntime()
	if a.workspaceStore == nil {
		return
	}
	favorites, err := a.workspaceStore.LoadFavorites()
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "home.error.load_favorites"}, err, notificationToast)
		return
	}
	if a.homeFavorites == nil {
		a.homeFavorites = make(map[string]bool, len(favorites))
	}
	for _, path := range favorites {
		a.homeFavorites[path] = true
	}
}

func homeTypeMatches(path, filter string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch filter {
	case homeFilterDocuments:
		return ext == ".docx" || ext == ".doc" || ext == ".txt"
	case homeFilterSpreadsheets:
		return ext == ".xlsx" || ext == ".xls" || ext == ".csv"
	case homeFilterPresentations:
		return ext == ".pptx" || ext == ".ppt"
	case homeFilterPDF:
		return ext == ".pdf"
	case homeFilterMarkdown:
		return ext == ".md" || ext == ".markdown"
	case homeFilterJSON:
		return ext == ".json"
	default:
		return true
	}
}

func extensionForPath(path string) string {
	ext := strings.TrimPrefix(strings.ToUpper(filepath.Ext(path)), ".")
	if len(ext) > 4 {
		return ext[:4]
	}
	if ext == "" {
		return "FILE"
	}
	return ext
}

func homeFileColor(path string) color.NRGBA {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xlsx", ".xls", ".csv":
		return color.NRGBA{R: 79, G: 166, B: 107, A: 255}
	case ".pptx", ".ppt":
		return color.NRGBA{R: 226, G: 59, B: 39, A: 255}
	case ".pdf":
		return color.NRGBA{R: 240, G: 68, B: 69, A: 255}
	case ".md", ".markdown":
		return color.NRGBA{R: 133, G: 88, B: 232, A: 255}
	case ".json":
		return color.NRGBA{R: 214, G: 154, B: 45, A: 255}
	case ".html", ".htm":
		return color.NRGBA{R: 18, G: 162, B: 161, A: 255}
	default:
		return color.NRGBA{R: 52, G: 120, B: 209, A: 255}
	}
}

func formatHomeFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.0f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

func (a *App) formatHomeModified(value time.Time) string {
	return a.formatHomeModifiedAt(value, time.Now())
}

func (a *App) formatHomeModifiedAt(value, now time.Time) string {
	value = value.Local()
	today := now.In(value.Location())
	date := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, value.Location())
	monthName := value.Format("Jan")
	if a.localeSnapshot().Resolved == i18n.LocaleChinese {
		monthName = fmt.Sprintf("%d月", value.Month())
	}
	args := i18n.Args{
		"MonthName": monthName,
		"Day":       value.Day(),
		"Year":      value.Year(),
	}
	switch {
	case date.Equal(todayDate):
		return a.text(i18n.Message{ID: "home.modified.today"})
	case date.Equal(todayDate.AddDate(0, 0, -1)):
		return a.text(i18n.Message{ID: "home.modified.yesterday"})
	case value.Year() == today.Year():
		return a.text(i18n.Message{ID: "home.modified.this_year", Args: args})
	default:
		return a.text(i18n.Message{ID: "home.modified.other_year", Args: args})
	}
}
