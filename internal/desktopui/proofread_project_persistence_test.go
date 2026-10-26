package desktopui

import (
	"bytes"
	"encoding/json"
	"glance/internal/fileutil"
	"glance/internal/proofreading"
	"glance/internal/workspace"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestProofreadingPlainTextSourceSupport(t *testing.T) {
	root := t.TempDir()
	txtPath := filepath.Join(root, "article.txt")
	content := "\uFEFF第一段中文测试内容\r\n\r\n第二段内容包含多行\n第三段测试内容"
	if err := os.WriteFile(txtPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	obj, err := loadProofreadingObject(txtPath)
	if err != nil {
		t.Fatalf("loadProofreadingObject(txt) failed: %v", err)
	}
	defer obj.Adapter.Close()

	if obj.Role != proofreadingRoleB || obj.Name != "article.txt" {
		t.Fatalf("unexpected object: %+v", obj)
	}
	doc, ok := obj.Adapter.(*proofreadingPlainTextDocument)
	if !ok || doc.FormatID() != "txt" {
		t.Fatalf("unexpected adapter type: %T", obj.Adapter)
	}
	units := doc.ContentUnits()
	if len(units) != 3 {
		t.Fatalf("expected 3 paragraph units, got %d: %+v", len(units), units)
	}
	if units[0].Text != "第一段中文测试内容" || units[1].Text != "第二段内容包含多行" || units[2].Text != "第三段测试内容" {
		t.Fatalf("unexpected unit texts: %+v", units)
	}

	projectText, blocks := proofreadingProjectionContent(units)
	wantProjectText := "第一段中文测试内容\n第二段内容包含多行\n第三段测试内容"
	if projectText != wantProjectText {
		t.Fatalf("proofreadingProjectionContent: got %q, want %q", projectText, wantProjectText)
	}
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}

	// Test persistence and source copying
	store, err := workspace.Open(filepath.Join(root, "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	taskDir := filepath.Join(root, "task-txt")
	p := workspace.ProofreadingProject{
		ID:        "task-txt",
		Title:     "测试文档",
		Directory: taskDir,
		SourceA:   filepath.Join(taskDir, "sources", "reference.pdf"),
		SourceB:   filepath.Join(taskDir, "sources", "manuscript.txt"),
	}
	m, _ := proofreading.New(projectText, []proofreading.PageID{1})
	snap := m.Snapshot()
	data := proofreadingProjectPayload{
		Draft: proofreadingPersistedDraft{Version: 2, Text: projectText, Model: &snap},
		Page:  0,
	}
	if err = persistProofreadingProject(store, p, data, io.NopCloser(bytes.NewBufferString("PDF")), txtPath); err != nil {
		t.Fatal(err)
	}

	savedSourceB, err := os.ReadFile(filepath.Join(taskDir, "sources", "manuscript.txt"))
	if err != nil {
		t.Fatalf("manuscript.txt not saved in sources: %v", err)
	}
	if !bytes.Equal(savedSourceB, []byte(content)) {
		t.Fatalf("saved source B content differs from original")
	}

	loaded, found, err := store.LoadProofreadingProject("task-txt")
	if err != nil || !found {
		t.Fatalf("failed to load saved txt project: %v", err)
	}
	if loaded.SourceB != filepath.Join(taskDir, "sources", "manuscript.txt") {
		t.Fatalf("unexpected loaded SourceB: %s", loaded.SourceB)
	}
}

func TestProofreadingWorkbenchProjectDeleteFlow(t *testing.T) {
	root := t.TempDir()
	store, err := workspace.Open(filepath.Join(root, "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	taskDir := filepath.Join(root, "task-to-delete")
	if err := os.MkdirAll(filepath.Join(taskDir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "proofreading-copy.md"), []byte("draft"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := workspace.ProofreadingProject{
		ID:        "task-to-delete",
		Title:     "待删除任务",
		Directory: taskDir,
		PageCount: 10,
	}
	if err := store.SaveProofreadingProject(p); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPreference("proofreading.pushed.task-to-delete", "git-hash-abc"); err != nil {
		t.Fatal(err)
	}

	a, _ := proofreadingModelFixture(t)
	a.managedBackgroundClosed = false
	a.desktopRuntime.workspaceStore = store
	w := a.ensureProofreadingWorkbench()
	w.projects = []workspace.ProofreadingProject{p}

	// 1. Trigger confirm delete
	a.confirmDeleteProofreadingProject(p)
	if !a.modal.Visible {
		t.Fatal("expected confirmation modal to be visible")
	}
	if a.modal.DialogID != "proofread.workbench.delete" {
		t.Fatalf("unexpected dialog ID: %s", a.modal.DialogID)
	}

	// 2. Cancel action keeps project
	a.modal.OnAction("cancel")
	if a.modal.Visible {
		t.Fatal("modal should be hidden after cancel")
	}
	if len(w.projects) != 1 {
		t.Fatalf("project should not be removed on cancel: %d", len(w.projects))
	}
	if _, err := os.Stat(taskDir); err != nil {
		t.Fatalf("directory should exist after cancel: %v", err)
	}

	// 3. Confirm delete
	a.confirmDeleteProofreadingProject(p)
	a.modal.OnAction("delete")

	// Wait for background job to finish
	for i := 0; i < 50; i++ {
		a.processProofreadingWorkbench()
		if !w.busy && len(w.projects) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(w.projects) != 0 {
		t.Fatalf("expected project removed from workbench state, got %d", len(w.projects))
	}

	// Verify database record deleted
	_, found, err := store.LoadProofreadingProject("task-to-delete")
	if err != nil || found {
		t.Fatalf("project still in database: found=%v err=%v", found, err)
	}

	// Verify pushed preference deleted
	pref, foundPref, err := store.Preference("proofreading.pushed.task-to-delete")
	if err != nil || foundPref || pref != "" {
		t.Fatalf("preference still in database: found=%v val=%v err=%v", foundPref, pref, err)
	}

	// Verify directory and git repo removed from disk
	if _, err := os.Stat(taskDir); !os.IsNotExist(err) {
		t.Fatalf("expected directory removed from disk, got err: %v", err)
	}
}

func TestProofreadingDraftFilePathDoesNotContainSourceFileName(t *testing.T) {
	path, err := proofreadingDraftFilePath("/path/to/very_secret_original_novel.docx", "proofread-a02f4238dd1eab1821f232bbf5c5e37beb879abb5eb80f1f8cf0eda1dab0f49f")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(path) != "proofreading-copy.md" {
		t.Fatalf("expected base file name proofreading-copy.md, got %s", filepath.Base(path))
	}
	dir := filepath.Dir(path)
	dirName := filepath.Base(dir)
	if dirName != "proofread-a02f4238dd1e" {
		t.Fatalf("expected dirName to be shortened to 'proofread-a02f4238dd1e', got %s", dirName)
	}
	parentDir := filepath.Base(filepath.Dir(dir))
	if parentDir != "proofreading" {
		t.Fatalf("expected proofreading directory to be lowercase 'proofreading', got %s", parentDir)
	}
	appRoot := filepath.Base(filepath.Dir(filepath.Dir(dir)))
	if appRoot != fileutil.AppDir {
		t.Fatalf("expected app root directory to match fileutil.AppDir %q, got %s", fileutil.AppDir, appRoot)
	}
	if strings.Contains(path, "very_secret_original_novel") {
		t.Fatalf("draft file path %q must not contain original file name", path)
	}

	// Verify persistence into database
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	project := workspace.ProofreadingProject{
		ID:        "proofread-a02f4238dd1eab1821f232bbf5c5e37beb879abb5eb80f1f8cf0eda1dab0f49f",
		Title:     "原书.pdf",
		Directory: dir,
		SourceA:   filepath.Join(dir, "sources", "reference.pdf"),
		SourceB:   filepath.Join(dir, "sources", "manuscript.docx"),
	}
	if err := store.SaveProofreadingProject(project); err != nil {
		t.Fatalf("SaveProofreadingProject failed: %v", err)
	}
	loaded, found, err := store.LoadProofreadingProject("proofread-a02f4238dd1eab1821f232bbf5c5e37beb879abb5eb80f1f8cf0eda1dab0f49f")
	if err != nil || !found {
		t.Fatalf("LoadProofreadingProject failed: found=%v err=%v", found, err)
	}
	if loaded.Directory != dir {
		t.Fatalf("expected loaded directory %s, got %s", dir, loaded.Directory)
	}
	if !strings.Contains(loaded.Directory, "proofreading") || strings.Contains(loaded.Directory, "Proofreading") {
		t.Fatalf("database record directory must contain lowercase 'proofreading', got %s", loaded.Directory)
	}
	if strings.Contains(loaded.Directory, "very_secret_original_novel") {
		t.Fatalf("database record directory must not contain original file name, got %s", loaded.Directory)
	}
}
