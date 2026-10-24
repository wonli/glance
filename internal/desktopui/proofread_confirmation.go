package desktopui

import (
	"context"
	"fmt"

	"glance/internal/proofhistory"
	"glance/internal/proofreading"
	"time"
)

type proofreadingHistoryState struct {
	notice      string
	tokens      map[int]proofreading.ContentToken
	head        string
	confirmed   map[int]string
	snapshots   map[int]string
	pending     bool
	pendingPage int
	results     chan proofreadingConfirmationResult
	pushed      string
	errorPage   int
	err         string
}
type proofreadingConfirmationResult struct {
	unavailable bool
	page        int
	hash        string
	commit      string
	err         error
}

func cloneProofreadingHashes(m map[int]string) map[int]string {
	c := make(map[int]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
func proofreadingStatusKey(reviewed bool, hash string, page int, h *proofreadingHistoryState) string {
	if !reviewed {
		return "proofread.history.unreviewed"
	}
	if saved := h.confirmed[page]; saved != "" && saved != hash {
		return "proofread.history.modified"
	}
	if h.pending && h.pendingPage == page {
		return "proofread.history.saving"
	}
	if h.err != "" && h.errorPage == page {
		return "proofread.history.failed"
	}
	if hash != "" && h.snapshots[page] == hash {
		return "proofread.history.saved"
	}
	return "proofread.history.confirmed_no_snapshot"
}
func (a *App) confirmProofreadingPage(page int) {
	s := a.proofreading
	if s == nil || s.controller == nil {
		return
	}
	for i := range s.xPageEditors {
		if editorHasActiveComposition(&s.xPageEditors[i]) {
			s.draftPersistError = a.textID("proofread.history.ime")
			return
		}
	}
	if s.history.pending {
		return
	}
	identity := a.savedProofreadingIdentity()
	p := proofreadingPageResult(s, page, identity)
	if err := s.controller.Model.Review(proofreadingPageID(page), true); err != nil {
		s.draftPersistError = err.Error()
		return
	}
	if s.history.confirmed == nil {
		s.history.confirmed = map[int]string{}
	}
	s.history.confirmed[page] = p.ContentHash()
	if s.history.tokens == nil {
		s.history.tokens = map[int]proofreading.ContentToken{}
	}
	s.history.tokens[page] = s.controller.Model.ContentToken(proofreadingPageID(page))
	s.history.err = ""
	s.history.notice = ""
	resetProofreadingNavigatorInput(s)
	a.proofreadingModelChanged(false)
	if s.draftFilePath == "" {
		return
	}
	if s.history.results == nil {
		s.history.results = make(chan proofreadingConfirmationResult, 1)
	}
	s.history.pending = true
	s.history.pendingPage = page
	channel, window := s.history.results, a.window
	dir := a.proofreadingProjectMetadata().Directory
	if !a.launchManagedBackground(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var err error
		unavailable := (proofhistory.Repository{Directory: dir}).Available(ctx) != nil

		if err == nil {
			err = p.Identity.Validate()
		}
		var commit proofhistory.CommitResult
		s.draftFileMu.Lock()
		if unavailable {
			err = fmt.Errorf("git unavailable")
		}
		if err == nil {
			commit, err = (proofhistory.Repository{Directory: dir}).Confirm(ctx, p)
		}
		s.draftFileMu.Unlock()
		channel <- proofreadingConfirmationResult{page: page, hash: p.ContentHash(), commit: commit.Hash, err: err, unavailable: unavailable}
		if window != nil {
			window.Invalidate()
		}
	}) {
		s.history.pending = false
	}
}
func (a *App) acceptProofreadingConfirmation() {
	s := a.proofreading
	if s == nil || s.history.results == nil {
		return
	}
	select {
	case r := <-s.history.results:
		s.history.pending = false
		if r.unavailable {
			s.history.err = ""
			s.history.notice = a.textID("proofread.history.install_git")
		} else if r.err != nil {
			s.history.errorPage = r.page
			s.history.err = a.textID("proofread.history.setup_required")
			s.history.notice = s.history.err
		} else {
			if s.history.snapshots == nil {
				s.history.snapshots = map[int]string{}
			}
			s.history.snapshots[r.page] = r.hash
			s.history.head = r.commit
			s.history.notice = ""
			s.history.err = ""
		}
		a.queueProofreadingDraftPersistence()
	default:
	}
}
