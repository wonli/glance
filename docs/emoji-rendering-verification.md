# Emoji rendering verification (2026-10-23)

`complex_markdown.md` in the supplied Downloads directory reproduced missing-glyph boxes in Markdown headings. Gio v0.10.3 already draws bitmap glyphs through `Shaper.Bitmaps`; missing renderer support was not the cause.

Gio's `shaperImpl.ResolveFace` delegates to fontscan, but does not implement its script forwarding interface. With the application's previous family list and a semibold query, fontscan cannot find an aspect-matching emoji face, searches the unset script and returns an arbitrary face. On this machine that was Arial Unicode, glyph 0. A probe with script `Common` found Apple Color Emoji bitmap glyphs; explicitly requesting Apple Color Emoji also worked without a script.

The shared `fontfallback.PreferredTypeface` now explicitly includes Apple Color Emoji, Segoe UI Emoji, Noto Color Emoji and the generic emoji family after the existing text families. This keeps existing text preference and allows the resolver to select an available emoji face. No font files are parsed or loaded in layout code by this change.

`TestMarkdownEmojiVisual` checks actual colored pixels separately for 🚀, 📑, 🎨, 📊, 🧮, 📈, 🎮, 🔖, ✨ and ✅, in normal, heading and code text at 1x and 2x. The original aggregate test failed before the change with zero colored pixels at both scales, and passed after the change. The per-glyph threshold allows mostly-gray glyphs such as 📑. A Metal device is required; a skipped headless test is not visual verification. `GLANCE_EMOJI_CAPTURE` may point to an existing directory for individual PNGs.

The existing `TestMarkdownAcceptanceVisualSnapshot` was also run against the supplied file. Its screenshot shows emoji in the source editor, outline and preview. The remote Mermaid SVG remains unsupported by the PNG/JPEG/GIF image loader; that resource warning is independent of emoji rendering.

The Gio fix covers editor and preview text. Markdown PDF uses a separate outline-font pipeline, so bitmap emoji could still fail with `no system font is available to shape Word text` (and a system collection could report a glyph that its PDF-extracted face did not map). Markdown PDF now shapes supported emoji grapheme clusters with the bundled Noto Color Emoji font and embeds the resulting PNG as an inline image; the common text checkmark `✓` uses the bundled `✔` glyph. This keeps emoji visible without requiring an installed emoji font. `TestMarkdownPDFExportUsesBundledBitmapFallbacks` verifies serialized output and rendered color pixels for single emoji and the tested skin-tone, family, flag, keycap and variation-selector sequences. These emoji are images, so they are not searchable PDF text. This is specific to Markdown PDF; Word PDF still uses system vector-font coverage, and other unsupported glyphs are not silently dropped.

## Final checks

All 60 per-glyph GPU cases passed (10 glyphs × 3 text styles × 2 scales). The supplied-file acceptance screenshot passed. `git diff --check` passed.

`go test ./...` failed in `desktopui`, `format/wordpdf`, `localaccess`, `mcpserver` and `officeqa`. A baseline run temporarily removing only this font change reproduced `TestAgentRibbonTextFormsCenteredGroup`, `TestMarkdownPageFocusKeepsTextDragSelection`, `TestSettingsFieldsVerticallyCenterText` and `TestPenRibbonBrandAndActiveStateVisual`; those failures are present without the fix. The other failures have not all been independently baseline-checked. Full-run failed tests:

```text
TestAgentHistorySearchAndDateGroups
TestAgentRibbonTextFormsCenteredGroup
TestParseCellRangeAddress
TestGridResizeBoundaryDoubleClickAutoFitsColumn
TestCellSelectionBoundsClampsToXLSXLimits
TestMarkdownPageFocusKeepsTextDragSelection
TestDesktopMCPStdioEditsTheLiveGioSession
TestPenRibbonEveryReferenceGlyphHasVisiblePixels
TestPenRibbonBrandAndActiveStateVisual
TestSettingsFieldBlankEdgesFocusEditor
TestSettingsSavedKeyEdgeClickFocusesVisiblePreview
TestSettingsAPIKeyEyeClickTogglesVisibility
TestSettingsEyeRevealsSavedKeyWithoutCreatingDraft
TestSettingsFieldsVerticallyCenterText
TestSpreadsheetStatusBarShowsEndModeIndicator
TestSpreadsheetZoomInteractionsAndClamping
TestSpreadsheetRecalculateAndRefreshAll
TestSpreadsheetToolbar_SpellCheckAndTranslate
TestSpreadsheetToolbarActionsCoverHomeAndDataGroups
TestExcelRibbonActionsUseBundledIconAssets
TestSpreadsheetViewModesAndRuler
TestHideMainWindowToTrayPreconditions
TestWordFlowDoubleClickSelectsParagraphAcrossFormattedRuns
TestTextEditorParagraphSelectionObservesCommandClick
TestSelectingWordImageKeepsDocumentGeometryAndShowsAllResizeHandles
TestWordCanvasAndHorizontalRulerMatchPenClpgmGeometry
TestWordRightArrowMovesToNextStyledRun
TestWordLeftArrowMovesToPreviousParagraphRun
TestWordDocumentBoundaryShortcutsNavigateAndExtendSelection
TestWordSelectAllShortcutSelectsTheWholeDocumentAfterPendingInput
TestWordBackspaceAtStartAndDeleteAtEndJoinParagraphs
TestWordQueuedRightArrowBeforeBackspaceDeletesInParagraph
TestWordParagraphJoinFlushesFocusedDraftBeforeJoining
TestWordParagraphJoinLeavesOrdinaryDeleteGesturesAlone
TestWordHomeRibbonKeepsCommandGeometryAfterAgentEntry
TestWordParagraphSplitCompletesAfterTabGenerationChanges
TestWordTextClickKeepsSelectedRibbonTab
TestJSONAndCSVCloseMemoryReclaim
TestImportedXLSSaveAsRejectsXLSAndKeepsSource
TestPrivateFontSceneExportKeepsLoadedResourceAfterRefresh
TestBrokerListsOnlyDirectChildrenAndSkipsGit
TestMCPCLIRecognizesServeAndDocumentsStdoutContract
TestLoadManifestLoadsCuratedCorpus
```
