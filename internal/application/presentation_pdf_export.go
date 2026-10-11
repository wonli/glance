package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"glance/internal/document"
	"glance/internal/format/presentationpdf"
)

type presentationSnapshotReader interface {
	PresentationSnapshot(context.Context) (document.PresentationSnapshot, error)
}

func registerPresentationPDFExportCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "presentation.document.export_pdf", Name: "导出演示文稿为 PDF",
			Description: "将明确指定的实时 PPTX 快照按每张幻灯片一页导出为 A4 PDF。target_path 必须以 .pdf 结尾；已有文件不会被覆盖，源文档路径、修订号和脏状态保持不变。可选 orientation 为 portrait 或 landscape；省略时按首张幻灯片宽高选择方向。输出使用 Glance 当前投影的支持子集，不包含动画、切换、媒体播放和未投影的 Office 内容；遇到无法安全渲染的内容会失败，音视频以文本占位符输出并在结果中警告。待提交编辑草稿、过期修订和已有目标文件会阻止导出。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"target_path":{"type":"string","minLength":1,"description":"必须以 .pdf 结尾的新输出路径；已存在的文件不会被覆盖。"},"orientation":{"type":"string","enum":["portrait","landscape"],"description":"A4 页面方向；省略时按首张幻灯片方向选择。"}},"required":["target_path"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1},"bytes_written":{"type":"integer","minimum":1},"page_count":{"type":"integer","minimum":1},"warnings":{"type":"array","maxItems":100,"items":{"type":"string","maxLength":512}},"warnings_truncated":{"type":"boolean"}},"required":["path","bytes_written","page_count","warnings","warnings_truncated"],"additionalProperties":false}`),
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
			reader, ok := capability.Adapter.(presentationSnapshotReader)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			release, err := capability.Runtime.lockDocumentRevision(ctx, capability.Document.DocumentID, capability.Document.Revision)
			if err != nil {
				return CapabilityResult{}, err
			}
			snapshot, err := reader.PresentationSnapshot(ctx)
			release()
			if err != nil {
				return CapabilityResult{}, err
			}
			if input.Orientation == "" {
				input.Orientation = presentationpdf.DefaultOrientation(snapshot)
			}
			result, err := presentationpdf.ExportSnapshot(ctx, path, snapshot, input.Orientation, func() (func(), error) {
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
