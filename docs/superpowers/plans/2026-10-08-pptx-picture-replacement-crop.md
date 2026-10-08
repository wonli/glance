# PPTX Picture Replacement and Crop Editing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add safe PNG/JPEG replacement and crop editing for supported slide-local PPTX pictures, including matching previews, file saves, and workspace recovery snapshots.

**Architecture:** Add crop and source bytes to the neutral presentation projection, with narrow replacement/crop methods on `internal/format/pptx.Document`. Keep go-pptx and OOXML behind that adapter; compose targeted crop and existing transform patches when saving. Reuse the workspace's asynchronous image picker pattern and render all picture previews through the same crop-aware helper.

**Tech Stack:** Go, Gio, `github.com/F31/go-pptx/v2` v2.0.3, PPTX OPC/XML package parts, Go image decoders, existing adapter and Gio tests.

**Spec:** `docs/superpowers/specs/2026-10-08-pptx-picture-replacement-crop-design.md`

## Global Constraints

- Only a selected, top-level `p:pic` on the active slide with a unique shape ID and a valid internal image relationship is editable.
- Replacement accepts validated PNG and JPEG files; the existing image-file cap is 128 MiB, maximum decoded size is 64 megapixels, and the maximum edge is 32768 pixels. Check dimensions with `image.DecodeConfig` before full decode.
- Crop edits use DrawingML `a:srcRect` source percentages in `0..100000`; crop the source first, then apply the object's existing rotation, without changing its frame.
- Keep all go-pptx, OPC, ZIP, and XML types behind `internal/format/pptx`; widgets consume neutral presentation data and send intents through the adapter.
- Keep chooser, file reads, image decoding, package serialization, and saves outside Gio layout callbacks. Reject stale results by document generation and selected picture ID.
- Save through a sibling temporary package, reopen and verify before replacing the destination, preserve dirty state after failures, and include pending changes in `SnapshotBytes()`.
- Unsupported, external, inherited, grouped, unresolved, or invalid picture records stay read-only and retain their original saved content.
- The same crop-aware renderer must serve canvas, thumbnails, and slideshow; crop previews remain transient until completion.

## Review Focus

- Corrupt bytes, mismatched extension/signature, images over 128 MiB, over 64 megapixels, or over 32768 pixels per edge must be rejected before adapter mutation; pin in Tasks 1 and 4.
- Duplicate/stale IDs, non-picture IDs, external relationships, and unresolved media must be rejected without changing revision or package state; pin in Task 1.
- Negative, over-100000, or fully hidden crop rectangles must be rejected; malformed existing crop XML must remain intact and unavailable for crop editing; pin in Tasks 1 and 2.
- Canceled or stale picker results must not change the selected picture, image bytes, dirty flag, or revision; pin in Task 4.
- A picture with simultaneous transform and crop changes must survive save and recovery reopen while untouched package content and shared media references remain intact; pin in Task 2.

---

### Task 1: Neutral picture crop and adapter mutations

**Files:**
- Modify: `internal/document/presentation.go`
- Modify: `internal/format/pptx/document.go`
- Modify: `internal/format/pptx/package_relationships.go` only if relationship lookup needs a narrow helper
- Test: `internal/format/pptx/inherited_test.go`
- Test: `internal/format/pptx/document_test.go`

**Interfaces:**
- Produces `document.PresentationCrop{Left, Top, Right, Bottom int}` using DrawingML units, plus `PresentationElement.Crop`, `PresentationElement.ImageBytes`, and `PresentationElement.CanEditPicture` for supported slide-local pictures.
- Produces `(*pptx.Document).ReplaceSlidePicture(slideIndex int, shapeID int64, mediaType string, data []byte) error` and `(*pptx.Document).UpdateSlidePictureCrop(slideIndex int, shapeID int64, crop document.PresentationCrop) error`.
- `CanEditPicture` is true only for a unique slide-local picture with a valid internal PNG/JPEG relationship and valid source crop. Crop validation rejects values outside `0..100000` and any rectangle with no positive visible source width or height. Each successful non-no-op mutation increments revision once; rejected and no-op calls preserve dirty state and revision.
- Crop overrides are resized/reindexed with slide add/delete operations, matching the existing slide-indexed transform caches.

- [x] **Step 1: Write failing adapter tests** named `TestPictureProjectionIncludesBytesAndCrop`, `TestReplaceSlidePicturePreservesIdentityAndResetsCrop`, `TestReplaceSlidePictureRejectsUnsupportedTargets`, `TestReplaceSlidePictureRejectsInvalidImageData`, `TestUpdateSlidePictureCropValidatesAndPreservesRevisionOnNoop`, `TestInvalidExistingPictureCropIsReadOnly`, and `TestPictureCropOverridesTrackSlideAddDelete`. Assert bytes, crop, `CanEditPicture`, shape ID, bounds, rotation, alt text, relationship safety, dirty state, revision, and slide-index cache lifecycle.
- [x] **Step 2: Run the focused tests and confirm the new API/behavior fails** with missing crop projection or mutation behavior.
- [x] **Step 3: Implement neutral crop projection and the two adapter methods.** Use `gopptx.PictureShape.ReplaceImage` with `gopptx.BytesMedia`; validate picture kind, unique ID, supported media, dimensions, and internal relationship before changing adapter state. Maintain adapter-owned crop overrides and refresh the selected picture's preview bytes/image after replacement.
- [x] **Step 4: Run `go test ./internal/format/pptx -run 'TestPictureProjection|TestReplaceSlidePicture|TestUpdateSlidePictureCrop' -count=1`** and confirm all focused adapter tests pass.
- [x] **Step 5: Commit** as `feat: add PPTX picture replacement and crop state`.

### Task 2: Crop and transform package persistence

**Files:**
- Modify: `internal/format/pptx/geometry.go`
- Modify: `internal/format/pptx/document.go`
- Test: `internal/format/pptx/inherited_test.go`
- Test: `internal/format/pptx/document_test.go`
- Fixture: generated slide-local picture with shared media and nonzero `a:srcRect`, built from existing PPTX fixture helpers.

**Interfaces:**
- Consumes Task 1's `PresentationCrop`, crop override map, and `UpdateSlidePictureCrop` method.
- Produces one targeted package patch path that composes `presentationTransformOverride` and crop overrides by slide and shape ID for both file saves and `SnapshotBytes()`.
- Save validation reopens the temporary package and compares picture bytes, crop, bounds, rotation, alt text, and representative untouched content before replacement.

- [x] **Step 1: Write failing tests** named `TestUpdateSlidePictureCropPersistsAndComposesWithTransform`, `TestPictureCropSnapshotBytesRetainsCrop`, `TestPictureCropPatchPreservesSharedMediaAndUntouchedParts`, and `TestPictureCropSaveFailurePreservesOriginalAndDirtyState`. Assert generated fixture crop values, transform composition, package-part preservation, and atomic failure behavior.
- [x] **Step 2: Run the focused tests and confirm crop is absent or not persisted** in save and recovery output.
- [x] **Step 3: Implement a shape-ID-targeted `a:srcRect` patch** that preserves unrelated shape XML and package entries; combine crop and transform changes in a single slide-part patch, validate saved crop values after reopening, and use the same path for `Save`, `SaveAs`, and `SnapshotBytes()`.
- [x] **Step 4: Run `go test ./internal/format/pptx -run 'TestUpdateSlidePictureCrop|TestPictureCrop' -count=1`** and confirm save, snapshot, composition, and failure tests pass.
- [x] **Step 5: Commit** as `feat: persist PPTX picture crop edits`.

### Task 3: Shared crop-aware picture renderer

**Files:**
- Create or modify: `internal/desktopui/presentation_picture_render.go`
- Modify: `internal/desktopui/presentation_canvas.go`
- Modify: `internal/desktopui/presentation_thumbnail.go`
- Test: `internal/desktopui/presentation_transform_test.go`
- Test: `internal/desktopui/presentation_layers_test.go`

**Interfaces:**
- Consumes Task 1's neutral crop and picture source bytes/image.
- Produces `drawPresentationPicture(gtx layout.Context, elem document.PresentationElement, size image.Point)` for the canvas, thumbnails, and slideshow canvas. The helper clips to the fixed viewport, maps the visible source rectangle into it, and leaves object rotation to the existing caller.

- [x] **Step 1: Write failing pixel tests** named `TestPresentationPictureRendererAppliesCrop` and `TestPresentationPictureRendererCropThenRotation`; use a generated four-color image to assert the visible pixels at zero and nonzero rotation.
- [x] **Step 2: Run the focused renderer tests and confirm uncropped rendering fails the crop assertions.**
- [x] **Step 3: Implement the shared crop-aware renderer** and route canvas and thumbnail picture drawing through it; slideshow continues through the canvas renderer.
- [x] **Step 4: Run `go test ./internal/desktopui -run 'TestPresentationPictureRenderer' -count=1`** and confirm both pixel tests pass.
- [x] **Step 5: Commit** as `feat: render cropped PPTX pictures consistently`.

### Task 4: Asynchronous picture replacement UI

**Files:**
- Create: `internal/desktopui/presentation_picture_picker.go`
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/presentation_state.go`
- Modify: `internal/desktopui/presentation_view.go`
- Modify: `internal/desktopui/layout.go` for picker result delivery
- Test: `internal/desktopui/presentation_workspace_test.go`
- Test: `internal/desktopui/workspace_ribbon_render_test.go`

**Interfaces:**
- Consumes Task 1's `ReplaceSlidePicture` and Task 3's crop-aware preview model.
- Produces a contextual **图片格式** ribbon tab for a selected supported picture, **替换图片** action, asynchronous PNG/JPEG loading capped at 128 MiB / 64 MP / 32768 px, and a result handler guarded by document generation plus slide/shape ID.
- File reading, dimension checks, decoding, and adapter replacement mutation run in the worker; the UI thread applies only a current, successful result and requests redraw.

- [x] **Step 1: Write failing UI tests** named `TestPresentationPictureTabRequiresSupportedSelection`, `TestPresentationPicturePickerCancelAndStaleResultAreNoops`, `TestPresentationPicturePickerValidatesDecodeLimits`, and `TestPresentationPictureReplacementUpdatesPreviewAndDirtyState`. Assert saving/format operations and unsupported pictures disable the controls; selected shape stays selected and identity/frame/rotation/alt text survive.
- [x] **Step 2: Run these tests and confirm the contextual tab, picker, or result path is missing.**
- [x] **Step 3: Implement persistent picker state and result channel** in a focused file; add the contextual ribbon tab and route picker launch/result through the existing workspace async event pattern, never doing file I/O or decoding during layout.
- [x] **Step 4: Run `go test ./internal/desktopui -run 'TestPresentationPicture(Tab|Picker|Replacement)' -count=1`** and confirm picker cancellation, staleness, validation, and replacement tests pass.
- [x] **Step 5: Commit** as `feat: add asynchronous PPTX picture replacement UI`.

### Task 5: Crop preview and editing gestures

**Files:**
- Create or modify: `internal/desktopui/presentation_picture_crop.go`
- Modify: `internal/desktopui/presentation_state.go`
- Modify: `internal/desktopui/presentation_view.go`
- Modify: `internal/desktopui/presentation_canvas.go`
- Modify: `internal/desktopui/presentation_interaction.go` only for shared coordinate helpers if needed
- Test: `internal/desktopui/presentation_interaction_test.go`
- Test: `internal/desktopui/presentation_transform_test.go`

**Interfaces:**
- Consumes Tasks 1 and 3 crop data/rendering and Task 4's selected-picture contextual tab.
- Produces persistent crop-mode state with original and preview crops, eight source-rectangle handles, inside-frame source panning, **完成裁剪**, **取消裁剪**, and **重置裁剪**.
- Drag math uses pointer coordinates normalized at the input boundary and inverse-rotated to picture-local space; the selected frame remains identical to the edit frame and never changes during crop mode.
- Crop completion sends the adapter mutation to a worker; cancel/Escape remains immediate and drops preview-only state.

- [x] **Step 1: Write failing interaction tests** named `TestPresentationCropPreviewDoesNotMutateDocument`, `TestPresentationCropHandlesAndPanKeepObjectFrameFixed`, `TestPresentationCropCommitCancelAndEscape`, and `TestPresentationCropInverseRotatesPointerCoordinates`. Assert preview revision stays unchanged, async completion updates revision once, cancel/Escape changes nothing, and transform controls resume after crop mode.
- [x] **Step 2: Run focused crop interaction tests and confirm the crop-mode state and gestures are absent.**
- [x] **Step 3: Implement crop preview state and pointer math** in the focused crop file; show all eight crop handles, pan the source inside the fixed viewport, and commit once through `UpdateSlidePictureCrop` only on completion.
- [x] **Step 4: Run `go test ./internal/desktopui -run 'TestPresentationCrop' -count=1`** and confirm handles, pan, preview, finish, cancel, Escape, and rotation-aware coordinates pass.
- [x] **Step 5: Commit** as `feat: add PPTX picture crop interactions` (`62b60d1`).

### Task 6: Support evidence and whole-feature verification

**Files:**
- Modify: `docs/office-support.md`
- Modify: `docs/office-library-evaluation.md` only if implementation evidence changes the selected-library assessment
- Modify: `../tools/docs/compatibility/office-feature-matrix.md` following repository guidance
- Test: `internal/format/pptx` and `internal/desktopui` package suites

**Interfaces:**
- Consumes all prior task interfaces and the generated crop fixture.
- Produces evidence-based PPTX support claims for replacement and crop, including unsupported cases and passing save/reopen/snapshot behavior.

- [x] **Step 1: Run focused adapter and UI suites** with `go test ./internal/format/pptx ./internal/desktopui` and inspect all output.
- [x] **Step 2: Run the full Go suite** with `go test ./...` and run supplied PPTX sample open/save checks against temporary copies; verify originals remain byte-identical.
- [x] **Step 3: Update compatibility documentation** to list only tested support and explicit limitations; include generated crop-fixture evidence and the local sample survey finding.
- [x] **Step 4: Run `git diff --check` and review the complete diff** for source, persistence, renderer, picker, crop interactions, and support claims.
- [x] **Step 5: Commit** as `docs: record PPTX picture editing support`.

## Completion Contract

- Replacement and crop pass adapter tests plus file save/reopen and `SnapshotBytes()` recovery checks.
- Crop output matches in canvas, thumbnails, and slideshow, including nonzero object rotation.
- Stale, unsupported, invalid, canceled, and failed edits preserve committed document state and the original source package.
- `go test ./...` and supplied sample checks pass, and compatibility claims match the evidence.
- The separate text and shape style-editing phase remains required by the larger user goal and needs its own design and implementation plan after this plan is complete.
