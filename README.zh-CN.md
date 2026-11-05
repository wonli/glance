<p align="center">
  <img src="design/logo.png" alt="Glance 标志" width="128" />
</p>

# Glance

[English](README.md) · 简体中文

把文档编辑、AI 助手和校对工作流整合在一起的桌面工作区。Glance 内置 Agent 和外部 MCP 客户端共用桌面中的实时文档会话，也能读取尚未保存的修改。界面支持简体中文和英文。

<p align="center">
  <img src="design/Glance%20Default%20Home.png" alt="Glance 主页界面" width="100%" />
</p>

## 功能

- 编辑 Word、Excel、CSV、受支持的 PowerPoint 内容、Markdown 和 JSON 文件。
- 阅读并批注 PDF；在 **小工具 → PDF 合并拆分** 中合并或拆分 PDF。
- 在 **小工具 → 文档工作台** 中使用 OCR 对齐 PDF 和稿件，逐字比对并恢复校对进度。安装并配置 Git 后可保留分页历史。
- 配置服务商后使用 AI 助手。

Agent 执行高影响操作前会请求确认。连接 MCP 客户端的方式见 [MCP 配置指南](docs/mcp-connection.md)。

旧版 `.xls` 可导入 XLSX 编辑器。旧版 `.doc` 仅 macOS 支持导入。详细限制见[格式支持说明](docs/office-support.md)。

## 快速开始

需要 Go 1.27.1 或更新版本。macOS 构建需要 Xcode Command Line Tools。

```sh
go run ./cmd/glance
```

启动时传入文件路径即可打开文档：`go run ./cmd/glance path/to/document.docx`。使用 **+** 菜单打开文件或新建草稿。在 **设置 → 基础设置** 中切换界面语言。

在 macOS 上运行 `make macos-app` 构建桌面应用；运行 `make windows` 构建 Windows x64 可执行文件。

## AI 与 OCR

在 **设置 → AI 提供商** 中配置服务商。Agent 会将聊天消息和你让它读取的文档摘录发送给所选服务商。文档工作台的 OCR 需要 PaddleOCR Token，会将参考 PDF 或所选页面图像上传到该服务。

Windows 上的 Markdown 图表需要 Microsoft Edge 或 Google Chrome。

## 更多文档

[兼容性](docs/compatibility/office-feature-matrix.md) · [架构](docs/architecture.md) · [第三方组件声明](packaging/notices/THIRD_PARTY_NOTICES.md)

## 许可证

仓库采用 [Apache-2.0](LICENSE)，未来的[企业模块](ee/LICENSE)及按自身许可发布的第三方组件除外。Glance 名称和标志保留商标权利，分支项目应使用自己的品牌标识。
