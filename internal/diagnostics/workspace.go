package diagnostics

import (
	"net/http"
	"sync"
)

// WorkspaceInspectionProvider returns arbitrary workspace diagnostic information.
type WorkspaceInspectionProvider func() any

// MemoryShrinkProvider performs engine-level memory shrinking (e.g. SQLite PRAGMA shrink_memory).
type MemoryShrinkProvider func() error

// WorkspaceActionProvider executes a diagnostic control action (e.g. vacuum, close-active, activate-recent).
type WorkspaceActionProvider func(action string) (any, error)

var (
	workspaceMu                 sync.RWMutex
	workspaceInspectionProvider WorkspaceInspectionProvider
	workspaceShrinkProvider     MemoryShrinkProvider
	workspaceActionProvider     WorkspaceActionProvider
)

// RegisterWorkspaceDiagnostics registers workspace inspection, memory shrinking, and action hooks.
func RegisterWorkspaceDiagnostics(provider WorkspaceInspectionProvider, shrinker MemoryShrinkProvider, actions ...WorkspaceActionProvider) {
	workspaceMu.Lock()
	defer workspaceMu.Unlock()
	workspaceInspectionProvider = provider
	workspaceShrinkProvider = shrinker
	if len(actions) > 0 {
		workspaceActionProvider = actions[0]
	}
}

func invokeWorkspaceShrinker() {
	workspaceMu.RLock()
	shrinker := workspaceShrinkProvider
	workspaceMu.RUnlock()
	if shrinker != nil {
		_ = shrinker()
	}
}

func registerWorkspaceHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /debug/workspace", func(w http.ResponseWriter, r *http.Request) {
		workspaceMu.RLock()
		provider := workspaceInspectionProvider
		workspaceMu.RUnlock()
		if provider == nil {
			writeDiagnosticJSON(w, map[string]any{"status": "workspace inspection provider not registered"})
			return
		}
		writeDiagnosticJSON(w, provider())
	})

	mux.HandleFunc("POST /debug/workspace/shrink", func(w http.ResponseWriter, r *http.Request) {
		invokeWorkspaceShrinker()
		writeDiagnosticJSON(w, readMemorySnapshot())
	})

	mux.HandleFunc("POST /debug/workspace/action", func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("do")
		workspaceMu.RLock()
		handler := workspaceActionProvider
		workspaceMu.RUnlock()
		if handler == nil {
			http.Error(w, "workspace action provider not registered", http.StatusNotImplemented)
			return
		}
		result, err := handler(action)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeDiagnosticJSON(w, map[string]any{
			"action": action,
			"result": result,
			"memory": readMemorySnapshot(),
		})
	})
}
