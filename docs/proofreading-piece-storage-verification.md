# 空盒补位与共享片段存储验证（2026-10-23）

## 行为与数据结构

- 当前盒吸空下一盒后，后续内容依次前移一盒，至已校验盒之前停止。盒子编号与 A/X 页面位置固定；字符身份和顺序不变。收取和补位同一步撤销。
- 固定页索引数组持有不可变 AVL 文本片段树。叶子共享 UTF-8 来源及字节范围，最长 1024 字符；缓存字符数和可退回后缀长度。同来源、身份连续、状态一致的叶子可合并引用，无需复制正文。
- 普通收退仅拆分／拼接局部片段树；整盒补位移动根引用并延迟清除旧 Origin，不逐字符重写。复杂度分别为局部树操作及 O(补位盒数)，不是声称整段补位 O(1)。
- 历史只记录变动盒子的前后根、状态和 Next。派生 Reflow 合并到原历史；无操作锁定／端点分页不消耗撤销，也不清除 redo。
- v2 保存格式兼容。旧锁定／Reflow 接口通过隔离转换桥处理；当前编辑和滚动不调用转换桥，不维护两套可变正文。
- 每帧累计所有补位页面，后续帧替换后台排版时合并未完成的 dirty 页面。可见编辑器使用本页快照；后台排版和保存捕获共享根，在工作线程展开整稿。

## 同基准比较

Apple M1，892 页，每页 8 个片段，每片段重复 32 次“中文校对 mixed English。”，`-benchtime=20x`。每轮包含操作与一次撤销，避免不断耗尽下一盒或无限增长历史。数据属于模型操作路径，不代表窗口总响应时间。

| 模型操作 | 修改前 | 最终源码 | 修改前分配 | 最终分配 |
| --- | ---: | ---: | ---: | ---: |
| 收取一个字符＋撤销 | 8,891,002 ns | 1,379 ns | 910,114 B | 1,546 B |
| 吸空下一盒、后续补位＋撤销 | 9,737,238 ns | 151,131 ns | 894,190 B | 275,692 B |

另外一次892页桌面路径采样：普通滚动帧 414,456 ns / 90,748 B；控制器逐字操作 250,377 ns / 198,220 B。它包含本页字素分段和 UI 工作，不能与上表纯模型耗时混用。

## 检查结果

- `go test ./internal/proofreading -count=1` 通过。
- application 的 Proofreading 定向回归通过；包含字素簇、实际排版行、过期邻页预览、连续收取。
- desktopui 的补位、跨帧排版、同帧多步、撤销、已校验隔离、保存迁移、失败保护及 Navigator 回归通过。
- `TestProofreadingNavigatorVisual`、`TestProofreadingPageScrollKeepsTextGeometryVisual`、`TestProofreadingLockedPageVisual`、`TestProofreadingLongPageViewportGPU` 通过，无 SKIP；覆盖实际 GPU 像素及各测试的缩放场景。这不是原生鼠标 UE 验收。
- 独立只读审查发现并修正派生历史合并和语义无操作两项问题；复核未发现剩余可操作问题。
- `git diff --check` 通过。按用户指定命令在本代码目录启动；链接器仍报告既有重复 `-lobjc` 警告。

## 未通过与验证限制

本轮 `go test ./internal/desktopui -run '^TestProofreading' -count=1 -v` 仍有两项失败；它们在更换存储结构前也已观察到，本次没有修改文字测量／字体布局来处理：

- `TestProofreadingScaledScrolledEditorAcceptsChineseAtVisibleLine`：点击插入所在行与预期第 50 行不同。
- `TestProofreadingLayoutMatchesVisibleEditorLines`：后台排版与可见编辑器行位置不同。

前面较广的 application 检查还观察到：`TestMarkdownExportPDFWritesValidatedNewFile`、`TestPresentationExportAndPrintCapabilitiesUseRealSnapshot`、`TestWordExportPDFWritesValidatedNewFileWithoutChangingSession`、`TestWordPrintCapabilityRequiresHumanAndValidatesPages` 失败，涉及字体矢量轮廓或投影边界；未纳入本次修复。没有据专项通过声称整个应用包或全仓通过。

原生验证前已退出旧进程，使用当前目录 `GLANCE_PPROF_ADDR=127.0.0.1:6060 go run ./cmd/glance` 启动新进程并核对可执行路径。Computer Use 无法绑定未打包的 go run 可执行文件（返回 Invalid app），因此未在新进程上完成“6 吸空 7、8 自动补位”的原生鼠标验收。未创建应用副本、独立缓存、截图或日志文件；未编辑用户原始 A/B 文件。


## 后续：连续滚动与当前页同步（2026-10-23）

用户实测后反馈滚动卡、首行似乎不变，随后澄清正文位于上一盒底部，并非已确认丢失。只读核查两个 892 页保存副本：模型正文与文件正文一致，正文 SHA-256 与导入时的 SourceBHash 一致；没有改写或回滚副本。用户取消新增拖动功能，本轮仅修复现有交互。

### 已复现和修正

- 普通输入此前 16dp 一字、每帧最多四字；现在 1dp 一字并保留小数累计，Shift 仍 16dp 一行。压缩输入队列，以约 4ms 工作预算处理，而非固定四字；反向输入清除未消费的反向积压，边界处丢弃无效余量。
- 字素边界按相邻盒子的不可变根缓存；普通收退复用，邻页编辑、补位、撤销等会失效重建。每字符保持独立历史。
- 文字示例从 2342 行开始，邻盒更新但当前页文字不变时，旧路径因为整稿 Revision 不一致而积压滚轮。现在以当前页正文／样式检查几何可用性，视口、可见文字和滚动条可及时前移。实际 GPU 页面区域像素也确认变化。
- 吸空下一盒后的旧 worker 会先处理整个后续书稿。现在先处理可见页和当前盒对；离屏脏页保留到可见时处理，部分结果只清除已处理页的 dirty 状态。空白后续页也不会被额外调度。
- 无字体快照的无窗口测量帮助函数现在与其 Editor 同用系统字体入口，解决此前两项测量／中文输入测试不一致。真实窗口明确传入托管字体快照的路径不变；不将无窗口字体修正描述成真实窗口 UE 验收。

### 最终检查

- `go test ./internal/proofreading -count=1` 通过。
- `go test ./internal/application -run '^TestProofreading' -count=1` 通过。
- `go test ./internal/desktopui -run '^TestProofreading' -count=1 -v` 全部通过，无 FAIL／SKIP；包括此前失败的 `TestProofreadingScaledScrolledEditorAcceptsChineseAtVisibleLine`、`TestProofreadingLayoutMatchesVisibleEditorLines`。
- 新增回归覆盖 1x/2x 编号正文及 GPU 像素、64 字突发输入与逐字撤销、892 页补位优先排可见页、离屏 dirty 保留及空白后续页不调度。
- 独立只读审查未发现新的可操作问题。`git diff --check` 通过。
- 退出旧 Glance，再在本目录用指定 `go run` 命令启动新代码；链接器保留重复 `-lobjc` 警告。未创建应用副本、独立缓存、截图或日志。自动化 GPU 与输入检查不能替代用户原生鼠标手感验收。


## 后续：顶部双向调整（2026-10-24）

已复现：停留本盒顶部在 `K8IiR0` 滚动仍只修改本盒末尾，并由 `revealProofreadingNavigatorBoundary` 强制跳到末尾，无法把本盒页首微调给上一盒。

本轮改为按当前浏览边界选择相邻盒对：顶部向下拉入上一盒末尾，向上送回本盒开头；两个方向保持本盒顶部，短盒子也不通过滚动方向切换边界。共享模型沿用相邻根转移，没有复制／删除正文。上一盒或本盒已校对时停止。Shift-up 按本盒首行，而非上一盒末行尚余宽度；Shift-down 按上一盒末行。

验证：新增 1x/2x、长短盒、首次向下拉取、向上退回、立即反向、上一盒已校对、字符 ID／全局顺序和撤销回归；首次运行观察到错误边界失败，修复后通过。application 的 Proofreading 定向检查通过；desktopui 全部 `^TestProofreading` 回归通过，无 FAIL／SKIP。独立只读复核未发现剩余可操作问题，`git diff --check` 通过。仍不把 Gio/GPU 自动化检查称作原生鼠标验收。
