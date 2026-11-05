<p align="center">
  <img src="design/logo.png" alt="Glance logo" width="128" />
</p>

# Glance

English · [简体中文](README.zh-CN.md)

A tabbed desktop workspace that brings document editing, an AI assistant, and proofreading together. Glance’s Agent and connected MCP clients work with the same live document sessions as the desktop, including unsaved changes. The interface supports English and Simplified Chinese.

<p align="center">
  <img src="design/Glance%20Default%20Home.png" alt="Glance home screen" width="100%" />
</p>

## Features

- Edit Word, Excel, CSV, supported PowerPoint content, Markdown, and JSON files.
- Read and annotate PDFs. Merge or split PDFs from **Tools → PDF Merge & Split**.
- Compare PDFs and manuscripts character by character in **Tools → Document Workbench**, with OCR alignment and resumable projects. Install and configure Git to keep page-level history.
- Use the AI assistant with a configured provider.

High-impact Agent actions require confirmation. See the [MCP setup guide](docs/mcp-connection.md) to connect another client.

Legacy `.xls` files import into the XLSX editor. Legacy `.doc` import is supported on macOS. See [format support](docs/office-support.md) for limitations.

## Quick start

Requires Go 1.27.1 or later. macOS builds require Xcode Command Line Tools.

```sh
go run ./cmd/glance
```

Pass a file path to open it on launch: `go run ./cmd/glance path/to/document.docx`. Use the **+** menu to open files or create drafts. Choose the interface language in **Settings → Basic**.

Build a desktop app on macOS with `make macos-app`, or a Windows x64 executable with `make windows`.

## AI and OCR

Configure a provider in **Settings → AI Providers**. Agent sends chat messages and document excerpts you ask it to read to that provider. OCR in Document Workbench requires a PaddleOCR token and uploads the reference PDF or selected page image to that service.

On Windows, Markdown diagrams require Microsoft Edge or Google Chrome.

## More

[Compatibility](docs/compatibility/office-feature-matrix.md) · [Architecture](docs/architecture.md) · [Third-party notices](packaging/notices/THIRD_PARTY_NOTICES.md)

## License

The repository is licensed under [Apache-2.0](LICENSE), except for the reserved [enterprise modules](ee/LICENSE) and third-party components under their own terms. Glance names and logos are trademarks. Forks should use their own branding.
