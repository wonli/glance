# OCR 云端试验包

独立 Go 包，仅使用标准库。按用户提供的 PaddleOCR 异步 jobs API 示例实现，默认模型 `PaddleOCR-VL-1.6`。这不是本地 PP-StructureV3，也尚未接入校对 UI 或实现 PDF—Markdown 自动对齐。

- `Submit` 接收文件 reader 或 URL，上传至云端；`Options{}` 对应示例中的三个 false。
- `Status` / `Wait` 查询、轮询任务，支持 context 取消；取消只停止客户端等待，不取消服务端任务。提交不会自动重试，避免重复计费。
- `Results` / `ParseResults` 按服务返回顺序提取逐页 Markdown，保留图片 URL 和原始页面 JSON（供后续读取坐标等字段）。Number 是输出顺序，不是书籍印刷页码。
- 不下载图片、不写入文档、不存储凭据。结果存储请求不携带 API 令牌，拒绝 HTTP 重定向。
- 原型上传暂在内存组装 multipart，请使用少量页面样本；尚未针对整本大 PDF 做流式上传、内存或服务页数上限验证。

从仓库根目录运行（令牌由调用环境提供）：

```sh
# 先在本机环境中设置 PADDLEOCR_TOKEN，不要把令牌写入仓库。
go run ./cmd/ocrprobe -file /absolute/path/sample.pdf -timeout 30m
# 或 -url https://example.com/sample.pdf
```

stdout 输出分页 JSON，stderr 输出任务 ID 与进度。调用会向 PaddleOCR 服务发送文档内容。业务调用必须在后台任务中执行，不能放在 Gio layout/frame 中。

针对性验证：`go test ./internal/ocr ./cmd/ocrprobe -count=1`。测试使用合成令牌和本机 HTTP 服务，覆盖文件/URL 提交、轮询、取消、异常状态、JSONL 分页和结果请求的令牌隔离；不证明实际古籍识别或 AB 定位准确。

## 2026-10-24 实际接口试验

用户指定的《唐诗汇评 增订本 4》PDF 共 892 页，约 206 MiB。本次仅将第 30 页渲染为长边 2400px 的 JPEG（约 274 KiB）提交默认云端接口，任务完成并返回 1 页 Markdown。识别内容以“卢纶”开头，页面末尾为“月香飘桂”。

对 OCR 结果和用户提供的 B 做临时匹配：仅保留 U+4E00–U+9FFF 汉字与数字后，OCR 的 363 个字符整体连续匹配 B 的归一化偏移 `[10558, 10921)`（零基、半开区间）。此处偏移不是原始 Markdown 的字符偏移，不可直接用于编辑；标点、版面或异体字准确性也未由这个结果证明。

这是横排单页的云端识别与锚点验证，不是整本或竖排准确率，也未实现自动首尾对齐算法。原始 PDF/MD 未修改；临时图片和识别 JSON 已在验证后删除。后续需要测试相邻页、重复文字、竖排及夹注，再实现保留原始 B 偏移的映射。

## 对齐试验入口

`Page.Layout()` 提取结构化版面（文字块、坐标、竖排标签、阅读顺序），保留空图片区域。独立的 `internal/textalign` 与 `cmd/alignprobe` 已可消费这些结果作只读定位；状态和真实前四页限制见 `../textalign/README.md`。仍未接入 UI 或应用自动分页。
