# PPTX Native Object Editing Design — Phase 1

## Goal

Add a native-feeling object manipulation loop to the existing Gio presentation canvas. Slide-local text, shape, and picture objects can be selected, moved, resized, and rotated; blank placeholder prompts remain display-only; geometry changes persist through PPTX save and workspace recovery.

This is phase 1 of the requested PPT editing work. Picture replacement/cropping and text/shape style editing remain follow-up phases with their own design and save-safety review.

## Current behavior

- Empty slide-local placeholders are projected as neutral text elements. Their localized prompt is synthesized by the UI and is not included in `PlainText()` or the PPTX package.
- Clicking a text element enters the persistent Gio editor. The prompt is hidden while the editor is active; text drafts commit when switching shapes/slides or saving.
- The canvas draws a selection outline and four decorative corner marks, but does not capture move/resize gestures or update document geometry.
- `PresentationElement.Bounds` is in points, and the neutral element already has a `RotationDeg` field. The adapter currently projects bounds but does not project rotation or expose bounds/rotation mutation.
- The pinned `go-pptx/v2 v2.0.3` API exposes shape bounds for reading, not setting. Its public shape API does not provide position, size, or rotation mutation.
- The local adapter serializes through go-pptx and validates the generated package before atomically replacing a file. New geometry edits must preserve the existing text-edit and file-replacement safety boundaries.

## Scope

### Included in phase 1

- Slide-local top-level text boxes, placeholders, auto-shapes (`p:sp`), and raster pictures (`p:pic`) with a supported preview, unique shape ID, and valid geometry.
- A selected-object state with a visible frame, eight resize handles, and a rotation handle.
- Moving supported objects by dragging their frame or interior.
- Resizing from each handle. Respect an object's explicit OOXML `noChangeAspect` lock when present; otherwise resize freely.
- Rotating text, shape, and picture objects through a handle above the selection frame. Rotation is continuous in the UI and stored in degrees in the neutral model.
- Preview-only transforms during a gesture. Commit the final transform once on pointer release; pointer cancel restores the original transform and does not change the document revision.
- Empty placeholder prompt remains synthesized UI content. A no-drag click on its prompt opens an empty editor and hides the prompt. If committed text remains empty, the prompt returns when editing ends. Typing real content never inserts or deletes prompt text.
- If a slide-local placeholder inherits bounds/rotation, materialize its own slide transform on the first committed move, resize, or rotation. Never modify its layout/master placeholder.
- Geometry serialization for normal save and recovery snapshots, with reopen validation before replacing an original file.
- Adapter and Gio regressions plus evidence updates to the Office support documents and compatibility matrix.

### Follow-up phases

- **Phase 2:** replace and crop slide-local pictures.
- **Phase 3:** edit text formatting and shape appearance (including fill/outline).

Each follow-up phase requires its own design review, scoped acceptance criteria, and package preservation evidence. This phase does not add controls that imply those operations are already supported.

### Excluded from phase 1

- Inherited decorative layout/master artwork, connectors, groups, tables, charts, audio/video, and unsupported/malformed shapes.
- Picture replacement, image crop editing, shape fill/outline changes, text font/size/color/emphasis/alignment controls.
- Shape insertion/deletion, multiple selection, z-order changes, snapping/guides, skew, flip controls, and a general undo/redo system.
- A claim of full PowerPoint rendering or interaction parity.

## Interaction design

1. A no-drag click on a supported object selects it. A selected text box enters text editing on a second click or Enter. A no-drag click on an empty placeholder prompt enters text editing immediately with an empty draft. Pointer movement beyond a 4 dp threshold starts a move gesture instead of a click.
2. A selected object shows its rotated frame, eight resize handles, and a rotation handle above the frame. The frame and handles are drawn in the object's local rectangle and rotated around its center.
3. Dragging the frame/interior moves the object. Dragging a resize handle changes its local width/height; the pointer delta is transformed into the object's unrotated local coordinates before changing bounds. Existing OOXML aspect-ratio lock metadata is honored.
4. Dragging the rotation handle previews a free, clockwise rotation around the object's center. Pointer cancel restores the initial angle; release commits the preview angle.
5. During all transforms, the view stores an original and preview transform. Geometry and rotation are committed to the adapter only on release. No-op releases do not dirty the document.
6. Objects may be positioned partly outside the slide, as in presentation software. Width and height must be finite and at least 8 pt. Rotation is normalized to `[0, 360)` degrees for neutral view state; the adapter serializes DrawingML's 60,000ths-of-a-degree integer.
7. Escape exits text editing using the existing draft-flush behavior and leaves the object selected. Save and shape-switch draft commit behavior remains unchanged.

## Architecture and data flow

```text
Gio pointer/key events
        ↓ normalized canvas coordinates
presentationViewState (selection + transient transform preview)
        ↓ final bounds + rotation on release
PPTX adapter UpdateSlideElementTransform(slide, shape, bounds, rotation)
        ↓ neutral snapshot / geometry overrides / dirty revision
go-pptx serialization + targeted slide XML transform patch
        ↓
save validation / workspace snapshot bytes
```

### UI state and transform math

- Keep selected shape ID, active move/resize/rotation gesture, original transform, preview transform, active handle, and press anchor in persistent presentation view state.
- Treat `PresentationElement.Bounds` and `RotationDeg` as committed document state. Project each object's `noChangeAspect` setting to a neutral `AspectRatioLocked` flag; layout reads the preview transform only for the active element.
- Bounds use slide points (`1 pt = 1/72 inch`); `RotationDeg` uses degrees. Convert device pixels using current `canvasScale` at the pointer boundary, then serialize points through the adapter's `emuPerPoint` constant.
- Normalize pointer coordinates once at the input boundary. For resize and rotation, use inverse rotation around the element center to map pointer positions into local coordinates. Translation uses the normalized slide-coordinate delta.
- Draw the whole element (shape path, fill, text, or picture) under one rotation transform. Apply the same transform when drawing the selection frame and computing pointer hit tests.
- Render committed rotation consistently in the main canvas, slide thumbnails, and slideshow. Only the main canvas displays selection handles and accepts object-transform gestures.
- Do not mutate document geometry during layout. Gesture state previews updates; release commits once; cancel discards the preview.
- Register hit targets for supported visible slide-local objects and their handles. Make handle hit bounds larger than the painted handles.

### Adapter mutation and serialization

- Add a format-neutral adapter operation that accepts `(slideIndex, shapeID, PresentationRect, rotationDeg)`. It validates slide/ID/kind, finite geometry, minimum dimensions, and finite rotation before mutating.
- Store committed transform overrides in the adapter, keyed by slide index and slide-local shape ID. Snapshot projection applies overrides after source/inherited geometry. Add/delete slide operations keep override indexes aligned with presentation slide order.
- A no-op transform does not mark dirty or increment revision. A real transform updates the override, snapshot, dirty flag, and revision once.
- Since go-pptx has no public geometry setter, keep mutation inside `internal/format/pptx`. Resolve the slide part through `presentation.xml` relationship order, then patch only the target shape identified by its owning slide and `cNvPr@id`.
- Update only `a:off`, `a:ext`, and the `a:xfrm@rot` value. Preserve flip/lock attributes, unknown attributes/children, placeholder metadata, and unrelated XML. For an inherited placeholder with no local `a:xfrm`, create one in its slide-local `p:spPr` using the effective layout/master transform plus the requested change.
- Require a unique slide and shape match. Malformed XML, ambiguous IDs, unsupported object kinds, or invalid geometry fail without changing the committed transform.
- Apply pending transform overrides to both file-save output and `SnapshotBytes()` so workspace recovery retains movement, resizing, and rotation.
- Serialize through a sibling temporary package, apply targeted XML changes, reopen it through the adapter, verify changed transforms and text, then atomically replace the target. Any failure preserves the original file and dirty state.

### Placeholder display

- Continue generating localized prompt paragraphs only in the UI projection layer.
- The prompt is visible when committed text is empty and the element is not being edited. The editor receives an empty draft for an empty placeholder.
- Save/reopen must prove prompt labels never become slide text and user-entered placeholder text persists.

## Files and responsibilities

- `internal/desktopui/presentation_state.go`: persistent selection and transform gesture state.
- `internal/desktopui/presentation_canvas.go` and a focused presentation interaction file: selection frame, handles, normalized gestures, and preview.
- `internal/desktopui/presentation_shape_render.go`, `internal/desktopui/presentation_thumbnail.go`, and `internal/desktopui/presentation_view.go`: apply committed `RotationDeg` consistently to shapes, text, and pictures in canvas, thumbnails, and slideshow.
- `internal/desktopui/presentation_editing.go`: synchronize text drafts when entering/leaving object selection and geometry gestures.
- `internal/document/presentation.go`: reuse `PresentationRect` and existing `RotationDeg`; add neutral `AspectRatioLocked` state for explicit OOXML resize constraints.
- `internal/format/pptx/document.go` and a focused `geometry.go` helper: project rotation/lock state, validate/update slide-local transforms, retain overrides, and patch slide XML for save and snapshots.
- `internal/format/pptx/document_test.go` and `internal/format/pptx/inherited_test.go`: transform mutation, materialized placeholder transform, no-op/error, save/reopen, and ownership coverage.
- `internal/desktopui/presentation_workspace_test.go` and/or a focused interaction test file: placeholder/edit behavior, selection, movement, eight resize handles, rotation, preview/release/cancel, and scale coverage.
- `docs/office-support.md`, `docs/office-library-evaluation.md`, and `docs/compatibility/office-feature-matrix.md`: record only the verified phase-1 subset and its remaining boundaries.

## Error handling and compatibility

- Reject NaN, infinity, non-positive size, dimensions below 8 pt, and invalid rotation before changing adapter state.
- Allow negative X/Y and right/bottom overflow so part of an object may leave the slide edge.
- Preserve picture/shape aspect-lock metadata; do not silently remove it during a transform. Objects without an explicit lock resize freely.
- Treat stale shape IDs and unsupported object structures as operation errors; never fall back to a different shape.
- Preserve dirty state after a failed save and leave the original file unchanged.
- Keep third-party and DrawingML package types behind `internal/format/pptx`.
- Do not broaden the PPTX overwrite claim for transforms until save/reopen evidence confirms geometry, rotation, and representative untouched slide content survive.

## Acceptance criteria

- A text box, supported auto-shape, or picture can be selected, moved, resized from eight handles, and rotated. Text placeholders and ordinary text boxes retain text-edit behavior.
- Transforms are correct at supported zoom and device scales, including local-coordinate resize for already-rotated elements.
- Active transforms are preview-only until pointer release. Cancel restores the original transform and creates no document revision.
- Picture/shape aspect-lock metadata is respected. Existing flip flags and other transform attributes survive editing.
- Direct slide-local transforms are updated. A placeholder without a local transform receives one on the slide and leaves layout/master parts unchanged.
- Save and workspace recovery retain the changed geometry and rotation. Reopening confirms the target transform and text while representative untouched slide content and relationships remain intact.
- Empty placeholder prompts remain display-only; user-entered content persists after save/reopen.
- Unsupported, ambiguous, or malformed edits fail safely without corrupting the package, dropping dirty state, or replacing the original file.
- Office support docs describe the exact supported object kinds and do not imply image crop/replacement, style editing, or full PowerPoint parity.

## Verification plan

- Adapter regressions use compact PPTX packages for shape/picture/text transforms, direct and inherited `a:xfrm`, rotation units/direction, aspect-lock/flip preservation, malformed/missing transforms, ambiguous IDs, bounds validation, no-op revision semantics, and target-only XML changes.
- Save/reopen regressions verify move/resize/rotate, blank/non-empty placeholders, representative untouched shapes and package parts, workspace snapshot recovery, and unchanged source bytes on failure.
- Gio interaction/render regressions cover selection/edit entry, movement, all eight handles, rotation, drag threshold, inverse-rotation resize math, pointer cancel/release, Chinese typing into empty placeholders, committed rotation in canvas/thumbnails/slideshow, and 1×/1.5×/2× scale.
- Run focused package tests and the full Go suite after implementation, then exercise disposable copies of supplied PPTX samples through canvas edit/save/reopen. Never use a sample source as the write target.
