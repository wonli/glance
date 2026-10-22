# Proofreading page content model verification

Date: 2026-10-22

## Interaction contract

- Scrolling in A mirrors the outer page position in X.
- Scrolling inside an X paper page, including its scrollbar, changes only that page's viewing offset. It does not change text or ownership.
- Scrolling in the X exterior uses shaped line boundaries to adjust the active locked page's ownership. Scrolling up at the adjustment start cannot pull text in. With no active locked page, X exterior scrolling remains linked to A.
- Page viewing offsets and ownership-adjustment offsets are separate, so filling a locked page does not move its reading position.

## Automated evidence

- `go test ./internal/proofreading ./internal/application -count=1` passed. This includes stable character ownership, undo/redo, shaped-line adjustment, forward/reverse adjustment, and upward scroll at the adjustment start after splitting a locked page.
- `go test ./internal/desktopui -run '^TestProofreading' -count=1` passed. Coverage includes local page scrolling, scrollbar drag and range, exterior ownership adjustment, no-lock A/X synchronization, A-to-X scroll mirroring, stable caret split after page scrolling, reviewed-page isolation, and long-text edit preservation.
- `go test ./internal/format/pdfdoc -count=1` passed, including visual PDF rendering without extracted text runs.
- `go build ./...` passed. The linker printed the existing macOS warning about duplicate `-lobjc` libraries.
- The latest `TestProofreadingLockedPageVisual` and `TestProofreadingLongPageViewportGPU` run reported `SKIP`: the test process could not create a headless Metal device. This is not visual-test evidence for the latest build.

## Earlier native-window limit (superseded by the follow-up below)

The disposable synthetic QA app could not be opened by macOS LaunchServices (`kLSNoExecutableErr`, -10827). Launching its executable directly from the PTY aborted inside Gio's macOS main loop (`SIGABRT` in `gio_main`). Therefore scroll routing has input-router regression coverage, but this pass did not verify it in a live native window. No user PDF or B document was opened or modified during the attempted QA.


## Follow-up: X layout and exterior adjustment fixes (2026-10-22)

### Reproduced causes and changes

- List rows shrink-wrapped X paper to the left inset when their cross-axis minimum was zero. Wheel/menu hit testing assumed a centered page. An input-router regression observed paper bounds `(24,12)-(424,612)` in a 1000px pane, versus the intended `(300,12)-(700,612)`. Rows now occupy the pane width and keep their vertical minimum clear.
- The background Word layout and visible Gio editor disagreed on real mixed Chinese/English line geometry: the second baseline delta was 21.5 versus 15, with increasing error down the page. The application accepts neutral source-aligned line geometry from a worker-owned Gio shaper. The editor uses the same document-coordinate shaping parameters and scales its recorded operations for display, including Retina. The native editor's bounded text window receives natural height so its private scroll-to-caret does not compete with the page viewport.
- Reversing exterior adjustment to the origin on a short locked page retained acquired characters simply because they still fit in one viewport. A failing controller regression acquired `cdefgh` onto locked `ab` and could not return them. At the origin, reverse adjustment now requests returning the acquired suffix; the model retains protected original/newly typed characters and ownership barriers.

### Validation

- New regressions cover real line/caret geometry at 1x, 1.25x and 2x; centered paper versus wheel bounds; Chinese insertion after a page-window scroll at 1x/2x; complete long-page tail access; and short-page acquisition/reversal.
- Metal pixel comparisons cover the same repeated lines before/after switching the bounded editor window at 1x and 2x, with a two-level channel tolerance for GPU antialiasing. These run with GPU access, rather than treating SKIP as success.
- Native QA used a disposable, single-window app with numbered synthetic Chinese/English lines and no workspace/persistence. Observed centered A/X pages; exterior acquisition without moving A or the paper; independent internal viewing; and reversal to the original 28-line locked base with the local scrollbar disappearing. The temporary QA source files were removed after validation.
- Independent review found no confirmed actionable issue. Remaining coverage limitation: private-font/RTL configurations have not been exercised; ordinary shell font/locale is covered.
- Detailed focused output: `/private/tmp/glance-proofread-final-gpu-tests.log`. Full-suite output: `/private/tmp/glance-proofread-full-unsandboxed.log`.

### Full-suite limitations

`go test ./...` was attempted both inside and outside the sandbox. The sandbox run encountered denied localhost listeners, GPU access and database writes. The GPU-enabled full run still failed outside this proofreading fix's scope (including locale-sensitive expectations, ribbon assets/geometry, spreadsheet/Word interactions, PDF font-resource lifetime, directory limits, MCP CLI help and corpus expectations). The full run compiled the earlier Chinese-input test event before its test harness was corrected to provide the actual selected replacement range; that one regression passes in the final focused run. The remaining full-run failing test names are recorded below; they were not repaired as part of this task.

- `TestAgentHistorySearchAndDateGroups`
- `TestAgentRibbonTextFormsCenteredGroup`
- `TestParseCellRangeAddress`
- `TestGridResizeBoundaryDoubleClickAutoFitsColumn`
- `TestCellSelectionBoundsClampsToXLSXLimits`
- `TestMarkdownPageFocusKeepsTextDragSelection`
- `TestDesktopMCPStdioEditsTheLiveGioSession`
- `TestPenRibbonEveryReferenceGlyphHasVisiblePixels`
- `TestPenRibbonBrandAndActiveStateVisual`
- `TestSettingsFieldBlankEdgesFocusEditor`
- `TestSettingsSavedKeyEdgeClickFocusesVisiblePreview`
- `TestSettingsAPIKeyEyeClickTogglesVisibility`
- `TestSettingsEyeRevealsSavedKeyWithoutCreatingDraft`
- `TestSettingsFieldsVerticallyCenterText`
- `TestSpreadsheetStatusBarShowsEndModeIndicator`
- `TestSpreadsheetZoomInteractionsAndClamping`
- `TestSpreadsheetRecalculateAndRefreshAll`
- `TestSpreadsheetToolbar_SpellCheckAndTranslate`
- `TestSpreadsheetToolbarActionsCoverHomeAndDataGroups`
- `TestExcelRibbonActionsUseBundledIconAssets`
- `TestSpreadsheetViewModesAndRuler`
- `TestHideMainWindowToTrayPreconditions`
- `TestWordFlowDoubleClickSelectsParagraphAcrossFormattedRuns`
- `TestTextEditorParagraphSelectionObservesCommandClick`
- `TestSelectingWordImageKeepsDocumentGeometryAndShowsAllResizeHandles`
- `TestWordCanvasAndHorizontalRulerMatchPenClpgmGeometry`
- `TestWordRightArrowMovesToNextStyledRun`
- `TestWordLeftArrowMovesToPreviousParagraphRun`
- `TestWordDocumentBoundaryShortcutsNavigateAndExtendSelection`
- `TestWordSelectAllShortcutSelectsTheWholeDocumentAfterPendingInput`
- `TestWordBackspaceAtStartAndDeleteAtEndJoinParagraphs`
- `TestWordQueuedRightArrowBeforeBackspaceDeletesInParagraph`
- `TestWordParagraphJoinFlushesFocusedDraftBeforeJoining`
- `TestWordParagraphJoinLeavesOrdinaryDeleteGesturesAlone`
- `TestWordHomeRibbonKeepsCommandGeometryAfterAgentEntry`
- `TestWordParagraphSplitCompletesAfterTabGenerationChanges`
- `TestWordTextClickKeepsSelectedRibbonTab`
- `TestJSONAndCSVCloseMemoryReclaim`
- `TestImportedXLSSaveAsRejectsXLSAndKeepsSource`
- `TestPrivateFontSceneExportKeepsLoadedResourceAfterRefresh`
- `TestBrokerListsOnlyDirectChildrenAndSkipsGit`
- `TestMCPCLIRecognizesServeAndDocumentsStdoutContract`
- `TestLoadManifestLoadsCuratedCorpus`
