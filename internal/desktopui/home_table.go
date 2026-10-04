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

	"glance/internal/workspace"
)

func (a *App) layoutHomeRecentTable(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(330))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	items := a.homeVisibleRecents(true)
	return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(14), func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutHomeTableHeader(gtx) }),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(items) == 0 {
					message := "最近还没有文件"
					if a.homeSection == homeSectionFavorites {
						message = "收藏的文件会显示在这里"
					} else if strings.TrimSpace(a.homeSearch.Text()) != "" || a.homeTypeFilter != "全部" {
						message = "没有符合条件的文件"
					}
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeTextLabel(gtx, a.theme, message, 13, homeMuted, false)
					})
				}
				return material.List(a.theme, &a.recentList).Layout(gtx, len(items), func(gtx layout.Context, index int) layout.Dimensions {
					return a.layoutHomeRecentRow(gtx, items[index])
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
				return homeTableColumns(gtx, a.theme, true, nil, a.homeSortLabel())
			})
		},
	)
}

func (a *App) layoutHomeRecentRow(gtx layout.Context, item workspace.Recent) layout.Dimensions {
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
		a.status = "最近文件操作"
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
				return homeTableColumns(gtx, a.theme, false, &homeRowData{app: a, item: item, open: button, favorite: star, more: more}, "")
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

func homeTableColumns(gtx layout.Context, theme *material.Theme, header bool, row *homeRowData, sortLabel string) layout.Dimensions {
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
			return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, label.Layout)
		}
	}
	dataColumns := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = nameWidth, nameWidth
			if header {
				return textStyle("名称", false)(gtx)
			}
			meta := row.app.homeMetadata[row.item.Path]
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return homeFileBadge(gtx, extensionForPath(row.item.Path), homeFileColor(row.item.Path), 34)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(14), Right: unit.Dp(12), Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						fg := color.NRGBA{R: 45, G: 52, B: 61, A: 255}
						if meta.unavailable {
							fg = homeMuted
						}
						return homeTextLabel(gtx, row.app.theme, filepath.Base(row.item.Path), unit.Sp(15*scale), fg, true)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = locationWidth, locationWidth
			if header {
				return textStyle("位置", false)(gtx)
			}
			return textStyle(filepath.Base(filepath.Dir(row.item.Path)), false)(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = modifiedWidth, modifiedWidth
			if header {
				return textStyle(sortLabel, false)(gtx)
			}
			meta := row.app.homeMetadata[row.item.Path]
			modified := meta.modified
			if modified.IsZero() {
				modified = row.item.OpenedAt
			}
			if modified.IsZero() {
				return textStyle("—", false)(gtx)
			}
			return textStyle(formatHomeModified(modified), false)(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = sizeWidth, sizeWidth
			if header {
				return textStyle("大小", false)(gtx)
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
				return row.open.Layout(gtx, layoutDataColumns)
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
						return homeIconButton(gtx, row.favorite, "star", unit.Dp(18*scale), color.NRGBA{R: 185, G: 192, B: 200, A: 255}, row.app.homeFavorites[row.item.Path])
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
	filtered := make([]workspace.Recent, 0, len(recents))
	query := strings.ToLower(strings.TrimSpace(a.homeSearch.Text()))
	for _, item := range recents {
		if !applyFilter {
			filtered = append(filtered, item)
			continue
		}
		if a.homeSection == homeSectionFavorites && !a.homeFavorites[item.Path] {
			continue
		}
		if a.homeTypeFilter != "全部" && !homeTypeMatches(item.Path, a.homeTypeFilter) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(filepath.Base(item.Path)), query) && !strings.Contains(strings.ToLower(filepath.Dir(item.Path)), query) {
			continue
		}
		filtered = append(filtered, item)
	}
	if applyFilter {
		sort.SliceStable(filtered, func(i, j int) bool {
			left := a.homeMetadata[filtered[i].Path].modified
			if left.IsZero() {
				left = filtered[i].OpenedAt
			}
			right := a.homeMetadata[filtered[j].Path].modified
			if right.IsZero() {
				right = filtered[j].OpenedAt
			}
			if a.homeSortModifiedDesc {
				return left.After(right)
			}
			return left.Before(right)
		})
	}
	if len(filtered) > 5 {
		filtered = filtered[:5]
	}
	return filtered
}

func (a *App) homeSortLabel() string {
	if a.homeSortModifiedDesc {
		return "修改时间 ↓"
	}
	return "修改时间 ↑"
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
		a.status = "收藏保存失败：" + err.Error()
	}
}

func (a *App) loadHomeFavorites() {
	a.ensureDesktopRuntime()
	if a.workspaceStore == nil {
		return
	}
	favorites, err := a.workspaceStore.LoadFavorites()
	if err != nil {
		a.status = "读取收藏失败：" + err.Error()
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
	case "文档":
		return ext == ".docx" || ext == ".doc" || ext == ".json" || ext == ".txt"
	case "表格":
		return ext == ".xlsx" || ext == ".xls" || ext == ".csv"
	case "演示":
		return ext == ".pptx" || ext == ".ppt"
	case "PDF":
		return ext == ".pdf"
	case "Markdown":
		return ext == ".md" || ext == ".markdown"
	case "HTML":
		return ext == ".html" || ext == ".htm"
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

func formatHomeModified(value time.Time) string {
	value = value.Local()
	today := time.Now().In(value.Location())
	date := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, value.Location())
	switch {
	case date.Equal(todayDate):
		return "今天"
	case date.Equal(todayDate.AddDate(0, 0, -1)):
		return "昨天"
	case value.Year() == today.Year():
		return fmt.Sprintf("%d月%d日", value.Month(), value.Day())
	default:
		return value.Format("2006/01/02")
	}
}
