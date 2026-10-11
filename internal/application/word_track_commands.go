package application

import (
	"context"
	"encoding/json"
)

type WordTrackRevisionsReader interface {
	GetWordTrackRevisions(context.Context) (bool, error)
}

type WordTrackRevisionsSetter interface {
	SetWordTrackRevisions(context.Context, bool) (bool, error)
}

type WordRevisionChangesController interface {
	ApplyWordRevisionChanges(context.Context, string) (int, error)
}

func registerWordTrackRevisionCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.review.track.get", Name: "读取 Word 修订跟踪状态",
				Description: "读取明确 DOCX 文档是否启用了修订跟踪；使用当前文档修订号。",
				Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"enabled":{"type":"boolean"}},"required":["enabled"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
				reader, ok := capability.Adapter.(WordTrackRevisionsReader)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				enabled, err := reader.GetWordTrackRevisions(ctx)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Enabled bool `json:"enabled"`
				}{enabled}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.review.track.set", Name: "设置 Word 修订跟踪",
				Description: "为明确 DOCX 文档开启或关闭修订跟踪。enabled 是绝对状态，Agent 应先查询当前值；使用当前文档修订号。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"enabled":{"type":"boolean"}},"required":["enabled"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"enabled":{"type":"boolean"}},"required":["changed","enabled"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Enabled *bool `json:"enabled"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, err
				}
				if input.Enabled == nil {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(WordTrackRevisionsSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetWordTrackRevisions(ctx, *input.Enabled)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool `json:"changed"`
					Enabled bool `json:"enabled"`
				}{changed, *input.Enabled}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.review.revisions.apply", Name: "接受或拒绝 Word 全部修订",
				Description: "对明确 DOCX 文档中的全部修订执行 accept_all 或 reject_all。接受会保留插入内容并永久移除删除内容；拒绝会移除插入内容并恢复删除内容。操作覆盖整份文档且不可按单条撤回，MCP 调用必须由桌面人类确认；执行前检查文档版本。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskHigh, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["accept_all","reject_all"],"description":"接受所有修订，或拒绝所有修订。"}},"required":["action"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["accept_all","reject_all"]},"revision_count":{"type":"integer","minimum":0},"changed":{"type":"boolean"}},"required":["action","revision_count","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Action string `json:"action"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.Action != "accept_all" && input.Action != "reject_all" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				controller, ok := capability.Adapter.(WordRevisionChangesController)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				count, err := controller.ApplyWordRevisionChanges(ctx, input.Action)
				if err != nil {
					return CapabilityResult{}, err
				}
				if count < 0 {
					return CapabilityResult{}, ErrInvalidCommandResult
				}
				return capabilityJSONResult(struct {
					Action        string `json:"action"`
					RevisionCount int    `json:"revision_count"`
					Changed       bool   `json:"changed"`
				}{input.Action, count, count > 0}, count > 0)
			},
		},
	}
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	return nil
}
