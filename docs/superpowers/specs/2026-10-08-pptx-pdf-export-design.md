# PPTX PDF Export Through the Shared Preview

**Status:** Implemented; build and static analysis pass. Representative visual acceptance remains pending.

## Problem

The PowerPoint File ribbon's `导出 PDF` action currently calls `startSaveAsPicker("pdf")`. That is the PPTX Save As route, which validates the active presentation as `.pptx`; it does not open a PDF export preview or produce a PDF.

The shared PDF preview in `internal/desktopui/pdf_export_preview.go` currently accepts spreadsheet, Word, and Markdown scenes. The PPTX adapter exposes a neutral `document.PresentationSnapshot`, but the pinned `go-pptx` dependency's `render` package defines only an interface and has no renderer implementation. There is no PPTX-to-PDF writer in the application.

## Goal

Make File > Export PDF for a PPTX open the shared PDF export preview, show one page per slide, let the user choose A4 portrait or landscape, and produce a valid PDF from the current presentation snapshot without changing the PPTX source path or clearing its dirty state.

## Chosen approach

Use an application-local exporter based on the neutral presentation snapshot and the subset of PPTX content already represented by the application. Do not add a runtime dependency on LibreOffice or another Office application. Extend the existing shared preview rather than introducing a PPTX-specific modal.

Each projected slide becomes one A4 PDF page. Preserve the slide's aspect ratio and center it on the selected paper size without cropping. Default to landscape when the slide is wider than it is tall; otherwise default to portrait. The orientation control changes the paper page and fit scale, not the source slide dimensions or content.

The preview and export must use the same immutable presentation snapshot. The thumbnail should render the selected slide within the A4 paper preview, and the page count/navigation should correspond to the slides in that snapshot. The dialog must state that the PDF reflects Glance-Go's supported PPTX rendering subset and may omit unsupported Office features; it must not imply full PowerPoint fidelity.

## User flow and state

1. File > Export PDF flushes any active text draft. If flushing fails, export stops and reports the error.
2. The UI captures a presentation snapshot and its revision, then initializes the shared preview with format `presentation`, the slide list, initial page, orientation, and page count.
3. Orientation changes update the shared preview's paper fit. Page navigation selects a slide without changing the active slide in the editor.
4. Cancel closes the dialog without opening a picker or creating a file.
5. Confirm closes the preview and opens the existing asynchronous PDF save picker. The export worker writes from the captured snapshot, not from mutable UI state.
6. Before creating the destination, the exporter verifies that the originating presentation is still open and its revision matches the captured revision. It writes to a sibling temporary file, validates the generated PDF, then exclusively links the staged file to the new destination. An existing destination causes failure and is never replaced. Failure or cancellation leaves the destination and PPTX source untouched.

Export does not adopt the PDF path as the presentation path, mark the PPTX clean, or otherwise alter its saved/dirty state. Export errors and success use the existing status reporting conventions.

## Components

- `internal/desktopui/presentation_view.go`: route `file_export` to a presentation PDF export request.
- A focused presentation PDF export file: flush drafts, capture/validate the snapshot, initialize the shared preview, own picker requests and asynchronous export results, and apply completion status.
- `internal/desktopui/pdf_export_preview.go`: add the `presentation` format, retain its immutable snapshot, render slide thumbnails, and update orientation/page controls without changing the other formats' behavior.
- `internal/format/pdf`: add the presentation PDF writer using the existing pinned PDF dependency and atomic-file/commit-guard conventions.
- `internal/desktopui/app.go`, `document_session.go`, and `layout.go`: hold and dispatch only the minimal result channel, request, and cancellation state needed to keep generation off the Gio frame loop.
- `docs/office-support.md` and the compatibility matrix: document the implemented subset and known fidelity gaps after implementation.

## Compatibility boundary

The export operates on the neutral `PresentationSnapshot`, not arbitrary package internals. It can include slide order and dimensions, text runs, shapes and custom paths, supported fills, and embedded raster images when those values are present in the snapshot and supported by the exporter. It does not claim support for animations, transitions, audio/video playback, SmartArt, hidden-slide semantics, unsupported linked content, or other OOXML content absent from the current neutral snapshot/rendering path.

The preview must display a persistent, concise limitation notice. If the exporter encounters a projected element it cannot safely render, it should report the element/slide and avoid silently claiming a complete export. Unsupported content that the adapter did not project cannot be enumerated reliably; the notice must explain that limitation.

## Error handling and resource bounds

- Reject an empty or invalid snapshot before opening the save picker.
- Keep PDF rendering and writing off the Gio layout/frame loop.
- Honor cancellation and application shutdown while rendering slides.
- Preserve an existing destination until the complete PDF passes structural validation.
- Reject a stale snapshot before destination replacement and ask the user to export again.
- Bound output raster dimensions and total rendered pixel memory if the implementation rasterizes slides; process slides incrementally rather than retaining every full-resolution page image.

## Acceptance criteria

- File > Export PDF on a PPTX opens the shared `导出 PDF 预览` modal instead of the PPTX Save As flow.
- The modal shows the correct slide count, an actual slide preview, page navigation, and the selected A4 orientation; orientation changes preserve slide aspect ratio and do not crop content.
- Confirm opens the PDF save picker and writes a structurally valid, multi-page PDF with one page per projected slide.
- Cancel, picker cancellation, stale revision, rendering failure, and write failure do not corrupt or replace an existing destination and do not change the source PPTX path or dirty state.
- The preview communicates the supported-subset limitation.
- Existing spreadsheet, Word, and Markdown PDF previews continue to behave as before.

## Verification approach

The export path structurally reopens the generated PDF before replacing the destination and checks page count and page sizes against the selected orientation. Build the affected desktop UI and PDF packages during implementation. Automated test additions or execution are outside this request unless separately requested. Do not claim broader PowerPoint fidelity than the documented subset.
