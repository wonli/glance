# Word Ribbon, Rulers, and Caret Navigation Design

## Goal

Align Glance's DOCX workspace with the Word editor layout in Pen node `Clpgm`, add horizontal and vertical canvas rulers, and let the caret move naturally across independently editable Word text runs and paragraphs with the arrow keys.

The reference is a 1440 × 900 editor with the existing document tabs at the top, a Word category row and grouped ribbon, a horizontal ruler, a gray workspace with a centered paper column and vertical ruler, and a bottom status bar. The implementation remains responsive when the window is resized.

## User-visible behavior

- Keep the existing document tab strip. Below it, render the Word category row and 96 dp command surface at a fixed 138 dp total ribbon height. Match `Clpgm`'s Home group geometry: a large paste action with stacked clipboard actions, a top row of font fields above one horizontal formatting row, one horizontal paragraph icon row, the style gallery, stacked Find/Replace/Select actions, and one horizontal Editor/Dictate/Add-ins/Designer row. Keep contextual Picture actions in one horizontal row and the group strip horizontally scrollable at narrow widths. Preserve the current Home, Insert, and contextual Picture commands. Only operations already supported by the DOCX adapter are enabled; unsupported controls are visibly disabled.
- Keep the reference's 900 × 600 dp white paper on a neutral gray workspace, centered horizontally with the Clpgm top and bottom offsets. Resize the paper width by dragging either paper edge or either visible outer-edge handle on the horizontal ruler.
- Place a 33 dp horizontal ruler below the ribbon and a 25 dp vertical ruler 14 dp left of the paper, inset 20 dp from its top and 33 dp from the workspace bottom. Match the reference's 15 horizontal labels and 60 dp vertical tick rhythm, scaling the horizontal ruler with the canvas width. Drag the left and right horizontal-ruler indent markers to adjust the selected paragraph and preserve those indents through the DOCX adapter. The vertical scale remains visual-only while the document is in continuous-flow mode.
- Add a bottom status strip in the reference's position and visual style. It may show truthful document information and the continuous-view state, but must not report page counts or pagination modes: Word pagination is not implemented. Controls for unsupported views or zoom remain disabled.
- Arrow navigation keeps the caret in editable text and does not mutate document content. With a collapsed caret, Left/Right move within the current run normally and transfer to the adjacent editable run at a run boundary, then across paragraph boundaries in document reading order. Table paragraphs participate in row-major order.
- Up/Down keep the preferred horizontal caret coordinate. The focused editor handles movement while a caret remains on another visual line within that editor. At its visual edge, navigation moves to the closest editable caret on the adjacent visual line, including another run or paragraph. If a target editor is offscreen, scroll it into view and focus it before applying the caret position.
- Navigation skips read-only text, images, and unsupported content as caret destinations. At the first or last editable position, movement stays at the document boundary. A non-collapsed selection and modified arrow shortcuts retain the editor's existing behavior.
- Moving the caret updates the selected run and paragraph so ribbon commands continue to target the text under the caret. Existing text drafts remain intact. Enter, Backspace, Delete, IME composition, save, undo, and redo keep their current semantics.

## Architecture and data flow

- Keep the implementation in `internal/desktopui`; caret movement is transient UI state and requires no adapter or OOXML mutation.
- Keep persistent `widget.Editor` instances per text run. Build one stable run order from `WordBlock` values, walking body paragraphs and table cells in source order. Use existing neutral IDs; never retain DOCX library types in view state.
- Record visible run editor bounds and caret geometry during layout. Intercept only eligible unmodified arrow events when the local caret reaches the relevant visual edge. Route focus through `wordPendingFocusRunID`, preserve the preferred vertical coordinate until horizontal movement or pointer selection resets it, and request a fresh frame after scrolling/focus changes.
- Compose `layoutWordFlow` from focused components: ribbon, horizontal ruler, vertical-ruler/canvas row, and status strip. Keep the scrollable document list responsive and outside document parsing or save operations.
- Put new ribbon rendering, ruler geometry/rendering, status rendering, and caret routing in focused Word UI files. Keep command capability checks and adapter calls in the existing Word command/controller path. Reuse `ribbon_chrome.go` icon assets and shared tab/button drawing where their layout matches the reference; avoid changing spreadsheet behavior.
- Preserve only the left/right paragraph indents represented by the current ruler handles. Tab stops, first-line/hanging-indent editing, page layout, and vertical-ruler page-margin editing remain unsupported.

## Visual details

- Use the Pen frame as the 1440 × 900 baseline: 42 px document tabs, a 2 px seam, 42 px ribbon categories, 96 px command panel, 33 px horizontal ruler, gray document workspace with a 900 × 600 px paper at x=270/y=231, and a 34 px status strip.
- Use the reference's white ribbon and ruler surfaces, thin gray separators, understated blue active state, compact Lucide/Material icons, and centered paper with a subtle shadow.
- Keep the workspace and paper widths responsive. Do not hard-code the 1440 px screenshot width into layout logic.
- Include all reference ribbon groups and labels for fidelity, but do not make unsupported controls look active or claim functionality the editor lacks.

## Scope limits

- No Word-compatible pagination, page count, headers/footers, print layout, or web layout claim.
- No paragraph-indent editing, tab-stop editing, zoom engine, font-family/size editing, list editing, Find/Replace, or format painter implementation.
- No changes to DOCX save safety or overwrite rules.
- No cross-run selection model; navigation transfers a collapsed caret. Selection remains local to the focused editor.

## Acceptance

- At the reference window size, the Word editor has the same top-to-bottom structure, Home ribbon group positions, and control-row geometry as `Clpgm`; resizing preserves the centered paper and keeps both rulers aligned with the canvas.
- Home, Insert, and Picture controls render with icons and retain their existing capability checks. Unsupported ribbon controls are visibly disabled; selecting or navigating the UI does not change the document.
- The horizontal and vertical ruler scales remain legible and aligned. Horizontal indent markers change the selected paragraph's text width and survive save/reopen; ruler edge handles resize the centered canvas. The vertical scale is explicitly visual-only in continuous-flow mode.
- Ribbon total height remains 138 dp; Home groups match `Clpgm`'s individual control-row geometry, contextual Picture actions remain on one horizontal row, and narrow layouts keep the group strip horizontally scrollable.
- Left/Right move through styled runs and adjacent paragraphs, including table-cell paragraphs, with stable focus and caret placement. Up/Down cross run and paragraph boundaries at visual line edges while preserving the preferred horizontal coordinate.
- Document-start/end movement is bounded. Read-only content is skipped. Typing after a handoff edits the target run; caret movement alone does not mark the document dirty or clear drafts.
- Existing text editing, paragraph split/join, image operations, saving, and spreadsheet ribbon behavior remain unchanged.

## Verification plan

- Add focused regression coverage for arrow handoff within paragraphs, across paragraphs and table cells, vertical preferred-column movement, read-only content, document boundaries, pending drafts, and non-interference with Enter/Backspace/Delete.
- Capture a Word UI layout at the 1440 × 900 reference size and a narrow window size; compare ribbon grouping, icon placement, rulers, paper centering, and status bar against Pen node `Clpgm`.
- Run the relevant DOCX and desktop UI regression suites and a build. Verify ruler indent and canvas-width drags, including DOCX save/reopen on fixture documents. Use temporary copies only; do not modify the external sample DOCX.
