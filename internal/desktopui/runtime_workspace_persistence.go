package desktopui

import (
	"context"
	"errors"
	"fmt"
	"glance/internal/diagramrender"
	"io"
	"log"
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
				if err := r.flushWorkspacePreferences(); err != nil {
					r.publishWorkspaceWriteResult(err)
				}
				close(job.barrier)
				continue
			}
			var err error
			if job.homeFolderSnapshot != nil {
				result := homeFolderWriteResult{
					revision: job.homeFolderRevision,
					snapshot: cloneHomeFolderRuntimeSnapshot(*job.homeFolderSnapshot),
					err:      r.writeHomeFolderSnapshot(*job.homeFolderSnapshot),
				}
				r.finishHomeFolderWrite(result)
				publishHomeFolderWriteResult(job.homeFolderResults, result)
				err = result.err
			} else if job.recents != nil {
				err = r.workspaceStore.SaveRecents(job.recents)
				publishHomeRecentWriteResult(job.homeRecentResults, homeRecentWriteResult{generation: job.homeRecentRevision, err: err})
			} else if job.saveFavorite {
				err = r.workspaceStore.SaveFavorite(job.favoritePath, job.favoriteValue)
			} else if !job.savePreferences {
				err = r.writeWorkspaceSnapshot(job)
			}
			if preferenceErr := r.flushWorkspacePreferences(); preferenceErr != nil {
				err = errors.Join(err, preferenceErr)
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

func publishHomeFolderWriteResult(results chan homeFolderWriteResult, result homeFolderWriteResult) {
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
	if job.format == "pdf" {
		return r.persistPDFWorkspaceSnapshot(r.collection, state, job.tabID, job.viewState)
	}
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

// persistPDFWorkspaceSnapshot serializes checkpoints for the shared PDF model.
// It captures a changed immutable revision reader while the adapter is
// read-locked, then releases that lock before SQLite I/O. The reader leases any
// mapped backing until the write finishes. The per-session mutex remains held
// through the database update so windows cannot commit checkpoints out of order.
func (r *desktopRuntime) persistPDFWorkspaceSnapshot(collection *workspace.Collection, state *documentSessionState, tabID string, viewState []byte) (err error) {
	if r == nil || collection == nil || state == nil || state.sharedDocumentSession == nil {
		return fmt.Errorf("工作区数据库不可用")
	}
	if state.closed.Load() {
		return nil
	}
	if r.workspaceStore == nil {
		return fmt.Errorf("工作区数据库不可用")
	}
	shared := state.sharedDocumentSession
	shared.pdfPersistenceMu.Lock()
	defer shared.pdfPersistenceMu.Unlock()

	var payload io.ReadCloser
	defer func() {
		if payload != nil {
			err = errors.Join(err, payload.Close())
		}
	}()
	var payloadSize int64
	var revision uint64
	var dirty bool
	writeSnapshot := false
	unlockAdapter := lockSessionAdapterRead(state)
	if state.pdfDoc == nil {
		unlockAdapter()
		return fmt.Errorf("PDF 文档不可用")
	}
	info := state.pdfDoc.Info()
	dirty = info.Dirty
	if !shared.pdfSnapshotPresent || shared.pdfPersistedRevision != info.Revision {
		var err error
		payload, payloadSize, revision, dirty, err = state.pdfDoc.SnapshotReaderWithRevision()
		if err != nil {
			unlockAdapter()
			return err
		}
		writeSnapshot = true
	}
	unlockAdapter()

	r.workspaceDBMu.Lock()
	defer r.workspaceDBMu.Unlock()
	if state.closed.Load() {
		return nil
	}
	if writeSnapshot {
		if err := collection.UpdatePDFSnapshotStream(tabID, payload, payloadSize, viewState, dirty); err != nil {
			return err
		}
		shared.pdfSnapshotPresent = true
		shared.pdfPersistedRevision = revision
		return nil
	}
	return collection.UpdatePDFMetadata(tabID, viewState, dirty)
}

func (r *desktopRuntime) writeHomeFolderSnapshot(snapshot workspace.HomeFolderSnapshot) error {
	if r == nil {
		return fmt.Errorf("工作区数据库不可用")
	}
	r.mu.Lock()
	store := r.workspaceStore
	r.mu.Unlock()
	if store == nil {
		return fmt.Errorf("工作区数据库不可用")
	}
	r.workspaceDBMu.Lock()
	defer r.workspaceDBMu.Unlock()
	return store.SaveHomeFolderSnapshot(snapshot)
}

func (r *desktopRuntime) shutdown() {
	if r == nil {
		return
	}
	r.shutdownOnce.Do(func() {
		defer diagramrender.CloseDefault()
		r.mu.Lock()
		r.shuttingDown = true
		r.quitRequested = true
		r.trayReady = false
		cancelTray := r.trayCancel
		r.trayCancel = nil
		trayDone := r.trayDone
		r.mu.Unlock()
		if cancelTray != nil {
			cancelTray()
		}
		if trayDone != nil {
			<-trayDone
		}
		if r.applicationEventCancel != nil {
			r.applicationEventCancel()
			r.applicationEventCancel = nil
		}
		r.mu.Lock()
		mcpIPC := r.mcpIPC
		r.mcpIPC = nil
		r.mu.Unlock()
		if mcpIPC != nil {
			if err := mcpIPC.Close(); err != nil {
				log.Printf("stop desktop MCP IPC endpoint: %v", err)
			}
		}
		r.closeAgent()
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
		applicationRuntime := r.applicationRuntime
		r.applicationRuntime = nil
		r.workspaceStore = nil
		r.collection = nil
		r.mu.Unlock()
		for _, session := range sessions {
			closeSharedDocumentSession(session)
		}
		if applicationRuntime != nil {
			applicationStore := applicationRuntime.WorkspaceStore()
			_ = applicationRuntime.Close(context.Background())
			if store != nil && store != applicationStore {
				_ = store.Close()
			}
		} else if store != nil {
			_ = store.Close()
		}
		if r.fonts != nil {
			_ = r.fonts.Close()
		}
		r.completionOnce.Do(func() {
			if r.completion != nil {
				close(r.completion)
			}
		})
	})
}

// Coalesce personalization writes so dragging never blocks the window loop.
func (r *desktopRuntime) queueWorkspacePreference(key, value string) {
	r.startWorkspaceWriter()
	r.workspaceQueueMu.Lock()
	defer r.workspaceQueueMu.Unlock()
	if r.workspaceWriterStopped || r.workspaceWriteJobs == nil {
		return
	}
	r.workspacePreferenceMu.Lock()
	if r.workspacePreferences == nil {
		r.workspacePreferences = make(map[string]string)
	}
	r.workspacePreferences[key] = value
	r.workspacePreferenceMu.Unlock()
	select {
	case r.workspaceWriteJobs <- workspaceWriteJob{savePreferences: true}:
	default:
	}
	// A full queue already has work that will flush the pending values.
}

func (r *desktopRuntime) flushWorkspacePreferences() error {
	r.workspacePreferenceMu.Lock()
	values := r.workspacePreferences
	r.workspacePreferences = nil
	r.workspacePreferenceMu.Unlock()
	var failures error
	for key, value := range values {
		if err := r.workspaceStore.SetPreference(key, value); err != nil {
			failures = errors.Join(failures, fmt.Errorf("save personalization: %w", err))
			r.workspacePreferenceMu.Lock()
			if r.workspacePreferences == nil {
				r.workspacePreferences = make(map[string]string)
			}
			if _, newer := r.workspacePreferences[key]; !newer {
				r.workspacePreferences[key] = value
			}
			r.workspacePreferenceMu.Unlock()
		}
	}
	return failures
}
