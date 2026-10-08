# PPTX Text and Shape Style Editing Design

## Goal

Add a bounded, save-safe first style-editing slice for slide-local text boxes, auto-shapes, and supported pictures. Selecting or entering a text box must never rewrite its typography or paragraph spacing; only a deliberate style command changes saved style. Keep Office XML and go-pptx types inside `internal/format/pptx` and feed the Gio canvas through the neutral presentation model.

## User experience

- The **开始** ribbon's existing font, bold, font-color, alignment, and fill actions become functional for one selected editable top-level text object or auto-shape. A contextual **形状格式** tab provides fill and outline controls; **图片格式** gains outline controls beside replace/crop.
- Text style commands apply to every supported run in the selected text object. A text-range formatting toolbar is deliberately deferred: the current `widget.Editor` stores one plain-text draft, so pretending it can safely format partial mixed-style selections would change unselected text. Existing run boundaries and properties remain intact for properties the user did not change.
- Supported text properties are font family, size, bold, italic, underline, sRGB color, paragraph alignment, line spacing, and paragraph space before/after. Line spacing accepts percentage or point values. Applying a command while text is being edited preserves the draft and selection frame; geometry and text content do not change.
- Supported shape properties are solid fill color/none, outline color/none, and outline width. The same outline controls apply to supported pictures. Choosing a solid fill on a gradient or theme fill is an intentional conversion to a direct sRGB solid fill; untouched fills, lines, gradients, effects, and theme values remain unchanged.
- UI controls show an active state only when the selected object's supported values agree. Mixed values show a mixed/blank state rather than guessing. Controls are disabled for inherited, grouped, table, linked, unresolved, or otherwise unsupported objects.
- Style commands are one committed mutation: the neutral snapshot and canvas update immediately, dirty/revision state advances once, and save/recovery uses the same pending style overlay.

## Supported boundary

- Editable targets are unique top-level slide-local `p:sp` auto-shapes/text boxes and supported top-level `p:pic` records. Text style editing requires ordinary `a:r` runs and the existing supported single-paragraph text-edit boundary. Fields, hyperlinks, special breaks, groups, layout/master objects, chart/table internals, and unsupported DrawingML structures remain read-only.
- The first slice does not add shapes or pictures, support multi-selection, partial text-range styling, theme/style galleries, gradients/patterns/effects editing, image filters, crop masks, or arbitrary picture corrections.
- Selecting an existing theme/gradient style is read-only until the user explicitly applies one of the supported direct sRGB controls.
- Font family and size validation use non-empty family names and 1–400 pt. Colors are normalized six-digit RGB values. Outline width is 0–20 pt, where zero is a hairline; line/fill `none` is explicit and distinct from leaving a property unchanged. Line spacing supports 0.5–4.0× or 1–200 pt; paragraph before/after spacing supports 0–200 pt.

## Data and adapter boundary

- Add neutral, optional `PresentationStylePatch` fields. Pointer/option presence distinguishes “leave unchanged” from explicit false, none, or zero. Do not expose third-party style structs in the model or UI.
- Keep pending style overrides keyed by active slide index and shape ID, consistent with current transform/crop overlays. Projection layers them onto `PresentationElement`; snapshots and the renderer immediately show the effective values without mutating the go-pptx object model.
- Add a narrow adapter operation that validates slide, unique ID, object kind, supported text structure, color/size/spacing bounds, and no-op semantics before recording one overlay and incrementing revision once.
- The pinned go-pptx `TextRun.SetFont` supports patching Latin/East Asian font names, size, bold, italic, and color. Underline, paragraph alignment/spacing, shape fills, and line styles have read APIs but no corresponding public mutation API. Implement missing writes as shape-ID-targeted DrawingML byte patches behind the adapter; do not serialize/rebuild the whole slide XML tree.
- Patch only requested properties. Preserve unknown attributes and children, original run segmentation, line dash/cap/join/arrows, unrelated text and shape XML, media relationships, and other ZIP entries. Replacing fill with `none` or a chosen solid fill may replace the existing fill subtree only on explicit user action.
- Combine style, geometry, and picture-crop overlays in the same package patch pipeline for both `SaveAs` and `SnapshotBytes`. Reopen the staged package and verify every requested property plus representative untouched text, geometry, media, and package parts before replacing the destination. A failed patch/save keeps dirty state and the original file.

## UI and editor behavior

- Keep persistent ribbon controls and any color/font/spacing popover state in `presentationViewState`. Use a small bounded palette and explicit custom numeric entry; canceling a popover is a no-op.
- For a selected text object, actions that require text are available in selection and edit mode. For a selected auto-shape with text, text commands target its paragraphs while fill/outline target the container. For pictures, only outline commands are enabled on this tab.
- Resolve active style from projected neutral values. Apply bold/italic/underline as toggles; alignment is a direct left/center/right/justify command. Font/color/size/spacing menus validate before dispatch.
- UI events send intents through the existing presentation/session path. File parsing, package writes, and save validation stay off Gio frame/layout callbacks. Ignore results for stale document generations or selections.
- Text editor metrics continue to match the projected paragraph alignment, visible base font size, line spacing, before/after spacing, and insets. Entering edit mode is presentation-only and must not create dirty state or style overrides.

## Verification

- Neutral/model tests cover optional false/none values, patch no-ops, overlay projection, and slide-index add/delete lifecycle.
- Adapter fixtures cover font name/size/bold/italic/underline/color, paragraph alignment and spacing, solid/no fill, outline color/width/no line, combined style+transform+crop patches, unknown-child preservation, unsupported targets, and failed validation with unchanged revision/source bytes.
- Save/reopen and recovery-snapshot tests verify exact targeted properties and representative untouched text, geometry, image bytes, relationships, and package entries.
- Gio tests cover control availability, active/mixed state, text draft and frame retention, canceled/stale color choices, line/fill edits, no-op dirty behavior, and selection/edit-mode consistency.
- Run focused PPTX/UI suites and the full repository suite. Update `office-support.md` and the sibling compatibility matrix only with tested support and explicit limitations.

## Acceptance criteria

- Merely selecting or editing a supported text object preserves its displayed alignment, visible font-size metric, and explicit paragraph spacing; it does not dirty the package.
- Supported text and shape/picture style commands change only the requested properties, preserve unrelated XML and object geometry, and survive save/reopen and `SnapshotBytes()` recovery.
- The identical selection/edit transform frame and active move/resize/rotate gestures remain available while text is being edited.
- Unsupported style structures remain selectable/read-only without being silently flattened or dropped.
