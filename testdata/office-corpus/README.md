# Office Render Corpus

This directory contains 54 `.xlsx`, `.docx`, and `.pptx` documents for headless render compatibility checks: 17 spreadsheets, 21 word-processing documents, and 16 presentations. The manifest records case IDs, relative paths, formats, feature tags, expected outcomes, and optional screenshot baselines. It intentionally has no per-document source or version fields.

Build and run the corpus from the repository root:

```sh
make desktop
./dist/glance-office qa render --input testdata/office-corpus --out artifacts/render-run
```

Add an actual regression document under `regressions/` or its format directory, preserve its bytes, and add a manifest entry with a unique ID, format, relevant feature tags, and expected outcome. Baseline screenshots belong under `baselines/` only after a person has reviewed them. Targets without a configured baseline are reported as `not_configured`, not as visual passes.

Avoid confidential or unnecessary personal data. A successful nonblank render is only a smoke result, not proof of full Office fidelity; known unsupported content should use `completed_with_diagnostics` when that is the expected behavior.
