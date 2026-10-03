package desktopui

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
	_, drafts := s.wordTextStateSnapshot()
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
