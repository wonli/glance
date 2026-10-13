package desktopui

import (
	"errors"
	"fmt"

	"glance/internal/workspace"
)

type homeFolderWriteResult struct {
	revision uint64
	snapshot workspace.HomeFolderSnapshot
	err      error
}

func (r *desktopRuntime) loadHomeFolderState() error {
	if r == nil {
		return errors.New("desktop runtime is unavailable")
	}
	r.mu.Lock()
	if r.homeFolderLoaded {
		err := r.homeFolderLoadErr
		r.mu.Unlock()
		return err
	}
	store := r.workspaceStore
	r.mu.Unlock()
	if store == nil {
		return errors.New("workspace database is unavailable")
	}
	r.workspaceDBMu.Lock()
	snapshot, err := store.LoadHomeFolderSnapshot()
	r.workspaceDBMu.Unlock()

	r.mu.Lock()
	if !r.homeFolderLoaded {
		r.homeFolderLoaded = true
		r.homeFolderLoadErr = err
		if err == nil {
			r.homeFolderWorking = cloneHomeFolderRuntimeSnapshot(snapshot)
			r.homeFolderCommitted = cloneHomeFolderRuntimeSnapshot(snapshot)
			r.homeFolderRevision = 0
			r.homeFolderCommittedRev = 0
		}
	}
	err = r.homeFolderLoadErr
	r.mu.Unlock()
	return err
}

func (r *desktopRuntime) homeFolderSnapshot() (workspace.HomeFolderSnapshot, uint64) {
	if r == nil {
		return workspace.HomeFolderSnapshot{}, 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneHomeFolderRuntimeSnapshot(r.homeFolderWorking), r.homeFolderRevision
}

func (r *desktopRuntime) homeFolderSelectionState(folderID string) (workspace.HomeFolderSnapshot, bool) {
	if r == nil {
		return workspace.HomeFolderSnapshot{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, pending := r.homeFolderPendingRemove[folderID]
	return cloneHomeFolderRuntimeSnapshot(r.homeFolderWorking), pending
}

func (r *desktopRuntime) homeFolderStateError() error {
	if r == nil {
		return errors.New("desktop runtime is unavailable")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.homeFolderLoaded {
		return errors.New("workspace folder state is not loaded")
	}
	return r.homeFolderLoadErr
}

func (r *desktopRuntime) beginHomeFolderRemoval(folderID string) bool {
	if r == nil || folderID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.homeFolderPendingRemove == nil {
		r.homeFolderPendingRemove = make(map[string]uint64)
	}
	if _, pending := r.homeFolderPendingRemove[folderID]; pending {
		return false
	}
	r.homeFolderPendingRemove[folderID] = 0
	return true
}

func (r *desktopRuntime) setHomeFolderRemovalRevision(folderID string, revision uint64) {
	if r == nil || folderID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.homeFolderPendingRemove == nil {
		return
	}
	if revision == 0 {
		delete(r.homeFolderPendingRemove, folderID)
		return
	}
	r.homeFolderPendingRemove[folderID] = revision
	r.settleHomeFolderRemovalsLocked()
}

func (r *desktopRuntime) settleHomeFolderRemovalsLocked() {
	if len(r.homeFolderPendingRemove) == 0 || r.homeFolderCompletedRev < r.homeFolderRevision {
		return
	}
	for _, revision := range r.homeFolderPendingRemove {
		if revision == 0 {
			return
		}
	}
	clear(r.homeFolderPendingRemove)
}

func (r *desktopRuntime) cancelHomeFolderRemoval(folderID string) {
	if r == nil || folderID == "" {
		return
	}
	r.mu.Lock()
	delete(r.homeFolderPendingRemove, folderID)
	r.mu.Unlock()
}

func (r *desktopRuntime) homeFolderRemovalPending(folderID string) bool {
	if r == nil || folderID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, pending := r.homeFolderPendingRemove[folderID]
	return pending
}

func (r *desktopRuntime) updateHomeFolderSnapshot(next workspace.HomeFolderSnapshot, results chan homeFolderWriteResult) (uint64, error) {
	_, revision, _, err := r.mutateHomeFolderSnapshot(func(workspace.HomeFolderSnapshot) (workspace.HomeFolderSnapshot, bool, error) {
		return cloneHomeFolderRuntimeSnapshot(next), true, nil
	}, results)
	return revision, err
}

// mutateHomeFolderSnapshot applies a folder operation to the latest shared
// snapshot while holding the same queue lock used to order writer jobs. This
// prevents two windows that started from the same revision from overwriting
// one another's folder changes.
func (r *desktopRuntime) mutateHomeFolderSnapshot(mutate func(workspace.HomeFolderSnapshot) (workspace.HomeFolderSnapshot, bool, error), results chan homeFolderWriteResult) (workspace.HomeFolderSnapshot, uint64, bool, error) {
	if r == nil {
		return workspace.HomeFolderSnapshot{}, 0, false, errors.New("desktop runtime is unavailable")
	}
	if results == nil {
		return workspace.HomeFolderSnapshot{}, 0, false, errors.New("home folder write result channel is required")
	}
	if mutate == nil {
		return workspace.HomeFolderSnapshot{}, 0, false, errors.New("home folder mutation is required")
	}
	r.startWorkspaceWriter()
	r.workspaceQueueMu.Lock()
	defer r.workspaceQueueMu.Unlock()
	r.mu.Lock()
	if !r.homeFolderLoaded {
		r.mu.Unlock()
		return workspace.HomeFolderSnapshot{}, 0, false, errors.New("workspace folder state is not loaded")
	}
	if r.homeFolderLoadErr != nil {
		err := fmt.Errorf("workspace folder state cannot be overwritten: %w", r.homeFolderLoadErr)
		r.mu.Unlock()
		return workspace.HomeFolderSnapshot{}, 0, false, err
	}
	jobs := r.workspaceWriteJobs
	if jobs == nil || r.workspaceWriterStopped {
		r.mu.Unlock()
		return workspace.HomeFolderSnapshot{}, 0, false, errors.New("workspace writer is unavailable")
	}
	current := cloneHomeFolderRuntimeSnapshot(r.homeFolderWorking)
	next, changed, err := mutate(current)
	if err != nil {
		r.mu.Unlock()
		return workspace.HomeFolderSnapshot{}, 0, false, err
	}
	if !changed {
		revision := r.homeFolderRevision
		r.mu.Unlock()
		return current, revision, false, nil
	}
	if err := workspace.ValidateHomeFolderSnapshot(next); err != nil {
		r.mu.Unlock()
		return workspace.HomeFolderSnapshot{}, 0, false, err
	}
	revision := r.homeFolderRevision + 1
	snapshot := cloneHomeFolderRuntimeSnapshot(next)
	r.homeFolderRevision = revision
	r.homeFolderWorking = cloneHomeFolderRuntimeSnapshot(snapshot)
	r.mu.Unlock()
	jobs <- workspaceWriteJob{
		homeFolderSnapshot: &snapshot,
		homeFolderRevision: revision,
		homeFolderResults:  results,
	}
	return snapshot, revision, true, nil
}

func (r *desktopRuntime) finishHomeFolderWrite(result homeFolderWriteResult) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if result.revision > r.homeFolderCompletedRev {
		r.homeFolderCompletedRev = result.revision
	}
	if result.err == nil {
		if result.revision > r.homeFolderCommittedRev {
			r.homeFolderCommittedRev = result.revision
			r.homeFolderCommitted = cloneHomeFolderRuntimeSnapshot(result.snapshot)
		}
		r.settleHomeFolderRemovalsLocked()
		return false
	}
	if result.revision != r.homeFolderRevision {
		r.settleHomeFolderRemovalsLocked()
		return false
	}
	r.homeFolderWorking = cloneHomeFolderRuntimeSnapshot(r.homeFolderCommitted)
	r.settleHomeFolderRemovalsLocked()
	return true
}

func cloneHomeFolderRuntimeSnapshot(snapshot workspace.HomeFolderSnapshot) workspace.HomeFolderSnapshot {
	return workspace.HomeFolderSnapshot{
		Folders:     append([]workspace.HomeFolder(nil), snapshot.Folders...),
		Assignments: append([]workspace.HomeFolderAssignment(nil), snapshot.Assignments...),
	}
}
