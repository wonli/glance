package desktopui

import (
	"errors"
	"fmt"
	"strings"

	"glance/internal/diagnostics"
	"glance/internal/workspace"
)

// DocumentSessionDiagnostics details an in-memory document session and its adapters.
type DocumentSessionDiagnostics struct {
	DocumentID      string `json:"document_id"`
	Path            string `json:"path"`
	SourceFormat    string `json:"source_format"`
	HasWord         bool   `json:"has_word"`
	HasBook         bool   `json:"has_book"`
	HasPDF          bool   `json:"has_pdf"`
	HasJSON         bool   `json:"has_json"`
	HasMarkdown     bool   `json:"has_markdown"`
	HasPPTX         bool   `json:"has_pptx"`
	WordBlocksCount int    `json:"word_blocks_count,omitempty"`
	WordRevision    uint64 `json:"word_revision,omitempty"`
	SaveAsRequired  bool   `json:"save_as_required"`
}

// WindowDiagnostics details UI view state and active layout engines in an open window.
type WindowDiagnostics struct {
	WindowID             uint64 `json:"window_id"`
	ActiveTabID          string `json:"active_tab_id"`
	SessionCount         int      `json:"session_count"`
	SessionTabIDs        []string `json:"session_tab_ids,omitempty"`
	HasWordLayoutEngine  bool   `json:"has_word_layout_engine"`
	HasWordPDFFontSource bool   `json:"has_word_pdf_font_source"`
	ActiveWordPageCount  int    `json:"active_word_page_count,omitempty"`
}

// WorkspaceDiagnosticsSnapshot contains a complete view of SQLite, tabs, and sessions.
type WorkspaceDiagnosticsSnapshot struct {
	Store           *workspace.StoreDiagnostics     `json:"store,omitempty"`
	Collection      workspace.CollectionDiagnostics `json:"collection"`
	RuntimeSessions []DocumentSessionDiagnostics    `json:"runtime_sessions"`
	Windows         []WindowDiagnostics             `json:"windows"`
}

func (r *desktopRuntime) registerDiagnostics() {
	if r == nil {
		return
	}
	registerMemoryShrinkHook(r.shrinkWorkspace)
	diagnostics.RegisterWorkspaceDiagnostics(r.diagnoseWorkspace, r.shrinkWorkspace, r.handleWorkspaceAction)
}

func (r *desktopRuntime) handleWorkspaceAction(action string) (any, error) {
	if r == nil {
		return nil, errors.New("runtime unavailable")
	}
	switch action {
	case "vacuum":
		r.workspaceDBMu.Lock()
		defer r.workspaceDBMu.Unlock()
		if r.workspaceStore == nil {
			return nil, errors.New("workspace store unavailable")
		}
		if err := r.workspaceStore.Vacuum(); err != nil {
			return nil, err
		}
		diag, _ := r.workspaceStore.Diagnostics()
		return map[string]any{"status": "vacuumed", "store": diag}, nil
	case "close-active":
		r.mu.Lock()
		app := r.windows[r.primaryWindowID]
		r.mu.Unlock()
		if app == nil {
			return nil, errors.New("primary window unavailable")
		}
		activeID := app.activeTabID
		if activeID == "" || activeID == workspace.RecentTabID {
			return map[string]any{"status": "no active document tab to close"}, nil
		}
		app.closeWorkspaceTabNow(activeID)
		return map[string]any{"status": "closed", "tab_id": activeID}, nil
	case "activate-recent":
		r.mu.Lock()
		app := r.windows[r.primaryWindowID]
		r.mu.Unlock()
		if app == nil {
			return nil, errors.New("primary window unavailable")
		}
		app.activateRecent()
		return map[string]any{"status": "activated recent"}, nil
	case "evict-inactive":
		r.mu.Lock()
		app := r.windows[r.primaryWindowID]
		r.mu.Unlock()
		if app == nil {
			return nil, errors.New("primary window unavailable")
		}
		app.evictInactiveSessions()
		return map[string]any{"status": "evicted inactive sessions"}, nil
	case "save":
		r.mu.Lock()
		app := r.windows[r.primaryWindowID]
		r.mu.Unlock()
		if app == nil {
			return nil, errors.New("primary window unavailable")
		}
		if err := app.save(); err != nil {
			return nil, err
		}
		return map[string]any{"status": "saved"}, nil
	case "shrink":
		if err := r.shrinkWorkspace(); err != nil {
			return nil, err
		}
		requestMemoryRelease()
		return map[string]any{"status": "shrunk"}, nil
	default:
		if strings.HasPrefix(action, "open:") {
			targetPath := strings.TrimPrefix(action, "open:")
			r.mu.Lock()
			app := r.windows[r.primaryWindowID]
			r.mu.Unlock()
			if app == nil {
				return nil, errors.New("primary window unavailable")
			}
			if err := app.open(targetPath); err != nil {
				return nil, err
			}
			return map[string]any{"status": "opened", "path": targetPath}, nil
		}
		if strings.HasPrefix(action, "close:") {
			tabID := strings.TrimPrefix(action, "close:")
			r.mu.Lock()
			app := r.windows[r.primaryWindowID]
			r.mu.Unlock()
			if app == nil {
				return nil, errors.New("primary window unavailable")
			}
			app.closeWorkspaceTabNow(tabID)
			return map[string]any{"status": "closed", "tab_id": tabID}, nil
		}
		if strings.HasPrefix(action, "activate:") {
			tabID := strings.TrimPrefix(action, "activate:")
			r.mu.Lock()
			app := r.windows[r.primaryWindowID]
			r.mu.Unlock()
			if app == nil {
				return nil, errors.New("primary window unavailable")
			}
			app.activateWorkspaceTab(tabID)
			return map[string]any{"status": "activated", "tab_id": tabID}, nil
		}
		return nil, fmt.Errorf("unknown action %q", action)
	}
}

func (r *desktopRuntime) shrinkWorkspace() error {
	if r == nil {
		return nil
	}
	r.evictOrphanSessions()
	r.workspaceDBMu.Lock()
	defer r.workspaceDBMu.Unlock()
	if r.workspaceStore != nil {
		_, _ = r.workspaceStore.AutoMaintainSpace()
		return r.workspaceStore.ShrinkMemory()
	}
	return nil
}

func (r *desktopRuntime) diagnoseWorkspace() any {
	if r == nil {
		return map[string]string{"error": "runtime unavailable"}
	}
	out := WorkspaceDiagnosticsSnapshot{}

	r.workspaceDBMu.Lock()
	if r.workspaceStore != nil {
		if storeDiag, err := r.workspaceStore.Diagnostics(); err == nil {
			out.Store = &storeDiag
		}
	}
	col := r.collection
	r.workspaceDBMu.Unlock()

	if col != nil {
		out.Collection = col.Diagnostics()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for docID, session := range r.sessions {
		if session == nil {
			continue
		}
		item := DocumentSessionDiagnostics{
			DocumentID:     docID,
			Path:           session.path,
			SourceFormat:   string(session.sourceFormat),
			HasWord:        session.word != nil,
			HasBook:        session.book != nil,
			HasPDF:         session.pdfDoc != nil,
			HasJSON:        session.jsonDoc != nil,
			HasMarkdown:    session.markdownDoc != nil,
			HasPPTX:        session.pptxDoc != nil,
			SaveAsRequired: session.saveAsRequired,
		}
		if session.word != nil {
			session.wordBlocksMu.RLock()
			item.WordBlocksCount = len(session.wordBlocks)
			session.wordBlocksMu.RUnlock()
			item.WordRevision = session.wordRevision
		}
		out.RuntimeSessions = append(out.RuntimeSessions, item)
	}

	for winID, app := range r.windows {
		if app == nil {
			continue
		}
		sessionTabIDs := make([]string, 0, len(app.sessions))
		for tabID := range app.sessions {
			sessionTabIDs = append(sessionTabIDs, tabID)
		}
		wDiag := WindowDiagnostics{
			WindowID:             winID,
			ActiveTabID:          app.activeTabID,
			SessionCount:         len(app.sessions),
			SessionTabIDs:        sessionTabIDs,
			HasWordLayoutEngine:  app.wordLayoutEngine != nil,
			HasWordPDFFontSource: app.wordPDFFontSource != nil,
		}
		if app.documentSessionState != nil && app.documentSessionState.wordLayoutResult != nil {
			wDiag.ActiveWordPageCount = len(app.documentSessionState.wordLayoutResult.Pages)
		}
		out.Windows = append(out.Windows, wDiag)
	}

	return out
}
