# Office Render Compatibility Corpus and Runner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Check in representative Word, Excel, and PowerPoint files and add a headless batch command that reports open/render outcomes, screenshots, and comparable performance observations.

**Architecture:** Keep interactive opening unchanged. Add an exported headless document/render seam inside `internal/desktopui` that uses the existing adapters and workspace layout. Put manifest validation, input discovery, report generation, image comparison, performance comparison, and CLI parsing in `internal/officeqa`; adapt the desktop renderer at `cmd/glance` so the QA package can be tested with a fake renderer.

**Tech Stack:** Go 1.27, Gio 0.10.2 `gpu/headless`, existing XLSX/DOCX/PPTX adapters, standard `flag`, `encoding/json`, `image/png`, `runtime`, and existing `golang.org/x/sys`.

**Spec:** `docs/superpowers/specs/2026-10-09-office-render-compatibility-design.md`

## Global Constraints

- Keep `./glance <file>` interactive and read-only QA runs must never save source documents.
- Store only selected `.xlsx`, `.docx`, and `.pptx` files; do not commit the temporary acquisition script, upstream version/source annotations, or download cache.
- Do not add persistent renderer/download dependencies; reuse Gio headless and the existing format adapters.
- Run batch work outside Gio's native `app.Main()` loop and outside layout/frame code.
- Keep report memory labels exact: per-file Go heap/allocation observations are not per-file peak RSS; RSS is run-level.
- A nonblank screenshot is only a smoke result; only reviewed screenshots get baselines.
- Compare performance reports only when corpus fingerprint and rendering environment match; do not add performance pass/fail thresholds.
- Bound input-side work before parsing: manifest reads stop at 2 MiB, oversized documents are rejected without a full-content fingerprint read, and PNG dimensions are validated before decoding.

## Review Focus

- Malformed or unexpectedly large Office files must be reported as failures without stopping later files; pin this in the runner failure-continuation and bounds tests.
- Directory traversal must be deterministic and must not escape through symlinks or manifest paths; pin this in discovery and manifest tests.
- Missing headless/GPU capability must be an explicit run-level failure, never a successful zero-image report; pin this in the headless runner test.
- Multi-slide PPTX files must produce one target per slide, while XLSX/DOCX coverage is accurately labeled as first viewport/page; pin this in renderer tests.
- Reports from different corpora or rendering environments must not be presented as directly comparable; pin this in report comparison tests.

---

## File Map

- `testdata/office-corpus/{xlsx,docx,pptx,regressions,baselines}/`: selected binary documents and reviewed baseline PNGs; no upstream-source directory labels.
- `testdata/office-corpus/README.md`, `manifest.json`: sample categories, case IDs, expected outcomes, optional baseline mappings, and usage instructions only.
- `.gitignore`: narrow exception for corpus `.xlsx` files and ignore generated `artifacts/render-run/` output.
- `internal/desktopui/headless_render.go`: open/close a document for headless rendering and capture the same application layout.
- `internal/desktopui/headless_render_test.go`: adapter-backed XLSX/DOCX/PPTX render smoke tests.
- `internal/officeqa/corpus.go`: manifest types/validation, supported extensions, deterministic discovery, and corpus fingerprint.
- `internal/officeqa/corpus_test.go`: manifest/discovery/fingerprint unit tests.
- `internal/officeqa/runner.go`: serial batch orchestration, per-file stages, screenshot/report output, continuation after individual failures.
- `internal/officeqa/runner_test.go`: fake-renderer tests for reports, image outputs, failures, and read-only behavior.
- `internal/officeqa/report.go`: versioned JSON report and exact performance metric fields.
- `internal/officeqa/compare.go`: compatible-report deltas and optional baseline image comparison.
- `internal/officeqa/compare_test.go`: metric deltas, environment mismatch, and baseline mismatch tests.
- `internal/officeqa/cli.go`: `qa render` and `qa compare` parsing/exit codes, independent of native app startup.
- `internal/officeqa/cli_test.go`: argument parsing and command result tests.
- `cmd/glance/main.go`: dispatch `qa` before starting Gio, preserve existing positional-file behavior.
- `cmd/glance/renderer.go`: adapt `desktopui.HeadlessDocument` to the `officeqa.Renderer` interface.
- `docs/compatibility/office-render-qa.md`: one-time corpus intake, command examples, output interpretation, and environment requirements.

## Interfaces

`internal/desktopui` owns the UI-coupled headless boundary:

```go
type HeadlessRenderOptions struct {
	Width, Height int
	Scale         float32
}

type HeadlessTarget struct {
	Name  string
	Image *image.RGBA
}

func OpenHeadlessDocument(path string) (*HeadlessDocument, error)
func (d *HeadlessDocument) Render(ctx context.Context, opts HeadlessRenderOptions) ([]HeadlessTarget, error)
func (d *HeadlessDocument) Close() error
```

`internal/officeqa` owns the renderer-neutral batch boundary:

```go
type Target struct {
	Name  string
	Image image.Image
}

type Document interface {
	Render(context.Context, Viewport) ([]Target, error)
	Close() error
}

type Renderer interface {
	Open(context.Context, string) (Document, error)
}

func Run(context.Context, RunConfig, Renderer) RunReport
func DispatchCLI([]string, Renderer, io.Writer, io.Writer) (handled bool, exitCode int)
func CompareReports(older, newer RunReport) (ReportComparison, error)
```

The `cmd/glance` adapter maps desktopui targets to `officeqa.Target`; neither `desktopui` nor document adapters import `officeqa`. Default viewport is 1280 x 800 at scale 1.0. `qa render` returns 0 only when all manifest expectations and configured baselines pass, 1 for run/file/render/baseline failures or unmet expectations, and 2 for invalid command usage. `qa compare` returns 0 for compatible reports and 2 for unreadable or incompatible reports.

---

## Task 1: Curate Corpus and Validate Manifest

**Files:**
- Create: `internal/officeqa/corpus.go`
- Test: `internal/officeqa/corpus_test.go`
- Create: `testdata/office-corpus/README.md`
- Create: `testdata/office-corpus/manifest.json`
- Create: selected `.xlsx`, `.docx`, and `.pptx` files under format directories
- Modify: `.gitignore`

**Interfaces:**
- Produces: `Case`, `Manifest`, and `LoadManifest(root string) (Manifest, error)` for runner tasks.

- [x] **Step 1: Write manifest validation tests**

Add `TestLoadManifestRejectsDuplicateIDsAndPaths`, `TestLoadManifestRejectsUnsupportedFormatsAndPathTraversal`, and `TestLoadManifestRequiresExistingCaseFiles`. Use temporary directory trees so the tests do not depend on downloaded fixtures.

- [x] **Step 2: Run the tests and verify they fail**

Run: `go test ./internal/officeqa -run 'TestLoadManifest' -count=1`
Expected: compile/test failure because the manifest types and loader do not exist.

- [x] **Step 3: Implement manifest validation**

Define manifest JSON fields as case ID, relative path, format, feature tags, expected outcome, and optional target-to-baseline paths. Reject duplicate IDs/paths, unsupported extensions, paths escaping the corpus root, missing files, and non-PNG baseline paths. Do not define upstream source, source version, or per-file provenance fields.

- [x] **Step 4: Run manifest tests and verify they pass**

Run: `go test ./internal/officeqa -run 'TestLoadManifest' -count=1`
Expected: PASS.

- [x] **Step 5: Acquire and curate actual Office files**

Create a disposable script under `/tmp` to retrieve candidate test data from Apache POI and LibreOffice Writer into separate temporary directories. Select representative, redistributable `.xlsx`, `.docx`, and `.pptx` files from POI, plus representative `.docx` files from LibreOffice Writer; place selected binaries under `testdata/office-corpus/{xlsx,docx,pptx}` and discard both the script and candidate trees. Store no source/version annotations. Populate the manifest with case IDs and tested feature categories.

- [x] **Step 6: Permit only corpus spreadsheets through `.gitignore`**

Add a path-specific negation after the global `**.xlsx` rule for `testdata/office-corpus/**/*.xlsx`; add `/artifacts/render-run/` to keep generated reports/screenshots out of Git. Verify with `git check-ignore -v` that corpus XLSX is not ignored but an unrelated temporary XLSX remains ignored.

- [x] **Step 7: Verify the curated asset set**

Run: `go test ./internal/officeqa -run 'TestLoadManifest' -count=1`
Run: `file testdata/office-corpus/xlsx/* testdata/office-corpus/docx/* testdata/office-corpus/pptx/*`
Expected: manifest tests pass and each selected asset is detected as its intended OOXML file type.

---

## Task 2: Add the Headless Desktop Renderer

**Files:**
- Create: `internal/desktopui/headless_render.go`
- Test: `internal/desktopui/headless_render_test.go`
- Reuse: `internal/desktopui/app.go`, `layout.go`, `presentation_view.go`, and the existing `gioui.org/gpu/headless` test patterns.

**Interfaces:**
- Produces: `OpenHeadlessDocument`, `HeadlessDocument.Render`, and `HeadlessDocument.Close` as defined above.

- [x] **Step 1: Write renderer smoke tests**

Add `TestHeadlessRenderXLSXFirstWorksheet`, `TestHeadlessRenderDOCXFirstViewport`, `TestHeadlessRenderPPTXEverySlide`, and `TestHeadlessRenderMalformedDocument`. Assert target names/counts, expected viewport dimensions, and a nonblank pixel count; skip only when Gio headless is unavailable in the test environment.

- [x] **Step 2: Run the renderer tests and verify they fail**

Run: `go test ./internal/desktopui -run 'TestHeadlessRender' -count=1`
Expected: compile failure because the headless renderer API is absent.

- [x] **Step 3: Implement open/render/close lifecycle**

Use `loadDocument` followed by `App.installDocument` so parsing goes through the same adapters as interactive open. Reuse `App.layout` with fixed constraints/metrics and `headless.NewWindow`; capture the first spreadsheet viewport and first Word viewport, and switch the presentation view through every slide. Keep `runtime.LockOSThread` scoped to Gio rendering and close the headless window/document on all paths. Do not start `app.Main()` or use a native `app.Window`.

- [x] **Step 4: Run renderer tests and verify they pass**

Run: `go test ./internal/desktopui -run 'TestHeadlessRender' -count=1`
Expected: PASS or explicit environment skip only for unavailable headless device; normal format/render errors fail.

---

## Task 3: Implement Corpus Discovery, Rendering, and JSON Reports

**Files:**
- Create: `internal/officeqa/report.go`
- Create: `internal/officeqa/runner.go`
- Test: `internal/officeqa/runner_test.go`
- Reuse: `internal/officeqa/corpus.go`

**Interfaces:**
- Consumes: `Manifest` from Task 1 and renderer-neutral `Renderer`/`Document` interfaces above.
- Produces: `RunConfig`, `RunReport`, `DocumentResult`, `TargetResult`, and `Run(ctx, config, renderer)`.

- [x] **Step 1: Write discovery, report, and fake-renderer tests**

Add tests for a single file and recursively discovered directory, deterministic ordering, case-insensitive supported extensions, unknown-file omission, relative report paths, stable corpus fingerprint, fingerprint change when bytes change, screenshot output, blank screenshot failure, and continuation after one document fails to open/render.

- [x] **Step 2: Run the runner tests and verify they fail**

Run: `go test ./internal/officeqa -run 'Test(Discover|Run|CorpusFingerprint)' -count=1`
Expected: compile/test failure because runner/report APIs are absent.

- [x] **Step 3: Implement discovery and corpus fingerprint**

Walk directories in sorted order without following symlinks. Resolve corpus cases against the validated manifest. Hash sorted relative paths and file bytes into one automatic SHA-256 corpus fingerprint; do not store per-file upstream metadata.

- [x] **Step 4: Implement serial render and report output**

For each input, measure open/parse, first-target, and total render durations; capture Go `TotalAlloc` delta and `HeapAlloc` snapshots; write target PNGs under a safe case-ID directory; test nonblank pixels; record diagnostics and continue to later files after errors. Save a schema-versioned `report.json` even when some files fail. Preserve input bytes.

- [x] **Step 5: Add process peak RSS reporting**

Implement `peakProcessRSS() (bytes uint64, available bool, err error)` behind OS-specific files for Darwin, Linux, and Windows, with an explicit unsupported fallback. Record RSS once per run, never as per-file peak memory. Add unit coverage for report serialization with the metric available and unavailable.

- [x] **Step 6: Run runner tests and verify they pass**

Run: `go test ./internal/officeqa -run 'Test(Discover|Run|CorpusFingerprint)' -count=1`
Expected: PASS.

---

## Task 4: Add Baseline and Version-Report Comparison

**Files:**
- Create: `internal/officeqa/compare.go`
- Test: `internal/officeqa/compare_test.go`
- Create: reviewed baseline PNGs only for selected cases under `testdata/office-corpus/baselines/`
- Modify: `testdata/office-corpus/manifest.json`

**Interfaces:**
- Consumes: `RunReport`, case baseline mappings, and PNG targets from Task 3.
- Produces: `CompareReports(older, newer RunReport) (ReportComparison, error)` and baseline comparison results.

- [x] **Step 1: Write report and image comparison tests**

Add `TestCompareReportsReportsPerFileAndAggregateDeltas`, `TestCompareReportsRejectsCorpusOrEnvironmentMismatch`, `TestCompareBaselineDetectsChangedPixels`, and `TestMissingBaselineIsNotVisualPass`. Use tiny generated PNGs and report values.

- [x] **Step 2: Run comparison tests and verify they fail**

Run: `go test ./internal/officeqa -run 'Test(Compare|MissingBaseline)' -count=1`
Expected: compile/test failure because comparison APIs are absent.

- [x] **Step 3: Implement report comparison**

Compare per-file open/render/total-allocation values and aggregate values only when corpus fingerprint, Go version, OS/architecture, viewport, and scale match. Allow Glance build version to differ. Return an explicit compatibility error rather than silently comparing mismatched reports.

- [x] **Step 4: Implement reviewed PNG baseline comparison**

Decode the declared PNG and compare same-sized images pixel-by-pixel; report changed-pixel count and ratio. A size mismatch is a failed baseline. For cases without a baseline, report `not_configured`, not `pass`. Treat a configured mismatch as a run failure.

- [x] **Step 5: Run comparison tests and verify they pass**

Run: `go test ./internal/officeqa -run 'Test(Compare|MissingBaseline)' -count=1`
Expected: PASS.

- [x] **Step 6: Capture and review selected baselines**

Run the current headless renderer for one representative sample of each format, inspect the PNGs, and only then add the accepted images and manifest mappings. Do not baseline every sample automatically.

---

## Task 5: Wire CLI Commands Without Changing Interactive Open

**Files:**
- Create: `internal/officeqa/cli.go`
- Test: `internal/officeqa/cli_test.go`
- Create: `cmd/glance/renderer.go`
- Test: `cmd/glance/main_test.go`
- Modify: `cmd/glance/main.go`

**Interfaces:**
- Consumes: `Run`, `CompareReports`, and renderer interface from Tasks 2–4.
- Produces: `DispatchCLI(args, renderer, stdout, stderr) (handled bool, exitCode int)`.

- [x] **Step 1: Write CLI dispatch tests**

Add `TestDispatchQARenderParsesInputAndOutput`, `TestDispatchQACompareLoadsTwoReports`, `TestDispatchInvalidQAUsageReturnsTwo`, and a command-routing test proving a normal positional file is not consumed as a QA command.

- [x] **Step 2: Run CLI tests and verify they fail**

Run: `go test ./internal/officeqa ./cmd/glance -run 'Test(Dispatch|QARender|QACompare|InvalidQA)' -count=1`
Expected: compile/test failure because the CLI dispatcher is absent.

- [x] **Step 3: Implement QA dispatch and desktop renderer adapter**

Implement `qa render --input PATH --out DIR [--width 1280 --height 800 --scale 1]` and `qa compare OLDER.json NEWER.json` using the standard library flag parser. Map exit codes exactly as in the Interfaces section. Adapt desktopui's `HeadlessDocument` in `cmd/glance/renderer.go`.

- [x] **Step 4: Route QA before Gio startup**

In `main`, call `DispatchCLI(os.Args[1:], ...)` before `desktopui.Run`/`app.Main`. Only exit for handled `qa` commands. Preserve the existing no-argument and positional-path desktop behavior.

- [x] **Step 5: Run CLI tests and verify they pass**

Run: `go test ./internal/officeqa ./cmd/glance -run 'Test(Dispatch|QARender|QACompare|InvalidQA)' -count=1`
Expected: PASS.

---

## Task 6: Document Usage and Verify the Whole Workflow

**Files:**
- Create: `docs/compatibility/office-render-qa.md`
- Modify: `testdata/office-corpus/README.md`
- Verify: all files from Tasks 1–5.

- [x] **Step 1: Document corpus and commands**

Document the one-time selection rule, how to add a new regression file without source/version tags, build command `make desktop`, render command `./dist/glance-office qa render --input testdata/office-corpus --out artifacts/render-run`, report comparison command, metric semantics, and how to interpret open/render failures versus unsupported diagnostics and unbaselined screenshots.

- [x] **Step 2: Verify one-file and full-corpus command paths**

Run: `./dist/glance-office qa render --input testdata/office-corpus/xlsx --out artifacts/render-run/xlsx`
Run: `./dist/glance-office qa render --input testdata/office-corpus --out artifacts/render-run/all`
Expected: valid report JSON and expected screenshot targets; source files remain unchanged.

- [x] **Step 3: Verify report comparison and compatibility rejection**

Run `qa compare` with two same-environment reports and with a report from a modified corpus fingerprint.
Expected: deltas print for compatible reports; mismatched corpus is rejected with no misleading delta.

- [x] **Step 4: Run full verification**

Run: `go test ./...`
Run: `make desktop`
Run: `git diff --check`
Result: focused Office QA/CLI tests, headless smoke tests, `go vet ./internal/officeqa ./cmd/glance`, `make desktop`, Windows cross-compilation, and `git diff --check` pass. `go test ./...` has only the approved pre-existing `TestCellEditorUsesRenderedStyleAndKeepsSingleCellGeometry` failure (`grid_paint_test.go:93`, editor size 12). A real corpus run produced 9 results (7 rendered, 2 with expected diagnostics, no open/render/baseline failures or expectation mismatches), and its report compared successfully with the prior same-environment run.

- [ ] **Step 5: Commit the completed implementation**

Commit focused changes after each task; finish with a clean `git status` and a final compatibility run report saved under the versioned output directory.
