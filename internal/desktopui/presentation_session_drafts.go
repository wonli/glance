package desktopui

const (
	presentationTextDraft      = "text"
	presentationTextEditorOpen = "text_editor"
	presentationNotesDraft     = "notes"
	presentationTransformDraft = "transform"
	presentationCropDraft      = "picture_crop"
)

func (s *sharedDocumentSession) setPresentationDraftDirty(windowID uint64, kind string, dirty bool) {
	if s == nil || windowID == 0 || kind == "" {
		return
	}
	s.presentationDraftMu.Lock()
	if s.presentationDraftDirty == nil {
		s.presentationDraftDirty = make(map[uint64]map[string]bool)
	}
	if dirty {
		if s.presentationDraftDirty[windowID] == nil {
			s.presentationDraftDirty[windowID] = make(map[string]bool)
		}
		s.presentationDraftDirty[windowID][kind] = true
	} else {
		delete(s.presentationDraftDirty[windowID], kind)
		if len(s.presentationDraftDirty[windowID]) == 0 {
			delete(s.presentationDraftDirty, windowID)
		}
	}
	s.presentationDraftMu.Unlock()
}

func (s *sharedDocumentSession) clearPresentationDrafts(windowID uint64) {
	if s == nil || windowID == 0 {
		return
	}
	s.presentationDraftMu.Lock()
	delete(s.presentationDraftDirty, windowID)
	s.presentationDraftMu.Unlock()
}

func (s *sharedDocumentSession) hasPendingPresentationDraft() bool {
	if s == nil {
		return false
	}
	s.presentationDraftMu.RLock()
	defer s.presentationDraftMu.RUnlock()
	for _, drafts := range s.presentationDraftDirty {
		for kind, dirty := range drafts {
			if kind == presentationTextEditorOpen {
				continue
			}
			if dirty {
				return true
			}
		}
	}
	return false
}

func (s *sharedDocumentSession) hasOpenPresentationTextEditor() bool {
	if s == nil {
		return false
	}
	s.presentationDraftMu.RLock()
	defer s.presentationDraftMu.RUnlock()
	for _, drafts := range s.presentationDraftDirty {
		if drafts[presentationTextEditorOpen] {
			return true
		}
	}
	return false
}

func (s *sharedDocumentSession) hasPendingPresentationCropDraft() bool {
	if s == nil {
		return false
	}
	s.presentationDraftMu.RLock()
	defer s.presentationDraftMu.RUnlock()
	for _, drafts := range s.presentationDraftDirty {
		if drafts[presentationCropDraft] {
			return true
		}
	}
	return false
}
