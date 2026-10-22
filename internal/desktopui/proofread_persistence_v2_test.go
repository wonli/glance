package desktopui

import (
	"bytes"
	"glance/internal/proofreading"
	"os"
	"path/filepath"
	"testing"
)

func TestProofreadingV2SaveMigrationPreservesLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "copy.md")
	legacy := proofreadingPersistedDraft{Version: 1, Text: "甲\n乙", SourceAHash: "A", SourceBHash: "B"}
	old, err := encodeProofreadingDraftFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	m, _ := proofreading.New(legacy.Text, []proofreading.PageID{1})
	s := m.Snapshot()
	v2 := legacy
	v2.Version = 2
	v2.Model = &s
	if err = writeProofreadingDraftFile(path, v2); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".v1-backup")
	if err != nil || !bytes.Equal(backup, old) {
		t.Fatal("legacy copy not preserved", err)
	}
	data, _ := os.ReadFile(path)
	restored, err := decodeProofreadingDraftFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Model == nil || restored.Text != legacy.Text {
		t.Fatal("model not restored")
	}
	if err := validateProofreadingDraftForRestore(restored, "A", "B"); err != nil {
		t.Fatalf("valid v2 saved copy was rejected by the restore path: %v", err)
	}
	if err := validateProofreadingDraftForRestore(restored, "wrong A", "B"); err == nil {
		t.Fatal("v2 saved copy was restored for a different source")
	}
	corrupt := append([]byte(nil), data...)
	corrupt = append(corrupt, []byte("bad")...)
	if _, err = decodeProofreadingDraftFile(corrupt); err == nil {
		t.Fatal("text/model mismatch accepted")
	}
}
func TestProofreadingSaveDoesNotOverwriteUnreadableCopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "copy.md")
	old := []byte("invalid original content")
	_ = os.WriteFile(path, old, 0o600)
	m, _ := proofreading.New("new", []proofreading.PageID{1})
	snapshot := m.Snapshot()
	if err := writeProofreadingDraftFile(path, proofreadingPersistedDraft{Version: 2, Model: &snapshot, Text: "new"}); err == nil {
		t.Fatal("overwrote unreadable copy")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, old) {
		t.Fatal("old bytes changed")
	}
}

func TestProofreadingFailedRestoreDoesNotStartFreshOverwrite(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	path := filepath.Join(t.TempDir(), "copy.md")
	s.draftFilePath = path
	s.projectDocumentID = "project"
	s.draftRestoreFailed = true
	a.queueProofreadingDraftPersistence()
	if s.draftSavePending {
		t.Fatal("failed recovery started a fresh save over the saved copy")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed recovery created or overwrote the saved copy: %v", err)
	}
}
