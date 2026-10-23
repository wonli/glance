# Process-wide and copyable font manager design

## Intent

Move font lifecycle and policy out of `internal/application` into a self-contained `internal/fontmanager` subtree. One manager instance will supply font resources to all family-based Glance UI text, document rendering, measurement, print, and PDF export. The immediate motivation is the previous PDF export failure: PDF and Markdown used inconsistent font state. One manager must coordinate downloaded resources and pass the same catalog snapshot and shaping path to every consumer.

The migration preserves today's UI appearance: keep current face-family order, typeface values, typography tokens, sizes, weights, line metrics, spacing, and control geometry. This changes where font resources and versions come from; it is not a visual redesign or a font-family refresh.

The manager is process-wide in Glance and designed so its directory can be copied into another project with only the module import prefix and host configuration adapted. It is not a separately versioned Go module and does not install, enable, or remove fonts in the operating system.

## Current state

- `internal/fontcatalog` already owns neutral family/face metadata, immutable snapshots, file inspection, platform scanning, content identity, and the disk index.
- `internal/application.FontService` owns scan serialization, registration, source preferences, cache persistence, subscription events, and shutdown. The desktop runtime shares one instance across windows.
- The live manager publishes snapshots through `fontcatalog.SetDefault`; `wordfont.NewSystem`, Word pagination, Excel auto-fit, and parts of the picker read `fontcatalog.Default` directly.
- Document/content text can build Gio font collections from catalog snapshots, but the desktop theme's primary typeface still comes from `fontfallback.PreferredTypeface`, and the initial Gio shaper may use platform system fonts outside the live manager.
- PDF adapters construct Word shapers internally. Application PDF and print capabilities do not currently receive a font snapshot.
- `internal/fontdownload` is currently a top-level downloader that validates downloaded resources before registration; it should become a subpackage under `internal/fontmanager` so the manager owns the complete download-to-catalog flow.

This means document font handling is partially shared, but interface text can bypass the catalog, the manager is stored in the application layer, and rendering consumers can bypass it through a process-global default.

## Goals

- Add `internal/fontmanager` as the owner of scan, refresh, register, preference, event, and manager shutdown behavior.
- Move download/provider implementation under `internal/fontmanager` and have the manager coordinate listing, fetch, validation, durable publication, and catalog registration.
- Put the neutral catalog, download implementation, and font-family fallback policy inside one copyable package subtree with no Glance application, document, workspace, or Gio dependencies.
- Put Glance's bundled fallback and emoji font resources under the same manager-owned resource snapshot, while leaving Gio shaping and specialized PDF/Markdown bitmap painting in thin host adapters.
- Have one manager instance per Glance process, shared by all desktop windows and the process's `application.Runtime`.
- Use the same manager snapshot for every supported document type and format, including Word, Excel, Markdown, presentation, PDF, JSON, CSV, and other text-based documents wherever they render, measure, preview, print, or export text.
- Make all family-based interface UI text and all document rendering/measurement consumers resolve font resources from immutable snapshots owned by that manager.
- Resolve the primary Gio theme face and UI shaper from the manager catalog while preserving distinct role sizes, weights, and typography tokens.
- Capture one snapshot at the start of a layout, measurement, print, or export task and use that version for the full task.
- Remove production reads and writes through `fontcatalog.Default` and `fontcatalog.SetDefault`.
- Keep font scanning, parsing, and collection construction outside Gio layout and frame code.
- Keep existing catalog cache behavior, digest checks, render invalidation, and download/register versus apply-format separation.

## Non-goals

- OS account font installation, activation, or removal.
- A new online font provider or non-empty public manifest.
- Changes to DOCX/XLSX font embedding, document family names, formula behavior, row/column dimensions, or UI typography tokens.
- Replacing `fontcatalog`, the platform scanner, the Word shaping engine, or the existing download validation policy.
- Forcing every control and document style to use one family, size, or weight; the source is unified while role-specific typography remains.
- Turning the package into a separately versioned public Go module in this task; source-level reuse by copying the package subtree is the target.
- Replacing the special bitmap emoji renderer; its font bytes and identity still come from the manager-owned resource snapshot.

## Constraints

- The workspace architecture remains Gio UI -> application/session -> neutral document model -> format adapters -> local files. Font manager types must not leak into document models or widgets beyond the shared runtime boundary.
- `fontcatalog.Snapshot` remains immutable to callers. A snapshot identifies the catalog version and resource digests; a task that has already loaded bytes continues to own those bytes.
- External source changes before first use continue to fail digest validation and request a refresh rather than silently switching outlines.
- A font refresh must not mutate a document, mark it dirty, rewrite a stored family name, change spreadsheet geometry, or cancel an already prepared export.
- All code, builds, and any later verification stay in `/Users/mima1234/project/glance-go`. No worktree is created for this change.

## Approaches

1. **Create a self-contained `internal/fontmanager` subtree with manager, `fontcatalog`, `fontdownload`, and font-family fallback packages.** The manager owns lifecycle and acquisition policy, while the child packages keep catalog metadata and transfer validation focused. The complete subtree can be copied to another project; host paths, provider, and locale are injected. This avoids a separate module/versioning change while making the core portable. **Selected by the user.**
2. Publish a separately versioned Go module immediately. This gives consumers a stable import path but requires module/repository versioning and distribution work the user does not need for source-copy reuse.
3. Move only `application.FontService` into `internal/fontmanager`, leaving the catalog, downloader, and interface theme fonts on their existing paths. This minimizes file movement but leaves the same font divergence between consumers.

## Proposed architecture

```text
Glance process composition root (`desktopRuntime` or headless CLI root)
        └── one `fontmanager.Manager`
              ├── fontcatalog (metadata, scans, immutable snapshots)
              ├── fontdownload (trusted online entries and validated downloads)
              ├── fontfallback (platform and script fallback policy)
              └── builtin resources (bundled fallback and emoji font bytes)
                    ↓ immutable snapshot
        Gio / WebView adapters + document content / Word shaping / Excel / PDF / print
```

`internal/fontmanager` contains the manager plus `fontcatalog`, `fontdownload`, and `fontfallback` child packages. Its tree has no imports from `glance/internal/application`, `desktopui`, `document`, or `workspace`; the only project-specific change needed when copying it is adapting the module import prefix. Cache and data paths, locale, and trusted provider are supplied by the host project instead of hardcoded in the core.

The subtree also owns bundled fallback font resources, including the current emoji font, as immutable snapshot resources. Gio collection construction and the specialized emoji-to-bitmap drawing paths remain outside the portable core, but they receive the exact resource bytes/digests selected by the manager. A small package README records the supported Go version, third-party module requirements (including `github.com/go-text/typesetting` where used), platform scanner requirements, and host options. To reuse the code, another project copies the subtree within its module, adapts the import prefix, adds the listed dependencies, and supplies its own paths, locale, and trusted provider. This is source reuse, not a promise of a stable cross-module API.

`fontmanager.Manager` exposes the current immutable catalog snapshot, snapshot resolution, interface-family resolution from a supplied locale/platform policy, status/version subscriptions, cache initialization, refresh, private registration, trusted entry listing, download-and-register with progress/cancellation, local source preference, and close/drain. The `fontdownload` child validates and publishes downloads but does not import the parent manager. The manager owns the sequence: list trusted entries, fetch/validate/publish, then register into the catalog.

The process composition root constructs the manager once and stores it on `desktopRuntime`. It passes the same manager (or a narrow snapshot source) into `application.RuntimeOptions` and each attached window. Headless `mcp serve --stdio --open PATH` creates and owns its own manager because it is a separate process. The stdio bridge to an already running desktop continues to use that desktop process's Runtime and manager.

The manager is not a package-global singleton. A process root owns exactly one instance in production; isolated runtimes and tests can inject independent managers without overwriting process state. Remove `fontcatalog.Default`/`SetDefault` once production callers have migrated.

The `fontcatalog` child remains responsible for neutral catalog data, scanning, identity, resource reads, and atomic index persistence. The nested `fontmanager/fontdownload` package remains responsible for transport-level manifest checks, byte limits, checksums, font validation, staging, and durable object/license publication. A built-in resource child provides bundled fallback/emoji bytes and stable identities without depending on Gio. The manager owns the sequence: list trusted entries, fetch/validate/publish, then register into the catalog. The picker calls only the manager API; download completion still does not apply a font to the current document.

## Consumer changes

- **Desktop runtime and windows:** initialize the shared manager asynchronously outside frame/layout code. Picker, alias resolution, online entry listing, download/register, grid auto-fit, Word layout keys, and every Gio text collection go through the shared manager. Resolve `material.Theme.Face` and its `Shaper` against the same catalog used by document canvas text; this covers shell labels, menus, dialogs, editors, settings, ribbon, and canvas labels. Build primary and script-fallback faces only from the captured snapshot, including manager-owned bundled resources. Preserve today's resolved face order and typeface values; do not alter sizes, weights, line metrics, colors, spacing, or control geometry. If a preferred family is absent, try the same existing ordered fallback candidates from that snapshot; if none can be loaded, report a stable font-resolution error instead of falling through to Gio's hidden platform fonts. A single captured snapshot is passed into each background measurement or rendering job. Font-dependent startup and operations wait for first-scan readiness whether or not a compatible cache exists. Existing event handling invalidates Word, Gio UI/grid text, and Markdown browser caches only when the rendering version changes.
- **Application Runtime:** accept the process manager or a narrow snapshot source in `RuntimeOptions`. Every Word/Excel/Markdown/PowerPoint PDF export and print capability captures one snapshot before starting and passes it into the format adapter.
- **All document formats and shaping adapters:** use `wordfont.NewWithSnapshot` (already available) or an equivalent explicit constructor for Word, Excel, Markdown, presentation, PDF, and print text measurement/rendering. JSON, CSV, and other editor-based formats use the same manager-backed Gio theme collection. PDF and print APIs receive a `fontcatalog.Snapshot`; they no longer call `wordfont.NewSystem` internally. Each operation owns its shaper and loaded bytes for the task lifetime.
- **Default/system construction:** retain a clearly named standalone helper only for isolated utilities or tests that explicitly need to resolve a system catalog. It must load or receive an explicit snapshot and must not consult process-global mutable state. All production Glance call sites use the injected manager snapshot.
- **UI and embedded web text:** the manager supplies the source and resolved face for interface text as well as document/content text. Gio-based Markdown, JSON, and CSV views use manager-built collections. Browser-backed diagram rendering keeps its existing CSS family order, with that declaration stored in the manager's fallback policy; it remains a native WebView renderer. The emoji font bytes are owned by the manager subtree, while Gio and PDF/Markdown adapters preserve their existing vector/bitmap-specific drawing paths. Existing family selection order and typography remain unchanged.

If a library-level `application.Runtime` is constructed without a manager, normal document operations remain available. Font-dependent print/export operations must report that a font snapshot source is unavailable instead of reading hidden global state. Production desktop and standalone headless entry points always provide and initialize the manager. Tests that exercise font-dependent operations inject a temporary manager or an explicit synthetic snapshot.

## Lifecycle and concurrency

The process root creates the manager with the current cache and private-resource paths. Startup loads a compatible disk index first, publishes it to subscribers, then verifies it in a managed background task. Font-dependent startup and operations wait cancellably for that first verification even when a compatible cache exists; callers capture the verified snapshot after the scan. If verification fails, the last known good cache remains usable after the scan completes. With no usable snapshot, the caller receives the scan error and can retry after a later refresh. Readiness waiting never runs in Gio layout or frame code.

Explicit refresh remains serialized and force refresh rehashes source files. Unchanged scans update `CheckedAt` without changing the rendering version. Failed scans preserve the last known good snapshot and publish diagnostics.

Manager close cancels and joins manager-owned tasks. Shutdown first stops new MCP requests and waits for in-flight application/document tasks, then closes the application Runtime and manager in an order that does not release font resources while exports still use them. Window subscriptions are removed as each window shuts down; closing one window never closes the process manager.

Each consumer holds the exact snapshot it started with. A newly registered font or changed source advances the version only when rendered resources or local preferences change. Prepared tasks continue with their captured snapshot and already loaded bytes; later tasks observe the new version.

## Migration and compatibility

1. Move `FontService`, events, options, and lifecycle tests from `internal/application` to `internal/fontmanager` without changing scan, cache, refresh, registration, preference, or close semantics.
2. Move `internal/fontcatalog`, `internal/fontdownload`, `internal/fontfallback`, and the font-resource assets currently owned by `internal/emojifont` under `internal/fontmanager` as focused child packages/resources. Keep their imports limited to standard and third-party libraries plus sibling fontmanager packages, so the full subtree can be copied without Glance business code.
3. Route provider listing, fetch progress/cancellation, and catalog registration through the manager API so desktop UI no longer owns a downloader or provider.
4. Create the manager at desktop process composition, inject it into `application.Runtime` and windows, and initialize it at desktop or standalone headless process startup.
5. Replace each production catalog global read/write and `wordfont.NewSystem` path with explicit manager snapshot injection. Thread snapshots through PDF/print adapter boundaries so PDF and Markdown PDF resolve the same resources and shaping rules.
6. Make the desktop theme's primary `Face`, every Gio UI `Shaper` collection, and Markdown/WebView font declarations derive from the same manager snapshot, removing independent platform font loads from UI text paths. Keep Gio shaping and bitmap rendering in adapters that consume manager-selected bytes.
7. Remove obsolete global catalog publication after every production call site has migrated. Keep isolated utility fallback behavior explicit.
8. Update `docs/architecture.md` and the font verification record with the new ownership boundary and evidence. Do not broaden document-format support claims.

## Acceptance evidence

- Manager package tests preserve existing scan coalescing, refresh failure retention, refresh/register ordering, unchanged-version, preference persistence, subscriber, and close/drain behavior.
- Two windows and the application Runtime observe the same manager instance, version, refresh status, and newly registered private font.
- The manager subtree has no Glance application/UI/document/workspace/Gio imports, accepts host configuration through options/interfaces, documents its Go/toolchain and third-party prerequisites, and can be copied into another Go project's module by adapting its import prefix.
- Online entry listing, download, validation, durable publication, and registration are exposed as one manager workflow; the UI cannot publish a font without catalog registration.
- With a compatible cache, font operations can use it while verification runs. With no cache, font-dependent background work waits for first-scan completion, responds to cancellation, and reports initial-scan failure without blocking the window loop.
- A manager created for a second isolated test/runtime does not alter the first manager's snapshot or any package-global state.
- Shell UI controls, editor/canvas text, emoji, Word layout, Excel auto-fit, Gio collections, Markdown/WebView font declarations, print, and PDF export all use the captured manager version; a refresh during a task cannot change that task's selected font data. No production UI text path silently falls through to a platform font collection outside the manager snapshot.
- Before and after the migration, the primary UI typeface/fallback order and typography tokens, sizes, weights, line metrics, spacing, and control geometry are unchanged.
- Word, Excel, Markdown, presentation, JSON, CSV, and other text-bearing formats resolve rendered or measured text through the process manager when that format has a text-rendering path; format adapters do not create independent system shapers.
- Offscreen/in-memory text rendering checks representative Glance labels, controls, document canvas text, and PDF output against the same resolved font resource IDs; GPU visual checks confirm normal and Retina UI rendering uses the manager-built collection.
- PDF and Markdown PDF exports of representative content use the same captured snapshot and shaping policy, so the previously reported font divergence cannot recur through separate catalog acquisition paths.
- PDF and print use a synthetic private font from the supplied snapshot after the source is removed following resource loading, preserving task-owned resource lifetime.
- Existing digest mismatch, private registration, force-refresh, cache, no-op document formatting, and save/reopen behavior remain intact.
- Source has no production references to `fontcatalog.Default` or `fontcatalog.SetDefault`; no production font-sensitive path creates a new system shaper without an explicit snapshot.
- `git diff --check` is clean. Any runtime UI verification uses a fresh Glance process and distinguishes test or compile success from actual window rendering.

## Calls left to the builder

- Choose exact manager option names and whether `application.RuntimeOptions` accepts `*fontmanager.Manager` directly or the narrower snapshot interface; preserve a one-way dependency graph.
- Choose exact snapshot-aware export function signatures while making all production call paths explicit and keeping test fixtures readable.
- Keep version invalidation coalesced and publish immutable events; do not run font work from Gio layout.
