# Process-wide Font Manager Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make one process-owned font manager the source of font resources and snapshots for Glance UI and every text-bearing document format, without changing the current UI appearance.

**Architecture:** Move the lifecycle service, catalog, downloader, fallback policy, and bundled font bytes into a copyable `internal/fontmanager` subtree. Desktop and headless process roots own a manager and inject its immutable snapshots into UI shapers, document measurements, print, and export adapters; compatibility helpers may remain for explicit standalone utilities, but production paths must not use hidden package globals.

**Tech Stack:** Go 1.27.1, Gio 0.10.3, `github.com/go-text/typesetting` v0.3.5, existing fontcatalog scanner and PDF adapters.

**Spec:** `docs/superpowers/specs/2026-10-23-process-global-font-manager-design.md`

## Global Constraints

- Keep all edits and any builds or launches in `/Users/mima1234/project/glance-go`; do not create or switch worktrees.
- Keep current UI family order, typeface values, typography tokens, sizes, weights, line metrics, spacing, and control geometry.
- Keep the font manager subtree free of Glance application, document, workspace, and Gio imports; inject paths, locale, and providers from the host.
- Use one captured immutable manager snapshot for each layout, measurement, preview, print, and export task.
- `fontdownload` remains an independent implementation package nested under `fontmanager`; only the manager coordinates fetch, validation, publication, and registration.
- Do not add or run tests in this implementation pass. Use formatting, focused package compilation, diff checks, and a fresh-process UI smoke check where available.
- Preserve the current font cache and downloaded font data; do not clear existing user data or caches.

## Review Focus

- A refresh while a Word layout, Excel auto-fit, or PDF task is running must not replace that task's captured face resources.
- A first scan with no compatible cache must not let shell or document text use an unmanaged platform collection.
- A download must not become visible as a font until validation, durable publication, and manager registration complete.
- All existing face-family order and typography values must survive the source migration.
- Runtime shutdown must drain document work before closing the shared manager, and closing one window must not close it.

---

### Task 1: Create the reusable fontmanager subtree

**Files:**
- Move: `internal/application/fonts.go` → `internal/fontmanager/manager.go`
- Move: `internal/fontcatalog/*` → `internal/fontmanager/fontcatalog/*`
- Move: `internal/fontdownload/*` → `internal/fontmanager/fontdownload/*`
- Move: `internal/fontfallback/*` → `internal/fontmanager/fontfallback/*`
- Move: `internal/emojifont/assets/*` → `internal/fontmanager/builtin/*`
- Create: `internal/fontmanager/builtin/resources.go`
- Create: `internal/application/fonts.go` compatibility aliases for existing application callers.
- Modify: all Go imports that reference the moved packages.
- Create: `internal/fontmanager/README.md` with Go version, third-party dependencies, host configuration, and copy instructions.

**Interfaces:**
- `fontmanager.Manager` owns catalog lifecycle and downloader/provider coordination.
- `fontmanager.Options` accepts the scanner, cache path, private font root, and trusted provider.
- `fontmanager.Event` carries the immutable catalog snapshot and refresh/cache status.
- `Manager.Snapshot() fontcatalog.Snapshot`, `Manager.Ready(context.Context) (fontcatalog.Snapshot, error)`, `Manager.Subscribe() (<-chan Event, func())`, `Manager.Initialize(context.Context) error`, `Manager.Refresh(context.Context, bool) error`, `Manager.Register(context.Context, fontcatalog.PrivateResource) error`, `Manager.Prefer(fontcatalog.ResourceID) error`, `Manager.ListOnline(context.Context) ([]fontdownload.Entry, error)`, and `Manager.FetchAndRegister(context.Context, string, func(fontdownload.Progress)) error` form the host API.
- `fontmanager/builtin` exposes immutable bundled font bytes and stable resource identity without depending on Gio.
- `application.FontService`, `FontOptions`, and `FontEvent` remain source aliases during migration; production composition uses `fontmanager.Manager` directly.

- [x] Move the three existing font subpackages under `internal/fontmanager` and rewrite imports to sibling paths.
- [x] Move the existing font lifecycle implementation into `fontmanager.Manager`; keep cache, refresh ordering, registration, preference, event, and close behavior.
- [x] Add cancellable first-scan readiness; font-dependent startup and operations wait for filesystem verification even when a cache is present, then use the verified snapshot (or the last known good cached snapshot if verification fails).
- [x] Move bundled emoji bytes into `fontmanager/builtin`; change `internal/emojifont` to build Gio faces and bitmap output from manager-owned bytes.
- [x] Add manager-owned `ListOnline` and `FetchAndRegister`; the UI must not call downloader/provider implementations directly.
- [x] Add package README with copy requirements and verify no imports from `glance/internal/application`, `desktopui`, `document`, `workspace`, or Gio exist under `internal/fontmanager`.

### Task 2: Give each Glance process one manager

**Files:**
- Modify: `internal/desktopui/runtime.go`
- Modify: `internal/desktopui/font_service.go`
- Modify: `internal/desktopui/runtime_workspace_persistence.go`
- Modify: `internal/desktopui/font_picker_state.go`
- Modify: `internal/desktopui/font_picker_events.go`
- Modify: `internal/mcpserver/cli.go`
- Modify: `internal/application/runtime.go`

**Interfaces:**
- `application.RuntimeOptions` accepts a `*fontmanager.Manager`; a headless Runtime creates and owns one when no process root injects it.
- `desktopRuntime` owns one `*fontmanager.Manager`, shared with `application.Runtime` and every window.
- Standalone `mcp serve --stdio --open PATH` creates, initializes, injects, and closes its own manager.
- The already-running desktop MCP bridge continues to use its desktop Runtime and manager.

- [x] Replace `desktopRuntime.fonts`, `fontDownloads`, and `fontProvider` with the manager while preserving one instance across windows.
- [x] Initialize the manager outside Gio layout code, keep the existing cache/object paths, and route manager events to each attached window.
- [x] Change font picker listing/download actions to the manager API; preserve current row ordering, progress, cancellation, and error feedback.
- [x] Inject the manager snapshot source into `application.Runtime` and wire headless CLI ownership.
- [x] Keep shutdown ordered: stop MCP requests and document jobs, close Runtime, then close the process manager; window detach only unsubscribes.

### Task 3: Route all document formats through explicit snapshots

**Files:**
- Modify: `internal/wordfont/system.go`
- Modify: `internal/wordfont/catalog.go`
- Modify: `internal/format/wordpdf/export.go`
- Modify: `internal/format/markdownpdf/export.go`
- Modify: `internal/format/sheetpdf/export.go`
- Modify: `internal/format/presentationpdf/export.go`
- Modify: `internal/application/word_pdf_export.go`
- Modify: `internal/application/markdown_pdf_export.go`
- Modify: `internal/application/spreadsheet_pdf_export.go`
- Modify: `internal/application/presentation_pdf_export.go`
- Modify: `internal/application/word_print.go`
- Modify: `internal/application/spreadsheet_print.go`
- Modify: `internal/application/presentation_print.go`
- Modify: text-measurement and layout entry points in `internal/desktopui/word_pagination.go`, `word_pdf_export.go`, `markdown_pdf_export.go`, `grid_auto_fit.go`, and presentation render/export files.
- Modify: `internal/desktopui/presentation_pdf_export.go`, `presentation_rich_text.go`, `presentation_text_layout.go`, `spreadsheet_print.go`, and `word_native_print.go` where those paths create or retain document text shaping resources.

**Interfaces:**
- Font-sensitive application operations capture their manager snapshot before work begins and pass it through to format adapters.
- Add snapshot-aware export entry points alongside any explicitly documented standalone compatibility helpers; production must call only snapshot-aware entry points.
- `wordfont.NewWithSnapshot(fontcatalog.Snapshot)` remains the shaper constructor used by production Word, Excel, Markdown, presentation, PDF, and print work.

- [x] Replace production `fontcatalog.Default()` reads with the owning runtime's manager snapshot, including Word layout version keys and Excel auto-fit.
- [x] Make Word pagination, PDF preview/export, and print share the same captured snapshot and task-owned shaper.
- [x] Make Markdown PDF use that same snapshot and preserve its existing Markdown resource behavior.
- [x] Pass snapshots into spreadsheet and presentation PDF/print exporters instead of constructing their own system shapers.
- [x] Keep JSON, CSV, and other editor-based formats on the shared manager-backed UI shaper, with no format-local system font source.
- [x] Retain only explicit standalone/system helpers for isolated utilities; remove package-global catalog access from production paths.

### Task 4: Make all Glance UI text use the process manager

**Files:**
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/text_shaper.go`
- Modify: `internal/desktopui/font_rendering.go`
- Modify: `internal/desktopui/ui_font_collection.go`
- Modify: `internal/desktopui/font_collection_builder.go`
- Modify: `internal/desktopui/font_picker_events.go`
- Modify: `internal/desktopui/font_service.go`
- Modify: `internal/desktopui/markdown_preview.go`, `markdown_layout.go`, and `markdown_pdf_glyphs.go` where Markdown text/emoji are shaped or cached.
- Modify: `internal/desktopui/proofread_text_layout.go` and `proofread_controller.go` for background proofreading layout.
- Modify: `internal/diagramrender/page.go` and `internal/fontmanager/fontfallback/families.go` to source the existing WebView diagram family order from the manager subtree.
- Modify: `internal/desktopui/grid_paint.go` only if required to resolve the existing unchanged family chain from manager-provided faces.

**Interfaces:**
- `material.Theme.Face` remains the current `fontfallback.PreferredTypeface()` value; only its resolver's collection source changes.
- Each attached window receives the current manager snapshot event and builds its Gio shaper on a managed worker.
- All shell labels, controls, menus, dialogs, editors, canvas, spreadsheet cells, JSON/CSV text, Markdown text, and emoji use manager-selected resources. Browser-backed diagram rendering takes its existing family declaration from the manager fallback policy.

- [x] Build the initial and refreshed Gio text collections from the manager snapshot while keeping current primary family order, script fallback order, emoji behavior, and typography tokens.
- [x] Replace the initial unrestricted `newTextShaper(nil)` source; keep collection parsing outside layout/frame code and publish completed shapers through synchronized state.
- [x] Resolve Markdown text and the existing WebView diagram family declaration from manager-owned fallback policy without changing CSS typography values or layout rules.
- [x] Preserve all role-specific family/size/weight choices and existing document family names; font refresh invalidates caches only when the rendering version changes.
- [x] Ensure no production UI text path silently creates an unmanaged platform shaper when the first scan is unavailable.

### Task 5: Remove stale global paths and record the boundary

**Files:**
- Modify: all production references to `fontcatalog.Default` and `fontcatalog.SetDefault`.
- Modify: `docs/architecture.md`
- Modify: `docs/office-support.md` only if the font ownership change affects a documented format path.
- Modify: font package documentation and the design spec as implementation details settle.

- [x] Remove production calls to `fontcatalog.Default` and `fontcatalog.SetDefault`; classify any remaining legacy system constructor as standalone-only and ensure process and isolated runtimes cannot overwrite one another's snapshots.
- [x] Search all production source for `wordfont.NewSystem`, direct downloader/provider use, Gio shapers without explicit manager collections, and system-font CSS fallbacks; classify any retained helper as standalone-only.
- [x] Update architecture docs to show process composition → fontmanager → immutable snapshot → all UI/document adapters.
- [x] Format changed Go files and inspect the complete diff for accidental UI token, geometry, or document-style changes.
- [x] Run focused package builds in the authorized workspace and `git diff --check`; build success is recorded separately from visual verification.
- [ ] Launch a fresh Glance process and inspect actual UI rendering; not performed in this implementation pass.

