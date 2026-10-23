# Offline Markdown diagram verification (2026-10-23)

## Native loader and batch correction

### Markdown close responsiveness

Closing a view cancels its parser and detaches chart state immediately. Native
session Close and idle shared-engine teardown run on background workers, including
the window-shutdown fallback. Capturing an idle-cleanup task takes no renderer
lock on the window thread. A usage generation rejects cleanup superseded by a
new document; teardown finishes the old native page before publishing a new
default engine. macOS teardown no longer invokes private WebKit force-close or
WebContent-kill selectors on the main queue.

`TestMarkdownViewReleaseDoesNotWaitForNativeClose` deliberately blocks native
Close and verifies the Markdown view is detached and its work cancelled while
cleanup is still blocked. Idle-cleanup regressions cover lock contention and a
new renderer use superseding old cleanup. Related Vega lifecycle, pointer and
native batch-render checks passed.

Actual window check: started a fresh process from the authorized code directory,
opened the original document, switched to Vega-Lite preview, moved the pointer
into the chart and observed a second active WebContent process. Closing the tab
returned to the home page; the window goroutine was waiting normally for events,
with no native teardown on its stack. Reopening from Recent displayed the source
again. No precise end-to-end latency or Windows close behavior is claimed.

The macOS navigation guard previously compared `NSURL.path` with `blank` for
`about:blank`. Foundation returns no path for that opaque URL, so the guard
cancelled its own renderer page and setup waited indefinitely. The guard now
checks the full local URL, and pending loads have an eight-second completion
guard. Null script arguments are normalized to an empty dictionary.

Markdown formulas, Mermaid and Vega/Vega-Lite initial images now render in one
page with distinct resource IDs through one native evaluation and snapshot.
Returned element bounds split the bitmap into independent resource images;
individual script errors remain attached to their resource. The batch is bounded
to 128 items and four million CSS pixels. Only pointer input opens a persistent
interactive chart WebView; layout alone retains the initial static image.

Manual Objective-C ownership now balances WebView/configuration/session retains
and scopes temporary native allocations in autorelease pools. Snapshot width
accounts for the screen backing scale: a requested 2x snapshot previously became
4x on Retina in the newly exercised batch path.

Verified in the authorized code directory using existing caches:

- `TestRenderBatchOneCallAndIndependentResults`: one engine call, independent
  resource errors and correct image cropping.
- `TestRenderBatchNativeMarkdownResources`: real WebKit produced visible pixels
  for inline math, matrix inverse, multiline align, Chinese Mermaid and Vega-Lite;
  each image has exactly twice its logical dimensions.
- `TestComplexMarkdownEndToEnd`: reads the supplied
  `/Users/mima1234/Downloads/complex_markdown.md`; five images, zero resource errors,
  including its plain official Vega-Lite schema and recovered Yuque flowchart.
- Targeted Gio checks cover no live WebView allocation during static chart
  layout, pointer dispatch at 1x/2x, late-session disposal and hidden-chart closure.

After restarting from the code directory with the original document path, the
actual Gio preview visibly displayed inline math, the matrix inverse and the
multiline derivation. The running application had one active WebContent process;
two pre-existing orphan WebContent processes had less than 1 MiB RSS each. Main
process RSS was still about 653 MiB, and active WebContent about 190 MiB. These RSS
readings are not directly comparable to Activity Monitor's memory metric and do
not establish that overall memory usage is acceptable. Full post-fix visual
acceptance of the chart section and Windows native execution remain unverified.

## Implemented boundary

Mermaid fenced blocks (`mermaid`, case-insensitive), Vega fences, and SVG image bytes enter the background Markdown resource pipeline. The default desktop renderer uses the system WKWebView on macOS and installed Edge or Chrome on Windows; ordinary PNG/JPEG/GIF continue through their existing decoder path. The app embeds pinned renderer assets and never downloads JavaScript at runtime. HTTP SVG image sources are fetched by the existing Markdown image loader before the SVG bytes enter the renderer.

Windows requires an installed Edge or Chrome for WebView rendering; the app does not install WebView2 or another browser. macOS uses the system WebKit framework. The repository still has a headless-runtime packaging target and explicit test fallback, but `diagramrender.Default()` uses `nativewebview.Default()` and does not consult that sidecar. Vega and Mermaid JavaScript assets and their upstream notices are embedded in the application; no Node.js or OS font installation is required for renderer setup.

Windows drives Edge/Chrome through CDP with a private profile; macOS owns WebKit sessions. One-shot rendering is limited to one active job, a 16-job pending queue, 10 seconds and a 32-entry/32-MiB result LRU; Vega allows four persistent sessions. Mermaid source is capped at 64 KiB and 500 edges. Vega specs are capped at 1 MiB, 10,000 inline rows, 4,096 CSS pixels per dimension and 4 megapixels. External data, images, patches, plugins and mark hyperlinks are rejected; Windows network patterns and macOS navigation policy add native request guards. SVG XML rejects DTD/entities and oversized output. The Markdown resource pipeline retains its 128-item/64-MiB/30-second bounds. Font catalog publication invalidates font-dependent results.

Vega and Vega-Lite fences use pinned embedded Vega 6.4.0, Vega-Lite 6.5.0, Vega-Embed 7.3.0 and the expression interpreter, with version and digest provenance plus upstream notices in `internal/diagramrender/assets/`. Static snapshots use the native WebView one-shot path; interactive preview uses up to four persistent native WebView sessions. Gio draws returned snapshots and dispatches pointer and modifier-wheel input outside layout. PDF freezes settled active charts and uses the initial offline snapshot for unopened charts; both render as static images in PDF. Specs are limited to inline data and reject external data/image/patch/plugin resources; mark hyperlinks are disabled. The local CSP blocks network, object/frame loads and dynamic evaluation, while allowing inline scripts containing the app's embedded renderer assets; Markdown content is not inserted as executable HTML or JavaScript.

Pictures retain logical geometry separately from their 2x raster pixels. Gio previews and the neutral PDF scene use that logical geometry. An unavailable diagram remains an explicit image placeholder/resource warning in PDF and an error plus source in preview. Existing math-failure behavior remains unchanged. PDF export uses accepted image bytes; it does not fetch/render a second time after confirmation.

## Observed evidence

- `TestRenderChineseFlowchart`, `TestRenderFlowchartFeatures`: TD/TB/LR, Chinese labels, edge labels, decisions, branches, subgraph and class styling rendered with real browser pixels.
- `TestRenderSVGTextAndForeignObject`, `TestSuppliedSVGWithForeignObjectLabels`: document SVG text and foreignObject, including the actual SVG referenced by `complex_markdown.md`, rendered through the image context.
- Local BOM/comment SVG and HTTP SVG with a misleading MIME/extension loaded successfully. Ordinary raster and malformed-image regressions passed.
- `TestRendererRejectsExternalRequests`: HTTP trap saw no SVG script/CSS/image/foreignObject fetches; Mermaid configuration relaxation was rejected. DTD/entities and bad XML were rejected. Oversized SVG and excessive graph edges were rejected by the actual renderer.
- Crash recovery, canceled requests, profile cleanup, bounded queue/cache and font-version invalidation passed. The real renderer package passed under `-race`.
- Existing macOS emoji regression plus Windows private-font 1x/2x pixel tests passed. Windows native execution is a separate outstanding check (see `windows-emoji-verification.md`).
- `TestMarkdownDiagramPreviewLogicalSize` proves that 2x pixels do not double layout geometry at 1x/2x display scales.
- `TestMarkdownDiagramPDFKeepsFrozenResources` rendered a Chinese Mermaid flowchart and local Chinese SVG, prepared the scene, closed the renderer and deleted the SVG, then exported and reopened the PDF successfully. There were exactly two image placements. Poppler reported a valid one-page A4 PDF without JavaScript; the actual rendered page was inspected for readable labels, arrows, placement and clipping.
- `testdata/flowchart.md` is an offline acceptance document with a real Mermaid fence and a local copy of the supplied SVG. The local SVG's original URL is `https://cdn.nlark.com/yuque/__mermaid_v3/ecd9b9e67824f36c6ab0266adc41a8ae.svg`. The frozen test fixture avoids treating upstream network availability as offline support.
- The existing acceptance-capture test ran from `dist/desktopui.test` next to its actual runtime sidecar, demonstrating application-relative runtime resolution, not PATH/browser fallback. Its PNG was inspected: Chinese nodes, diamonds, arrows and class/subgraph styling are visible.

## Reproduction

On macOS, run the native integration tests without a runtime override. On Windows, run them on a host with Edge or Chrome installed. `GLANCE_TEST_DIAGRAM_RUNTIME` is only a test fallback when no native WebView engine can be created; it points to an absolute staged headless-runtime directory and does not change production behavior:

```sh
GLANCE_TEST_DIAGRAM_RUNTIME="$PWD/dist/render-runtime" go test ./internal/diagramrender -race -count=1 -v
GLANCE_TEST_DIAGRAM_RUNTIME="$PWD/dist/render-runtime" go test ./internal/format/markdowndoc ./internal/format/pdf ./packaging/portable ./packaging/render-runtime -count=1
GLANCE_TEST_DIAGRAM_RUNTIME="$PWD/dist/render-runtime" go test ./internal/desktopui -run '^Test(WindowsEmoji|MarkdownDiagram|MarkdownComposition|UIFont|FontRendering|FontCollection)' -count=1 -v
```

That environment variable is read only by test helpers. Renderer tests explicitly requested with a path fail if the sidecar is incomplete. Without a working native engine or explicit fallback, tests requiring real browser pixels skip; a skip is not evidence that rendering passed. Gio pixel tests likewise require a usable GPU.

To capture the ordinary desktop preview path on a host with native WebView support, set `GLANCE_MARKDOWN_ACCEPTANCE_SOURCE` and an existing `GLANCE_MARKDOWN_CAPTURE` directory, then run `TestMarkdownAcceptanceVisualSnapshot`. `GLANCE_MARKDOWN_DIAGRAM_PDF` may name a new destination for the frozen-resource PDF acceptance test (existing PDFs are not overwritten).

## Platform/release limits

macOS arm64 WebKit rendering and GPU output were tested. Windows amd64 `internal/desktopui` cross-build passed; native Edge/Chrome interaction still needs a Windows host smoke test. Current-sandbox WebKit setup is also blocked by denied LaunchServices/font/WebKit extensions, so the earlier macOS interaction evidence could not be re-run here. HTTP SVG source images require network reachability; local SVG and Mermaid/Vega assets render offline. The renderer does not load external fonts/images referenced inside diagrams. First-release support claims cover the tested flowchart subset; arbitrary Mermaid diagram types and arbitrary SVG features are not claimed.

Earlier Mermaid/SVG verification recorded a `go test ./...` run that failed in `desktopui`, `format/wordpdf`, `localaccess`, `mcpserver` and `officeqa`. It repeated all 43 failures in `emoji-rendering-verification.md` and had one additional `TestInactiveWordTabReleasesLayoutEngine` failure; rerunning that test alone passed, so that extra failure did not reproduce. The full run occurred before the final FIFO and package-cache validation fixes. Focused `markdowndoc`, renderer, PDF, desktop, font and packaging checks passed after those fixes; the repository-wide suite is not green.

Vega verification on 2026-10-23: parser/resource-key tests, renderer validation and external-resource rejection, manifest digest/license provenance, fake-session forwarding and four-session limit, pointer validation, Markdown preview/PDF integration, and related application/PDF/emoji-font package tests passed. The targeted preview/PDF regressions also passed under `-race`. Review regressions cover blocked `href` channels and URI schemes, unique resource keys for repeated chart fences, bounded/coalesced input, hidden-chart input draining, pointer-leave image fallback, and explicit PDF failure when a hidden chart cannot settle. The native WebKit click-state and four-live-chart tests had passed during Task 3 (four sessions opened in 1.47 seconds). The current sandbox could not repeat native rendering: macOS denied LaunchServices, font and WebKit bootstrap sandbox extensions, and the real renderer test timed out in WebKit setup. Windows amd64 `go test -c` cross-builds passed with `CGO_ENABLED=0` for `internal/nativewebview`, `internal/diagramrender` and `internal/desktopui`; no Windows host was available for installed Edge/Chrome interaction. A repository-wide `go test -timeout=90s -p=1 ./...` attempt also surfaced unrelated locale, spreadsheet boundary/undo assertions and sandbox-denied local `httptest` listeners in `desktopui`, `diagnostics` and `fontdownload`; `diagramrender` hit the same WebKit setup timeout. The run was stopped after these blocking failures. Markdown PDF's bundled-font emoji fallback is separately covered by rendered-PDF pixel checks, including composite graphemes; emoji are visible images, not searchable text.

The final review found and fixed two edge cases: local Markdown resources now reject FIFOs before opening them, and package staging validates the required browser files in cached and copied runtime directories. Removing `icudtl.dat` from an isolated staged cache triggered a successful rebuild. The final packaging rebuild and review are recorded in the task results.
