# Repository Guidance

## Architecture

- Follow [docs/architecture.md](docs/architecture.md): Gio UI -> session/controller -> neutral document model -> format adapters -> local files.
- Keep third-party Office library types behind format adapters. UI widgets consume neutral view data and send intents through the controller.
- Keep the Gio window loop responsive. Perform parsing and saves outside layout/frame code and publish results through synchronized application state.
- Keep source files focused; split layout, event handling, state, and format operations by responsibility as they grow.

## Gio UI

- Persistent widgets (`widget.Editor`, `widget.List`, `widget.Clickable`) belong in stable state, not as throwaway values constructed on every frame.
- Set the axis of every `widget.List` explicitly. Editors in vertical list rows use the full available width; reset inherited minimum constraints for small badges/chips.
- Keep layout/drawing separate from event handling and controller operations. Normalize pointer coordinates at the input boundary before using them in drag or selection math.
- For Excel row-height/column-width dragging, reuse `../mysql-ai/docs/drag-patterns.md`: keep preview geometry in grid coordinates, keep the resize cursor active over the preview, expand preview bounds past the old grid edge, rebase anchors when coordinate spaces change, and commit dimensions only on release. Convert pixels to file-format units through one tested helper; do not persist pixels directly.
- Dialog drafts must receive the latest editor text before confirm/close. Track committed document values separately from transient editor drafts.
- For spreadsheet editing, preserve geometry while entering edit mode, make no-op commit a no-op, support cancel, and avoid committing intermediate IME composition text. Cover empty and styled cells and Chinese input in interaction regressions.
- See [docs/architecture.md](docs/architecture.md) and the sibling reference `../mysql-ai/docs/gio-form-layout-gotchas.md` for rationale and examples.
- The approved XLSX virtual-grid design is [docs/superpowers/specs/2026-09-28-virtualized-spreadsheet-grid-design.md](docs/superpowers/specs/2026-09-28-virtualized-spreadsheet-grid-design.md).

## Document Safety

- A format library's API surface is not a compatibility guarantee. Record selected dependencies and evidence in [docs/office-library-evaluation.md](docs/office-library-evaluation.md).
- Keep the implemented boundary and known gaps current in [docs/office-support.md](docs/office-support.md), especially when adding legacy formats, style rendering, or resize interactions.
- Do not enable overwrite for a file class until save/reopen tests show edited values and representative untouched content survive. Use a temporary sibling file and replace the source only after a successful write.
- Preserve dirty state on save failure; never silently drop unsupported content or claim formula recalculation/Word pagination without evidence.
- Maintain an evidence-based compatibility matrix as features land, following `../tools/docs/compatibility/office-feature-matrix.md`.

## Reference Implementations

- `../tools` is the reference for neutral document models, format adapters, session lifecycle, compatibility matrices, and round-trip save gates.
- `../mysql-ai` is the reference for Gio event/layout practices, persistent widget state, dialogs, and coordinate-space lessons. Reuse proven patterns; avoid copying its unrelated domain structure or growing a monolithic UI file.
