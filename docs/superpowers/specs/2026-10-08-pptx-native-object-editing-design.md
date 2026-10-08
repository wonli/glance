# PPTX Native Object Editing Design

## Goal

Give slide-local text placeholders and text boxes a PowerPoint-like editing loop: placeholder prompts stay out of document text, objects can be selected and moved, and their frames can be resized with visible handles. Persist changed geometry through the PPTX adapter so save, reopen, and workspace recovery retain the edits.

## Current behavior

- Empty slide-local placeholders are projected as neutral text elements. Their localized prompt is synthesized by the UI and is not included in `PlainText()` or the PPTX package.
- Clicking a text element enters the persistent Gio editor. The prompt is hidden while the editor is active; text drafts commit when switching shapes/slides or saving.
- The canvas draws a selection outline and four decorative corner marks, but does not capture move/resize gestures or update document geometry.
- `PresentationElement.Bounds` already represents slide geometry in points. The adapter projects shape bounds and inherited placeholder geometry, but has no public geometry mutation operation.
- The pinned `go-pptx/v2 v2.0.3` API exposes shape bounds for reading, not setting. Its current `Shape` interface does not provide position/size mutation.
- The local adapter serializes through go-pptx and validates the generated package before atomically replacing a file. Geometry support must preserve that save boundary and the existing text-edit boundary.

## Scope

### Included

- Slide-local text elements represented as `ElementKindText`, including empty placeholders and ordinary text boxes.
- A selected-object state with a visible frame and eight resize handles.
- Moving the selected object by dragging its frame or interior when the gesture crosses the drag threshold.
- Resizing from any of the eight handles, with a positive minimum frame size. Text-box aspect ratio is not locked.
- Preview-only geometry during an active gesture; commit the new geometry once on pointer release. Pointer cancel restores the original geometry.
- Empty placeholder prompt remains synthesized UI content. Clicking an empty placeholder opens an empty editor and hides the prompt. If the committed value is still empty, the prompt is shown again after editing ends. Typing real content replaces the visual prompt without first inserting or deleting prompt text.
- Explicit local slide transforms when a placeholder currently inherits geometry from its layout or master. Layout/master artwork remains read-only and is never modified.
- Geometry serialization for normal save and recovery snapshots, followed by reopen validation before an original file is replaced.
- Focused adapter and Gio regressions and evidence updates to the Office support documents and compatibility matrix.

### Excluded

- Moving/resizing pictures, non-text shapes, connectors, tables, charts, groups, or inherited layout/master artwork.
- Shape insertion/deletion, rotation, multi-selection, alignment tools, aspect-ratio locking, snapping, guides, z-order changes, or a general undo/redo system.
- Reflow or typography fidelity beyond the existing text renderer and supported text-editing subset.
- A claim of PowerPoint rendering or interaction parity.

## Interaction design

1. A no-drag click on a non-empty text box selects it. Clicking the selected text box again or pressing Enter enters in-place text editing. Pointer movement beyond a 4 dp threshold starts a move gesture instead of a click.
2. A no-drag click on an empty placeholder prompt enters text editing immediately. The editor starts empty, and the prompt is not included in the editable draft. Escape ends editing and returns to the selected-object frame using the existing draft-flush behavior; save and shape-switch commit behavior remains unchanged.
3. A selected object shows a frame and eight handles (four corners and four edge centers). Pressing a handle starts a resize preview. Pressing the frame or interior starts a move preview.
4. During movement or resize, the preview is shown in slide coordinates. Position and size are converted to points through one named helper. The adapter receives only the final rectangle on release; pointer cancellation leaves the document unchanged.
5. Objects may be positioned partly outside the slide bounds, as in presentation software. Width and height must remain finite and at least 8 pt.
6. The selected frame and handles remain aligned at the current canvas zoom and device scale. Pointer coordinates are normalized at the input boundary before conversion to slide points.

## Architecture and data flow

```text
Gio pointer/key events
        ↓ normalized canvas coordinates
presentationViewState (selection + transient geometry preview)
        ↓ final PresentationRect on release
PPTX adapter UpdateSlideElementBounds(slide, element, bounds)
        ↓ neutral model mutation / dirty revision
go-pptx serialization + targeted slide XML geometry patch
        ↓
save validation / workspace snapshot bytes
```

### UI state and geometry

- Keep selection, active move/resize session, original rectangle, preview rectangle, and active handle in persistent presentation view state.
- Treat `PresentationElement.Bounds` as committed document state. Layout reads the preview rectangle only for the element participating in an active pointer gesture.
- Use points as the neutral geometry unit (`1 pt = 1/72 inch`). Convert to/from device pixels using the current `canvasScale` at the pointer boundary; convert points to DrawingML EMUs with the adapter's single `emuPerPoint` constant.
- Do not alter element geometry while laying out a frame. A gesture starts on press, updates transient preview during drag, commits on release, and discards preview on cancel.
- Register pointer hit regions for visible text objects and their handles only. Keep handle bounds sufficiently large for use while drawing compact visual handles.

### Adapter mutation and serialization

- Add a format-neutral adapter operation that accepts `(slideIndex, shapeID, PresentationRect)` and validates the slide, element kind, finite values, and minimum dimensions before mutating.
- A geometry no-op does not mark the document dirty or increment its revision. A real change updates the neutral snapshot, dirty flag, and revision once.
- Store committed geometry overrides in the adapter, keyed by slide index and slide-local shape ID. Snapshot projection applies those overrides after source/inherited geometry. Add/delete slide operations keep override indexes aligned with the presentation's slide order.
- Since go-pptx has no public bounds setter, keep geometry mutation inside `internal/format/pptx`. Resolve the slide part through presentation relationship order, then patch only the target shape's DrawingML transform identified by its owning slide and shape ID. Update only `a:off` and `a:ext`; preserve other transform attributes and unrelated XML.
- For a placeholder without a slide-local transform, materialize `a:xfrm` within its slide-local `p:spPr`; never patch the matched layout or master placeholder.
- Require a unique slide and shape match. If XML is malformed, the target is ambiguous, or the element kind is unsupported, return an error and preserve the dirty state and original target file.
- Apply pending geometry patches to both file-save output and `SnapshotBytes()` so workspace recovery does not lose movement or resizing.
- Reopen the patched temporary package and verify the edited geometry before atomic replacement. Failed patching, validation, or replacement must leave the original file intact.

### Placeholder display

- Continue generating localized prompt paragraphs only in the UI projection layer.
- The prompt is visible only when committed text is empty and the element is not being edited. The in-place editor receives an empty draft for an empty placeholder.
- Save/reopen checks must confirm that prompt labels never appear in slide text and that user-entered placeholder text persists.

## Files and responsibilities

- `internal/desktopui/presentation_state.go`: persistent selection and geometry-gesture state.
- `internal/desktopui/presentation_canvas.go` and a focused presentation interaction file: selected frame/handles, pointer capture, normalized drag math, and preview rendering.
- `internal/desktopui/presentation_editing.go`: ensure text drafts are synchronized before switching between edit and object-selection states.
- `internal/document/presentation.go`: reuse `PresentationRect`; add a small neutral mutation request type only if needed by the adapter boundary.
- `internal/format/pptx/document.go` and a focused `geometry.go` helper: validate/update slide-local element bounds, keep revision/dirty semantics, project stored overrides, and patch slide XML for both save and snapshot serialization.
- `internal/format/pptx/document_test.go` and `internal/format/pptx/inherited_test.go`: geometry mutation, transform materialization, no-op/error, save/reopen, and placeholder ownership coverage.
- `internal/desktopui/presentation_workspace_test.go` and/or a focused interaction test file: prompt behavior, selection, move/resize preview, release/cancel, scale conversion, and save/reopen interaction coverage.
- `docs/office-support.md`, `docs/office-library-evaluation.md`, and `docs/compatibility/office-feature-matrix.md`: record the newly supported operation and its evidence-based boundaries.

## Error handling and compatibility

- Reject NaN, infinity, zero/negative size, and dimensions below the minimum before changing adapter state.
- Allow negative X/Y and right/bottom overflow so users can place part of an object beyond the slide edge.
- Treat an invalid or stale shape ID as an operation error; do not silently move a different object.
- Preserve dirty state after a failed save and leave the original file unchanged.
- Keep all third-party types and DrawingML package operations behind `internal/format/pptx`.
- Do not broaden the existing PPTX overwrite claim until saved edits reopen with the intended geometry and representative untouched slide content remains intact.

## Acceptance criteria

- Empty placeholder prompts are display-only. They disappear when an empty placeholder enters editing; the editor begins empty; committed non-empty user text remains after save/reopen; empty committed text shows the prompt again.
- A text box can be selected, dragged, and resized from each of eight handles. Movement and resizing use correct slide-point geometry at supported zoom/device scales.
- Active geometry is only a preview until pointer release. Cancel restores the original frame and creates no document revision.
- Direct slide-local transforms are updated; inherited placeholder transforms are materialized locally without changing layout/master parts.
- Save and workspace recovery preserve changed geometry. Save/reopen confirms the target geometry and text, and representative untouched slide content/relationships remain intact.
- Unsupported/ambiguous/malformed geometry edits fail without corrupting the package, dropping dirty state, or replacing the original file.
- Office support documentation describes the exact implemented text-element subset and does not claim full PowerPoint parity.

## Verification plan

- Adapter regressions use minimal PPTX packages for direct transforms, inherited placeholder geometry, malformed/missing transforms, ambiguous IDs, bounds validation, no-op revision semantics, and target-only XML changes.
- Save/reopen regressions verify moved and resized objects, blank/non-empty placeholders, representative untouched shapes and package parts, snapshot recovery, and unchanged source bytes on failure.
- Gio interaction regressions cover selection/edit entry, all eight resize handles, movement, drag threshold, pointer cancel/release, Chinese typing into empty placeholders, and geometry at 1×/1.5×/2× scale.
- Run the focused package tests and complete Go suite after implementation, then exercise a disposable copy of a supplied PPTX sample through the actual canvas and save path. Never use a sample source as the write target during acceptance.
