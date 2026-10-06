# Markdown editor design

## Intent and approved scope

Implement local Markdown support in Glance, following Pen frame `VvYgo` and the complete toolbar `ybPPK`. The user approved all toolbar actions and explicitly requested reuse of the Excel ribbon implementation experience. Work takes place in the isolated `markdown-editor` worktree.

## Architecture

Extend the existing document/session lifecycle with a Markdown adapter rather than treating Markdown as JSON or DOCX. `internal/format/markdowndoc` owns exact source text, dirty state, snapshots, parsing, and atomic saves. Neutral Markdown blocks, inline spans, heading locations, and diagnostics live under `internal/document`. Gio owns persistent editor, search, list, and button state. Controller operations publish revision-tagged results; stale parse or export results cannot replace a newer document or draft.

Source text is authoritative. Preview, outline, and PDF are derived views and never rewrite the source. Preserve UTF-8, BOM and existing line endings on untouched round trips; reject undecodable input explicitly. New documents use UTF-8. Save through a temporary sibling file and replace only after successful completion. A failed save retains the draft and dirty state.

## Workspace integration

Support `.md` and `.markdown`, case insensitive, through new-document actions, the homepage, file dialogs, folder imports, recent files, tab switching, save, save-as, and restart recovery. Markdown uses its own format identity and exact source snapshot. Switching tabs, modes, saving, exporting, and closing must synchronize current editor drafts without committing incomplete IME composition. No-op synchronization must not mark a document dirty. Restore source and view state independently.

## Pen layout and Excel ribbon reuse

The 1440 × 900 reference has a 42 dp document strip, 54 dp toolbar, a 248 dp outline, and a 34 dp status bar. Reuse the existing workspace document strip instead of recreating the mock tabs. The body has a white writing surface and a subdued outline background, with a centered reading column approximately 768 dp wide.

Reuse `ribbonAction`, `layoutToolbarActionItem`, `toolbarIconButton`, SVG tint/raster helpers, and persistent horizontal lists from Excel/JSON where suitable. Maintain fixed icon and caption slots, clear inherited constraints before captions, use real icons rather than text glyph substitutes, and keep group dividers aligned. The Markdown toolbar remains a single 54 dp row. Actions scroll horizontally on narrow windows, while mode buttons remain accessible; the outline can collapse. Validate both standard and high-DPI metrics. Do not introduce an Excel category-tab row into Markdown.

## Complete toolbar behavior

| Action | Behavior |
| --- | --- |
| 打开 | Existing asynchronous file picker and document installation |
| 保存 | Flush draft, validate destination, atomically save |
| 另存为 | Choose Markdown destination and update session identity on success |
| 导出 PDF | Export current derived document with Chinese text, images and formulas |
| 标题 | Apply/change an ATX heading on selected lines |
| 加粗 / 斜体 | Wrap selected text; insert delimiters with editable selection when empty |
| 引用 | Prefix selected lines with quote markers |
| 代码 | Inline code for single-line text and fenced code for multiline text |
| 链接 | Collect label and destination; insert Markdown link at retained selection |
| 图片 | Choose local image or enter destination/alternative text; insert image syntax |
| 列表 | Prefix selected lines with unordered list markers |
| 待办 | Prefix selected lines with unchecked task markers |
| 公式 | Insert inline `$…$` or block `$$…$$` expressions and render them |

Selection offsets must be Unicode safe. Dialog confirmation receives the latest editor values. Formatting retains editor focus and selects the inserted editable content. Commands update the authoritative draft through the controller, then refresh derived content. No toolbar action may ship as a status-only placeholder.

## Editor, preview and split modes

Edit mode displays Markdown source and supports multiline editing, keyboard selection, undo/redo, Chinese input, and standard clipboard behavior. Use the existing persistent Gio editor flow; rich source decoration must not disrupt caret placement or composition. Preview renders headings, paragraphs, emphasis, links, images, fenced code, block quotes, ordered/unordered lists, task lists, thematic breaks, and tables. Split mode displays source and preview side by side from the same draft. Switching modes preserves source, selection and scroll state.

Parse and image decoding run outside frame/layout callbacks, with cancellation or generation checks. Outline entries record source offsets, not title-based identities, so repeated headings are distinct. Search filters headings, clicking moves the source caret or preview to the matching section, and collapsing the outline expands the content area. Ignore heading-looking text inside fenced code. Handle unfinished Markdown without blocking editing.

## Images, formulas and PDF

Resolve relative image destinations against the document directory. Bound image decoding and caches; missing images display their alternative text and a clear diagnostic. Preserve all image destinations in source. Opening a link is an explicit user action. Preview rendering does not execute embedded HTML or scripts.

Formula rendering covers inline/block math, Greek symbols, superscripts, subscripts, fractions, roots, and common operators. Keep renderer types behind the rendering/adapter boundary. Unsupported or invalid expressions retain their source and show an explicit diagnostic; do not advertise arbitrary TeX compatibility. Rendering must remain local and must not require a network service or a system TeX installation. Select and verify a suitable implementation before adding dependencies, and record the supported corpus and evidence.

PDF export derives from a frozen revision of the current draft and renders the same document content as preview, with pagination, Chinese glyphs, images and supported formulas. Reuse existing PDF fonts, output replacement, picker and background job patterns. Saving a PDF does not clear Markdown dirty state or change its path. If content cannot be rendered, report it before claiming a successful complete export. Export failure preserves existing destination data.

## Status

Display source character/word statistics, estimated reading time, Unicode-correct caret line and column, UTF-8 encoding, Markdown format, and accurate saved/dirty state. Compute document-wide statistics with the parsed snapshot, not by scanning the entire document in every frame.

## Verification and documentation

Add adapter round-trip tests for empty files, Chinese text, BOM, CRLF, exact source snapshots, no-op edits, failed writes, and untouched content. Add parser/outline tests for nested/duplicate headings, fences, tables, tasks, unfinished syntax and formulas. Exercise every toolbar command with empty and selected Unicode text, dialog cancellation, and selection retention. Verify draft recovery, switching tabs/modes, save-as and stale background result rejection.

Reuse ribbon visual-test infrastructure to inspect 1440 dp, narrow windows and high-DPI layouts for clipping, horizontal scroll, fixed caption alignment, and usable mode controls. Validate PDF content and rendered pages with Chinese text, local images and the supported formula corpus. Run the repository test suite and perform native interaction checks for IME and keyboard focus. Update `docs/architecture.md`, `docs/office-support.md` and `docs/office-library-evaluation.md` with actual implemented boundaries and evidence.

## Alternatives considered

Extending JSON's adapter directly would incorrectly couple Markdown to JSON validation. Routing source through DOCX would lose source fidelity. A separate source-preserving adapter with shared ribbon, lifecycle and PDF infrastructure provides the existing architecture's separation while reusing proven UI patterns.
