package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	defaultRecentListLimit  = 20
	maxRecentListLimit      = 100
	maxRecentListOffset     = 10000
	maxRecentListQueryRunes = 256
)

var ErrWorkspaceUnavailable = errors.New("workspace storage is not configured")

type recentDocumentResult struct {
	Path     string `json:"path"`
	Format   string `json:"format"`
	OpenedAt string `json:"opened_at"`
}

type recentDocumentListResult struct {
	Items      []recentDocumentResult `json:"items"`
	HasMore    bool                   `json:"has_more"`
	NextOffset int                    `json:"next_offset,omitempty"`
}

func registerWorkspaceCapabilities(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "workspace.recent.list", Name: "查询最近使用文件",
			Description: "查询当前 Glance 工作区保存的最近文件，按打开时间从新到旧返回。可按格式和路径关键词筛选；使用 limit 与 offset 分页。每次最多返回 100 项，关键词按字面子串匹配。需要当前运行时已连接工作区存储。",
			Category:    CapabilityQuery, Risk: RiskLow,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"format":{"type":"string","description":"按文件格式筛选：doc、docx、xlsx、xls、pptx、json、markdown 或 pdf。"},"query":{"type":"string","description":"路径中的字面子串，最多 256 个 Unicode 字符。"},"limit":{"type":"integer","description":"最多返回条数，默认为 20，最大为 100。","minimum":1,"maximum":100},"offset":{"type":"integer","description":"跳过符合筛选条件的最近项，范围为 0 到 10000。","minimum":0,"maximum":10000}},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"path":{"type":"string"},"format":{"type":"string"},"opened_at":{"type":"string"}},"required":["path","format","opened_at"],"additionalProperties":false}},"has_more":{"type":"boolean"},"next_offset":{"type":"integer"}},"required":["items","has_more"],"additionalProperties":false}`),
		},
		Handler: func(_ context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Format string `json:"format"`
				Query  string `json:"query"`
				Limit  int    `json:"limit"`
				Offset int    `json:"offset"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return CapabilityResult{}, fmt.Errorf("decode recent list arguments: %w", ErrInvalidCommandArguments)
			}
			if input.Limit == 0 {
				input.Limit = defaultRecentListLimit
			}
			if input.Limit < 1 || input.Limit > maxRecentListLimit || input.Offset < 0 || input.Offset > maxRecentListOffset || utf8.RuneCountInString(input.Query) > maxRecentListQueryRunes {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			format := strings.ToLower(strings.TrimSpace(input.Format))
			if format != "" {
				switch Format(format) {
				case FormatDOC, FormatDocx, FormatXLSX, FormatXLS, FormatCSV, FormatPPTX, FormatPDF, FormatJSON, FormatMarkdown:
				default:
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			}
			store := capability.Runtime.WorkspaceStore()
			if store == nil {
				return CapabilityResult{}, ErrWorkspaceUnavailable
			}
			recents, err := store.ListRecents(input.Limit+1, input.Offset, format, input.Query)
			if err != nil {
				return CapabilityResult{}, fmt.Errorf("list workspace recents: %w", err)
			}
			result := recentDocumentListResult{Items: make([]recentDocumentResult, 0, input.Limit)}
			if len(recents) > input.Limit {
				result.HasMore = true
				result.NextOffset = input.Offset + input.Limit
				recents = recents[:input.Limit]
			}
			for _, recent := range recents {
				result.Items = append(result.Items, recentDocumentResult{
					Path: recent.Path, Format: recent.Kind, OpenedAt: recent.OpenedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
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
