package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"glance/internal/format/wordpdf"
)

func registerWordPDFExportCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.document.export_pdf", Name: "导出 Word 为 PDF",
			Description: "将明确 DOCX 文档在 expected_revision 对应的实时快照排版并导出到 target_path。target_path 必须以 .pdf 结尾；已有文件不会被覆盖，源文档路径、修订号和脏状态保持不变。Word 待提交文本草稿会阻止导出。可选 orientation 为 document（采用文档页面设置，默认值）、portrait 或 landscape；显式方向应用于默认页面设置和首节。布局警告会随结果返回；阻止可靠输出的布局错误会使导出失败。",
			Category:    CapabilityWorkflow, Formats: []Format{FormatDocx}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"target_path":{"type":"string","minLength":1},"orientation":{"type":"string","enum":["document","portrait","landscape"]}},"required":["target_path"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1},"bytes_written":{"type":"integer","minimum":1},"page_count":{"type":"integer","minimum":1},"diagnostics":{"type":"array","maxItems":1000,"items":{"type":"object","properties":{"severity":{"type":"string"},"source_id":{"type":"string"},"code":{"type":"string"},"detail":{"type":"string"}},"required":["severity","source_id","code","detail"],"additionalProperties":false}},"diagnostics_truncated":{"type":"boolean"}},"required":["path","bytes_written","page_count","diagnostics","diagnostics_truncated"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				TargetPath  string `json:"target_path"`
				Orientation string `json:"orientation"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, err
			}
			path := strings.TrimSpace(input.TargetPath)
			if path == "" {
				return CapabilityResult{}, ErrInvalidDocumentPath
			}
			if input.Orientation == "" {
				input.Orientation = "document"
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
			result, err := wordpdf.ExportSnapshot(ctx, path, snapshot, capability.Document.DocumentID, capability.Document.Revision, input.Orientation, func() (func(), error) {
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

// lockDocumentRevision serializes a short file-commit operation against
// commands that could make an already-rendered document snapshot stale.
func (r *Runtime) lockDocumentRevision(ctx context.Context, documentID string, expectedRevision uint64) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session, err := r.session(documentID)
	if err != nil {
		return nil, err
	}
	session.mu.Lock()
	if err := ctx.Err(); err != nil {
		session.mu.Unlock()
		return nil, err
	}
	if session.closed {
		session.mu.Unlock()
		return nil, ErrDocumentNotFound
	}
	if session.revision != expectedRevision {
		current := session.revision
		session.mu.Unlock()
		return nil, fmt.Errorf("%w: current=%d expected=%d", ErrRevisionConflict, current, expectedRevision)
	}
	if drafts, ok := session.adapter.(interface{ HasPendingDrafts() bool }); ok && drafts.HasPendingDrafts() {
		session.mu.Unlock()
		return nil, ErrPendingEditorDraft
	}
	return session.mu.Unlock, nil
}
