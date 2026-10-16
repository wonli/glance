# Excel AutoSum Formula Implementation Plan

> **For Codex:** Execute this plan task by task in the current session. Follow the repository's TDD guidance: add a failing test first, observe the failure, then implement the smallest production change that makes it pass.

**Goal:** Make Excel AutoSum write native formulas for Sum, Average, Count, Max, and Min, preserving selection inference and grouping each batch into one undo step.

**Architecture:** Add a neutral formula-batch contract to `internal/document`, implement validated atomic writes in the XLSX adapter, expose a revision-checked application capability, then route the existing desktop AutoSum commands through it. Keep format-specific types inside the XLSX adapter and do not send formulas through the value-only cell batch API.

**Tech Stack:** Go, Gio desktop UI, Excelize XLSX adapter, existing application capability/runtime framework.

---

### Task 1: Add a neutral formula batch contract and transactional XLSX implementation

**Files:**
- Modify: `internal/document/model.go`
- Modify: `internal/format/xlsx/workbook.go`
- Modify: `internal/format/xlsx/workbook_edit.go`
- Create: `internal/format/xlsx/formula_batch_test.go`

- [x] Add `document.CellFormulaEdit` with row, column, and formula fields, plus `SpreadsheetFormulaBatchEditor.SetCellFormulasChanged(sheet, edits) (bool, error)`.
- [x] Write adapter tests first for formula writes, validation before mutation, duplicate/invalid destinations, empty or oversized batches, UTF-16 formula limits, no-op identity, one-step undo/redo, and save/reopen persistence.
- [x] Run the new adapter tests and confirm they fail because the batch contract/implementation is missing.
- [x] Implement full-batch validation, no-op detection, one snapshot, writes, one history entry, rollback on write/history errors, linked-value refresh, and dirty state.
- [x] Run focused XLSX formula batch tests.

### Task 2: Expose a revision-checked formula batch application capability

**Files:**
- Modify: `internal/application/document_commands.go`
- Modify: `internal/application/format_adapters.go`
- Modify: `internal/application/spreadsheet_commands.go`
- Create: `internal/application/spreadsheet_formula_batch_capability_test.go`
- Modify: `internal/application/core_commands_test.go` if the capability manifest requires updating

- [x] Add `SpreadsheetFormulaBatchSetter` and the corresponding adapter function/method.
- [x] Write capability tests first for valid batches, exact changed results, duplicate/invalid coordinates, oversized batches, formula length, revision advancement, and failure atomicity.
- [x] Run the focused test and confirm the new capability is absent or rejects its valid payload.
- [x] Register `spreadsheet.formulas.set` as a medium-risk, revision-checked XLSX capability with a 1–10,000 edit schema and 8192 UTF-16-unit formula limit.
- [x] Validate the full batch before invoking the adapter; keep `spreadsheet.cells.set` value-only.
- [x] Run focused `internal/application` tests and the command manifest test.

### Task 3: Route desktop AutoSum through formula batches

**Files:**
- Modify: `internal/desktopui/application_session_adapter.go`
- Modify: `internal/desktopui/spreadsheet_autosum.go`
- Modify: `internal/desktopui/spreadsheet_commands_test.go`
- Modify: `internal/desktopui/spreadsheet_commands.go` only if a shared formula-batch command helper is needed

- [x] Replace numeric result planning with formula destination planning for all five aggregate operations, retaining the current bounded contiguous-run inference and trailing-blank behavior.
- [x] Add a managed background formula-batch path that uses `spreadsheet.formulas.set` for registered sessions and the neutral adapter contract otherwise.
- [x] Write UI tests first for vertical, horizontal, trailing-blank, single-cell, all-five-operation, and status semantics; XLSX/application tests cover no-op and revision behavior.
- [x] Verify failing tests before production changes, then implement the UI path.
- [x] Report inserted destinations and formula count without claiming a calculated result when the local formula engine has not recalculated it.

### Task 4: Update compatibility documentation and run verification

**Files:**
- Modify: `docs/office-support.md`
- Modify: `docs/compatibility/office-feature-matrix.md`

- [x] Record that AutoSum writes native XLSX formulas and each batch is one undo step; state the preview recalculation boundary accurately.
- [x] Run focused document/XLSX/application/desktop UI tests, plus the existing spreadsheet AutoSum tests.
- [x] Run `git diff --check` and review the final diff for atomicity, formula/value API separation, and truthful UI status.

**Review focus:** rollback leaves workbook state/history unchanged; no-op does not advance revision; formula limits count UTF-16 units; one command creates exactly one undo step; formula ranges exclude the inferred destination; UI does not present cached or locally calculated numbers as guaranteed results.
