package desktopui

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"glance/internal/i18n"
	"glance/internal/workspace"
)

type homeFolderDropTarget struct {
	FolderID string
	Bounds   image.Rectangle
}

type homeDocumentDragState struct {
	drag        gesture.Drag
	origin      image.Point
	press       image.Point
	pointer     image.Point
	pressOffset image.Point
	rowSize     image.Point
	started     bool
	target      string
}

func homeFolderDropTargetAt(point image.Point, targets []homeFolderDropTarget) string {
	for _, target := range targets {
		if !target.Bounds.Empty() && point.In(target.Bounds) {
			return target.FolderID
		}
	}
	return ""
}

func homeFolderDropTargetsForVisibleRows(rows []homeFolderTreeRow, first, offset, count int, origin image.Point, viewport image.Rectangle, menuFolderID string, rowHeight, menuExtraHeight int) []homeFolderDropTarget {
	if count <= 0 || rowHeight <= 0 || viewport.Empty() {
		return nil
	}
	first = max(0, first)
	end := min(len(rows), first+count)
	y := origin.Y - offset
	targets := make([]homeFolderDropTarget, 0, end-first)
	for index := first; index < end; index++ {
		bounds := image.Rect(origin.X, y, viewport.Max.X, y+rowHeight).Intersect(viewport)
		if !bounds.Empty() {
			targets = append(targets, homeFolderDropTarget{FolderID: rows[index].folder.ID, Bounds: bounds})
		}
		y += rowHeight
		if rows[index].folder.ID == menuFolderID {
			y += menuExtraHeight
		}
	}
	return targets
}

func (a *App) homeDocumentDragState(path string) *homeDocumentDragState {
	if a.homeDocumentDrags == nil {
		a.homeDocumentDrags = make(map[string]*homeDocumentDragState)
	}
	state := a.homeDocumentDrags[path]
	if state == nil {
		state = &homeDocumentDragState{}
		a.homeDocumentDrags[path] = state
	}
	return state
}

func (a *App) homeRecentRowOrigin(index int, gtx layout.Context) image.Point {
	rowHeight := gtx.Dp(unit.Dp(56))
	if a.homeSection == homeSectionRecent && a.homeSelectedFolderID != "" {
		return image.Pt(a.homeRecentListRootOrigin.X, a.homeRecentListRootOrigin.Y+index*rowHeight)
	}
	return image.Pt(
		a.homeRecentListRootOrigin.X,
		a.homeRecentListRootOrigin.Y-a.recentList.Position.Offset+(index-a.recentList.Position.First)*rowHeight,
	)
}

func (a *App) updateHomeDocumentDrag(gtx layout.Context, path string, origin image.Point) {
	state := a.homeDocumentDragState(path)
	state.origin = origin
	state.rowSize = image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(56)))
	for event, ok := state.drag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; event, ok = state.drag.Update(gtx.Metric, gtx.Source, gesture.Both) {
		local := event.Position.Round()
		point := origin.Add(local)
		switch event.Kind {
		case pointer.Press:
			state.press = local
			state.pointer = point
			state.pressOffset = local
			state.started = false
			state.target = ""
			a.homeActiveDropFolderID = ""
		case pointer.Drag:
			if !state.started {
				delta := local.Sub(state.press)
				slop := gtx.Dp(unit.Dp(3))
				if delta.X*delta.X+delta.Y*delta.Y <= slop*slop {
					continue
				}
				state.started = true
			}
			state.target = homeFolderDropTargetAt(point, a.homeFolderDropTargets)
			state.pointer = point
			a.homeActiveDropFolderID = state.target
			a.homeActiveDragPath = path
			if a.window != nil {
				a.window.Invalidate()
			}
		case pointer.Release:
			if state.started {
				target := homeFolderDropTargetAt(point, a.homeFolderDropTargets)
				if target != "" {
					if err := a.assignHomeDocumentToFolder(path, target); err != nil {
						a.setStatusErrorMessage(i18n.Message{ID: "home.folder.error.categorize_failed"}, err, notificationToast)
					}
				}
			}
			a.clearHomeDocumentDrag(state)
		case pointer.Cancel:
			a.clearHomeDocumentDrag(state)
		}
	}
}

func (a *App) clearHomeDocumentDrag(state *homeDocumentDragState) {
	if state == nil {
		return
	}
	state.started = false
	state.target = ""
	if a.homeActiveDragPath != "" && a.homeDocumentDrags[a.homeActiveDragPath] == state {
		a.homeActiveDragPath = ""
	}
	a.homeActiveDropFolderID = ""
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) layoutHomeDocumentDragOverlay(gtx layout.Context) {
	if a.homeActiveDragPath == "" || a.homeDocumentDrags == nil {
		return
	}
	state := a.homeDocumentDrags[a.homeActiveDragPath]
	if state == nil || !state.started || state.rowSize.X <= 0 || state.rowSize.Y <= 0 {
		return
	}
	origin := state.pointer.Sub(state.pressOffset)
	ghost := image.Rectangle{Min: origin, Max: origin.Add(state.rowSize)}
	if ghost.Intersect(image.Rectangle{Max: gtx.Constraints.Max}).Empty() {
		return
	}
	transform := op.Offset(origin).Push(gtx.Ops)
	defer transform.Pop()
	gtx.Constraints = layout.Exact(state.rowSize)
	shape := clip.UniformRRect(image.Rectangle{Max: state.rowSize}, gtx.Dp(unit.Dp(8)))
	shadow := clip.UniformRRect(image.Rect(0, gtx.Dp(unit.Dp(4)), state.rowSize.X, state.rowSize.Y+gtx.Dp(unit.Dp(4))), gtx.Dp(unit.Dp(8)))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 20, G: 28, B: 38, A: 48}, shadow.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 242}, shape.Op(gtx.Ops))
	paintRoundedBorder(gtx.Ops, shape, float32(gtx.Dp(unit.Dp(1))), homeBorder)
	name := filepath.Base(a.homeActiveDragPath)
	extension := strings.TrimPrefix(strings.ToUpper(filepath.Ext(name)), ".")
	layout.Inset{Left: unit.Dp(22), Right: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return homeFileBadge(gtx, extension, homeFileColor(a.homeActiveDragPath), unit.Dp(34))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(14), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					return homeTextLabel(gtx, a.theme, name, unit.Sp(15), homeText, true)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
		)
	})
}

func (a *App) assignHomeDocumentToFolder(path, folderID string) error {
	path = a.homeDocumentIdentity(path)
	var targetName string
	if _, err := a.mutateHomeFolderSnapshot(func(snapshot workspace.HomeFolderSnapshot) (workspace.HomeFolderSnapshot, bool, error) {
		for _, assignment := range snapshot.Assignments {
			if a.homeDocumentIdentity(assignment.Path) == path && assignment.FolderID == folderID {
				return snapshot, false, nil
			}
		}
		next, err := snapshot.AssignDocument(path, folderID)
		if err != nil {
			return snapshot, false, err
		}
		for _, folder := range next.Folders {
			if folder.ID == folderID {
				targetName = folder.Name
				break
			}
		}
		return next, true, nil
	}); err != nil {
		return err
	}
	if targetName != "" {
		a.setStatusMessage(i18n.Message{ID: "home.folder.status.categorized", Args: i18n.Args{"Name": targetName}}, feedbackSuccess, notificationToast)
	}
	return nil
}

func (a *App) updateHomeFolderDropTargets(rows []homeFolderTreeRow, gtx layout.Context) {
	viewport := image.Rectangle{Min: a.homeFolderTreeRootOrigin, Max: a.homeFolderTreeRootOrigin.Add(gtx.Constraints.Max)}
	a.homeFolderTreeViewport = viewport
	a.homeFolderDropTargets = homeFolderDropTargetsForVisibleRows(
		rows,
		a.homeFolderTreeList.Position.First,
		a.homeFolderTreeList.Position.Offset,
		a.homeFolderTreeList.Position.Count,
		a.homeFolderTreeRootOrigin,
		viewport,
		"",
		gtx.Dp(unit.Dp(36)),
		0,
	)
}
