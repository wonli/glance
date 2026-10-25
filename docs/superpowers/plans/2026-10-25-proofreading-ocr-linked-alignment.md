# Proofreading OCR Linked Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show real OCR-to-B alignment, first/last located anchors, page-level matching progress, and a non-destructive animated linked comparison in the proofreading UI.

**Architecture:** Extend `internal/textalign` with truthful per-page candidate-search progress and globally supported path choices. In `internal/desktopui`, map the resulting B rune ranges into X page editor windows, show source/result highlights, and temporarily decouple X scrolling while a selected PDF-to-B link is inspected. The existing A/X synchronized behavior resumes when the link is dismissed.

**Tech Stack:** Go, Gio, `internal/textalign`, proofreading workspace model, embedded `zh-CN`/`en-US` catalogs.

**Spec:** [2026-10-25-proofreading-ocr-linked-alignment-design.md](../specs/2026-10-25-proofreading-ocr-linked-alignment-design.md)

## Global Constraints

- A/X normally share page scrolling; linked comparison temporarily holds A on its source page and moves X to the B range, then restores normal synchronization when dismissed.
- Mapping and highlighting use Unicode rune ranges over the current B working draft; never treat rune offsets as UTF-8 byte offsets.
- OCR, candidates, progress, and highlighting are read-only; do not change B text, character ownership, confirmed-page state, or page boundaries.
- A `point` or `range` boundary is evidence for review, not an automatic page split. `unresolved` must not receive a guessed boundary.
- Only candidate-search page progress is animated before global path resolution; do not show a provisional candidate as a final match.
- Reject stale link state when A/B snapshots, OCR generation, or X page ownership changes.
- Reuse the completed OCR page cache added in commit `fed4c5e (OCR缓存)`. It is keyed by PDF content hash, model, options, and page count; it stores OCR page/layout data only. A cache hit skips credentials and cloud OCR, then recomputes alignment against the current B draft. Cache misses/invalid entries fall back to the existing resumable remote job flow. Do not describe the cache as a per-page/partial OCR checkpoint: local cache is reusable only after a complete book result is written.
- Cached and freshly recognized OCR pages must enter the same alignment helper and publish the same real candidate-search progress. B draft changes invalidate alignment/link state but should continue to reuse matching OCR cache data.
- Implement and validate in `/Users/mima1234/project/glance-go`; do not create, move, or switch Git worktrees.
- Keep parsing, matching, and persistence out of Gio layout/frame code; use synchronized progress state and invalidate the window from the established worker path.
- Keep tests and verification in the authorized source directory and reuse existing toolchain/cache; do not create a separate cache or leave verification artifacts.

## Review Focus

- Repetitive or low-coverage OCR with competing B spans: only globally supported alternatives can be cycled, and ambiguous status remains visible. Test in Task 1 and Task 4.
- Han characters, emoji/non-BMP runes, combining marks, and a range crossing X pages: each highlight must land on the intended characters. Test in Task 2 and Task 3.
- Empty X pages and a B range beginning/ending exactly on an X page edge: omit empty intersections and retain half-open boundaries. Test in Task 2.
- B edits, A/B replacement, OCR cancellation, or page-ownership reflow while linked: clear the old highlight and restore safe scroll state. Test in Task 4.
- An inactive proofreading tab or hidden OCR page while animation is active: stop frame invalidations and never animate a stale page. Test in Task 5.
- A complete cache hit, a changed B draft, an invalid/incomplete cache, and a cache write error: preserve the existing cache contract and keep alignment progress/results consistent. Verify in Tasks 1 and 5.

---

### Task 1: Unify Cached/Fresh Alignment Progress and Expose Supported Alternatives

**Files:**
- Modify: `internal/textalign/model.go`
- Modify: `internal/textalign/path.go`
- Modify: `internal/desktopui/proofread_ocr.go`
- Create or extend: `internal/desktopui/proofread_ocr_cache_test.go`
- Create: `internal/textalign/progress_test.go`
- Create: `internal/textalign/path_choices_test.go`

**Interfaces:**
- Produces `textalign.AlignmentProgress{Phase string, Page int, CompletedPages int, TotalPages int}` and optional `Options.OnProgress func(AlignmentProgress)`.
- Defines `AlignmentPhaseCandidates = "candidates"` and `AlignmentPhasePath = "path"` for the two progress phases.
- Candidate events use `Phase=AlignmentPhaseCandidates`, `Page=page.Number`; the before-search event carries the prior `CompletedPages`, and the after-search event carries `CompletedPages+1`. The path event uses `Page=0`, `CompletedPages=TotalPages`.
- Produces `Result.PathChoices []Candidate`, containing only candidates that the completed global path solver marks as supported alternatives for that page; it is not the full local `Candidates` list.
- `PathChoices` puts the selected global-path candidate first, followed by distinct globally supported alternatives in deterministic score/range order. A page without a selected path may expose supported alternatives but remains ambiguous.
- Extends `alignProofreadingOCRPages` to accept/publish alignment progress. Both the cache-hit branch and newly recognized cloud-result branch call this one helper; it recomputes alignment for the current B draft and reports identical progress semantics regardless of OCR page origin.
- A complete cache hit remains a local path: do not look up the service token, submit/poll a cloud job, or rewrite the OCR cache. Cache misses, invalid/incomplete entries, and cache read errors continue through existing authentication/job-resume behavior. Cache-save errors must not suppress the recognized pages or their alignment.

- [x] **Step 1: Write failing alignment progress tests and lock the cache contract**

Create `TestAlignReportsCandidateProgressAndPreservesResults` with three increasing pages. Assert start/completion events use 1-based page numbers, completion advances in order, the total is stable, and `Align` with and without a callback returns identical result values.

Create `TestAlignProgressCancellationStopsSearch` whose callback cancels context at the second page start; assert `Align` returns `context.Canceled` and emits no third-page candidate event.

Create `TestProofreadingOCRAlignmentProgressIsIndependentOfPageOrigin` around the shared OCR alignment helper. Run the same recognized OCR fixture through the helper twice (representing cache-hit and just-recognized input); assert equal results and identical candidate-page/path phase events. Change only the B draft and assert the helper recalculates ranges from the new draft rather than retaining cached alignment.

Create `TestProofreadingOCRCacheHitSkipsCredentialLookupAndUsesCurrentDraft` using a complete local OCR page cache and synthetic credential store. Assert a hit reaches alignment without credential lookup or remote-job submission; after changing only B text, assert OCR pages are still reused and the evidence range is recomputed for the new draft.

Create `TestProofreadingOCRCacheRoundTripsCompleteBook` and `TestProofreadingOCRCacheRejectsIncompleteOrMismatchedPageSet` for the cache code introduced in `fed4c5e`: complete page/layout records round-trip, while incomplete manifests, wrong source/model/options/page count, or missing/corrupt page records are not treated as a valid hit. These pin the existing contract as this feature starts consuming the cache.

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/textalign ./internal/desktopui -run 'Test(Align(ReportsCandidateProgressAndPreservesResults|ProgressCancellationStopsSearch)|ProofreadingOCR(Cache(RoundTripsCompleteBook|RejectsIncompleteOrMismatchedPageSet)|AlignmentProgressIsIndependentOfPageOrigin|CacheHitSkipsCredentialLookupAndUsesCurrentDraft))$' -count=1`

Expected: the new alignment-progress and cache-hit flow tests FAIL because `Options.OnProgress` and `AlignmentProgress` are not defined and neither cache/fresh path publishes candidate-search progress through the shared alignment helper. The cache serialization contract tests may pass against the preceding commit; record that baseline rather than changing cache behavior unnecessarily.

- [x] **Step 3: Implement progress events in `internal/textalign/path.go`**

Call the optional callback immediately before and after each page's candidate search, then report the transition to global path resolution. Keep callbacks synchronous and document that callers must publish quickly/non-blockingly. Continue honoring context cancellation before processing the next page and during existing DP loops. Pass the callback through the shared OCR page alignment helper so both cached and fresh OCR results use it; cache hits still rerun against the current B text. Remove the synthetic last-page matching progress emitted before alignment on fresh OCR results; the matcher callback is the source of the active A page, for both cached and fresh inputs.

- [x] **Step 4: Run the progress tests**

Run: `go test ./internal/textalign ./internal/desktopui -run 'Test(Align(ReportsCandidateProgressAndPreservesResults|ProgressCancellationStopsSearch)|ProofreadingOCR(Cache(RoundTripsCompleteBook|RejectsIncompleteOrMismatchedPageSet)|AlignmentProgressIsIndependentOfPageOrigin|CacheHitSkipsCredentialLookupAndUsesCurrentDraft))$' -count=1`

Expected: PASS; callback-enabled and callback-free matching results are equal, cached/fresh OCR page inputs produce equivalent progress and results for the same B draft, a complete cache hit bypasses credentials while B changes trigger fresh alignment, and invalid cache records remain misses.

- [x] **Step 5: Write a failing path-choice test**

Create `TestAlignPathChoicesExcludeLocallyRankedButGloballyUnsupportedCandidates`. Use repeated text on adjacent pages where a local candidate overlaps or reverses the selected next-page span. Assert `PathChoices` contains the selected path and other globally supported near-optimal paths only; an overlapping local decoy is absent. Assert an unambiguous matched result has only its selected choice.

- [x] **Step 6: Run the path-choice test to verify it fails**

Run: `go test ./internal/textalign -run '^TestAlignPathChoicesExcludeLocallyRankedButGloballyUnsupportedCandidates$' -count=1`

Expected: FAIL because `Result.PathChoices` is not defined.

- [x] **Step 7: Populate `Result.PathChoices` from the path solver support set**

Preserve the current local `Candidates` API. Deduplicate supported candidate indices, keep deterministic order, and copy only those candidates justified by the best route or the existing near-optimal route checks. Do not turn the path score into a probability.

- [x] **Step 8: Run alignment regression tests**

Run these separately:

```sh
go test ./internal/textalign -count=1
go test ./internal/desktopui -run '^TestProofreadingOCR(Cache(RoundTripsCompleteBook|RejectsIncompleteOrMismatchedPageSet)|AlignmentProgressIsIndependentOfPageOrigin|CacheHitSkipsCredentialLookupAndUsesCurrentDraft)$' -count=1
```

Expected: PASS, including the progress and path-choice tests plus existing global path and boundary regressions.

- [x] **Step 9: Commit Task 1**

Commit the cache-aware `textalign` API, UI progress bridge, path choices, and focused tests as `feat: add cache-aware OCR alignment progress and choices`.

### Task 2: Summarize Located Endpoints and Project B Ranges onto X Pages

**Files:**
- Create: `internal/desktopui/proofread_ocr_alignment.go`
- Create: `internal/desktopui/proofread_ocr_alignment_test.go`
- Modify: `internal/desktopui/proofread_ocr.go`
- Modify: `internal/desktopui/proofread_controller.go`

**Interfaces:**
- Consumes Task 1's `Result.PathChoices`.
- Produces `proofreadingOCRRangeSegment{PageIndex, Start, End int}` and `proofreadingOCRRangeSegments(pages []proofreadingXPage, r textalign.Range) []proofreadingOCRRangeSegment`; `PageIndex` is zero-based, while `Start`/`End` are page-local half-open rune offsets.
- Produces `proofreadingOCRAlignmentAnchor{APage int, BRange textalign.Range, XFirstPage int, XLastPage int}` and `proofreadingOCRAlignmentSummary{First, Last, ProvisionalFirst, ProvisionalLast *proofreadingOCRAlignmentAnchor, ModelRevision uint64}`. Page numbers in summary anchors are one-based for display. Confirmed first/last use earliest/latest `matched` A pages; provisional fields separately report earliest/latest `ambiguous` pages. Never label an ambiguous result as confirmed. Recompute X page spans from current page ownership instead of retaining a stale page map.

- [x] **Step 1: Write failing range projection tests**

Create `TestProofreadingOCRRangeSegmentsAcrossPagesAndUnicode` with `proofreadingXPage` ownership ranges for Han text, a non-BMP rune, a base letter plus combining mark, and a final span crossing three pages; assert each returned page-local rune interval exactly matches the global half-open range.

Create `TestProofreadingOCRRangeSegmentsSkipsEmptyPagesAndRejectsInvalidRange` with adjacent/empty page ownership, an exact page-edge range, reversed offsets, and an out-of-bounds end. Assert empty pages produce no segment and invalid ranges produce none.

- [x] **Step 2: Run these tests to verify they fail**

Run: `go test ./internal/desktopui -run '^TestProofreadingOCRRangeSegments' -count=1`

Expected: FAIL because the range projection helper is not defined.

- [x] **Step 3: Implement range projection in `proofread_ocr_alignment.go`**

Use only `proofreadingXPage.ContentStart`/`ContentEnd`; compute interval intersections in rune coordinates, preserve half-open endpoints, and omit empty intersections. Validate that page spans cover the requested range; do not silently clamp an invalid mapping. Do not decode Markdown or map UTF-8 bytes in the UI helper.

- [x] **Step 4: Run range projection tests**

Run: `go test ./internal/desktopui -run '^TestProofreadingOCRRangeSegments' -count=1`

Expected: PASS for Han, non-BMP, exact-edge, cross-page, and empty-page cases.

- [x] **Step 5: Write failing alignment summary tests**

Create `TestProofreadingOCRAlignmentSummaryUsesFirstAndLastMatchedPages` with leading/trailing ambiguous and unmatched pages, two matched pages, and a `BoundaryAfter` range. Assert only `matched` pages supply confirmed endpoints, ambiguous pages remain provisional, and the cut envelope is preserved unchanged.

Create `TestProofreadingOCRAlignmentSummaryRecomputesAfterOwnershipChange` by moving an existing boundary without changing B text; assert the summary's X page span reflects the current ownership ranges.

Create `TestProofreadingOCRAlignmentSummaryHasNoConfirmedAnchorWhenAllPagesAreAmbiguousOrUnmatched`; assert no start/end is invented when the book edges are not matched.

- [x] **Step 6: Run the summary test to verify it fails**

Run: `go test ./internal/desktopui -run '^TestProofreadingOCRAlignmentSummary(HasNoConfirmedAnchorWhenAllPagesAreAmbiguousOrUnmatched|RecomputesAfterOwnershipChange|UsesFirstAndLastMatchedPages)$' -count=1`

Expected: FAIL because the summary type/helper is not defined.

- [x] **Step 7: Implement the summary helper and publish it with OCR results**

Derive A endpoints from the completed ordered results and X spans from range projection. Do not assume A page 1 maps to B offset 0 or that the final A page maps to the end of B.

- [x] **Step 8: Run summary and OCR state tests**

Run: `go test ./internal/desktopui -run 'Test(ProofreadingOCRAlignmentSummary(HasNoConfirmedAnchorWhenAllPagesAreAmbiguousOrUnmatched|PublishedWhenResultsArrive|RecomputesAfterOwnershipChange|UsesFirstAndLastMatchedPages)|ProofreadingOCRResultsClearWhenDraftChangesAfterScan|ProofreadingOCRResultsClearWhenSourceHashChangesAfterScan)$' -count=1`

Expected: PASS; summaries use current OCR results and are recomputed from current X ownership after a boundary move.

- [x] **Step 9: Commit Task 2**

Commit the alignment range model and tests as `feat: summarize OCR alignment endpoints`.

### Task 3: Render B Evidence Highlights in X Without Changing the Draft

**Files:**
- Create: `internal/desktopui/proofread_ocr_highlight.go`
- Create: `internal/desktopui/proofread_ocr_highlight_test.go`
- Modify: `internal/desktopui/proofread_projection.go`
- Modify: `internal/desktopui/proofread_window.go`

**Interfaces:**
- Consumes Task 2's `proofreadingOCRRangeSegment` and the current scan's active `Evidence.Range`.
- Produces a short-lived highlight state keyed by A page, OCR block ID, B range, source hashes, OCR generation, and proofreading model revision.
- Renders highlight regions through the existing X `widget.Editor.Regions` window; the editor's text, selection, caret, and ownership are unchanged.

- [x] **Step 1: Write failing highlight-state and geometry tests**

Create `TestProofreadingOCRHighlightClipsToEditorWindow` for a target B range partially outside the current X editor window; assert only the visible intersection produces highlight regions and local offsets are correct.

Create `TestProofreadingOCRHighlightRejectsStaleOwnershipAndGeneration` where either the draft identity, proofreading model revision, or OCR generation changes; assert no active highlight survives.

- [x] **Step 2: Run these tests to verify they fail**

Run: `go test ./internal/desktopui -run '^TestProofreadingOCRHighlight(ClipsToEditorWindow|RejectsStaleOwnershipAndGeneration)$' -count=1`

Expected: FAIL because OCR linked-highlight state and clipping are not defined.

- [x] **Step 3: Add highlight state and a focused drawing helper**

Add a read-only `drawProofreadingOCRMatchHighlight` helper that intersects the active B range with `proofreadingXPage` ownership and the rendered editor window, then paints `editor.Regions` above text with a translucent OCR-match color. Keep existing blue caret/selection and review-page decoration visible.

- [x] **Step 4: Run highlight tests**

Run: `go test ./internal/desktopui -run '^TestProofreadingOCRHighlight(ClipsToEditorWindow|RejectsStaleOwnershipAndGeneration)$' -count=1`

Expected: PASS; no text or cursor state changes occur.

- [x] **Step 5: Add a rendered visual regression**

Add `TestProofreadingOCRLinkedRangeVisual` using synthetic text and a fake alignment range. Assert the intended glyph region receives the translucent match overlay across a wrapped line and that the underlying text pixels remain readable.

- [x] **Step 6: Run the rendered visual test**

Run: `go test ./internal/desktopui -run '^TestProofreadingOCRLinkedRangeVisual$' -count=1 -v`

Expected: PASS on the supported Metal/GPU test runner; a skip must be recorded as unverified, not as visual success.

- [x] **Step 7: Commit Task 3**

Commit the non-destructive X highlight as `feat: highlight OCR evidence in proofreading text`.

### Task 4: Link OCR Blocks to X Candidates with Temporary Independent Scrolling

**Files:**
- Create: `internal/desktopui/proofread_ocr_link.go`
- Create: `internal/desktopui/proofread_ocr_link_test.go`
- Modify: `internal/desktopui/proofread_ocr_overlay.go`
- Modify: `internal/desktopui/proofread_ocr_chrome.go`
- Modify: `internal/desktopui/proofread_projection.go`
- Modify: `internal/desktopui/proofread_ocr.go`
- Modify: `internal/desktopui/proofread_window.go`
- Modify: `internal/desktopui/pdf_workspace_layout.go`
- Modify: `internal/desktopui/proofread_page_filter.go`

**Interfaces:**
- Consumes Tasks 1–3: OCR block ID, globally supported `PathChoices`, range projection, and highlight state.
- Produces `proofreadingOCRLinkState` with selected A page/block, current path-choice index, source/result fingerprints, and linked X target span.
- `proofreadingOCRLinkState` contains `Active bool`, zero-based `APage int`, OCR `BlockID int`, zero-based `PathChoiceIndex int`, `BRange textalign.Range`, source hashes, OCR generation, and proofreading model revision.
- Produces `proofreadingOCRBlockRange{PathChoiceIndex int, Range textalign.Range}`, `proofreadingOCRBlockChoice(page proofreadingOCRPage, blockID int) []proofreadingOCRBlockRange`, and `proofreadingOCRChoiceIndex(current, delta, choiceCount int) int` for testable evidence selection and cycling. Link activation captures the proofreading model revision so page-ownership edits invalidate the link even if B text is unchanged.
- `proofreadingOCRChoiceIndex` returns `-1` when there are no choices; otherwise it wraps in `[0, choiceCount)`.

- [x] **Step 1: Write failing link and candidate tests**

Create `TestProofreadingOCRBlockChoiceUsesOnlyPathChoices` with two OCR blocks, multiple path choices, and a locally ranked decoy in `Result.Candidates`; assert the chosen block exposes only its own evidence from `Result.PathChoices`. Create `TestProofreadingOCRBlockChoiceRejectsUnmatchedOrLimitedPage` to assert those statuses never open a B link.

Create `TestProofreadingOCRChoiceIndexWraps` with a selected choice, one supported alternative, and zero choices; assert next/previous wraps deterministically and an empty choice list returns `-1`.

- [x] **Step 2: Run link tests to verify they fail**

Run: `go test ./internal/desktopui -run '^TestProofreadingOCR(BlockChoice(UsesOnlyPathChoices|RejectsUnmatchedOrLimitedPage)|ChoiceIndexWraps)$' -count=1`

Expected: FAIL because OCR block-choice and path-choice handlers are not defined.

- [x] **Step 3: Implement stable OCR block interaction and link state**

Register pointer hit regions only for visible OCR blocks with evidence. A unique matched block activates its B range. An ambiguous page shows only globally supported path choices, keeps the amber review state, and exposes the current choice count with stable previous/next controls in the OCR status chrome; it never promotes a candidate to `matched`. Unmatched and limited pages do not activate links or B highlights.

- [x] **Step 4: Add failing independent-scroll interaction tests**

Create `TestProofreadingOCRLinkedCompareKeepsSourcePageAndMovesXToBPage` and `TestProofreadingOCRUnlinkRestoresSynchronizedScroll`. Assert A stays on the clicked PDF page while X moves to the B range; X can scroll through a multi-page B span while A stays pinned; explicit unlink and Escape restore the established A/X shared position. Include A manual-scroll exit, B-edit invalidation, and model-revision invalidation when only ownership changes.

Create `TestProofreadingOCRLinkClearsOnCancelOrGenerationChange` to assert cancelling the scan or receiving a newer OCR generation removes link and highlight state.

- [x] **Step 5: Run the independent-scroll tests to verify they fail**

Run: `go test ./internal/desktopui -run '^TestProofreadingOCR(LinkedCompareKeepsSourcePageAndMovesXToBPage|UnlinkRestoresSynchronizedScroll|LinkInvalidatesOnOwnershipRevision|LinkClearsOnCancelOrGenerationChange)$' -count=1`

Expected: FAIL because linked comparison mode is not implemented.

- [x] **Step 6: Implement temporary linked scrolling and safe exit behavior**

While a link is active, stop the usual X-to-A and A-to-X position mirroring, hold A at its source page, and navigate X to the projected evidence range. X scrolling remains independent so the user can inspect a multi-page range. Escape, explicit unlink, A manual navigation/scroll, filter change, draft change, source change, model-revision change, or stale generation clears the link and resumes A/X synchronization with A authoritative. Keep all clickables and event tags in stable state.

- [x] **Step 7: Run link and OCR regression tests**

Run: `go test ./internal/desktopui -run 'TestProofreadingOCR(BlockChoice(UsesOnlyPathChoices|RejectsUnmatchedOrLimitedPage)|ChoiceIndexWraps|LinkedCompareKeepsSourcePageAndMovesXToBPage|UnlinkRestoresSynchronizedScroll|LinkInvalidatesOnOwnershipRevision|LinkClearsOnCancelOrGenerationChange|ResultsClearWhenDraftChangesAfterScan|ResultsClearWhenSourceHashChangesAfterScan)$' -count=1`

Expected: PASS; manual navigation and stale data always restore normal synchronized scrolling.

- [x] **Step 8: Commit Task 4**

Commit the interaction and scrolling behavior as `feat: link OCR blocks to proofreading spans`.

### Task 5: Show First/Last Anchors, Boundary Evidence, and Real Matching Animation

**Files:**
- Modify: `internal/desktopui/proofread_ocr.go`
- Modify: `internal/desktopui/proofread_ocr_chrome.go`
- Modify: `internal/desktopui/proofread_ocr_overlay.go`
- Modify: `internal/desktopui/proofread_layout.go`
- Modify: `internal/desktopui/pdf_workspace_layout.go`
- Modify: `internal/desktopui/proofread_page_filter.go`
- Modify: `internal/i18n/locales/zh-CN.json`
- Modify: `internal/i18n/locales/en-US.json`
- Modify: `internal/desktopui/proofread_ocr_alignment_test.go`
- Modify: `internal/desktopui/proofread_visual_test.go` or create a focused OCR visual test file

**Interfaces:**
- Consumes Task 1's `AlignmentProgress` and Task 2's `proofreadingOCRAlignmentSummary`.
- The UI publishes progress through the existing synchronized channel; matching updates carry phase, current A page, completed candidate pages, and total pages.
- The OCR scan state owns the current progress and the active `proofreadingOCRLinkState`; animation never owns or mutates match results.
- Scan-follow defaults on during OCR/matching. A manual A navigation disables it; the status chrome can re-enable it. X follows A during ordinary scanning, while linked comparison keeps X independent as defined in Task 4.

- [x] **Step 1: Write failing scan-state tests for progress and endpoints**

Create `TestProofreadingOCRMatchingProgressPublishesCurrentCandidatePage` to assert worker progress updates the active A page and UI status without publishing final matches early.

Create `TestProofreadingOCRMatchingFollowStopsOnManualSourceNavigation` to assert scan-follow begins enabled, a manual A navigation disables it, and the stable follow control re-enables it.

Create `TestProofreadingOCRStatusShowsFirstLastLocatedAnchorAndBoundary` to assert first/last matched page numbers, X target span, and `point`/`range`/`unresolved` are localized as evidence states, including no confident anchors.

Create `TestProofreadingOCRAnimationStopsForHiddenPageOrInactiveView` to assert animation schedules no frame invalidation when the progress page is hidden, the proofreading tab is inactive, or the scan has ended.

- [x] **Step 2: Run these tests to verify they fail**

Run: `go test ./internal/desktopui -run '^TestProofreadingOCR(MatchingProgressPublishesCurrentCandidatePage|MatchingFollowStopsOnManualSourceNavigation|StatusShowsFirstLastLocatedAnchorAndBoundary|AnimationStopsForHiddenPageOrInactiveView)$' -count=1`

Expected: FAIL because text-alignment progress and endpoint status are not surfaced.

- [x] **Step 3: Publish `textalign.Align` progress through the OCR worker**

Pass `Options.OnProgress`; send non-blocking progress events tagged with OCR generation. Ignore stale events. Distinguish candidate search from global path resolution in the status line. Keep final result publication unchanged and wait for full path completion before exposing matches.

- [x] **Step 4: Show endpoint and boundary status in both locales**

Extend the OCR status UI with earliest/latest algorithmically matched A page and X page span; report provisional-only or absent anchors accurately. Show the active A page's page-cut `point`, `range`, or unresolved state without applying it.

- [x] **Step 5: Animate real matching progress and result pulses**

During candidate search, use real progress to move A/X to the current A page and sweep the existing laser over that page; show current and completed/total candidate pages. Manual A navigation disables scan-follow; provide a stable status control to re-enable it. Restart the laser phase on entry to matching. After final results, briefly pulse the clicked OCR block and its B range together, then leave a low-alpha persistent highlight. Stop scheduled invalidations on cancellation, result completion, hidden/inactive tab, or window close.

- [x] **Step 6: Run progress and focused OCR tests**

Run: `go test ./internal/textalign ./internal/desktopui ./internal/i18n -run 'Test(AlignReportsCandidateProgressAndPreservesResults|AlignProgressCancellationStopsSearch|ProofreadingOCR(AlignmentProgressIsIndependentOfPageOrigin|CacheHitSkipsCredentialLookupAndUsesCurrentDraft|MatchingProgressPublishesCurrentCandidatePage|MatchingFollowStopsOnManualSourceNavigation|StatusShowsFirstLastLocatedAnchorAndBoundary|AnimationStopsForHiddenPageOrInactiveView|LinkedRangeVisual))$' -count=1 -v`

Then run: `go test ./internal/i18n -count=1`

Expected: targeted tests PASS and both locale catalogs parse with no missing or divergent keys.

- [x] **Step 7: Verify animation behavior at 1x and 2x render scales**

Run: `go test ./internal/desktopui -run '^TestProofreadingOCR.*Visual$' -count=1 -v`

Expected: PASS on the supported GPU runner; record any GPU-dependent `SKIP` as not visually verified.

- [x] **Step 8: Commit Task 5**

Commit the progress, endpoint UI, and animation as `feat: animate OCR text alignment progress`.

### Task 6: Update Algorithm Notes and Verify the Real Workbench

**Files:**
- Modify: `internal/textalign/README.md`
- Modify: `internal/ocr/README.md`
- Verify: existing proofreading UI and OCR tests

- [x] **Step 1: Update package documentation**

Remove stale statements that text alignment is not connected to the UI. Document that progress is page-level candidate-search progress, results remain conservative, and X highlights/links are read-only. Preserve the existing first-six-page evidence and its uncertainty. Document that the local OCR cache stores only complete OCR page/layout results keyed by PDF/model/options/page count; cached OCR is realigned against the current B draft, and interrupted partial cloud OCR still resumes through its saved job ID.

- [x] **Step 2: Run the targeted regression command**

Run: `go test ./internal/textalign ./internal/ocr ./internal/desktopui ./internal/i18n -run 'Test(Align|ProofreadingOCR|InactiveProofreadingTabProcessesOCRSubmissionPersistence|PDFEntryResumesComparisonWithoutDuplicateOrHistory|Proofreading.*Linked|Proofreading.*Range|Proofreading.*Highlight)' -count=1`

Expected: PASS. Do not interpret a skipped GPU visual test as a pass.

- [x] **Step 3: Check patch formatting and localization parity**

Run: `git diff --check`

Expected: PASS. Confirm both locale catalogs parse and contain matching keys, including the new progress, candidate, endpoint, and boundary messages.

- [x] **Step 4: Run the fresh real-workbench verification**

Before launching, find and kill all running Glance app processes and `go run ./cmd/glance` launchers, then run from `/Users/mima1234/project/glance-go`:

```sh
GLANCE_PPROF_ADDR=127.0.0.1:6060 go run ./cmd/glance
```

Use the fresh process to verify it opens with the new code. Exercise progress animation, A-block activation, X-range highlight, ambiguity choice, unlink, and ordinary scroll sync through the available UI or a previously completed small OCR task. If none is available, use a bounded first-six-page sample from the previously authorized source; do not submit a new whole-book cloud OCR task solely for this UI check. Keep temporary artifacts inside the current code directory and remove only those created for this check.

Expected: record which behavior was observed in the fresh process and which was covered only by automated tests; separately record any CUA/native-window attachment limitation. Do not claim actual window rendering passed unless observed in that fresh process.

- [x] **Step 5: Final diff review and commit Task 6**

Review `git diff --check` and the final source/doc changes, confirm no unrelated files or verification artifacts changed, then commit the documentation changes as `docs: document OCR linked alignment behavior`.

### Verification record

- The fresh `GLANCE_PPROF_ADDR=127.0.0.1:6060 go run ./cmd/glance` process opened the previously loaded 892-page PDF at page 1. No existing Glance app or `go run ./cmd/glance` launcher was running before launch.
- The window was visible in a screen capture, but the active workspace was the regular PDF reader; no proofreading B document, completed OCR cache result, or alignment session was open. The CUA connector timed out when attaching to the unbundled `go run` process (`-10005`), and macOS accessibility exposed no controls for that custom window. A-block activation, X highlighting, candidate choice, and linked scrolling therefore remain verified by automated interaction/render tests rather than observed in this fresh window.
- No cloud OCR task was submitted for this UI check. The temporary screen capture was removed after inspection.
