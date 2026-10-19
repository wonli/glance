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

	"glance/internal/i18n"
	"glance/internal/workspace"
)

const homeFolderPageSize = 30

const homeFolderContentsRefreshInterval = 5 * time.Second

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
	folder      workspace.HomeFolder
	depth       int
	hasChildren bool
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
		name = a.text(i18n.Message{ID: "home.folder.local_name"})
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
		a.setStatusErrorMessage(i18n.Message{ID: "home.folder.error.register_failed"}, result.err, notificationToast)
		return
	}
	if result.path == "" {
		a.setStatusMessage(i18n.Message{ID: "home.folder.status.open_cancelled"}, feedbackInfo, notificationSuppressed)
		return
	}
	if err := a.registerCanonicalHomeFolderPath(result.path); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "home.folder.error.register_failed"}, err, notificationToast)
		return
	}
	a.setStatusMessage(i18n.Message{ID: "home.folder.status.added"}, feedbackSuccess, notificationToast)
}

func (a *App) selectHomeFolder(folderID string) {
	changed := a.homeSelectedFolderID != folderID
	snapshot, _ := a.homeFolderSnapshot()
	a.setHomeFolderOpenPath(snapshot, folderID)
	sourcePath := homeFolderSourcePathFromSnapshot(snapshot, folderID)
	a.homeSection = homeSectionRecent
	a.homeSelectedFolderID = folderID
	if changed {
		a.homeFolderPageID = folderID
		a.homeFolderPageSize = homeFolderPageSize
	}
	a.dismissHomeFolderMenu()
	a.resetHomeContentScroll()
	if folderID != "" && sourcePath != "" {
		a.refreshHomeFolderContents(folderID, sourcePath)
	}
}

func (a *App) setHomeFolderOpenPath(snapshot workspace.HomeFolderSnapshot, folderID string) {
	if a.homeFolderExpanded == nil {
		a.homeFolderExpanded = make(map[string]bool)
	}
	clear(a.homeFolderExpanded)
	if folderID == "" {
		return
	}
	folders := make(map[string]workspace.HomeFolder, len(snapshot.Folders))
	hasChildren := make(map[string]bool, len(snapshot.Folders))
	for _, folder := range snapshot.Folders {
		folders[folder.ID] = folder
		if folder.ParentID != "" {
			hasChildren[folder.ParentID] = true
		}
	}
	for currentID := folderID; currentID != ""; {
		folder, exists := folders[currentID]
		if !exists {
			break
		}
		if hasChildren[currentID] {
			a.homeFolderExpanded[currentID] = true
		}
		currentID = folder.ParentID
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
	snapshot, _ := a.homeFolderSnapshot()
	folder, exists := homeFolderByID(snapshot, folderID)
	if !exists {
		return
	}
	if !a.homeFolderExpanded[folderID] {
		a.selectHomeFolder(folderID)
		return
	}
	if homeFolderIsWithin(snapshot, a.homeSelectedFolderID, folderID) {
		a.selectHomeFolder(folder.ParentID)
		return
	}
	a.selectHomeFolder(a.homeSelectedFolderID)
}

func homeFolderByID(snapshot workspace.HomeFolderSnapshot, folderID string) (workspace.HomeFolder, bool) {
	for _, folder := range snapshot.Folders {
		if folder.ID == folderID {
			return folder, true
		}
	}
	return workspace.HomeFolder{}, false
}

func homeFolderIsWithin(snapshot workspace.HomeFolderSnapshot, folderID, ancestorID string) bool {
	folders := make(map[string]workspace.HomeFolder, len(snapshot.Folders))
	for _, folder := range snapshot.Folders {
		folders[folder.ID] = folder
	}
	for currentID := folderID; currentID != ""; {
		if currentID == ancestorID {
			return true
		}
		folder, exists := folders[currentID]
		if !exists {
			return false
		}
		currentID = folder.ParentID
	}
	return false
}

func (a *App) toggleHomeFolderMenu(folderID string) {
	if a.homeFolderMenuFolderID == folderID {
		a.dismissHomeFolderMenu()
		return
	}
	a.homeFolderMenuFolderID = folderID
	a.homeFolderMenuIndex = -1
	a.homeFolderMenuBounds = image.Rectangle{}
	a.homeFolderMenuAnchorBounds = image.Rectangle{}
}

func (a *App) dismissHomeFolderMenu() {
	a.homeFolderMenuFolderID = ""
	a.homeFolderMenuIndex = -1
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

func homeFolderEmptyStateMessage(hasUnderlyingItems bool) i18n.Message {
	if !hasUnderlyingItems {
		return i18n.Message{ID: "home.folder.empty"}
	}
	return i18n.Message{ID: "home.folder.no_matches"}
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
		a.setStatusErrorMessage(i18n.Message{ID: "home.folder.error.save_failed"}, result.err, notificationToast)
	} else if a.statusRef.ID == "home.folder.error.save_failed" {
		a.clearStatus()
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
	delete(a.homeFolderRows, removedID)
	delete(a.homeFolderContentsLatest, removedID)
	delete(a.homeFolderContentsLoading, removedID)
	delete(a.homeFolderContentsLastScan, removedID)
	a.selectHomeFolder("")
}

func homeFolderSourcePathFromSnapshot(snapshot workspace.HomeFolderSnapshot, folderID string) string {
	folder, exists := homeFolderByID(snapshot, folderID)
	if !exists {
		return ""
	}
	return folder.SourcePath
}

func (a *App) refreshHomeFolderContents(folderID, sourcePath string) {
	a.ensureDesktopRuntime()
	if a.homeFolderContentsResults == nil {
		a.homeFolderContentsResults = make(chan homeFolderContentsResult, 8)
	}
	if a.homeFolderContents == nil {
		a.homeFolderContents = make(map[string][]workspace.Recent)
	}
	if a.homeFolderContentsLoading == nil {
		a.homeFolderContentsLoading = make(map[string]bool)
	}
	if a.homeFolderContentsLastScan == nil {
		a.homeFolderContentsLastScan = make(map[string]time.Time)
	}
	if a.homeFolderContentsLatest == nil {
		a.homeFolderContentsLatest = make(map[string]uint64)
	}
	if a.homeFolderContentsLoading[folderID] {
		return
	}
	if lastScan := a.homeFolderContentsLastScan[folderID]; !lastScan.IsZero() && time.Since(lastScan) < homeFolderContentsRefreshInterval {
		return
	}
	a.homeFolderContentsGeneration++
	generation := a.homeFolderContentsGeneration
	a.homeFolderContentsLatest[folderID] = generation
	a.homeFolderContentsLoading[folderID] = true
	a.homeFolderContentsLastScan[folderID] = time.Now()
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
	if a.homeFolderContentsLatest[result.folderID] != result.generation {
		return
	}
	delete(a.homeFolderContentsLoading, result.folderID)
	if a.homeFolderContentsLastScan != nil {
		a.homeFolderContentsLastScan[result.folderID] = time.Now()
	}
	snapshot, _ := a.homeFolderSnapshot()
	if !hasHomeFolder(snapshot, result.folderID) {
		return
	}
	if result.err != nil {
		delete(a.homeFolderContents, result.folderID)
		if a.homeSelectedFolderID == result.folderID {
			a.setStatusErrorMessage(i18n.Message{ID: "home.folder.error.content_failed"}, result.err, notificationToast)
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
			rows = append(rows, homeFolderTreeRow{folder: folder, depth: depth, hasChildren: len(children[folder.ID]) != 0})
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
	a.modal.ShowInputMessage("home.folder.create",
		i18n.Message{ID: "home.folder.dialog.create.title"},
		i18n.Message{ID: "home.folder.dialog.create.body", Args: i18n.Args{"Path": parentPath}},
		a.text(i18n.Message{ID: "home.folder.default_name"}),
		i18n.Message{ID: "home.folder.dialog.name_hint"}, func(action string) {
			if action == "cancel" {
				a.modal.Hide()
				return
			}
			if action != "confirm" {
				return
			}
			if err := a.createHomeFolder(parentID, a.modal.Input.Text()); err != nil {
				a.setStatusErrorMessage(i18n.Message{ID: "home.folder.error.create_failed"}, err, notificationToast)
				return
			}
			a.modal.Hide()
		}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.create"}, Style: modalDialogActionPrimary})
}

func (a *App) showRenameHomeFolderDialog(folderID string) {
	snapshot, _, err := a.currentHomeFolderSnapshot()
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "home.folder.error.read_failed"}, err, notificationToast)
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
		a.setStatusMessage(i18n.Message{ID: "home.folder.error.folder_missing"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowInputMessage("home.folder.rename",
		i18n.Message{ID: "home.folder.dialog.rename.title"},
		i18n.Message{ID: "home.folder.dialog.rename.body", Args: i18n.Args{"Name": name}},
		name,
		i18n.Message{ID: "home.folder.dialog.name_hint"}, func(action string) {
			if action == "cancel" {
				a.modal.Hide()
				return
			}
			if action != "confirm" {
				return
			}
			if err := a.renameHomeFolder(folderID, a.modal.Input.Text()); err != nil {
				a.setStatusErrorMessage(i18n.Message{ID: "home.folder.error.rename_failed"}, err, notificationToast)
				return
			}
			a.modal.Hide()
		}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.save"}, Style: modalDialogActionPrimary})
}

func (a *App) showRemoveHomeFolderConfirm(folderID string) {
	snapshot, _, err := a.currentHomeFolderSnapshot()
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "home.folder.error.read_failed"}, err, notificationToast)
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
		a.setStatusMessage(i18n.Message{ID: "home.folder.error.folder_missing"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowConfirmMessage("home.folder.remove",
		i18n.Message{ID: "home.folder.dialog.remove.title"},
		i18n.Message{ID: "home.folder.dialog.remove.body", Args: i18n.Args{"Name": name}}, modalDialogConfirmStyle{}, func(action string) {
			if action == "cancel" {
				a.modal.Hide()
				return
			}
			if action != "confirm" {
				return
			}
			if err := a.removeHomeFolder(folderID); err != nil {
				a.setStatusErrorMessage(i18n.Message{ID: "home.folder.error.remove_failed"}, err, notificationToast)
				return
			}
			a.modal.Hide()
		}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "home.folder.action.remove"}, Style: modalDialogActionDanger})
}
