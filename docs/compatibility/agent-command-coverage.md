# Agent 命令覆盖清单

本清单从当前 Ribbon 定义盘点每个可触发操作，记录稳定 Action ID、MCP 能力 ID、操作说明、目标、参数、修订要求、状态影响和风险。新增 Ribbon 操作时，必须同步增加清单项并通过 `TestRibbonActionCoverageManifest`。

## 覆盖范围

| 格式 | Ribbon 操作数 |
|---|---:|
| `json` | 12 |
| `markdown` | 17 |
| `pdf` | 23 |
| `presentation` | 74 |
| `spreadsheet` | 88 |
| `word` | 104 |
| **合计** | 314 |

演示文稿“表格”与“图表”按钮分别与 MCP 共用 `presentation.element.table.create` 和 `presentation.element.chart.create`，Agent 可按稳定 slide_id、point 坐标、矩阵单元格或类别/系列数据创建对象；表格最多 1000 个单元格，图表最多 1000 个数据点。Ribbon 表格入口提供 2×2、3×3、4×4 空白网格；图表类型入口使用明示的三类示例数据。演示文稿“拼写检查”按钮与 MCP 共用 `presentation.text.proofread`：按幻灯片分页检查重复中文标点和中文括号/弯引号数量，最多扫描 1000 张幻灯片并返回稳定 ID；此启发式检查不含词典拼写、语法或语义分析。演示文稿“辅助功能”按钮与 MCP 共用 `document.inspect`：检查结果在 `content.accessibility` 中分页返回缺少可读取文本和图片替代文本的问题，并携带稳定幻灯片/元素 ID。装饰性图片不会误报；`complete=false` 表示需要继续分页或存在元素读取上限。Ribbon 最多扫描 1000 张幻灯片并展示 30 条发现。此规则不评估图片语义、颜色对比度或内容质量。

演示文稿“排练计时”与 MCP 共用 `presentation.view.set`：`rehearsal_start` 从当前页启动放映并计时，`rehearsal_stop` 暂停累计，`rehearsal_reset` 清零；`presentation.view.get` 返回窗口的运行状态和累计毫秒数。退出放映自动停止计时，数据只属于窗口会话，不写入 PPTX。

Word“布局/位置”通过 `word.image.move` 把明确的可编辑行内图片移动到稳定段落和段内锚点；MCP 可指定任意支持的正文流位置，Ribbon 复用同一命令将图片放到当前光标位置。Word“布局/环绕文字”通过高风险 `word.image.wrap.set` 将可安全识别的 DrawingML 浮动图片转换为嵌入型，保留图片数据和尺寸，移除浮动定位及环绕规则；正文可能重新分页。复杂或未知结构会拒绝修改，Agent 调用需经桌面人类确认。`document.inspect` 会返回图片稳定 ID、当前环绕模式及 `wrap_editable`。Runtime 回归 `TestWordImageCapabilitiesRoundTripAndDelete` 覆盖插入/旋转/缩放/替换/删除的无效稳定目标和失败原子性、移动成功与 no-op、逐操作 revision/dirty、替换/删除的人类审批及 DOCX 保存重开；`TestWordImageWrapCapabilityRequiresHumanAndPreservesImage` 覆盖确认/拒绝、浮动转嵌入、重复 no-op 及图片数据和尺寸保存重开。Word 原生批注通过 `word.comment.add` 和高风险 `word.comment.delete` 按明确 run_id/comment_id 创建和删除；Ribbon 与 Agent 共用命令，Agent 删除需人类确认；Agent-only `word.comments.list` 按页返回批注作者、正文和可映射的 run_id。Word/PPTX 批注 Runtime 测试也覆盖无效添加/删除目标、dirty/revision 和保存后 clean 状态。

演示文稿“复制”通过 `presentation.element.text.copy` 按显式稳定 slide_id 和 element_id 返回整个可编辑文本元素的纯文本；不依赖窗口选区，不保留格式或对象结构，未提交桌面文本草稿时 Agent 查询会被拒绝。桌面“粘贴”从系统剪贴板读取文本，再通过 `presentation.element.text.set` 写入原先选中的稳定元素；MCP 直接提供明确 ID 和 text。当前写入最多 32767 个字符且只接受单段落，换行会明确报错，不会静默丢弃段落。

Markdown 编辑/预览/分屏和 PDF 阅读/批注/签名控件纳入视图操作清单；PDF 打印和下载分别作为外部输出与文件输出动作。PDF 页码只是展示信息，不算可调用操作。上下文菜单、选择器中的值属于参数，不单独伪装为命令。

## 参数和目标

JSON 清单是完整覆盖索引，记录能力 ID、操作说明、精确参数 Schema、目标、状态影响和风险。文档命令要求 `document_id`，写操作要求 `expected_revision`；表格按工作表和范围定位，Word 使用段落/文本运行 ID，演示文稿使用幻灯片/元素 ID，JSON 使用节点 ID 或路径，PDF 使用页码/文本范围。文件操作直接接收路径，不模拟文件对话框。视图动作要求窗口 ID；无附着窗口时返回 `no_attached_view`。参数 Schema 与能力目录保持一致，MCP 工具从注册描述和处理器生成。

能力目录在注册时校验 Schema 定义，并在每次命令分发前强制执行输入 Schema 中实际使用的类型、必填项、额外字段、枚举/常量、数值边界、字符串长度/正则、数组长度/唯一性及 `anyOf`/`allOf`/`oneOf` 约束。Schema 的 `description` 与字段说明用于 MCP 工具解释和生成调用参数；这些约束不是仅供模型参考的提示。当前实现的是能力目录使用的 JSON Schema 子集，不宣称通用 Draft 兼容。

验证边界：`TestEveryManifestCapabilityIsRegisteredAndCallable` 验证清单能力均发布为 MCP 工具并能到达目录分发；`TestEveryCommandRejectsStaleRevisionBeforeHandlerExecution` 对当前 128 项 `CapabilityCommand` 使用 Schema 合法参数验证旧 revision 在处理器执行前被拒绝且文档状态不变。`TestEveryCommandDispatchesSchemaValidRequestWithoutPartialFailure` 将 128 项命令分发到对应格式的隔离空白会话，41 项样例请求完成，87 项返回已验证错误且 revision/dirty 不发生部分变化。`TestEveryHighRiskCommandRequiresHumanConfirmation` 逐一确认当前 22 项高风险 `CapabilityCommand` 在 Agent 请求后等待人类审批，拒绝不会修改文档。通用 Command Bus 测试覆盖无效参数、格式不支持和幂等冲突。目录分发矩阵与专题编辑行为、Office 客户端互操作分别记录，不互相替代。

`TestPresentationObjectCapabilitiesUseStableTargetsAndRoundTrip` 通过 application Runtime 覆盖 PPTX 文本框/形状/表格/图表/图片创建、文本读取与替换、组合样式、几何变换、图片裁剪/替换；检查稳定 ID、无效目标和数据失败原子性、文本/样式/变换/裁剪/替换 no-op，以及两次保存重开后的图表与表格 OOXML、样式/几何、图片裁剪和精确媒体字节。`TestPresentationSlideNotesAndDeleteCapabilitiesUseStableIDsAndConfirmation` 覆盖幻灯片创建、按 expected_notes 比较更新/冲突/no-op、备注保存重开、高风险 Agent 删除的人类确认，以及禁止删除最后一页。它们验证 Glance 自己生成的 PPTX，不代表 PowerPoint/LibreOffice 互操作验收。

`TestPresentationMediaCapabilitiesValidateAndEmbedExplicitFiles` 通过 application Runtime 插入 WAV/MP3 音频和 MP4 视频，覆盖正确媒体签名、格式/扩展名不匹配、空音频、伪装 MP4 与装饰性视频替代文本校验；无效请求不改变 revision/dirty。成功插入后检查返回的媒体大小、稳定元素 ID、几何和视频替代文本，并在保存重开后比较嵌入媒体的精确字节。此测试不验证 Office 客户端播放。

新增的专题证据覆盖此前缺少直接行为回归的命令：`TestWordCommentCapabilitiesAddListDeleteRoundTrip` 与 `TestWordThemeEffectsAndAddinsCapabilitiesRoundTrip` 检查 Word 批注的有效/重复/无效目标、删除和 DOCX 保存重开，主题效果设置与 no-op 重开，以及加载项目录的操作说明和 Schema；`TestSpreadsheetCommentCapabilitiesRoundTripAndNoop` 检查 XLSX 批注查询/新增/删除、无效工作表、重复新增和重复删除 no-op，以及两次保存重开；`TestPresentationCommentCapabilitySavesAndReopensWithoutLosingSlideContent` 检查 PPTX 批注添加、列出、无效删除、删除及二次保存重开；`TestPresentationThemesAndSlideDuplicateRoundTrip`、`TestPresentationGroupCapabilityUsesExplicitAdjacentElementIDs` 检查主题/配色、稳定幻灯片/元素目标、no-op、失败原子性和 PPTX 保存重开；`TestPresentationAnimationCapabilitiesUseStableTargetsAndRoundTrip` 检查动画列表、有效动画目标、无效幻灯片/元素/索引、时长修改及 no-op、revision/dirty 不变和 PPTX 保存重开；`TestPresentationTransitionCapabilitiesApplyToAllAndRoundTrip` 检查 `presentation.transition.get/set` 的稳定幻灯片目标、跨幻灯片应用、无效组合失败原子性、no-op 和保存重开；`TestPresentationBackgroundAndCanvasSizeCapabilitiesRoundTrip` 检查幻灯片背景色和画布尺寸的有效/无效目标、no-op、dirty-state 和 PPTX 保存重开；`TestPresentationExportAndPrintCapabilitiesUseRealSnapshot` 与 `TestPDFPrintCapabilityUsesCapturedPrinterAndPreservesRevision` 检查有效导出、可解析 PDF、打印参数和源修订/dirty 不变。XLSX 新建文档保存路径接管回归还发现并修复了适配器仍把工作簿当成独立副本保存的问题；无图片 PPTX PDF 导出回归曾发现并修复零字节图像被误判为超限的问题。

XLSX 的另外两组能力由 `TestSpreadsheetDefinedNameCapabilitiesRoundTripAndNoop` 与 `TestSpreadsheetHyperlinkCapabilitiesRoundTripAndNoop` 覆盖；前者同时回归空 scope 与 Excelize `Workbook` scope 的一致比较，后者验证超链接目标的读写/删除，但 tooltip 读取受当前格式库 API 限制，不能据此宣称 tooltip 往返兼容。`TestSpreadsheetFormulaRangeCapabilityAdjustsReferencesAndTracksRevision` 覆盖 `spreadsheet.formulas.fill` 按活动单元格偏移公式引用、明确范围、revision 递增及相同填充 no-op。`TestSpreadsheetFillDownCapabilityTracksRevisionAndNoOp` 覆盖 `spreadsheet.range.fill_down` 的首行公式传播、修订递增及相同填充 no-op；XLSX 格式测试进一步覆盖格式与单步撤销。

`TestSpreadsheetViewAndPageSetupCapabilitiesRoundTripAndNoop` 验证工作表视图、标尺、冻结窗格、网格线与页面方向能力的有效目标、无效请求原子性、no-op 和保存重开；其证据只覆盖 OOXML 状态，不代表 Excel UI 展示完全一致。

`TestSpreadsheetFillRightCapabilityTracksRevisionAndNoOp` 验证 `spreadsheet.range.fill_right` 从选区最左列复制值与公式，按列调整相对引用，修订递增，且重复操作 no-op；XLSX 专题测试继续覆盖格式、一步撤销与不安全目标拒绝。`TestSpreadsheetZoomCapabilitiesRouteToAttachedWindowWithoutDocumentMutation` 通过 application Runtime 调用 `spreadsheet.zoom.get/apply`，验证无附着视图和错误窗口被拒、查询/设置/放大/复位、重复设置 no-op，以及缩放不改变文档 revision/dirty。缩放属于窗口视图工作流，不计入 128 项 `CapabilityCommand` 专题矩阵。

`TestPDFEditCapabilitiesValidateAtomicityHistoryAndSaveReopen` 通过 application Runtime 调用 PDF 页旋转、文本替换、批注、高亮、墨迹和历史命令，验证显式页面目标、无效目标失败原子性、空历史/重复文本/空批注 no-op、revision/dirty 变化，以及 PDF 保存重开后保留修改和未编辑页面。该证据只验证 Glance 生成 PDF 的行为，不代表第三方 PDF 编辑器互操作。

`TestSpreadsheetImageCapabilitiesRequireHumanForDestructiveEditsAndRoundTrip` 通过 application Runtime 验证 XLSX 图片能力的显式锚点、PNG 插入、列表查询、失效图片 ID 的失败原子性、移动、保比例缩放、no-op、revision/dirty，以及 JPEG 替换和删除的人工确认闸门；拒绝替换不改文档，批准替换后图片字节/锚点/尺寸经 XLSX 保存重开保持，批准删除后列表为空。移动后测试重新查询图片 ID，避免依赖位置变化前的标识。

`TestSpreadsheetProtectionCapabilitiesRequireHumanAndRoundTripWithoutLeakingPassword` 通过 application Runtime 验证工作表/工作簿保护查询、明确作用域、启用/移除前的人类确认、重复设置 no-op、错误密码失败原子性，以及保护状态的 XLSX 保存重开。测试还检查密码不会出现在待确认响应、确认记录、最终结果和错误信息中；此保护不是文件加密。

`TestSpreadsheetShapeCapabilityUsesExplicitTargetsAndRoundTrips` 通过 application Runtime 验证 `spreadsheet.shape.create` 的明确工作表/锚点、几何类型、尺寸和文本，文本框默认尺寸，错误单元格/缺失工作表失败原子性、revision/dirty，以及两个形状在 XLSX 保存重开后的 DrawingML 锚点与文字。外部 Office 渲染兼容性仍未验证。

`TestSpreadsheetFilterCapabilityReadsExplicitRangeAndRoutesWindowIntent` 通过 application Runtime 验证筛选按明确范围/列和值或源行读取真实工作簿数据，生成匹配的可见行集并路由到显式附着窗口；无窗口与无效筛选条件失败，筛选和清除不改文档 revision/dirty。实际 Gio 网格隐藏/恢复行由 `TestValueFilterHidesRowsAndClearsToOriginalHeights` 覆盖。筛选是临时窗口状态，不写回 XLSX。

`TestSpreadsheetRecalculateCapabilityRecalculatesWorkbook` 通过 Runtime 写入 `SUM` 公式、修改其输入并调用注册重算能力，验证返回缓存失效状态、revision/dirty、Glance 读取到更新结果，以及 XLSX 保存重开后公式和值仍可读取。Excelize 的适配器移除公式缓存值供 Excel 打开时重算，Glance 本地读取调用其支持范围内的公式计算器；不遍历预计算所有公式或刷新外部连接。

`TestSpreadsheetPrintCapabilityRequiresHumanAndPreservesWorkbook` 通过 Agent 请求和人类批准路径调用 `spreadsheet.document.print`，验证明确工作表/范围、landscape 方向、份数和页码参数、捕获到的 PDF 可解析，打印前不会调用打印服务，打印不改变文档修订或脏状态。测试使用捕获式打印服务，不启动真实系统打印队列。

`TestWordPrintCapabilityRequiresHumanAndValidatesPages` 通过 Agent 请求和人类批准路径调用 `word.document.print`，验证 DOCX 实时快照被渲染为可解析 PDF，批准前不调用打印服务，页码范围参数生效且越界范围失败不会打印，打印不改变 Word 文档修订或 dirty 状态。该用例使用注入式捕获打印服务，不产生真实打印副作用。

`TestJSONTransformAndValidationCapabilitiesPreserveStateAndRoundTrip` 以外部 Agent 身份调用 JSON 格式化、压缩、键排序和只读校验，检查有效/无效 JSON 诊断、无效操作失败原子性、重复转换 no-op、revision/dirty，以及保存重开后的排序文本。干净文件重开后的排序 no-op 仍保持 clean。

`TestJSONSearchCapabilityReturnsEscapedPointersWithoutMutation` 验证 `json.document.search` 的 RFC 6901 键转义、匹配类型与位置、结果上限截断、大小写敏感查询、空查询拒绝，以及搜索后原文、revision 和 dirty 状态不变。`TestMarkdownEditCapabilityUsesRuneRangesAndRoundTrips` 通过 Runtime 逐项调用 `markdown.text.edit` 的标题、加粗、斜体、代码、链接、图片、列表、待办、引用和块级公式操作；覆盖中文 rune 偏移、操作结果、越界范围/缺少目标的失败原子性，以及 Markdown 保存重开。

`TestPDFSearchCapabilityPaginatesWithoutMutation` 验证 `pdf.document.search` 的不区分大小写匹配、逐页续查、稳定页码/run 与非空文本边界、越界起始页拒绝，以及搜索保持 revision/dirty。用例发现并修复了检查器使用 `width/height`、搜索解码却读取 `w/h` 导致命中边界尺寸归零的问题。

`TestWordDocumentProtectionCapabilityRequiresHumanAndRoundTrips` 验证 `word.document.protection.get/set` 的 Agent 查询与高风险确认门禁、Agent 无法批准、桌面人类批准、重复设置 no-op、关闭标记和 DOCX 保存重开。保护只是要求 Word 只读打开的编辑标记，不是密码或加密。

`TestDocumentPlanApplyRequiresHumanAndReportsPartialFailure` 覆盖 `document.plan.apply` 的 Agent 校验计划、桌面人类批准、按顺序应用并逐条递增 revision；第二个场景使中间命令因无效工作表失败，验证首条已完成操作保留、后续命令标记 `not_run`、返回 `partial_failed` 且不会自动回滚。此测试明确记录计划应用的部分提交语义。

`TestDocumentSaveCapabilityRequiresHumanAndWritesLiveSession` 验证 Agent 对现有 DOCX 的 `document.save` 请求在人工批准前不会覆盖源文件，Agent 可用 `confirmation.get` 查询待处理和完成状态，批准后写入同一实时会话的内容并可关闭重开；干净文档保存返回 `saved=false` 且不改变 revision。

`TestPresentationProofreadCapabilityPaginatesStableFindingsWithoutMutation` 验证 PPTX 启发式文本检查按幻灯片分页，报告重复中文标点和不平衡括号并携带稳定 slide_id/element_id；无效起点被拒绝，查询不改变文档 revision/dirty。该能力不代表词典拼写、语法或语义检查。

`TestWordHeaderFooterCapabilitiesRoundTrip` 验证 `word.header.set`、`word.footer.set` 和 `word.page_number.set` 的首写、重复 no-op、revision 与 DOCX 保存重开；桌面实时会话也重复调用三项能力，确认 revision 不递增。重复写入以实际 DOCX 部件内容相同为 no-op；页脚命令仍按说明替换页脚内容，因此重复页脚检查在新增动态页码前执行。

`TestWordLanguageAndStatisticsCapabilities` 验证 `word.proofing.language.set` 对明确 `run_id` 的首写、同语言重复 no-op、失效运行 ID 失败原子性，以及后续 Word 统计查询只读；桌面实时会话也确认重复设置不推进 revision。设置语言只标记 DOCX 校对语言，不执行拼写检查或语言检测。

`TestWordFontListAndSpacingCapabilitiesRoundTrip` 验证字体族、字号、段落列表和段前/段后间距的重复设置返回 `changed=false` 且不推进 revision；`TestWordParagraphAlignmentAndIndentCapabilitiesRoundTrip` 覆盖对齐和左右缩进的重复 no-op，并验证有效布局值查询及 DOCX 保存重开。`TestWordTextRunStyleSetIsIdempotentAndRoundTrips` 覆盖组合文本样式的重复 no-op 与保存重开。

`TestWordTextRunReplaceCommandUsesNeutralRunIdentity` 覆盖稳定 `run_id` 的有效替换、重复 no-op、缺失 run 失败原子性及 DOCX 保存重开；`TestWordTextRangeReplaceUsesUnicodeRuneOffsets` 覆盖中文/emoji rune 区间替换、等效范围 no-op、逆序范围失败原子性和保存重开。保存重开后按解码的文本字段核对，避免把 JSON 转义字节误当文档内容。

`TestSpreadsheetFormulaCapabilityUsesExplicitCellAndRoundTrips` 以外部 Agent 身份为 XLSX 的明确工作表/单元格写入公式，检查缺失工作表、非法地址、空公式和超过限制的公式失败原子性，验证含可选等号最多 8192 个 UTF-16 单元的公式边界（补充平面字符按 2 个单元计）、带/不带等号的重复公式 no-op，以及 Save As、关闭重开后的公式持久化和 clean 状态。该限制对齐 Excel 公式内容上限；Glance 仅验证公式存储，不背书计算引擎或 Excel 互操作。

`TestSpreadsheetCellSetCommandMutatesAndRequeriesLiveDocument` 覆盖单格设置的返回值、重复 no-op、缺失工作表/非法地址失败原子性、revision/dirty 和 XLSX 保存重开；`TestSpreadsheetBatchCellSetIsAtomicIdempotentAndRoundTrips` 覆盖多格批量写入、重复 no-op、重复坐标与缺失工作表失败时保持全部旧值、revision/dirty，以及 XLSX 保存重开。

`TestPresentationLayoutCapabilitiesUseLiveCatalogAndRoundTrip` 通过 `presentation.layouts.list` 读取当前修订的版式目录，再用稳定 `slide_id` 应用版式；验证无效 slide_id/目录索引失败原子性、重复版式 no-op、revision/dirty 和 PPTX 保存重开后的关系持久化。

`TestSpreadsheetSortAndHistoryCapabilitiesUseLiveWorkbookAndRoundTrip` 以外部 Agent 身份调用 XLSX 区域排序和工作簿撤销/重做，验证明确范围内的行值关联、缺失工作表和包含公式区域的失败原子性、排序 no-op、空历史 no-op、revision/dirty，以及排序后的 XLSX 保存重开。

`TestSpreadsheetDuplicateRemovalAndTextSplitCapabilitiesRoundTrip` 通过已确认的人类命令路径验证高风险的删重与拆列处理器：重复行只删除重复项且保留首次出现，拆列按明确列和分隔符写出字段；缺失工作表/越界列失败原子性、两项操作的 no-op、revision/dirty 和 XLSX 保存重开均有断言。MCP 外部 Agent 的确认门禁由高风险能力策略统一执行。

`TestSpreadsheetFlashFillCapabilityTreatsFilledRangeAsNoop` 以外部 Agent 身份调用 `spreadsheet.flash_fill.apply`，验证数字序列补全、已有源模式但目标已满时成功 no-op、无源数据失败原子性、revision/dirty 与 XLSX 保存重开。无源模式仍会返回明确错误。

`TestSpreadsheetChartCapabilityUsesExplicitRangeAndRoundTrips` 以外部 Agent 身份调用 `spreadsheet.chart.create`，验证图表明确引用的工作表、数据范围和锚点、缺失工作表失败时 revision/dirty 不变，以及保存重开后图表标题、分类范围和数值范围仍存在。该命令创建图表，不适用重复请求 no-op 语义。

`TestSpreadsheetConditionalFormatAndValidationCapabilitiesRoundTrip` 以外部 Agent 身份调用 `spreadsheet.conditional_format.set` 和 `spreadsheet.data_validation.set`，覆盖四类条件格式、下拉列表和数字范围验证、缺失工作表失败原子性、重复设置/清除 no-op、revision/dirty，以及规则设置/清除后的 XLSX 保存重开。

`TestSpreadsheetTableCapabilityCreatesExplicitTableAndRoundTrips` 验证 Agent 以明确工作表、范围、表名和样式创建结构化表格；缺失工作表和重名请求失败时 revision/dirty 不变，保存重开后表名和 A1:B3 范围仍存在。

`TestSpreadsheetPivotCapabilityUsesNamedFieldsAndRoundTrips` 验证 Agent 透视表使用显式源范围、字段名和输出区域；缺失源字段失败原子性、返回参数、revision/dirty，以及 XLSX 透视表和缓存部件保存重开均有断言。

`TestSpreadsheetRangeQueryAndFormattingCapabilitiesUseExplicitTargets` 验证有界区域复制和大小写不敏感搜索为只读查询，并检查缺失工作表、反向区域、超单元格上限和超结果数上限均被拒绝且不改变 revision/dirty；`spreadsheet.range.format.set` 与 `spreadsheet.format.paint` 使用明确区域/源单元格，检查无效工作表失败原子性、重复格式 no-op、revision/dirty 及保存重开后的目标样式。格式刷的 no-op 以规范化后的样式 ID 判定，避免把同一外观重复写入历史。

`TestSpreadsheetStructureCapabilityRequiresHumanAndRoundTrips` 验证高风险 `spreadsheet.structure.edit` Agent 请求必须等待人类确认，拒绝不会修改文档，批准后创建的工作表会递增一次 revision、标脏，并在 XLSX 保存重开后仍存在。

`TestWordTrackRevisionsCapabilitiesUseAbsoluteState` 验证修订跟踪启用/关闭、同状态 no-op、错误参数失败原子性、revision/dirty，以及两种状态各自经 DOCX 保存重开后的查询结果。`TestWordWatermarkCapabilitiesRoundTripThroughRuntime` 检查水印写入后的 dirty/revision、重复设置 no-op、保存后 clean 和 DOCX 保存重开内容。`TestPresentationGroupCapabilityUsesExplicitAdjacentElementIDs` 验证 Agent 组合请求先进入人类确认、审批前文档无变化、批准后组合成功、无效元素失败原子性及 PPTX 保存重开。

Word 文本操作补齐了稳定 ID 错误路径和格式往返：`TestWordFontListAndSpacingCapabilitiesRoundTrip`、`TestWordParagraphAlignmentAndIndentCapabilitiesRoundTrip`、`TestWordTextRunStyleSetIsIdempotentAndRoundTrips` 检查无效 run/paragraph ID、no-op 时 revision/dirty 保持及 DOCX 保存重开；`TestWordFormatPainterUsesExplicitSourceAndTargetRuns` 同时覆盖源/目标 ID 失效、no-op、dirty 和保存重开。`TestWordTextInsertCapabilitySupportsExplicitRunAndParagraphTargets`、`TestWordEquationCapabilityReplacesRunsAndInsertsTextBoxes`、`TestWordPageBreakCapabilitySplitsUnicodeRun` 覆盖未知稳定目标失败原子性并检查 DOCX 保存重开；`TestWordTextRangeCopyCutAndPasteUseExplicitUnicodeRanges` 另验证空范围 cut no-op 和复制/剪切/粘贴后的文本保存重开。封面插入用例也断言写入后 dirty、保存后 clean。`TestWordHeaderFooterCapabilitiesRoundTrip`、`TestWordPageBackgroundCapabilitySetsAndClears` 和 `TestWordAllParagraphSpacingCapabilityRoundTripsAndNoops` 进一步断言有效 mutation 后 dirty、no-op/无效请求不改变状态、保存后 clean；Word 与 PPTX 批注删除同样检查 dirty/revision 及保存后的 clean 状态。

这批测试只把之前缺少直接行为证据的命令补齐到 Glance 生成的文档往返边界；测试通过不等于真实 Office、LibreOffice 或打印硬件互操作验收。其余命令仍需按有效目标、无效目标、no-op、dirty-state 和适用的保存重开证据逐项核对；格式能力证据见 `office-feature-matrix.md` 和 `office-support.md`。

当前 318 项 Ribbon 操作均已接入能力目录（Word 104、表格 88、演示文稿 74、JSON 12、Markdown 17、PDF 23），清单中不再有 `planned` 项。能力目录另含 41 项 Agent 专属查询与工作流。`spreadsheet.cells.transfer` 是同工作簿富单元格粘贴能力，保留普通公式、类型和样式并作一步撤销；Agent 调用受明确目标与修订校验约束。`spreadsheet.range.fill_down` 将 Ctrl/Cmd+D 的首行向下填充作为一项有界、可撤销的明确区域命令，并通过 MCP 暴露。`spreadsheet.range.fill_right` 将 Ctrl/Cmd+R 的最左列向右填充作为一项有界、可撤销的明确区域命令，并通过 MCP 暴露。`spreadsheet.cells.merge` 为 Home“合并后居中”提供高风险 XLSX 合并/取消合并操作，外部 Agent 调用须经人工确认；内容丢失前会返回内容绑定的确认令牌，实际合并作为一步撤销操作。 `spreadsheet.cell.date.insert` 将 Control+; 映射到当前单元格的静态日期写入；General 单元格使用 Excel 内置日期格式，已有非 General 样式保留，重复写入当天不新增历史。`spreadsheet.cell.time.insert` 将 Windows/Linux Ctrl+Shift+; 与 macOS Command+; 映射到当前单元格的静态时间写入；General 单元格使用 Excel 内置 24 小时制格式，已有非 General 样式保留，重复写入同一分钟不新增历史。`workspace.recent.list` 为“最近使用”提供有界、可筛选和可分页的 MCP 查询；需要已连接工作区存储。`word.version.list` 为已保存 DOCX 查询最多 50 条历史元数据，不读取快照正文。`word.autosave.set` 为明确 Word 实时会话启用或关闭自动保存；MCP 调用须经桌面人类确认，因为后续编辑会自动写回源文件；未保存的新文档在另存为前不会尝试写文件。`document.export_copy` 将实时 DOCX/XLSX 快照写入显式新路径，不改变源会话路径、修订号或脏状态，不覆盖已有目标；桌面 Word 文本或表格单元格草稿未提交时会拒绝导出，Word 和 XLSX 的“共享”Ribbon 复用此命令。`word.document.export_pdf` 根据 DOCX 实时快照排版并生成新 PDF，支持文档方向、纵向和横向，返回页数、文件大小与布局诊断；Ribbon PDF 文件生成使用同一 PDF 写入与校验器，MCP 导出以修订锁保护最终新文件提交。`word.review.editor.analyze` 为 Word“开始/编辑器”和“审阅/编辑器”提供同一有界 MCP 查询：按 `start_block`、`max_blocks` 分页，最多返回 500 条发现，并给出稳定正文块 ID、索引、段落、单元格和文本运行 ID；只检查重复中文标点和中文括号/弯引号数量平衡，启发式分数只为兼容旧版 Ribbon 展示，不代表质量，不做词典拼写、语法或语义分析。`markdown.document.export_pdf` 从 Markdown 实时快照生成纵向/横向 PDF，复用相同的 PDF 校验与独占新建服务；不可用图片会保留占位符并以有界警告返回，未同步草稿或输入法组合文本会阻止导出。`spreadsheet.document.export_pdf` 按明确工作表范围导出经校验的新 PDF，支持纵向/横向、锚定图片和最多 5000×100（总计 500000）个单元格；Ribbon 导出复用此能力，使用当前工作表已用范围，MCP 可指定范围，目标存在时拒绝覆盖。`spreadsheet.document.print` 按明确工作表范围生成临时 PDF 并发送到桌面原生打印服务；Ribbon 打印当前工作表已用范围，MCP 可指定范围、方向、份数和页码，MCP 请求经 Gio 人工确认；无原生服务时调用会明确返回不支持。`pdf.document.print` 将指定实时 PDF 修订版本发送到同一打印服务；Ribbon 打印全部页面，MCP 可指定一基页码范围和份数，桌面请求经 Gio 人工确认；无原生服务、未提交草稿或修订已变化时明确失败。`spreadsheet.workbook.recalculate` 由“重新计算”和“刷新全部”Ribbon 共用：使公式缓存失效，后续读取时按 Excelize 支持范围按需计算，不遍历整本工作簿、不刷新外部数据连接。`spreadsheet.protection.get/set` 查询或设置明确目标；MCP 写操作须人工确认，待确认密码仅短暂保留于进程内存，确认摘要与结果不回显。`spreadsheet.text.proofread` 由 Ribbon 拼写检查入口和 MCP 共用，按明确工作表/范围执行有界只读检查；可检查连续 ASCII 空格与不可见字符，Agent 可选传入拉丁词典核对词条。未传词典时不判断单词拼写；词典未命中的词只表示需要复核，不宣称必然拼错。`word/review/review-compare` 通过 `document.compare` 比较当前文档与另一份已打开的 Word 文档；比较按双方 document_id 和 expected_revision 执行，正文分页最多 1000 块，Ribbon 以后台任务显示变化路径，能力只读。`word.macros.inspect` 由 Word“宏”入口与 MCP 共用，按明确 DOCX 实时快照检查 VBA 部件名和宏启用内容类型；它不读取或执行宏代码，也不提供恶意宏判定。`word.addins.list` 由 Word“加载项”入口与 MCP 共用，按当前运行时目录列出适用于 Word 的能力 ID、用途、风险和 JSON Schema，并可按类别筛选；它展示内建能力，不安装第三方扩展。`word.document.print` 由 Word“打印”入口与 MCP 共用，绑定明确修订版本，按可选份数/页码生成 PDF 并交给桌面原生打印服务；MCP 请求经 Gio 人工确认，未提交草稿或版本变化会阻止打印，无桌面打印服务的无窗口 Runtime 明确返回不支持。Word 首批包括 1 项最近文件查询、1 项版本历史查询、1 项自动保存设置、3 项文档生命周期、1 项副本导出、5 项文本样式、4 项段落对齐、3 项段落缩进、2 项字号、1 项字体、2 项列表、2 项段落间距、1 项文本范围替换、1 项文本范围复制、1 项文本范围剪切、1 项文本范围粘贴、1 项格式刷、1 项文本水印、1 项修订跟踪开关，以及接受和拒绝全部修订 2 项、1 项符号插入、1 项排列对齐、1 项有界文本搜索、1 项表格插入、1 项封面生成、2 项分页/分栏操作、1 项空白页、1 项文档信息、1 项页面颜色、1 项主题配色、1 项主题字体、1 项文档主题间距预设、2 项段落间距预设、1 项翻译文本替换、1 项设计器版式预设、1 项显式目标听写文本插入、6 项图片操作、1 项书签、1 项超链接、1 项文本框、1 项形状、1 项艺术字、1 项图表模板数据表、1 项公式文本、1 项页眉、1 项页脚、1 项页码、6 项页面布局设置、1 项校对语言、1 项字数统计、1 项编辑器启发式审阅（两个 Ribbon 入口共用）、原生批注查询及新建/删除能力、1 项文档编辑保护、1 项宏部件检查、1 项 Word 原生打印和 11 项窗口视图设置和查询。Word 视图工具可按窗口查询和组合设置版式、阅读模式、标尺、网格线、导航窗格、单页/多页布局及缩放、新窗口和并排查看；`word.view.set` 还支持按稳定 run_id 与 Unicode 字符偏移选择明确正文范围或全文，“选择”Ribbon 按钮复用该处理器；选择只改变窗口状态，不修改文档。文本样式工具可以组合多个属性；对齐和缩进工具使用明确的段落 ID 和绝对值，Agent 可一次执行更完整的编辑。复制按显式范围读取并返回文本，剪切按显式范围原子删除并返回文本，粘贴通过现有范围替换命令写入；它们都不依赖隐式 UI 选区。表格首批支持打开/另存、共享副本导出、单格文本、翻译结果写入、公式、最多 1000 个单元格的批量写入、区域读取与复制、受限区域格式、显式源和目标区域的格式刷、稳定排序、最多返回 1000 个匹配项的查找、工作簿历史撤销/重做、页面布局、网格线、定义名称、冻结窗格、普通视图与分页预览、标尺、工作表缩放、按值筛选、批注查询与修改、单元格超链接读写与删除、条件格式、数据验证、结构化表格（空白或重复标题会自动改名）、数据透视表（显式源区域及行列/求和字段）、柱状/折线/饼图插入、任意单元格锚点形状/文本框插入和需人类确认的结构修改；对应 Ribbon 操作复用这些命令。剪切和粘贴由 Agent 使用显式范围读取与批量写入组合完成。JSON 已接入格式化、压缩、键排序和校验；三种转换通过同一个 `json.text.transform` 工具的 operation 参数选择，校验通过 `json.document.validate` 返回结构化结果。`json.document.search` 在键名和值中执行有界搜索，并返回 JSON Pointer；最多读取 8 MiB、遍历 200000 个值和返回 500 项，Ribbon 已输入搜索词时复用该查询。`json.view.set` 将全部展开、全部折叠和展开层级路由到附着窗口的 ViewIntent，只改变窗口视图。`markdown.view.set` 通过同一机制切换编辑、预览和分屏模式。`json.document.read_text` 返回最多 8 MiB 的完整原文，JSON Ribbon 复制按钮也复用该读取处理器。Markdown 的标题、文本样式、引用、代码、链接、图片、列表、待办和公式按钮通过 `markdown.text.edit` 处理；Agent 使用明确 rune 区间和参数，桌面按钮提交当前区间。Agent 保存到当前源路径的操作通过 `document.save` 返回人工确认请求；新路径仍由不会覆盖已有文件的 `document.save_as` 处理。PDF 的“另存为”和“下载”均复用 `document.save_as`，Agent 提供明确路径，目标扩展名须匹配且不覆盖已有文件。演示文稿经典批注通过 `presentation.comments.list`、`presentation.comment.add` 和高风险 `presentation.comment.delete` 提供按 slide_id 分页查询、明确 point 坐标创建和 comment_id 删除；Ribbon 新建/删除入口复用同一能力，删除请求经 Gio 人类确认。当前不编辑新版 PowerPoint 批注格式。演示文稿“新建幻灯片”通过 `presentation.slide.add` 在文档末尾追加默认版式页，Ribbon 和 MCP 共用处理器；删除通过高风险能力 `presentation.slide.delete` 按 `document.inspect` 返回的稳定 `slide_id` 定位，Agent 调用须经桌面人类确认，Ribbon 人工操作复用同一处理器，最后一页不能删除；窗口视图通过 `presentation.view.set` 导航幻灯片、缩放、适配窗口、控制备注面板和水平/垂直标尺，并切换普通、幻灯片浏览和放映模式；幻灯片浏览卡片按稳定 slide_id 返回普通视图并定位目标页。`presentation.view.get` 读取状态，修改幻灯片或进入放映时会保护未提交的文本草稿。演示文稿形状通过 `presentation.element.shape.create` 按稳定 slide_id、几何类型和 point 坐标创建矩形、圆角矩形或椭圆，默认提供可见填充和轮廓；“开始/形状”和“插入/形状”按钮通过点击页面创建矩形并复用此命令。演示文稿图片通过 `presentation.element.picture.create` 显式传入不超过 32 MiB 的 PNG/JPEG Base64 数据、point 坐标、适配方式和替代文本；Ribbon 选图后点击画布复用同一处理器；图片替换通过 `presentation.element.picture.replace` 按稳定 slide_id 和 element_id 传入最多 32 MiB 的新图数据，保留元素 ID 与几何框并清除旧裁剪，Ribbon 选中图片后选图共用此能力；`presentation.element.picture.crop.set` 按四边万分比设置图片裁剪，Ribbon 点击“完成”后提交预览并调用同一处理器；进入、取消和重置裁剪预览通过 `presentation.view.set` 附着到窗口，按明确图片 ID 更新 ViewIntent，不修改文档。演示文稿音频和视频分别通过 `presentation.element.audio.create` 与 `presentation.element.video.create` 按稳定 slide_id、point 坐标和本机 source_path 嵌入，MCP 读取 MP3/WAV 音频最多 64 MiB、MP4/WebM 视频最多 128 MiB；Ribbon 选择文件后复用同一命令，画布以“音频/视频”占位标签显示。演示文稿文本框通过 `presentation.element.textbox.create` 按稳定幻灯片 ID 和 point 坐标创建，返回稳定元素 ID；Ribbon 文本框操作进入放置模式，点击页面后复用同一命令。演示文稿文本通过 `presentation.element.text.set` 按稳定幻灯片 ID 和元素 ID 写入；样式通过 `presentation.element.style.set` 一次组合多个明确字段（含项目符号状态），21 个 Ribbon 格式动作共用此处理器。`document.inspect` 返回稳定 ID、文本段落、运行样式、几何和可编辑标记；特殊字段/多段落等暂不安全的内容会明确拒绝。桌面 Ribbon 和 MCP 共用命令目录、修订检查与格式适配器；桌面会话聚合所有窗口的未提交文本草稿，Agent 写入遇到草稿时返回 `pending_editor_draft`。`presentation.element.transform.set` 可按 point 设置明确元素的位置、大小和旋转角度；桌面拖动/缩放/旋转手势和相对画布的“排列/对齐”预设也复用该处理器，Agent 可直接设置任意几何参数。`presentation.slide.notes.set` 通过稳定 slide_id 设置最多 32767 字符的演讲者备注，调用方传入查询得到的 `expected_notes` 执行原子比较后写入，桌面备注编辑器复用同一命令；并发冲突保留本地草稿并提供“加载最新/保留我的修改”选择。形状与艺术字 Ribbon 复用 `word.text_box.insert`，其当前格式边界是文本框表格和可见文字前缀；图表 Ribbon 复用 `word.table.insert`，当前生成模板数据表而非嵌入图表；公式 Ribbon 复用 `word.equation.insert`，Agent 可按稳定 run_id 替换公式文本，或按段落 ID 插入文本框，能力明确不创建 Office 数学对象。主题配色复用页面底色能力，主题字体复用明确文本运行字体能力；Agent 可对任意颜色或 run_id 操作，能力范围大于 Ribbon 预设。文档主题复用全段落间距能力，Agent 可传任意段前/段后值。翻译入口复用文本运行替换能力，由调用方生成译文；Glance 不在此工具内调用外部翻译服务。Word“设计器”复用全段落间距能力，当前提供固定预设；Agent 可传任意段前/段后值。听写入口复用 `word.text.insert`，支持指定 run 追加或指定段落后插入文本框，输入可来自任意外部识别/生成流程；Glance 不声称提供语音识别。表格翻译入口复用 `spreadsheet.cells.set`，Ribbon 写入当前单元格，Agent 可一次写入最多 1000 个明确单元格，翻译由调用方生成。演示文稿“背景格式”通过 `presentation.slide.background.set` 与 MCP 共用显式 slide_id 和 #RRGGBB 纯色背景命令；它只替换幻灯片背景填充并保留背景属性中的其他效果。演示文稿“幻灯片大小”通过 `presentation.slide.size.set` 接收明确 point 宽高，MCP 可设置 36–4032 point 范围内的尺寸，Ribbon 提供 16:9、4:3 和 A4 横向预设；改画布不会自动缩放或移动现有对象，暂存 PPTX 会重开并校验尺寸后再替换。演示文稿“导出 PDF”通过 `presentation.document.export_pdf` 接收明确的新目标路径及可选 A4 方向，Ribbon 预览与 MCP 共享实时快照、渲染器、修订检查和不覆盖写入器；支持范围以已投影内容为准，音视频返回占位符警告，无法安全渲染的内容失败关闭。演示文稿“打印”通过 `presentation.document.print` 使用相同的 A4 渲染器，并可指定份数和页码范围；Ribbon 打印预览确认后调用同一能力，MCP 请求经 Gio 人工确认，平台无打印服务时明确不可用。动画 Ribbon“动画窗格”复用只读 `presentation.animations.list`，按明确 slide_id 检查单页 OOXML 时序，按 XML 顺序返回 OOXML 时序节点（不等于实际播放顺序）、目标元素、效果预设和原始时长；最多解析 4 MiB、500 个动画项与 100000 个 XML 令牌，超限会返回截断标记。固定版本 go-pptx 只开放动画时序读取；Glance 通过保留未知 XML 的暂存包覆盖追加进入动画。`presentation.element.animation.add` 和 Ribbon“出现/淡入/飞入/添加动画”共用明确的 slide_id、element_id、效果及持续时间参数；不安全的现有时序会被拒绝。Agent-only `presentation.element.animation.delete` 使用同一修订下的动画列表序号与明确元素 ID，删除完整的 Glance 简单进入动画组并保留其余时序；复杂或共享目标的 XML 失败关闭。保存重开行为由 `TestPresentationAnimationCapabilitiesUseStableTargetsAndRoundTrip` 覆盖；PowerPoint/LibreOffice 实际播放仍待验证。动画时长通过 `presentation.element.animation.duration.set` 按 `presentation.animations.list` 的一基序号修改指定元素的 animEffect/animMotion 时长，Ribbon 会自动选中该元素最近一条可调进入动画；Agent 可直接指定任意 1–60000 毫秒。以上能力均已登记到清单；未支持的格式能力必须返回诊断，不能静默改写。Word 图片替换、删除、编辑保护变更、自动保存、Word、表格和 PDF 原生打印被标为高风险，Agent 调用须经人工确认；其他高影响动作按其能力风险级别执行确认策略。

## Ribbon 之外的 Agent 能力

Agent-only 能力见 JSON 清单，包括表格图片列表查询、按明确锚点插入和移动等更通用的图片操作（图片 ID 按当前锚点生成，移动后应重新查询），以及有界文档检查、Word 文档信息、页面设置和窗口视图查询、Word 页面底色和水印查询、工作簿审计、JSON 文档搜索、原文读取和附着窗口 JSON 树视图、Markdown 编辑视图、PDF 附着窗口视图查询、有界文本搜索和高风险文本替换，以及演示文稿窗口视图查询、按稳定 ID 写入演讲者备注、使用 `presentation.slide.duplicate` 复制整张幻灯片，`presentation.layouts.list` 按当前修订列出最多 500 个文档版式及其名称/文档内索引，并通过 `presentation.slide.layout.set` 按稳定 `slide_id` 和同一修订下的 `layout_index` 修改现有幻灯片版式；Ribbon“版式”选择器复用该能力。`workbook.audit` 已接入 MCP，可对限定区域执行只读检查并报告重复表头、必需表头缺失、重复键和错误样式值；`word.paragraph.append` 可让 Agent 在新建 Word 文档中逐段生成内容。修改计划限制在同一文档，先校验并绑定基准修订，再提供只读步骤预览；通用处理器没有模拟执行接口，所以预览明确不生成结构化文档差异。应用计划前由 Gio 人类确认，逐条执行，遇到失败停止并报告部分结果，不自动回滚。`confirmation.get` 查询高影响操作状态；批准动作只由 Gio 人类确认界面触发。演示文稿“淡化”“推进”“效果选项”“持续时间”和“全部应用”复用 `presentation.transition.set`；Agent 可按稳定 slide_id 选择 fade/push/wipe/split/cover/cut/dissolve，设置方向、速度、黑场和点击翻页，或应用至全体幻灯片。`presentation.transition.get` 读取当前设置。PPTX 速度是慢/中/快三档；不支持的扩展切换会拒绝读取或修改。演讲者视图通过 `presentation.view.set` 的 `mode=presenter` 进入当前窗口控制台，显示当前页、演讲者备注、下一页文本摘要，并自动开始窗口计时；Ribbon 与 MCP 共用 ViewIntent。此版本为单窗口演讲者视图，不提供双屏独立控制台。其它清单项仍按实施计划推进。这些能力没有单个 Ribbon 按钮，是 Agent 工作流的额外能力。

## 详细清单

逐项稳定 ID、说明、参数草案、目标和风险以同目录 `agent-command-coverage.json` 为准。MCP 工具描述与最终 Schema 从应用能力目录导出；覆盖测试核对 UI Action ID 与清单来源键，不替代格式保存/重开验收。`agent_status` 标记每项操作当前是否已映射到可调用的能力。

`spreadsheet.cell.set` 和 `spreadsheet.cells.set` 的 MCP 参数保留 JSON 原生类型：字符串按文本原样写入（如 `"00123"` 不会被推断成数字），数字和布尔值保存为对应 XLSX 类型；单格和批量写入均覆盖重复 no-op、失败原子性及保存重开。文本上限按 32767 个 Unicode 字符计算。证据见 `TestSpreadsheetCellCommandsPreserveExplicitJSONValueTypes` 与 `TestAgentSpreadsheetCellCommandsPreserveTypedValuesInLiveSession`；桌面 Ribbon 仍保留原有文本推断行为。

Word 的 `word.paragraph.append`、`word.text.insert`、`word.text_box.insert`、`word.equation.insert`、`word.header.set` 和 `word.footer.set` 的长度上限统一按 Schema 的 Unicode 字符数执行；16000 字符的限制不再把多字节中文错误计为多个字符。聚焦回归覆盖长中文成功写入、超限/换行失败不改 revision 或 dirty，并验证文本框保存重开。Ribbon 路径不受此 Agent 参数约束影响。

表格命令 `spreadsheet.comment.add` 的批注作者/正文、`spreadsheet.range.search` 的查询词和 `spreadsheet.text_to_columns.split` 的分隔符都按各自 Schema 的 Unicode 字符上限校验。此前处理器按 UTF-8 字节计数，会拒绝 Schema 已接受的中文或 emoji。`TestSpreadsheetCommentUsesUnicodeCharacterLimits` 覆盖长中文批注、作者、超限失败原子性及 XLSX 保存重开；`TestSpreadsheetRangeQueryAndFormattingCapabilitiesUseExplicitTargets` 覆盖多字节查询仍可执行且只读；`TestSpreadsheetDuplicateRemovalAndTextSplitCapabilitiesRoundTrip` 覆盖多字节分隔符拆分及 XLSX 保存重开。

`spreadsheet.defined_names.set` 的引用、作用域和说明按 Schema 声明的 Unicode 字符数检查；`word.text_range.replace` 的替换文本上限也按 1,048,576 个 Unicode 字符执行。`TestSpreadsheetDefinedNameCapabilitiesRoundTripAndNoop` 覆盖 100 个中文字符说明保存重开及超限失败原子性；`TestWordTextRangeReplaceUsesSchemaUnicodeLength` 以超过 1 MiB UTF-8 字节但低于 Schema 字符上限的文本验证范围写入与后续范围替换，并确认实时正文。

`TestWordRevisionApplyUsesLiveDOCXAndRoundTrips` 通过真实 DOCX Runtime 分别验证接受和拒绝全部修订：Agent 请求等待桌面人类审批，Agent 自批和人类拒绝不改文档；批准后返回实际修订数、更新 revision/dirty，保存重开后修订 XML 标记消失且保留的插入/删除文本符合操作语义。对已无待处理修订的重复操作返回 0 项并保持 clean 状态和 revision。

`TestWordBlankPageCapabilityInsertsTwoPageBreaks` 验证空白页命令可用明确的非空 Unicode rune 区间替换原文，同时保留前后文本、返回可编辑插入位置并写入两个分页符；缺失 run 和越界区间均失败且 revision/dirty 不变，保存后验证 DOCX 正文结构。

`word.bookmark.insert` 和 `word.hyperlink.insert` 的显式 `run_id` 不存在时会失败，不再由 DOCX 格式层回退到首个文本运行；空 `run_id` 的格式层默认插入行为仍保留给未指定目标的调用方。`TestWordBookmark_UnknownExplicitRunDoesNotFallBackOrMutate`、`TestWordHyperlink_UnknownExplicitRunDoesNotFallBackOrMutate` 验证格式层错误和无 XML/undo 变化；`TestWordBookmarkAndHyperlinkCapabilitiesRoundTrip` 验证 Runtime 命令拒绝失效目标且保持 revision/dirty。

`word.table.insert` 和 `word.text_box.insert` 的显式 `after_paragraph_id` 不存在时会失败，不再由 DOCX 格式层静默插入文档末尾；空 ID 的既有默认插入语义保持不变。`TestWordInsertTable_UnknownExplicitParagraphDoesNotFallBackOrMutate` 和 `TestWordTextBox_UnknownExplicitParagraphDoesNotFallBackOrMutate` 验证格式层错误不改变 XML、undo 或 dirty 状态；`TestWordTableInsertCapabilityCreatesEditableTable` 和 `TestWordTextBoxCapabilityRoundTrips` 验证 Runtime 命令拒绝失效目标且保持 revision/dirty。

DOCX 正文段落编辑器的 `InsertParagraphAfter` 对显式未知段落 ID 同样失败关闭，不再把插入位置改到文档末尾；空目标仍表示正文末尾追加。`TestInsertParagraphAfterUnknownExplicitTargetDoesNotFallBackOrMutate` 验证 XML、undo 和 dirty 均不变，`TestInsertParagraphAfterSpecificParagraphAndUndo` 验证有效目标仍能插入并撤销。`word.paragraph.append` 的 Agent 参数不包含锚点，仍按语义追加到正文末尾。

`json.view.set`、`markdown.view.set`、`pdf.view.set`、`word.zoom.dialog`、`word.view.new_window` 和 `word.view.side_by_side` 的能力层回归 `TestWindowCapabilitiesRouteValidRequestsAndRejectInvalidContext` 验证每种文档格式都把稳定 action、原始参数、文档 ID、revision 和明确窗口 ID 路由给附着视图；额外字段在路由前拒绝，未附着窗口返回 `ErrNoAttachedView`，操作不推进文档 revision。Gio 内的视图状态变化和 Ribbon 工作流另由各格式桌面测试覆盖。

Word 与 PPTX 原生批注的写入上限按 Schema 的 Unicode 字符数执行，不再错误地按 UTF-8 字节数限制中文。Word 批注正文/作者最多 32768/256 个字符，PPTX 最多 8192/256 个字符；批注列表仍将预览限制为 8 KiB，并在 rune 边界截断。`TestWordCommentUsesUnicodeCharacterLimitsAndRoundTrips` 与 `TestPresentationCommentUsesUnicodeCharacterLimitsAndRoundTrips` 覆盖 Schema 上限中文写入、有效 UTF-8 预览及 DOCX/PPTX 保存重开后的全文、作者和目标。
