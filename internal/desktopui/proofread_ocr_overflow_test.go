package desktopui

import (
	"strings"
	"testing"

	"glance/internal/application"
	"glance/internal/format/pdfdoc"
	"glance/internal/ocr"
	"glance/internal/proofreading"
)

func TestProofreadingOCRBoundaryExactPagePrefixWithoutNextOCR(t *testing.T) {
	current, err := proofreadingOCRCacheTestPage(1, "天地玄黄宇宙洪荒日月盈昃辰宿列张").Layout()
	if err != nil {
		t.Fatal(err)
	}
	tail := strings.Repeat("后页正文", 11000)
	text := "天地玄黄\n宇宙洪荒日月盈昃辰宿列张\n" + tail
	got, ok := proofreadingOCRBoundaryTailFromPages(current, ocr.Layout{}, text)
	if !ok || got.Start != 18 || got.RuneCount != 44000 {
		t.Fatalf("exact full page prefix must split before next page without its OCR: got %+v, ok=%v", got, ok)
	}
	for _, text := range []string{"天地玄黄宇宙洪荒日月盈昃辰宿列张", "天地玄黄宇宙洪荒错字盈昃辰宿列张" + tail} {
		if got, ok := proofreadingOCRBoundaryTailFromPages(current, ocr.Layout{}, text); ok {
			t.Fatalf("must not infer overflow without a complete matching page and extra text: %+v", got)
		}
	}
}

func proofreadingOCROverflowFixture(t *testing.T) (*App, *proofreadingWorkspace) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	a, s := proofreadingModelFixture(t)
	const current = "天地玄黄宇宙洪荒日月盈昃辰宿列张"
	tail := strings.Repeat("后页正文", 11000)
	m, err := proofreading.New(current+tail+"下一页原文", []proofreading.PageID{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.AssignInitial([]int{16 + 44000, 16 + 44000 + 5, 16 + 44000 + 5}); err != nil {
		t.Fatal(err)
	}
	snapshot := m.Snapshot()
	snapshot.Pages[0].Manual = true
	snapshot.Pages[1].Manual = true
	m, err = proofreading.Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	s.controller = application.NewProofreadingController(m)
	a.refreshProofreadingModelView()
	a.syncProofreadingEditors()
	pdf, err := pdfdoc.OpenSnapshot(pdfTestBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pdf.Close() })
	a.documentSessionState = &documentSessionState{sharedDocumentSession: &sharedDocumentSession{liveDocumentState: liveDocumentState{pdfDoc: pdf}}}
	a.ensurePDFView()
	s.aSourceHash = strings.Repeat("a", 64)
	step := a.proofreadingOCRStep()
	page, err := proofreadingOCRCacheTestPage(1, current).Layout()
	if err != nil {
		t.Fatal(err)
	}
	step.pages[0] = proofreadingOCRPage{layout: page}
	step.pageSourceHash[0] = s.aSourceHash
	return a, s
}

func TestProofreadingOCRCompareMovesTailPastManualBoundary(t *testing.T) {
	a, s := proofreadingOCROverflowFixture(t)
	const current = "天地玄黄宇宙洪荒日月盈昃辰宿列张"
	tail := strings.Repeat("后页正文", 11000)
	m, step := s.controller.Model, s.ocrStep
	a.compareProofreadingOCRStep()
	if got := m.Text(1); got != current {
		t.Fatalf("current page kept overflow: %d runes", len([]rune(got)))
	}
	if got := m.Text(2); got != tail+"下一页原文" {
		t.Fatalf("next page did not receive ordered tail: %d runes", len([]rune(got)))
	}
	if step.phase == proofreadingOCRStepCheckingBoundary {
		t.Fatal("exactly matched page must not wait for next-page OCR after splitting")
	}
	if !m.Undo() {
		t.Fatal("undo unavailable")
	}
	if m.Text(1) != current+tail || m.Text(2) != "下一页原文" {
		t.Fatal("boundary transfer must remain undoable")
	}
}

func TestProofreadingOCRBoundaryKeepsProtectedPages(t *testing.T) {
	for _, kind := range []string{"current_locked", "next_locked", "current_reviewed", "next_reviewed", "next_typed", "current_typed_tail"} {
		t.Run(kind, func(t *testing.T) {
			a, s := proofreadingOCROverflowFixture(t)
			m := s.controller.Model
			var err error
			switch kind {
			case "current_locked":
				err = m.Lock(1, true)
			case "next_locked":
				err = m.Lock(2, true)
			case "current_reviewed":
				err = m.Review(1, true)
			case "next_reviewed":
				err = m.Review(2, true)
			case "next_typed":
				err = m.ReplacePage(2, "用户编辑下一页")
			case "current_typed_tail":
				err = m.ReplacePage(1, m.Text(1)+"用户编辑")
			}
			if err != nil {
				t.Fatal(err)
			}
			a.refreshProofreadingModelView()
			before, next := m.Text(1), m.Text(2)
			a.compareProofreadingOCRStep()
			if m.Text(1) != before || m.Text(2) != next {
				t.Fatal("comparison moved protected content")
			}
		})
	}
}

func TestProofreadingOCRCompareRetriesIncompleteCache(t *testing.T) {
	a, s := proofreadingOCROverflowFixture(t)
	step, page := s.ocrStep, s.xPages[0]
	hash := proofreadingOCRDraftSnapshotHash(page.Content)
	step.boundarySkip = &proofreadingOCRBoundarySkip{page: 0, sourceHash: s.aSourceHash, draftHash: hash}
	incomplete := proofreadingOCRStepComparison{APage: 0, BPage: 0, SourceHash: s.aSourceHash, DraftHash: hash, BText: page.Content, BStart: page.ContentStart, BEnd: page.ContentEnd, HasEvidence: true, Complete: false}
	if err := saveProofreadingOCRComparisonCache(incomplete); err != nil {
		t.Fatal(err)
	}
	a.compareProofreadingOCRStep()
	// This fixture disables background launches. A fresh comparison attempt must
	// report that failure, rather than presenting the cached zero-character result.
	if step.phase != proofreadingOCRStepFailed || step.err == "" {
		t.Fatalf("incomplete cache prevented retry: phase=%v err=%q", step.phase, step.err)
	}
}

func TestProofreadingOCRCompareRefreshesOldCharacterGeometry(t *testing.T) {
	a, s := proofreadingOCROverflowFixture(t)
	step, page := s.ocrStep, s.xPages[0]
	old := step.pages[0]
	old.layout.CharacterVersion = 1
	step.pages[0] = old
	hash := proofreadingOCRDraftSnapshotHash(page.Content)
	step.boundarySkip = &proofreadingOCRBoundarySkip{page: 0, sourceHash: s.aSourceHash, draftHash: hash}
	cached := proofreadingOCRStepComparison{APage: 0, BPage: 0, SourceHash: s.aSourceHash, DraftHash: hash, BText: page.Content, BStart: page.ContentStart, BEnd: page.ContentEnd, HasEvidence: true, Complete: true}
	if err := saveProofreadingOCRComparisonCache(cached); err != nil {
		t.Fatal(err)
	}
	a.compareProofreadingOCRStep()
	// The fixture blocks workers. Reaching the launch failure proves the old
	// geometry goes through localization again instead of displaying its cache.
	if step.phase != proofreadingOCRStepFailed {
		t.Fatalf("completed comparison reused obsolete character geometry: phase=%v", step.phase)
	}
}
