# PPTX Native Object Editing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users select, move, resize, and rotate slide-local text, auto-shape, and raster-picture objects while keeping placeholder prompts display-only and preserving transforms through PPTX saves and workspace recovery.

**Architecture:** Keep pointer preview and selection state in Gio, represent committed geometry in the neutral presentation model, and persist geometry/rotation inside the PPTX adapter. Since go-pptx v2.0.3 has no public geometry setter, apply validated, shape-ID-targeted DrawingML transform patches after serialization for both file saves and recovery snapshots.

**Tech Stack:** Go, Gio v0.10.2, `github.com/F31/go-pptx/v2 v2.0.3`, ZIP/XML package rewriting.

**Spec:** `docs/superpowers/specs/2026-10-08-pptx-native-object-editing-design.md`

## Global Constraints

- Keep third-party Office library and OOXML package types behind `internal/format/pptx`.
- Bounds use points (`1 pt = 1/72 inch`); DrawingML geometry uses EMUs and rotation uses 60,000ths of a degree.
- Commit geometry only on pointer release; pointer cancel must restore the original transform without changing document revision.
- Minimum width and height are 8 pt; negative X/Y and right/bottom overflow are allowed.
- Inherited layout/master artwork stays read-only; inherited placeholder transforms materialize only on the slide being edited.
- Respect explicit OOXML `noChangeAspect` metadata and preserve flip/lock attributes when patching transforms.
- File save and `SnapshotBytes()` must apply the same committed transform overrides.
- Failed or ambiguous edits preserve dirty state and the original file. Do not broaden PPTX overwrite claims before save/reopen preservation evidence exists.
- Image replacement/cropping and text/shape style editing are follow-up phases, not part of this plan.

## Review Focus

- Resize math for an already rotated object must operate in local coordinates — `TestPresentationResizeUsesLocalCoordinates` in Task 2.
- A placeholder inheriting geometry/rotation from a layout or master must create a slide-local transform without changing inherited parts — `TestInheritedPlaceholderTransformMaterializesLocally` in Task 1.
- Aspect-ratio locks and flip flags must survive both resize and rotation — `TestTransformPatchPreservesLocksAndFlips` in Task 1.
- Pointer cancellation and no-op releases must not commit or increment revision — `TestPresentationTransformCancelAndNoopRelease` in Task 3.
- Malformed XML, missing/ambiguous shape IDs, and failed writes must not replace the original package — `TestTransformSaveFailurePreservesOriginal` in Task 1.

---

### Task 1: Neutral Transform State and PPTX Adapter Persistence

**Files:**
- Modify: `internal/document/presentation.go`
- Modify: `internal/format/pptx/document.go`
- Create: `internal/format/pptx/geometry.go`
- Modify: `internal/format/pptx/document_test.go`
- Modify: `internal/format/pptx/inherited_test.go`

**Interfaces:**
- Produces: `PresentationElement.AspectRatioLocked bool` and `(*Document).UpdateSlideElementTransform(slideIndex int, shapeID int64, bounds document.PresentationRect, rotationDeg float64) error`.
- Produces: private `presentationTransformOverride{Bounds document.PresentationRect, RotationDeg float64}` stored by slide index and shape ID; `Snapshot()` applies it after source/inherited projection.
- Produces: private package patch helper `patchPresentationTransforms(sourcePath, targetPath string, overrides []map[uint32]presentationTransformOverride) error`, used by both file save and recovery snapshot serialization.

- [ ] **Step 1: Add adapter regressions for transform mutation and save/reopen**

Add tests named `TestUpdateSlideElementTransformPersistsBoundsAndRotation`, `TestUpdateSlideElementTransformNoopPreservesRevision`, `TestUpdateSlideElementTransformRejectsInvalidGeometry`, `TestInheritedPlaceholderTransformMaterializesLocally`, `TestTransformSnapshotBytesRetainsGeometry`, `TestTransformPatchPreservesLocksAndFlips`, and `TestTransformSaveFailurePreservesOriginal`.

Assert point-to-EMU round trips, rotation projection, 8 pt minimum dimensions, dirty/revision behavior, unique shape targeting, a slide-local placeholder transform, unchanged inherited layout/master parts, recovery snapshot reopen, preserved unrelated package entries, and source-byte preservation on failure.

- [ ] **Step 2: Run the focused adapter regressions to confirm they fail for the missing behavior**

Run: `go test ./internal/format/pptx -run 'TestUpdateSlideElementTransform|TestInheritedPlaceholderTransformMaterializesLocally|TestTransform(SnapshotBytes|PatchPreservesLocksAndFlips|SaveFailurePreservesOriginal)' -count=1`

Expected: failures identify missing transform projection/mutation/persistence behavior; fixture/setup errors are fixed before implementation.

- [ ] **Step 3: Add neutral transform metadata and adapter override state**

Add `AspectRatioLocked bool` to `document.PresentationElement`. In `pptx.Document`, retain slide-local transform overrides and inherited placeholder transforms (bounds plus rotation); initialize and shift these caches with document creation, slide addition, and deletion. Project DrawingML rotation and explicit `noChangeAspect` values into neutral elements, then apply geometry overrides last.

- [ ] **Step 4: Implement `UpdateSlideElementTransform` in `internal/format/pptx/document.go`**

Validate the slide index, shape ID, supported slide-local object kind (`ElementKindText`, `ElementKindShape`, or `ElementKindPicture`), finite bounds/rotation, and minimum dimensions. A no-op returns without changing dirty/revision state; a real update stores one override and increments revision once.

- [ ] **Step 5: Implement targeted transform XML patching in `internal/format/pptx/geometry.go`**

Resolve slide part names from presentation relationship order. Patch only the target `p:sp`/`p:pic` identified by `cNvPr@id`; update `a:off`, `a:ext`, and `a:xfrm@rot`, preserving lock/flip/unknown XML. Materialize `a:xfrm` inside the slide-local placeholder's `p:spPr` when needed, using effective inherited geometry as its starting transform.

- [ ] **Step 6: Apply transforms to save and recovery serialization**

Route `saveAtomicLocked` and `SnapshotBytes()` through the same transform patch helper. Reopen patched output through the adapter and compare every overridden transform before replacement. Write patched output to a sibling temporary package; only replace the original after validation succeeds.

- [ ] **Step 7: Run adapter regressions and commit**

Run: `go test ./internal/format/pptx -run 'TestUpdateSlideElementTransform|TestInheritedPlaceholderTransformMaterializesLocally|TestTransform(SnapshotBytes|PatchPreservesLocksAndFlips|SaveFailurePreservesOriginal)' -count=1`

Expected: PASS, with no source replacement on a rejected or invalid transform.

Commit: `feat: persist PPTX object transforms`

---

### Task 2: Transform Preview Math and Rotation Rendering

**Files:**
- Create: `internal/desktopui/presentation_transform.go`
- Create: `internal/desktopui/presentation_transform_test.go`
- Modify: `internal/desktopui/presentation_shape_render.go`
- Modify: `internal/desktopui/presentation_thumbnail.go`
- Modify: `internal/desktopui/presentation_view.go`

**Interfaces:**
- Consumes: `document.PresentationElement.Bounds`, `RotationDeg`, and `AspectRatioLocked` from Task 1.
- Produces: private `presentationTransform{Bounds document.PresentationRect, RotationDeg float64}` and pure helpers for device-pixel/slide-point conversion, inverse-rotation local coordinates, move preview, resize preview, and angle normalization.
- Produces: shared rotated element rendering used by canvas, thumbnails, and slideshow; only the canvas adds interactive handles.

- [ ] **Step 1: Add pure transform-math regressions**

Add tests named `TestPresentationResizeUsesLocalCoordinates`, `TestPresentationResizeHonorsAspectLock`, `TestPresentationRotateHandleProducesClockwiseAngle`, `TestPresentationTransformAllowsOffSlidePosition`, `TestPresentationTransformEnforcesMinimumSize`, and `TestPresentationRotationRenderAcrossCanvasThumbnailSlideshow`.

Assert 0°/90°/arbitrary-angle resize, locked/unlocked aspect behavior, normalized angles in `[0, 360)`, partial off-slide movement, and 8 pt minimum dimensions.

- [ ] **Step 2: Run transform-math regressions to confirm they fail**

Run: `go test ./internal/desktopui -run 'TestPresentation(ResizeUsesLocalCoordinates|ResizeHonorsAspectLock|RotateHandleProducesClockwiseAngle|TransformAllowsOffSlidePosition|TransformEnforcesMinimumSize|RotationRenderAcrossCanvasThumbnailSlideshow)' -count=1`

Expected: failures identify missing preview math; fixture/setup errors are corrected first.

- [ ] **Step 3: Implement reusable transform math helpers**

In `presentation_transform.go`, define the local transform and stable handle enum; convert pointer deltas to slide points through `canvasScale`; inverse-rotate resize pointer coordinates around the shape center; preserve an explicit aspect lock; clamp width/height to 8 pt; normalize rotation to `[0, 360)`.

- [ ] **Step 4: Apply committed rotation to shared render paths**

Render shape path, fill, text, and picture in the same local-coordinate rotation transform. Update thumbnail and slideshow paths to consume `RotationDeg`; do not draw selection handles outside the main editing canvas.

- [ ] **Step 5: Run math and render regressions and commit**

Run: `go test ./internal/desktopui -run 'TestPresentation(ResizeUsesLocalCoordinates|ResizeHonorsAspectLock|RotateHandleProducesClockwiseAngle|TransformAllowsOffSlidePosition|TransformEnforcesMinimumSize|RotationRenderAcrossCanvasThumbnailSlideshow)' -count=1`

Expected: PASS, including pixel checks for a rotated shape and picture in canvas, thumbnail, and slideshow rendering.

Commit: `feat: render rotated PPTX objects`

---

### Task 3: Canvas Selection and Move/Resize/Rotate Gestures

**Files:**
- Modify: `internal/desktopui/presentation_state.go`
- Modify: `internal/desktopui/presentation_canvas.go`
- Modify: `internal/desktopui/presentation_editing.go`
- Create: `internal/desktopui/presentation_interaction.go`
- Modify: `internal/desktopui/presentation_workspace_test.go`
- Create: `internal/desktopui/presentation_interaction_test.go`

**Interfaces:**
- Consumes: transform preview math from Task 2 and `UpdateSlideElementTransform` from Task 1.
- Produces: persistent `presentationTransformGesture` state with selected shape ID, gesture mode/handle, original transform, preview transform, and normalized anchor.
- Produces: UI flow where release commits one adapter update and pointer cancel discards preview; no-op release does not call the adapter.

- [ ] **Step 1: Add Gio interaction regressions**

Add tests named `TestPresentationCanvasSelectsTextShapeAndPicture`, `TestPresentationCanvasMovesObjectsOnDrag`, `TestPresentationCanvasResizesAllHandles`, `TestPresentationCanvasRotatesObjects`, `TestPresentationTransformCancelAndNoopRelease`, `TestEmptyPlaceholderPromptStartsBlankAndReappears`, and `TestPresentationTransformGesturesAtDeviceScales`.

Use temporary presentations containing one text box, one auto-shape, one PNG picture, and one empty placeholder. Assert selection, threshold behavior, all eight handles, move/resize/rotate preview, adapter commit only on release, cancel/no-op semantics, Chinese entry into a blank placeholder, and geometry at 1×/1.5×/2×.

- [ ] **Step 2: Run the Gio interaction regressions to confirm they fail**

Run: `go test ./internal/desktopui -run 'TestPresentationCanvas(SelectsTextShapeAndPicture|MovesObjectsOnDrag|ResizesAllHandles|RotatesObjects)|TestPresentationTransform(CancelAndNoopRelease|GesturesAtDeviceScales)|TestEmptyPlaceholderPromptStartsBlankAndReappears' -count=1`

Expected: failures identify missing canvas selection/gesture behavior; headless GPU skips are reported only for pixel rendering cases.

- [ ] **Step 3: Add persistent gesture state and pointer handling**

Keep all state in `presentationViewState`. In `presentation_interaction.go`, register hit regions for supported slide-local text/shape/picture objects, eight handles, and the rotation handle; normalize pointer coordinates once; treat movement past 4 dp as a drag. Draw previews without changing the document.

- [ ] **Step 4: Wire selection, text editing, and release/cancel behavior**

Update `presentation_canvas.go` and `presentation_editing.go` so non-empty text selects on the first click and edits on a second click or Enter; a blank placeholder enters an empty editor on a no-drag click. Escape flushes the editor draft and retains selection. On release, call `UpdateSlideElementTransform` once; on cancel, clear gesture state and restore the original preview.

- [ ] **Step 5: Verify interactive and visual regressions and commit**

Run: `go test ./internal/desktopui -run 'TestPresentationCanvas(SelectsTextShapeAndPicture|MovesObjectsOnDrag|ResizesAllHandles|RotatesObjects)|TestPresentationTransform(CancelAndNoopRelease|GesturesAtDeviceScales|RotationRenderAcrossCanvasThumbnailSlideshow)|TestEmptyPlaceholderPromptStartsBlankAndReappears' -count=1`

Expected: PASS; text prompt labels remain absent from the model and drag previews do not alter document revision before release.

Commit: `feat: add PPTX object manipulation gestures`

---

### Task 4: Compatibility Evidence and End-to-End Acceptance

**Files:**
- Modify: `docs/office-support.md`
- Modify: `docs/office-library-evaluation.md`
- Modify: `docs/compatibility/office-feature-matrix.md`
- Modify: `internal/desktopui/presentation_workspace_test.go` if a save/reopen interaction gap remains

**Interfaces:**
- Consumes: adapter and UI behavior from Tasks 1–3.
- Produces: evidence-based PPTX transform support boundary; image replacement/cropping and text/shape style editing remain explicitly unsupported in phase 1.

- [ ] **Step 1: Add an end-to-end transform save/reopen regression**

Add `TestPPTXCanvasTransformSaveReopenPreservesUntouchedContent` using a disposable copy or generated temporary deck. Move, resize, and rotate text, a shape, and a picture; save; reopen; compare transforms, placeholder text, and representative untouched content; confirm original sample bytes are unchanged.

- [ ] **Step 2: Run the focused end-to-end regression**

Run: `go test ./internal/desktopui -run 'TestPPTXCanvasTransformSaveReopenPreservesUntouchedContent' -count=1`

Expected: PASS, with reopened geometry matching requested bounds/rotation and untouched content remaining intact.

- [ ] **Step 3: Update compatibility documentation from verified evidence**

Document supported top-level `p:sp`/`p:pic` transforms, placeholder materialization, rotation and resize-lock behavior, tests/sample corpus, and exact unsupported cases. Do not claim image crop/replacement, styles, group editing, or PowerPoint parity.

- [ ] **Step 4: Run the complete Go test suite and inspect the final diff**

Run: `go test ./...`

Expected: PASS. Then run `GLANCE_PPTX_TEST_DIR="$HOME/Downloads" go test ./internal/desktopui -run 'TestPPTXLayoutMasterSamplesOpenAndRender|TestPPTXCanvasTransformSaveReopenPreservesUntouchedContent' -count=1`; confirm each source sample is unchanged, then inspect `git diff --check` and `git status`.

- [ ] **Step 5: Commit the compatibility evidence**

Commit: `docs: record PPTX object transform support`
