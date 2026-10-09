# DOCX Corpus Compatibility Design

## Goal

Make every curated DOCX sample readable in the normal Word preview, including its tested special content. A localized limitation may remain visible for content whose exact Office behavior is not represented, but one unsupported inline element must not replace the document's entire page with an error card.

## Evidence and scope

The current headless run opens all 17 DOCX files, but 10 produce error diagnostics. `wordlayout` sets `CanPreview` false when a layout error exists, and the UI and PDF path consequently hide or reject the complete document. The affected corpus features are:

| Sample | Required content |
| --- | --- |
| `sample.docx`, `page-breaks.docx` | Ignore stored `lastRenderedPageBreak` hints, keep real page breaks, and render default/text run breaks as line breaks. |
| `numbered-lists.docx` | Render the sample's nested decimal and alphabetic numbering, including per-level marker substitution. |
| `footnotes.docx`, `endnotes.docx` | Show the reference and corresponding read-only note text from the related package parts. |
| `headers.docx` | Render each section's header on pages in that section; tolerate equivalent page geometry that differs only by OOXML rounding. |
| `floating-image-wrap.docx` | Resolve its VML image and lay it out with a stable text-wrap fallback. |
| `drawing-shapes.docx` | Preserve readable body text, nested-table content, percentage-width tables, and supported image/shape content. Unrenderable OLE parts must have a located visible fallback. |
| `long-table.docx` | Follow the file's actual AlternateContent group/textbox/chart structure. Correct corpus labels that currently describe it as a table. |

`field-codes.docx` already displays cached field results read-only. Keep the `cached-field-result` warning because Glance does not recalculate fields. Keep other warnings where they identify a real bounded-fidelity fallback.

## Design

- Extend the neutral document model only for semantics required by the corpus; keep OOXML package and relationship types inside `internal/format/docx`.
- Project soft line breaks, note references/content, supported numbering formats, section header content, and VML/DrawingML image references into neutral read-only data.
- Preserve the existing shared pagination model. Add placement data where a feature belongs to page/section geometry; do not fabricate a global body paragraph for page furniture.
- Keep nested-table and group/textbox source ordering. Use measured parent-cell geometry for percentages and recursive child-table layout. If an embedded Office object has no safe renderer, preserve its readable fallback text and emit a source-located warning.
- Do not suppress an error diagnostic merely to make a corpus status pass. Remove an error only after the sample's visible content is projected and the relevant layout behavior has a regression assertion.
- Update the compatibility manifest and support matrix only after the new behavior is verified. Correct the `long-table` feature tags to describe the actual fixture.

## Compatibility and safety

This work is read-only projection and rendering. It does not rewrite source DOCX packages, recalculate fields, or claim exact Word pagination, floating-object wrap, or arbitrary embedded-object fidelity. Existing unknown package parts and media must remain byte-preserved on save. Unsupported or malformed geometry must remain a located error if a visible fallback cannot be produced safely.

## Verification

- Projection tests assert source order, read-only status, note IDs/text, break kinds, numbering markers, section-to-header assignment, and image bytes.
- Layout tests assert that text and supported page elements land on the expected page/section and that corpus cases remain previewable.
- Run `go test ./internal/format/docx ./internal/wordlayout -count=1`, the DOCX-related desktop UI tests, and `go test ./... -count=1`.
- Run `go run ./cmd/glance qa render --input testdata/office-corpus --out <temporary-directory>` and inspect every DOCX result and every baseline change. The corpus should have no open/render failures or unexpected DOCX cases; remaining diagnostics must be warnings tied to visible fallbacks.
