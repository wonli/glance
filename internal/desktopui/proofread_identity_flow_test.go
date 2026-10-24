package desktopui

import (
	"path/filepath"
	"testing"

	"glance/internal/workspace"
)

func TestProofreadingPDFEntryDoesNotRequireIdentity(t *testing.T) {
	a := New(nil, "")
	entry := a.ensureProofreadingPDFEntry()
	entry.results <- proofreadingEntryResult{
		candidates: []proofreadingEntryCandidate{
			{project: workspace.ProofreadingProject{Title: "Book A", Directory: "/tmp/book-a"}},
			{project: workspace.ProofreadingProject{Title: "Book B", Directory: "/tmp/book-b"}},
		},
	}

	a.processProofreadingPDFEntryResult()

	if !a.modal.Visible || a.modal.DialogID != "proofread.project.choose" {
		t.Fatalf("PDF entry should proceed to existing project selection without identity, dialog=%q visible=%v", a.modal.DialogID, a.modal.Visible)
	}
}

func TestProofreadingGitInstallHintShowsOnlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !proofreadingShouldShowGitInstallHint(store) {
		t.Fatal("first missing-Git confirmation should show the install hint")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = workspace.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if proofreadingShouldShowGitInstallHint(store) {
		t.Fatal("later confirmations, including after restart, should not repeat the install hint")
	}
}

func TestConfirmProofreadingPageDoesNotRequireIdentity(t *testing.T) {
	a, s := proofreadingModelFixture(t)

	a.confirmProofreadingPage(0)

	page := s.controller.Model.PageSnapshot(1).Pages[0]
	if !page.Reviewed {
		t.Fatal("page confirmation should be recorded without a Git identity")
	}
	if a.modal.Visible {
		t.Fatalf("page confirmation should not prompt for Git identity, dialog=%q", a.modal.DialogID)
	}
}
