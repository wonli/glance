package desktopui

import (
	"encoding/json"
	"errors"
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/ocr"
	"glance/internal/proofreading"
)

func proofreadingBatchFixture(t *testing.T) (*App, *proofreadingOCRStepState) {
	t.Helper()
	a, s := proofreadingModelFixture(t)
	s.aSourceHash = "batch-source"
	a.pdfView = &pdfViewState{info: document.PDFInfo{Pages: 3}}
	step := a.proofreadingOCRStep()
	step.batch = proofreadingOCRBatch{checkpoint: proofreadingOCRBatchCheckpoint{SourceHash: s.aSourceHash, Total: 3}, open: true, running: true, started: true}
	step.page, step.bPage = 0, 0
	step.phase = proofreadingOCRStepCompared
	p := s.xPages[0]
	step.comparisons[proofreadingOCRStepPair{}] = proofreadingOCRStepComparison{SourceHash: s.aSourceHash, BText: p.Content, BStart: p.ContentStart, BEnd: p.ContentEnd, Complete: true, HasEvidence: true, AOnly: 1}
	return a, step
}

func TestProofreadingOCRBatchRunsThreeCachedPagesThroughSinglePagePipeline(t *testing.T) {
	a, s := proofreadingOCROverflowFixture(t)
	m, err := proofreading.New("abcdefghijkl", []proofreading.PageID{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.AssignInitial([]int{4, 8, 12}); err != nil {
		t.Fatal(err)
	}
	s.controller = application.NewProofreadingController(m)
	a.refreshProofreadingModelView()
	a.syncProofreadingEditors()
	a.pdfView.info.Pages = 3
	step := a.proofreadingOCRStep()
	step.sourceHash = s.aSourceHash
	for page, p := range s.xPages {
		layout, err := proofreadingOCRCacheTestPage(page+1, p.Content).Layout()
		if err != nil {
			t.Fatal(err)
		}
		layout.CharacterVersion = ocr.CharacterGeometryVersion
		step.pages[page] = proofreadingOCRPage{layout: layout}
		step.pageSourceHash[page] = s.aSourceHash
		c := proofreadingOCRStepComparison{APage: page, BPage: page, SourceHash: s.aSourceHash, DraftHash: proofreadingOCRDraftSnapshotHash(p.Content), BText: p.Content, BStart: p.ContentStart, BEnd: p.ContentEnd, Complete: true, HasEvidence: true, Matches: 4}
		for i, ch := range p.Content {
			c.Frames = append(c.Frames, proofreadingOCRDiffFrame{Kind: proofreadingOCRDiffEqual, AChar: ch, BChar: ch, BPosition: p.ContentStart + i})
		}
		if err := saveProofreadingOCRComparisonCache(c); err != nil {
			t.Fatal(err)
		}
	}
	revision := m.Revision()
	a.requestProofreadingOCRBatch()
	now := time.Now()
	for frame := 0; frame < 30 && !step.batch.checkpoint.Finished; frame++ {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Now: now.Add(time.Duration(frame) * time.Second)}
		a.processProofreadingOCRStep(gtx)
		a.processProofreadingOCRBatch(gtx)
	}
	if !step.batch.checkpoint.Finished || step.batch.checkpoint.Completed != 3 || len(step.comparisons) != 3 || a.pdfView.page != 2 {
		t.Fatalf("whole-book pipeline did not finish: %+v phase=%v err=%s", step.batch, step.phase, step.err)
	}
	if m.Revision() != revision {
		t.Fatal("cached comparison changed manuscript or confirmation state")
	}
}

func tickProofreadingBatch(a *App) {
	var ops op.Ops
	a.processProofreadingOCRBatch(layout.Context{Ops: &ops, Now: time.Now()})
}

func TestProofreadingOCRBatchAdvancesWithDifferencesWithoutReviewing(t *testing.T) {
	a, step := proofreadingBatchFixture(t)
	revision := a.proofreading.controller.Model.Revision()
	tickProofreadingBatch(a)
	b := &step.batch
	if b.checkpoint.Page != 1 || b.checkpoint.Completed != 1 || len(b.checkpoint.DifferencePages) != 1 || b.started || !b.running {
		t.Fatalf("ordinary difference stopped or duplicated processing: %+v", b)
	}
	if a.proofreading.controller.Model.Revision() != revision {
		t.Fatal("scheduler changed document or marked a page reviewed")
	}
}

func TestProofreadingOCRBatchPauseFreezesLateResultsAndPlayback(t *testing.T) {
	a, step := proofreadingBatchFixture(t)
	step.diffRunning = true
	step.diffCursor = 1
	a.pauseProofreadingOCRBatch()
	step.results <- proofreadingOCRStepResult{generation: step.generation, sourceHash: a.proofreading.aSourceHash, err: errors.New("late result")}
	var ops op.Ops
	a.processProofreadingOCRStep(layout.Context{Ops: &ops, Now: time.Now()})
	tickProofreadingBatch(a)
	if len(step.results) != 1 || step.diffCursor != 1 || !step.diffPaused || step.batch.checkpoint.Completed != 0 || !step.batch.open {
		t.Fatal("paused task consumed a result, advanced playback, or lost its shield")
	}
	a.endProofreadingOCRBatch()
	tickProofreadingBatch(a)
	if step.batch.running || step.batch.open || step.batch.checkpoint.Page != 0 || step.diffRunning {
		t.Fatal("end did not preserve a stopped checkpoint")
	}
}

func TestProofreadingOCRBatchStopsOnIncompleteOrFailedComparison(t *testing.T) {
	for _, failed := range []bool{false, true} {
		a, step := proofreadingBatchFixture(t)
		if failed {
			step.phase, step.err = proofreadingOCRStepFailed, "synthetic recognition error"
		} else {
			c := step.comparisons[proofreadingOCRStepPair{}]
			c.Complete = false
			step.comparisons[proofreadingOCRStepPair{}] = c
		}
		tickProofreadingBatch(a)
		if failed {
			if !step.batch.running || step.batch.retryAt.IsZero() || step.batch.checkpoint.Page != 0 {
				t.Fatal("failure did not schedule a retry")
			}
		} else {
			if step.batch.running || step.batch.err == "" || step.batch.checkpoint.Page != 0 {
				t.Fatal("incomplete page did not stop the batch")
			}
		}
	}
}

func TestProofreadingOCRBatchCompletesOnlyAfterLastPlayback(t *testing.T) {
	a, step := proofreadingBatchFixture(t)
	step.batch.checkpoint.Total = 1
	a.pdfView.info.Pages = 1
	step.diffRunning = true
	tickProofreadingBatch(a)
	if step.batch.checkpoint.Finished {
		t.Fatal("finished before playback")
	}
	step.diffRunning = false
	tickProofreadingBatch(a)
	if !step.batch.checkpoint.Finished || step.batch.running || step.batch.checkpoint.Completed != 1 || !step.batch.open {
		t.Fatal("last page did not produce a completion summary")
	}
}

func TestProofreadingOCRBatchCheckpointRecoveryIsPausedAndSourceBound(t *testing.T) {
	a, step := proofreadingBatchFixture(t)
	step.batch.checkpoint.Page = 1
	step.batch.checkpoint.Completed = 1
	data, err := json.Marshal(proofreadingProjectPayload{OCRBatch: step.batch.snapshot()})
	if err != nil {
		t.Fatal(err)
	}
	var payload proofreadingProjectPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	var restored proofreadingOCRBatch
	restored.restore(payload.OCRBatch, a.proofreading.aSourceHash)
	if restored.running || restored.open || restored.started || restored.checkpoint.Page != 1 {
		t.Fatal("recovery lost checkpoint or automatically ran", restored)
	}
	restored.restore(payload.OCRBatch, "different-source")
	if restored.checkpoint.SourceHash != "" {
		t.Fatal("accepted another PDF's checkpoint")
	}
}

func TestProofreadingOCRBatchResumeAfterEditRejectsLateComparison(t *testing.T) {
	a, step := proofreadingBatchFixture(t)
	a.pauseProofreadingOCRBatch()
	generation := step.generation
	if err := a.proofreading.controller.Model.ReplacePage(1, "modified"); err != nil {
		t.Fatal(err)
	}
	a.resumeProofreadingOCRBatch()
	if step.batch.started || step.batch.suspended || !step.batch.running || step.generation == generation {
		t.Fatal("resume reused stale work after editing")
	}
	step.results <- proofreadingOCRStepResult{generation: generation, sourceHash: a.proofreading.aSourceHash, err: errors.New("obsolete")}
	var ops op.Ops
	a.processProofreadingOCRStep(layout.Context{Ops: &ops, Now: time.Now()})
	if step.err != "" {
		t.Fatal("late result overwrote resumed state")
	}
}

func TestProofreadingOCRBatchUsesExistingSinglePageBoundaryFlow(t *testing.T) {
	a, s := proofreadingOCROverflowFixture(t)
	step := a.proofreadingOCRStep()
	step.batch = proofreadingOCRBatch{checkpoint: proofreadingOCRBatchCheckpoint{SourceHash: s.aSourceHash, Total: a.pdfView.info.Pages}, open: true, running: true}
	tickProofreadingBatch(a)
	if got := s.controller.Model.Text(1); got != "天地玄黄宇宙洪荒日月盈昃辰宿列张" {
		t.Fatal("batch did not invoke existing boundary adjustment")
	}
}

func TestProofreadingOCRBoundaryInvalidationRetainsEarlierPageEvidence(t *testing.T) {
	a, step := proofreadingBatchFixture(t)
	step.comparisons[proofreadingOCRStepPair{APage: 1, BPage: 1}] = proofreadingOCRStepComparison{APage: 1, BPage: 1}
	a.invalidateProofreadingOCRStepComparisons(1)
	if _, ok := step.comparisons[proofreadingOCRStepPair{}]; !ok {
		t.Fatal("later boundary change discarded earlier differences")
	}
	if _, ok := step.comparisons[proofreadingOCRStepPair{APage: 1, BPage: 1}]; ok {
		t.Fatal("changed page kept stale comparison")
	}
}

func TestProofreadingOCRBatchRetriesAdjacentPageRecognitionFailure(t *testing.T) {
	a, s := proofreadingOCROverflowFixture(t)
	step := a.proofreadingOCRStep()
	step.sourceHash = s.aSourceHash
	step.batch = proofreadingOCRBatch{checkpoint: proofreadingOCRBatchCheckpoint{SourceHash: s.aSourceHash, Total: a.pdfView.info.Pages}, open: true, running: true, started: true}
	step.phase = proofreadingOCRStepCheckingBoundary
	step.overflowRequest = &proofreadingOCROverflowRequest{preComparison: true, mainGeneration: step.generation, overflowGeneration: step.overflowGeneration, sourceHash: s.aSourceHash, modelRevision: s.controller.Model.Revision(), currentText: s.xPages[0].Content, nextText: s.xPages[1].Content, comparison: proofreadingOCRStepComparison{APage: 0, BPage: 0}}
	step.results <- proofreadingOCRStepResult{generation: step.generation, overflowGeneration: step.overflowGeneration, sourceHash: s.aSourceHash, overflowPrefetch: true, err: errors.New("adjacent OCR failed")}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now()}
	a.processProofreadingOCRStep(gtx)
	a.processProofreadingOCRBatch(gtx)
	if !step.batch.running || step.batch.retryAt.IsZero() || step.batch.checkpoint.Completed != 0 || step.boundarySkip != nil {
		t.Fatalf("adjacent OCR failure was skipped: batch=%+v step=%v/%s", step.batch, step.phase, step.err)
	}
}

func TestProofreadingOCRBatchExhaustedRetriesAndTimeoutStopBatch(t *testing.T) {
	a, step := proofreadingBatchFixture(t)
	for attempt := 0; attempt < 3; attempt++ {
		step.phase, step.err = proofreadingOCRStepFailed, "temporary failure"
		step.batch.started = true
		step.batch.retryAt = time.Time{}
		tickProofreadingBatch(a)
	}
	if step.batch.running || step.batch.err == "" || step.batch.checkpoint.Page != 0 {
		t.Fatal("exhausted retries did not stop the batch")
	}
	a, step = proofreadingBatchFixture(t)
	step.batch.started = true
	step.phase = proofreadingOCRStepRunning
	step.batch.attemptDeadline = time.Now().Add(-time.Second)
	tickProofreadingBatch(a)
	if step.batch.retryAt.IsZero() || step.active() {
		t.Fatal("unresponsive page had no timeout/retry")
	}
}

func TestProofreadingOCRBatchStartFromCurrentPage(t *testing.T) {
	a, step := proofreadingBatchFixture(t)
	// Batch had stopped at page 0 with an error.
	step.batch.running = false
	step.batch.err = "some error on page 0"
	step.batch.checkpoint.Page = 0
	step.batch.checkpoint.Completed = 0

	// User navigated to page 2.
	a.pdfView.page = 2

	// User clicks start from current page.
	a.startProofreadingOCRBatchFromCurrentPage()

	b := &step.batch
	if !b.running || !b.open || b.started || b.err != "" {
		t.Fatalf("unexpected batch state after startCurrent: running=%v open=%v started=%v err=%q", b.running, b.open, b.started, b.err)
	}
	if b.checkpoint.Start != 2 || b.checkpoint.Page != 2 || b.checkpoint.Completed != 0 || b.checkpoint.Total != 3 {
		t.Fatalf("checkpoint not reset to current page: %+v", b.checkpoint)
	}
	if a.pdfView.page != 2 {
		t.Fatalf("pdfView page = %d, want 2", a.pdfView.page)
	}

	// Verify dialog button layout includes startCurrent
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Constraints: layout.Exact(image.Pt(900, 650))}
	dims := a.layoutProofreadingOCRBatchDialog(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("batch dialog did not layout: %v", dims)
	}
}
