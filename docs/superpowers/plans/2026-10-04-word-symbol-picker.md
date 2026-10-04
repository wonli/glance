# Word Symbol Picker Implementation Plan

**Goal:** Enable Insert > Symbol with a small, stateful Unicode symbol panel that inserts the selected character at the current caret or replaces the current selection.

**Architecture:** Keep picker visibility, stable Gio clickables, and the captured Word run/rune range in the per-window `App`. Open the picker through the existing toolbar action flow after pending drafts flush. Apply the chosen symbol through `document.WordTextRangeEditor.ReplaceTextRange` in the existing asynchronous Word mutation pipeline, then refresh the shared blocks and place the caret immediately after the inserted symbol.

**Tech Stack:** Go, Gio, existing Word document capability interfaces, DOCX adapter, existing `wordCommandResults` asynchronous mutation path.

**Spec:** `docs/word-ribbon-todo-zh.md`, Insert > Symbol (`insert-symbol`): provide a character panel and insert Unicode at the current caret; do not replace the entire run.

## Global Constraints

- Preserve the existing run and paragraph formatting by changing only the selected rune range.
- Keep rune offsets consistent with `WordTextRangeEditor`; supplementary-plane symbols must advance the caret by one rune.
- Keep Gio widgets in stable per-window state and keep document mutation outside layout/frame code.
- Require an editable body text run and the range-editing capability; otherwise keep the action unavailable.
- Do not alter unsupported DOCX parts or broaden the feature to equations/complex objects.

## Review Focus

- Draft flush completes before the insertion location is captured.
- The picker retains the originally targeted run/range while open.
- Selecting a symbol replaces only the selection, preserves surrounding text and formatting, and positions the caret after the inserted rune.
- Mutation errors leave the visible document text and selection recoverable and show an error status.
- Closing the picker without choosing a symbol performs no edit.

## Tasks

### Task 1: Lock down range insertion behavior

- [x] Add focused regressions for insertion at a caret, replacing selected text, supplementary-plane rune caret position, disabled state without an editable selected run, table-cell disablement, stale picker state, cancel, and mutation failure.
- [x] Run the focused action test first and confirm it failed because Insert > Symbol was disabled; enable the capability gate and confirm the same assertion passes.

### Task 2: Add the stateful character panel

- [x] Add 24 common punctuation, currency, arithmetic, and arrow symbols with stable per-window Gio clickables.
- [x] Add open, choose, and cancel behavior; capture the current editable run and rune range after draft flushing. Reject stale document revisions or newly changed draft epochs while the panel is open.
- [x] Add accessible labels for each symbol button.

### Task 3: Apply symbol insertion through the document controller

- [x] Enable the ribbon action only when a selected body run is editable and implements `WordTextRangeEditor`.
- [x] Execute `ReplaceTextRange` in the existing asynchronous mutation pipeline, refreshing shared blocks and placing the caret after the inserted rune on success.
- [x] Preserve error handling, dirty state, session revision, and persisted session behavior.

### Task 4: Verify and update TODO

- [x] Run focused desktop UI and DOCX range-editing regressions; the existing DOCX adapter test verifies save/reopen and formatting preservation.
- [x] Update the Chinese per-icon TODO only after the command is connected and save/reopen evidence passes.
- [x] Record the fixed symbol set and lack of arbitrary character search without claiming full Word feature parity.

## Verification Notes

- Passed: `go test ./internal/desktopui -run 'TestWord(InsertSymbol|RibbonSymbolButton|Toolbar|PageBreak)' -count=1`.
- Passed: `go test ./internal/format/docx -count=1`.
- The broader `desktopui` suite is not green in this checkout: unrelated existing grid geometry/font and Home-filter tests fail; a `TestWord` sweep also reports failures in image resizing, ruler dragging, paragraph focus/joining, and vertical caret navigation. These are outside the symbol picker code paths and remain follow-up work.
- No Computer Use window was opened for this feature; the repository's overall Pen 1:1 visual acceptance remains open.
