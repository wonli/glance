package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"glance/internal/i18n"
	"glance/internal/workspace"
)

const (
	homeRecentMenuWidth       = 218
	homeRecentMenuHeight      = 213
	homeRecentMenuItemHeight  = 36
	homeRecentMenuItemCount   = 5
	homeRecentMenuShadowAlpha = 12
)

type homeRecentMenuAction struct {
	id, label, icon string
	labelMessage    i18n.Message
	remove          bool
}

var homeRecentMenuActions = [...]homeRecentMenuAction{
	{id: "open", label: "打开", labelMessage: i18n.Message{ID: "home.recent.action.open"}, icon: "folder-open"},
	{id: "open-folder", label: "打开文件夹", labelMessage: i18n.Message{ID: "home.recent.action.open_folder"}, icon: "folder"},
	{id: "copy-path", label: "复制路径", labelMessage: i18n.Message{ID: "home.recent.action.copy_path"}, icon: "link"},
	{id: "copy-name", label: "复制文件名", labelMessage: i18n.Message{ID: "home.recent.action.copy_name"}, icon: "copy"},
	{id: "remove", label: "移除", labelMessage: i18n.Message{ID: "home.recent.action.remove"}, icon: "list-minus", remove: true},
}

type homeRecentUndoRecord struct {
	path             string
	before, after    []workspace.Recent
	removeGeneration uint64
	undoGeneration   uint64
	pendingUndo      bool
}

type homeRecentWriteResult struct {
	generation uint64
	err        error
}

func homeRecentMenuNextSelection(current, direction int) int {
	if current < 0 {
		if direction < 0 {
			return homeRecentMenuItemCount - 1
		}
		return 0
	}
	next := (current + direction) % homeRecentMenuItemCount
	if next < 0 {
		next += homeRecentMenuItemCount
	}
	return next
}

func homeRecentFloatingMenuBounds(anchor, viewport image.Rectangle, size image.Point, gap int) image.Rectangle {
	if viewport.Empty() || size.X <= 0 || size.Y <= 0 {
		return image.Rectangle{}
	}
	width := min(size.X, viewport.Dx())
	height := min(size.Y, viewport.Dy())
	left := anchor.Max.X - width
	if left < viewport.Min.X {
		left = viewport.Min.X
	}
	if left+width > viewport.Max.X {
		left = viewport.Max.X - width
	}
	top := anchor.Max.Y + gap
	if top+height > viewport.Max.Y {
		top = anchor.Min.Y - gap - height
	}
	if top < viewport.Min.Y {
		top = viewport.Min.Y
	}
	if top+height > viewport.Max.Y {
		top = viewport.Max.Y - height
	}
	return image.Rect(left, top, left+width, top+height)
}

func homeRecentMoreButtonBounds(rowOrigin image.Point, rowWidth int, gtx layout.Context) image.Rectangle {
	inset := gtx.Dp(unit.Dp(22))
	contentWidth := max(0, rowWidth-2*inset)
	metrics := homeTableColumnMetricsForWidth(gtx, contentWidth)
	starWidth := gtx.Dp(unit.Dp(34 * metrics.scale))
	buttonWidth := gtx.Dp(unit.Dp(18 * metrics.scale))
	actionWidth := gtx.Dp(unit.Dp(52 * metrics.scale))
	left := rowOrigin.X + inset + contentWidth - actionWidth + starWidth
	top := rowOrigin.Y + (gtx.Dp(unit.Dp(56))-buttonWidth)/2
	return image.Rect(left, top, left+buttonWidth, top+buttonWidth)
}

func (a *App) toggleHomeRecentMenu(path string, gtx layout.Context) {
	if a.homeRecentMenuPath == path {
		a.dismissHomeRecentMenu()
		if button := a.homeMoreButtons[path]; button != nil {
			gtx.Execute(key.FocusCmd{Tag: button})
		}
		return
	}
	if a.homeFolderMenuFolderID != "" {
		a.dismissHomeFolderMenu()
	}
	a.homeRecentOpenPath = ""
	a.homeRecentMenuPath = path
	a.homeRecentMenuIndex = -1
	a.homeRecentMenuBounds = image.Rectangle{}
	gtx.Execute(key.FocusCmd{Tag: &a.homeRecentMenuDismiss})
}

func (a *App) dismissHomeRecentMenu() {
	a.homeRecentMenuPath = ""
	a.homeRecentOpenPath = ""
	a.homeRecentMenuBounds = image.Rectangle{}
	a.homeRecentMenuAnchorBounds = image.Rectangle{}
}

func (a *App) layoutHomeRecentMenuDismissInput(gtx layout.Context) {
	if a.homeRecentMenuPath == "" || gtx.Constraints.Max.X <= 0 || gtx.Constraints.Max.Y <= 0 {
		return
	}
	for event, ok := gtx.Event(key.Filter{Focus: &a.homeRecentMenuDismiss}); ok; event, ok = gtx.Event(key.Filter{Focus: &a.homeRecentMenuDismiss}) {
		keyEvent, isKey := event.(key.Event)
		if !isKey || keyEvent.State != key.Press {
			continue
		}
		switch keyEvent.Name {
		case key.NameEscape:
			path := a.homeRecentMenuPath
			a.dismissHomeRecentMenu()
			if button := a.homeMoreButtons[path]; button != nil {
				gtx.Execute(key.FocusCmd{Tag: button})
			}
			return
		case key.NameUpArrow:
			a.homeRecentMenuIndex = homeRecentMenuNextSelection(a.homeRecentMenuIndex, -1)
		case key.NameDownArrow:
			a.homeRecentMenuIndex = homeRecentMenuNextSelection(a.homeRecentMenuIndex, 1)
		case key.NameEnter, key.NameReturn:
			index := a.homeRecentMenuIndex
			if index < 0 {
				index = 0
			}
			a.executeHomeRecentMenuAction(gtx, index)
			return
		}
	}
	if a.homeRecentMenuDismiss.Clicked(gtx) {
		history := a.homeRecentMenuDismiss.History()
		if len(history) != 0 {
			point := history[len(history)-1].Position
			if !point.In(a.homeRecentMenuBounds) && !point.In(a.homeRecentMenuAnchorBounds) {
				a.dismissHomeRecentMenu()
			}
		}
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	a.homeRecentMenuDismiss.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	pass.Pop()
}

func (a *App) layoutHomeRecentFloatingMenu(gtx layout.Context) layout.Dimensions {
	if a.homeRecentMenuPath == "" {
		return layout.Dimensions{}
	}
	viewport := image.Rectangle{Max: gtx.Constraints.Max}
	if a.homeRecentMenuAnchorBounds.Intersect(viewport).Empty() {
		a.dismissHomeRecentMenu()
		return layout.Dimensions{}
	}
	size := image.Pt(gtx.Dp(unit.Dp(homeRecentMenuWidth)), gtx.Dp(unit.Dp(homeRecentMenuHeight)))
	a.homeRecentMenuBounds = homeRecentFloatingMenuBounds(a.homeRecentMenuAnchorBounds, viewport, size, gtx.Dp(unit.Dp(8)))
	if a.homeRecentMenuBounds.Empty() {
		a.dismissHomeRecentMenu()
		return layout.Dimensions{}
	}
	transform := op.Offset(a.homeRecentMenuBounds.Min).Push(gtx.Ops)
	defer transform.Pop()
	menuSize := a.homeRecentMenuBounds.Size()
	gtx.Constraints = layout.Exact(menuSize)
	return a.layoutHomeRecentFloatingMenuPanel(gtx)
}

func (a *App) layoutHomeRecentFloatingMenuPanel(gtx layout.Context) layout.Dimensions {
	menuSize := gtx.Constraints.Max
	for _, shadow := range []struct{ offset, alpha int }{{8, homeRecentMenuShadowAlpha}, {4, 7}} {
		shadowBounds := image.Rectangle{Max: menuSize}.Add(image.Pt(0, gtx.Dp(unit.Dp(shadow.offset))))
		paint.FillShape(gtx.Ops, color.NRGBA{R: 25, G: 34, B: 46, A: uint8(shadow.alpha)}, clip.UniformRRect(shadowBounds, gtx.Dp(unit.Dp(9))).Op(gtx.Ops))
	}
	return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(9), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, homeRecentMenuItemCount+6)
			for index := range homeRecentMenuActions {
				index := index
				if index == homeRecentMenuItemCount-1 {
					children = append(children,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return homeRecentMenuDivider(gtx) }),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(0, gtx.Dp(unit.Dp(4)))}
						}),
					)
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutHomeRecentMenuItem(gtx, index)
				}))
				if index < homeRecentMenuItemCount-1 {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(0, gtx.Dp(unit.Dp(4)))}
					}))
				}
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

func homeRecentMenuDivider(gtx layout.Context) layout.Dimensions {
	height := max(1, gtx.Dp(unit.Dp(1)))
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, homeRowBorder, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
		},
	)
}

func (a *App) layoutHomeRecentMenuItem(gtx layout.Context, index int) layout.Dimensions {
	action := homeRecentMenuActions[index]
	button := &a.homeRecentMenuItems[index]
	if button.Clicked(gtx) {
		a.executeHomeRecentMenuAction(gtx, index)
	}
	height := gtx.Dp(unit.Dp(homeRecentMenuItemHeight))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bg := color.NRGBA{A: 0}
				if button.Hovered() || a.homeRecentMenuIndex == index {
					bg = homeHoverFill
				}
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(6))).Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return homeIcon(gtx, action.icon, unit.Dp(16), homeRecentMenuActionColor(action))
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(10), Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = 0
								return homeTextLabel(gtx, a.theme, a.messageText(action.labelMessage, action.label), unit.Sp(13), homeRecentMenuActionColor(action), false)
							})
						}),
					)
				})
			},
		)
	})
}

func homeRecentMenuActionColor(action homeRecentMenuAction) color.NRGBA {
	if action.remove {
		return color.NRGBA{R: 196, G: 73, B: 73, A: 255}
	}
	return homeText
}

func (a *App) executeHomeRecentMenuAction(gtx layout.Context, index int) {
	if index < 0 || index >= len(homeRecentMenuActions) || a.homeRecentMenuPath == "" {
		return
	}
	path := a.homeRecentMenuPath
	switch homeRecentMenuActions[index].id {
	case "open":
		if meta, exists := a.homeMetadata[path]; exists && meta.unavailable {
			a.setStatusMessage(i18n.Message{ID: "home.error.file_unavailable"}, feedbackWarning, notificationToast)
			return
		}
		if a.opening || a.isSaving() {
			a.setStatusMessage(i18n.Message{ID: "home.error.operation_busy"}, feedbackWarning, notificationToast)
			return
		}
		a.homeRecentOpenPath = path
		a.startOpen(path)
	case "open-folder":
		if a.fileCommandRunning {
			a.setStatusMessage(i18n.Message{ID: "home.error.operation_busy"}, feedbackWarning, notificationToast)
			return
		}
		a.startFileCommand(commandRevealFile, path)
	case "copy-path":
		value, err := homeRecentMenuCopyValue("copy-path", path)
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "home.error.copy_path"}, err, notificationToast)
			return
		}
		a.copyHomeRecentMenuValue(gtx, value, "copy-path")
	case "copy-name":
		value, err := homeRecentMenuCopyValue("copy-name", path)
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "home.error.copy_name"}, err, notificationToast)
			return
		}
		a.copyHomeRecentMenuValue(gtx, value, "copy-name")
	case "remove":
		a.removeHomeRecent(path)
	}
}

func homeRecentMenuCopyValue(actionID, path string) (string, error) {
	switch actionID {
	case "copy-path":
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		return absolute, nil
	case "copy-name":
		return filepath.Base(path), nil
	default:
		return "", fmt.Errorf("unknown recent-menu copy action %q", actionID)
	}
}

func homeRecentMenuFeedbackMessage(actionID string) (i18n.Message, bool) {
	switch actionID {
	case "copy-path":
		return i18n.Message{ID: "home.feedback.path_copied"}, true
	case "copy-name":
		return i18n.Message{ID: "home.feedback.file_name_copied"}, true
	default:
		return i18n.Message{}, false
	}
}

func (a *App) copyHomeRecentMenuValue(gtx layout.Context, value, actionID string) {
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(value))})
	a.dismissHomeRecentMenu()
	if message, ok := homeRecentMenuFeedbackMessage(actionID); ok {
		a.setStatusMessage(message, feedbackSuccess, notificationToast)
	}
}

func (a *App) removeHomeRecent(path string) {
	if a.collection == nil {
		a.setStatusMessage(i18n.Message{ID: "home.error.recent_unavailable"}, feedbackError, notificationToast)
		return
	}
	before := a.collection.Recents()
	after, _, ok := removeHomeRecentSnapshot(before, path)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "home.error.recent_missing"}, feedbackWarning, notificationSuppressed)
		a.dismissHomeRecentMenu()
		return
	}
	a.collection.StageRecentsSnapshot(after)
	a.homeRecentWriteGeneration++
	record := &homeRecentUndoRecord{
		path:             path,
		before:           before,
		after:            after,
		removeGeneration: a.homeRecentWriteGeneration,
	}
	a.homeRecentUndo = record
	a.dismissHomeRecentMenu()
	a.setStatusMessage(i18n.Message{ID: "home.status.removing_recent"}, feedbackInfo, notificationSuppressed)
	a.queueHomeRecentWrite(after, record.removeGeneration)
}

func removeHomeRecentSnapshot(recents []workspace.Recent, path string) ([]workspace.Recent, workspace.Recent, bool) {
	updated := make([]workspace.Recent, 0, len(recents))
	var removed workspace.Recent
	found := false
	for _, recent := range recents {
		if recent.Path == path && !found {
			removed = recent
			found = true
			continue
		}
		updated = append(updated, recent)
	}
	return updated, removed, found
}

func (a *App) queueHomeRecentWrite(recents []workspace.Recent, generation uint64) {
	if a.homeRecentWriteResults == nil {
		a.homeRecentWriteResults = make(chan homeRecentWriteResult, 1)
	}
	snapshot := append(make([]workspace.Recent, 0, len(recents)), recents...)
	a.ensureDesktopRuntime()
	if a.workspaceWriteJobs != nil {
		a.workspaceWriteJobs <- workspaceWriteJob{recents: snapshot, homeRecentResults: a.homeRecentWriteResults, homeRecentRevision: generation}
		return
	}
	store := a.workspaceStore
	if store == nil {
		publishHomeRecentWriteResult(a.homeRecentWriteResults, homeRecentWriteResult{generation: generation, err: fmt.Errorf("workspace database is unavailable")})
		return
	}
	results := a.homeRecentWriteResults
	a.launchManagedBackground(func() {
		err := store.SaveRecents(snapshot)
		publishHomeRecentWriteResult(results, homeRecentWriteResult{generation: generation, err: err})
		if a.window != nil {
			a.window.Invalidate()
		}
	})
}

func publishHomeRecentWriteResult(results chan homeRecentWriteResult, result homeRecentWriteResult) {
	if results == nil {
		return
	}
	select {
	case results <- result:
		return
	default:
	}
	select {
	case <-results:
	default:
	}
	select {
	case results <- result:
	default:
	}
}

func (a *App) handleHomeRecentWriteResult(result homeRecentWriteResult) {
	record := a.homeRecentUndo
	if record == nil {
		return
	}
	if result.generation == record.removeGeneration && !record.pendingUndo {
		if result.err != nil {
			a.collection.StageRecentsSnapshot(record.before)
			a.homeRecentUndo = nil
			a.setStatusErrorMessage(i18n.Message{ID: "home.error.remove_recent"}, result.err, notificationToast)
			a.homeRecentMenuPath = record.path
			a.homeRecentMenuIndex = 4
			return
		}
		a.setStatusMessage(i18n.Message{ID: "home.feedback.removed_recent"}, feedbackSuccess, notificationToast)
		a.feedback.actionLabel = "撤销"
		a.feedback.actionLabelRef = i18n.Message{ID: "common.action.undo"}
		return
	}
	if result.generation != record.undoGeneration || !record.pendingUndo {
		return
	}
	record.pendingUndo = false
	if result.err != nil {
		a.collection.StageRecentsSnapshot(record.after)
		a.setStatusErrorMessage(i18n.Message{ID: "home.error.undo_remove_recent"}, result.err, notificationToast)
		a.feedback.actionLabel = "重试"
		a.feedback.actionLabelRef = i18n.Message{ID: "common.action.retry"}
		return
	}
	a.homeRecentUndo = nil
	a.setStatusMessage(i18n.Message{ID: "home.feedback.undo_removed_recent"}, feedbackSuccess, notificationToast)
}

func (a *App) undoHomeRecentRemoval() {
	record := a.homeRecentUndo
	if record == nil || record.pendingUndo || a.collection == nil {
		return
	}
	a.collection.StageRecentsSnapshot(record.before)
	a.homeRecentWriteGeneration++
	record.undoGeneration = a.homeRecentWriteGeneration
	record.pendingUndo = true
	a.feedback.actionLabel = ""
	a.feedback.actionLabelRef = i18n.Message{}
	a.queueHomeRecentWrite(record.before, record.undoGeneration)
}

func revealHomeRecentFile(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("读取文件路径失败：%w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return fmt.Errorf("文件不存在或无法访问：%w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("目标不是文件：%s", absolute)
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", "-R", absolute)
	case "windows":
		command = exec.Command("explorer.exe", "/select,", absolute)
	default:
		command = exec.Command("xdg-open", filepath.Dir(absolute))
	}
	if output, err := command.CombinedOutput(); err != nil {
		if strings.TrimSpace(string(output)) != "" {
			return fmt.Errorf("打开文件所在文件夹失败：%s", strings.TrimSpace(string(output)))
		}
		return fmt.Errorf("打开文件所在文件夹失败：%w", err)
	}
	return nil
}
