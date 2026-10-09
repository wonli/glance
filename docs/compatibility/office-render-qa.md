# Office Render QA

The compatibility corpus contains 50 real Word, Excel, and PowerPoint documents (17 DOCX, 17 XLSX, and 16 PPTX). `qa render` opens them through Glance's existing adapters and application layout, captures a fixed headless viewport, and writes `report.json` plus PNG targets. It is a rendering smoke test, not a claim of Office parity. Input files are never saved or modified.

## Run

Build the desktop binary, then render the full corpus or one supported file/directory:

```sh
make desktop
./dist/glance-office qa render --input testdata/office-corpus --out artifacts/render-run/v1
./dist/glance-office qa render --input testdata/office-corpus/xlsx --out artifacts/render-run/xlsx
./dist/glance-office qa render --input path/to/document.docx --out artifacts/render-run/document
```

The default viewport is 1280 x 800 at scale 1. It can be changed with `--width`, `--height`, and `--scale`, within the renderer's image limits. XLSX and DOCX currently capture the first visible worksheet/page viewport. PPTX captures each slide separately. Reports use paths relative to the input root. The corpus fingerprint hashes sorted paths and document bytes up to the 256 MiB per-file limit; an oversized file is rejected without reading its contents for the fingerprint, which binds that entry to its path and size instead.

The existing interactive command is unchanged:

```sh
./dist/glance-office path/to/document.docx
```

## Reports

Each run writes `report.json` and a directory per case ID containing target PNGs. File outcomes are `rendered`, `completed_with_diagnostics`, `open_failed`, `render_failed`, or `not_run`. A headless-device failure is a run-level error and does not count as an empty successful render. The runner continues after an individual malformed or oversized document, up to 256 MiB per file and 10,000 discovered documents by default. Manifest input is capped at 2 MiB, and baseline/render images are checked against the renderer's 4096-pixel and 16-megapixel limits before full PNG decoding.

`open_duration_ms` covers the adapter open/parse step. `time_to_first_target_ms` measures from per-file processing start through the first captured target, so it includes open/parse time. `render_duration_ms` includes render, screenshot capture, and PNG writing, but excludes baseline comparison. `total_duration_ms` covers per-file processing through close and excludes baseline comparison. Go allocation/heap fields cover that same per-file processing window; they are Go-runtime observations, not total process memory. `process_peak_rss_bytes` is one OS process high-water measurement for the entire run, with an explicit availability flag; it is not a per-document peak.

Reports record Glance version/revision, Go version, OS/architecture, viewport, corpus fingerprint, timings, target dimensions, diagnostics, and expected outcomes. The command-line summary prints diagnostics beneath each affected file. A manifest case may list `expected_diagnostics`; every listed value must appear exactly in the report, while additional diagnostics are allowed. Missing expected diagnostics make the case unexpected and `qa render` exits nonzero. A nonblank image proves only that pixels were produced. Unsupported-feature diagnostics remain distinct from a rendering crash. Separately, `internal/desktopui/word_corpus_layout_test.go` runs the production Word shaper/layout engine over every curated DOCX and asserts each fixture has a previewable page scene.

## Baselines And Comparison

Only reviewed PNGs are configured as baselines. Pixel comparisons are exact: a changed pixel, dimension mismatch, missing configured file, or missing configured target is reported as a baseline failure. Targets without a baseline are `not_configured`; they are not marked as visually passing. New baseline images should be reviewed before adding their relative path to the case's `baselines` map in `testdata/office-corpus/manifest.json`.

Keep output directories separate when recording versions, then compare reports:

```sh
./dist/glance-office qa render --input testdata/office-corpus --out artifacts/render-run/v2
./dist/glance-office qa compare artifacts/render-run/v1/report.json artifacts/render-run/v2/report.json
```

`qa compare` reports newer-minus-older per-file and aggregate timing/allocation deltas. It rejects reports with different corpus fingerprints, Go/runtime platform, or viewport/scale, and permits the Glance build version to differ. Metrics are observational; there are no performance pass/fail thresholds. Repeated runs should use the same machine, fonts, and environment for useful comparisons.

## Latest Corpus Verification

On 2026-10-09, `go run ./cmd/glance qa render --input testdata/office-corpus --out /tmp/glance-go-officeqa-final` processed all 50 corpus files: 46 rendered without diagnostics, 4 completed with expected warnings, and there were no open failures, render failures, baseline failures, or unexpected outcomes. All 17 DOCX files produced previewable page scenes. The remaining DOCX warnings identify bounded fallbacks: cached field results, floating-image inline placement, unsupported embedded preview content, percentage-width table geometry, and image scaling. `go test ./... -count=1` passed in the same verification pass.

## Add A Regression

Place the actual `.xlsx`, `.docx`, or `.pptx` fixture in this corpus, preferably under `regressions/`, and add a manifest entry with a stable ID, relative path, format, feature tags, expected outcome, and concise notes. Add `expected_diagnostics` when a specific diagnostic is important to preserve; entries use the exact diagnostic strings printed by `qa render` (for example, `warning:floating-image-inline-fallback`). Keep source/version provenance out of per-file metadata. Do not include confidential or unnecessary personal content. The default directory scan includes every supported Office file, so a new regression is exercised automatically.

Exit codes: `0` means all file outcomes matched manifest expectations and configured baselines passed; `1` means a run/input/render/baseline failure or an unmet manifest expectation; `2` means invalid command usage or incompatible/unreadable comparison reports.
