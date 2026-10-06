package desktopui

import "glance/internal/document"

func (s *sharedDocumentSession) wordTextStateSnapshot() (committed, drafts map[string]string) {
	committed = make(map[string]string)
	drafts = make(map[string]string)
	if s == nil {
		return committed, drafts
	}
	s.wordDraftMu.RLock()
	for runID, value := range s.wordRunCommitted {
		committed[runID] = value
	}
	for runID, value := range s.wordRunDrafts {
		drafts[runID] = value
	}
	s.wordDraftMu.RUnlock()
	return committed, drafts
}

func (s *sharedDocumentSession) wordDraftSnapshot() map[string]string {
	drafts := make(map[string]string)
	if s == nil {
		return drafts
	}
	s.wordDraftMu.RLock()
	for runID, value := range s.wordRunDrafts {
		drafts[runID] = value
	}
	s.wordDraftMu.RUnlock()
	return drafts
}

func (s *sharedDocumentSession) wordDraft(runID string) (string, bool) {
	if s == nil {
		return "", false
	}
	s.wordDraftMu.RLock()
	value, ok := s.wordRunDrafts[runID]
	s.wordDraftMu.RUnlock()
	return value, ok
}

func (s *sharedDocumentSession) committedWordText(runID string) (string, bool) {
	if s == nil {
		return "", false
	}
	s.wordDraftMu.RLock()
	value, ok := s.wordRunCommitted[runID]
	s.wordDraftMu.RUnlock()
	return value, ok
}

func (s *sharedDocumentSession) setCommittedWordText(runID, value string) {
	if s == nil || runID == "" {
		return
	}
	s.wordDraftMu.Lock()
	if s.wordRunCommitted == nil {
		s.wordRunCommitted = make(map[string]string)
	}
	s.wordRunCommitted[runID] = value
	s.wordDraftMu.Unlock()
}

func (s *sharedDocumentSession) setCommittedWordTextAndClearDraft(runID, value string) {
	if s == nil || runID == "" {
		return
	}
	s.wordDraftMu.Lock()
	if s.wordRunCommitted == nil {
		s.wordRunCommitted = make(map[string]string)
	}
	if s.wordRunDrafts == nil {
		s.wordRunDrafts = make(map[string]string)
	}
	s.wordRunCommitted[runID] = value
	if draft, exists := s.wordRunDrafts[runID]; exists && draft == value {
		delete(s.wordRunDrafts, runID)
		s.bumpWordDraftEpoch()
	}
	s.wordDraftMu.Unlock()
}

func (s *sharedDocumentSession) discardWordDraftIfCommitted(runID, value string) bool {
	if s == nil {
		return false
	}
	s.wordDraftMu.Lock()
	defer s.wordDraftMu.Unlock()
	if s.wordRunDrafts[runID] != value || value != s.wordRunCommitted[runID] {
		return false
	}
	delete(s.wordRunDrafts, runID)
	s.bumpWordDraftEpoch()
	return true
}

func (s *sharedDocumentSession) removeWordTextState(runID string) {
	if s == nil {
		return
	}
	s.wordDraftMu.Lock()
	_, hadCommitted := s.wordRunCommitted[runID]
	_, hadDraft := s.wordRunDrafts[runID]
	delete(s.wordRunCommitted, runID)
	delete(s.wordRunDrafts, runID)
	if hadCommitted || hadDraft {
		s.bumpWordDraftEpoch()
	}
	s.wordDraftMu.Unlock()
}

func (s *sharedDocumentSession) captureWordTextDraft(runID, value string) bool {
	if s == nil || runID == "" {
		return false
	}
	s.wordDraftMu.Lock()
	defer s.wordDraftMu.Unlock()
	if s.wordRunDrafts == nil {
		s.wordRunDrafts = make(map[string]string)
	}
	if value == s.wordRunCommitted[runID] {
		if _, exists := s.wordRunDrafts[runID]; exists {
			delete(s.wordRunDrafts, runID)
			s.bumpWordDraftEpoch()
			return true
		}
		return false
	}
	if previous, exists := s.wordRunDrafts[runID]; exists && previous == value {
		return false
	}
	s.wordRunDrafts[runID] = value
	s.bumpWordDraftEpoch()
	return true
}

func (s *sharedDocumentSession) replaceWordTextState(blocks []document.WordBlock) {
	s.syncWordTextState(blocks, true)
}

func (s *sharedDocumentSession) updateWordTextState(blocks []document.WordBlock) {
	s.syncWordTextState(blocks, false)
}

func wordCommittedTextMap(blocks []document.WordBlock) map[string]string {
	committed := make(map[string]string)
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.Editable {
				committed[inline.Text.ID] = inline.Text.Text
			}
		}
	}
	return committed
}

func (s *sharedDocumentSession) setWordRestoreError(message string) uint64 {
	if s == nil {
		return 0
	}
	s.wordRestoreNoticeMu.Lock()
	if s.wordRestoreError == message {
		epoch := s.wordRestoreErrorEpoch
		s.wordRestoreNoticeMu.Unlock()
		return epoch
	}
	s.wordRestoreError = message
	s.wordRestoreErrorEpoch++
	epoch := s.wordRestoreErrorEpoch
	s.wordRestoreNoticeMu.Unlock()
	return epoch
}

func (s *sharedDocumentSession) wordRestoreNotice() (string, uint64) {
	if s == nil {
		return "", 0
	}
	s.wordRestoreNoticeMu.Lock()
	message, epoch := s.wordRestoreError, s.wordRestoreErrorEpoch
	s.wordRestoreNoticeMu.Unlock()
	return message, epoch
}

func (s *sharedDocumentSession) wordRestoreNoticeEpoch() uint64 {
	_, epoch := s.wordRestoreNotice()
	return epoch
}

func (s *sharedDocumentSession) queueWordRestoreAfterDrafts(versionID int64) bool {
	if s == nil || versionID == 0 {
		return false
	}
	s.wordRestoreLaunchMu.Lock()
	defer s.wordRestoreLaunchMu.Unlock()
	if s.wordRestoreAfterDraftsID != 0 || s.wordRestoreLaunchClaimed {
		return false
	}
	s.wordRestoreAfterDraftsID = versionID
	return true
}

func (s *sharedDocumentSession) pendingWordRestoreAfterDrafts() int64 {
	if s == nil {
		return 0
	}
	s.wordRestoreLaunchMu.Lock()
	versionID := s.wordRestoreAfterDraftsID
	s.wordRestoreLaunchMu.Unlock()
	return versionID
}

func (s *sharedDocumentSession) wordRestoreLaunchInProgress() bool {
	if s == nil {
		return false
	}
	s.wordRestoreLaunchMu.Lock()
	claimed := s.wordRestoreLaunchClaimed
	s.wordRestoreLaunchMu.Unlock()
	return claimed
}

func (s *sharedDocumentSession) claimWordRestoreAfterDrafts() (versionID int64, handled bool) {
	if s == nil {
		return 0, false
	}
	s.wordRestoreLaunchMu.Lock()
	defer s.wordRestoreLaunchMu.Unlock()
	if s.wordRestoreLaunchClaimed {
		return 0, true
	}
	if s.wordRestoreAfterDraftsID == 0 {
		return 0, false
	}
	versionID = s.wordRestoreAfterDraftsID
	s.wordRestoreAfterDraftsID = 0
	s.wordRestoreLaunchClaimed = true
	return versionID, true
}

func (s *sharedDocumentSession) releaseWordRestoreClaim() {
	if s == nil {
		return
	}
	s.wordRestoreLaunchMu.Lock()
	s.wordRestoreLaunchClaimed = false
	s.wordRestoreLaunchMu.Unlock()
}

func (s *sharedDocumentSession) clearPendingWordRestore() {
	if s == nil {
		return
	}
	s.wordRestoreLaunchMu.Lock()
	s.wordRestoreAfterDraftsID = 0
	s.wordRestoreLaunchMu.Unlock()
}

func (a *App) surfacePendingWordRestoreError(state *documentSessionState) {
	if a == nil || state == nil {
		return
	}
	message, epoch := state.wordRestoreNotice()
	if message == "" || epoch <= state.appliedWordRestoreErrorEpoch.Load() {
		return
	}
	a.setStatus(message)
	state.appliedWordRestoreErrorEpoch.Store(epoch)
}

func (a *App) publishWordRestoreNotice(state *documentSessionState, message string) {
	if a == nil || state == nil || message == "" {
		return
	}
	epoch := recordWordRestoreNotice(a.ensureDesktopRuntime(), state, message)
	if state == a.documentSessionState {
		a.setStatus(message)
		state.appliedWordRestoreErrorEpoch.Store(epoch)
	}
}

func recordWordRestoreNotice(runtime *desktopRuntime, state *documentSessionState, message string) uint64 {
	if state == nil || message == "" {
		return 0
	}
	epoch := state.setWordRestoreError(message)
	if runtime != nil {
		runtime.invalidateSessionViews(state.documentID)
	}
	return epoch
}

func (s *sharedDocumentSession) syncWordTextState(blocks []document.WordBlock, clearDrafts bool) {
	if s == nil {
		return
	}
	committed := wordCommittedTextMap(blocks)
	s.wordDraftMu.Lock()
	drafts := make(map[string]string)
	if !clearDrafts {
		for runID, value := range s.wordRunDrafts {
			if committedValue, exists := committed[runID]; exists && value != committedValue {
				drafts[runID] = value
			}
		}
	}
	changed := !sameWordTextMap(s.wordRunCommitted, committed) || !sameWordTextMap(s.wordRunDrafts, drafts)
	s.wordRunCommitted = committed
	s.wordRunDrafts = drafts
	s.wordTextResetEpoch.Add(1)
	if changed || clearDrafts {
		s.bumpWordDraftEpoch()
	}
	s.wordDraftMu.Unlock()
}

func sameWordTextMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
