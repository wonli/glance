package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func registerDocumentExportCopyCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "document.export_copy", Name: "导出文档副本",
			Description: "将明确的 DOCX 或 XLSX 实时会话快照写入新的本地文件，不改变当前会话路径、修订号或脏状态。目标扩展名必须匹配，已有文件不会被覆盖；桌面 Word 文本或表格单元格存在未提交草稿时会拒绝导出。",
			Category:    CapabilityCommand, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
			Formats:      []Format{FormatDocx, FormatXLSX, FormatCSV},
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1}},"required":["path"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1},"bytes_written":{"type":"integer","minimum":1}},"required":["path","bytes_written"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, err
			}
			path := strings.TrimSpace(input.Path)
			if path == "" {
				return CapabilityResult{}, ErrInvalidDocumentPath
			}
			path = canonicalPath(path)
			if !extensionMatchesFormat(filepath.Ext(path), capability.Document.Format) {
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
			data, err := snapshooter.Snapshot(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			if len(data) == 0 {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			if err := writeNewFile(ctx, path, data); err != nil {
				return CapabilityResult{}, err
			}
			return capabilityJSONResult(struct {
				Path         string `json:"path"`
				BytesWritten int64  `json:"bytes_written"`
			}{Path: path, BytesWritten: int64(len(data))}, false)
		},
	})
}

func writeNewFile(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrTargetExists
		}
		return err
	}
	var written bool
	defer func() {
		if !written {
			_ = os.Remove(path)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	written = true
	return nil
}
