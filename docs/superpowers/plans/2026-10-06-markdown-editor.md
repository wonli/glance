# Markdown Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the complete local Markdown editor represented by Pen `VvYgo`, including every action in `ybPPK`.

**Architecture:** A source-preserving Markdown adapter exposes neutral parse snapshots to Gio. Extend existing session, workspace, ribbon and PDF infrastructure. Background workers publish only results matching their source revision.

**Tech Stack:** Go 1.27, Gio 0.10.2, existing SQLite recovery and PDF/font infrastructure; a locally executing Markdown parser and formula renderer selected through fixture evaluation.

**Spec:** `docs/superpowers/specs/2026-10-06-markdown-editor-design.md`

## Global Constraints

- Source text is authoritative. Preview, outline, and PDF are derived views and never rewrite the source.
- Save through a temporary sibling file and replace only after successful completion.
- A failed save retains the draft and dirty state.
- Parse and image decoding run outside frame/layout callbacks, with cancellation or generation checks.
- No toolbar action may ship as a status-only placeholder.
- Rendering must remain local and must not require a network service or a system TeX installation.
- Reuse `ribbonAction`, `layoutToolbarActionItem`, `toolbarIconButton`, SVG tint/raster helpers, and persistent horizontal lists from Excel/JSON where suitable.

## Review Focus

- Empty files and BOM/CRLF documents preserve exact source on reopen and recovery (Task 1).
- Duplicate headings and heading syntax within code fences navigate to the correct source location (Task 2).
- Unicode selections and unfinished Chinese composition survive toolbar actions, save and mode switching (Tasks 3–4).
- An older parse, image or export job cannot replace newer content or overwrite another session's destination (Tasks 2–5).
- Narrow/high-DPI windows keep actions and mode controls reachable without clipping captions (Task 4).

---

### Task 1: Source adapter and document lifecycle

**Files:** Create `internal/format/markdowndoc/document.go`, `document_test.go`, `internal/desktopui/markdown_session_test.go`. Modify `internal/desktopui/app.go`, `document_session.go`, `workspace.go`, `workspace_layout.go`, `workspace_create_menu.go`, `home_layout.go`, `layout.go`, platform file pickers and the view-state definitions they use.

**Interfaces:** Produces `markdowndoc.NewBlank() *Document`, `Open(path string) (*Document, error)`, `OpenSnapshot(payload []byte) (*Document, error)` and methods `Text() string`, `SetText(string) error`, `Snapshot() ([]byte,error)`, `Save() error`, `SaveAs(string) error`, `Dirty() bool`, `SetDirty(bool)`, `SetSourcePath(string)`, `Close() error`. These match existing text-adapter lifecycle operations while preserving BOM and source bytes.

- [ ] Write `TestMarkdownSourceRoundTrip` asserting snapshot/reopened bytes equal empty, Chinese, BOM and CRLF fixture bytes; `TestMarkdownNoOpAndSaveFailure` asserting no-op stays clean and an invalid destination retains dirty text. Add `TestMarkdownWorkspaceRecovery` asserting format identity, empty draft, tab switch and recovered source.
- [ ] Run `go test ./internal/format/markdowndoc ./internal/desktopui -run Markdown`; expected missing adapter/integration failures.
- [ ] Implement the adapter and add a separate Markdown field to loaded/shared/view state; thread its lifecycle through open/install/close, save-as validation, persistence, folder imports, creation menus and pickers. Keep dirty ownership and serialized snapshots consistent with JSON's established lifecycle.
- [ ] Run the same command; expected PASS, then run `go test ./internal/workspace ./internal/desktopui` for existing lifecycle regressions.
- [ ] Commit `feat: add source-preserving Markdown sessions`.

### Task 2: Neutral parse, math and image snapshots

**Files:** Create `internal/document/markdown.go`, `internal/format/markdowndoc/parse.go`, `parse_test.go`, `math.go`, `math_test.go`, `internal/desktopui/markdown_state.go`, `markdown_async.go`, `markdown_async_test.go`. Update dependency evidence in `docs/office-library-evaluation.md`.

**Interfaces:** Produces `Parse(source string) (document.MarkdownSnapshot,error)` containing source, blocks, spans, headings with rune offsets, statistics and diagnostics. `RenderMath(expression string, display bool) (image.Image,error)` returns bounded raster output. `markdownViewState` owns mode, persistent editor/search/lists/buttons, retained selections and accepted/pending generation identifiers. `(*App).requestMarkdownParse()` dispatches source-tagged work; accepted snapshots remain immutable.

- [ ] Add `TestMarkdownOutline` asserting duplicate headings retain distinct offsets, nested levels are correct, and fenced `# text` is absent. Add parser fixtures for tables, tasks, emphasis, links, images and incomplete syntax; assert source remains byte-identical.
- [ ] Add `TestMarkdownMathCorpus` for Greek symbols, powers, subscripts, fractions, roots and operators; assert rendered extents and ink are nonempty, malformed/unsupported input returns diagnostics. Add `TestMarkdownStaleParseRejected` with old/new revisions and a switched/closed session.
- [ ] Run `go test ./internal/format/markdowndoc ./internal/desktopui -run Markdown`; expected failures for missing parsing/rendering behavior.
- [ ] Evaluate primary parser/renderer documentation and fixture results; record a dependency choice. Implement neutral conversion and local math rendering behind these interfaces, then implement background parsing/image decoding with bounded caches and generation rejection. Resolve local images relative to the source path; preserve destinations and alt text on errors.
- [ ] Run the same tests; expected PASS. Inspect formula corpus images to verify mathematical placement, not only raster existence.
- [ ] Commit `feat: parse Markdown previews and render local math`.

### Task 3: All editing commands and draft synchronization

**Files:** Create `internal/format/markdowndoc/edit.go`, `edit_test.go`, `internal/desktopui/markdown_commands.go`, `markdown_dialogs.go`, `markdown_commands_test.go`. Modify controller draft flush points and dialog/event dispatch files as needed.

**Interfaces:** Produces `ApplyEdit(source, action string, start, end int, args EditArgs) (EditResult,error)` using rune offsets; `EditResult` contains text and selection. `(*App).handleMarkdownAction(gtx layout.Context,id string)` dispatches commands. `(*App).flushMarkdownDraft()` synchronizes completed draft text to its owning session. `EditArgs` carries link/image label and destination or inline/block math choice.

- [ ] Add `TestMarkdownToolbarEdits` covering heading, bold, italic, quote, inline/fenced code, link, image, list, todo and inline/block formulas. Assert exact resulting source and selection for empty, selected Chinese and multiline inputs. Assert canceled dialogs make no changes.
- [ ] Add `TestMarkdownDraftFlush` covering switching modes/tabs, save/export, undo/redo and dirty/no-op behavior. Exercise active composition without committing intermediate text using the same event harness as existing editor regressions.
- [ ] Run `go test ./internal/format/markdowndoc ./internal/desktopui -run Markdown`; expected missing edit/controller failures.
- [ ] Implement editing transforms and selection retention, persistent dialog drafts, image selection and link destination entry; connect file actions to existing background lifecycle operations. Retain raw source for undo and recover it after restart.
- [ ] Run the same tests; expected PASS.
- [ ] Commit `feat: connect complete Markdown editing toolbar`.

### Task 4: Pen workspace and reusable ribbon

**Files:** Create `internal/desktopui/markdown_layout.go`, `markdown_ribbon.go`, `markdown_preview.go`, `markdown_outline.go`, `markdown_layout_test.go`, `markdown_ribbon_visual_test.go`, and any missing SVG assets under `assets/icons/markdown/`. Modify document view/event routing.

**Interfaces:** Consumes Tasks 1–3. Produces `(*App).layoutMarkdownDocument(gtx layout.Context) layout.Dimensions`, `layoutMarkdownRibbon`, `layoutMarkdownPreview`, `layoutMarkdownOutline`, and a handler for persistent widget events. The toolbar uses shared `ribbonAction` data and Excel button/group helpers.

- [ ] Add `TestMarkdownRibbonActions` asserting all 14 Pen actions map to real commands and have rasterizable icons. Add standard/narrow/high-DPI visual cases asserting 54 dp toolbar, 34 dp status, no caption clipping, reachable mode buttons and horizontal action scrolling.
- [ ] Add `TestMarkdownModesAndOutline` asserting source/selection survives edit/preview/split, filtered duplicate headings jump by offset, outline collapse restores content width, and preview reflects the current revision.
- [ ] Run `go test ./internal/desktopui -run Markdown`; expected missing UI behavior failures.
- [ ] Implement the 248 dp collapsible outline, centered white writing column, preview and split surfaces, status statistics/caret position, and fixed-height toolbar. Reuse existing tab chrome and shared icon/caption layout. Render semantic spans, lists, code, tables, images and math from neutral snapshots with bounded visible work.
- [ ] Run the same tests; expected PASS. Inspect rendered captures against `VvYgo` and correct alignment/overflow. Manually check Chinese composition, keyboard focus, toolbar insertion and mode switching in the native app.
- [ ] Commit `feat: add Pen Markdown workspace and ribbon`.

### Task 5: PDF export and completion verification

**Files:** Create `internal/format/pdf/markdown.go`, `markdown_test.go`, `internal/desktopui/markdown_pdf_export.go`, `markdown_pdf_export_test.go`. Update `docs/architecture.md`, `docs/office-support.md`, and `docs/office-library-evaluation.md`.

**Interfaces:** Consumes immutable neutral snapshots and local image/math output. Produces `ExportMarkdown(ctx context.Context, snapshot document.MarkdownSnapshot, path string) error` behind the PDF adapter with explicit resource access. The desktop export job retains owning session/revision and uses existing destination-picker/output-replacement patterns.

- [ ] Add `TestMarkdownPDFExport` covering Chinese paragraphs, headings, tables, images and supported math; assert text/page content and nonempty image placements. Add `TestMarkdownPDFExportFailure` asserting existing output and Markdown dirty/path state survive failure, cancellation and stale jobs.
- [ ] Run `go test ./internal/format/pdf ./internal/desktopui -run Markdown`; expected missing export failures.
- [ ] Implement pagination and render content using existing PDF fonts/helpers; freeze current draft revision before export, reject unsupported render content with diagnostics, and atomically replace output only on success.
- [ ] Run the same tests; expected PASS. Render a representative PDF and inspect Chinese glyphs, formulas, image bounds, tables and page breaks.
- [ ] Update support/evaluation documentation with measured support and remaining syntax limitations. Run `go test ./...`, `go vet ./internal/format/markdowndoc ./internal/desktopui ./internal/format/pdf`, and `git diff --check`; expected PASS (report actual platform/toolchain warnings separately).
- [ ] Commit `feat: export Markdown PDF and document verified support`.
- [ ] Review the entire branch against the spec and every Review Focus item; fix actionable defects with failing regressions, then rerun affected checks. Leave the isolated worktree and branch available for review.

## Execution evidence (2026-10-06)

All five feature tasks implemented in the isolated `codex/markdown-editor` worktree. The adapter, parser, edit and preview geometry regressions were observed failing before their corresponding fixes. PDF work was delegated within its isolated file ownership and independently checked with integrated tests and raster inspection. Independent review identified table resources, destination escaping, BOM handling, source-line coordinates, code-fence collision, resource type validation and per-frame scanning; fixes now have regression/evidence coverage. Final implementation is committed as one coherent feature change because the neutral snapshot, lifecycle and UI files have mutual compilation dependencies; the approved design has its own preceding commit.

Validation: `go test ./...`; `go vet ./internal/format/markdowndoc ./internal/desktopui ./internal/format/pdf`; `go test -race ./internal/format/markdowndoc ./internal/format/pdf ./internal/desktopui -run Markdown -count=1`; `git diff --check`. All pass. Real Gio headless captures at 1440/640 widths for all modes inspected; high-DPI/narrow semantic bounds and horizontal reachability pass. Representative PDF raster inspected for Chinese text, images, formula placement and tables. Gio router composition-event regression verifies intermediate Chinese text remains transient and final composition commits. Native operating-system IME candidate-panel interaction is not manually verified and is recorded in office-support.md.

Math and asynchronous parser tests live in the parser/controller fixture files rather than separate proposed test filenames. Runtime dependencies were selected through fixtures and primary-source review and recorded in office-library-evaluation.md. No merge or push is part of this execution; preserve the attached worktree for review.
