package workspace

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// HomeFolder is a logical node in the workspace's home navigation tree.
// ParentID and SourcePath are empty for ordinary root folders.
type HomeFolder struct {
	ID         string
	ParentID   string
	Name       string
	SortOrder  int
	SourcePath string
}

// HomeFolderAssignment records the single logical folder that contains a
// recent document. Path is canonicalized by desktopui before it is stored.
type HomeFolderAssignment struct {
	Path     string
	FolderID string
}

// HomeFolderSnapshot is the complete, replaceable home folder state.
type HomeFolderSnapshot struct {
	Folders     []HomeFolder
	Assignments []HomeFolderAssignment
}

// ValidateHomeFolderSnapshot rejects malformed or ambiguous folder state.
func ValidateHomeFolderSnapshot(snapshot HomeFolderSnapshot) error {
	foldersByID := make(map[string]HomeFolder, len(snapshot.Folders))
	for _, folder := range snapshot.Folders {
		if folder.ID == "" {
			return errors.New("home folder ID is required")
		}
		if _, exists := foldersByID[folder.ID]; exists {
			return fmt.Errorf("duplicate home folder ID %q", folder.ID)
		}
		if folder.Name != strings.TrimSpace(folder.Name) {
			return fmt.Errorf("home folder %q name is not trimmed", folder.ID)
		}
		if _, err := normalizeHomeFolderName(folder.Name); err != nil {
			return fmt.Errorf("home folder %q: %w", folder.ID, err)
		}
		if folder.SourcePath != "" {
			if strings.TrimSpace(folder.SourcePath) == "" {
				return fmt.Errorf("home folder %q source path is empty", folder.ID)
			}
			if folder.ParentID != "" {
				return fmt.Errorf("home folder %q has a source path but is not a root folder", folder.ID)
			}
		}
		foldersByID[folder.ID] = folder
	}

	siblingNames := make(map[string][]string, len(snapshot.Folders))
	sourcePaths := make(map[string]string)
	for _, folder := range snapshot.Folders {
		if folder.ParentID != "" {
			if _, exists := foldersByID[folder.ParentID]; !exists {
				return fmt.Errorf("home folder %q has missing parent %q", folder.ID, folder.ParentID)
			}
		}
		for _, name := range siblingNames[folder.ParentID] {
			if strings.EqualFold(name, folder.Name) {
				return fmt.Errorf("home folder %q duplicates sibling name %q", folder.ID, folder.Name)
			}
		}
		siblingNames[folder.ParentID] = append(siblingNames[folder.ParentID], folder.Name)
		if folder.SourcePath != "" {
			if previous, exists := sourcePaths[folder.SourcePath]; exists {
				return fmt.Errorf("home folders %q and %q have the same source path", previous, folder.ID)
			}
			sourcePaths[folder.SourcePath] = folder.ID
		}
	}

	// Parent links form a functional graph. Marking each traversed chain avoids
	// recursion and catches cycles without depending on the number of folders.
	state := make(map[string]uint8, len(foldersByID))
	for _, folder := range snapshot.Folders {
		current := folder.ID
		chain := make([]string, 0, 8)
		for current != "" && state[current] == 0 {
			state[current] = 1
			chain = append(chain, current)
			current = foldersByID[current].ParentID
		}
		if current != "" && state[current] == 1 {
			return fmt.Errorf("home folder tree contains a parent cycle at %q", current)
		}
		for _, id := range chain {
			state[id] = 2
		}
	}

	paths := make(map[string]struct{}, len(snapshot.Assignments))
	for _, assignment := range snapshot.Assignments {
		if assignment.Path == "" {
			return errors.New("home folder document path is required")
		}
		if assignment.FolderID == "" {
			return fmt.Errorf("home folder assignment for %q has no folder ID", assignment.Path)
		}
		if _, exists := foldersByID[assignment.FolderID]; !exists {
			return fmt.Errorf("home folder assignment for %q references missing folder %q", assignment.Path, assignment.FolderID)
		}
		if _, exists := paths[assignment.Path]; exists {
			return fmt.Errorf("home folder document path %q is assigned more than once", assignment.Path)
		}
		paths[assignment.Path] = struct{}{}
	}
	return nil
}

// AddFolder appends a normalized folder after its siblings.
func (snapshot HomeFolderSnapshot) AddFolder(folder HomeFolder) (HomeFolderSnapshot, error) {
	if folder.ID == "" {
		return HomeFolderSnapshot{}, errors.New("home folder ID is required")
	}
	name, err := normalizeHomeFolderName(folder.Name)
	if err != nil {
		return HomeFolderSnapshot{}, err
	}
	folder.Name = name
	maxOrder := -1
	for _, sibling := range snapshot.Folders {
		if sibling.ParentID != folder.ParentID {
			continue
		}
		if sibling.SortOrder > maxOrder {
			maxOrder = sibling.SortOrder
		}
	}
	folder.SortOrder = maxOrder + 1
	result := cloneHomeFolderSnapshot(snapshot)
	result.Folders = append(result.Folders, folder)
	if err := ValidateHomeFolderSnapshot(result); err != nil {
		return HomeFolderSnapshot{}, err
	}
	return result, nil
}

// RenameFolder changes a logical label without changing its source path.
func (snapshot HomeFolderSnapshot) RenameFolder(id, name string) (HomeFolderSnapshot, error) {
	name, err := normalizeHomeFolderName(name)
	if err != nil {
		return HomeFolderSnapshot{}, err
	}
	result := cloneHomeFolderSnapshot(snapshot)
	found := false
	for i := range result.Folders {
		if result.Folders[i].ID != id {
			continue
		}
		result.Folders[i].Name = name
		found = true
		break
	}
	if !found {
		return HomeFolderSnapshot{}, fmt.Errorf("home folder %q does not exist", id)
	}
	if err := ValidateHomeFolderSnapshot(result); err != nil {
		return HomeFolderSnapshot{}, err
	}
	return result, nil
}

// RemoveFolder removes a node, unassigns its direct documents, and promotes
// its children into its parent while keeping their relative order.
func (snapshot HomeFolderSnapshot) RemoveFolder(id string) (HomeFolderSnapshot, error) {
	var removed HomeFolder
	found := false
	for _, folder := range snapshot.Folders {
		if folder.ID == id {
			removed = folder
			found = true
			break
		}
	}
	if !found {
		return HomeFolderSnapshot{}, fmt.Errorf("home folder %q does not exist", id)
	}

	result := HomeFolderSnapshot{
		Folders:     make([]HomeFolder, 0, len(snapshot.Folders)-1),
		Assignments: make([]HomeFolderAssignment, 0, len(snapshot.Assignments)),
	}
	maxOrder := -1
	for _, folder := range snapshot.Folders {
		if folder.ID == id {
			continue
		}
		if folder.ParentID == removed.ParentID && folder.SortOrder > maxOrder {
			maxOrder = folder.SortOrder
		}
		result.Folders = append(result.Folders, folder)
	}
	promoted := make([]HomeFolder, 0)
	for i := range result.Folders {
		if result.Folders[i].ParentID == id {
			result.Folders[i].ParentID = removed.ParentID
			promoted = append(promoted, result.Folders[i])
		}
	}
	sort.SliceStable(promoted, func(i, j int) bool {
		if promoted[i].SortOrder != promoted[j].SortOrder {
			return promoted[i].SortOrder < promoted[j].SortOrder
		}
		return promoted[i].ID < promoted[j].ID
	})
	promotedSortOrders := make(map[string]int, len(promoted))
	for _, folder := range promoted {
		maxOrder++
		promotedSortOrders[folder.ID] = maxOrder
	}
	for i := range result.Folders {
		if sortOrder, isPromoted := promotedSortOrders[result.Folders[i].ID]; isPromoted {
			result.Folders[i].SortOrder = sortOrder
		}
	}
	for _, assignment := range snapshot.Assignments {
		if assignment.FolderID != id {
			result.Assignments = append(result.Assignments, assignment)
		}
	}
	if err := ValidateHomeFolderSnapshot(result); err != nil {
		return HomeFolderSnapshot{}, err
	}
	return result, nil
}

// AssignDocument assigns a path to one folder, replacing any prior group.
func (snapshot HomeFolderSnapshot) AssignDocument(path, folderID string) (HomeFolderSnapshot, error) {
	if path == "" {
		return HomeFolderSnapshot{}, errors.New("home folder document path is required")
	}
	result := cloneHomeFolderSnapshot(snapshot)
	if err := ValidateHomeFolderSnapshot(result); err != nil {
		return HomeFolderSnapshot{}, err
	}
	if _, exists := findHomeFolder(result.Folders, folderID); !exists {
		return HomeFolderSnapshot{}, fmt.Errorf("home folder %q does not exist", folderID)
	}
	for i := range result.Assignments {
		if result.Assignments[i].Path == path {
			result.Assignments[i].FolderID = folderID
			return result, nil
		}
	}
	result.Assignments = append(result.Assignments, HomeFolderAssignment{Path: path, FolderID: folderID})
	return result, nil
}

func normalizeHomeFolderName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("home folder name is required")
	}
	if strings.ContainsAny(name, `/\`) {
		return "", errors.New("home folder name cannot contain path separators")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", errors.New("home folder name cannot contain control characters")
		}
	}
	return name, nil
}

func cloneHomeFolderSnapshot(snapshot HomeFolderSnapshot) HomeFolderSnapshot {
	return HomeFolderSnapshot{
		Folders:     append([]HomeFolder(nil), snapshot.Folders...),
		Assignments: append([]HomeFolderAssignment(nil), snapshot.Assignments...),
	}
}

func findHomeFolder(folders []HomeFolder, id string) (HomeFolder, bool) {
	for _, folder := range folders {
		if folder.ID == id {
			return folder, true
		}
	}
	return HomeFolder{}, false
}

func orderedHomeFolders(folders []HomeFolder) []HomeFolder {
	children := make(map[string][]HomeFolder)
	for _, folder := range folders {
		children[folder.ParentID] = append(children[folder.ParentID], folder)
	}
	for parentID := range children {
		sort.Slice(children[parentID], func(i, j int) bool {
			if children[parentID][i].SortOrder != children[parentID][j].SortOrder {
				return children[parentID][i].SortOrder < children[parentID][j].SortOrder
			}
			return children[parentID][i].ID < children[parentID][j].ID
		})
	}
	ordered := make([]HomeFolder, 0, len(folders))
	var appendChildren func(parentID string)
	appendChildren = func(parentID string) {
		for _, child := range children[parentID] {
			ordered = append(ordered, child)
			appendChildren(child.ID)
		}
	}
	appendChildren("")
	return ordered
}
