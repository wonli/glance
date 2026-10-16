# Word/Excel 选区验证（2026-10-16）

## 本次行为

- Word 分页和流式视图双击选择当前段落，覆盖格式片段；Backspace/Delete 删除完整文字选区，支持反向选择与同一正文/表格单元格中的相邻段落。
- Excel 行号、列字母、左上角分别选择整行、整列、整表；Shift 与拖动扩展同类选区。字体修改作用于默认行列样式和已有单元格，空白单元格后续输入继承字体。Delete 清空内容和公式，保留尺寸与样式。
- 单元格内拖选文字后删除的基础交互测试通过，未为此添加未经复现的删除逻辑改动。

## 已通过

- `go test ./internal/desktopui -run '^(TestGridHeader|TestGridText|TestCellEdit|TestCellSelection|TestFormula|TestWordPage|TestWordFlow|TestWordSelectionDelete|TestWordParagraphSplit|TestWordEnter|TestWordTextAndEnter)' -count=1 -v`（退出码 0）
- 实际 Metal GPU 的表头选择像素验证：1×/2× 均通过，无 SKIP。
- 注册运行时命令路径：整行、整列、整表字体/字号及内容删除，未选区域与已有格式保留、后续空白单元格字体继承、撤销/重做、Save As 后重开。
- `go test ./internal/application ./internal/format/xlsx ./internal/format/docx ./internal/wordlayout -count=1`（所有包通过）
- `TestImportedXLSUsesVirtualGridAndTabsBelowHeaders` 单独复测通过。旧测试把新增表头的灰色文字误判为蓝色工作表标签，现按实际语义标签位置与该区域的渲染像素检查。
- `go build -o dist/glance-office ./cmd/glance` 和 `git diff --check` 通过。

新增的共享公式、计算链、部分数组公式拒绝、格式失败回滚、无改动操作、前缀命名空间与带空白 XML 顺序回归均观察过修复前失败、修复后通过。

## 全量回归仍未通过

执行了 `go test ./...`。执行期间发现新增 `spreadsheet.range.clear` 后登记列表需要更新，此项已修复，并通过上面的完整 application 包复测；导入 XLS 的像素误判已单独复测通过。以下其余失败尚未逐项做基线归因或修复：

- `TestDesktopMCPStdioEditsTheLiveGioSession`
- `TestSettingsFieldBlankEdgesFocusEditor`
- `TestSettingsSavedKeyEdgeClickFocusesVisiblePreview`
- `TestSettingsAPIKeyEyeClickTogglesVisibility`
- `TestSettingsFieldsVerticallyCenterText`
- `TestSpreadsheetRecalculateAndRefreshAll`
- `TestSpreadsheetToolbar_SpellCheckAndTranslate`
- `TestSpreadsheetViewModesAndRuler`
- `TestHideMainWindowToTrayPreconditions`
- `TestWordUpDownKeepsPreferredVisualColumnAcrossVisualLines`
- `TestWordToolbar_WatermarkTrackRevisionsEditorAndLayout`
- `TestWordToolbar_RemainingActionsBatch`
- `TestMCPCLIRecognizesServeAndDocumentsStdoutContract`
- `TestLoadManifestLoadsCuratedCorpus`

完整日志：`/private/tmp/glance-selection-full-tests.log`。针对性日志：`/private/tmp/glance-selection-focused.log`、`/private/tmp/glance-selection-packages.log`。

## 保留边界

Word 选区中的图片、域、分节与跨表格单元格内容，以及 XLSX 部分数组/数据表公式清除，采用失败不改写文档的边界。整行列 outside 边框不支持；不同轴默认样式相交时新增显式样式单元格上限为一百万。没有声称全部 Office 格式兼容。

当前已经打开的旧 Glance 进程不会自动加载新代码；需保存文档并重启后使用。此轮验证没有关闭用户现有窗口。
