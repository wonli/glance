# Word ruler verification — 2026-10-19

Implemented against Pencil node `jDV9e`: 28 dp, centimetres and quarter-centimetre ticks, margin shading, distinct first-line/hanging/whole-left/right controls. Existing document corner marks are unchanged.

## Passing checks

- Final Metal GPU run: `GLANCE_RULER_SCREENSHOT_DIR=/tmp go test ./internal/desktopui -run '^TestWord(Ruler|ViewRuler|NativePinch|PageFit|Page.*Scroll)' -count=1 -v`. Both 1x and 2x visual tests ran and passed, without GPU skips. Synthetic ruler screenshots were inspected against the design.
- `TestWordRulerFourGesturesSaveAndCancel`: actual routed pointer gestures at 1x/2x, legacy fallback and canonical 50%/150% zoom, including scroll offset, release-only document mutation, Escape, pointer cancellation, no movement, and DOCX save/reopen.
- Geometry tests cover centimetre conversion, zoom/density/scroll, negative-indent bounds, existing out-of-bounds no-op values, empty table cells, columns, and caret continuations across columns/paired pages.
- `go test ./internal/format/docx ./internal/wordlayout`: complete package suites pass. DOCX tests cover atomic writes, mutually exclusive first-line/hanging attributes, untouched paragraph content/properties and repeated no-op; layout tests cover resolved character units and independent cloned metric maps.
- `go build -o /tmp/glance-word-ruler ./cmd/glance`: passes. Apple's duplicate `-lobjc` linker warning remains.
- `git diff --check`: passes.
- Independent read-only review: no remaining important findings after no-op, table bounds and caret-continuation fixes.

## Full-suite result

`go test ./...` was run both inside the sandbox and with GPU/local-socket access. It is **not green**. The GPU run completed with desktopui taking 449 seconds. It preceded the last review corrections and old ruler-height fixture update; focused reruns after those changes pass. The full suite was not repeated after that final iteration.

The full-run ruler pixel assertion was corrected to account for the small marker's antialiased white outline at 1x; final actual pixel verification passed. The old `TestWordCanvasAndHorizontalRulerMatchPenClpgmGeometry` ruler and canvas Y bounds were updated for the approved 33-to-28 dp change; its unrelated Ribbon tab assertions already fail in HEAD.

Failures reproduced using an unmodified `git archive HEAD` source copy:

- `TestAgentHistorySearchAndDateGroups`
- `TestParseCellRangeAddress`
- `TestGridResizeBoundaryDoubleClickAutoFitsColumn`
- `TestCellSelectionBoundsClampsToXLSXLimits`
- `TestWordCanvasAndHorizontalRulerMatchPenClpgmGeometry` — only the pre-existing “文件/开始” Ribbon tab bounds remain; ruler/canvas expectations updated.
- `TestPrivateFontSceneExportKeepsLoadedResourceAfterRefresh`
- `TestLoadManifestLoadsCuratedCorpus`

Other full-run failures (not investigated or claimed pre-existing):

- `TestAgentRibbonTextFormsCenteredGroup`
- `TestMarkdownPageFocusKeepsTextDragSelection`
- `TestDesktopMCPStdioEditsTheLiveGioSession`
- `TestSettingsFieldBlankEdgesFocusEditor`
- `TestSettingsSavedKeyEdgeClickFocusesVisiblePreview`
- `TestSettingsAPIKeyEyeClickTogglesVisibility`
- `TestSettingsFieldsVerticallyCenterText`
- `TestSpreadsheetStatusBarShowsEndModeIndicator`
- `TestSpreadsheetZoomInteractionsAndClamping`
- `TestSpreadsheetRecalculateAndRefreshAll`
- `TestSpreadsheetToolbar_SpellCheckAndTranslate`
- `TestSpreadsheetToolbarActionsCoverHomeAndDataGroups`
- `TestExcelRibbonActionsUseBundledIconAssets`
- `TestSpreadsheetViewModesAndRuler`
- `TestHideMainWindowToTrayPreconditions`
- `TestWordHomeRibbonKeepsCommandGeometryAfterAgentEntry`
- `TestWordVersionAutoSaveUsesThrottledSource`
- `TestJSONAndCSVCloseMemoryReclaim`
- `TestImportedXLSSaveAsRejectsXLSAndKeepsSource`
- `TestMCPCLIRecognizesServeAndDocumentsStdoutContract`
- `TestDesktopMCPIPCServesExistingCapabilityCatalog`

The sandbox-only run additionally hit restricted TCP/Unix socket creation; the GPU run's MCP checks reported the stable endpoint already active. These are not counted as passing checks.

## Remaining validation limits

No native IME candidate-panel or external Word/WPS reopening was manually exercised. Pending synthetic text drafts and DOCX save/reopen are covered by automated regressions. Dragging previews the markers; canonical text repaginates after release. First-line/hanging drags convert character units to absolute twips, while whole-left/right drags preserve the existing first-line representation.

## Loading-frame follow-up

The opening flash came from the pre-pagination fallback: it painted guessed page geometry and legacy canvas-width grips before the background scene arrived. The runtime now reserves the 28 dp ruler slot as workspace background until a usable scene is accepted. Empty/unsupported results also leave the slot neutral; repagination retains any usable accepted scene.

`TestWordRulerLoadingDoesNotExposeLegacyControls` reproduces the original failure for pre-start, pending, failed, empty and unsupported layout states. `TestWordRulerLoadingToReadyVisual` checks real 1x/2x GPU pixels: the loading slot is uniform workspace background, and the first accepted partial scene, completed scene and subsequent stable frame have identical ruler pixels. The old sidebar legacy-grip expectation was replaced with `TestWordRulerLoadingWithAgentSidebarHidesLegacyHandle`, which checks that the hidden grip neither paints nor changes canvas width.

## Global visibility and queued-result follow-up

Two regressions were reproduced before this correction: `TestWordRulerReadyResultsDoNotWaitForUserInput` left a previewable completion queued behind a non-previewable partial after one result-processing pass, and `TestWordRulerControlsVisibleWithoutTextSelection` found all four marker controls absent before a text click. Background invalidations can coalesce, so consuming only one result per frame made the ready ruler depend on a later input event.

The result consumer now processes a bounded channel-capacity batch and requests another frame if a queue remains. An independent per-view ruler context chooses the first visible paragraph initially and retains the last active paragraph/run/caret after text selection clears; rendering does not create a text selection or focus an editor. Geometry and drag intents use that same context.

Both reproductions pass after correction. `TestWordRulerKeepsContextAfterTextSelectionClears` compares actual 1x/2x Metal pixels before/after the first text click and after clearing selection in another paragraph. `TestWordRulerFourGesturesSaveAndCancel` now starts without a text selection and still passes real pointer/save/reopen checks at 1x/2x and 50%/150% zoom. The complete ruler, view-toggle, pinch and fit-zoom focused run passes without GPU skips. Independent review found no important remaining issue. Full-suite failures from the prior run remain recorded above; this follow-up does not claim a green full suite.

## Full-width surface follow-up

The approved follow-up expands only the ruler surface/outer border to the content viewport. Page geometry, centimetre spacing, shaded page margins, unit label and indent positions retain their document coordinates. Navigation and AI sidebars remain outside the ruler viewport. The neutral loading slot remains unchanged.

`TestWordRulerSurfaceFillsWorkspaceWithoutStretchingTicks` first failed on the old workspace-colored outer area, then passed after the change. It checks actual 1x/2x Metal pixels for full-width surface and border, page-relative 1 cm ticks and the real AI-sidebar layout. The complete focused ruler/loading/focus/drag/zoom/fit regression run passes without GPU skips. Independent review found no actionable issues.

The authoritative latest Pen `jDV9e` was re-read after the color clarification: the text span is white `#FFFFFF`; both non-text sides use `#E0E4EA` through the viewport edges, including the page margins and space outside the page. The full-width border is top/bottom only (`#CCD2DB`), with separate page-edge lines (`#B8C1CE`) and text-boundary lines (`#9AA9BD`). The `cm` label stays 16 dp from the workspace left edge in `#687586`. Uniform gray was an incorrect interpretation and is replaced by these actual node partitions. The 1440x28 synthetic rendering was inspected against the node screenshot; 1x/2x pixel and interaction regressions pass.
