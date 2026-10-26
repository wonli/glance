# Mini Tools page with a Document Workbench entry

## Intent

Restore the missing **小工具** page shown in the supplied screenshot so people can open the proofreading Document Workbench without first opening a PDF. The first version contains one entry only: **文档工作台**.

## Goals

- Add **小工具** to the main home navigation and show the selected state on its page.
- Match the reference's Lucide `blocks` navigation icon (design node `DeJoU`, 20 × 20, `#3F4752`), using the same 20dp `homeIcon` family and tint path as adjacent navigation items.
- Keep the page shell recognizable from the screenshot: title, short subtitle, and a bordered tool card with an icon tile, name, description, and launch affordance.
- Make the single card open the existing proofreading module's task list in its separate window, with no source PDF required.
- Keep the existing route from an open PDF into proofreading unchanged.
- Localize new interface text in Chinese and English.

## Non-goals and anti-goals

- Do not add conversion, merge, compression, OCR, rename, comparison, or other tool cards in this pass.
- Do not change PDF editing, proofreading, or task persistence behavior.
- Do not create an empty proofreading task or select a source file just to show the task list.
- Do not modify the Pen file; implement the screen in the Glance application from the supplied screenshot.

## Approaches

1. **Dedicated 小工具 page with one workbench card (selected).** This matches the supplied screenshot and the request that the missing page be restored. The single card opens the existing task list directly and keeps a wider featured layout. The search field, four category tabs, and visible count stay present and functional even with one entry; the multi-tool upgrade banner remains deferred.
2. **Place a workbench shortcut on 最近.** This requires less navigation work, but it does not restore the missing page or match the supplied reference.

The selected route revises the earlier architecture constraint against adding a home entry for proofreading: the app gains a general **小工具** page, while the card opens the proofreading module's existing workbench. It does not add a standalone **文档校对** item to the home navigation or a new PDF toolbar action.

## Behavior and implementation constraints

- The main home UI currently routes only 最近, 收藏, AI 助手, and 设置. Add a persistent tools section and handle its navigation before choosing the page layout, following the existing navigation flow.
- The current PDF proofreading launcher requires an open PDF and a snapshot. Add a workbench-only module request that can open or raise the existing proofreading window without a source snapshot. The window should select its workbench list, load saved tasks normally, and leave document/task creation to the existing workbench controls.
- Keep source-backed proofreading requests and their snapshot lifecycle intact. Do not perform filesystem, database, or parsing work in Gio layout.
- Preserve the existing uncommitted change in `internal/desktopui/proofread_workbench_layout.go` while implementing this request.

## Visual direction

Retain the existing home sidebar and use the screenshot's page title and subtitle. Place the search field beside the heading, then show the four category tabs and filtered result count. Show one broad card in the main content area with the existing card border, radius, muted description, colored icon tile, and upper-right launch affordance. Keep its copy focused on continuing or opening proofreading work. Defer only the multi-tool PRO banner while the page has one entry.

## Verification notes

If implementation verification is requested, use the required current-directory startup and process-replacement procedure from `AGENTS.md`. Report code/test checks separately from actual window rendering; a skipped GPU check is not a visual pass.

## Addendum: restore the tools toolbar

The page keeps the reference toolbar even while the catalog contains one item: a search field beside the heading, category tabs for **全部工具**, **文档与 PDF**, **图片与识别**, and **文件管理**, plus a count of the currently visible tools. Search matches tool titles and descriptions; category tabs filter the same catalog. The sole **文档工作台** entry belongs to **文档与 PDF**. Empty categories and unmatched searches show a localized empty state. The catalog contains no placeholder tools; future entries use these categories and the existing card presentation.

This is a user-directed refinement of the original one-card scope: keeping the toolbar now avoids removing and reintroducing it when OCR and PDF utilities arrive. The card remains the only current entry.
