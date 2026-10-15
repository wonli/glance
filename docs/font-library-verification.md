# Shared font library verification — 2026-10-15

## Delivered behavior

Word and Excel use one 388dp compact picker. Document fonts are first; local families support English/Chinese alias search and inline previews. Missing document families remain visible without rewriting them. No online category, label or empty placeholder is rendered when the provider is empty. The footer exposes cached state, refresh progress and retry. Search text survives refresh; the search field's upper/lower padding focuses its stable editor. Popup blank space blocks underlying document actions, while outside clicks dismiss and pass through.

The runtime shares one background catalog service. A compatible disk index is shown before verification; forced refresh rehashes even same-size/same-mtime replacements. Successful check time persists without bumping the rendering version on unchanged scans. Source preferences distinguish conflicting local/private families. Downloaded resources and license declarations survive index eviction; staged/uncommitted files are not discovered.

Font versions invalidate Word pagination and Gio text caches. Word/Excel formatting uses existing commands and stale-selection guards. Download completion registers a resource without applying a font. Each export owns its shaper and loaded bytes. A changed/deleted source before first load reports an error; a loaded resource remains usable for a prepared scene after source removal and a new catalog. Refresh does not cancel active PDF work or discard prepared native print bytes; actual document changes still invalidate printing.

## Automated evidence

| Command / scope | Observed result |
| --- | --- |
| `go test -race ./internal/fontcatalog ./internal/fontdownload ./internal/wordfont ./internal/application -count=1` | All four packages pass. |
| `go test -race ./internal/fontdownload -count=1 -v` after final download tests | Pass, including concurrent deduplication, mid-request cancellation/retry, cross-origin redirect rejection, cache reuse identity checks, digest/size/family checks and empty provider. |
| `go test ./internal/desktopui -run 'TestFontPicker\|TestFontRendering\|TestFontRefresh\|TestWordPreparedPrint\|TestWordPrint\|TestWordFontFamily\|TestSpreadsheetFontFamily\|TestOfficeRibbonDropdownRecordingBoundary' -count=1 -v` | Pass. Includes full search hit area, popup blank-area interception, stale target, source conflicts, mixed-range formatting and prepared print after refresh. |
| `go test ./internal/format/docx ./internal/format/xlsx ./internal/wordlayout ./internal/format/sheetpdf ./internal/format/markdownpdf ./internal/format/presentationpdf -count=1` | DOCX/XLSX/Word layout pass. The three PDF adapter packages compile but have no package tests; this is not end-to-end PDF evidence for them. |
| `go test ./internal/format/wordpdf -run 'TestPrivateFont\|TestExportSnapshotCommitGuard' -count=1 -v` | Pass. Real private Go font shapes a scene and produces a validated PDF after the source is removed and catalog replaced; stale-output guard also passes. |
| `go test ./internal/desktopui -run 'TestFontPickerVisual\|TestPenRibbonBrandAndActiveStateVisual' -count=1 -v` | GPU 1x/2x pass, no SKIP. Picker width/viewport/footer ink and existing Ribbon brand/active state pixels checked. |
| `go build -o /tmp/GlanceFontQA.app/Contents/MacOS/Glance ./cmd/glance` | Pass; existing duplicate `-lobjc` linker warning. |
| `git diff --check` | Pass. |

`TestFontRenderingUsesOneConsistentSourcePerDescriptor` checks both descriptor uniqueness and actual `i` glyph advance: preferring a private monospace face changes measured glyph metrics. `TestSetTextRunFontFamilyRoundTripsAndPreservesOtherRunProperties` verifies DOCX save/reopen and untouched package parts. XLSX's existing round-trip formatting case now changes a synthetic font family while preserving formula, other styles, row height and merges. The offscreen font summary test uses row 2000.

Synthetic GPU screenshots are generated with `GLANCE_FONT_SCREENSHOT=<output-directory>` by `TestFontPickerVisual`; they include a synthetic online entry to exercise that row, not a live public service.

## Full-suite comparison

Full-suite execution and an implementation-before baseline at commit `5701764` were both run. The baseline already fails the following 14 top-level tests:

- `TestSettingsFieldBlankEdgesFocusEditor`
- `TestSettingsSavedKeyEdgeClickFocusesVisiblePreview`
- `TestSettingsAPIKeyEyeClickTogglesVisibility`
- `TestSettingsFieldsVerticallyCenterText`
- `TestSpreadsheetRecalculateAndRefreshAll`
- `TestSpreadsheetToolbar_SpellCheckAndTranslate`
- `TestSpreadsheetViewModesAndRuler`
- `TestHideMainWindowToTrayPreconditions`
- `TestWordUpDownKeepsPreferredVisualColumnAcrossVisualLines`
- `TestWordToolbar_WatermarkTrackRevisionsEditorAndLayout`
- `TestWordToolbar_RemainingActionsBatch`
- `TestExportSnapshotUsesSameParagraphGeometryAsDesktopPages` (fixture is two canonical pages, expected one)
- `TestMCPCLIRecognizesServeAndDocumentsStdoutContract` (`--instance-id` help contract)
- `TestLoadManifestLoadsCuratedCorpus` (unsupported `rendered-with-warning` expected outcome)

Initial full runs also exposed two pagination fixtures with zero font version after another test published a live catalog; the fixtures now capture the current version. They pass independently. A QA process and later its stale Unix socket interfered with `TestDesktopMCPStdioEditsTheLiveGioSession`; the QA process was stopped and the exact socket removed only after confirming no listener. Full regression is also run with `-p 1` to avoid package-level sharing of that endpoint. The final `go test -p 1 ./...` run reports exactly the same 14 top-level failures as that baseline. The two pagination fixture failures and the MCP endpoint failure are absent. Full-suite results remain explicitly non-green; unrelated failures are not changed by the font feature.

## Native window evidence

A fresh macOS app was built and launched against temporary copies of repository fixtures, rather than editing user documents. Old QA processes were exited between builds.

- Word: opened the compact picker at the font Ribbon anchor, searched `宋`, observed `Songti SC`/`Songti TC`, clicked refresh and observed `刷新中…` with the query/results preserved. No empty online section appeared.
- Excel: opened the same picker from the Home font field, observed the document's missing `Calibri` annotation, searched `Menlo`, applied it to A1 and observed the actual monospace glyph change. This exposed the Ribbon's hardcoded font label, which was replaced with the selected run/cell family. A subsequent fresh build displayed `Menlo` in the Excel Ribbon alongside the actual monospace cell text.
- Native screenshots are Retina; synthetic GPU tests separately exercise 1x and 2x. User workspace tabs in the native window were not edited or published as evidence artifacts.

## Limits

The production online manifest is intentionally empty; no public-service download is claimed. Static single-face TrueType download validation is covered by local HTTPS tests. Protected OS resources, all variable/color font formats, every collection face and every PDF embedding permission are not certified.

Chinese committed text and native alias search were verified. Real IME candidate composition, a physical printer, and native Windows/Linux interaction were not manually tested. Tab or Alt+arrow transfers search focus to the list; ordinary search arrows/Return remain available to the IME. The list uses arrows/Enter/Escape, and Escape restores the Ribbon anchor focus.

Excel summaries cover style font declarations, including offscreen styles, and may include unused declarations; full rich-text font inventory remains outside this implementation. Glance's local source preference is cached and is not a portable Office font binding. DOCX/XLSX do not embed downloaded fonts. See [office-support.md](office-support.md).

## Independent review

A read-only code review identified and drove fixes for export cancellation, uncommitted staging discovery, same-name source divergence, mixed-range no-op formatting, deleted private-object resurrection, late subscriber rollback of the global catalog, FontData identity bypass, newly created platform font roots, per-frame catalog work and provider cancellation. Regression evidence is in the catalog/service/picker/rendering tests. The review did not claim native IME or general Office layout conformance.
