package desktopui

import (
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"glance/internal/i18n"
)

const proofreadingOCRBatchAttemptTimeout = 3 * time.Minute

// This is a scheduler only. All recognition, boundary changes, comparisons,
// playback and cached evidence are produced by compareProofreadingOCRStep.
type proofreadingOCRBatchCheckpoint struct {
	SourceHash      string `json:"source_hash"`
	Start           int    `json:"start"`
	Page            int    `json:"next_page"`
	Total           int    `json:"total"`
	Completed       int    `json:"completed"`
	DifferencePages []int  `json:"difference_pages,omitempty"`
	Finished        bool   `json:"finished,omitempty"`
}

type proofreadingOCRBatch struct {
	checkpoint                                             proofreadingOCRBatchCheckpoint
	open, running, started                                 bool
	suspended                                              bool
	pausedRevision                                         uint64
	err                                                    string
	retries                                                int
	retryAt, attemptDeadline                               time.Time
	entry, pause, resume, inspect, end, close, differences, startCurrent widget.Clickable
	scrim, card                                            widget.Clickable
	focusTag                                               struct{}
}

func (b *proofreadingOCRBatch) snapshot() *proofreadingOCRBatchCheckpoint {
	if b.checkpoint.SourceHash == "" {
		return nil
	}
	copy := b.checkpoint
	copy.DifferencePages = append([]int(nil), copy.DifferencePages...)
	return &copy
}

func (b *proofreadingOCRBatch) restore(c *proofreadingOCRBatchCheckpoint, source string) {
	*b = proofreadingOCRBatch{}
	if c == nil || c.SourceHash != source || c.Total < 1 || c.Start < 0 || c.Start >= c.Total || c.Page < c.Start || c.Page > c.Total || c.Completed != c.Page-c.Start || c.Finished != (c.Page == c.Total) {
		return
	}
	for _, page := range c.DifferencePages {
		if page < c.Start || page >= c.Page {
			return
		}
	}
	b.checkpoint = *c
	b.checkpoint.DifferencePages = append([]int(nil), c.DifferencePages...)
}

func (a *App) requestProofreadingOCRBatch() {
	if a.proofreading == nil || a.pdfDoc == nil {
		return
	}
	s, step, view := a.proofreading, a.proofreadingOCRStep(), a.ensurePDFView()
	b := &step.batch
	if b.checkpoint.SourceHash == "" || b.checkpoint.SourceHash != s.aSourceHash || b.checkpoint.Total != view.info.Pages || b.checkpoint.Finished {
		step.cancelTask()
		step.autoCompareAfterOCR = false
		step.diffRunning, step.diffPaused = false, false
		if s.ocrScan != nil && s.ocrScan.active() {
			s.ocrScan.cancelTask()
		}
		page := max(0, min(view.info.Pages-1, view.page))
		b.checkpoint = proofreadingOCRBatchCheckpoint{SourceHash: s.aSourceHash, Start: page, Page: page, Total: view.info.Pages}
		b.running, b.started, b.err = true, false, ""
		b.retries = 0
		b.retryAt, b.attemptDeadline = time.Time{}, time.Time{}
	}
	step.inspector.open = false
	b.open = true
	a.queueProofreadingDraftPersistence()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) startProofreadingOCRBatchFromCurrentPage() {
	if a == nil || a.proofreading == nil || a.pdfView == nil {
		return
	}
	s, step, view := a.proofreading, a.proofreadingOCRStep(), a.ensurePDFView()
	if view == nil || view.info.Pages <= 0 {
		return
	}
	b := &step.batch
	step.cancelTask()
	step.autoCompareAfterOCR = false
	step.diffRunning, step.diffPaused = false, false
	if s.ocrScan != nil && s.ocrScan.active() {
		s.ocrScan.cancelTask()
	}
	page := max(0, min(view.info.Pages-1, view.page))
	s.pageFilter = proofreadingPageFilterAll
	a.setProofreadingCurrentPage(page)
	b.checkpoint = proofreadingOCRBatchCheckpoint{
		SourceHash: s.aSourceHash,
		Start:      page,
		Page:       page,
		Total:      view.info.Pages,
	}
	b.running, b.open, b.started, b.suspended, b.err = true, true, false, false, ""
	b.retries = 0
	b.retryAt, b.attemptDeadline = time.Time{}, time.Time{}
	step.inspector.open = false
	a.queueProofreadingDraftPersistence()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) pauseProofreadingOCRBatch() {
	step := a.proofreadingOCRStep()
	if step == nil {
		return
	}
	b := &step.batch
	b.running = false
	b.suspended = b.started
	if a.proofreading.controller != nil {
		b.pausedRevision = a.proofreading.controller.Model.Revision()
	}
	if step.diffRunning {
		step.diffPaused = true
	}
	a.queueProofreadingDraftPersistence()
}

func (a *App) resumeProofreadingOCRBatch() {
	step := a.proofreadingOCRStep()
	if step == nil || a.pdfView == nil {
		return
	}
	s, b := a.proofreading, &step.batch
	if b.checkpoint.SourceHash != s.aSourceHash || b.checkpoint.Total != a.pdfView.info.Pages || b.checkpoint.Finished {
		return
	}
	s.pageFilter = proofreadingPageFilterAll
	a.setProofreadingCurrentPage(b.checkpoint.Page)
	if b.err != "" || s.controller == nil || b.pausedRevision != s.controller.Model.Revision() {
		step.cancelTask()
		step.autoCompareAfterOCR = false
		step.diffRunning, step.diffPaused = false, false
		b.started = false
	}
	b.running, b.open, b.err, b.suspended = true, true, "", false
	b.retries, b.retryAt = 0, time.Time{}
	b.attemptDeadline = time.Now().Add(proofreadingOCRBatchAttemptTimeout)
	if step.diffPaused {
		step.diffPaused = false
		step.diffNextAt = time.Now().Add(proofreadingOCRDiffInitialDelay)
	}
	step.inspector.open = false
	a.queueProofreadingDraftPersistence()
}

func (a *App) endProofreadingOCRBatch() {
	step := a.proofreadingOCRStep()
	if step == nil {
		return
	}
	a.pauseProofreadingOCRBatch()
	step.cancelTask()
	step.autoCompareAfterOCR = false
	step.diffRunning, step.diffPaused = false, false
	step.batch.open, step.batch.started = false, false
	step.batch.suspended = false
}

func (a *App) processProofreadingOCRBatch(gtx layout.Context) {
	step := a.proofreadingOCRStep()
	if step == nil || !step.batch.running || a.modal.Visible {
		return
	}
	s, b := a.proofreading, &step.batch
	fail := func(message string) {
		b.err = message
		b.started = false
		b.retries = 0
		b.retryAt = time.Time{}
		a.pauseProofreadingOCRBatch()
		gtx.Execute(op.InvalidateCmd{})
	}
	if a.pdfView == nil || b.checkpoint.SourceHash != s.aSourceHash || b.checkpoint.Total != a.pdfView.info.Pages {
		fail(a.textID("proofread.ai.batch.source_changed"))
		return
	}
	if !b.retryAt.IsZero() {
		if gtx.Now.Before(b.retryAt) {
			gtx.Execute(op.InvalidateCmd{At: b.retryAt})
			return
		}
		b.retryAt = time.Time{}
	}
	if !b.started {
		s.pageFilter = proofreadingPageFilterAll
		a.setProofreadingCurrentPage(b.checkpoint.Page)
		b.started = true
		b.err = ""
		b.attemptDeadline = gtx.Now.Add(proofreadingOCRBatchAttemptTimeout)
		gtx.Execute(op.InvalidateCmd{At: b.attemptDeadline})
		// One entry point for manual single-page and automatic whole-book work.
		a.compareProofreadingOCRStep()
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	if step.active() && !b.attemptDeadline.IsZero() && !gtx.Now.Before(b.attemptDeadline) {
		step.cancelTask()
		step.phase, step.err = proofreadingOCRStepFailed, a.textID("proofread.ai.batch.timeout")
	}
	if step.phase == proofreadingOCRStepFailed {
		if b.retries < 2 {
			b.retries++
			b.started = false
			b.retryAt = gtx.Now.Add(time.Duration(b.retries) * time.Second)
			b.err = a.textArgs("proofread.ai.batch.retrying", i18n.Args{"Page": proofreadOCRStepPageLabel(a, b.checkpoint.Page), "Attempt": b.retries, "Error": step.err})
			gtx.Execute(op.InvalidateCmd{At: b.retryAt})
			return
		}
		fail(step.err)
		return
	}
	if step.phase == proofreadingOCRStepTokenRequired {
		fail(a.textID("proofread.ai.token.required"))
		return
	}
	if step.active() || step.diffRunning || step.diffPaused {
		return
	}
	pair := proofreadingOCRStepPair{APage: b.checkpoint.Page, BPage: b.checkpoint.Page}
	c, ok := step.comparisons[pair]
	if step.phase != proofreadingOCRStepCompared || !ok || !proofreadingOCRStepComparisonValid(s, c) || !c.Complete || !c.HasEvidence {
		fail(a.textID("proofread.ai.batch.incomplete"))
		return
	}
	if c.AOnly > 0 || c.BOnly > 0 {
		b.checkpoint.DifferencePages = append(b.checkpoint.DifferencePages, b.checkpoint.Page)
	}
	b.advancePage(a)
	gtx.Execute(op.InvalidateCmd{})
}

func (b *proofreadingOCRBatch) advancePage(a *App) {
	b.checkpoint.Completed++
	b.checkpoint.Page++
	b.started = false
	b.retries = 0
	b.retryAt, b.attemptDeadline = time.Time{}, time.Time{}
	b.err = ""
	if b.checkpoint.Page >= b.checkpoint.Total {
		b.checkpoint.Finished, b.running = true, false
	}
	a.queueProofreadingDraftPersistence()
	if a.window != nil {
		a.window.Invalidate()
	}
}
