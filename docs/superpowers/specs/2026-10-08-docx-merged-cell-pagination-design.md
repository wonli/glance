# DOCX Merged-Cell Pagination Design

**Status:** Approved for specification by the user on 2026-10-08. Implementation plan and code changes require review of this document first.

## Goal

Open the reported Chengdu Health Commission DOCX in the normal paginated view, including its merged tables. The DOCX adapter must remain read-only for layout projection, and screen preview, PDF export, and native print must continue consuming the same neutral page scenes.

The reported document has three sections with identical projected page geometry and zero-valued character indents. Those cases must not block layout. Its three merged tables use horizontal `w:gridSpan` spans from two to five grid columns and vertical `w:vMerge` restart/continue chains. Continuation cells in those chains contain no text, and the chains match their originating grid columns.

## Design

### Neutral table model

Extend `document.WordTableCell` with a horizontal grid span and a vertical merge state. A missing grid span means one grid column. Vertical merge states are `none`, `restart`, and `continue`; the model contains no OOXML node or library type. Existing table consumers that do not inspect merge state retain their current behavior for unmerged cells.

### DOCX projection

Project `w:gridSpan` and `w:vMerge` into the neutral cell fields. An absent `w:vMerge/@w:val` means `continue`, as specified by WordprocessingML. Valid spans and merge chains are no longer emitted as unsupported diagnostics. Invalid or out-of-grid spans, orphan or mismatched continuations, and unsupported legacy `w:hMerge` remain located errors. Nested tables and other unsupported structures remain unchanged.

Zero `w:firstLineChars` or `w:hangingChars` values remain explicit zero indents and do not block layout. Nonzero character-unit indents remain diagnosed until the renderer can derive their width reliably.

### Shared pagination scene

The table layout engine will use the source grid as the column coordinate system. It will place each cell at its starting grid column, calculate its width by summing the covered column widths, and advance by its span. If grid widths are missing, existing inferred-width behavior will divide a cell's known width over its spanned grid columns before normalizing to the available table width.

The engine will precompute row content heights before placement so it can determine each vertical merge's full row extent. A restart cell owns the merged content; a continuation cell must not contain text and contributes no duplicate content. Cell placements will retain row-level rectangles so a merge can continue across pages. Placement edge flags will suppress horizontal borders inside a vertical merge while preserving the outer edges and each page segment's visible boundary. Rows continue to move as indivisible units under the current page-flow behavior.

The new page-scene geometry is shared by Gio, PDF export, and native print. PDF and Gio border drawing will honor the new edge flags. No DOCX package bytes are rewritten by projection or pagination.

### Error handling and limits

Malformed merge geometry fails closed with a located diagnostic and blocks faithful paginated output. Unsupported `w:hMerge`, nested tables, and nonempty continuation cells remain errors rather than being silently flattened. Different section page geometries and unsupported section transitions remain outside this change. This work adds viewing and pagination only; it does not add merge editing or change save capabilities.

## Verification and acceptance

- DOCX projection tests cover default span 1, explicit horizontal spans, `vMerge` restart/continue, omitted `vMerge` value, malformed spans, orphan/mismatched continuations, and preserved snapshot bytes.
- Word layout tests assert the x-position and width of cells spanning multiple grid columns, vertical merge border edges, row-height aggregation, and merge segments across page boundaries.
- Gio and PDF tests verify that page-scene edge flags suppress internal borders without removing outside borders.
- Existing unmerged table, row-splitting, PDF, and print tests continue to pass.
- A local acceptance run opens the user-provided DOCX without adding it to the repository, confirms page scenes are accepted, and checks that the three tables produce merged geometry without `table-grid-span` or `table-vertical-merge` errors.

## Compatibility record

The change expands the read-only DOCX layout projection and the neutral `WordTableCell`/`TableCellPlacement` contracts. It does not change third-party Office dependencies, package mutation, file overwrite behavior, or save/reopen guarantees. The compatibility matrix and `docs/office-support.md` will be updated with the tested span and merge subset after implementation.
