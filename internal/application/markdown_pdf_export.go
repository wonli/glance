package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"glance/internal/format/markdownpdf"
)

func registerMarkdownPDFExportCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "markdown.document.export_pdf", Name: "导出 Markdown 为 PDF",
			Description: "将明确 Markdown 文档在 expected_revision 对应的实时快照排版并导出到 target_path。target_path 必须以 .pdf 结尾；已有文件不会被覆盖，源文档路径、修订号和脏状态保持不变。可选 orientation 为 portrait（默认）或 landscape。导出会按 Glance 既有 Markdown 资源规则读取最多 128 项本地图片或 HTTP/HTTPS 图片，并把不可用图片替换为占位符；结果会返回缺失图片数量、资源警告和布局诊断。无法安全解析或排版的内容会使导出失败。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatMarkdown}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"target_path":{"type":"string","minLength":1,"description":"必须以 .pdf 结尾的新输出路径；已存在的文件不会被覆盖。"},"orientation":{"type":"string","enum":["portrait","landscape"],"description":"页面方向，默认 portrait。"}},"required":["target_path"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1},"bytes_written":{"type":"integer","minimum":1},"page_count":{"type":"integer","minimum":1},"missing_image_count":{"type":"integer","minimum":0},"resource_warnings":{"type":"array","maxItems":100,"items":{"type":"string","maxLength":512}},"resource_warnings_truncated":{"type":"boolean"},"diagnostics":{"type":"array","maxItems":1000,"items":{"type":"object","properties":{"severity":{"type":"string"},"source_id":{"type":"string"},"code":{"type":"string"},"detail":{"type":"string"}},"required":["severity","source_id","code","detail"],"additionalProperties":false}},"diagnostics_truncated":{"type":"boolean"}},"required":["path","bytes_written","page_count","missing_image_count","resource_warnings","resource_warnings_truncated","diagnostics","diagnostics_truncated"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				TargetPath  string `json:"target_path"`
				Orientation string `json:"orientation"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			path := strings.TrimSpace(input.TargetPath)
			if path == "" {
				return CapabilityResult{}, ErrInvalidDocumentPath
			}
			path = canonicalPath(path)
			if !strings.EqualFold(filepath.Ext(path), ".pdf") {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if source := capability.Document.SourcePath; source != "" && canonicalPath(source) == path {
				return CapabilityResult{}, ErrTargetExists
			}
			if _, err := os.Lstat(path); err == nil {
				return CapabilityResult{}, ErrTargetExists
			} else if !errors.Is(err, os.ErrNotExist) {
				return CapabilityResult{}, err
			}
			snapshooter, ok := capability.Adapter.(interface {
				Snapshot(context.Context) ([]byte, error)
			})
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			snapshot, err := snapshooter.Snapshot(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			fonts, err := capability.Runtime.FontSnapshot(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			result, err := markdownpdf.ExportSnapshotWithFontSnapshot(ctx, path, snapshot, capability.Document.SourcePath, capability.Document.DocumentID, capability.Document.Revision, input.Orientation, fonts, func() (func(), error) {
				return capability.Runtime.lockDocumentRevision(ctx, capability.Document.DocumentID, capability.Document.Revision)
			})
			if err != nil {
				if errors.Is(err, os.ErrExist) {
					return CapabilityResult{}, ErrTargetExists
				}
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(result, false)
		},
	})
}
