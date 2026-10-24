package desktopui

import (
	"bytes"
	"encoding/json"
	"glance/internal/proofreading"
	"glance/internal/workspace"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProofreadingProjectSQLiteAndSourceRecovery(t *testing.T) {
	root := t.TempDir()
	store, err := workspace.Open(filepath.Join(root, "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b := filepath.Join(root, "draft.md")
	os.WriteFile(b, []byte("正文"), 0600)
	m, _ := proofreading.New("修订正文", []proofreading.PageID{1})
	snap := m.Snapshot()
	p := workspace.ProofreadingProject{ID: "task", Directory: filepath.Join(root, "task"), SourceA: filepath.Join(root, "task", "sources", "a.pdf"), SourceB: filepath.Join(root, "task", "sources", "b.md")}
	data := proofreadingProjectPayload{Draft: proofreadingPersistedDraft{Version: 2, Text: "修订正文", Model: &snap}, Page: 5, Offset: 42, XOffsets: []float64{17}, Confirmed: map[int]string{0: "hash"}}
	if err = persistProofreadingProject(store, p, data, io.NopCloser(bytes.NewBufferString("PDF")), b); err != nil {
		t.Fatal(err)
	}
	os.Remove(b)
	loaded, found, err := store.LoadProofreadingProject("task")
	if err != nil || !found {
		t.Fatal(err)
	}
	var restored proofreadingProjectPayload
	if err = json.Unmarshal(loaded.Payload, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Page != 5 || restored.Offset != 42 || restored.XOffsets[0] != 17 || restored.Draft.Text != "修订正文" {
		t.Fatal(restored)
	}
	src, err := os.ReadFile(loaded.SourceB)
	if err != nil || string(src) != "正文" {
		t.Fatal(string(src), err)
	}
}

func TestProofreadingProjectResumeRestoresPageAndLocalScroll(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	snapshot := s.controller.Model.Snapshot()
	s.controller = nil
	a.pdfView = &pdfViewState{}
	s.aDocument = &proofreadingObject{}
	s.aSourceHash = "A"
	s.sourceBHash = "B"
	s.projectDocumentID = "book"
	s.resume = &workspace.ProofreadingProject{ID: "book", Directory: t.TempDir()}
	s.resumePayload = &proofreadingProjectPayload{Draft: proofreadingPersistedDraft{Version: 2, SourceAHash: "A", SourceBHash: "B", Text: "abcdefghijkl", Model: &snapshot}, Page: 1, Offset: 42, XOffsets: []float64{17, 18, 19}}
	s.draftRestoreResults = make(chan proofreadingDraftRestoreResult, 1)
	a.startProofreadingDraftRestore()
	a.processProofreadingResults()
	view := a.ensurePDFView()
	if view.page != 1 || view.canvas.Position.Offset != 42 || len(s.xPageScrollOffsets) != 3 || s.xPageScrollOffsets[1] != 18 {
		t.Fatal(view.page, view.canvas.Position, s.xPageScrollOffsets)
	}
}

func TestProofreadingProjectPayloadRoundTripsOCRJobAndReadsLegacyPayload(t *testing.T) {
	createdAt := time.Date(2026, 10, 24, 1, 2, 3, 0, time.UTC)
	want := proofreadingOCRJobRecord{
		JobID: "job-17", Endpoint: "https://ocr.example/api/v2/jobs", Model: "PaddleOCR-VL-1.6",
		SourceAHash: "pdf-hash", SourceBHash: "source-b-hash", DraftHash: "draft-hash",
		CreatedAt: createdAt, State: "running",
	}
	data, err := json.Marshal(proofreadingProjectPayload{OCRJob: &want})
	if err != nil {
		t.Fatal(err)
	}
	var got proofreadingProjectPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.OCRJob == nil || *got.OCRJob != want {
		t.Fatalf("OCR job record did not round trip: %#v", got.OCRJob)
	}

	var legacy proofreadingProjectPayload
	if err := json.Unmarshal([]byte(`{"page":3,"draft":{"version":2,"text":"old"}}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Page != 3 || legacy.OCRJob != nil {
		t.Fatalf("legacy payload should remain readable without OCR metadata: %#v", legacy)
	}
}
