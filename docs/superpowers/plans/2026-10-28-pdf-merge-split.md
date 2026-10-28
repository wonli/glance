# PDF 合并拆分小工具 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Glance「小工具」页增加 PDF 合并拆分工具，支持多文件排序合并、按页码范围拆分、逐页拆分，并将安全校验后的输出写入用户选择的本地文件夹。

**Architecture:** Gio 小工具卡片进入主窗口内的 PDF 工作台页面。UI 通过 `application.Runtime` 的中立 PDF 工具接口提交操作；`internal/format/pdfdoc` 负责读取、导入页面、验证和输出，`gopdf` 类型不离开格式适配器。PDF 解析、处理和写入在受管理的后台任务中完成。

**Tech Stack:** Go 1.27.1、Gio v0.10.3、当前固定的 `github.com/SalvioniDigitalSolutions/gopdf`、`internal/fileutil`、现有 i18n JSON。

**Spec:** [docs/superpowers/specs/2026-10-28-pdf-merge-split-design.md](../specs/2026-10-28-pdf-merge-split-design.md)

## Global Constraints

- 所有实现、构建、测试和验证都直接在 `/Users/mima1234/project/glance-go` 执行；未经用户明确授权，不创建、移动或切换 Git worktree。
- 验证复用当前工具链和缓存；不创建独立 `GOCACHE`、`GOMODCACHE`、构建目录、副本或重复依赖缓存。
- 继续使用 `github.com/SalvioniDigitalSolutions/gopdf v0.0.0-20260922202729-55a8f5fed5f5`，不为本功能增加 PDF 依赖。
- gopdf 的 Reader、Document 和其他第三方类型只存在于 `internal/format/pdfdoc`；UI 与应用层只传递中立数据。
- PDF 文件读取、页面导入和写入不得在 Gio layout/frame 中执行；通过受管理后台任务返回结果。
- 输出写入用户选择的本地文件夹；不提供下载；不修改源 PDF，不覆盖已有文件。
- 实际界面验证必须先查找并终止已运行的 Glance 进程及其 `go run ./cmd/glance` 启动器，然后从本目录运行 `GLANCE_PPROF_ADDR=127.0.0.1:6060 go run ./cmd/glance`。不得使用旧进程作为新代码证据。
- 测试、构建成功或跳过不能替代真实界面验证；GPU 验证如跳过，应明确报告未验证部分。

## Review Focus

- 页码范围含非法字符、空项、逆序、越界或重叠时必须拒绝，且不创建输出；由 `TestParsePageRangesRejectsInvalidAndOverlapping` 覆盖（Task 1）。
- 加密、修复失败或截断的 PDF 必须返回可见错误并清理临时文件；由 `TestPDFUtilityRejectsUnsupportedInputAndCleansStaging` 覆盖（Task 2）。
- 输出路径等于源文件或目标文件已存在时必须不覆盖；由 `TestPDFUtilityRejectsSourceAndExistingTargets` 覆盖（Task 2）。
- 拆分批次在中途取消或提交失败时，已写出的文件必须准确列出，其余临时文件必须清理；由 `TestPDFUtilitySplitReportsPartialCommitAndCleansStaging` 覆盖（Task 2）。
- 用户切换工具页面或关闭窗口后，迟到的后台结果不能串到另一个模式；由 `TestPDFMergeSplitToolIgnoresStaleJobResults` 覆盖（Task 5）。

## File Map

- Create `internal/format/pdfdoc/utility_ranges.go`: 1-based inclusive page range type and strict parser.
- Create `internal/format/pdfdoc/utility_files.go`: PDF metadata, merge/split page import, staged validation and output commit.
- Create `internal/application/pdf_utility.go`: neutral types and Runtime methods consumed by desktop UI.
- Create `internal/desktopui/pdf_merge_split_state.go`: stable mode state, editors, clickables, job generation and cancellation ownership.
- Create `internal/desktopui/pdf_merge_split_layout.go`: workbench UI based on Ink nodes `Bv0wm`, `pmPg28a`, and `sp28_000`.
- Create `internal/desktopui/pdf_merge_split_events.go`: mode/file/range/reorder/output actions at the event boundary.
- Create `internal/desktopui/pdf_merge_split_jobs.go`: managed background picker/inspection/merge/split jobs and result channel.
- Modify `internal/desktopui/app.go`: add the persistent PDF utility state field.
- Modify `internal/desktopui/home_tools.go`: register and render the PDF 合并拆分 tool card.
- Modify `internal/desktopui/home_layout.go`: select the tool page before laying out the tools catalog.
- Modify `internal/i18n/locales/zh-CN.json` and `internal/i18n/locales/en-US.json`: add all entry, workbench, status, validation, and error messages.
- Create `internal/format/pdfdoc/utility_ranges_test.go`, `internal/format/pdfdoc/utility_files_test.go`, `internal/application/pdf_utility_test.go`, and `internal/desktopui/pdf_merge_split_test.go`.
- Modify `docs/office-support.md`, `docs/office-library-evaluation.md`, and `docs/compatibility/office-feature-matrix.md` with the delivered capability and observed compatibility boundary.

---

### Task 1: Add the page range parser

**Files:**
- Create: `internal/format/pdfdoc/utility_ranges.go`
- Create: `internal/format/pdfdoc/utility_ranges_test.go`

**Interfaces:**
- Produces `type PageRange struct { Start, End int }`, with 1-based inclusive bounds.
- Produces `func ParsePageRanges(input string, pageCount int) ([]PageRange, error)`.

- [ ] **Step 1: Write parser tests** `TestParsePageRangesAcceptsInclusiveRanges` for `1-12, 18, 23-48`, whitespace, single pages, and exact parsed bounds; write `TestParsePageRangesRejectsInvalidAndOverlapping` for empty items, non-digits, non-positive pages, descending ranges, page count zero, out-of-range pages, duplicates, and overlapping ranges.
- [ ] **Step 2: Run the targeted test to confirm it fails.**

  Run: `go test ./internal/format/pdfdoc -run '^TestParsePageRanges' -count=1`

  Expected: FAIL because `PageRange` and `ParsePageRanges` are not implemented.
- [ ] **Step 3: Implement `PageRange` and `ParsePageRanges`** in `utility_ranges.go`. Parse comma-separated decimal page numbers and `start-end` tokens; require ascending inclusive ranges within `1..pageCount`; reject repeated page ownership across tokens.
- [ ] **Step 4: Run the parser tests and confirm they pass.**

  Run: `go test ./internal/format/pdfdoc -run '^TestParsePageRanges' -count=1`

  Expected: PASS.
- [ ] **Step 5: Commit the parser and its tests.**

### Task 2: Implement validated PDF merge and split operations

**Files:**
- Create: `internal/format/pdfdoc/utility_files.go`
- Create: `internal/format/pdfdoc/utility_files_test.go`
- Consume: `internal/format/pdfdoc/utility_ranges.go`

**Interfaces:**
- Produces `type UtilityFileInfo struct { Path string; PageCount int; SizeBytes int64 }`.
- Produces `type UtilityOutput struct { Path string; PageCount int; SizeBytes int64 }`.
- Produces `func InspectUtilityPDF(ctx context.Context, path string) (UtilityFileInfo, error)`.
- Produces `func MergeUtilityPDFs(ctx context.Context, sources []string, targetPath string) (UtilityOutput, error)`.
- Produces `func SplitUtilityPDFByRanges(ctx context.Context, sourcePath, outputDir, prefix string, ranges []PageRange) ([]UtilityOutput, error)`.
- Produces `func SplitUtilityPDFByPage(ctx context.Context, sourcePath, outputDir, prefix string) ([]UtilityOutput, error)`.
- On split error, the returned outputs list contains only files already committed; the error identifies the failed stage and target.

- [ ] **Step 1: Write synthetic PDF operation tests** `TestPDFUtilityInspectReadsMetadata`, `TestPDFUtilityMergePreservesInputOrder`, `TestPDFUtilitySplitByRangesWritesOneFilePerRange`, and `TestPDFUtilitySplitByPageWritesSinglePageFiles`. Assert reopened outputs have the exact expected page count and order. Build fixtures in memory with the already selected gopdf test helpers; do not add fixture PDFs to the repository.
- [ ] **Step 2: Add failure tests** `TestPDFUtilityRejectsUnsupportedInputAndCleansStaging` for encrypted/repaired/truncated inputs, `TestPDFUtilityRejectsSourceAndExistingTargets` for zero/one-source merge, mismatched range/page count, source-as-target and existing targets, `TestPDFUtilityReportsDestinationFailureAndCleansStaging` for an unusable destination, plus `TestPDFUtilitySplitReportsPartialCommitAndCleansStaging` for cancellation and partial output cleanup.
- [ ] **Step 3: Run the new operation tests to confirm they fail.**

  Run: `go test ./internal/format/pdfdoc -run '^TestPDFUtility' -count=1`

  Expected: FAIL because the utility operations are not implemented.
- [ ] **Step 4: Implement the four operations** in `utility_files.go`. Use `pdfdoc.Open` and its validated reader so encrypted/repaired rejection matches the current adapter. Import pages in the requested order into fresh gopdf documents, check `ctx.Err()` between page imports and writes, and keep all third-party objects in this package.
- [ ] **Step 5: Implement staged output commit.** Write each output to a sibling temporary file, reopen it, verify its exact expected page count, and commit only when the target does not exist and is not a source path. Reuse `fileutil` atomic write/commit guards; delete staged files on error or cancellation. Generate split names as `<safe-prefix>-01.pdf`, incrementing in requested order. Commit split outputs sequentially and return already committed paths if a later output fails.
- [ ] **Step 6: Add `BenchmarkPDFUtilitySplitByPage`** over a generated 100-page PDF with bounded synthetic page content. Run: `go test ./internal/format/pdfdoc -run '^$' -bench '^BenchmarkPDFUtilitySplitByPage$' -benchmem -count=1`. Record time/allocations in `docs/office-support.md`, derive any hard output-count/file-size guard from that evidence, and retain sequential output generation so split batches do not keep every output document in memory.
- [ ] **Step 7: Run PDF utility tests and confirm they pass.**

  Run: `go test ./internal/format/pdfdoc -run '^(TestParsePageRanges|TestPDFUtility)' -count=1`

  Expected: PASS.
- [ ] **Step 8: Commit the PDF adapter operations and tests.**

### Task 3: Add neutral application Runtime operations

**Files:**
- Create: `internal/application/pdf_utility.go`
- Create: `internal/application/pdf_utility_test.go`
- Consume: `internal/format/pdfdoc/utility_ranges.go`, `internal/format/pdfdoc/utility_files.go`

**Interfaces:**
- Produces neutral types `PDFUtilityPageRange`, `PDFUtilityFileInfo`, and `PDFUtilityOutput`; none expose gopdf types.
- Produces `type PDFUtilityMode string` with `PDFUtilityModeRanges` and `PDFUtilityModeByPage`.
- Produces `func ParsePDFUtilityPageRanges(input string, pageCount int) ([]PDFUtilityPageRange, error)` for range preview and validation.
- Produces `func (r *Runtime) InspectPDFUtilityFile(ctx context.Context, path string) (PDFUtilityFileInfo, error)`.
- Produces `func (r *Runtime) MergePDFUtilityFiles(ctx context.Context, sources []string, targetPath string) (PDFUtilityOutput, error)`.
- Produces `func (r *Runtime) SplitPDFUtilityFile(ctx context.Context, sourcePath, outputDir, prefix string, mode PDFUtilityMode, ranges []PDFUtilityPageRange) ([]PDFUtilityOutput, error)`.

- [ ] **Step 1: Write `TestRuntimePDFUtilityInspectMergeSplit`** to inspect a local PDF, merge and split synthetic PDFs through the public Runtime API, reject an unknown split mode, propagate cancellation, return partial committed outputs, and confirm an already-open document keeps the same path, revision, dirty state, and session identity.
- [ ] **Step 2: Run Runtime tests to confirm they fail.**

  Run: `go test ./internal/application -run '^TestRuntimePDFUtility' -count=1`

  Expected: FAIL because the Runtime methods are not implemented.
- [ ] **Step 3: Implement the neutral API** in `application/pdf_utility.go`. Check nil/closed Runtime and context before work, map neutral ranges and results to/from `pdfdoc`, and do not register these file-path operations as MCP/Agent capabilities.
- [ ] **Step 4: Run Runtime PDF utility tests.**

  Run: `go test ./internal/application -run '^TestRuntimePDFUtility' -count=1`

  Expected: PASS.
- [ ] **Step 5: Commit the application API and tests.**

### Task 4: Add the tool card, route, and stable workbench state

**Files:**
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/home_tools.go`
- Modify: `internal/desktopui/home_layout.go`
- Create: `internal/desktopui/pdf_merge_split_state.go`
- Create: `internal/desktopui/pdf_merge_split_layout.go`
- Modify: `internal/i18n/locales/zh-CN.json`
- Modify: `internal/i18n/locales/en-US.json`
- Create: `internal/desktopui/pdf_merge_split_test.go`

**Interfaces:**
- Produces a stable `pdfMergeSplitToolState` owned by `App`, containing separate merge and split drafts, mode, persistent editors/clickables, operation state, generation, and cancellation function.
- Produces `func (a *App) layoutPDFMergeSplitTool(gtx layout.Context) layout.Dimensions`.
- The home tools card sets/clears the tool-page state; `layoutHomeContentPage` selects the tool page before selecting the tools catalog.

- [ ] **Step 1: Write `TestPDFMergeSplitToolNavigation`, `TestPDFMergeSplitToolModeSwitch`, `TestPDFMergeSplitToolStableState`, and `TestPDFMergeSplitToolVisual`** for the tool card opening `Bv0wm`'s workbench, returning to tools, retaining independent merge/split drafts across mode switches, keeping widgets stable across layout calls, and checking visible entry/workbench icon pixels and geometry at 1x and 2x.
- [ ] **Step 2: Run the targeted UI tests to confirm they fail.**

  Run: `go test ./internal/desktopui -run '^TestPDFMergeSplitTool(Navigation|ModeSwitch|StableState|Visual)$' -count=1`

  Expected: FAIL because the tool card and route do not exist.
- [ ] **Step 3: Add the card and page route** in `home_tools.go` and `home_layout.go`; keep the utility in the current Glance main window and return to the tools list from the workbench header.
- [ ] **Step 4: Add stable mode state and base layouts.** Match the three referenced Ink nodes, use independent merge and split drafts, and set every list axis explicitly. Keep all editors and clickables in the `App`-owned state.
- [ ] **Step 5: Add Chinese and English messages** for the entry card, modes, fields, empty states, page range preview, busy/cancel states, partial success, and errors.
- [ ] **Step 6: Run the targeted UI navigation/state tests.**

  Run: `go test ./internal/desktopui -run '^TestPDFMergeSplitTool(Navigation|ModeSwitch|StableState|Visual)$' -count=1`

  Expected: PASS.
- [ ] **Step 7: Commit the entry, route, state, layout, localization, and tests.**

### Task 5: Wire file selection, merge/split actions, and asynchronous results

**Files:**
- Create: `internal/desktopui/pdf_merge_split_events.go`
- Create: `internal/desktopui/pdf_merge_split_jobs.go`
- Modify: `internal/desktopui/pdf_merge_split_layout.go`
- Modify: `internal/desktopui/pdf_merge_split_state.go`
- Modify: `internal/desktopui/pdf_merge_split_test.go`
- Consume: `internal/application/pdf_utility.go`

**Interfaces:**
- Produces picker/job event handling called once per frame before the page branch is laid out.
- The PDF chooser calls existing `chooseOfficeFile` with `AllowedExtensions: []string{"pdf"}`; repeated calls add merge inputs one at a time. The output chooser calls existing `chooseOfficeFolder`.
- Produces a buffered `pdfMergeSplitJobResult` channel tagged with job generation and mode; workers call only the neutral application Runtime methods.

- [ ] **Step 1: Write `TestPDFMergeSplitToolFileFlow`, `TestPDFMergeSplitToolReorderAndRangePreview`, and `TestPDFMergeSplitToolPickerAndFolderCancel`** for adding/removing/clearing PDFs, metadata display, merge reorder, range preview count, output folder cancel, range/single-page submission, merge submission, and progress/error/partial-result rendering.
- [ ] **Step 2: Add `TestPDFMergeSplitToolIgnoresStaleJobResults`** to cancel on tool exit/shutdown and ignore late worker results after the owning generation changes.
- [ ] **Step 3: Run the targeted UI tests to confirm they fail.**

  Run: `go test ./internal/desktopui -run '^TestPDFMergeSplitTool' -count=1`

  Expected: FAIL because file and operation event handling is not wired.
- [ ] **Step 4: Add native file and folder selection** through the existing picker helpers, filtering inputs to PDF. Add merge files one at a time, inspect page count/size off-layout through `Runtime`, and show actionable errors without adding unreadable files.
- [ ] **Step 5: Implement merge list behavior and split preview.** Reorder merge rows with persistent row identity and the repository's Gio drag-coordinate pattern. Parse range editor changes at the event boundary through `ParsePDFUtilityPageRanges`; update output count without doing file I/O in layout. Show merged page count and source-byte-size sum as the summary, and show one output for every valid range token or every source page in page-by-page mode.
- [ ] **Step 6: Implement asynchronous operation lifecycle.** Start `launchManagedBackground` jobs with `context.WithCancel`, publish buffered results and invalidate the owning window on completion. Disable mode/list/back mutations while busy; allow cancellation. Reject late results by generation, retain committed partial outputs, and show their local paths.
- [ ] **Step 7: Implement destination and filename flow.** Ask for the output folder before processing; use the merge filename field for one PDF and a source-basename default prefix plus `-01` numbering for split files. Keep the drop zone clickable and use chooser instructions: the current Gio public app API does not expose a cross-platform external-file-drop event.
- [ ] **Step 8: Run the targeted UI tests.**

  Run: `go test ./internal/desktopui -run '^TestPDFMergeSplitTool' -count=1`

  Expected: PASS.
- [ ] **Step 9: Commit the complete workbench interactions and tests.**

### Task 6: Record the support boundary and verify the delivered feature

**Files:**
- Modify: `docs/office-support.md`
- Modify: `docs/office-library-evaluation.md`
- Modify: `docs/compatibility/office-feature-matrix.md`
- Consume: all previous tasks.

**Interfaces:**
- Documentation records only behavior demonstrated by the implementation and focused checks; it does not claim complete PDF standard compatibility.

- [ ] **Step 1: Update PDF support documentation** with merge, range split, page-by-page split, local-folder output, encrypted/repaired rejection, and the accepted loss of internal links/non-URI annotations/signature validity.
- [ ] **Step 2: Update the library evaluation and compatibility matrix** with the exact gopdf API path exercised by the new tests, source immutability, output reopen/page-count evidence, and known preservation gaps.
- [ ] **Step 3: Run the focused package checks from the authorized repository directory.**

  Run: `go test ./internal/format/pdfdoc -run '^(TestParsePageRanges|TestPDFUtility)' -count=1`

  Run: `go test ./internal/application -run '^TestRuntimePDFUtility' -count=1`

  Run: `go test ./internal/desktopui -run '^TestPDFMergeSplitTool' -count=1`

  Expected: all commands PASS without creating a separate cache or checkout.
- [ ] **Step 4: Review the actual Glance interface.** First terminate existing Glance and `go run ./cmd/glance` processes, then launch only `GLANCE_PPROF_ADDR=127.0.0.1:6060 go run ./cmd/glance` from `/Users/mima1234/project/glance-go`. Check the Tools entry, both modes, back navigation, folder-save success, and validation/error states. Clean any temporary verification screenshot/output from the repository; use synthetic PDFs only.
- [ ] **Step 5: Run `git diff --check` and review the three documentation changes.**
- [ ] **Step 6: Commit the compatibility record and final verified feature.**
