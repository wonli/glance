# PDF workspace implementation plan

> Execute inline with superpowers:executing-plans and test-driven-development.

Goal: Implement Pen wLKrt as a functional local PDF workspace, with one revisioned document shared by viewing, editing, thumbnails, history, recovery and saving.

Architecture: The PDF adapter owns the canonical package/object graph; each successful command atomically publishes a new validated package revision. Neutral page snapshots and raster scenes are derived from that revision, never independent editable copies. UI modes change tools only. Existing unknown objects are retained through incremental updates. Gio receives asynchronous immutable page results.

Design: Approved conversation (2026-10-07), Pen node wLKrt in pencil-new.pen: 54dp toolbar, 220dp thumbnail sidebar, gray canvas, 300dp annotations panel, 34dp status bar. Actual document content replaces design placeholder content and author names. Read/annotation/signature tools share one scene; native text edits are constrained to supported runs. Signature means ink, not digital certification. Unsupported native fonts and encrypted documents must fail clearly.

## Tasks
- [x] Adapter tests: edit/view/snapshot/reopen equivalence, undo/redo, no-op, stale commands, failed save, retained other pages and annotations.
- [x] Implement neutral PDF DTOs and revisioned pdfdoc adapter with read/render, text replacement, notes/highlight/ink, rotate, history, atomic save.
- [x] Integrate PDF open, file filters, shared sessions, recovery, save-as, dirty/close lifecycle.
- [x] Implement Pen toolbar, real thumbnails, page canvas, text selection/edit dialog, search, annotations, ink/signature, undo/redo and native print from current snapshot. Use background workers; reject stale outputs and bound raster caches.
- [x] Run adapter/UI regressions, full Go tests and race checks, inspect Gio headless screenshots at desktop sizes. Document remaining compatibility limits.

## Review focus
- Read and edit must render the same revision; stale workers cannot repaint after undo.
- Invalid/font-unsupported edits preserve bytes, dirty state and active draft.
- Rotated pages use consistent pointer coordinates; no hidden duplicate text updates.
- Existing package structures and untouched pages survive updates.
- No parsing/rendering/saving within Gio layout callbacks; bounded page cache and worker concurrency.


## Acceptance evidence
- Full `go test ./...` passed; focused PDF race checks passed.
- Captured and inspected 1440×900 and 800×900 Gio frames, plus the user's local Chinese `测试word文档.pdf` and 15-page `大猫.pdf` without changing originals.
- Visual inspection caught and corrected unbounded panel painting, oversized annotation cards, and native renderer scaling. Added canvas/page pixel and native 1×/2× geometry assertions.
- Review findings addressed: stale/mixed revisions, shared-window invalidation, save-status refresh, horizontal page scrolling, background dirty checks and failed/busy-dialog draft retention.
- Compatibility boundaries are in `docs/office-support.md`: constrained native text-run edits, no OCR/reflow, handwritten rather than digital signatures, no edits to signed documents; native physical printing remains untested.
- Independent review produced useful findings but did not finish because its agent hit a usage limit. Final code was inspected and tested in the main task.
