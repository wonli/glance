package desktopui

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"glance/internal/workspace"
)

const homeFolderDoubleClickWindow = 450 * time.Millisecond

const homeFolderPageSize = 30

func (a *App) currentHomeFolderSnapshot() (workspace.HomeFolderSnapshot, uint64, error) {
	if a == nil || a.desktopRuntime == nil {
		return workspace.HomeFolderSnapshot{}, 0, errors.New("工作区文件夹不可用")
	}
	if err := a.homeFolderStateError(); err != nil {
		return workspace.HomeFolderSnapshot{}, 0, fmt.Errorf("工作区文件夹不可用：%w", err)
	}
	snapshot, revision := a.homeFolderSnapshot()
	return snapshot, revision, nil
}

func hasHomeFolder(snapshot workspace.HomeFolderSnapshot, folderID string) bool {
	for _, folder := range snapshot.Folders {
		if folder.ID == folderID {
			return true
		}
	}
	return false
}

type homeFolderTreeRow struct {
	folder workspace.HomeFolder
	depth  int
}

type homeFolderContentsResult struct {
	folderID       string
	generation     uint64
	recents        []workspace.Recent
	metadata       map[string]homeFileMetadata
	canonicalPaths map[string]string
	err            error
}

func (a *App) mutateHomeFolderSnapshot(mutate func(workspace.HomeFolderSnapshot) (workspace.HomeFolderSnapshot, bool, error)) (uint64, error) {
	if a == nil || a.desktopRuntime == nil {
		return 0, errors.New("工作区文件夹不可用")
	}
	if a.homeFolderWriteResults == nil {
		a.homeFolderWriteResults = make(chan homeFolderWriteResult, 1)
	}
	_, revision, _, err := a.desktopRuntime.mutateHomeFolderSnapshot(mutate, a.homeFolderWriteResults)
	return revision, err
}

func (a *App) createHomeFolder(parentID, name string) error {
	id := newDraftSessionID("home-folder")
	var created workspace.HomeFolder
	if _, err := a.mutateHomeFolderSnapshot(func(snapshot workspace.HomeFolderSnapshot) (workspace.HomeFolderSnapshot, bool, error) {
		next, err := snapshot.AddFolder(workspace.HomeFolder{ID: id, ParentID: parentID, Name: name})
		if err != nil {
			return snapshot, false, err
		}
		created = next.Folders[len(next.Folders)-1]
		return next, true, nil
	}); err != nil {
		return err
	}
	if parentID != "" {
		a.homeFolderExpanded[parentID] = true
	}
	a.selectHomeFolder(created.ID)
	return nil
}

func (a *App) renameHomeFolder(folderID, name string) error {
	trimmed := strings.TrimSpace(name)
	_, err := a.mutateHomeFolderSnapshot(func(snapshot workspace.HomeFolderSnapshot) (workspace.HomeFolderSnapshot, bool, error) {
		for _, folder := range snapshot.Folders {
			if folder.ID == folderID && folder.Name == trimmed {
				return snapshot, false, nil
			}
		}
		next, err := snapshot.RenameFolder(folderID, name)
		return next, err == nil, err
	})
	return err
}

func (a *App) removeHomeFolder(folderID string) error {
	if a == nil || a.desktopRuntime == nil {
		return errors.New("工作区文件夹不可用")
	}
	if !a.desktopRuntime.beginHomeFolderRemoval(folderID) {
		return errors.New("文件夹删除正在保存，请稍后再试")
	}
	revision, err := a.mutateHomeFolderSnapshot(func(snapshot workspace.HomeFolderSnapshot) (workspace.HomeFolderSnapshot, bool, error) {
		next, err := snapshot.RemoveFolder(folderID)
		return next, err == nil, err
	})
	if err != nil {
		a.desktopRuntime.cancelHomeFolderRemoval(folderID)
		return err
	}
	a.desktopRuntime.setHomeFolderRemovalRevision(folderID, revision)
	return nil
}

func (a *App) registerHomeFolderPath(path string) error {
	return a.registerCanonicalHomeFolderPath(canonicalDocumentPath(path))
}

// registerCanonicalHomeFolderPath accepts a path canonicalized by the
// background directory-picker command, so the event handler does no filesystem
// lookup while updating the workspace snapshot.
func (a *App) registerCanonicalHomeFolderPath(path string) error {
	if a == nil || a.desktopRuntime == nil {
		return errors.New("工作区文件夹不可用")
	}
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		absolute, err := filepath.Abs(cleaned)
		if err != nil {
			return err
		}
		cleaned = absolute
	}
	name := filepath.Base(cleaned)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "本地文件夹"
	}
	id := newDraftSessionID("home-folder")
	if _, err := a.mutateHomeFolderSnapshot(func(snapshot workspace.HomeFolderSnapshot) (workspace.HomeFolderSnapshot, bool, error) {
		for _, folder := range snapshot.Folders {
			if folder.SourcePath != "" && folder.SourcePath == cleaned {
				return snapshot, false, fmt.Errorf("文件夹已登记：%s", filepath.Base(cleaned))
			}
		}
		next, err := snapshot.AddFolder(workspace.HomeFolder{
			ID:         id,
			Name:       name,
			SourcePath: cleaned,
		})
		return next, err == nil, err
	}); err != nil {
		return err
	}
	a.selectHomeFolder(id)
	return nil
}

func (a *App) handleHomeFolderPickerResult(result fileCommandResult) {
	if result.err != nil {
		a.setStatus("登记文件夹失败：" + result.err.Error())
		return
	}
	if result.path == "" {
		a.setStatus("已取消打开文件夹")
		return
	}
	if err := a.registerCanonicalHomeFolderPath(result.path); err != nil {
		a.setStatus("登记文件夹失败：" + err.Error())
		return
	}
	a.setStatus("已添加文件夹到导航")
}

func (a *App) selectHomeFolder(folderID string) {
	changed := a.homeSelectedFolderID != folderID
	a.homeSection = homeSectionRecent
	a.homeSelectedFolderID = folderID
	if changed {
		a.homeFolderPageID = folderID
		a.homeFolderPageSize = homeFolderPageSize
	}
	a.dismissHomeFolderMenu()
	a.resetHomeContentScroll()
	if changed && folderID != "" {
		if sourcePath := a.homeFolderSourcePath(folderID); sourcePath != "" {
			a.refreshHomeFolderContents(folderID, sourcePath)
		}
	}
}

func (a *App) visibleHomeFolderPageSize(folderID string) int {
	if a.homeFolderPageID != folderID || a.homeFolderPageSize <= 0 {
		a.homeFolderPageID = folderID
		a.homeFolderPageSize = homeFolderPageSize
	}
	return a.homeFolderPageSize
}

func (a *App) loadMoreHomeFolderItems(folderID string, total int) {
	if folderID == "" || total <= 0 || a.homeContentViewportHeight <= 0 {
		return
	}
	pageSize := a.visibleHomeFolderPageSize(folderID)
	if pageSize >= total || a.homeContentList.Position.Length <= 0 {
		return
	}
	viewportEnd := a.homeContentList.Position.Offset + a.homeContentViewportHeight
	if viewportEnd < a.homeContentList.Position.Length-a.homeContentViewportHeight/10 {
		return
	}
	a.homeFolderPageSize = min(total, pageSize+homeFolderPageSize)
}

func (a *App) toggleHomeFolderExpanded(folderID string) {
	if a.homeFolderExpanded == nil {
		a.homeFolderExpanded = make(map[string]bool)
	}
	if a.homeFolderExpanded[folderID] {
		delete(a.homeFolderExpanded, folderID)
		return
	}
	a.homeFolderExpanded[folderID] = true
}

func (a *App) homeFolderNameClicked(folderID string, now time.Time) {
	wasDoubleClick := a.homeFolderLastNameID == folderID && !now.Before(a.homeFolderLastNameAt) && now.Sub(a.homeFolderLastNameAt) <= homeFolderDoubleClickWindow
	a.selectHomeFolder(folderID)
	if wasDoubleClick {
		a.toggleHomeFolderExpanded(folderID)
		a.homeFolderLastNameID = ""
		a.homeFolderLastNameAt = time.Time{}
		return
	}
	a.homeFolderLastNameID = folderID
	a.homeFolderLastNameAt = now
}

func (a *App) toggleHomeFolderMenu(folderID string) {
	if a.homeFolderMenuFolderID == folderID {
		a.dismissHomeFolderMenu()
		return
	}
	a.homeFolderMenuFolderID = folderID
	a.homeFolderMenuBounds = image.Rectangle{}
	a.homeFolderMenuAnchorBounds = image.Rectangle{}
}

func (a *App) dismissHomeFolderMenu() {
	a.homeFolderMenuFolderID = ""
	a.homeFolderMenuBounds = image.Rectangle{}
	a.homeFolderMenuAnchorBounds = image.Rectangle{}
}

func (a *App) homeFolderVisibleRecents(folderID string) []workspace.Recent {
	return a.filterHomeRecents(a.homeFolderRecents(folderID), true, nil)
}

func (a *App) homeFolderRecents(folderID string) []workspace.Recent {
	if a == nil || folderID == "" {
		return nil
	}
	snapshot, _ := a.homeFolderSnapshot()
	assigned := make(map[string]bool)
	for _, assignment := range snapshot.Assignments {
		if assignment.FolderID == folderID {
			assigned[a.homeDocumentIdentity(assignment.Path)] = true
		}
	}
	all := make([]workspace.Recent, 0, len(assigned)+len(a.homeFolderContents[folderID]))
	seen := make(map[string]bool, len(assigned)+len(a.homeFolderContents[folderID]))
	if a.collection != nil {
		for _, item := range a.collection.Recents() {
			key := a.homeDocumentIdentity(item.Path)
			if assigned[key] {
				all = append(all, item)
				seen[key] = true
			}
		}
	}
	for _, item := range a.homeFolderContents[folderID] {
		key := a.homeDocumentIdentity(item.Path)
		if !seen[key] {
			all = append(all, item)
			seen[key] = true
		}
	}
	return all
}

func homeFolderEmptyStateMessage(hasUnderlyingItems bool) string {
	if !hasUnderlyingItems {
		return "暂无文件"
	}
	return "没有符合条件的文件"
}

func cleanHomeDocumentPath(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func (a *App) homeDocumentIdentity(path string) string {
	cleaned := cleanHomeDocumentPath(path)
	if a != nil {
		if canonical, ok := a.homeCanonicalPaths[cleaned]; ok {
			return canonical
		}
	}
	return cleaned
}

func (a *App) handleHomeFolderWriteResult(result homeFolderWriteResult) {
	if result.err != nil {
		a.setStatus("文件夹保存失败：" + result.err.Error())
	} else if strings.HasPrefix(a.status, "文件夹保存失败：") {
		a.setStatus("")
	}
	a.reconcileHomeFolderSelection()
}

func (a *App) reconcileHomeFolderSelection() {
	if a == nil || a.homeSelectedFolderID == "" {
		return
	}
	snapshot, pending := a.desktopRuntime.homeFolderSelectionState(a.homeSelectedFolderID)
	if pending {
		return
	}
	if hasHomeFolder(snapshot, a.homeSelectedFolderID) {
		return
	}
	removedID := a.homeSelectedFolderID
	delete(a.homeFolderContents, removedID)
	delete(a.homeFolderExpanded, removedID)
	delete(a.homeFolderRows, removedID)
	a.homeFolderContentsGeneration++
	a.selectHomeFolder("")
}

func (a *App) homeFolderSourcePath(folderID string) string {
	snapshot, _ := a.homeFolderSnapshot()
	for _, folder := range snapshot.Folders {
		if folder.ID == folderID {
			return folder.SourcePath
		}
	}
	return ""
}

func (a *App) refreshHomeFolderContents(folderID, sourcePath string) {
	a.ensureDesktopRuntime()
	if a.homeFolderContentsResults == nil {
		a.homeFolderContentsResults = make(chan homeFolderContentsResult, 8)
	}
	if a.homeFolderContents == nil {
		a.homeFolderContents = make(map[string][]workspace.Recent)
	}
	a.homeFolderContentsGeneration++
	generation := a.homeFolderContentsGeneration
	a.launchManagedBackground(func() {
		result := homeFolderContentsResult{folderID: folderID, generation: generation}
		result.recents, result.metadata, result.canonicalPaths, result.err = scanHomeFolderContents(sourcePath)
		select {
		case a.homeFolderContentsResults <- result:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	})
}

func scanHomeFolderContents(sourcePath string) ([]workspace.Recent, map[string]homeFileMetadata, map[string]string, error) {
	recents, err := scanOfficeFolder(sourcePath)
	if err != nil {
		return nil, nil, nil, err
	}
	metadata := make(map[string]homeFileMetadata, len(recents))
	canonicalPaths := make(map[string]string, len(recents))
	for index := range recents {
		originalPath := cleanHomeDocumentPath(recents[index].Path)
		recents[index].Path = canonicalDocumentPath(recents[index].Path)
		canonicalPaths[originalPath] = recents[index].Path
		info, statErr := os.Stat(recents[index].Path)
		if statErr != nil {
			metadata[recents[index].Path] = homeFileMetadata{unavailable: true}
			continue
		}
		if info.IsDir() {
			recents[index].Path = ""
			continue
		}
		metadata[recents[index].Path] = homeFileMetadata{modified: info.ModTime(), size: info.Size()}
	}
	filtered := recents[:0]
	for _, item := range recents {
		if item.Path != "" {
			filtered = append(filtered, item)
		}
	}
	return filtered, metadata, canonicalPaths, nil
}

func (a *App) applyHomeFolderContentsResult(result homeFolderContentsResult) {
	if result.generation != a.homeFolderContentsGeneration {
		return
	}
	snapshot, _ := a.homeFolderSnapshot()
	if !hasHomeFolder(snapshot, result.folderID) {
		return
	}
	if result.err != nil {
		delete(a.homeFolderContents, result.folderID)
		if a.homeSelectedFolderID == result.folderID {
			a.setStatus("读取文件夹内容失败：" + result.err.Error())
			if a.window != nil {
				a.window.Invalidate()
			}
		}
		return
	}
	if a.homeFolderContents == nil {
		a.homeFolderContents = make(map[string][]workspace.Recent)
	}
	a.homeFolderContents[result.folderID] = append([]workspace.Recent(nil), result.recents...)
	if a.homeMetadata == nil {
		a.homeMetadata = make(map[string]homeFileMetadata)
	}
	for path, metadata := range result.metadata {
		a.homeMetadata[path] = metadata
	}
	if a.homeCanonicalPaths == nil {
		a.homeCanonicalPaths = make(map[string]string)
	}
	for path, canonical := range result.canonicalPaths {
		a.homeCanonicalPaths[path] = canonical
	}
	if a.homeSelectedFolderID == result.folderID && a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) homeFolderDisplayPath(folderID string) string {
	snapshot, _ := a.homeFolderSnapshot()
	byID := make(map[string]workspace.HomeFolder, len(snapshot.Folders))
	for _, folder := range snapshot.Folders {
		byID[folder.ID] = folder
	}
	var names []string
	for current := byID[folderID]; current.ID != ""; current = byID[current.ParentID] {
		names = append(names, current.Name)
		if current.ParentID == "" {
			break
		}
	}
	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}
	return strings.Join(names, " / ")
}

func homeFolderTreeRows(snapshot workspace.HomeFolderSnapshot, expanded map[string]bool) []homeFolderTreeRow {
	children := make(map[string][]workspace.HomeFolder)
	for _, folder := range snapshot.Folders {
		children[folder.ParentID] = append(children[folder.ParentID], folder)
	}
	for parentID := range children {
		sort.SliceStable(children[parentID], func(i, j int) bool {
			if children[parentID][i].SortOrder == children[parentID][j].SortOrder {
				return children[parentID][i].ID < children[parentID][j].ID
			}
			return children[parentID][i].SortOrder < children[parentID][j].SortOrder
		})
	}
	rows := make([]homeFolderTreeRow, 0, len(snapshot.Folders))
	var appendChildren func(string, int)
	appendChildren = func(parentID string, depth int) {
		for _, folder := range children[parentID] {
			rows = append(rows, homeFolderTreeRow{folder: folder, depth: depth})
			if expanded[folder.ID] {
				appendChildren(folder.ID, depth+1)
			}
		}
	}
	appendChildren("", 0)
	return rows
}

func (a *App) showCreateHomeFolderDialog(parentID string) {
	parentPath := "Glance"
	if parentID != "" {
		parentPath += " / " + a.homeFolderDisplayPath(parentID)
	}
	a.modal.ShowInput("新建文件夹", "创建位置："+parentPath, "未命名文件夹", "文件夹名称", func(action string) {
		if action == "cancel" {
			a.modal.Hide()
			return
		}
		if action != "confirm" {
			return
		}
		if err := a.createHomeFolder(parentID, a.modal.Input.Text()); err != nil {
			a.setStatus("创建文件夹失败：" + err.Error())
			return
		}
		a.modal.Hide()
	}, modalDialogActionSpec{ID: "cancel", Label: "取消"}, modalDialogActionSpec{ID: "confirm", Label: "创建"})
}

func (a *App) showRenameHomeFolderDialog(folderID string) {
	snapshot, _, err := a.currentHomeFolderSnapshot()
	if err != nil {
		a.setStatus(err.Error())
		return
	}
	name := ""
	for _, folder := range snapshot.Folders {
		if folder.ID == folderID {
			name = folder.Name
			break
		}
	}
	if name == "" {
		a.setStatus("文件夹不存在")
		return
	}
	a.modal.ShowInput("重命名文件夹", "重命名「"+name+"」", name, "文件夹名称", func(action string) {
		if action == "cancel" {
			a.modal.Hide()
			return
		}
		if action != "confirm" {
			return
		}
		if err := a.renameHomeFolder(folderID, a.modal.Input.Text()); err != nil {
			a.setStatus("重命名失败：" + err.Error())
			return
		}
		a.modal.Hide()
	}, modalDialogActionSpec{ID: "cancel", Label: "取消"}, modalDialogActionSpec{ID: "confirm", Label: "保存"})
}

func (a *App) showRemoveHomeFolderConfirm(folderID string) {
	snapshot, _, err := a.currentHomeFolderSnapshot()
	if err != nil {
		a.setStatus(err.Error())
		return
	}
	name := ""
	for _, folder := range snapshot.Folders {
		if folder.ID == folderID {
			name = folder.Name
			break
		}
	}
	if name == "" {
		a.setStatus("文件夹不存在")
		return
	}
	a.modal.ShowConfirm("移除文件夹", "移除「"+name+"」？\n仅从 Glance 的文件夹列表中移除；本地文件与子文件夹不会被删除。", modalDialogConfirmStyle{}, func(action string) {
		if action == "cancel" {
			a.modal.Hide()
			return
		}
		if action != "confirm" {
			return
		}
		if err := a.removeHomeFolder(folderID); err != nil {
			a.setStatus("移除文件夹失败：" + err.Error())
			return
		}
		a.modal.Hide()
	}, modalDialogActionSpec{ID: "cancel", Label: "取消"}, modalDialogActionSpec{ID: "confirm", Label: "从列表移除", Style: modalDialogActionDanger})
}
