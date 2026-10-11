package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	maxDocumentPlanCommands = 32
	maxDocumentPlanBytes    = 256 << 10
	maxDocumentPlans        = 128
	documentPlanTTL         = 30 * time.Minute
)

var (
	ErrDocumentPlanUnavailable = errors.New("document plan is missing, expired, stale, or already applying")
	ErrDocumentPlanStoreFull   = errors.New("document plan store is full")
)

type documentPlanCommand struct {
	CapabilityID string          `json:"capability_id"`
	Name         string          `json:"name"`
	Risk         RiskClass       `json:"risk"`
	Arguments    json.RawMessage `json:"arguments"`
}

type documentPlanIssue struct {
	Index        int    `json:"index"`
	CapabilityID string `json:"capability_id,omitempty"`
	Code         string `json:"code"`
	Message      string `json:"message"`
}

type documentPlanRecord struct {
	ID           string
	DocumentID   string
	BaseRevision uint64
	Commands     []documentPlanCommand
	CreatedAt    time.Time
	ExpiresAt    time.Time
	State        string
	Result       json.RawMessage
}

type documentPlanService struct {
	mu      sync.Mutex
	records map[string]*documentPlanRecord
	order   []string
}

func newDocumentPlanService() *documentPlanService {
	return &documentPlanService{records: make(map[string]*documentPlanRecord)}
}

func (s *documentPlanService) create(record documentPlanRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(time.Now())
	for len(s.records) >= maxDocumentPlans {
		removed := false
		for index, id := range s.order {
			candidate := s.records[id]
			if candidate == nil || candidate.State != "applying" {
				delete(s.records, id)
				s.order = append(s.order[:index], s.order[index+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			return ErrDocumentPlanStoreFull
		}
	}
	record.State = "validated"
	cloned := cloneDocumentPlan(record)
	s.records[record.ID] = &cloned
	s.order = append(s.order, record.ID)
	return nil
}

func (s *documentPlanService) get(id string) (documentPlanRecord, error) {
	if s == nil {
		return documentPlanRecord{}, ErrDocumentPlanUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(time.Now())
	record := s.records[id]
	if record == nil || !time.Now().Before(record.ExpiresAt) {
		return documentPlanRecord{}, ErrDocumentPlanUnavailable
	}
	return cloneDocumentPlan(*record), nil
}

func (s *documentPlanService) beginApply(id, documentID string, revision uint64) (documentPlanRecord, json.RawMessage, error) {
	if s == nil {
		return documentPlanRecord{}, nil, ErrDocumentPlanUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(time.Now())
	record := s.records[id]
	if record == nil || !time.Now().Before(record.ExpiresAt) || record.DocumentID != documentID || record.BaseRevision != revision {
		return documentPlanRecord{}, nil, ErrDocumentPlanUnavailable
	}
	if record.State == "applied" || record.State == "failed" || record.State == "partial_failed" {
		return cloneDocumentPlan(*record), append(json.RawMessage(nil), record.Result...), nil
	}
	if record.State != "validated" {
		return documentPlanRecord{}, nil, ErrDocumentPlanUnavailable
	}
	record.State = "applying"
	return cloneDocumentPlan(*record), nil, nil
}

func (s *documentPlanService) finish(id, state string, result json.RawMessage) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if record := s.records[id]; record != nil {
		record.State = state
		record.Result = append(json.RawMessage(nil), result...)
	}
	s.mu.Unlock()
}

func (s *documentPlanService) pruneLocked(now time.Time) {
	kept := s.order[:0]
	for _, id := range s.order {
		record := s.records[id]
		if record == nil || !now.Before(record.ExpiresAt) {
			delete(s.records, id)
			continue
		}
		kept = append(kept, id)
	}
	s.order = kept
}

func cloneDocumentPlan(record documentPlanRecord) documentPlanRecord {
	record.Commands = append([]documentPlanCommand(nil), record.Commands...)
	for index := range record.Commands {
		record.Commands[index].Arguments = append(json.RawMessage(nil), record.Commands[index].Arguments...)
	}
	record.Result = append(json.RawMessage(nil), record.Result...)
	return record
}

func registerDocumentPlanCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "document.plan.validate", Name: "校验文档修改计划",
				Description: "校验同一文档上的一组已注册修改命令、参数 Schema、格式支持和基准修订，并生成短期有效的计划 ID。计划最多 32 条命令、总参数 256 KiB；校验只读，不会修改文档。",
				Category:    CapabilityWorkflow, Risk: RiskLow, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"commands":{"type":"array","minItems":1,"maxItems":32,"items":{"type":"object","properties":{"capability_id":{"type":"string","minLength":1,"maxLength":128},"arguments":{"type":"object"}},"required":["capability_id","arguments"],"additionalProperties":false}}},"required":["commands"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"plan_id":{"type":"string"},"valid":{"type":"boolean"},"document_id":{"type":"string"},"base_revision":{"type":"integer"},"created_at":{"type":"string"},"expires_at":{"type":"string"},"commands":{"type":"array","items":{"type":"object","properties":{"capability_id":{"type":"string"},"name":{"type":"string"},"risk":{"type":"string"},"arguments":{"type":"object"}},"required":["capability_id","name","risk","arguments"],"additionalProperties":false}},"issues":{"type":"array","items":{"type":"object","properties":{"index":{"type":"integer"},"capability_id":{"type":"string"},"code":{"type":"string"},"message":{"type":"string"}},"required":["index","code","message"],"additionalProperties":false}}},"required":["plan_id","valid","document_id","base_revision","created_at","expires_at","commands","issues"],"additionalProperties":false}`),
			},
			Handler: handleDocumentPlanValidate,
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "document.plan.preview", Name: "预览文档修改计划",
				Description: "只读预览已校验计划的目标文档、基准修订、命令顺序、风险级别和参数。通用命令处理器尚未提供模拟执行接口，因此结果说明执行步骤和已知影响，不伪造未计算的文档差异。",
				Category:    CapabilityWorkflow, Risk: RiskLow, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"plan_id":{"type":"string","minLength":1}},"required":["plan_id"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"plan_id":{"type":"string"},"valid":{"type":"boolean"},"document_id":{"type":"string"},"base_revision":{"type":"integer"},"state":{"type":"string"},"commands":{"type":"array","items":{"type":"object","properties":{"capability_id":{"type":"string"},"name":{"type":"string"},"risk":{"type":"string"},"arguments":{"type":"object"}},"required":["capability_id","name","risk","arguments"],"additionalProperties":false}},"limitations":{"type":"array","items":{"type":"string"}},"issues":{"type":"array","items":{"type":"object","properties":{"index":{"type":"integer"},"capability_id":{"type":"string"},"code":{"type":"string"},"message":{"type":"string"}},"required":["index","code","message"],"additionalProperties":false}}},"required":["plan_id","valid","document_id","base_revision","state","commands","limitations","issues"],"additionalProperties":false}`),
			},
			Handler: handleDocumentPlanPreview,
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "document.plan.apply", Name: "应用文档修改计划",
				Description: "对已校验计划逐条执行修改。每个命令都会重新检查当前修订；执行前必须由桌面人类用户批准。若一条命令失败，后续命令停止，并返回已完成、失败和未执行明细；已完成的步骤不会自动回滚。",
				Category:    CapabilityWorkflow, Risk: RiskHigh, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"plan_id":{"type":"string","minLength":1}},"required":["plan_id"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"plan_id":{"type":"string"},"status":{"type":"string"},"start_revision":{"type":"integer"},"end_revision":{"type":"integer"},"commands":{"type":"array","items":{"type":"object","properties":{"index":{"type":"integer"},"capability_id":{"type":"string"},"status":{"type":"string"},"revision":{"type":"integer"},"data":{"type":"object"},"error":{"type":"string"}},"required":["index","capability_id","status","revision"],"additionalProperties":false}}},"required":["plan_id","status","start_revision","end_revision","commands"],"additionalProperties":false}`),
			},
			Handler: handleDocumentPlanApply,
		},
	}
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	return nil
}

func handleDocumentPlanValidate(_ context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
	if len(raw) > maxDocumentPlanBytes+16<<10 {
		return CapabilityResult{}, ErrInvalidCommandArguments
	}
	var input struct {
		Commands []struct {
			CapabilityID string          `json:"capability_id"`
			Arguments    json.RawMessage `json:"arguments"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return CapabilityResult{}, ErrInvalidCommandArguments
	}
	issues := make([]documentPlanIssue, 0)
	commands := make([]documentPlanCommand, 0, len(input.Commands))
	totalBytes := 0
	for index, command := range input.Commands {
		command.CapabilityID = strings.TrimSpace(command.CapabilityID)
		registration, exists := capability.Runtime.catalog.lookup(command.CapabilityID)
		if !exists || registration.descriptor.Category != CapabilityCommand || !registration.descriptor.RequiresDocument || !registration.descriptor.RequiresRevision {
			issues = append(issues, documentPlanIssue{Index: index, CapabilityID: command.CapabilityID, Code: "unsupported_capability", Message: "计划只接受需要明确文档和修订号的已注册修改命令。"})
			continue
		}
		if len(registration.descriptor.Formats) > 0 && !containsFormat(registration.descriptor.Formats, capability.Document.Format) {
			issues = append(issues, documentPlanIssue{Index: index, CapabilityID: command.CapabilityID, Code: "unsupported_format", Message: "该命令不支持目标文档格式。"})
			continue
		}
		arguments, err := canonicalJSON(command.Arguments)
		if err != nil || validateArguments(registration.descriptor.InputSchema, arguments) != nil {
			issues = append(issues, documentPlanIssue{Index: index, CapabilityID: command.CapabilityID, Code: "invalid_arguments", Message: "参数不符合该命令的输入 Schema。"})
			continue
		}
		totalBytes += len(arguments)
		if totalBytes > maxDocumentPlanBytes {
			issues = append(issues, documentPlanIssue{Index: index, CapabilityID: command.CapabilityID, Code: "plan_too_large", Message: "计划参数总量超过 256 KiB。"})
			break
		}
		commands = append(commands, documentPlanCommand{
			CapabilityID: registration.descriptor.ID, Name: registration.descriptor.Name,
			Risk: registration.descriptor.Risk, Arguments: arguments,
		})
	}
	if len(input.Commands) == 0 || len(input.Commands) > maxDocumentPlanCommands {
		issues = append(issues, documentPlanIssue{Index: 0, Code: "invalid_command_count", Message: "计划必须包含 1 到 32 条命令。"})
	}
	valid := len(issues) == 0 && len(commands) == len(input.Commands)
	createdAt := time.Now().UTC()
	expiresAt := createdAt.Add(documentPlanTTL)
	planID := ""
	if valid {
		var err error
		planID, err = newDocumentPlanID()
		if err != nil {
			return CapabilityResult{}, err
		}
		record := documentPlanRecord{
			ID: planID, DocumentID: capability.Document.DocumentID, BaseRevision: capability.Document.Revision,
			Commands: commands, CreatedAt: createdAt, ExpiresAt: expiresAt,
		}
		if err := capability.Runtime.documentPlans.create(record); err != nil {
			return CapabilityResult{}, err
		}
	}
	data, err := json.Marshal(struct {
		PlanID       string                `json:"plan_id"`
		Valid        bool                  `json:"valid"`
		DocumentID   string                `json:"document_id"`
		BaseRevision uint64                `json:"base_revision"`
		CreatedAt    time.Time             `json:"created_at"`
		ExpiresAt    time.Time             `json:"expires_at"`
		Commands     []documentPlanCommand `json:"commands"`
		Issues       []documentPlanIssue   `json:"issues"`
	}{planID, valid, capability.Document.DocumentID, capability.Document.Revision, createdAt, expiresAt, commands, issues})
	return CapabilityResult{Data: data}, err
}

func handleDocumentPlanPreview(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
	var input struct {
		PlanID string `json:"plan_id"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return CapabilityResult{}, ErrInvalidCommandArguments
	}
	record, err := capability.Runtime.documentPlans.get(input.PlanID)
	if err != nil || record.DocumentID != capability.Document.DocumentID || record.BaseRevision != capability.Document.Revision {
		issue := documentPlanIssue{Index: 0, Code: "plan_stale_or_expired", Message: "计划不存在、已过期，或目标文档修订已经变化。请重新校验计划。"}
		return capabilityJSONResult(struct {
			PlanID       string                `json:"plan_id"`
			Valid        bool                  `json:"valid"`
			DocumentID   string                `json:"document_id"`
			BaseRevision uint64                `json:"base_revision"`
			State        string                `json:"state"`
			Commands     []documentPlanCommand `json:"commands"`
			Limitations  []string              `json:"limitations"`
			Issues       []documentPlanIssue   `json:"issues"`
		}{input.PlanID, false, capability.Document.DocumentID, capability.Document.Revision, "invalid", []documentPlanCommand{}, []string{}, []documentPlanIssue{issue}}, false)
	}
	if err := ctx.Err(); err != nil {
		return CapabilityResult{}, err
	}
	return capabilityJSONResult(struct {
		PlanID       string                `json:"plan_id"`
		Valid        bool                  `json:"valid"`
		DocumentID   string                `json:"document_id"`
		BaseRevision uint64                `json:"base_revision"`
		State        string                `json:"state"`
		Commands     []documentPlanCommand `json:"commands"`
		Limitations  []string              `json:"limitations"`
		Issues       []documentPlanIssue   `json:"issues"`
	}{record.ID, true, record.DocumentID, record.BaseRevision, record.State, record.Commands,
		[]string{"当前预览列出计划命令、参数、顺序和风险；通用命令处理器没有模拟执行接口，因此不生成虚构的文档差异。"}, []documentPlanIssue{}}, false)
}

func handleDocumentPlanApply(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
	var input struct {
		PlanID string `json:"plan_id"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return CapabilityResult{}, ErrInvalidCommandArguments
	}
	record, previousResult, err := capability.Runtime.documentPlans.beginApply(input.PlanID, capability.Document.DocumentID, capability.Document.Revision)
	if err != nil {
		return CapabilityResult{}, err
	}
	if len(previousResult) > 0 {
		return CapabilityResult{Data: previousResult}, nil
	}
	startRevision := record.BaseRevision
	currentRevision := startRevision
	results := make([]struct {
		Index        int             `json:"index"`
		CapabilityID string          `json:"capability_id"`
		Status       string          `json:"status"`
		Revision     uint64          `json:"revision"`
		Data         json.RawMessage `json:"data,omitempty"`
		Error        string          `json:"error,omitempty"`
	}, 0, len(record.Commands))
	status := "applied"
	for index, command := range record.Commands {
		if err := ctx.Err(); err != nil {
			status = "partial_failed"
			if index == 0 {
				status = "failed"
			}
			results = append(results, planCommandFailure(index, command.CapabilityID, currentRevision, err.Error()))
			for rest := index + 1; rest < len(record.Commands); rest++ {
				results = append(results, planCommandSkipped(rest, record.Commands[rest].CapabilityID, currentRevision))
			}
			break
		}
		invocation, invokeErr := capability.Runtime.invokeCapability(ctx, CapabilityRequest{
			RequestID:  fmt.Sprintf("%s/%d", capability.RequestID, index+1),
			DocumentID: record.DocumentID, CapabilityID: command.CapabilityID,
			ExpectedRevision: currentRevision, expectedRevisionSet: true,
			Operator: capability.Operator, Arguments: command.Arguments,
		}, true)
		if invokeErr != nil {
			status = "partial_failed"
			if index == 0 {
				status = "failed"
			}
			results = append(results, planCommandFailure(index, command.CapabilityID, currentRevision, invokeErr.Error()))
			for rest := index + 1; rest < len(record.Commands); rest++ {
				results = append(results, planCommandSkipped(rest, record.Commands[rest].CapabilityID, currentRevision))
			}
			break
		}
		currentRevision = invocation.Revision
		results = append(results, struct {
			Index        int             `json:"index"`
			CapabilityID string          `json:"capability_id"`
			Status       string          `json:"status"`
			Revision     uint64          `json:"revision"`
			Data         json.RawMessage `json:"data,omitempty"`
			Error        string          `json:"error,omitempty"`
		}{index, command.CapabilityID, "completed", currentRevision, invocation.Data, ""})
	}
	result := struct {
		PlanID        string `json:"plan_id"`
		Status        string `json:"status"`
		StartRevision uint64 `json:"start_revision"`
		EndRevision   uint64 `json:"end_revision"`
		Commands      []struct {
			Index        int             `json:"index"`
			CapabilityID string          `json:"capability_id"`
			Status       string          `json:"status"`
			Revision     uint64          `json:"revision"`
			Data         json.RawMessage `json:"data,omitempty"`
			Error        string          `json:"error,omitempty"`
		} `json:"commands"`
	}{record.ID, status, startRevision, currentRevision, results}
	data, err := json.Marshal(result)
	if err != nil {
		capability.Runtime.documentPlans.finish(record.ID, "failed", nil)
		return CapabilityResult{}, err
	}
	capability.Runtime.documentPlans.finish(record.ID, status, data)
	return CapabilityResult{Data: data}, nil
}

func planCommandFailure(index int, capabilityID string, revision uint64, message string) struct {
	Index        int             `json:"index"`
	CapabilityID string          `json:"capability_id"`
	Status       string          `json:"status"`
	Revision     uint64          `json:"revision"`
	Data         json.RawMessage `json:"data,omitempty"`
	Error        string          `json:"error,omitempty"`
} {
	return struct {
		Index        int             `json:"index"`
		CapabilityID string          `json:"capability_id"`
		Status       string          `json:"status"`
		Revision     uint64          `json:"revision"`
		Data         json.RawMessage `json:"data,omitempty"`
		Error        string          `json:"error,omitempty"`
	}{index, capabilityID, "failed", revision, nil, message}
}

func planCommandSkipped(index int, capabilityID string, revision uint64) struct {
	Index        int             `json:"index"`
	CapabilityID string          `json:"capability_id"`
	Status       string          `json:"status"`
	Revision     uint64          `json:"revision"`
	Data         json.RawMessage `json:"data,omitempty"`
	Error        string          `json:"error,omitempty"`
} {
	return struct {
		Index        int             `json:"index"`
		CapabilityID string          `json:"capability_id"`
		Status       string          `json:"status"`
		Revision     uint64          `json:"revision"`
		Data         json.RawMessage `json:"data,omitempty"`
		Error        string          `json:"error,omitempty"`
	}{index, capabilityID, "not_run", revision, nil, "前序命令失败，未执行。"}
}

func newDocumentPlanID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("create document plan ID: %w", err)
	}
	return "plan-" + hex.EncodeToString(bytes[:]), nil
}
