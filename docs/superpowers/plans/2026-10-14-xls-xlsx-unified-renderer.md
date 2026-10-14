# `.xls` 统一 XLSX 渲染实现计划

> **供代码执行者使用：** REQUIRED SUB-SKILL：按任务逐项执行本计划时，使用 `superpowers:executing-plans`；除非用户选择代理驱动执行。步骤使用复选框跟踪。

**目标：** 把旧版 `.xls` 导入为临时 `.xlsx`，随后使用现有 XLSX 网格显示和编辑；公式、缓存结果及单元格样式必须保留，原 `.xls` 只能读，输出只允许 `.xlsx`。

**架构：** 扩展现有 Apache-2.0 BIFF 读取器，让它提供逐工作表、逐行读取的带类型单元格、公式 token 和 XF 样式数据；在后台流式写入应用管理的临时 `.xlsx`。由现有 XLSX 适配器打开临时文件后，把导入会话接入同一网格；会话仍保留 `.xls` 来源路径，但 XLSX 工作簿保存路径保持为空，直到用户另存为 `.xlsx`。

**技术栈：** Go、现有 `github.com/extrame/xls v0.0.1`（在 `internal/format/xls/biff` 内维护带 Apache-2.0 许可证的本地扩展）、Excelize v2.11.0 流式写入器、现有 `document.GridReader` 和 Gio 桌面会话。

**规格：** [2026-10-14-xls-xlsx-unified-renderer-design.md](../specs/2026-10-14-xls-xlsx-unified-renderer-design.md)

## 全局约束

- 解析和转换在后台执行，不能放在 Gio 帧布局代码中。
- 转换必须通过应用管理的临时 `.xlsx` 文件，并流式处理中间数据；不能构建另一份完整 `[][]document.Cell` 或整个 XLSX 字节缓冲。
- 活动工作簿仍使用现有 XLSX/Excelize 内存模型；以实测资源预算拒绝超限文件，不能静默截断。
- 保留所有工作表、顺序、名称、带类型单元格值、公式表达式、公式缓存结果，以及可映射的字体、填充、边框、对齐和数字格式。
- 不重新计算公式；不能支持的公式或工作簿功能必须明确报错，不能悄悄改成常量或丢弃。
- `.xls` 来源必须保持只读；导入会话的“保存”必须转入 `.xlsx` 另存为，保存失败时保留脏状态。
- 保存前后及取消流程都必须确保原始 `.xls` 字节不变。

## 审查重点

1. 共享公式、数组公式、跨表引用和未解析名称：分别覆盖公式解码成功及明确报错，不能生成伪造公式。
2. 公式缓存为文本、布尔值、错误值或数字：转换后仍可读取缓存，并保留公式文本。
3. 空白但带样式的单元格、自定义数字格式和边框：转换及保存重开后样式仍存在。
4. 稀疏与密集大表、接近 BIFF8 行列边界的工作表：按实测资源预算完整转换或明确拒绝，不截断。
5. 加密、损坏、截断或取消的导入：给出清楚错误并清理所有临时文件。

---

### Task 1：扩展 BIFF 读取边界并建立真实样本

**文件：**

- 新建：`internal/format/xls/biff/`（含复制后维护的解析器源码、独立 `go.mod` 和原 Apache-2.0 许可证）
- 修改：`internal/format/xls/reader.go`
- 新建：`internal/format/xls/reader_import_test.go`
- 新建：`internal/format/xls/testdata/formula-style-multisheet.xls`
- 新建：`internal/format/xls/testdata/unsupported-chart.xls`
- 新建：`internal/format/xls/testdata/encrypted.xls`
- 修改：`go.mod`、`go.sum`
- 修改：`docs/office-library-evaluation.md`

**接口：**

```go
type ImportedCell struct {
    Column       int // 1-based
    Value        any // 原始类型值；错误值使用 CellError；公式单元格存缓存结果
    FormulaToken []byte
    XF           uint16
}

type CellError struct {
    Code byte // BIFF error code, retained so XLSX output can use t="e".
}

type ImportedRow struct {
    Number int // 1-based，按升序回调
    Cells  []ImportedCell // 按 Column 升序
}

type UnsupportedFeature struct {
    Sheet    string
    Name     string
    RecordID uint16
}

type ImportReader interface {
    SheetNames() []string
    VisitRows(sheet string, visit func(ImportedRow) error) error
    ExcelizeStyle(xf uint16) (excelize.Style, error)
    UnsupportedFeatures() []UnsupportedFeature
    Close() error
}

func OpenImportReader(path string) (ImportReader, error)
```

- [x] **步骤 1：先写失败测试**

  `TestImportReaderReturnsAllSheetsAndTypedCellMetadata` 断言样本中的所有工作表按原顺序返回；文本、数值、布尔值和错误值保持类型；访问行和单元格按升序；高行号稀疏单元格不会被挤到前面。`TestImportReaderPreservesFormulaTokensAndXFIndices` 断言公式 token 和 XF 索引可读。`TestImportReaderReportsUnsupportedFeatures` 使用单独的图表工作簿样本断言无法保留的图表功能被识别。`TestImportReaderRejectsEncryptedWorkbook` 使用加密样本，`TestImportReaderRejectsCorruptWorkbook` 与 `TestImportReaderRejectsTruncatedWorkbook` 使用合成损坏输入，断言解析器明确拒绝。

- [x] **步骤 2：运行测试确认按预期失败**

  运行：`go test ./internal/format/xls -run '^TestImportReader' -count=1`。

  预期：因导入读取接口尚不存在而失败。

- [x] **步骤 3：扩展本地 BIFF 读取器并添加最小样本**

  在本地副本中保留上游 `github.com/extrame/xls` 模块路径，通过根 `go.mod` 的 `replace` 指向 `internal/format/xls/biff`，以便现有导入继续使用同一个包并可维护解析扩展；同时保留上游 Apache-2.0 许可证。逐工作表访问、逐行回调非空行；每个单元格保留 1-based 列号、原始类型值、公式原始 token 和 XF 索引；BIFF 错误值使用 `CellError` 保留错误码。多表样本至少有稀疏行、各类单元格、一个公式及一个带样式空白单元格。将依赖版本、许可证、API 缺口及本地扩展原因写入 `docs/office-library-evaluation.md`。

- [x] **步骤 4：运行读取器测试**

  运行：`go test ./internal/format/xls -run '^TestImportReader' -count=1`。

  预期：工作表、类型值、行列坐标、公式 token、XF 索引均符合测试；加密、损坏和截断输入明确报错。

- [x] **步骤 5：提交本任务**

  ```bash
  git add go.mod go.sum internal/format/xls/biff internal/format/xls/reader.go internal/format/xls/reader_import_test.go internal/format/xls/testdata/formula-style-multisheet.xls internal/format/xls/testdata/unsupported-chart.xls internal/format/xls/testdata/encrypted.xls docs/office-library-evaluation.md docs/superpowers/specs/2026-10-14-xls-xlsx-unified-renderer-design.md docs/superpowers/plans/2026-10-14-xls-xlsx-unified-renderer.md
  git commit -m "feat: expose typed xls import records"
  ```

### Task 2：解码 BIFF 公式并映射 XF 样式

**文件：**

- 新建：`internal/format/xls/formula.go`
- 新建：`internal/format/xls/formula_test.go`
- 新建：`internal/format/xls/style.go`
- 新建：`internal/format/xls/style_test.go`
- 修改：`internal/format/xls/biff/`（共享公式、数组公式和样式记录访问）

**接口：**

```go
type FormulaResolver interface {
    SheetName(externSheetIndex uint16) (string, error)
    DefinedName(nameIndex uint16) (string, error)
}

func decodeBIFFFormula(tokens []byte, baseCell string, resolver FormulaResolver) (string, error)
func (r *importReader) ExcelizeStyle(xf uint16) (excelize.Style, error)
```

- [x] **步骤 1：先写公式解码失败测试**

  `TestDecodeBIFFFormulaOperatorsReferencesAndFunctions` 覆盖算术、比较、一元运算、百分号、绝对/相对单元格引用、区域及 `PtgFunc`/`PtgFuncVar` 函数调用；`TestDecodeBIFFFormulaCrossSheetReference` 覆盖跨表引用；`TestDecodeBIFFFormulaDefinedName` 和 `TestDecodeBIFFFormulaUnknownNameReturnsError` 覆盖定义名称及无法解析名称；`TestDecodeBIFFSharedFormulaAtEachCell` 覆盖共享公式的相对引用展开；`TestDecodeBIFFArrayFormula` 覆盖数组公式；`TestDecodeBIFFFormulaUnsupportedTokenReturnsError` 断言未知 token 返回错误而不是部分或伪造公式。

- [x] **步骤 2：运行测试确认解码行为缺失**

  运行：`go test ./internal/format/xls -run '^TestDecodeBIFF(Formula|SharedFormula|ArrayFormula)' -count=1`

  预期：因 `decodeBIFFFormula` 尚不存在而失败。

- [x] **步骤 3：实现完整 token 消费和公式输出**

  解析 BIFF8 后缀表达式 token，转换为 Excelize 接受的公式文本；正确处理运算优先级、括号、3D 工作表引用、由 resolver 提供的定义名称，以及共享/数组公式关联记录。当前导入器尚未解析 `NAME` 表，因此遇到真实定义名称时应带单元格上下文明确报错，不能保留伪造公式或缓存值。

- [x] **步骤 4：运行公式回归**

  运行：`go test ./internal/format/xls -run '^TestDecodeBIFFFormula|^TestDecodeBIFFSharedFormula|^TestDecodeBIFFArrayFormula' -count=1`。

  预期：全部支持的公式输出稳定；不支持项明确失败。

- [x] **步骤 5：先写 XF 到 XLSX 样式的失败测试**

  `TestXFStyleMapsFontFillBorderAlignmentAndNumberFormat` 使用样本中的多个 XF，断言字体、填充、边框、水平/垂直对齐、换行和数字格式映射到对应 Excelize 样式；`TestXFStyleValuesRemainDistinctByXFIdentity` 断言相同 XF 映射稳定、不同 XF 的样式属性不会串用；转换器中新建的 Excelize 样式 ID 缓存由 Task 3 实现。

- [x] **步骤 6：实现样式解析及缓存并运行测试**

  解析 BIFF 字体、格式、调色板和 XF 关联数据，通过 `excelize.File.NewStyle` 创建目标样式；按 XF 索引缓存 ID。运行：`go test ./internal/format/xls -run '^TestXFStyle' -count=1`。

  预期：样本所覆盖的样式属性与预期一致；无法映射的必需 XF 返回错误。

- [x] **步骤 7：提交本任务**

  ```bash
  git add internal/format/xls/formula.go internal/format/xls/formula_test.go internal/format/xls/style.go internal/format/xls/style_test.go internal/format/xls/biff
  git commit -m "feat: decode xls formulas and styles"
  ```

### Task 3：逐行转换为临时 XLSX 并验证保真

**文件：**

- 新建：`internal/format/xls/converter.go`
- 新建：`internal/format/xls/converter_test.go`
- 修改：`internal/format/xls/testdata/formula-style-multisheet.xls`

**接口：**

```go
type TempXLSX struct {
    Path string
    // 内部字段记录本次导入独占的临时目录。
}

func ConvertToTempXLSX(ctx context.Context, sourcePath string) (*TempXLSX, error)
func (t *TempXLSX) Cleanup() error

// Tests may pass a small budget to prove rejection behavior.
func convertToTempXLSX(ctx context.Context, sourcePath string, maxCells uint64) (*TempXLSX, error)
```

- [x] **步骤 1：先写转换与资源预算失败测试**

  `TestConvertXLSPreservesSheetsFormulaCachesAndStyles` 转换多表样本后用 `xlsx.Open` 打开结果，断言工作表顺序/名称、公式文本、公式缓存值、数字/文本/布尔/错误单元格及代表性 XF 样式都存在。`TestConvertXLSPreservesBIFF8BoundaryCoordinates` 断言 BIFF8 最大行列坐标不会被截断或压缩。`TestConvertXLSRejectsUnsupportedWorkbookFeature` 断言发现未支持工作簿功能时转换明确报错。`TestConvertXLSRejectsUnsupportedFormulaWithoutReplacingItWithValue` 断言不支持的公式不会变成常量。`TestConvertXLSPreservesBlankCellStyle`、`TestConvertXLSPreservesFormulaCachedStringBooleanAndError`、`TestConvertXLSCancellationCleansTempDirectory`、`TestConvertXLSFailureCleansTempDirectory`、`TestConvertXLSResourceBudgetRejectsWithoutTruncating` 和 `TestConvertXLSPreservesSharedStringsAcrossBIFFContinuations` 覆盖空白样式、缓存类型、临时文件清理、超限拒绝及跨 CONTINUE 的中文共享字符串。资源预算测试通过内部 `maxCells` 参数注入较小阈值，并统计所有输出单元格，包括仅带样式的空白单元格。

- [x] **步骤 2：运行测试确认转换器尚不存在**

  运行：`go test ./internal/format/xls -run '^TestConvertXLS' -count=1`

  预期：因转换 API 尚不存在而失败。

- [x] **步骤 3：实现独占临时目录和 Excelize 流式写入**

  在创建输出或写入行之前检查 `ImportReader.UnsupportedFeatures()`；存在不能保留的功能时返回含工作表和功能名称的错误。用 `excelize.NewFile()` 创建输出，移除默认工作表并按原顺序创建工作表。将 XF 转为 Excelize 样式 ID 并按 XF 缓存；将 BIFF `DEFCOLWIDTH`/`COLINFO` 宽度设置到 `StreamWriter`，并从 `ROW` 信息向每行写入高度；隐藏/分组/格式元数据、合并区、超链接等未保留功能必须预检后失败。对每个原工作表用 `StreamWriter.SetRow` 写入递增的原始行号；单元格用 `excelize.Cell{StyleID, Formula, Value}` 同时承载公式、缓存值和样式。对错误缓存值记录单元格坐标，关闭流式工作簿后在磁盘上流式改写对应 worksheet XML 的错误类型和值，不把整个 XLSX 包载入内存。逐行检查 `ctx.Err()` 和注入的单元格预算；预算在调用旧解析器物化工作表之前按 BIFF 单元格记录预检，并在写出时再次防守；计数包括仅带样式的空白单元格，超限必须失败而不能截断。每个错误路径都删除该次导入目录。不得构建整张表的第二份矩阵或 `WriteToBuffer` 整包缓冲。

- [x] **步骤 4：运行转换保真测试**

  运行：`go test ./internal/format/xls -run '^TestConvertXLS' -count=1`

  预期：普通 XLSX 适配器可打开转换结果，前一步列出的数据全部可读。

- [x] **步骤 5：基准测量后设置默认资源预算**

  新增 `BenchmarkConvertLegacyXLSByPopulatedCellCount`，至少覆盖稀疏与密集样本；运行 `go test ./internal/format/xls -run '^$' -bench '^BenchmarkConvertLegacyXLSByPopulatedCellCount$' -benchmem -count=1`。记录转换及 XLSX 适配器打开后的峰值 Go 堆、进程 RSS、耗时和实际写入单元格数；堆使用 `runtime.MemStats` 采样，RSS 使用当前宿主平台的进程资源监视器并在记录中写明平台和命令。根据这些测量结果设置 `ConvertToTempXLSX` 的默认资源拒绝阈值，并记录测量及阈值依据到 `docs/office-library-evaluation.md`。运行：`go test ./internal/format/xls -run '^TestConvertXLS' -count=1`。

- [x] **步骤 6：提交本任务**

  ```bash
  git add internal/format/xls/converter.go internal/format/xls/converter_test.go internal/format/xls/testdata/formula-style-multisheet.xls docs/office-library-evaluation.md
  git commit -m "feat: convert legacy xls to staged xlsx"
  ```

### Task 4：接入打开流程和 XLSX 会话身份

**文件：**

- 修改：`internal/desktopui/document_open.go`
- 修改：`internal/desktopui/runtime.go`
- 修改：`internal/desktopui/document_session.go`
- 修改：`internal/desktopui/application_session_adapter.go`
- 修改：`internal/application/runtime.go`
- 新建：`internal/desktopui/xls_open_test.go`
- 修改：`internal/application/format_adapters.go`
- 新建：`internal/application/xls_factory_test.go`
- 修改：`internal/application/runtime_test.go`

**接口：**

```go
// adapter 的实际编辑格式可与源文件格式不同。
type SourceFormatAdapter interface {
    SourceFormat() Format
}
```

`loadedDocument.path` 与 `sharedDocumentSession.path` 继续保存原 `.xls` 来源路径，新增 `sourceFormat` 与 `saveAsRequired` 状态。`.xls` 分支返回由 XLSX 适配器打开的 `document.GridReader`，且工作簿自身保存路径在首次另存为前为空。`desktopSessionAdapter.Format()` 返回 `FormatXLSX`，并通过 `SourceFormatAdapter.SourceFormat()` 声明其来源为 `FormatXLS`。`Runtime.OpenDocument` 仅在该接口明确匹配请求来源格式时接受格式转换；这样应用运行时以 XLSX 格式管理编辑能力，同时保留原始来源身份。

- [x] **步骤 1：先写打开、工厂、运行时和布局测试**

  `TestLoadLegacyXLSReturnsEditableGridReaderForAllSheets` 断言 `.xls` 打开为可编辑 `GridReader`、所有工作表可用、源路径仍为 `.xls`、会话需要另存为且工作簿没有 `.xls` 保存路径。`TestImportedXLSUsesVirtualGridAndTabsBelowHeaders` 断言工作表标签位于列标题与网格下方。`TestOpenXLSFactoryReturnsEditableXLSXAdapter` 断言默认 XLS 工厂返回可编辑 XLSX 适配器并清理暂存文件。`TestRuntimeOpenAcceptsAdapterDeclaredSourceFormat` 与 `TestRuntimeOpenRejectsUndeclaredFormatMismatch` 覆盖精确来源格式匹配和错误格式拒绝。

- [x] **步骤 2：运行测试确认旧行为**

  运行：`go test ./internal/desktopui -run '^TestLoadLegacyXLSReturnsEditableGridReaderForAllSheets$|^TestImportedXLSUsesVirtualGridAndTabsBelowHeaders$' -count=1`，以及 `go test ./internal/application -run '^(TestOpenXLSFactoryReturnsEditableXLSXAdapter|TestRuntimeOpenAcceptsAdapterDeclaredSourceFormat|TestRuntimeOpenRejectsUndeclaredFormatMismatch)$' -count=1`。

  预期：当前返回的只读 XLS 读取器不实现 `document.GridReader`，测试失败。

- [x] **步骤 3：将 XLS 加载改为后台转换后打开 XLSX**

  在 `loadDocument` 和默认 `FormatXLS` 工厂分支调用 `ConvertToTempXLSX`，用 `xlsx.Open` 打开输出，清空 XLSX 工作簿内部保存路径，确认工作表有效后清理临时文件。任何打开错误都关闭已创建的 workbook 并清理临时文件。将来源格式和 `saveAsRequired` 从 `loadedDocument` 传到共享会话；默认工厂返回的适配器声明 `SourceFormat() == FormatXLS`。

- [x] **步骤 4：实现显式格式别名并运行测试**

  在 `internal/application/runtime.go` 的格式检查中只对 `SourceFormatAdapter` 的精确匹配放行；其他格式不匹配保持报错。成功打开后，运行时编辑格式必须是 `FormatXLSX`，来源路径仍对应原 `.xls`。

- [x] **步骤 5：运行打开、工厂和布局回归**

  运行：`go test ./internal/desktopui -run '^TestLoadLegacyXLSReturnsEditableGridReaderForAllSheets$|^TestImportedXLSUsesVirtualGridAndTabsBelowHeaders$' -count=1`，以及 `go test ./internal/application -run '^(TestOpenXLSFactoryReturnsEditableXLSXAdapter|TestRuntimeOpenAcceptsAdapterDeclaredSourceFormat|TestRuntimeOpenRejectsUndeclaredFormatMismatch)$' -count=1`。

  预期：导入工作簿以 XLSX 适配器和原 `.xls` 来源身份进入桌面会话。

- [x] **步骤 6：提交本任务**

  ```bash
  git add internal/desktopui/document_open.go internal/desktopui/document_session.go internal/desktopui/runtime.go internal/desktopui/application_session_adapter.go internal/desktopui/document_save.go internal/desktopui/xls_open_test.go internal/application/format_adapters.go internal/application/runtime.go internal/application/runtime_test.go internal/application/xls_factory_test.go docs/superpowers/plans/2026-10-14-xls-xlsx-unified-renderer.md
  git commit -m "feat: open xls as converted xlsx sessions"
  ```

### Task 5：强制 XLSX 另存为并保护原件

**文件：**

- 修改：`internal/desktopui/document_save.go`
- 修改：`internal/desktopui/workspace_async_results.go`
- 修改：`internal/desktopui/application_session_adapter.go`
- 修改：`internal/application/format_adapters.go`
- 新建：`internal/desktopui/xls_save_as_test.go`
- 修改：`internal/application/xls_factory_test.go`

- [x] **步骤 1：先写保存路径失败测试**

  `TestImportedXLSSaveStartsXLSXSaveAs` 断言对有脏修改的导入工作簿执行普通“保存”会启动 `.xlsx` 另存为流程，而不会调用 `Workbook.Save()`。`TestImportedXLSSaveAsRejectsXLSAndKeepsSource` 断言 `.xls` 目标被拒绝、源字节不变、脏状态保留。`TestImportedXLSRuntimeSaveAsRejectsXLS` 断言应用运行时也拒绝 `.xls` 目标。`TestImportedXLSSaveAsAdoptsXLSXPathAndClearsRequirement` 断言成功另存为后 `.xlsx` 成为保存路径并清除 Save As 标记；测试随后重开输出文件，确认刚编辑的单元格与代表性未编辑公式、缓存值和样式仍在。

- [x] **步骤 2：运行测试确认保护逻辑缺失**

  运行：`go test ./internal/desktopui -run '^TestImportedXLSSave' -count=1`

  预期：当前保存逻辑没有导入 XLS 的 Save As 标记，测试失败。

- [x] **步骤 3：让导入会话的普通保存转入 XLSX 另存为**

  在 `startSaveForReason` 确认文档有脏修改后、开始普通保存操作之前检查 `saveAsRequired`；若为真，启动 `startSaveAsPicker("xlsx")` 并结束当前普通保存路径。保留现有扩展名校验和取消语义。

- [x] **步骤 4：另存为成功后转移保存身份**

  处理成功的 `saveResult.saveAs` 时更新 XLSX 工作簿源路径、共享会话路径和标签来源路径为目标 `.xlsx`，并清除 `saveAsRequired`。失败或取消不能改路径、清标记或清脏状态。

- [x] **步骤 5：运行保存回归和扩展名验证**

  运行：`go test ./internal/desktopui -run '^(TestImportedXLSSave|TestSaveAsRequiresMatchingFormatExtension)$' -count=1`，以及 `go test ./internal/application -run '^TestImportedXLSRuntimeSaveAsRejectsXLS$' -count=1`。

  预期：桌面与应用运行时均只接受 `.xlsx` 目标；保存成功后关闭重开仍保留编辑值和代表性原内容；取消、拒绝 `.xls` 和写入失败均不改变源 `.xls` 或丢失脏状态。

- [x] **步骤 6：提交本任务**

  ```bash
  git add internal/desktopui/document_save.go internal/desktopui/application_session_adapter.go internal/desktopui/xls_save_as_test.go internal/application/format_adapters.go internal/application/xls_factory_test.go docs/superpowers/plans/2026-10-14-xls-xlsx-unified-renderer.md
  git commit -m "fix: require xlsx save as for imported xls"
  ```

### Task 6：更新兼容性说明并完成回归

**文件：**

- 修改：`internal/desktopui/layout.go`
- 修改：`internal/desktopui/spreadsheet_sheet_tabs_layout.go`
- 修改：`docs/office-support.md`
- 修改：`docs/office-library-evaluation.md`
- 参考：`../tools/docs/compatibility/office-feature-matrix.md` 的证据记录结构，不修改 sibling 仓库。

- [x] **步骤 1：删除旧 XLS 行列表渲染路径**

  删除 `layoutWorkbook` 的非 `GridReader` 行列表分支和 `layoutSheetTabs` 对非网格工作簿调用 `loadSheet` 的布局路径。删除后所有活动表格文档都必须实现 `document.GridReader`；工作表标签只由网格布局在列标题/网格下方绘制。保留非渲染用途所需的 `document.Spreadsheet` 接口。

- [x] **步骤 2：更新支持矩阵和格式评估**

  在 `docs/office-support.md` 记录首个 BIFF 版本、公式/样式支持边界、资源预算来源、明确报错的内容和 XLSX-only 保存行为；在 `docs/office-library-evaluation.md` 记录依赖来源、许可证、本地扩展、公式与 XF 测试证据。按 sibling 兼容性矩阵的证据结构记录支持范围，不复制或修改该仓库文件。

- [x] **步骤 3：运行最终验证**

  运行：`go test ./internal/format/xls ./internal/format/xlsx ./internal/application ./internal/desktopui -count=1`，`go test ./internal/format/xls -count=1`，XLS 桌面导入/布局/另存为定向回归，以及 `git diff --check`。

  结果：XLS、XLSX、application 通过；全部 XLS 定向桌面回归通过；`git diff --check` 无输出。整包 desktopui 有三个与本改动无关的原生 UI 测试失败（主页收藏导航、托盘隐藏前置条件、PDF 原生保存面板）；前两项也出现在基线日志 `/private/tmp/glance-go-xls-baseline-elevated.log`，PDF 保存面板测试单独重跑通过，整包时失败，表现为原生面板测试互相干扰。

  代码审查后的追加验证：`go test ./internal/format/xls -count=1`、XLSX、application、独立 BIFF 解析器模块全量测试和 XLS 定向 desktopui 回归均通过；额外的字符串公式缓存、资源预算预检、COLINFO/ROW 几何和不支持元数据 fail-closed 回归通过。再次运行组合包命令时，XLS/XLSX/application 均通过，但 desktopui 的 `httptest` 用例因当前 sandbox 禁止绑定 IPv6 loopback (`[::1]:0`) 而 panic，故该次不能作为整包 desktopui 结果；对应 XLS UI 定向回归在同一环境通过。

- [x] **步骤 4：提交本任务**

  ```bash
  git add internal/desktopui/app.go internal/desktopui/document_edit.go internal/desktopui/document_open.go internal/desktopui/document_session.go internal/desktopui/layout.go internal/desktopui/session_state.go internal/desktopui/spreadsheet_grid_frame_layout.go internal/desktopui/spreadsheet_sheet_tabs_layout.go internal/desktopui/workspace_sessions.go internal/desktopui/workspace_state.go internal/desktopui/xls_open_test.go internal/desktopui/layout_fill.go internal/format/xls/reader.go internal/format/xls/reader_test.go docs/office-support.md docs/office-library-evaluation.md docs/superpowers/plans/2026-10-14-xls-xlsx-unified-renderer.md
  git commit -m "fix: unify xls and xlsx rendering"
  ```
