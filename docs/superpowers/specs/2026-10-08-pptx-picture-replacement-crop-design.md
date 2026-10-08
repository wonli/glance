# PPTX Picture Replacement and Crop Editing Design

## Goal

Extend the existing PPTX object editor with native-feeling replacement and crop workflows for selected slide-local pictures. Preserve the current picture object identity and frame, keep all DrawingML and third-party library types behind `internal/format/pptx`, and validate edits through save/reopen before broadening the support claim.

This is the required second phase of the larger PPTX editing goal. Text and shape style editing remains a required, separate third phase with its own design and save-safety review; this phase does not remove it from the goal.

## User experience

- Selecting a supported picture exposes a contextual **图片格式** tab in the presentation ribbon. It provides **替换图片** and **裁剪**. The normal transform frame remains the same object bounds used by the selection and edit frames.
- **替换图片** opens the existing asynchronous image-file selection flow, limited to PNG and JPEG. A canceled chooser is a no-op. A successful replacement keeps the picture shape ID, bounds, position, rotation, and alternative text. It clears any previous crop so a crop from the old image cannot distort a replacement with a different aspect ratio.
- **裁剪** enters a crop mode for the selected picture. Eight crop handles change the visible source rectangle while the picture frame and its position/size/rotation stay fixed. Dragging inside the picture pans the source image within the fixed viewport without changing the current crop width or height.
- Crop previews are transient until the user chooses **完成裁剪**. **取消裁剪** and Escape discard every preview made in that crop session. **重置裁剪** previews the full source image and is committed only by **完成裁剪**.
- Crop interaction is measured in the picture's unrotated local coordinate system. The picture is cropped first, then the existing object rotation is applied. Normal move/resize/rotate controls return after leaving crop mode.
- Text and shape contextual formatting controls are outside this phase and remain in the subsequent style-editing phase.

## Supported boundary

- Only a selected, top-level `p:pic` on the active slide with a unique shape ID and a valid internal image relationship is editable.
- Replacement accepts validated PNG and JPEG files. Imported GIF previews remain read-only for replacement because the selected library's replacement API supports PNG/JPEG.
- Crop editing operates on the picture's DrawingML `a:srcRect` source rectangle. It changes neither `a:xfrm` geometry nor the media bytes.
- Inherited layout/master pictures, linked/external images, unresolved relationships, invalid crop rectangles, unsupported media, grouped pictures, video posters, and crop edits to shape fills remain read-only. Invalid crop data does not make the picture disappear or change the saved XML. No image filters, masks, or shape styles are introduced here.

## Approaches considered

1. **Use only go-pptx public mutation methods.** The pinned v2.0.3 library has a tested `PictureShape.ReplaceImage` method that handles media relationships, shared references, content-hash reuse, and crop removal. It does not expose a crop mutator, so this cannot implement crop editing by itself.
2. **Patch all picture changes directly in OOXML.** This gives full control over `a:srcRect`, but would duplicate media-part creation, content-type updates, relationship updates, shared-reference protection, and cleanup already handled by the tested replacement method.
3. **Use a hybrid behind the adapter (recommended).** Use `PictureShape.ReplaceImage` for media replacement, retain its tested relationship behavior, and use a shape-ID-targeted `a:srcRect` patch for crop persistence. UI code consumes neutral image/crop data and calls narrow adapter operations. The patch path validates the written package by reopening it and comparing the requested crop and replacement image.

## Data and rendering

- Add a neutral crop value to `document.PresentationElement`, with left/top/right/bottom source percentages in DrawingML's `0..100000` units. Missing `a:srcRect` projects as all zeroes.
- Parse crop values from the owning slide's picture XML and apply any pending adapter override last, like existing transform overrides. Reject negative values, values above 100000, and crops that leave no positive visible source width or height.
- Store committed crop overrides by slide index and picture shape ID. Update slide-indexed caches when slides are added or deleted. Preview crops stay in `presentationViewState` and do not modify adapter revision.
- The shared picture renderer must show the same crop in the canvas, thumbnails, and slideshow. It draws the selected source rectangle into the fixed picture viewport, then applies the object's existing rotation. It does not alter the original `ImageBytes`.
- The adapter owns two narrow operations: replace a supported slide picture from PNG/JPEG bytes, and update a picture's crop. Each validates slide, shape kind, unique shape ID, image relationship, file type/crop bounds, and no-op semantics before changing dirty state and revision once.
- Replacement stores the new neutral preview image/bytes until a fresh package projection is available. The committed package mutation remains in go-pptx; the UI never receives its picture or OPC types.
- Apply pending crop overrides to both file-save output and `SnapshotBytes()` recovery output. Combine crop and transform changes into one targeted slide-part patch so changing one picture preserves other XML and package entries.

## UI state and event flow

- Keep persistent toolbar buttons, crop mode, its original crop value, preview crop value, selected shape ID, document generation, and file-picker running/result state in presentation/application state.
- The image chooser, image read/decode, and adapter mutation run outside Gio layout callbacks. Publish results through the existing synchronized result-channel pattern and invalidate the window after applying them.
- A picker result is accepted only when the same document generation and picture ID are still active. Stale, canceled, invalid, or oversized input leaves the document revision and source image unchanged.
- Crop pointer coordinates are normalized at the input boundary and inverse-rotated into picture-local coordinates. Handle drags update only preview state; pointer release advances the crop preview. Completing crop calls the adapter once. Escape/cancel restores the crop captured on entry.
- Disable crop and replacement controls while saving, while another format operation runs, or for unsupported picture records. Report failures without dropping dirty state.

## Save and compatibility safety

- The user-selected image is read through a bounded file loader and decoded before mutation. Enforce the existing 128 MiB image-file cap, a maximum decoded size of 64 megapixels, and a 32768-pixel edge limit. Check dimensions with `image.DecodeConfig` before full decode. The adapter repeats signature and dimension validation before mutation.
- Serialize to a sibling temporary package, apply targeted crop patches, reopen it through the PPTX adapter, and verify the picture ID, image bytes, crop, bounds, position, rotation, and alternative text. Compare representative untouched slide content and relationships. Replace the destination only after all checks pass; on failure keep the original package and dirty state.
- Recovery `SnapshotBytes()` must include both replacement media and crop values. Reopening the recovery snapshot must project the same picture content and crop.
- Original sample PPTX files are opened read-only; mutation tests use temporary copies and verify source bytes remain unchanged.

## Verification

- Adapter tests cover replacement of only the targeted shape, PNG/JPEG validation, no-op behavior, preservation of shape ID/frame/rotation/alt text, crop reset on replacement, shared media references, invalid IDs/kinds/relationships, crop parsing/limits/no-op revision, and preservation of unrelated package parts.
- Save/reopen and snapshot tests compare replacement bytes and source crop values. They also verify that transform and crop patches compose on the same picture and that failure does not replace the original package or clear dirty state.
- Gio tests cover contextual tab visibility, picker cancellation and stale-generation rejection, replacement result application, crop preview without revision changes, pan and all eight crop handles, complete/escape cancellation, and controls becoming unavailable for unsupported pictures.
- A generated PPTX fixture with a slide-local `p:pic`, shared-media reference, and nonzero `a:srcRect` values is required; it covers crop edits because the four supplied local samples contain no slide-local `p:pic` with `a:srcRect`. The read-only 2026-10-08 sample survey found `PPT_RealTest.pptx` has 18 inherited pictures across layouts/masters and none uses `a:srcRect`.
- Pixel regressions verify cropped content in canvas, thumbnail, and slideshow at zero and nonzero rotation. UI pointer tests verify the object bounds do not change while cropping.
- Run focused adapter/UI tests, the full Go suite, and the supplied PPTX sample checks before recording support. Update `office-support.md`, `office-library-evaluation.md`, and `docs/compatibility/office-feature-matrix.md` only from those results.

## Acceptance criteria

- A supported selected picture can be replaced asynchronously without changing its object identity or frame, and the replacement survives save/reopen with its bytes verified.
- A supported selected picture can be cropped and panned inside a fixed frame. The preview matches canvas, thumbnail, and slideshow renderers; completion commits once; Escape/cancel commits nothing.
- Crop percentages survive ordinary file saves and workspace recovery snapshots. Replacement clears old crop values by design.
- Invalid, stale, unsupported, canceled, or failed edits leave the original source and committed document state unchanged.
- Untouched slide content and relationships survive representative round-trip tests. The application does not claim universal PowerPoint fidelity.

## Follow-up phase

The third phase remains text and shape style editing. The existing ribbon already shows placeholder actions for font, bold, font color, alignment, and fill. The selected go-pptx version can write text-run font family, size, bold, italic, and color, but exposes underline, paragraph alignment, shape fill, and shape line mainly as read-only data. The next design must specify selected-text versus whole-object behavior, common shape fill/outline controls, renderer updates, and safe OOXML write boundaries before implementation.
