package desktopui

import (
	"fmt"
	"time"

	"glance/internal/workspace"
)

func (r *desktopRuntime) startWorkspaceWriter() {
	if r == nil {
		return
	}
	r.workspaceQueueMu.Lock()
	r.mu.Lock()
	if r.workspaceStore == nil || r.workspaceWriteJobs != nil || r.workspaceWriterStopped {
		r.mu.Unlock()
		r.workspaceQueueMu.Unlock()
		return
	}
	r.workspaceWriteJobs = make(chan workspaceWriteJob, 128)
	r.workspaceWriteResults = make(chan error, 1)
	r.workspaceWriterDone = make(chan struct{})
	jobs, done := r.workspaceWriteJobs, r.workspaceWriterDone
	r.mu.Unlock()
	r.workspaceQueueMu.Unlock()
	go func() {
		defer close(done)
		for job := range jobs {
			if job.barrier != nil {
				close(job.barrier)
				continue
			}
			var err error
			if job.recents != nil {
				err = r.workspaceStore.SaveRecents(job.recents)
			} else if job.saveFavorite {
				err = r.workspaceStore.SaveFavorite(job.favoritePath, job.favoriteValue)
			} else {
				err = r.writeWorkspaceSnapshot(job)
			}
			r.publishWorkspaceWriteResult(err)
			r.invalidateWindows()
		}
	}()
}

func (r *desktopRuntime) drainWorkspaceWriter() {
	if r == nil {
		return
	}
	r.workspaceQueueMu.Lock()
	if r.workspaceWriterStopped {
		r.workspaceQueueMu.Unlock()
		return
	}
	r.mu.Lock()
	jobs, done := r.workspaceWriteJobs, r.workspaceWriterDone
	r.mu.Unlock()
	if jobs == nil || done == nil {
		r.workspaceQueueMu.Unlock()
		return
	}
	barrier := make(chan struct{})
	jobs <- workspaceWriteJob{barrier: barrier}
	r.workspaceQueueMu.Unlock()
	<-barrier
}

func (r *desktopRuntime) publishWorkspaceWriteResult(err error) {
	if r == nil || r.workspaceWriteResults == nil {
		return
	}
	select {
	case r.workspaceWriteResults <- err:
		return
	default:
	}
	select {
	case <-r.workspaceWriteResults:
	default:
	}
	select {
	case r.workspaceWriteResults <- err:
	default:
	}
}

func (r *desktopRuntime) writeWorkspaceSnapshot(job workspaceWriteJob) error {
	state := job.state
	if state == nil || state.closed.Load() {
		return nil
	}
	token, ok := state.beginOperationWait(sessionOperationSnapshot)
	if !ok {
		return nil
	}
	defer state.finishOperation(token)
	payload, err := snapshotState(state)
	if err != nil {
		return err
	}
	if r.workspaceStore == nil {
		return fmt.Errorf("工作区数据库不可用")
	}
	r.workspaceDBMu.Lock()
	defer r.workspaceDBMu.Unlock()
	if state.closed.Load() {
		return nil
	}
	return r.workspaceStore.SaveDocument(workspace.Document{ID: job.documentID, Format: job.format, Payload: payload, ViewState: job.viewState, Dirty: sessionDirty(state), UpdatedAt: time.Now().UTC()})
}

func (r *desktopRuntime) shutdown() {
	if r == nil {
		return
	}
	r.shutdownOnce.Do(func() {
		r.workspaceWriterStopOnce.Do(func() {
			r.workspaceQueueMu.Lock()
			r.workspaceWriterStopped = true
			r.mu.Lock()
			if r.workspaceWriteJobs != nil {
				close(r.workspaceWriteJobs)
			}
			done := r.workspaceWriterDone
			r.mu.Unlock()
			r.workspaceQueueMu.Unlock()
			if done != nil {
				<-done
			}
		})
		r.mu.Lock()
		sessions := make([]*sharedDocumentSession, 0, len(r.sessions))
		for _, session := range r.sessions {
			sessions = append(sessions, session)
		}
		r.sessions = make(map[string]*sharedDocumentSession)
		r.sessionsByCanonicalPath = make(map[string]string)
		store := r.workspaceStore
		r.workspaceStore = nil
		r.collection = nil
		r.mu.Unlock()
		for _, session := range sessions {
			closeSharedDocumentSession(session)
		}
		if store != nil {
			_ = store.Close()
		}
		r.completionOnce.Do(func() {
			if r.completion != nil {
				close(r.completion)
			}
		})
	})
}
