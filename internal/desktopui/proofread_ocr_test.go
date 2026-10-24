package desktopui

import (
	"context"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/pdfdoc"
	"glance/internal/i18n"
	"glance/internal/ocr"
	"glance/internal/proofreading"
	"glance/internal/textalign"
	"glance/internal/workspace"
)

func TestProofreadingOCRTokenDialogMasksAndDoesNotRetainTokenOnCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.md")
	if err := os.WriteFile(path, []byte("# 卷一\n\n正文"), 0o600); err != nil {
		t.Fatal(err)
	}
	object, err := loadProofreadingObject(path)
	if err != nil {
		t.Fatal(err)
	}
	defer object.Adapter.Close()

	a := New(nil, "")
	a.proofreading = &proofreadingWorkspace{
		bDocument: object,
		ocrScan:   newProofreadingOCRScan(),
	}
	a.pdfDoc = &pdfdoc.Document{}
	a.proofreading.ocrScan.button.Click()
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(500, 54))}
	a.layoutProofreadingOCRAction(gtx)
	if !a.modal.Visible || !a.modal.HasInput || a.modal.Input.Mask != '•' {
		t.Fatalf("token dialog not visible and masked: visible=%v input=%v mask=%q", a.modal.Visible, a.modal.HasInput, a.modal.Input.Mask)
	}
	if a.modal.Input.Len() != 0 {
		t.Fatal("token dialog should open empty")
	}
	a.modal.Input.SetText("synthetic-token")
	a.modal.OnAction("cancel")
	if a.modal.Visible || a.modal.Input.Len() != 0 || a.modal.Input.Mask != 0 {
		t.Fatalf("cancel retained token or left dialog open: visible=%v input=%q mask=%q", a.modal.Visible, a.modal.Input.Text(), a.modal.Input.Mask)
	}
}

func TestProofreadingWorkspaceRendersTokenModalActions(t *testing.T) {
	a := New(nil, "")
	a.managedBackgroundClosed = true
	a.proofreading = &proofreadingWorkspace{ocrScan: newProofreadingOCRScan()}
	a.modal.ShowInputMessage(
		"proofread.ocr.token",
		i18n.Message{ID: "proofread.ai.token.title"},
		i18n.Message{ID: "proofread.ai.token.body"},
		"",
		i18n.Message{ID: "proofread.ai.token.hint"},
		func(string) {},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		modalDialogActionSpec{ID: "start", LabelRef: i18n.Message{ID: "proofread.ai.scan.start"}},
	)
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Source:      router.Source(),
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1280, 800)),
	}
	a.layout(gtx)
	router.Frame(&ops)

	want := a.textID("proofread.ai.scan.start")
	for _, label := range collectSemanticLabels(&router) {
		if label == want {
			return
		}
	}
	t.Fatalf("proofreading root layout did not register token modal action %q", want)
}

func TestProofreadingTabOCRClickRendersOwnedTokenModal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.md")
	if err := os.WriteFile(path, []byte("# 卷一\n\n正文"), 0o600); err != nil {
		t.Fatal(err)
	}
	object, err := loadProofreadingObject(path)
	if err != nil {
		t.Fatal(err)
	}
	defer object.Adapter.Close()

	runtime := newDesktopRuntime()
	host := newAppWithRuntime(nil, runtime)
	host.managedBackgroundClosed = true
	editor := newAppWithRuntime(nil, runtime)
	editor.managedBackgroundClosed = true
	editor.pdfDoc = &pdfdoc.Document{}
	editor.proofreading = &proofreadingWorkspace{
		aDocument:     &proofreadingObject{Name: "book.pdf", Path: "book.pdf"},
		bDocument:     object,
		bDraftText:    "# 卷一\n\n正文",
		sourceLoading: true,
		ocrScan:       newProofreadingOCRScan(),
	}
	tab := &proofreadingTab{id: "proofreading:test", editor: editor}
	host.proofreadingTabs = []*proofreadingTab{tab}
	host.selectedProofreadingTabID = tab.id

	var ops op.Ops
	var router input.Router
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1280, 800)),
		}
		editor.layoutProofreadingOCRAction(gtx)
		host.layoutRootWithOverlays(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
		router.Frame(&ops)
	}
	frame()
	point := f32.Pt(71, 16)
	router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point},
	)
	frame()

	if !editor.modal.Visible || !editor.modal.HasInput {
		t.Fatalf("OCR click did not open the tab-owned token input: visible=%v input=%v", editor.modal.Visible, editor.modal.HasInput)
	}
	want := editor.textID("common.action.cancel")
	for _, label := range collectSemanticLabels(&router) {
		if label == want {
			return
		}
	}
	t.Fatalf("host root did not render the selected tab's token modal action %q", want)
}

func TestProofreadingOCRLayoutPreservesPageOrderAndBlockGeometry(t *testing.T) {
	pages := []ocr.Page{
		{Raw: []byte(`{"prunedResult":{"width":100,"height":200,"parsing_res_list":[{"block_id":8,"block_label":"text","block_content":"後頁","block_bbox":[10,20,90,180],"block_order":2},{"block_id":3,"block_label":"vertical_text","block_content":"前頁","block_bbox":[2,4,8,100],"block_order":1}]}}`)},
	}
	layouts, aligned, err := proofreadingOCRAlignmentPages(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(layouts) != 1 || len(aligned) != 1 || aligned[0].Number != 1 {
		t.Fatalf("wrong page output: layouts=%d pages=%+v", len(layouts), aligned)
	}
	if len(aligned[0].Blocks) != 2 || aligned[0].Blocks[0].ID != 3 || aligned[0].Blocks[0].Text != "前頁" || aligned[0].Blocks[1].ID != 8 {
		t.Fatalf("block reading order not preserved: %+v", aligned[0].Blocks)
	}
	if aligned[0].Blocks[0].Bounds != [4]float64{2, 4, 8, 100} {
		t.Fatalf("block coordinates lost: %+v", aligned[0].Blocks[0].Bounds)
	}
}

func TestProofreadingOCRScanCancelsWhenCurrentDraftChanges(t *testing.T) {
	scan := newProofreadingOCRScan()
	scan.generation = 4
	scan.stage = proofreadingOCRRunning
	scan.markdownSnapshot = "原始 B 文本"
	scan.markdownSnapshotSet = true
	a := &App{proofreading: &proofreadingWorkspace{
		bDraftText: "修改后的 B 文本",
		ocrScan:    scan,
	}}

	a.processProofreadingOCRScan(layout.Context{})

	if scan.stage != proofreadingOCRCancelled || scan.notice != "proofread.ai.status.draft_changed" {
		t.Fatalf("changed B draft did not stop OCR scan: stage=%v notice=%q", scan.stage, scan.notice)
	}
	if len(scan.pages) != 0 {
		t.Fatalf("stale OCR page results remained visible: %d pages", len(scan.pages))
	}
}

func TestProofreadingOCRResultsClearWhenDraftChangesAfterScan(t *testing.T) {
	scan := newProofreadingOCRScan()
	scan.stage = proofreadingOCRComplete
	scan.markdownSnapshot = "原始 B 文本"
	scan.markdownSnapshotSet = true
	scan.pages = []proofreadingOCRPage{{result: textalign.Result{Status: "matched"}}}
	a := &App{proofreading: &proofreadingWorkspace{
		bDraftText: "修改后的 B 文本",
		ocrScan:    scan,
	}}

	a.processProofreadingOCRScan(layout.Context{})

	if scan.stage != proofreadingOCRCancelled || scan.notice != "proofread.ai.status.draft_changed" || len(scan.pages) != 0 {
		t.Fatalf("editing B left old alignment visible: stage=%v notice=%q pages=%d", scan.stage, scan.notice, len(scan.pages))
	}
}

func TestProofreadingOCRResultsClearWhenSourceHashChangesAfterScan(t *testing.T) {
	scan := newProofreadingOCRScan()
	scan.stage = proofreadingOCRComplete
	scan.markdownSnapshot = "B 文本"
	scan.markdownSnapshotSet = true
	scan.sourceAHash = "old-A"
	scan.sourceBHash = "old-B"
	scan.pages = []proofreadingOCRPage{{result: textalign.Result{Status: "matched"}}}
	a := &App{proofreading: &proofreadingWorkspace{
		aSourceHash: "new-A", sourceBHash: "old-B", bDraftText: "B 文本", ocrScan: scan,
	}}
	a.processProofreadingOCRScan(layout.Context{})
	if scan.stage != proofreadingOCRCancelled || scan.notice != "proofread.ai.status.resume_source_changed" || len(scan.pages) != 0 {
		t.Fatalf("changed source retained stale OCR results: stage=%v notice=%q pages=%d", scan.stage, scan.notice, len(scan.pages))
	}
}

func TestProofreadingOCRCompletionAttachesConservativeMissingPageCandidate(t *testing.T) {
	markdown := "甲乙"
	scan := newProofreadingOCRScan()
	scan.stage = proofreadingOCRMatching
	scan.markdownSnapshot = markdown
	scan.markdownSnapshotSet = true
	scan.sourceAHash = "pdf-hash"
	scan.sourceBHash = "b-source-hash"
	scan.results <- proofreadingOCRResult{
		pages: []proofreadingOCRPage{
			ocrPageWithMatch("matched", 0, 1, "甲"),
			ocrPageWithMatch("unmatched", -1, -1, "OCR 缺文"),
			ocrPageWithMatch("matched", 1, 2, "乙"),
		},
	}
	a := &App{proofreading: &proofreadingWorkspace{
		aSourceHash: "pdf-hash", sourceBHash: "b-source-hash", bDraftText: markdown,
		xPages: []proofreadingXPage{
			{ContentStart: 0, ContentEnd: 1, Content: "甲"},
			{ContentStart: 1, ContentEnd: 1, Content: ""},
			{ContentStart: 1, ContentEnd: 2, Content: "乙"},
		},
		ocrScan: scan,
	}}
	var ops op.Ops
	a.processProofreadingOCRScan(layout.Context{Ops: &ops})

	if scan.stage != proofreadingOCRComplete || len(scan.pages) != 3 {
		t.Fatalf("OCR completion not applied: stage=%v pages=%d", scan.stage, len(scan.pages))
	}
	candidate := scan.pages[1].missingPage
	if candidate == nil || candidate.APage != 1 || candidate.BPage != 1 || candidate.Preview != "OCR 缺文" {
		t.Fatalf("unique blank page candidate not attached: %#v", candidate)
	}
	if candidate.SourceAHash != "pdf-hash" || candidate.SourceBHash != "b-source-hash" || candidate.DraftHash != proofreadingOCRDraftSnapshotHash(markdown) {
		t.Fatalf("candidate did not capture source snapshots: %#v", candidate)
	}
}

func TestProofreadingOCRMissingPagePreviewRequiresCandidateAndCancelDoesNotEdit(t *testing.T) {
	a, s := proofreadingOCRMissingPageModelFixture(t)
	before := s.controller.Model.Snapshot()
	if !a.openProofreadingOCRMissingPagePreview(1) || !a.modal.Visible || s.ocrPreview == nil {
		t.Fatal("candidate preview did not open")
	}
	a.modal.OnAction("cancel")
	if a.modal.Visible || s.ocrPreview != nil {
		t.Fatal("cancel did not close and clear the OCR preview")
	}
	if got := s.controller.Model.Snapshot(); !reflect.DeepEqual(before, got) {
		t.Fatalf("cancel changed B model: before=%+v after=%+v", before, got)
	}
	if a.openProofreadingOCRMissingPagePreview(0) || a.modal.Visible {
		t.Fatal("page without a unique candidate opened a preview")
	}
}

func TestProofreadingOCRApplyMissingPageReplacesOnlyTargetAndUndoRestoresIt(t *testing.T) {
	a, s := proofreadingOCRMissingPageModelFixture(t)
	if !a.openProofreadingOCRMissingPagePreview(1) {
		t.Fatal("candidate preview did not open")
	}
	a.modal.OnAction("fill")
	if a.modal.Visible || s.ocrPreview != nil {
		t.Fatal("successful fill left preview open")
	}
	if got := s.controller.Model.Text(1); got != "甲" {
		t.Fatalf("left page changed: %q", got)
	}
	if got := s.controller.Model.Text(2); got != "缺失正文" {
		t.Fatalf("target page was not filled: %q", got)
	}
	if got := s.controller.Model.Text(3); got != "乙" {
		t.Fatalf("right page changed: %q", got)
	}
	if target, _ := s.controller.Model.Page(2); target.Reviewed {
		t.Fatal("OCR fill incorrectly marked the target page reviewed")
	}
	if !s.controller.Model.CanUndo() {
		t.Fatal("fill was not recorded in proofreading history")
	}
	a.undoProofreadingPagination()
	if got := s.controller.Model.Text(2); got != "" {
		t.Fatalf("undo did not restore the empty target page: %q", got)
	}
}

func TestProofreadingOCRApplyMissingPageRejectsStaleSourcesDraftAndTarget(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*App, *proofreadingWorkspace)
	}{
		{name: "A source changed", mutate: func(_ *App, s *proofreadingWorkspace) { s.aSourceHash = "new-A" }},
		{name: "B source changed", mutate: func(_ *App, s *proofreadingWorkspace) { s.sourceBHash = "new-B" }},
		{name: "B draft changed", mutate: func(a *App, s *proofreadingWorkspace) {
			if err := s.controller.Model.ReplacePage(1, "改甲"); err != nil {
				t.Fatal(err)
			}
			a.refreshProofreadingModelView()
		}},
		{name: "target is no longer blank", mutate: func(a *App, s *proofreadingWorkspace) {
			if err := s.controller.Model.ReplacePage(2, "已有内容"); err != nil {
				t.Fatal(err)
			}
			a.refreshProofreadingModelView()
		}},
		{name: "page map fingerprint changed", mutate: func(_ *App, s *proofreadingWorkspace) {
			s.ocrPreview.PageMapHash = "old-page-map"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, s := proofreadingOCRMissingPageModelFixture(t)
			if !a.openProofreadingOCRMissingPagePreview(1) {
				t.Fatal("candidate preview did not open")
			}
			before := s.controller.Model.Snapshot()
			test.mutate(a, s)
			beforeConfirm := s.controller.Model.Snapshot()
			a.modal.OnAction("fill")
			if s.ocrPreview != nil || a.modal.Visible {
				t.Fatal("stale preview remained actionable")
			}
			if !reflect.DeepEqual(beforeConfirm, s.controller.Model.Snapshot()) {
				t.Fatalf("stale confirmation changed the model; before=%+v original=%+v", beforeConfirm, before)
			}
			if s.ocrScan.notice != "proofread.ai.status.missing_page_stale" {
				t.Fatalf("stale candidate did not report refresh status: %q", s.ocrScan.notice)
			}
		})
	}
}

func TestProofreadingOCRMissingPageBadgeRendersAndOpensPreviewOnClick(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a, _ := proofreadingOCRMissingPageModelFixture(t)
	size := image.Pt(360, 260)
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
	render := func() *image.RGBA {
		ops.Reset()
		paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		a.layoutProofreadingOCRMissingPageBadge(gtx, size, 1)
		shot := renderPresentationTestOps(t, &ops, size)
		router.Frame(&ops)
		return shot
	}
	shot := render()
	pixel := shot.RGBAAt(315, 18)
	if pixel.R < 80 || pixel.B < 130 || pixel.B <= pixel.G {
		t.Fatalf("missing page badge did not render its distinct color: %v", pixel)
	}
	router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(315, 18)},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(315, 18)},
	)
	render()
	if !a.modal.Visible || a.modal.DialogID != "proofread.ocr.missing-page" {
		t.Fatal("clicking the X page missing-text badge did not open its OCR preview")
	}
}

func proofreadingOCRMissingPageModelFixture(t *testing.T) (*App, *proofreadingWorkspace) {
	t.Helper()
	m, err := proofreading.New("甲乙", []proofreading.PageID{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.AssignInitial([]int{1, 1, 2}); err != nil {
		t.Fatal(err)
	}
	scan := newProofreadingOCRScan()
	scan.stage = proofreadingOCRComplete
	scan.markdownSnapshot = "甲乙"
	scan.markdownSnapshotSet = true
	scan.sourceAHash = "source-A"
	scan.sourceBHash = "source-B"
	scan.pages = []proofreadingOCRPage{
		ocrPageWithMatch("matched", 0, 1, "甲"),
		ocrPageWithMatch("unmatched", -1, -1, "缺失正文"),
		ocrPageWithMatch("matched", 1, 2, "乙"),
	}
	a := New(nil, "")
	a.managedBackgroundClosed = true
	a.proofreading = &proofreadingWorkspace{
		controller: application.NewProofreadingController(m), modelInitialAssigned: true,
		bDocument: &proofreadingObject{}, bDraftInitialized: true,
		pageFrames:  []proofreadingPageFrame{{PageIndex: 0, Width: 400, Height: 600}, {PageIndex: 1, Width: 400, Height: 600}, {PageIndex: 2, Width: 400, Height: 600}},
		aSourceHash: "source-A", sourceBHash: "source-B", ocrScan: scan,
	}
	a.proofreading.controller.Layout.Pages = map[proofreading.PageID]application.ProofreadingPageLayout{}
	a.refreshProofreadingModelView()
	a.refreshProofreadingOCRMissingPageCandidates()
	return a, a.proofreading
}

func TestProofreadingOCRFollowUsesFilteredCanvasRows(t *testing.T) {
	newApp := func() *App {
		session := &documentSessionState{
			sharedDocumentSession: &sharedDocumentSession{},
			pdfView: &pdfViewState{
				info:  document.PDFInfo{Pages: 4},
				page:  0,
				cache: map[int]pdfPageResult{},
			},
		}
		session.pdfDoc = &pdfdoc.Document{}
		return &App{
			documentSessionState: session,
			proofreading: &proofreadingWorkspace{
				pageFilter: proofreadingPageFilterReviewed,
				pageOwnership: []proofreadingPageOwnership{
					{Reviewed: true}, {}, {Reviewed: true}, {},
				},
				ocrScan: newProofreadingOCRScan(),
			},
		}
	}

	t.Run("page included in active filter", func(t *testing.T) {
		a := newApp()
		a.followProofreadingOCRPage(2)
		if a.pdfView.page != 2 || a.pdfView.canvas.Position.First != 1 || len(a.pdfView.pageMap) != 2 || a.pdfView.pageMap[1] != 2 {
			t.Fatalf("OCR follow used a physical page as a filtered row: page=%d row=%d map=%v", a.pdfView.page, a.pdfView.canvas.Position.First, a.pdfView.pageMap)
		}
	})

	t.Run("page excluded from active filter", func(t *testing.T) {
		a := newApp()
		a.followProofreadingOCRPage(1)
		if a.proofreading.pageFilter != proofreadingPageFilterAll || a.pdfView.page != 1 || a.pdfView.canvas.Position.First != 1 {
			t.Fatalf("OCR follow did not reveal excluded page: filter=%v page=%d row=%d map=%v", a.proofreading.pageFilter, a.pdfView.page, a.pdfView.canvas.Position.First, a.pdfView.pageMap)
		}
	})
}

func TestProofreadingOCRCurrentPageUsesExtractedPageCount(t *testing.T) {
	for _, test := range []struct {
		extracted int
		total     int
		want      int
	}{
		{extracted: 0, total: 892, want: 0},
		{extracted: 1, total: 892, want: 0},
		{extracted: 2, total: 892, want: 1},
		{extracted: 892, total: 892, want: 891},
		{extracted: 900, total: 892, want: 891},
	} {
		if got := proofreadingOCRCurrentPage(test.extracted, test.total); got != test.want {
			t.Errorf("extracted=%d total=%d => current page %d, want %d", test.extracted, test.total, got, test.want)
		}
	}
}

func TestProofreadingOCRNewJobIsPersistedBeforePolling(t *testing.T) {
	client := &recordingProofreadingOCRClient{job: ocr.Job{ID: "job-new", State: "pending"}, pages: []ocr.Page{{Markdown: "識別結果"}}}
	persisted := false
	waitSawPersistedJob := false
	client.beforeWait = func() { waitSawPersistedJob = persisted }
	run, err := runProofreadingOCRJob(context.Background(), client, "", ocr.Source{URL: "https://example.test/book.pdf"}, func(job ocr.Job) error {
		if job.ID != "job-new" {
			t.Fatalf("wrong job passed to persistence: %#v", job)
		}
		persisted = true
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !waitSawPersistedJob || strings.Join(client.events, ",") != "submit,wait:job-new,results:job-new" {
		t.Fatalf("job must be persisted before polling; persisted-at-wait=%v events=%v", waitSawPersistedJob, client.events)
	}
	if len(run.Pages) != 1 || run.Pages[0].Markdown != "識別結果" {
		t.Fatalf("job result was not returned: %#v", run.Pages)
	}
}

func TestProofreadingOCRResumeUsesSavedJobWithoutSubmittingAgain(t *testing.T) {
	client := &recordingProofreadingOCRClient{job: ocr.Job{ID: "job-saved", State: "done"}, pages: []ocr.Page{{Markdown: "恢復結果"}}}
	run, err := runProofreadingOCRJob(context.Background(), client, "job-saved", ocr.Source{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(client.events, ","), "wait:job-saved,results:job-saved"; got != want {
		t.Fatalf("resume should poll and fetch the same job without upload: got %q want %q", got, want)
	}
	if run.CompletedJob.ID != "job-saved" || len(run.Pages) != 1 || run.Pages[0].Markdown != "恢復結果" {
		t.Fatalf("resume returned wrong result: %#v", run)
	}
}

func TestProofreadingOCRJobContinuesWhenPersistenceAcknowledgmentFails(t *testing.T) {
	client := &recordingProofreadingOCRClient{job: ocr.Job{ID: "job-unsaved", State: "pending"}, pages: []ocr.Page{}}
	run, err := runProofreadingOCRJob(context.Background(), client, "", ocr.Source{URL: "https://example.test/book.pdf"}, func(ocr.Job) error {
		return errors.New("workspace unavailable")
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.PersistenceErr == nil || run.CompletedJob.ID != "job-unsaved" || strings.Join(client.events, ",") != "submit,wait:job-unsaved,results:job-unsaved" {
		t.Fatalf("current scan should continue but report that it cannot resume: run=%#v events=%v", run, client.events)
	}
}

func TestProofreadingOCRCredentialLookupUsesSavedTokenWithoutOpeningDialog(t *testing.T) {
	root := t.TempDir()
	bPath := filepath.Join(root, "book.md")
	if err := os.WriteFile(bPath, []byte("校对稿快照"), 0o600); err != nil {
		t.Fatal(err)
	}
	object, err := loadProofreadingObject(bPath)
	if err != nil {
		t.Fatal(err)
	}
	defer object.Adapter.Close()
	store, err := workspace.Open(filepath.Join(root, "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const reference = "proofreading-ocr-test-ref"
	auth := proofreadingOCRAuthentication{Endpoint: ocr.DefaultEndpoint, CredentialRef: reference}
	encoded, err := encodeProofreadingOCRAuthentication(auth)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetPreference(proofreadingOCRAuthenticationPreferenceKey, encoded); err != nil {
		t.Fatal(err)
	}
	credentials := &fakeProofreadingOCRCredentials{values: map[string]string{reference: "synthetic-token"}}
	state := &proofreadingWorkspace{
		bDocument:          object,
		bDraftText:         "校对稿快照",
		ocrScan:            newProofreadingOCRScan(),
		ocrCredentialStore: credentials,
	}
	a := &App{
		desktopRuntime: &desktopRuntime{workspaceStore: store},
		documentSessionState: &documentSessionState{sharedDocumentSession: &sharedDocumentSession{
			liveDocumentState: liveDocumentState{pdfDoc: &pdfdoc.Document{}},
		}},
		proofreading: state,
		shutdown:     make(chan struct{}),
	}
	a.requestProofreadingOCRScan()
	if a.modal.Visible {
		t.Fatal("saved credential lookup should not open the token dialog")
	}
	select {
	case result := <-state.ocrScan.credentials:
		defer func() { result.token = "" }()
		if result.err != nil || !result.found || result.token != "synthetic-token" {
			t.Fatalf("saved OCR credential was not loaded: found=%v err=%v", result.found, result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("credential lookup did not return")
	}
	a.stopAndWaitManagedBackground()
}

func TestProofreadingOCRMatchingDraftSaveAcknowledgesSubmittedJob(t *testing.T) {
	scan := newProofreadingOCRScan()
	scan.awaitingPersistJob = "job-17"
	state := &proofreadingWorkspace{ocrScan: scan, draftSaveResults: make(chan proofreadingDraftSaveResult, 1)}
	state.draftPersistGen.Store(9)
	state.draftSaveResults <- proofreadingDraftSaveResult{generation: 9, ocrJobID: "job-17"}
	a := &App{proofreading: state}

	a.processProofreadingResults()
	select {
	case ack := <-scan.persistAcks:
		if ack.jobID != "job-17" || ack.err != nil || scan.awaitingPersistJob != "" {
			t.Fatalf("wrong job persistence acknowledgment: ack=%#v pending=%q", ack, scan.awaitingPersistJob)
		}
	default:
		t.Fatal("matching draft-save result did not release the OCR worker to poll")
	}
}

type recordingProofreadingOCRClient struct {
	events     []string
	job        ocr.Job
	pages      []ocr.Page
	beforeWait func()
}

func (c *recordingProofreadingOCRClient) Submit(context.Context, ocr.Source, ocr.Options) (ocr.Job, error) {
	c.events = append(c.events, "submit")
	return c.job, nil
}

func (c *recordingProofreadingOCRClient) Wait(_ context.Context, jobID string, progress func(ocr.Job)) (ocr.Job, error) {
	if c.beforeWait != nil {
		c.beforeWait()
	}
	c.events = append(c.events, "wait:"+jobID)
	job := c.job
	job.ID = jobID
	job.State = "done"
	if progress != nil {
		progress(job)
	}
	return job, nil
}

func (c *recordingProofreadingOCRClient) Results(_ context.Context, job ocr.Job) ([]ocr.Page, error) {
	c.events = append(c.events, "results:"+job.ID)
	return c.pages, nil
}
