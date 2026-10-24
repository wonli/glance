package desktopui

import (
	"gioui.org/layout"
	"io"
	"strings"
	"testing"

	"glance/internal/workspace"
)

func TestPDFEntryResumesComparisonWithoutDuplicateOrHistory(t *testing.T) {
	host := newAppWithRuntime(nil, newDesktopRuntime())
	host.proofreadingModule = true
	defer func() { host.closeAllProofreadingTabs(); host.stopAndWaitManagedBackground() }()
	enter := func() {
		host.ensureProofreadingPDFEntry().pending = &proofreadingEntryResult{
			request:    proofreadingWindowRequest{sourceSnapshot: io.NopCloser(strings.NewReader("invalid test PDF"))},
			candidates: []proofreadingEntryCandidate{{project: workspace.ProofreadingProject{ID: "book", Title: "book.pdf", SourceA: "/books/book.pdf"}, hasHistory: true}},
		}
		host.openSelectedProofreadingEntry(0)
		host.processProofreadingTabs(layout.Context{})
	}
	enter()
	if len(host.proofreadingTabs) != 1 {
		t.Fatalf("tabs=%d", len(host.proofreadingTabs))
	}
	tab := host.proofreadingTabs[0]
	w := tab.editor.ensureProofreadingWorkbench()
	if w.historyOpen || w.active {
		t.Fatal("PDF entry opened history/list instead of comparison")
	}
	// Re-enter while the existing task is showing history.
	w.historyOpen = true
	w.historyLoading = true
	w.active = true
	previousRequest := w.historyRequest
	enter()
	if len(host.proofreadingTabs) != 1 || host.selectedProofreadingTabID != tab.id {
		t.Fatal("PDF entry duplicated the task instead of selecting it")
	}
	if w.historyOpen || w.active || w.historyLoading {
		t.Fatal("existing task did not return to comparison")
	}
	if w.historyRequest <= previousRequest {
		t.Fatal("late history results were not invalidated")
	}
	if host.pdfDoc != nil || len(host.sessions) != 0 || len(host.tabMeta) != 0 {
		t.Fatal("PDF entry opened a duplicate regular PDF")
	}
}
