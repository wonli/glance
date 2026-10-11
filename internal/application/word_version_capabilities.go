package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const maxWordVersionHistoryResults = 50

type wordVersionMetadata struct {
	ID        int64  `json:"id"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

type wordVersionListResult struct {
	Versions []wordVersionMetadata `json:"versions"`
}

func registerWordVersionCapabilities(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.version.list", Name: "查询 Word 版本历史",
			Description: "查询指定已保存 DOCX 文档在当前工作区中的版本历史，按时间从新到旧返回。结果最多 50 项，只包含版本 ID、来源、时间、摘要和大小，不会读取版本正文；新文档必须先保存到路径。",
			Category:    CapabilityQuery, Risk: RiskLow, Formats: []Format{FormatDocx}, RequiresDocument: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"versions":{"type":"array","items":{"type":"object","properties":{"id":{"type":"integer"},"source":{"type":"string"},"created_at":{"type":"string"},"sha256":{"type":"string"},"size_bytes":{"type":"integer"}},"required":["id","source","created_at","sha256","size_bytes"],"additionalProperties":false}}},"required":["versions"],"additionalProperties":false}`),
		},
		Handler: func(_ context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
			path := strings.TrimSpace(capability.Document.SourcePath)
			if path == "" {
				return CapabilityResult{}, ErrNoSavePath
			}
			store := capability.Runtime.WorkspaceStore()
			if store == nil {
				return CapabilityResult{}, ErrWorkspaceUnavailable
			}
			versions, err := store.ListWordVersions(canonicalPath(path))
			if err != nil {
				return CapabilityResult{}, err
			}
			if len(versions) > maxWordVersionHistoryResults {
				versions = versions[:maxWordVersionHistoryResults]
			}
			result := wordVersionListResult{Versions: make([]wordVersionMetadata, 0, len(versions))}
			for _, version := range versions {
				result.Versions = append(result.Versions, wordVersionMetadata{
					ID: version.ID, Source: string(version.Source),
					CreatedAt: version.CreatedAt.UTC().Format(time.RFC3339Nano),
					SHA256:    version.SHA256, SizeBytes: version.SizeBytes,
				})
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				return CapabilityResult{}, err
			}
			return CapabilityResult{Data: encoded}, nil
		},
	})
}
