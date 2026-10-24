# Shared proofreading pieces implementation plan

Goal: absorb empty boxes without full-book character work or full-snapshot undo.
Architecture: fixed page index, immutable balanced piece trees, changed-box history; v2 snapshots at compatibility boundaries.

- [x] Establish 892-page fragmented-content baseline and persistence/identity regressions.
- [x] Add immutable weighted piece tree with split/concat and lazy metadata changes.
- [x] Make per-box roots canonical; use local edits, adjacent transfer, reference refill and delta history. Isolate obsolete locked/reflow conversion.
- [x] Remove full Snapshot from fine-scroll controller; publish affected pages and retain pending layout.
- [x] Compare identical benchmarks; run model/controller/input/render checks, review, restart latest app and record limits.

原生窗口交互验收未完成，原因与已知失败见 ../../proofreading-piece-storage-verification.md；启动成功不作界面验收通过。
