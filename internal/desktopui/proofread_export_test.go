package desktopui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/application"
	"glance/internal/fontmanager/fontcatalog"
	"glance/internal/proofhistory"
	"glance/internal/proofreading"
	"glance/internal/workspace"
)

type proofreadingExportFixture struct {
	store       *workspace.Store
	project     workspace.ProofreadingProject
	original    string
	originalRaw []byte
}

func newProofreadingExportFixture(t *testing.T, extension string, saveEnds bool, confirmed map[int]string, stale map[int]string, sourceContent ...string) proofreadingExportFixture {
	t.Helper()
	root := t.TempDir()
	store, err := workspace.Open(filepath.Join(root, "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close proofreading export store: %v", err)
		}
	})

	projectDir := filepath.Join(root, "task")
	sourcesDir := filepath.Join(projectDir, "sources")
	if err := os.MkdirAll(sourcesDir, 0700); err != nil {
		t.Fatal(err)
	}
	originalPDFPath := filepath.Join(root, "original-reference.pdf")
	pdfPath := filepath.Join(sourcesDir, "reference.pdf")
	pdfBytes := pdfTestBytes(t)
	for _, path := range []string{originalPDFPath, pdfPath} {
		if err := os.WriteFile(path, pdfBytes, 0600); err != nil {
			t.Fatal(err)
		}
	}
	pdfDigest := sha256.Sum256(pdfBytes)
	pdfHash := hex.EncodeToString(pdfDigest[:])
	originalBPath := filepath.Join(root, "Original Manuscript"+extension)
	bPath := filepath.Join(sourcesDir, "manuscript"+extension)
	source := "aa\nbb\ncc\ndd"
	if len(sourceContent) > 0 {
		source = sourceContent[0]
	}
	for _, path := range []string{originalBPath, bPath} {
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	object, err := loadProofreadingObject(bPath)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := proofreadingProjectionContent(object.contentUnits())
	_ = object.Adapter.Close()
	ids := []proofreading.PageID{1, 2, 3, 4}
	model, err := proofreading.New(original, ids)
	if err != nil {
		t.Fatal(err)
	}
	ends := make([]int, len(ids))
	runeCount := len([]rune(original))
	for i := range ends {
		ends[i] = (runeCount * (i + 1)) / len(ends)
	}
	if err = model.AssignInitial(ends); err != nil {
		t.Fatal(err)
	}
	projectID, sourceBHash := proofreadingProjectDocumentID(originalPDFPath, pdfHash, originalBPath, original)
	confirmedHashes := map[int]string{}
	for page, content := range confirmed {
		id := proofreading.PageID(page + 1)
		if err = model.ReplacePage(id, content); err != nil {
			t.Fatal(err)
		}
		if err = model.Review(id, true); err != nil {
			t.Fatal(err)
		}
		confirmedHashes[page] = proofreadingExportCurrentPageHash(projectID, pdfHash, model, id)
	}
	for page, content := range stale {
		if err = model.ReplacePage(proofreading.PageID(page+1), content); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := model.Snapshot()
	var current strings.Builder
	for _, run := range snapshot.Runs {
		current.WriteString(run.Text)
	}
	draft := proofreadingPersistedDraft{
		Version: 2, SourceAHash: pdfHash, SourceBHash: sourceBHash,
		Text: current.String(), Model: &snapshot,
	}
	if saveEnds {
		draft.OriginalPageEnds = ends
	}
	payload, err := json.Marshal(proofreadingProjectPayload{Draft: draft, Confirmed: confirmedHashes})
	if err != nil {
		t.Fatal(err)
	}
	project := workspace.ProofreadingProject{
		ID: projectID, Title: "Reference", Directory: projectDir, SourceA: pdfPath, SourceAOriginalPath: originalPDFPath,
		SourceAHash: pdfHash, SourceB: bPath, SourceBName: "Original Manuscript" + extension, SourceBOriginalPath: originalBPath, PageCount: len(ids), Payload: payload,
	}
	if err = store.SaveProofreadingProject(project); err != nil {
		t.Fatal(err)
	}
	project, found, err := store.LoadProofreadingProjectMetadata(projectID)
	if err != nil || !found {
		t.Fatalf("load saved proofreading metadata: found=%v err=%v", found, err)
	}
	return proofreadingExportFixture{store: store, project: project, original: original, originalRaw: []byte(source)}
}

func proofreadingExportCurrentPageHash(projectID, sourceHash string, model *proofreading.Model, id proofreading.PageID) string {
	pageSnapshot := model.PageSnapshot(id)
	page := pageSnapshot.Pages[0]
	page.Reviewed = true
	page.Locked = false
	return (proofhistory.PageResult{
		Version: 1, ProjectID: projectID, SourceHash: sourceHash,
		Page: page, Runs: pageSnapshot.Runs,
	}).ContentHash()
}

func TestBuildProofreadingExportCombinesOriginalAndConfirmedPagesWithoutGit(t *testing.T) {
	fixture := newProofreadingExportFixture(t, ".txt", true, map[int]string{2: "\nCC"}, nil)
	got, extension, err := buildProofreadingProjectExport(context.Background(), fixture.store, fixture.project, "Go", fontcatalog.Snapshot{})
	if err != nil {
		t.Fatalf("buildProofreadingProjectExport: %v", err)
	}
	if extension != ".txt" {
		t.Fatalf("extension = %q, want .txt", extension)
	}
	if want := "aa\nbb\nCC\ndd"; got != want {
		t.Fatalf("export = %q, want %q", got, want)
	}
	if string(fixture.originalRaw) != "aa\nbb\ncc\ndd" {
		t.Fatal("fixture source changed before export")
	}
	retained, err := os.ReadFile(fixture.project.SourceB)
	if err != nil || string(retained) != string(fixture.originalRaw) {
		t.Fatalf("export changed retained Source B: %q, %v", retained, err)
	}
	external, err := os.ReadFile(fixture.project.SourceBOriginalPath)
	if err != nil || string(external) != string(fixture.originalRaw) {
		t.Fatalf("export changed original Source B: %q, %v", external, err)
	}
}

func TestBuildProofreadingExportRejectsModifiedConfirmedPage(t *testing.T) {
	fixture := newProofreadingExportFixture(t, ".txt", true, map[int]string{2: "BB\n"}, map[int]string{2: "edited after confirm\n"})
	_, _, err := buildProofreadingProjectExport(context.Background(), fixture.store, fixture.project, "Go", fontcatalog.Snapshot{})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "page 3") {
		t.Fatalf("buildProofreadingProjectExport error = %v, want page 3 confirmation error", err)
	}
}

func TestBuildProofreadingExportReconstructsLegacyOriginalPageMap(t *testing.T) {
	fixture := newProofreadingExportFixture(t, ".txt", false, nil, nil)
	prepareLegacyProofreadingExportFixture(t, &fixture, false)
	got, _, err := buildProofreadingProjectExport(context.Background(), fixture.store, fixture.project, "Go", fontcatalog.Snapshot{})
	if err != nil {
		t.Fatalf("buildProofreadingProjectExport legacy mapping: %v", err)
	}
	if got != fixture.original {
		t.Fatalf("legacy export = %q, want original projection %q", got, fixture.original)
	}
}

func TestBuildProofreadingExportReconstructsMixedLegacyPageMap(t *testing.T) {
	fixture := newProofreadingExportFixture(t, ".txt", false, nil, nil)
	prepareLegacyProofreadingExportFixture(t, &fixture, true)
	got, _, err := buildProofreadingProjectExport(context.Background(), fixture.store, fixture.project, "Go", fontcatalog.Snapshot{})
	if err != nil {
		t.Fatalf("buildProofreadingProjectExport mixed legacy mapping: %v", err)
	}
	if want := fixture.original + "confirmed"; got != want {
		t.Fatalf("mixed legacy export = %q, want %q", got, want)
	}
}

func TestBuildProofreadingExportUsesOriginalForEditedLegacyUnconfirmedPage(t *testing.T) {
	fixture := newProofreadingExportFixture(t, ".txt", false, nil, nil)
	prepareLegacyProofreadingExportFixture(t, &fixture, false)
	stored, found, err := fixture.store.LoadProofreadingProject(fixture.project.ID)
	if err != nil || !found {
		t.Fatalf("load legacy export fixture: found=%v err=%v", found, err)
	}
	payload, err := decodeProofreadingExportPayload(stored)
	if err != nil {
		t.Fatal(err)
	}
	model, err := proofreading.Restore(*payload.Draft.Model)
	if err != nil {
		t.Fatal(err)
	}
	changed := "aA" + strings.TrimPrefix(fixture.original, "aa")
	if err := model.ReplacePage(1, changed); err != nil {
		t.Fatal(err)
	}
	snapshot := model.Snapshot()
	payload.Draft.Model = &snapshot
	payload.Draft.Text = changed
	stored.Payload, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.SaveProofreadingProject(stored); err != nil {
		t.Fatal(err)
	}
	got, _, err := buildProofreadingProjectExport(context.Background(), fixture.store, fixture.project, "Go", fontcatalog.Snapshot{})
	if err != nil {
		t.Fatalf("buildProofreadingProjectExport with edited legacy page: %v", err)
	}
	if got != fixture.original {
		t.Fatalf("export = %q, want unchanged original B content %q", got, fixture.original)
	}
}

func TestBuildProofreadingExportUsesSavedLegacyPageOwnershipWhenLayoutDiffers(t *testing.T) {
	fixture := newProofreadingExportFixture(t, ".txt", false, nil, nil)
	stored, found, err := fixture.store.LoadProofreadingProject(fixture.project.ID)
	if err != nil || !found {
		t.Fatalf("load legacy export fixture: found=%v err=%v", found, err)
	}
	payload, err := decodeProofreadingExportPayload(stored)
	if err != nil {
		t.Fatal(err)
	}
	reconstructedEnds, err := reconstructProofreadingOriginalPageEnds(context.Background(), stored, "Go", fontcatalog.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	shiftedEnds := append([]int(nil), reconstructedEnds...)
	shifted := false
	for i := 0; i+1 < len(shiftedEnds); i++ {
		previous := 0
		if i > 0 {
			previous = shiftedEnds[i-1]
		}
		if shiftedEnds[i]+1 < shiftedEnds[i+1] {
			shiftedEnds[i]++
			shifted = true
			break
		}
		if shiftedEnds[i] > previous+1 {
			shiftedEnds[i]--
			shifted = true
			break
		}
	}
	if !shifted {
		t.Fatal("fixture has no movable page boundary")
	}
	model, err := proofreading.New(fixture.original, []proofreading.PageID{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if err := model.AssignInitial(shiftedEnds); err != nil {
		t.Fatal(err)
	}
	if err := model.ReplacePage(4, "CONFIRMED"); err != nil {
		t.Fatal(err)
	}
	if err := model.Review(4, true); err != nil {
		t.Fatal(err)
	}
	snapshot := model.Snapshot()
	payload.Draft.Model = &snapshot
	var current strings.Builder
	for _, run := range snapshot.Runs {
		current.WriteString(run.Text)
	}
	payload.Draft.Text = current.String()
	payload.Draft.OriginalPageEnds = nil
	payload.Confirmed = map[int]string{3: proofreadingExportCurrentPageHash(stored.ID, payload.Draft.SourceAHash, model, 4)}
	stored.Payload, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.SaveProofreadingProject(stored); err != nil {
		t.Fatal(err)
	}
	wantPages, err := proofreadingOriginalPagesFromEnds(fixture.original, shiftedEnds)
	if err != nil {
		t.Fatal(err)
	}
	var want strings.Builder
	for i := 0; i < len(wantPages)-1; i++ {
		want.WriteString(wantPages[i])
	}
	want.WriteString("CONFIRMED")
	got, _, err := buildProofreadingProjectExport(context.Background(), fixture.store, fixture.project, "Go", fontcatalog.Snapshot{})
	if err != nil {
		t.Fatalf("buildProofreadingProjectExport with edited legacy page ownership: %v", err)
	}
	if got != want.String() {
		t.Fatalf("legacy export = %q, want saved page ownership result %q", got, want.String())
	}
}

func TestInferLegacyProofreadingPageEndsRejectsContradictoryOwnership(t *testing.T) {
	snapshot := proofreading.Snapshot{
		Version: 2,
		Next:    5,
		Pages:   []proofreading.Page{{ID: 1}, {ID: 2}},
		Runs: []proofreading.Run{
			{First: 3, Text: "cd", Page: 1},
			{First: 1, Text: "ab", Page: 2},
		},
	}
	if _, err := inferLegacyProofreadingPageEnds(snapshot, "abcd", []int{2, 4}); err == nil || !strings.Contains(err.Error(), "page order") {
		t.Fatalf("inferLegacyProofreadingPageEnds error = %v, want contradictory page ownership error", err)
	}
}

func prepareLegacyProofreadingExportFixture(t *testing.T, fixture *proofreadingExportFixture, confirmSecondPage bool) {
	t.Helper()
	stored, found, err := fixture.store.LoadProofreadingProject(fixture.project.ID)
	if err != nil || !found {
		t.Fatalf("load legacy export fixture: found=%v err=%v", found, err)
	}
	payload, err := decodeProofreadingExportPayload(stored)
	if err != nil {
		t.Fatal(err)
	}
	ends, err := reconstructProofreadingOriginalPageEnds(context.Background(), stored, "Go", fontcatalog.Snapshot{})
	if err != nil {
		t.Fatalf("construct legacy ownership fixture: %v", err)
	}
	model, err := proofreading.New(fixture.original, []proofreading.PageID{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if err := model.AssignInitial(ends); err != nil {
		t.Fatal(err)
	}
	if confirmSecondPage {
		if err := model.ReplacePage(2, "confirmed"); err != nil {
			t.Fatal(err)
		}
		if err := model.Review(2, true); err != nil {
			t.Fatal(err)
		}
		payload.Confirmed = map[int]string{1: proofreadingExportCurrentPageHash(stored.ID, payload.Draft.SourceAHash, model, 2)}
	}
	snapshot := model.Snapshot()
	payload.Draft.Model = &snapshot
	payload.Draft.Text = fixture.original
	payload.Draft.OriginalPageEnds = nil
	stored.Payload, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.SaveProofreadingProject(stored); err != nil {
		t.Fatal(err)
	}
	fixture.project = stored
}

func TestBuildProofreadingExportRejectsUnrecoverableLegacyPageMap(t *testing.T) {
	fixture := newProofreadingExportFixture(t, ".txt", false, nil, nil)
	if err := os.WriteFile(fixture.project.SourceA, []byte("not a PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture.project.SourceAHash = ""
	if err := fixture.store.SaveProofreadingProject(fixture.project); err != nil {
		t.Fatal(err)
	}
	_, _, err := buildProofreadingProjectExport(context.Background(), fixture.store, fixture.project, "Go", fontcatalog.Snapshot{})
	if err == nil {
		t.Fatal("buildProofreadingProjectExport() error = nil, want unverifiable legacy mapping error")
	}
}

func TestBuildProofreadingExportPreservesSupportedSourceExtension(t *testing.T) {
	for _, extension := range []string{".md", ".markdown", ".txt"} {
		t.Run(extension, func(t *testing.T) {
			fixture := newProofreadingExportFixture(t, extension, true, nil, nil)
			_, got, err := buildProofreadingProjectExport(context.Background(), fixture.store, fixture.project, "Go", fontcatalog.Snapshot{})
			if err != nil {
				t.Fatal(err)
			}
			if got != extension {
				t.Fatalf("extension = %q, want %q", got, extension)
			}
		})
	}
}

func TestProofreadingMarkdownExportPreservesInlineFormatting(t *testing.T) {
	raw := "# Book\n\nIntro **bold** and $\\underline{underlined}$ text.\n"
	original, _, err := proofreadingMarkdownProjection(raw)
	if err != nil {
		t.Fatalf("proofreadingMarkdownProjection: %v", err)
	}
	for _, tc := range []struct {
		name, before, after, want string
	}{
		{name: "bold", before: "bold", after: "strong", want: "# Book\n\nIntro **strong** and $\\underline{underlined}$ text.\n"},
		{name: "underline", before: "underlined", after: "emphasized", want: "# Book\n\nIntro **bold** and $\\underline{emphasized}$ text.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, err := proofreading.New(original, []proofreading.PageID{1})
			if err != nil {
				t.Fatal(err)
			}
			if err := model.AssignInitial([]int{len([]rune(original))}); err != nil {
				t.Fatal(err)
			}
			changed := strings.Replace(original, tc.before, tc.after, 1)
			if changed == original {
				t.Fatalf("test edit %q was not present in original projection", tc.before)
			}
			if err := model.ReplacePage(1, changed); err != nil {
				t.Fatal(err)
			}
			got, err := preserveProofreadingMarkdownFormatting(raw, original, model.Snapshot().Runs)
			if err != nil {
				t.Fatalf("preserveProofreadingMarkdownFormatting: %v", err)
			}
			if got != tc.want {
				t.Fatalf("formatted export = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProofreadingMarkdownExportPreservesHTMLUnderline(t *testing.T) {
	raw := "Intro <u>underlined</u> text."
	original, _, err := proofreadingMarkdownProjection(raw)
	if err != nil {
		t.Fatalf("proofreadingMarkdownProjection: %v", err)
	}
	model, err := proofreading.New(original, []proofreading.PageID{1})
	if err != nil {
		t.Fatal(err)
	}
	if err := model.AssignInitial([]int{len([]rune(original))}); err != nil {
		t.Fatal(err)
	}
	if err := model.ReplacePage(1, strings.Replace(original, "underlined", "emphasized", 1)); err != nil {
		t.Fatal(err)
	}
	got, err := preserveProofreadingMarkdownFormatting(raw, original, model.Snapshot().Runs)
	if err != nil {
		t.Fatalf("preserveProofreadingMarkdownFormatting: %v", err)
	}
	want := "Intro <u>emphasized</u> text."
	if got != want {
		t.Fatalf("formatted export = %q, want %q", got, want)
	}
}

func TestBuildProofreadingMarkdownExportPreservesEditedFormatting(t *testing.T) {
	raw := "**aa**\n\n**bb**\n\n**cc**\n\n**dd**"
	want := "**strong**\n\n**bb**\n\n**cc**\n\n**dd**"
	for _, tc := range []struct {
		name      string
		confirmed map[int]string
	}{
		{name: "all pages confirmed", confirmed: map[int]string{0: "strong", 1: "\nbb", 2: "\ncc", 3: "\ndd"}},
		{name: "unreviewed pages retain original formatting", confirmed: map[int]string{0: "strong"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newProofreadingExportFixture(t, ".md", true, tc.confirmed, nil, raw)
			got, extension, err := buildProofreadingProjectExport(context.Background(), fixture.store, fixture.project, "Go", fontcatalog.Snapshot{})
			if err != nil {
				t.Fatalf("buildProofreadingProjectExport: %v", err)
			}
			if extension != ".md" || got != want {
				t.Fatalf("export = (%q, %q), want (%q, %q)", got, extension, want, ".md")
			}
		})
	}
}

func TestBuildProofreadingExportRejectsWordBeforeReadingSources(t *testing.T) {
	project := workspace.ProofreadingProject{ID: "word", SourceB: filepath.Join(t.TempDir(), "source.docx")}
	_, _, err := buildProofreadingProjectExport(context.Background(), nil, project, "Go", fontcatalog.Snapshot{})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "docx") {
		t.Fatalf("buildProofreadingProjectExport error = %v, want unsupported Word format", err)
	}
}

func TestWriteProofreadingProjectExportAtomicallyReplacesTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.md")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeProofreadingProjectExport(path, "new document"); err != nil {
		t.Fatalf("writeProofreadingProjectExport: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new document" {
		t.Fatalf("target = %q, %v; want atomically replaced content", got, err)
	}
	if err = writeProofreadingProjectExport(dir, "failure"); err == nil {
		t.Fatal("writeProofreadingProjectExport() error = nil for directory target")
	}
}

func newProofreadingExportFlowFixture(t *testing.T, extension string) (*App, *proofreadingWorkbenchState, proofreadingExportFixture) {
	t.Helper()
	fixture := newProofreadingExportFixture(t, extension, true, nil, nil)
	a := New(nil, "")
	a.managedBackgroundClosed = false
	a.shutdown = make(chan struct{})
	a.desktopRuntime.workspaceStore = fixture.store
	t.Cleanup(func() {
		close(a.shutdown)
		a.stopAndWaitManagedBackground()
	})
	w := a.ensureProofreadingWorkbench()
	return a, w, fixture
}

func finishProofreadingExportFlow(a *App) {
	a.managedBackgroundTasks.Wait()
	a.processProofreadingWorkbench()
}

func TestProofreadingWorkbenchRowExportButtonIsRegistered(t *testing.T) {
	a, _ := proofreadingModelFixture(t)
	project := workspace.ProofreadingProject{ID: "export-row", Title: "Book", PageCount: 4}
	var ops op.Ops
	g := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(988, 0), Max: image.Pt(988, 800)}}
	a.layoutProofreadingProjectRow(g, project)
	buttons := a.ensureProofreadingWorkbench().buttons
	for _, id := range []string{"delete-" + project.ID, "open-" + project.ID, "export-" + project.ID} {
		if buttons[id] == nil {
			t.Errorf("expected stable row action button %q", id)
		}
	}
	for _, icon := range []string{"trash-2", "pencil", "download"} {
		if _, ok := svgIconTintAtSize("home", icon, 14, proofreadingDesignBlue); !ok {
			t.Errorf("proofreading row action icon %q is unavailable", icon)
		}
	}
}

func TestProofreadingExportFlowUsesOriginalExtensionAndBasename(t *testing.T) {
	for _, extension := range []string{".md", ".markdown", ".txt"} {
		t.Run(extension, func(t *testing.T) {
			a, w, fixture := newProofreadingExportFlowFixture(t, extension)
			wantDefault := "Reference-校对进度(0／4)"
			chosen := filepath.Join(t.TempDir(), "chosen"+extension)
			var gotExtension, gotDefault string
			w.exportPathPicker = func(ext string, labels filePickerLabels) (string, error) {
				gotExtension, gotDefault = ext, labels.UntitledFile
				return chosen, nil
			}
			a.startProofreadingProjectExport(fixture.project)
			finishProofreadingExportFlow(a)
			if gotExtension != strings.TrimPrefix(extension, ".") || gotDefault != wantDefault {
				t.Fatalf("picker ext/default = %q/%q, want %q/%q", gotExtension, gotDefault, strings.TrimPrefix(extension, "."), wantDefault)
			}
			if _, err := os.Stat(chosen); err != nil {
				t.Fatalf("export output %q was not created: %v (export detail: %s)", chosen, err, a.feedback.detail)
			}
			if w.message != "" || a.feedback.messageRef.ID != "proofread.export.success" || !strings.Contains(a.text(a.feedback.messageRef), chosen) || !a.feedback.toastVisible(time.Now()) {
				t.Fatalf("success should appear as a toast without changing workbench layout: workbench=%q toast=%q", w.message, a.text(a.feedback.messageRef))
			}
		})
	}
}

func TestProofreadingExportFilenameUsesANameAndProgress(t *testing.T) {
	a, w, fixture := newProofreadingExportFlowFixture(t, ".md")
	project := fixture.project
	project.SourceBName = "manuscript.md"
	project.SourceBOriginalPath = "/books/唐诗.增订版.md"
	project.Title = "唐诗汇评.增订版.pdf"
	project.ReviewedCount, project.PageCount = 2, 892
	w.exportPathPicker = func(ext string, labels filePickerLabels) (string, error) {
		if ext != "md" || labels.UntitledFile != "唐诗汇评.增订版-校对进度(2／892)" {
			t.Errorf("picker filename = %q.%s", labels.UntitledFile, ext)
		}
		return "", nil
	}
	a.startProofreadingProjectExport(project)
	finishProofreadingExportFlow(a)
}

func TestProofreadingExportFlowRejectsExtensionMismatchWithoutOverwritingExpectedExtension(t *testing.T) {
	a, w, fixture := newProofreadingExportFlowFixture(t, ".md")
	dir := t.TempDir()
	selected := filepath.Join(dir, "report.txt")
	protectedExpected := filepath.Join(dir, "report.md")
	if err := os.WriteFile(protectedExpected, []byte("keep this"), 0600); err != nil {
		t.Fatal(err)
	}
	w.exportPathPicker = func(string, filePickerLabels) (string, error) { return selected, nil }
	a.startProofreadingProjectExport(fixture.project)
	finishProofreadingExportFlow(a)
	if got, err := os.ReadFile(protectedExpected); err != nil || string(got) != "keep this" {
		t.Fatalf("expected-extension target = %q, %v; want untouched file", got, err)
	}
	if _, err := os.Stat(selected); !os.IsNotExist(err) {
		t.Fatalf("mismatched target exists, stat error = %v", err)
	}
	if w.message != "" || a.feedback.messageRef.ID != "proofread.export.extension_mismatch" || !a.feedback.toastVisible(time.Now()) {
		t.Fatalf("extension mismatch should appear as a toast: workbench=%q toast=%+v", w.message, a.feedback.messageRef)
	}
}

func TestValidateProofreadingExportTargetProtectsSourcesProjectAndRecoveryFiles(t *testing.T) {
	fixture := newProofreadingExportFixture(t, ".md", true, nil, nil)
	draft, err := proofreadingDraftFilePath(fixture.project.SourceB, fixture.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"retained Source A":  fixture.project.SourceA,
		"original Source A":  fixture.project.SourceAOriginalPath,
		"retained Source B":  fixture.project.SourceB,
		"original Source B":  fixture.project.SourceBOriginalPath,
		"draft recovery":     draft,
		"previous recovery":  draft + ".previous",
		"project directory":  filepath.Join(fixture.project.Directory, "export.md"),
		"workspace database": fixture.store.Path(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateProofreadingExportTarget(path, ".md", fixture.project, fixture.store); !errors.Is(err, errProofreadingExportProtectedPath) {
				t.Fatalf("validateProofreadingExportTarget(%q) = %v, want protected path error", path, err)
			}
		})
	}
	alias := filepath.Join(t.TempDir(), "source-alias.md")
	if err := os.Symlink(fixture.project.SourceBOriginalPath, alias); err != nil {
		t.Fatalf("create source alias symlink: %v", err)
	}
	if err := validateProofreadingExportTarget(alias, ".md", fixture.project, fixture.store); !errors.Is(err, errProofreadingExportProtectedPath) {
		t.Fatalf("symlink to original Source B returned %v, want protected path error", err)
	}
	projectAlias := filepath.Join(filepath.Dir(fixture.project.Directory), "task-alias")
	if err := os.Symlink(fixture.project.Directory, projectAlias); err != nil {
		t.Fatalf("create project directory symlink: %v", err)
	}
	if err := validateProofreadingExportTarget(filepath.Join(projectAlias, "export.md"), ".md", fixture.project, fixture.store); !errors.Is(err, errProofreadingExportProtectedPath) {
		t.Fatalf("symlinked project directory returned %v, want protected path error", err)
	}
}

func TestProofreadingExportFlowRejectsWordBeforeShowingPicker(t *testing.T) {
	a, w, fixture := newProofreadingExportFlowFixture(t, ".md")
	fixture.project.SourceB = filepath.Join(t.TempDir(), "source.docx")
	called := false
	w.exportPathPicker = func(string, filePickerLabels) (string, error) {
		called = true
		return "", nil
	}
	a.startProofreadingProjectExport(fixture.project)
	if called || w.busy || w.message != "" || a.feedback.messageRef.ID != "proofread.export.unsupported" || !a.feedback.toastVisible(time.Now()) {
		t.Fatalf("unsupported export called picker or lost toast feedback: called=%v busy=%v workbench=%q toast=%+v", called, w.busy, w.message, a.feedback.messageRef)
	}
}

func TestProofreadingExportProgressUsesToastWithoutChangingWorkbenchLayoutState(t *testing.T) {
	a, w, fixture := newProofreadingExportFlowFixture(t, ".md")
	w.exportPathPicker = func(string, filePickerLabels) (string, error) {
		return "", nil
	}
	a.startProofreadingProjectExport(fixture.project)
	if w.message != "" {
		t.Errorf("export progress changed workbench layout message to %q", w.message)
	}
	if a.feedback.messageRef.ID != "proofread.export.progress" || !a.feedback.toastVisible(time.Now()) {
		t.Errorf("export progress is not visible as a toast: message=%+v policy=%v", a.feedback.messageRef, a.feedback.policy)
	}
	finishProofreadingExportFlow(a)
	if a.feedback.toastVisible(time.Now()) {
		t.Fatal("cancelled export left its progress toast visible")
	}
}

func TestProofreadingExportFlowWaitsForOwnedEditorDraftSave(t *testing.T) {
	fixture := newProofreadingExportFixture(t, ".md", true, nil, nil)
	runtime := newDesktopRuntime()
	host := newAppWithRuntime(nil, runtime)
	host.managedBackgroundClosed = false
	host.shutdown = make(chan struct{})
	host.desktopRuntime.workspaceStore = fixture.store
	t.Cleanup(func() {
		close(host.shutdown)
		host.stopAndWaitManagedBackground()
	})

	_, liveState := proofreadingModelFixture(t)
	liveState.projectDocumentID = fixture.project.ID
	liveState.draftSavePending = true
	liveState.draftAutoSaved = true
	editor := newAppWithRuntime(nil, runtime)
	editor.proofreading = liveState
	host.desktopRuntime.workspaceStore = fixture.store
	host.proofreadingTabs = []*proofreadingTab{{id: "proofreading:export", editor: editor}}
	host.registerProofreadingTab(host.proofreadingTabs[0], fixture.project.ID)
	w := host.ensureProofreadingWorkbench()
	pickerCalled := false
	w.exportPathPicker = func(string, filePickerLabels) (string, error) {
		pickerCalled = true
		return filepath.Join(t.TempDir(), "export.md"), nil
	}
	host.startProofreadingProjectExport(fixture.project)
	if pickerCalled || w.busy || w.message != "" || host.feedback.messageRef.ID != "proofread.export.wait" || !host.feedback.toastVisible(time.Now()) {
		t.Fatalf("pending owner draft should block export with a toast: picker=%v busy=%v workbench=%q toast=%+v", pickerCalled, w.busy, w.message, host.feedback.messageRef)
	}
}

func TestProofreadingExportUsesSnapshotCapturedBeforeSavePicker(t *testing.T) {
	fixture := newProofreadingExportFixture(t, ".txt", true, nil, nil)
	stored, found, err := fixture.store.LoadProofreadingProject(fixture.project.ID)
	if err != nil || !found {
		t.Fatalf("load saved export fixture: found=%v err=%v", found, err)
	}
	payload, err := decodeProofreadingExportPayload(stored)
	if err != nil {
		t.Fatal(err)
	}
	model, err := proofreading.Restore(*payload.Draft.Model)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.ReplacePage(2, "LIVE"); err != nil {
		t.Fatal(err)
	}
	if err := model.Review(2, true); err != nil {
		t.Fatal(err)
	}
	pageHash := proofreadingExportCurrentPageHash(stored.ID, payload.Draft.SourceAHash, model, 2)

	runtime := newDesktopRuntime()
	host := newAppWithRuntime(nil, runtime)
	host.managedBackgroundClosed = false
	host.shutdown = make(chan struct{})
	t.Cleanup(func() {
		close(host.shutdown)
		host.stopAndWaitManagedBackground()
	})
	editor := newAppWithRuntime(nil, runtime)
	state := &proofreadingWorkspace{
		projectDocumentID: fixture.project.ID, controller: application.NewProofreadingController(model),
		aSourceHash: payload.Draft.SourceAHash, sourceBHash: payload.Draft.SourceBHash,
		originalPageEnds: append([]int(nil), payload.Draft.OriginalPageEnds...),
		history:          proofreadingHistoryState{confirmed: map[int]string{1: pageHash}}, draftAutoSaved: true,
	}
	editor.proofreading = state
	host.desktopRuntime.workspaceStore = fixture.store
	host.proofreadingTabs = []*proofreadingTab{{id: "proofreading:export", editor: editor}}
	host.registerProofreadingTab(host.proofreadingTabs[0], fixture.project.ID)

	w := host.ensureProofreadingWorkbench()
	enteredPicker := make(chan struct{})
	releasePicker := make(chan struct{})
	output := filepath.Join(t.TempDir(), "captured.txt")
	w.exportPathPicker = func(string, filePickerLabels) (string, error) {
		close(enteredPicker)
		<-releasePicker
		return output, nil
	}
	host.startProofreadingProjectExport(fixture.project)
	select {
	case <-enteredPicker:
	case <-time.After(3 * time.Second):
		t.Fatal("export did not open the save picker")
	}
	if err := state.controller.Model.ReplacePage(2, "LATER"); err != nil {
		t.Fatal(err)
	}
	close(releasePicker)
	finishProofreadingExportFlow(host)
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if want := "aaLIVE\ncc\ndd"; string(got) != want {
		t.Fatalf("export = %q, want captured pre-picker snapshot %q", got, want)
	}
}

func TestProofreadingExportFlowCancelDoesNotWrite(t *testing.T) {
	a, w, fixture := newProofreadingExportFlowFixture(t, ".md")
	w.exportPathPicker = func(string, filePickerLabels) (string, error) { return "", nil }
	a.startProofreadingProjectExport(fixture.project)
	finishProofreadingExportFlow(a)
	if w.message != "" || a.feedback.messageRef.ID != "" {
		t.Fatalf("cancel should clear the export progress toast without changing workbench layout: workbench=%q toast=%+v", w.message, a.feedback.messageRef)
	}
}

func TestProofreadingExportFlowReportsWriteError(t *testing.T) {
	a, w, fixture := newProofreadingExportFlowFixture(t, ".md")
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "output.md")
	w.exportPathPicker = func(string, filePickerLabels) (string, error) { return path, nil }
	a.startProofreadingProjectExport(fixture.project)
	finishProofreadingExportFlow(a)
	if w.message != "" || a.feedback.messageRef.ID != "proofread.export.failed_short" || a.feedback.detail == "" || !a.feedback.toastVisible(time.Now()) {
		t.Fatalf("write failure should appear as a detailed toast: workbench=%q toast=%+v detail=%q", w.message, a.feedback.messageRef, a.feedback.detail)
	}
}

func TestProofreadingExportFlowReportsConfirmedPageNumber(t *testing.T) {
	fixture := newProofreadingExportFixture(t, ".md", true, map[int]string{2: "confirmed"}, map[int]string{2: "changed after confirmation"})
	a := New(nil, "")
	a.managedBackgroundClosed = false
	a.shutdown = make(chan struct{})
	a.desktopRuntime.workspaceStore = fixture.store
	t.Cleanup(func() {
		close(a.shutdown)
		a.stopAndWaitManagedBackground()
	})
	w := a.ensureProofreadingWorkbench()
	w.exportPathPicker = func(string, filePickerLabels) (string, error) {
		return filepath.Join(t.TempDir(), "export.md"), nil
	}
	a.startProofreadingProjectExport(fixture.project)
	finishProofreadingExportFlow(a)
	if w.message != "" || a.feedback.messageRef.ID != "proofread.export.confirm_page" || !strings.Contains(a.text(a.feedback.messageRef), "3") || !a.feedback.toastVisible(time.Now()) {
		t.Fatalf("page-specific toast = %q, want page 3 without changing workbench layout", a.text(a.feedback.messageRef))
	}
}

func TestProofreadingMarkdownExportPreservesLayoutAtBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, raw, before, after, want string }{
		{"paragraph end", "# 标题\n\n**原文**\n\n下一段\n", "原文", "原文补充", "# 标题\n\n**原文补充**\n\n下一段\n"},
		{"paragraph start", "第一段\n\n**原文**\n", "原文", "补充原文", "第一段\n\n**补充原文**\n"},
		{"table", "| 名称 | 内容 |\n| --- | --- |\n| 原文 | **粗体** |\n", "原文", "新文", "| 名称 | 内容 |\n| --- | --- |\n| 新文 | **粗体** |\n"},
		{"code", "```txt\n原文\n```\n", "原文", "新文", "```txt\n新文\n```\n"},
		{"inline code", "文字 `原文` 结尾\n", "原文", "新文", "文字 `新文` 结尾\n"},
		{"list end", "- **原文**\n- 下一项\n", "原文", "原文补充", "- **原文补充**\n- 下一项\n"},
		{"code punctuation", "```txt\n原文\n```\n", "原文", "a_b", "```txt\na_b\n```\n"},
		{"math block", "$$\nx_1\n$$\n", "x_1", "y_2", "$$\ny_2\n$$\n"},
		{"line ending", "**原文**\r\n下一行\r\n", "原文", "新文", "**新文**\r\n下一行\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, _, err := proofreadingMarkdownProjection(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			model, err := proofreading.New(original, []proofreading.PageID{1})
			if err != nil {
				t.Fatal(err)
			}
			if err := model.AssignInitial([]int{len([]rune(original))}); err != nil {
				t.Fatal(err)
			}
			if err := model.ReplacePage(1, strings.Replace(original, tc.before, tc.after, 1)); err != nil {
				t.Fatal(err)
			}
			got, err := preserveProofreadingMarkdownFormatting(tc.raw, original, model.Snapshot().Runs)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("export = %q, want %q", got, tc.want)
			}
		})
	}
}
