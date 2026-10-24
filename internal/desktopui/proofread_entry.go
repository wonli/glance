package desktopui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"glance/internal/i18n"
	"glance/internal/proofhistory"
	"glance/internal/workspace"
)

type proofreadingEntryCandidate struct {
	project    workspace.ProofreadingProject
	payload    proofreadingProjectPayload
	hasHistory bool
}

type proofreadingEntryResult struct {
	request    proofreadingWindowRequest
	candidates []proofreadingEntryCandidate
	identity   proofhistory.Identity
	err        error
}

type proofreadingPDFEntryState struct {
	results chan proofreadingEntryResult
	busy    bool
	pending *proofreadingEntryResult
}

func (a *App) ensureProofreadingPDFEntry() *proofreadingPDFEntryState {
	if a.proofEntry == nil {
		a.proofEntry = &proofreadingPDFEntryState{results: make(chan proofreadingEntryResult, 2)}
	}
	return a.proofEntry
}

// closePendingProofreadingPDFEntry is called after managed workers stop so it
// can release snapshots still held by a dialog or an undelivered result.
func (a *App) closePendingProofreadingPDFEntry() {
	entry := a.proofEntry
	if entry == nil {
		return
	}
	if entry.pending != nil && entry.pending.request.sourceSnapshot != nil {
		_ = entry.pending.request.sourceSnapshot.Close()
	}
	entry.pending = nil
	for {
		select {
		case result := <-entry.results:
			if result.request.sourceSnapshot != nil {
				_ = result.request.sourceSnapshot.Close()
			}
		default:
			entry.busy = false
			return
		}
	}
}

func inspectProofreadingPDFEntry(store *workspace.Store, request proofreadingWindowRequest, source io.ReadCloser) proofreadingEntryResult {
	result := proofreadingEntryResult{request: request}
	defer source.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, source); err != nil {
		result.err = fmt.Errorf("读取 PDF 快照失败：%w", err)
		return result
	}
	sourceHash := hex.EncodeToString(hash.Sum(nil))

	projects, err := store.ListProofreadingProjects()
	if err != nil {
		result.err = err
		return result
	}
	for _, project := range projects {
		storedHash := project.SourceAHash
		if storedHash == "" || storedHash != sourceHash {
			continue
		}
		full, found, loadErr := store.LoadProofreadingProject(project.ID)
		if loadErr != nil {
			result.err = loadErr
			return result
		}
		if !found {
			continue
		}
		var payload proofreadingProjectPayload
		if err = json.Unmarshal(full.Payload, &payload); err != nil {
			result.err = fmt.Errorf("读取校对任务失败：%w", err)
			return result
		}
		repo := proofhistory.Repository{Directory: full.Directory}
		payload.Head, _ = repo.Head(context.Background())
		if pushed, found, e := store.Preference("proofreading.pushed." + full.ID); e == nil && found {
			payload.Pushed = pushed
		}
		if payload.Draft.Model != nil {
			payload.Draft.Model.Next = max(payload.Draft.Model.Next, repo.AllocationWatermark())
		}
		candidate := proofreadingEntryCandidate{project: full, payload: payload, hasHistory: len(payload.Snapshots) > 0}
		if gitErr := (proofhistory.Repository{Directory: full.Directory}).Available(context.Background()); gitErr == nil {
			if _, statErr := os.Stat(filepath.Join(full.Directory, ".git")); statErr == nil {
				entries, listErr := (proofhistory.Repository{Directory: full.Directory}).List(context.Background(), 0)
				candidate.hasHistory = listErr != nil || len(entries) > 0
				payload.Snapshots = map[int]string{}
				for page, pageHash := range payload.Confirmed {
					old, readErr := repo.ReadPage(context.Background(), "HEAD", proofreadingPageID(page))
					if readErr == nil && old.ProjectID == full.ID && old.ContentHash() == pageHash {
						payload.Snapshots[page] = pageHash
					}
				}
			}
		}
		candidate.payload = payload
		result.candidates = append(result.candidates, candidate)
	}

	identityData, _, err := store.Preference("proofreading.identity")
	if err != nil {
		result.err = err
		return result
	}
	if identityData != "" {
		if err = json.Unmarshal([]byte(identityData), &result.identity); err != nil {
			result.identity = proofhistory.Identity{}
		}
	}
	return result
}
func (a *App) processProofreadingPDFEntryResult() {
	entry := a.proofEntry
	if entry == nil {
		return
	}
	select {
	case result := <-entry.results:
		entry.busy = false
		if result.err != nil {
			if result.request.sourceSnapshot != nil {
				_ = result.request.sourceSnapshot.Close()
			}
			a.setStatusErrorMessage(i18n.Message{ID: "pdf.error.operation_failed"}, result.err, notificationToast)
			return
		}
		a.cacheProofreadingIdentity(result.identity)
		entry.pending = &result
		a.chooseProofreadingEntryCandidate()
	default:
	}
}

func (a *App) cacheProofreadingIdentity(identity proofhistory.Identity) {
	if runtime := a.desktopRuntime; runtime != nil {
		runtime.mu.Lock()
		runtime.proofIdentity = identity
		runtime.proofIdentityReady = true
		runtime.mu.Unlock()
	}
}

func (a *App) chooseProofreadingEntryCandidate() {
	entry := a.ensureProofreadingPDFEntry().pending
	if entry == nil {
		return
	}
	if len(entry.candidates) <= 1 {
		a.openSelectedProofreadingEntry(0)
		return
	}
	actions := make([]modalDialogActionSpec, 0, len(entry.candidates)+1)
	for i, candidate := range entry.candidates {
		label := fmt.Sprintf("%s · %s · %d/%d", candidate.project.Title, filepath.Base(candidate.project.Directory), candidate.project.ReviewedCount, candidate.project.PageCount)
		actions = append(actions, modalDialogActionSpec{ID: fmt.Sprintf("candidate-%d", i), Label: label, WidthDp: 0})
	}
	actions = append(actions, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
	a.modal.ShowMessage(
		"proofread.project.choose",
		i18n.Message{ID: "proofread.project.choose.title"},
		i18n.Message{ID: "proofread.project.choose.body"},
		func(action string) {
			if action == "cancel" {
				a.cancelPendingProofreadingEntry()
				return
			}
			index, err := strconv.Atoi(strings.TrimPrefix(action, "candidate-"))
			if err == nil {
				a.openSelectedProofreadingEntry(index)
			}
		},
		actions...,
	)
	a.modal.CardWidthDp = 560
}

func (a *App) cancelPendingProofreadingEntry() {
	state := a.ensureProofreadingPDFEntry()
	if pending := state.pending; pending != nil && pending.request.sourceSnapshot != nil {
		_ = pending.request.sourceSnapshot.Close()
	}
	state.pending = nil
	state.busy = false
	a.modal.Hide()
}

func (a *App) openSelectedProofreadingEntry(index int) {
	state := a.ensureProofreadingPDFEntry()
	entry := state.pending
	if entry == nil {
		return
	}
	request := entry.request
	// Enter the live comparison even when the matching task has snapshots.
	// History is opened only by an explicit history action.
	request.showHistory = false
	if len(entry.candidates) > 0 {
		if index < 0 {
			index = 0
		}
		if index >= len(entry.candidates) {
			return
		}
		candidate := entry.candidates[index]
		project := candidate.project
		request.resume = &project
		request.resumePayload = &candidate.payload
		request.sourcePath = project.SourceA
		request.sourceName = project.Title
		request.sourcePage = project.CurrentPage
	}
	state.pending = nil
	state.busy = false
	a.modal.Hide()
	if err := a.openProofreadingTab(request); err != nil {
		if request.sourceSnapshot != nil {
			_ = request.sourceSnapshot.Close()
		}
		a.setStatusErrorMessage(i18n.Message{ID: "pdf.error.operation_failed"}, err, notificationToast)
	}
}
