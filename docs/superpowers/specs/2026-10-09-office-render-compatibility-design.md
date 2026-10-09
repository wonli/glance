# Office Render Compatibility Corpus and Runner

**Status:** Scope agreed on 2026-10-09; expanded specification awaits review. Implementation is not yet approved.

## Problem

Glance-Go has focused adapter and Gio headless tests, but no workflow that opens a collection of real Office documents through the application and records whether each document opens and renders. The existing `cmd/glance` positional argument starts the interactive desktop application; it is not a batch renderer.

The compatibility test set must consist of actual Word, Excel, and PowerPoint documents checked into the repository. Apache POI and LibreOffice Writer have candidate test files, but their full upstream collections are not Glance-Go's acceptance corpus and must not be mirrored wholesale.

## Goal

Create a curated, checked-in corpus of representative `.docx`, `.xlsx`, and `.pptx` files, then provide a non-interactive command that opens those files through Glance-Go's document/rendering path, saves per-file screenshots and a machine-readable report, and returns a useful process status. Future documents that expose regressions can be added to the same corpus and included automatically.

The test flow is read-only: it must not save or modify input files.

## Chosen approach

Use a one-time, temporary acquisition script during implementation to retrieve candidate files from Apache POI and LibreOffice into temporary directories. Review and select representative files from that temporary source material, then check only the selected binary documents into the project. Do not commit the acquisition script or add persistent download/cache infrastructure.

Add an explicit command such as:

```text
./glance qa render --input ./testdata/office-corpus --out ./artifacts/render-run
```

Keep the existing `./glance path/to/document` behavior as interactive desktop open. The batch command must be routed before Gio's native `app.Main()` loop and render without opening a native desktop window per file.

The headless path should reuse Glance-Go's format adapters and supported document presentation/layout code rather than creating a second independent Office renderer. Keep batch orchestration, corpus metadata, and report serialization outside the normal layout/frame loop. Any extracted shared code must remain neutral and should not pull third-party Office package types into widgets.

The alternatives are: launch a separate native UI process for every document, which exercises the visible app but adds startup cost and platform-window flakiness; or render each adapter directly in a separate test-only renderer, which is simpler but can diverge from the actual workspace view. The selected in-process headless path keeps one command and the real Glance rendering path without requiring native windows.

## Corpus selection and organization

The initial corpus covers current supported OOXML inputs: `.xlsx`, `.docx`, and `.pptx`. Candidate files are selected once from Apache POI `test-data` for Excel, Word, and PowerPoint and LibreOffice Writer filter-test fixtures for Word. A temporary acquisition script can retrieve candidate files into a temporary directory; the script and downloaded candidate trees are discarded after selection. Do not keep per-file upstream repository, commit, version, or source-path annotations.

Select a compact set across rendering-relevant cases already described as supported or partial in `docs/office-support.md`; do not treat every upstream fixture as expected to render correctly. Every committed document has a minimal manifest entry with:

- Stable case ID and repository-relative path.
- Format and feature tags.
- Expected outcome category and concise notes about the behavior being exercised.

Only add samples whose redistribution basis is clear and which contain no unnecessary personal or confidential information. Preserve selected sample bytes as test fixtures. New regression samples can be added directly to the corpus with a case ID and expected behavior; do not add files containing unnecessary personal or confidential information.

The corpus belongs under a top-level `testdata/office-corpus/` directory so Go tooling treats it as test data. Add a narrow `.gitignore` exception for `.xlsx` files in this directory; unrelated spreadsheets remain ignored.

## Batch render flow

1. Parse `qa render` options without starting the desktop event loop. Accept either a single supported document or a directory; directory traversal is deterministic and includes supported extensions only.
2. Read-only open each file with its existing format adapter and build the same document view used by the application.
3. Render at fixed viewport dimensions and scale using Gio headless rendering. Produce at least one screenshot per render target: the first worksheet viewport for XLSX, first document page/viewport for DOCX, and each slide for PPTX. Report the target count so partial render coverage is visible.
4. Record file identity, open/render status, elapsed times, warnings/unsupported diagnostics, screenshot paths, dimensions, renderer parameters, and errors. Use paths relative to the input root in reports; do not copy document contents into JSON. Automatically compute a corpus fingerprint from sorted relative file paths and file bytes.
5. Classify results separately as open failure, render failure, completed-with-diagnostics, and rendered. A nonblank screenshot is a smoke check, not proof of visual fidelity.
6. Compare screenshots against checked-in baselines only for cases that have an explicitly reviewed baseline. For other samples, save images and mark visual comparison as not configured; do not silently call them visual passes.
7. Return non-zero for open/render failures or failed configured baseline checks. Unsupported-feature diagnostics remain visible and are not automatically treated as a renderer crash.

Run sequentially initially to keep Gio/font/render resources predictable. Bound output file names, image dimensions, and any per-file work so a malformed or unexpectedly large sample cannot consume unbounded resources.

## Proposed layout

```text
testdata/office-corpus/
  README.md
  manifest.json
  xlsx/
  docx/
  pptx/
  regressions/
  baselines/                 # only reviewed expected screenshots
```

```text
artifacts/render-run/
  report.json
  <case-id>/
    target-001.png
```

Generated run artifacts are not committed by default; the output directory is covered by a narrow `.gitignore` entry. Each future regression document is added to `regressions/` with a matching manifest entry and becomes part of the default corpus scan.

## Report and error behavior

The run report includes a schema version, Glance build/version identifier, UTC run time, Go version, operating system/architecture, viewport/scale, corpus fingerprint, aggregate counts, and per-file results. Each per-file result includes relative path, detected format, timings, diagnostic messages, screenshot/target list, and baseline comparison status when applicable. The report records the Glance version for comparisons, but does not annotate individual test documents with their upstream source or version.

Continue after an individual file failure so the run reports the whole corpus. Report read, parse, font/layout, headless-device, screenshot-write, and baseline errors distinctly. A missing/unavailable headless device is a run-level failure, not a success with zero images. Preserve partial screenshots/report data when a later file fails.

## Performance measurements and comparison

Record per-file input size, open/parse duration, time to first rendered target, total render duration, and rendered target count. Record the Go `TotalAlloc` delta during each file's processing window and `HeapAlloc` before/after with their exact metric names. These are Go-runtime observations, not total process memory or per-file peak RSS. Record process peak RSS once per run where the platform exposes a reliable high-water measurement; for a run containing one file, that value can be compared for that file. Do not imply per-document peak RSS for a multi-file in-process run.

Include Glance version/commit, Go version, OS/architecture, viewport/scale, and the automatically computed corpus fingerprint in each report. Provide `qa compare <older-report.json> <newer-report.json>` to print per-file and aggregate timing/memory deltas between two saved reports. Require matching corpus fingerprint and rendering environment for a like-for-like comparison; show a mismatch explicitly instead of presenting the numbers as directly comparable. Performance results are observational initially, with no pass/fail threshold until repeatability has been measured on a controlled environment. Generated reports can be retained under a versioned output name for later comparison; they are not committed by default.

## Non-goals

- Full mirrors of Apache POI or LibreOffice test data.
- Committing the temporary acquisition script, a persistent source download command, or an external fixture cache.
- Rendering formats Glance-Go does not currently open, or changing application feature/UI behavior.
- Claiming Excel, Word, or PowerPoint parity from a successful open or nonblank screenshot.
- Automatically baselining every public sample without human review.
- Saving, converting, or modifying source documents during a compatibility run.

## Acceptance criteria

- The repository contains actual, curated `.docx`, `.xlsx`, and `.pptx` files selected once from the candidate collections; the stored directory layout and manifest do not record upstream source or version for individual documents.
- The command processes one file and a directory without starting Gio's native event loop, while `./glance <file>` remains an interactive open.
- Each accepted file is opened through its normal format adapter and produces an output screenshot for the declared target(s) or an explicit failure result.
- A JSON report covers every input and distinguishes open failures, render failures, warnings/unsupported content, and baseline outcomes.
- Reports identify the Glance build and corpus fingerprint, include per-file open/render timings and accurately labeled memory observations, and can compare two compatible saved reports.
- Reviewed baseline checks detect changed screenshots; samples without a baseline are clearly reported as unbaselined.
- Input files remain byte-for-byte unchanged. Bad files do not stop reporting for subsequent corpus entries.
- The smoke corpus and one run can be exercised locally with a documented command. The implementation does not add persistent download/cache tooling or change product UI.

## Verification

Use small generated fixtures for deterministic report, traversal, failure-continuation, and baseline tests. Run the batch command against the curated corpus and inspect its JSON plus screenshots for XLSX, DOCX, and every PPTX slide. Confirm that the corpus fingerprint is stable across repeated runs and changes when a sample changes. Run `go test ./...` and `git diff --check`; document any platform-specific headless limitation instead of counting it as a pass.
