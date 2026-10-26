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

func TestProofreadingOCRBoundaryToleratesNextPageOCRTypo(t *testing.T) {
	currentText := "本页开头有足够连续文字用于确认页面来源\n中间有一处原文错字\n本页结尾也有足够连续文字确认分页边界。\n"
	nextText := "下一页开头有错字但后续正文仍然一致，较长连续上下文可以证明这里确实是下一页的起点，不能因为一个字而把几万字全部留在前页。"
	current, err := proofreadingOCRCacheTestPage(1, strings.Replace(currentText, "原文错字", "原文误字", 1)).Layout()
	if err != nil {
		t.Fatal(err)
	}
	next, err := proofreadingOCRCacheTestPage(2, strings.Replace(nextText, "有错字", "有误字", 1)).Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := currentText + nextText + strings.Repeat("后续各页正文", 8000)
	tail, ok := proofreadingOCRBoundaryTailFromPages(current, next, text)
	if !ok || tail.Start != len([]rune(currentText)) || tail.RuneCount != len([]rune(text))-tail.Start {
		t.Fatalf("next-page OCR typo blocked bounded page split: %+v ok=%v", tail, ok)
	}
	// Insufficient, unrelated, or ambiguous next-page evidence must not split.
	for _, value := range []string{"下一页开头有误字", "完全无关的识别结果与后续文字之间没有可靠的相似上下文，不能仅凭本页结尾匹配就猜测整个稿件的分页位置以及段落归属。"} {
		bad, err := proofreadingOCRCacheTestPage(2, value).Layout()
		if err != nil {
			t.Fatal(err)
		}
		if tail, ok := proofreadingOCRBoundaryTailFromPages(current, bad, text); ok {
			t.Fatalf("weak evidence inferred boundary: %+v", tail)
		}
	}
}

func TestProofreadingOCRBoundaryAlignsWholePageDespiteErrorsAtBothEnds(t *testing.T) {
	currentText := "本页起始的诗句和说明文字足够完整，正文中间还有许多不同的字词用于可靠定位，而不是要求某八个字完全相同，最后一行也需要允许识别差错。\n"
	nextText := "下一页起始的正文同样可能出现识别差错，但整段上下文仍能支持正确定位。这些文字用于共同约束分页位置，保留真实原文并完整交给后续页面。"
	for _, kind := range []string{"head", "tail", "both", "insert", "delete"} {
		t.Run(kind, func(t *testing.T) {
			r := []rune(strings.TrimSpace(currentText))
			switch kind {
			case "head":
				r[0] = '误'
			case "tail":
				r[len(r)-2] = '误'
			case "both":
				r[0] = '误'
				r[len(r)-2] = '误'
			case "insert":
				r = append([]rune("多"), r...)
			case "delete":
				r = r[1:]
			}
			current, err := proofreadingOCRCacheTestPage(1, string(r)).Layout()
			if err != nil {
				t.Fatal(err)
			}
			next, err := proofreadingOCRCacheTestPage(2, "误"+string([]rune(nextText)[1:])).Layout()
			if err != nil {
				t.Fatal(err)
			}
			tail, ok := proofreadingOCRBoundaryTailFromPages(current, next, currentText+nextText+strings.Repeat("后续原文", 12000))
			if !ok || tail.Start != len([]rune(currentText)) {
				t.Fatalf("%s: boundary %+v ok=%v", kind, tail, ok)
			}
		})
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

func TestProofreadingOCRBoundaryReplacesPrematureOCRPlaceholder(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	const current = "天地玄黄宇宙洪荒日月盈昃辰宿列张"
	const nextOCRText = "下一页正文内容，包含了关于某位历史人物的评述"
	tail := nextOCRText + strings.Repeat("后续大段文字", 100)

	m, err := proofreading.New(current+tail, []proofreading.PageID{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.AssignInitial([]int{len([]rune(current + tail)), len([]rune(current + tail)), len([]rune(current + tail))}); err != nil {
		t.Fatal(err)
	}
	s.controller = application.NewProofreadingController(m)

	// Prematurely fill page 2 with OCR text (as done by preview or previous fill)
	if err := m.ReplacePage(2, nextOCRText); err != nil {
		t.Fatal(err)
	}
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
	curLayout, err := proofreadingOCRCacheTestPage(1, current).Layout()
	if err != nil {
		t.Fatal(err)
	}
	nextLayout, err := proofreadingOCRCacheTestPage(2, nextOCRText).Layout()
	if err != nil {
		t.Fatal(err)
	}
	step.pages[0] = proofreadingOCRPage{layout: curLayout}
	step.pages[1] = proofreadingOCRPage{layout: nextLayout}
	step.pageSourceHash[0] = s.aSourceHash
	step.pageSourceHash[1] = s.aSourceHash

	a.compareProofreadingOCRStep()

	// Page 1 should be shrunk to current, and Page 2 should have received the true manuscript tail
	if got := m.Text(1); got != current {
		t.Fatalf("page 1 was not shrunk: got %d runes, want %d", len([]rune(got)), len([]rune(current)))
	}
	if got := m.Text(2); got != tail {
		t.Fatalf("page 2 did not receive manuscript tail: got %d runes, want %d", len([]rune(got)), len([]rune(tail)))
	}
}
