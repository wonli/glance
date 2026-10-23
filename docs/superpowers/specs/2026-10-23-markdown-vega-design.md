# Interactive Vega support in Markdown

**Status:** Proposed for review
**Date:** 2026-10-23

## Intent

Add Vega and Vega-Lite chart fences to Markdown preview while preserving Vega's
interactive value. Preview charts run in a persistent, app-owned WebView session.
Gio continues to own document layout and scrolling; pointer input is forwarded
to the chart session and refreshed snapshots are drawn in the chart's existing
layout slot. Markdown PDF export receives a static snapshot of each chart.

The implementation uses the operating system browser engine already selected by
`internal/nativewebview`; it does not package another browser or download assets
at application runtime. Supported hosts remain macOS WebKit and Windows Edge or
Chrome, as currently implemented. Other operating systems keep Vega fences as
literal code until a native renderer exists there.

## Goals

- Recognize fenced blocks tagged `vega` and `vega-lite` (case-insensitive) and
  retain the language in neutral Markdown view data.
- Render those specifications as interactive charts in Preview and Split modes.
- Support common Vega interactions driven by pointer movement, click, drag and
  modifier+wheel events, including hover tooltips and signal/selection updates.
  Normal wheel input continues scrolling the Markdown document. Keep event
  handling, rendering and snapshot capture outside Gio layout/frame callbacks.
- Export static chart snapshots to PDF through the existing frozen-resource
  pipeline. Capture the last settled chart state, excluding transient hover UI.
- Bundle pinned Vega, Vega-Lite, Vega-Embed and any required interpreter assets
  locally, with upstream licenses and version/digest metadata.
- Keep ordinary `json`, `javascript` and other fenced blocks as code.

## Non-goals

- Execute HTML or JavaScript embedded in Markdown or allow user-provided script
  blocks.
- Load remote data, images, patches or plugins referenced by a Vega spec in the
  first version. Support inline `data.values`; local external data can follow via
  a separately bounded Go resource loader.
- Open the Vega editor or external URLs from chart actions.
- Make PDF charts interactive. PDF output is a static image snapshot.
- Forward keyboard-only interaction or custom HTML form controls in the first
  version.
- Implement native WebView hosts for Linux in this change.
- Guarantee every browser-specific custom binding. First-version interaction
  focuses on pointer-driven Vega signals, selections and tooltips; custom HTML
  form bindings and keyboard-only interactions are outside the initial target.

## Approaches

1. **Persistent offscreen WebView sessions with Gio input forwarding — proposed.**
   Keep chart rendering in a per-chart WebView session. Gio renders its latest
   captured image and forwards pointer events from the stable chart hit region.
   This keeps the current Markdown column, list scrolling and PDF resource model.
   It requires persistent browser state, event-coordinate normalization,
   asynchronous snapshot refresh and bounded session lifecycle management. It
   also cannot promise browser-native accessibility or arbitrary HTML control
   behavior.
2. **Native WebView child views over the Gio window.** This would provide direct
   browser input and native accessibility, but Gio does not expose a shared
   cross-platform child-view host in the current adapter. macOS WKWebView and a
   Windows WebView2 host would need separate window integration, clipping,
   scrolling, focus and lifecycle work. This is a larger platform project.
3. **Static offscreen rendering only.** This reuses the existing screenshot API
   and is the least work, but it discards the interactive behavior that motivated
   Vega support, so it does not satisfy the requested outcome.

The proposed call is approach 1: preserve the Gio preview surface and deliver
pointer-driven interactivity with the existing OS browser engines. If
implementation spikes show that event forwarding cannot meet expected chart
behavior or latency, stop and return with evidence before switching to approach
2; do not silently ship static-only rendering as Vega support.

## Design

### Markdown model and resource identity

The format adapter recognizes the two language tags on fenced blocks and stores
the language alongside the source in neutral document data. `DiagramSpan` and
`ResourceKey` preserve that language so identical JSON text tagged as Vega and
Vega-Lite cannot collide. No browser or JavaScript types cross into the neutral
document package.

The Markdown preview keeps a stable chart state per block identity and document
generation. A chart state owns its input tag, last accepted image, pending event
job and WebView session handle. Source edits, document switches and view teardown
cancel pending jobs and close sessions. Results publish only when their document
generation and chart identity still match.

### WebView rendering and input

The WebView page loads only locally embedded, pinned vendor assets. It parses
the fence body as JSON, selects Vega or Vega-Lite mode from the retained language,
creates the Vega View in an SVG renderer and applies a fixed application theme.
Vega actions are disabled. A custom data/image loader rejects external URLs;
initial support accepts inline values only. Vega expressions always use the
bundled expression interpreter; do not fall back to `unsafe-eval`. If a
specification cannot run under the interpreter, show a diagnostic and retain its
source code.

Each live chart has a persistent WebView session so selections and signals do
not reset after every event. Gio pointer coordinates are normalized at the input
boundary into chart-local CSS coordinates. Move events are coalesced and
snapshot refreshes are rate-limited; press, release, drag and leave events retain
their boundaries. Normal wheel input stays with the Markdown list; Shift- or
Control-wheel over a chart is forwarded to Vega for wheel-bound signals. Chart
work runs on managed background jobs, never in layout. The last accepted
snapshot stays visible while a new event is processed.

The renderer applies explicit input byte, inline row-count, render-time,
dimension, pixel, cache and live-session limits. Failure leaves a readable code
fallback and a localized preview diagnostic. No network fallback or runtime
asset download is allowed.

### PDF export

PDF preparation captures each chart's settled current view on a background job,
dispatches pointer-leave to remove transient hover UI, and freezes the resulting
snapshot into the existing Markdown resource map. Vega selections and zoom state
remain as settled by prior user input. If a chart session has not been opened
yet, the same offline renderer creates its initial static view. Existing revision
checks still reject stale exports. PDF layout treats the result as an ordinary
diagram image; vector-PDF preservation is not part of this change.

## Validation

- Parser tests cover both language tags, mixed-case tags, source preservation and
  literal handling of `json`, nested Markdown examples and indented code.
- Renderer tests cover valid Vega and Vega-Lite examples, visible output,
  tooltip/selection interaction after forwarded input, bounds, invalid JSON,
  blocked external URLs and cancellation.
- Gio interaction tests cover chart-local coordinate mapping, pointer press/move/
  release/wheel, event coalescing, stale result rejection, scroll handoff and
  session cleanup on edit, document switch and close.
- PDF tests verify frozen chart snapshots are present and that export does not
  mutate the Markdown source or depend on a live view after preparation.
- Run package-level parser, renderer, preview and PDF tests; verify macOS WebKit
  rendering and the Windows Edge/Chrome build. Native Windows interaction
  evidence must be reported separately if no Windows host is available.
- Run `git diff --check` and verify license/version/digest metadata for all
  bundled JavaScript assets.

## Open implementation checks

- Confirm that synthetic/forwarded pointer events activate common Vega signal
  handlers on both WebKit and Chromium. If browser semantics differ, add a
  platform-specific event adapter behind `nativewebview` without changing the
  document model.
- Choose measured defaults for the maximum live chart count and event refresh
  rate after testing representative multi-chart documents.
- Keep the current Windows host requirement visible: it locates installed Edge
  or Chrome rather than creating an embedded WebView2 control. Do not add an
  installer requirement without a separate product decision.
