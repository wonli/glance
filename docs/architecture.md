# Glance-Go Desktop Architecture

## Goal

Build a local document workspace for people and Agents. Gio provides the desktop UI, and MCP provides a protocol entry point; both attach to the same application runtime and live document sessions. Keep format libraries replaceable and keep their types out of UI state.

Markdown uses `internal/format/markdowndoc` as an exact UTF-8 source adapter. Its neutral `document.MarkdownSnapshot` carries headings with rune offsets, semantic spans and table cells. Gio owns stable editors, outline/preview lists and ribbon buttons; source transforms run through the session mutation path. Completed drafts are synchronized before saving or leaving a tab; active IME composition stays transient. Parsing, bounded local-image decoding, formula rasterization and PDF pagination run in background workers. Preview results match source and path; PDF replacement additionally checks the owning session revision and draft epoch under the adapter lock. PDF export reuses the Word layout/font pipeline without exposing Markdown parser types to widgets. `internal/format/markdownpdf` turns exact Markdown snapshots and bounded resources into a neutral paginated scene, then shares `internal/format/wordpdf` PDF validation and exclusive new-file creation with the MCP workflow; the final commit checks the live revision and rejects transient editor drafts. The view caches text and source-line offsets by Gio text version, so unchanged frames avoid copying the whole source.

This architecture borrows the format-adapter, normalized-document, session, and compatibility boundaries used by the sibling `tools` project, and applies Gio interaction lessons from `mysql-ai`. Reuse the boundaries and proven interaction patterns; do not copy their Rust/Tauri/Svelte or database-specific implementation.

## Layers and ownership

```text
Gio UI / MCP client / built-in Agent
        ↓ view intents or registered capabilities
Application Runtime and live sessions (`internal/application`)
        ↓ format-neutral requests
Document adapters (`internal/format/*`)
        ↓ library-specific operations
Office libraries and local filesystem
```

### Desktop UI (`internal/desktopui`)

- Own the Gio window, theme, persistent `widget.Editor`/`widget.List`/`widget.Clickable` state, selection, scroll position, and transient drafts.
- Draw from a stable session snapshot and turn user events into controller calls.
- Do not hold Excelize, DOCX, OOXML, ZIP, or XML library objects in Gio widgets.
- Keep rendering/layout helpers separate from event handling and file/session logic. Avoid growing one all-purpose `App` file; use focused view, interaction, and widget-state files as the UI grows.
- `desktopRuntime` owns one process-wide `fontmanager.Manager` and shares it with all windows and `application.Runtime`. Window startup waits for the manager's first filesystem verification before drawing text. Gio shapers are built from manager snapshots plus the bundled emoji fallback exposed by `fontmanager/emojifont`; the normalized emoji bytes and family identity belong to `fontmanager/builtin`. Platform font discovery is disabled in the renderer; font collection parsing stays on managed workers.
- Shell, settings, editors, Word canvas, spreadsheet cells, presentation canvas, Markdown/JSON/CSV views, and proofreading layout use collections built from that manager catalog. The existing family order and typography settings remain UI-owned and unchanged.

### Reusable Gio controls (`ui`)

- `ui` is a public package in this Go module for reusable controls. It may depend on Gio and the standard library, but not on `internal/desktopui`, application/session code, document models, localization, or file-format packages.
- Applications own widget lifetimes, translated labels, theme colors, icon resources, and business actions. Controls receive stable Gio widget pointers and explicit styles; vertical visual offsets are resolved dp values where positive moves rendered content down.
- Keep app-specific brand art and unusual composition in the application layer. Extract a control only when its geometry and interaction behavior can be described without importing app state.

### Desktop UI localization (`internal/i18n`, `internal/systemlocale`, `internal/desktopui`)

- The desktop ships embedded `en-US` and `zh-CN` JSON catalogs. UI messages use stable IDs and named arguments; the complete English catalog is the fallback for missing or unsupported text.
- The stored `ui.language` selection is `system`, `zh-CN`, or `en-US`, persisted through the existing workspace preferences table without a schema migration. System language detection reads OS preferences and resolves Chinese to `zh-CN`, English to `en-US`, and unsupported, invalid, or unreadable values to `en-US`.
- The desktop runtime owns the selected and resolved locale and shares its catalog across windows. Read preferences and platform language outside Gio layout. A language change replaces the in-memory catalog, preserves modal IDs and drafts, document sessions and widget state, then invalidates open windows so their message references render in the selected language.
- Localize app-owned labels, dialogs, status and error summaries, and app-generated metadata formatting. Keep document contents, formulas, paths, file names, MCP identifiers/schemas, and model-generated conversation text as source data. Office cell and document value formatting remains owned by format adapters.
- Basic Settings follows Pen node `l7WA9J` and exposes only the supported interface-language choice. Do not show unsupported settings as disabled controls.

### Application/session layer (`internal/application`, `internal/workspace`, `internal/desktopui`)

- `internal/application.Runtime` owns live session identity, path, format, revision, capability registration, command execution, events, Agent workflows, and save/close lifecycle. A session adapter is the source of truth for committed document data.
- Register capabilities once with stable IDs, usage descriptions, JSON input/output schemas, format requirements, and risk. MCP tools are generated from this catalog; the protocol server does not implement document behavior itself.
- Host side effects use narrow injected interfaces. The native print service accepts PDF bytes captured from the requested live document revision; headless runtimes without that service report printing as unsupported. Agent print requests use the existing high-risk human confirmation flow.
- Gio attaches its existing sessions to the Runtime and sends registered capabilities or view intents. Widget editors remain transient drafts; adapters reject snapshots or edits while a relevant draft is pending.
- Font-sensitive print, measurement, preview, and PDF operations capture one immutable manager snapshot before work begins and pass it to the format adapter. Word pagination and desktop Word printing use that same snapshot version. Markdown PDF, spreadsheet PDF/auto-fit, and presentation PDF use snapshot-aware entry points; JSON and CSV text use the shared UI shaper.
- `internal/mcpserver` exposes the same catalog over MCP. Desktop clients launch `glance mcp serve --stdio`; that stdio bridge forwards MCP bytes over one stable, private per-user Unix socket (or Windows named pipe) to the single desktop Runtime and its unsaved sessions. A per-user OS lock keeps one interactive Glance process; later launches only activate its primary window and do not forward paths. Headless MCP children and multiple Gio windows inside that process are unaffected. The independent headless workflow is explicit: `glance mcp serve --stdio --open PATH`.
- Perform file/library work away from the Gio frame layout path. Report completion through synchronized state and request a redraw using the established Gio event-loop mechanism.
- Start App-owned background work through `launchManagedBackground`; window destruction closes the App shutdown signal and waits for those tasks before releasing session state. Runtime-owned workers such as the workspace snapshot writer keep their own explicit stop and completion boundary. Avoid adding untracked goroutines to App methods.
- Validate edits and capabilities here and in the adapter. A UI control being disabled is not validation.
- `internal/workspace` owns versioned SQLite metadata, recents, ordered tabs, active-tab ID, byte snapshots, dirty flags, and opaque per-tab view state. Format snapshots remain owned by the adapters; Gio widget instances stay in each in-memory document session.
- User personalization lives in the workspace SQLite `preferences` key/value table (schema version 6). `ai.sidebar.width_dp` stores the AI sidebar width in logical dp; startup restores valid values, while the viewport clamps its displayed width. Resize previews remain transient. Release queues a coalesced preference write through the runtime workspace worker; shutdown drains pending writes before closing SQLite.
- The workspace tab strip sits flush at the top edge of the full-height client area: recent/document tabs and the `+` tab share one horizontal list. File, folder, create, save, and save-as actions live in the `+` menu; the document canvas receives the space previously used by the path toolbar. On platforms where Gio reports client-side decorations, the tab strip occupies the transparent titlebar area; custom window controls and a dedicated drag strip preserve window actions. If the platform retains native decorations (currently X11), keep the system titlebar and place tabs directly below it rather than disabling move/resize behavior.
- The Recent tab is permanent. Closing a dirty document tab is blocked until it is saved; window-close confirmation is unavailable because Gio reports destruction after the native window has closed. Persist committed edits and drafts to the recovery store.
- File dialogs, document package opens/saves, and adapter serialization must not run in Gio layout callbacks. Do not serialize an entire workbook merely because the user selected a cell or scrolled; persist view state separately.

### Proofreading A/X workspace (`internal/proofreading`, `internal/application`, `internal/desktopui`)

- `internal/proofreading` owns stable character identities, ordering, unique page ownership, page states, and undo/redo. `internal/application` owns format-neutral editing, layout mapping, grapheme- and line-based ownership adjustment, and the versioned copy model. The desktop UI only projects A's visual page geometry and edits the independent B copy on X.
- Do not extract A's text or run OCR for ordinary proofreading. A supplies the visible reference and page geometry; the internal text layout engine shapes B into the X page boxes.
- A-area scrolling moves A and X together. The source reading line, one third down the viewport, selects the adjustment target. The 54dp text strip (Pen `K8IiR0`) collects/returns one extended grapheme cluster per accumulated 16dp of wheel travel, without acceleration; Shift uses shaped lines and waits for revision-matched geometry. Target changes clear queued input and fractional travel. Inside an X paper page, its scrollbar and wheel only browse that page. X exterior scrolling mirrors A/X and never changes ownership. No manual lock action is required.
- B remains immutable. Initial layout estimates page ownership once; subsequent editing, scrolling, and style changes must not repartition the book. The adjustment strip moves the boundary between the current page and its immediate next page only: collect the next page's prefix or return the current page's suffix, including originally preallocated characters. Never skip an empty next page or move a third page's characters. Reviewed current/next pages block transfer; newly typed suffixes remain local. Reviewed-page insertions/deletions affect only that page and never restore deleted source characters through allocation.
- `Manual` persists a chosen boundary but is not a barrier to adjacent adjustment. Legacy `Locked` remains for saved-copy compatibility; its menu action is removed. Each surviving character has exactly one owner, including whitespace and line breaks. Each wheel threshold step has its own shared undo transaction; publication, shaping requests, and saving are coalesced per frame.
- Boundary changes publish and request geometry only for the affected pair. Other edits and style changes refresh owned-page geometry without changing ownership. Cache validity includes the current page and the next-page preview used for line-based adjustment; visible stale/missing pages request background geometry. Offscreen editor widgets remain uninitialized until needed.
- Keep native editors bounded to the visible text window so long locked pages do not submit the full document to Gio/Metal. The model and saved copy still retain the full character sequence; background shaping and persistence publish only revision-matched results.

### Neutral document model (`internal/document` or `internal/docmodel`)

- Define product-facing DTOs for workbook sheets/cells/ranges and Word blocks such as paragraphs, text runs, tables, and image references.
- Represent unsupported content explicitly when it affects preview or save safety; do not silently imply complete Office support.
- Define format capabilities (read, edit constants/text, save) independently from library names.
- Keep memory and rendering bounded: use visible ranges/pages, avoid duplicating whole files as formatted strings, and provide cancellation for long operations where practical.
- JSON nodes retain source slices and parent/child/sibling links; paths and child slices are derived on demand. Sources larger than 512 KiB receive an allocation-free lexical count before parsing to allocate the node array exactly. This reduces per-node storage and excess capacity, but the tree still holds every node; it is not lazy parsing. Source previews shape only visible segments of at most 1 KiB.
- JSON labels and source segments are detached before entering Gio's text-layout cache so a cached substring cannot retain a whole file. Closing a view cancels worker publication through a synchronized signal; its owning UI loop drops the tree and editors. Large-file teardown coalesces background GC/OS-memory reclamation after the closing frame, with another request when an outstanding worker finishes. Workspace tab removal clears the unused pointer slot in its backing array so closed recovery snapshots are collectible.

### Format adapters (`internal/format/xlsx`, `internal/format/docx`)

- Implement a small shared open/read/edit/save contract, with typed format-specific operations behind it where necessary.
- Translate third-party library values to/from neutral DTOs at this boundary.
- Preserve source path/session context only where required to safely save. Use temporary sibling output and replace the original only after a successful write.
- Keep formulas as formula text plus cached result when available. Do not claim recalculation unless a calculation engine is separately integrated and verified.
- DOCX text mutation must preserve unrelated package parts and formatting in the supported corpus. If a library cannot do that reliably, limit the first release to read-only DOCX or narrowly scoped text replacement.
- XLSX and DOCX snapshots are self-contained Office package bytes; JSON snapshots preserve current editor text, including invalid intermediate JSON. Snapshot payloads are not interchange DTOs and must only be interpreted by their matching adapter.
- Shared Word pagination and system-font shaping live outside Gio in `internal/wordlayout` and `internal/wordfont`. `internal/format/wordpdf` renders the neutral page scene for desktop PDF output and the `word.document.export_pdf` MCP workflow. It validates the completed PDF, creates only a new output path, returns layout diagnostics, and checks the live revision and pending drafts before committing the file.

### Platform and file access

- Isolate open/save dialogs, clipboard, and OS-specific behavior behind small platform helpers, following the build-tag-specific file-picker pattern in `mysql-ai` where needed.
- Keep local-file processing local. Do not introduce a server path for document contents.

## Gio interaction guidance

- Gio is immediate mode for layout, but interactive widget state must persist across frames. Keep each editor/list/clickable in stable application or view state; do not construct a fresh editor during every layout pass.
- Layout functions should describe the current view and register input areas. Process clicks, keys, editor changes, and async completion as events, update state, then request another frame.
- Initialize every `widget.List` with its intended axis, including lists stored in newly created dialog state.
- Keep selection and editing distinct. For spreadsheet editing retain the original cell value, draft text, and edit mode; commit once, make no-op focus/blur a no-op, and cancel without mutating the workbook.
- Treat IME composition as an editing concern: do not commit intermediate composition text. Test Chinese input, Enter/Escape/Tab behavior, and focus transfer on actual supported platforms.
- Virtualize spreadsheet rows/cells and Word pages/blocks. Register hit targets only for visible content; use stable row/column or block identities rather than one widget per cell in the whole workbook.
- Keep bounded spreadsheet window reads, document opens, and saves off the Gio frame loop. Return results to the UI loop through synchronized state and invalidate the window after applying each result. Reject stale read results when the active document or sheet changes.
- Resolve pointer coordinates into one documented local/content coordinate system for selection and drag operations. If Gio reports nested-target coordinates, normalize them at the input boundary instead of mixing spaces in layout math.
- For Excel row-height and column-width handles, follow the stable resize pattern in `../mysql-ai/docs/drag-patterns.md`: resolve the pointer in grid-content coordinates, keep the preview line under the pointer, maintain the resize cursor throughout the active drag, and persist the new dimension only on release. Expand the temporary preview viewport when dragging past the current grid edge. Convert Gio screen pixels to logical grid units and then to the file format's row/column dimension units in one named adapter/helper; do not store raw pixels as workbook dimensions.
- Keep row and column resize preview state separate from committed workbook dimensions. Rebase the drag anchor if the input coordinate space changes during the gesture so the preview does not jump. Cover dragging both narrower and wider, the first and last visible dimension, scrolling/edge cases, and release/cancel behavior with geometry tests plus platform interaction checks.
- Text editors in vertical list rows should occupy available width. Clear inherited minimum constraints for compact chips, icons, and badges. Flush editor contents to a dialog draft before closing or saving.
- Keep files focused (mysql-ai uses a 500-line practical source-file guardrail); split view, event, and state code by responsibility as features grow.

## Package And Source-File Boundaries

- Keep Go package identifiers lowercase and aligned with their directory name. Use established format abbreviations such as `docx`, `xlsx`, and `pptx`; keep distinct responsibilities separate (`xls` versus `xlsx`, `pdfdoc` versus `pdf`) rather than renaming for cosmetic uniformity.
- Keep each Go source file focused on one cohesive functionality or responsibility. Aim for about 500 lines including imports and comments; split by ownership, not into one file per function. Apply the same rule to test files where test groups have clear feature ownership.
- Avoid package renames unless the existing name is misleading or blocks a real boundary. A path/package rename should update imports and be verified independently from behavior changes.
- `internal/fontmanager` is a copyable source subtree for catalog lifecycle, trusted font downloads, fallback policy, and bundled font resources. Keep `fontcatalog`, `fontdownload`, `fontfallback`, and `builtin` within it; the subtree must not import Glance application, document, workspace, or Gio packages. Another project can copy the directory, adjust its module import prefix, and provide host paths and a trusted provider. Gio and PDF shaping stay in host adapters and consume the manager's immutable snapshots.
- The current UI package calls several format-adapter packages for document creation, format-specific view data, and export. It does not directly import the Office libraries; moving adapter construction and those format-specific seams out of `desktopui` remains a staged architecture task, not something to mix into a file split.

The source-organization pass split Word toolbar functionality into same-package files, leaving `word_toolbar.go` under 500 lines and moving search, event handling, editing, page setup, insert, review, and design responsibilities to focused files. The spreadsheet command surface is split into focused command, filter, formula, page-layout, protection, data, and insertion files; the toolbar is split into action definitions, events, capabilities, layout, pickers, and commands. Additional behavior-preserving splits cover app/document lifecycle, Word layout/page interaction, spreadsheet view layout and async results, workspace/session persistence, DOCX projection/text/XML editing and document parsing/saving, PPTX document/geometry/view modules, PDF spreadsheet export and viewing, and the Word layout engine's setup, table, row, and paragraph phases. Recent focused splits also separate runtime session/window/workspace persistence, presentation canvas drawing/hit testing/text layout, workspace-tab lifecycle, Word ribbon rendering, XLSX workbook reading/editing/saving, DOCX image/paragraph operations, spreadsheet image loading/editing/painting, virtual-grid controls/loading/layout, Word paragraph insertion and navigation indexing, font-cache setup, DOCX core-properties package-link handling, PPTX style XML helpers, Word PDF rendering, confirmation-dialog styles, JSON tree projection from workspace actions, glyph-outline extraction/cache, and Word command completion from general async polling. Production and test Go files are organized by cohesive responsibility, generally around or below 500 lines; test splits preserve names, fixtures, and assertions. Do not treat line count alone as permission to move code across package boundaries.

## Agent turn budgets and conversation switching

Each send or resume starts a fresh tool budget. The defaults allow 200 tool execution rounds and 600 tool calls; explicit `HostOptions` can set smaller bounds. After the last permitted tool round or call, the model receives one final request without tools to report completed results and any unfinished work. A further tool request is rejected, and the final prompt remains subject to the context-size limit.

Creating a conversation detaches the old subscription and clears its transient UI state before asynchronous persistence starts. A busy old turn is cancelled in the background; its stored history remains available. Conversation IDs and history generations reject late events and results from the previous conversation.

## Save safety and compatibility

- Saving is an overwrite of a user's document; advertise edit/save capability only when that adapter and file class pass round-trip preservation checks.
- First editor scope should be small: XLSX cell constants (text, number, boolean, blank), and a clearly defined limited DOCX text workflow. Defer formula calculation, range paste, style editing, page-layout editing, and legacy `.xls`/`.doc` unless explicitly required.
- The adapter must report unsupported or failed operations. Failed saves retain the dirty session and must leave the original file intact.
- Track compatibility by concrete feature and fixture evidence. A successful parse is not evidence of lossless save. Reopen saved files and compare changed values plus representative untouched styles, merges, formulas/cached values, images, tables, and package relationships.
- Never claim Word pagination or Excel visual parity based only on extracted text. Keep preview layout fidelity separate from document read/write fidelity.

## Reference projects

- [Virtualized spreadsheet grid design](superpowers/specs/2026-09-28-virtualized-spreadsheet-grid-design.md) — two-axis virtual extent, inline editing, common styles, merged ranges, and dimension resizing.
- `../tools/docs/compatibility/office-feature-matrix.md` — evidence-based feature matrix and distinction between supported, partial, and unsupported behavior.
- `../tools/docs/superpowers/specs/2026-09-27-xlsx-cell-editing-design.md` — session, dirty state, atomic save, and XLSX round-trip safety.
- `../mysql-ai/AGENTS.md` — focused-file and Gio form guardrails.
- `../mysql-ai/docs/gio-form-layout-gotchas.md` — persistent widget/list, row width, constraints, draft flush, and end-to-end state tracing.
- `../mysql-ai/docs/drag-patterns.md` — stable preview/persist separation and pointer-coordinate normalization.

These sibling paths are local references, not build dependencies. When documenting a rule in this repository, keep its rationale and actionable form here so the project remains understandable if siblings move.


## Shared font library (2026-10-15)

`fontcatalog` owns neutral family/face metadata, localized aliases, content identities, platform scanning and the atomic disk index. It streams metadata and hashes without parsing all glyphs. `application.FontService` serializes scans, private registration and source preferences; the desktop runtime shares it across windows. Windows subscribe to immutable versions. Startup publishes a compatible index before background verification; explicit refresh hashes files again rather than using go-text's process-wide `SystemFonts` cache. Unchanged scans update `CheckedAt` without changing the rendering version.

`fontdownload` accepts a trusted, versioned HTTPS manifest, validates the byte limit, SHA-256, static TrueType format and family, then publishes a private object and license declaration. Objects live in the user configuration/data area; the index lives in the user cache area. Cache eviction does not remove font objects. The production provider initially returns no entries. Download/register and apply-format are separate intents.

The shared Gio picker stores its editor, list and clickables in stable window state. It captures session/revision/selection identity before applying through existing Word or spreadsheet commands. Document font summaries, scanning, downloading and construction of Gio font collections run in managed background jobs. The picker has document fonts first, followed by the local catalog and optional online rows. It does not contain empty online sections.

Word shaping and each PDF export acquire a catalog snapshot; loaded bytes remain owned by that task's shaper. Resource reads verify the captured digest. If an external font changes before first use, the task reports a resource error instead of switching outlines. Word pagination keys include font version; publication clears Word/Gio grid text caches without modifying document formats, row/column dimensions or canceling prepared exports. Gio receives bounded collections of the selected/visible fonts, leaving UI typography tokens unchanged. A same-name source preference is local and cached, never written as a new document family name.

## Offline Markdown diagrams and Windows emoji (2026-10-23)

`diagramrender` is a Go format-resource adapter over `nativewebview`: macOS uses the system WKWebView; Windows discovers an installed Edge or Chrome and drives it through CDP. Windows therefore needs one of those browsers installed, but no separate WebView2 installer or Node.js. The app embeds the pinned Mermaid/Vega JavaScript and expression interpreter and does not download renderer assets at runtime. Rendering stays on background resource jobs and returns pixels plus neutral logical dimensions with bounded queue, cache, time and output size. Fixed Mermaid settings, a restrictive local page CSP that permits app-embedded renderer scripts but blocks dynamic evaluation and external resources, an SVG image context and WebView request/navigation blocking constrain document execution and resource loading; normal Markdown HTTP image fetching remains in `markdowndoc`. The existing headless-runtime packaging target is not used by the default desktop resource loader.

`document.ResourceImage` separates logical geometry from 2x diagram/SVG pixels without exposing browser types or replacing existing `image.Image` maps. Markdown preview and PDF projection consume that geometry and reuse accepted resources across export confirmation. Font catalog publication invalidates diagram/browser font caches and pending Markdown resource generations.

`emojifont` owns the embedded OFL bitmap Noto font under a private alias. Desktop Gio shapers include it as a fallback; the platform family order keeps Apple Color Emoji ahead of it on macOS and the existing private-font preference on Windows. Immutable parsed data is shared while each Gio shaper obtains an independent face. Initial rendering, font refresh and measurement shapers use the same factory; parsing never belongs in a frame. For Markdown PDF only, unsupported emoji graphemes are embedded as bundled-font PNGs because the vector PDF text shaper cannot embed bitmap color-font tables; those emoji are visible but not searchable text. Platform evidence and limits are in `windows-emoji-verification.md` and `markdown-diagrams-verification.md`.

Markdown `vega` and `vega-lite` fences keep their language in the neutral document model and render through pinned, embedded Vega assets plus the offline expression interpreter. Static preview resources use the native WebView one-shot path; interactive charts open bounded persistent native WebView sessions, while Gio draws their snapshots and forwards pointer/modifier-wheel input asynchronously. PDF freezes a settled chart image (or uses the initial offline image for unopened charts), so exported charts are static. External data, image, patch, plugin and hyperlink resources are rejected; runtime downloads and `unsafe-eval` are disabled. Windows keeps installed Edge/Chrome discovery and adds no browser installer requirement.
