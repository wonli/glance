# Word Ribbon, Rulers, and Caret Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the DOCX workspace chrome around the Pen `Clpgm` layout, add horizontal and vertical rulers, and provide Word-like arrow-key caret movement across text runs and paragraphs.

**Architecture:** Keep the DOCX projection and persistent per-run Gio editors. Compose a Word-specific ribbon, rulers, and status bar in `internal/desktopui`; route boundary arrows through neutral block/run IDs and existing session focus state. Route horizontal-ruler paragraph-indent edits through neutral paragraph DTOs and the DOCX adapter; keep pixel geometry out of the saved model.

**Tech Stack:** Go, Gio (`gioui.org/layout`, `widget.Editor`, `widget.List`), existing `internal/document` Word DTOs and `internal/desktopui` ribbon icon assets.

**Spec:** `docs/superpowers/specs/2026-10-01-word-ribbon-ruler-navigation-design.md`

## Global Constraints

- Follow `docs/architecture.md`: Gio UI -> session/controller -> neutral document model -> format adapters -> local files.
- Persistent widgets belong in stable session state; set every `widget.List` axis explicitly.
- Keep layout, event handling, and controller operations separate.
- Keep parsing and saves outside frame layout code.
- Only existing DOCX-supported operations may be enabled in the ribbon.
- Keep the ribbon at 138 dp total height and reproduce the Home group's row structure and positions from Pen node `Clpgm`; keep contextual Picture actions on one horizontal row.
- Do not claim pagination, page counts, tab-stop editing, vertical-ruler page-margin editing, or zoom behavior that is not implemented.
- Preserve dirty drafts and existing save, split, join, IME, and image behavior.

## Review Focus

- A target run may be below the viewport or not yet measured; caret handoff must scroll and focus it without dropping the active draft.
- CJK and combining-character text must keep caret offsets in Gio's rune/selection coordinate system and avoid splitting grapheme input during handoff.
- Mixed-style runs can share a visual line; up/down must choose a target by measured screen position and retain the preferred x coordinate.
- A table may contain several cells and paragraphs; navigation order must follow rows, cells, and paragraphs rather than map iteration.
- Unsupported ribbon categories must remain visually disabled and must not mutate document or session state when selected.

---

### Task 1: Word ribbon layout

**Files:**
- Create: `internal/desktopui/word_ribbon_layout.go`
- Modify: `internal/desktopui/word_toolbar.go`
- Modify: `internal/desktopui/session_state.go`
- Modify: `internal/desktopui/ribbon_chrome.go` only if a needed icon is absent

**Interfaces:**
- Consumes: `wordToolbarActions`, `wordToolbarActionEnabled`, `wordToolbarActionActive`, existing toolbar actions and icon rendering.
- Produces: `layoutWordRibbon(layout.Context) layout.Dimensions`, composed from a 42 dp category row and a 96 dp grouped command panel. Keep supported Home, Insert, File/Save, and contextual Picture actions bound to the existing handlers.

- [x] Render the full Clpgm category row and reference group order with icons, keeping only implemented commands enabled.
- [x] Keep Word tab widgets and action state in `documentSessionState`; do not share mutable Word state with spreadsheet tabs.
- [x] Remove or redirect the old single-row Word toolbar layout so there is one source of Word ribbon rendering.
- [x] Inspect the toolbar action dispatch paths and `git diff --check` for duplicate bindings or accidental spreadsheet changes.

### Task 2: Rulers, paper workspace, and status strip

**Files:**
- Create: `internal/desktopui/word_ruler.go`
- Create: `internal/desktopui/word_status_bar.go`
- Modify: `internal/desktopui/word_layout.go`
- Modify: `internal/desktopui/session_state.go` only for persistent state needed by the view

**Interfaces:**
- Consumes: `wordPageGeometry`, `wordPageHorizontalMargin`, the 900 × 600 dp `wordCanvasWidthDP` defaults, `wordListState`, and the existing scroll/resize gestures.
- Produces: `layoutWordHorizontalRuler(layout.Context, *documentSessionState)`, `layoutWordVerticalRuler(layout.Context, *documentSessionState)`, and `layoutWordStatusBar(layout.Context, *documentSessionState)`.

- [x] Compose Word view in the reference order: ribbon, horizontal ruler, vertical ruler beside the existing canvas, status strip.
- [x] Draw responsive ruler ticks and labels aligned to the centered paper/text origin; keep the vertical ruler in the same scroll viewport as the canvas.
- [x] Add the reference indent markers as visual-only markers; do not write pixel values or unsupported indents to DOCX.
- [x] Render a truthful continuous-view status and disable unsupported page/zoom/view controls.
- [x] Keep existing paper edge resizing, scroll routing, and canvas minimum width working at wide and narrow constraints.
- [x] Preserve ribbon total height and align Home group geometry with Pen node `Clpgm`.
- [x] Add visible horizontal-ruler page-width handles and drag them to resize the centered canvas.
- [x] Make horizontal-ruler left/right paragraph indent markers draggable; commit neutral EMU values through the DOCX adapter and preserve unrelated `w:pPr` properties.
- [x] Inspect visual/layout code paths and `git diff --check` for alignment and sizing mistakes.

### Task 3: Cross-run and cross-paragraph arrow navigation

**Files:**
- Create: `internal/desktopui/word_navigation.go`
- Modify: `internal/desktopui/word_layout.go`
- Modify: `internal/desktopui/session_state.go`

**Interfaces:**
- Produces: `wordNavigationRuns([]document.WordBlock) []wordNavigationRun`, in body/table reading order; `handleWordArrowNavigation(layout.Context, document.WordParagraph, document.WordTextRun, *widget.Editor) bool`, which returns true only when it consumes a supported boundary-arrow event.
- Session state: retain preferred vertical x, visible run/editor geometry, and pending caret offset per document session.

- [x] Build deterministic editable-run order from body paragraphs and table row/cell paragraph slices; skip read-only runs and non-text inlines as caret targets.
- [x] Preserve Gio's ordinary movement inside a run; hand off collapsed Left/Right at run edges and across adjacent paragraphs in reading order.
- [x] Route Up/Down at visual editor edges to the nearest editable line by saved x coordinate, keeping the preferred x until horizontal movement or pointer selection resets it.
- [x] Scroll an offscreen target into view, focus its persistent editor, apply its caret offset after layout, and keep draft/save state untouched.
- [x] Avoid consuming modified arrows, selection movement, Enter, Backspace, Delete, or IME events.
- [x] Review event ordering against `handleWordParagraphJoinKey` and the existing paragraph split/join paths; use `git diff --check`.

### Task 4: Integrate and review the whole Word view

**Files:**
- Modify only the files listed above, plus the design/spec documents if implementation decisions require an update.

- [x] Match `Clpgm`’s 1440 × 900 row dimensions and reference order; verify the flow retains viewport dimensions at 480 px width.
- [x] Review disabled command semantics, truthful status text, list axis initialization, stable widget state, and scroll/event routing.
- [x] Run `go build ./...` and review `git diff --check`; do not alter the external sample DOCX.
- [x] Report any behavior that remains unverified because it requires runtime interaction or DOCX round-trip evidence.
