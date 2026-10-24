package desktopui

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/document"
	"glance/internal/format/pdfdoc"
	"glance/internal/i18n"
	"glance/internal/ocr"
	"glance/internal/textalign"
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
