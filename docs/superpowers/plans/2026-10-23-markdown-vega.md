# Markdown Vega Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Add offline, pointer-interactive Vega and Vega-Lite fenced charts to Markdown Preview and Split modes, and export a static snapshot of each chart to Markdown PDF.

**Architecture:** The Markdown adapter preserves each fence language in neutral document data. `diagramrender` owns embedded Vega assets, spec validation and offline rendering; `nativewebview` owns persistent browser sessions and platform-specific pointer dispatch. Gio owns stable chart hit regions and renders asynchronously published snapshots, while PDF preparation freezes chart images into the existing Markdown resource map.

**Tech Stack:** Go, Gio, pinned embedded JavaScript assets (Vega, Vega-Lite, Vega-Embed, expression interpreter), macOS WKWebView, Windows Edge/Chrome DevTools, existing Markdown resource and PDF pipelines.

**Spec:** [docs/superpowers/specs/2026-10-23-markdown-vega-design.md](../specs/2026-10-23-markdown-vega-design.md)

## Global Constraints

- Stay in the current authorized workspace and branch; do not create or switch worktrees.
- Bundle pinned Vega, Vega-Lite, Vega-Embed and interpreter assets, with upstream licenses and version/digest metadata; never download assets at runtime.
- Never execute Markdown-supplied JavaScript or HTML, and never permit remote spec data, images, patches or plugins.
- Always use the bundled expression interpreter; do not fall back to `unsafe-eval`.
- Bound source bytes, inline rows, render time, chart dimensions/pixels, cache size and live sessions.
- Keep WebView work outside Gio layout/frame callbacks; reject stale results and close sessions when source, document or preview view changes.
- Normal wheel input scrolls Markdown; Shift/Ctrl-wheel is forwarded to Vega.
- PDF uses a settled static snapshot with transient hover cleared; a chart never opened in Preview uses the offline initial snapshot.
- Unsupported charts remain readable as source with a localized diagnostic. Preserve existing Mermaid, math, image and ordinary code-fence behavior.
- macOS uses WebKit; Windows continues locating installed Edge/Chrome and must not acquire a new installer/WebView2 runtime requirement. Linux native rendering is outside this change.

## Review Focus

1. External URLs in data, image, patch and loader paths must be rejected without network requests. Test in Task 2 with each URL-bearing spec form.
2. Malformed and oversized specs must fail safely within limits. Test in Task 2 for invalid JSON, rows, dimensions, pixels, timeout and cancellation.
3. Events must target the correct chart in CSS coordinates at 1x/2x and with multiple charts. Test in Tasks 3 and 4 with chart-local coordinates and hit bounds.
4. Rapid input and source/document changes must not publish stale frames or leak sessions. Test in Tasks 3 and 4 with coalescing, cancellation and cleanup.
5. PDF must include settled selection/zoom while excluding transient hover and remain usable after sessions close. Test in Task 5 for active and unopened charts.
6. Normal wheel must scroll Markdown while modifier-wheel reaches Vega. Test in Task 4 with both paths.
7. Windows must retain installed Edge/Chrome discovery and compile without WebView2 installer changes. Test in Task 3 and record whether native Windows interaction was available.

---

### Task 1: Preserve chart fence language in neutral Markdown data

**Files:**
- Modify: `internal/document/markdown.go`
- Modify: `internal/format/markdowndoc/parse.go`
- Test: `internal/format/markdowndoc/parse_test.go`
- Test: `internal/format/markdowndoc/diagram_resources_test.go`

**Interfaces:**
- Consumes: Goldmark fenced-code blocks and existing `MarkdownBlock{Kind, Text, Language}`.
- Produces: chart blocks as `MarkdownBlock{Kind: "diagram", Language: "vega"|"vega-lite", Text: source}`; `DiagramSpan(block)` returns `MarkdownSpan{Kind: "diagram", Language: block.Language, Text: block.Text}`. `ResourceKey(span)` includes diagram language; Mermaid retains its established key.

- [x] **Step 1: Write failing parser and resource-key tests.** Cover `vega`, `vega-lite`, mixed-case tags, exact source/language preservation, language-separated keys, and unchanged `json`, `javascript`, nested-fence examples, and indented code behavior.
- [x] **Step 2: Run tests and verify failure.** Run `go test ./internal/format/markdowndoc -run 'Test.*(Fence|Vega|Diagram)' -count=1`; expect missing chart recognition and language separation.
- [x] **Step 3: Implement neutral language retention and fence recognition.** Canonicalize only the two supported tags; leave all other code fences untouched.
- [x] **Step 4: Run tests and verify pass.** Run `go test ./internal/document ./internal/format/markdowndoc -count=1` and `git diff --check`.
- [x] **Step 5: Commit the task.** `feat(markdown): recognize Vega chart fences`.

### Task 2: Bundle and securely render static Vega charts offline

**Files:**
- Create: pinned vendor JS and license files under `internal/diagramrender/assets/`
- Modify: `internal/diagramrender/assets/manifest.json`
- Modify: `internal/diagramrender/bundle.go`
- Modify: `internal/diagramrender/renderer.go`
- Modify: `internal/diagramrender/page.go`
- Modify: `internal/format/markdowndoc/resources.go`
- Test: `internal/diagramrender/renderer_test.go`
- Test: `internal/diagramrender/security_test.go`
- Test: `internal/diagramrender/bundle_test.go`
- Test: `internal/format/markdowndoc/diagram_resources_test.go`

**Interfaces:**
- Consumes: `diagramrender.Request{Kind, Source, Density}` where `Kind` is `vega` or `vega-lite`.
- Produces: existing `Renderer.Render(context.Context, Request) (Result, error)` returns the initial PNG and logical dimensions for resource preview/PDF; validation and page creation are shared by static and interactive rendering.

- [x] **Step 1: Write failing renderer/security tests.** Cover representative Vega and Vega-Lite `data.values` specs, visible PNG output, invalid JSON, invalid spec diagnostics, blocked URLs in data/image/patch paths, oversized source/rows/dimensions/pixels, cancellation, and distinct cache keys by kind.
- [x] **Step 2: Run tests and verify failure.** Run `go test ./internal/diagramrender -run 'Test.*(Vega|Security|Bundle)' -count=1`; expect unsupported kind or missing security behavior.
- [x] **Step 3: Vendor pinned assets and provenance.** Bundle compatible Vega, Vega-Lite, Vega-Embed and expression-interpreter releases; include their upstream licenses and exact versions plus SHA-256 in `manifest.json`. Verify hashes against embedded bytes.
- [x] **Step 4: Implement shared offline page and validation.** Add strict local rendering, inline-values-only data, a loader rejecting external resources, disabled actions, bundled interpreter, fixed theme, and byte/row/time/dimension/pixel limits. Forward each diagram span's language into the renderer and return diagnostics through the existing resource error path.
- [x] **Step 5: Run tests and verify pass.** Run `go test ./internal/diagramrender -count=1`; verify every manifest digest and license.
- [x] **Step 6: Commit the task.** `feat(markdown): bundle offline Vega renderer`.

### Task 3: Add persistent WebView sessions and pointer dispatch

**Files:**
- Modify: `internal/nativewebview/engine.go`
- Modify: `internal/nativewebview/engine_darwin.go`
- Modify: `internal/nativewebview/engine_darwin.m`
- Modify: `internal/nativewebview/engine_windows.go`
- Modify: `internal/nativewebview/engine_fallback.go`
- Test: `internal/nativewebview/*_test.go`
- Modify: `internal/diagramrender/renderer.go`
- Modify: `internal/diagramrender/page.go`
- Test: `internal/diagramrender/*_test.go`

**Interfaces:**
- Consumes: Task 2's validated chart page and existing one-shot `Engine`.
- Produces: `PointerEventKind` values `move`, `down`, `up`, `leave`, `wheel`; `PointerEvent{Kind PointerEventKind, X, Y float64, Button int, Buttons uint32, DeltaX, DeltaY float64, Shift, Control bool}`; and `SessionRequest{HTML, Script string, Density int}`. `SessionEngine.OpenSession(context.Context, SessionRequest) (Session, error)`; `Session.Dispatch(context.Context, PointerEvent) (RenderResult, error)`, `Session.Snapshot(context.Context) (RenderResult, error)`, `Session.Close() error`. `diagramrender.Renderer.OpenChart(context.Context, Request) (ChartSession, error)` returns `ChartSession{Dispatch(context.Context, nativewebview.PointerEvent) (Result, error), Snapshot(context.Context) (Result, error), Close() error}`.

- [x] **Step 1: Write failing session contract tests.** Verify the chart state survives a forwarded down/up/click and later snapshots, event data reaches the session unchanged, the four-session bound releases cleanly, close is idempotent, and closed sessions reject use. Add fake-engine tests for deterministic lifecycle.
- [x] **Step 2: Run tests and verify failure.** Run `go test ./internal/nativewebview ./internal/diagramrender -run 'Test.*(Session|Pointer|Chart)' -count=1`; expect missing session API.
- [x] **Step 3: Add the platform-neutral session contract.** Keep `SessionEngine` as an optional extension of the existing one-shot engine, so existing render clients remain compatible. Serialize calls per session and use CSS-pixel coordinates.
- [x] **Step 4: Implement macOS session support.** Keep a persistent WKWebView alive, dispatch synthetic pointer/wheel input on its owner queue, await Vega `view.runAsync()` before snapshots, and honor cancellation/close. Do not wait on `requestAnimationFrame`: hidden offscreen WKWebViews can pause animation frames.
- [x] **Step 5: Implement Windows session support.** Keep a persistent Edge/Chrome DevTools target per live chart, dispatch equivalent DOM events, bound target/browser cleanup, and preserve current installed-browser discovery. Add Windows cross-build verification.
- [x] **Step 6: Set session/resource limits and place event coalescing at the Gio boundary.** Four persistent charts opened in 1.47s on the available macOS host and remained isolated; the fake-engine test verifies a released slot can be reused. Move coalescing, refresh rate limiting, and last-image retention to Task 4, where Gio input events and published images are managed.
- [x] **Step 7: Run tests and verify pass.** `go test -timeout=120s -p=1 ./internal/nativewebview ./internal/diagramrender ./internal/format/markdowndoc -count=1` passes on macOS, including a click-driven signal update and four independent live sessions. Windows native runtime is unavailable here; `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c` passes for `nativewebview` and `diagramrender`.
- [x] **Step 8: Commit the task.** `feat(markdown): add interactive chart webview sessions`.

### Task 4: Integrate stable chart state into Markdown Preview

**Files:**
- Modify: `internal/desktopui/markdown_state.go`
- Modify: `internal/desktopui/markdown_async.go`
- Modify: `internal/desktopui/markdown_preview.go`
- Modify: `internal/desktopui/markdown_preview_interactions.go`
- Modify: `internal/desktopui/markdown_lifecycle.go`
- Test: related `internal/desktopui/markdown_*_test.go`

**Interfaces:**
- Consumes: Task 1's chart-language block/span and Task 3's `diagramrender.ChartSession`.
- Produces: stable per-chart state keyed by block identity and source generation, holding session, last image, pending event job and event tag; chart events are launched through the existing managed background task path and accepted only for the matching source generation.

- [x] **Step 1: Write failing preview interaction tests.** Cover hit bounds, chart-local mapping at 1x/2x, pointer move/down/up/leave and modifier-wheel dispatch, normal-wheel document scrolling, stale result rejection after edits/switches, and session cleanup on source/view teardown.
- [x] **Step 2: Run tests and verify failure.** Run `go test ./internal/desktopui -run 'TestMarkdown.*(Vega|Chart)' -count=1`; expect missing chart interaction/state.
- [x] **Step 3: Add stable per-chart state and async event publication.** Create sessions outside layout; preserve last snapshots; publish updates through synchronized state and invalidate the window. Keep the chart's layout dimensions stable during refresh.
- [x] **Step 4: Add chart drawing and input arbitration.** Draw snapshots in the normal diagram slot; normalize pointer coordinates at the Gio input boundary; route normal wheel to Markdown and Shift/Ctrl-wheel to the chart. Preserve readable source and localized diagnostics for failures.
- [x] **Step 5: Add lifecycle and stale-result rejection.** Cancel pending jobs and close sessions on source edits, document switch, preview teardown and close. Require both generation and chart identity to match before accepting results.
- [x] **Step 6: Run tests and verify pass.** Focused chart/preview/PDF tests pass and cover 1x/2x coordinates and multi-chart event routing. The full `internal/desktopui` run was attempted as part of Task 6 and is blocked by unrelated locale/spreadsheet assertions plus sandbox-denied `httptest` listeners; see the task ledger and [verification record](../../markdown-diagrams-verification.md).
- [x] **Step 7: Commit the task.** `feat(markdown): add interactive Vega preview`.

### Task 5: Freeze chart snapshots into Markdown PDF

**Files:**
- Modify: `internal/desktopui/markdown_pdf_export.go`
- Modify: `internal/desktopui/markdown_pdf_preview.go`
- Modify: relevant Markdown PDF export tests in `internal/desktopui/`
- Modify: relevant tests in `internal/format/markdownpdf/` and `internal/format/pdf/`

**Interfaces:**
- Consumes: Task 4's active chart sessions and existing preview resource images.
- Produces: a frozen resource map where active chart keys are replaced with their settled session snapshots after pointer-leave; unopened charts retain the static offline initial snapshot from `PreviewResourcesContext`.

- [x] **Step 1: Write failing PDF snapshot tests.** Verify an active chart exports settled selection/zoom without hover UI, an unopened chart exports its initial offline image, and prepared PDF data remains usable after session close.
- [x] **Step 2: Run tests and verify failure.** With the snapshot-freeze call temporarily bypassed, `TestMarkdownPDFPreflightFreezesChartSession` failed by timing out after 5 seconds; restoring the call made the same test pass. The broader regex is not used because it includes a previously observed native formula/WebKit hang in this sandbox (recorded in the task ledger).
- [x] **Step 3: Collect snapshots on the export background path.** Before `prepareMarkdownPDF`, clone current resources, capture active sessions after pointer-leave, replace their resource images, and keep existing revision/cancellation checks. Do not call WebView APIs from layout.
- [x] **Step 4: Verify frozen export behavior.** Confirm prepared PDF has no live-session dependency and capture does not mutate source or settled selection/zoom.
- [x] **Step 5: Run tests and verify pass.** Focused Markdown PDF/Vega tests pass in `internal/desktopui`; related suites pass in `internal/application`, `internal/format/markdownpdf`, `internal/format/pdf`, and `internal/emojifont`.
- [x] **Step 6: Commit the task.** `feat(markdown): export Vega chart snapshots to PDF`.

### Task 6: Full integration, provenance and platform verification

**Files:**
- Modify: existing Markdown support documentation, if one describes supported preview fences
- Test: parser, renderer, nativewebview, desktop preview and PDF suites
- Verify: `internal/diagramrender/assets/manifest.json` and vendor licenses

**Interfaces:** Consumes all previous task deliverables; produces a verified implementation and records any platform coverage limitation.

- [x] **Step 1: Run focused package suites.** Parser/resource-key tests, renderer security/provenance/session-contract tests, native pointer validation, Markdown Preview/PDF integration, and related application/PDF packages pass. Current sandbox denied native WebKit setup, so the native rendering tests could not be repeated; Task 3 records their earlier pass.
- [x] **Step 2: Run repository checks.** Attempted `go test -timeout=90s -p=1 ./...`; unrelated locale/spreadsheet assertions and sandbox-denied local listeners failed, and the native renderer hit the WebKit setup timeout. `git diff --check` passes. See the verification record for details.
- [x] **Step 3: Audit security and provenance.** Focused tests verify no runtime browser lookup, a restrictive CSP without `unsafe-eval` (allowing only app-embedded inline renderer scripts), inline-only data, external-resource and hyperlink rejection, embedded asset SHA-256 values and upstream licenses. No runtime download path or Markdown-supplied script execution was added.
- [x] **Step 4: Verify platform behavior.** Task 3 recorded passing macOS WebKit click/session tests; the current sandbox denied WebKit bootstrap extensions. Windows amd64 `internal/desktopui` cross-build passed with `CGO_ENABLED=0`; native Windows Edge/Chrome interaction was unavailable. Limits are documented.
- [x] **Step 5: Review lifecycle and limits.** Focused regressions cover four independent live sessions, slot reuse, event forwarding, stale preview state, bounded/coalesced input, duplicate chart resources, hidden input draining, export concurrency and stale-snapshot rejection. These pass; native renderer re-execution is environment-blocked as above.
- [x] **Step 6: Commit final docs or follow-up fixes.** The final follow-up fixes and support documentation are committed as `fix(markdown): harden Vega chart export lifecycle`.

## Self-review

- **Spec coverage:** Tasks 1–6 cover fence recognition, local assets, security, pointer interaction, normal/modifier wheel policy, lifecycle, static PDF snapshots and platform constraints.
- **Step scan:** Each task has a failing-test step, an expected failing run, implementation steps and passing verification. Each change boundary has an independently testable output.
- **Type consistency:** Task 1 emits chart-language spans; Task 2 consumes `Request{Kind, Source, Density}`; Task 3's `OpenChart` consumes that request and exposes `ChartSession`; Tasks 4–5 consume the session and `ResourceKey` consistently.
- **Review focus:** Every listed failure mode has a test in the task that owns its behavior.
- **Proportion:** The plan stays close to the approved design scope and avoids unrelated Markdown/editor changes.
