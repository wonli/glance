package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrHumanApprovalRequired  = errors.New("only a human operator can approve this operation")
	ErrConfirmationNotFound   = errors.New("confirmation request was not found")
	ErrConfirmationNotPending = errors.New("confirmation request is no longer pending")
	ErrConfirmationQueueFull  = errors.New("too many pending confirmation requests")
)

type ConfirmationStatus string

const (
	ConfirmationPending   ConfirmationStatus = "pending"
	ConfirmationRunning   ConfirmationStatus = "running"
	ConfirmationCompleted ConfirmationStatus = "completed"
	ConfirmationRejected  ConfirmationStatus = "rejected"
	ConfirmationFailed    ConfirmationStatus = "failed"
	ConfirmationExpired   ConfirmationStatus = "expired"
	ConfirmationCancelled ConfirmationStatus = "cancelled"
)

const (
	confirmationTTL         = 10 * time.Minute
	confirmationRecordTTL   = 24 * time.Hour
	maxPendingConfirmations = 64
	maxConfirmationRecords  = 256
)

// ConfirmationInfo is the safe, user-facing summary of a staged operation.
// It intentionally does not include the original arguments.
type ConfirmationInfo struct {
	ConfirmationID   string                      `json:"confirmation_id"`
	Status           ConfirmationStatus          `json:"status"`
	CapabilityID     string                      `json:"capability_id"`
	CapabilityName   string                      `json:"capability_name"`
	WindowID         uint64                      `json:"-"` // Local routing context; omitted from confirmation status payloads.
	DocumentID       string                      `json:"document_id,omitempty"`
	DocumentLabel    string                      `json:"document_label,omitempty"`
	ExpectedRevision uint64                      `json:"expected_revision"`
	ImpactSummary    string                      `json:"impact_summary"`
	CreatedAt        time.Time                   `json:"created_at"`
	ExpiresAt        time.Time                   `json:"expires_at"`
	ResolvedBy       string                      `json:"resolved_by,omitempty"`
	Error            string                      `json:"error,omitempty"`
	Result           *CapabilityInvocationResult `json:"result,omitempty"`
}

type confirmationRecord struct {
	info        ConfirmationInfo
	request     CapabilityRequest
	payloadHash string
}

type expiredConfirmationEvent struct {
	info      ConfirmationInfo
	requestID string
}

type confirmationService struct {
	runtime *Runtime
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	closed  bool
	records map[string]*confirmationRecord
	order   []string
	byKey   map[string]string
	byCall  map[string]string
}

func newConfirmationService(runtime *Runtime) *confirmationService {
	ctx, cancel := context.WithCancel(context.Background())
	return &confirmationService{
		runtime: runtime, ctx: ctx, cancel: cancel,
		records: make(map[string]*confirmationRecord),
		byKey:   make(map[string]string), byCall: make(map[string]string),
	}
}

func (r *Runtime) GetConfirmation(ctx context.Context, id string) (ConfirmationInfo, error) {
	if r == nil || r.confirmations == nil {
		return ConfirmationInfo{}, ErrRuntimeClosed
	}
	if err := ctx.Err(); err != nil {
		return ConfirmationInfo{}, err
	}
	return r.confirmations.get(id)
}

// ResolveConfirmation is an in-process human approval endpoint. It is not
// registered as an MCP capability, so an Agent cannot approve its own request.
func (r *Runtime) ResolveConfirmation(ctx context.Context, id string, approved bool, operator Operator) error {
	if r == nil || r.confirmations == nil {
		return ErrRuntimeClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(operator.ID) == "" || operator.Kind != OperatorHuman {
		return ErrHumanApprovalRequired
	}
	return r.confirmations.resolve(id, approved, operator)
}

func highRisk(risk RiskClass) bool {
	return risk == RiskHigh || risk == RiskCritical
}

func (r *Runtime) stageCommandConfirmation(ctx context.Context, request CapabilityRequest, command CommandRequest, descriptor CapabilityDescriptor) (CapabilityInvocationResult, error) {
	if err := ctx.Err(); err != nil {
		return CapabilityInvocationResult{}, err
	}
	if command.ExpectedRevision == 0 {
		return CapabilityInvocationResult{}, ErrExpectedRevisionRequired
	}
	arguments, err := canonicalJSON(command.Arguments)
	if err != nil {
		return CapabilityInvocationResult{}, fmt.Errorf("%w: %v", ErrInvalidCommandArguments, err)
	}
	if err := rejectAgentConfirmationFlag(request); err != nil {
		return CapabilityInvocationResult{}, err
	}
	if err := validateArguments(descriptor.InputSchema, arguments); err != nil {
		return CapabilityInvocationResult{}, err
	}
	if command.View.DocumentID != "" && command.View.DocumentID != command.DocumentID || command.View.Selection != nil && command.View.Selection.TargetType == "" {
		return CapabilityInvocationResult{}, ErrViewContextMismatch
	}
	session, err := r.session(command.DocumentID)
	if err != nil {
		return CapabilityInvocationResult{}, err
	}
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return CapabilityInvocationResult{}, ErrDocumentNotFound
	}
	info := session.infoLocked()
	if command.ExpectedRevision != info.Revision || command.View.Revision != 0 && command.View.Revision != info.Revision {
		session.mu.Unlock()
		return CapabilityInvocationResult{}, fmt.Errorf("%w: current=%d expected=%d", ErrRevisionConflict, info.Revision, command.ExpectedRevision)
	}
	if len(descriptor.Formats) > 0 && !containsFormat(descriptor.Formats, info.Format) {
		session.mu.Unlock()
		return CapabilityInvocationResult{}, fmt.Errorf("%w: %s does not support %s", ErrCapabilityUnsupported, descriptor.ID, info.Format)
	}
	if session.adapter.ReadOnly() {
		session.mu.Unlock()
		return CapabilityInvocationResult{}, ErrReadOnlyDocument
	}
	session.mu.Unlock()
	request.Arguments = arguments
	confirmationInfo, err := r.confirmations.stage(ctx, request, descriptor)
	if err != nil {
		return CapabilityInvocationResult{}, err
	}
	return confirmationInvocationResult(request, confirmationInfo)
}

func confirmationInvocationResult(request CapabilityRequest, info ConfirmationInfo) (CapabilityInvocationResult, error) {
	if info.Status == ConfirmationCompleted && info.Result != nil {
		result := *info.Result
		result.Data = append(json.RawMessage(nil), result.Data...)
		return result, nil
	}
	status := "confirmation_required"
	switch info.Status {
	case ConfirmationRunning:
		status = "confirmation_running"
	case ConfirmationRejected:
		status = "confirmation_rejected"
	case ConfirmationFailed:
		status = "confirmation_failed"
	case ConfirmationExpired:
		status = "confirmation_expired"
	case ConfirmationCancelled:
		status = "confirmation_cancelled"
	}
	data, err := json.Marshal(struct {
		Status           string    `json:"status"`
		ConfirmationID   string    `json:"confirmation_id"`
		CapabilityID     string    `json:"capability_id"`
		CapabilityName   string    `json:"capability_name"`
		DocumentID       string    `json:"document_id,omitempty"`
		DocumentLabel    string    `json:"document_label,omitempty"`
		ExpectedRevision uint64    `json:"expected_revision"`
		ImpactSummary    string    `json:"impact_summary"`
		ExpiresAt        time.Time `json:"expires_at"`
		Error            string    `json:"error,omitempty"`
	}{status, info.ConfirmationID, info.CapabilityID, info.CapabilityName, info.DocumentID, info.DocumentLabel, info.ExpectedRevision, info.ImpactSummary, info.ExpiresAt, info.Error})
	if err != nil {
		return CapabilityInvocationResult{}, err
	}
	return CapabilityInvocationResult{
		RequestID: request.RequestID, DocumentID: request.DocumentID,
		Revision: request.ExpectedRevision, Data: data,
	}, nil
}

func (s *confirmationService) stage(ctx context.Context, request CapabilityRequest, descriptor CapabilityDescriptor) (ConfirmationInfo, error) {
	if err := ctx.Err(); err != nil {
		return ConfirmationInfo{}, err
	}
	if err := rejectAgentConfirmationFlag(request); err != nil {
		return ConfirmationInfo{}, err
	}
	canonical, err := canonicalJSON(request.Arguments)
	if err != nil {
		return ConfirmationInfo{}, fmt.Errorf("%w: %v", ErrInvalidCommandArguments, err)
	}
	request.Arguments = canonical
	if request.DocumentID != "" {
		request.expectedRevisionSet = true
	}
	hash, err := confirmationPayloadHash(request)
	if err != nil {
		return ConfirmationInfo{}, err
	}
	ownerKey := string(request.Operator.Kind) + "\x00" + request.Operator.ID
	key := ""
	if request.IdempotencyKey != "" {
		key = ownerKey + "\x00" + request.IdempotencyKey
	}
	callKey := ownerKey + "\x00" + request.RequestID
	id, err := newConfirmationID()
	if err != nil {
		return ConfirmationInfo{}, err
	}
	now := time.Now().UTC()
	info := ConfirmationInfo{
		ConfirmationID: id, Status: ConfirmationPending, CapabilityID: descriptor.ID,
		CapabilityName: descriptor.Name, WindowID: request.View.WindowID, DocumentID: request.DocumentID,
		DocumentLabel:    confirmationDocumentLabel(ctx, s.runtime, request.DocumentID),
		ExpectedRevision: request.ExpectedRevision,
		ImpactSummary:    confirmationImpactSummary(s.runtime, descriptor, canonical, request.View),
		CreatedAt:        now, ExpiresAt: now.Add(confirmationTTL),
	}
	s.expireAndTrim(now)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ConfirmationInfo{}, ErrRuntimeClosed
	}
	var existingMatch *confirmationRecord
	for _, existingID := range []string{s.byKey[key], s.byCall[callKey]} {
		if existingID == "" {
			continue
		}
		existing := s.records[existingID]
		if existing == nil {
			continue
		}
		if existing.payloadHash != hash {
			s.mu.Unlock()
			return ConfirmationInfo{}, ErrIdempotencyConflict
		}
		if existingMatch == nil {
			existingMatch = existing
		}
	}
	if existingMatch != nil {
		result := cloneConfirmationInfo(existingMatch.info)
		s.mu.Unlock()
		return result, nil
	}
	pending := 0
	for _, record := range s.records {
		if record.info.Status == ConfirmationPending || record.info.Status == ConfirmationRunning {
			pending++
		}
	}
	if pending >= maxPendingConfirmations {
		s.mu.Unlock()
		return ConfirmationInfo{}, ErrConfirmationQueueFull
	}
	s.trimRecordLimitLocked()
	if len(s.records) >= maxConfirmationRecords {
		s.mu.Unlock()
		return ConfirmationInfo{}, ErrConfirmationQueueFull
	}
	request.Arguments = append(json.RawMessage(nil), canonical...)
	s.records[id] = &confirmationRecord{info: info, request: request, payloadHash: hash}
	s.order = append(s.order, id)
	if key != "" {
		s.byKey[key] = id
	}
	s.byCall[callKey] = id
	s.mu.Unlock()
	s.publish(info, request.Operator.Kind, request.RequestID)
	return cloneConfirmationInfo(info), nil
}

func (s *confirmationService) get(id string) (ConfirmationInfo, error) {
	if strings.TrimSpace(id) == "" {
		return ConfirmationInfo{}, ErrConfirmationNotFound
	}
	now := time.Now().UTC()
	s.mu.Lock()
	record := s.records[id]
	if record == nil {
		s.mu.Unlock()
		return ConfirmationInfo{}, ErrConfirmationNotFound
	}
	changed := false
	if record.info.Status == ConfirmationPending && !now.Before(record.info.ExpiresAt) {
		record.info.Status = ConfirmationExpired
		changed = true
	}
	info := cloneConfirmationInfo(record.info)
	requestID := record.request.RequestID
	s.mu.Unlock()
	if changed {
		s.publish(info, OperatorSystem, requestID)
	}
	return info, nil
}

func (s *confirmationService) resolve(id string, approved bool, operator Operator) error {
	now := time.Now().UTC()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrRuntimeClosed
	}
	record := s.records[id]
	if record == nil {
		s.mu.Unlock()
		return ErrConfirmationNotFound
	}
	if record.info.Status != ConfirmationPending {
		s.mu.Unlock()
		return ErrConfirmationNotPending
	}
	if !now.Before(record.info.ExpiresAt) {
		record.info.Status = ConfirmationExpired
		info := cloneConfirmationInfo(record.info)
		requestID := record.request.RequestID
		s.mu.Unlock()
		s.publish(info, OperatorSystem, requestID)
		return ErrConfirmationNotPending
	}
	record.info.ResolvedBy = operator.ID
	if !approved {
		record.info.Status = ConfirmationRejected
		info := cloneConfirmationInfo(record.info)
		requestID := record.request.RequestID
		s.mu.Unlock()
		s.publish(info, OperatorHuman, requestID)
		return nil
	}
	record.info.Status = ConfirmationRunning
	info := cloneConfirmationInfo(record.info)
	request := cloneCapabilityRequest(record.request)
	requestID := request.RequestID
	s.mu.Unlock()
	s.publish(info, OperatorHuman, requestID)
	go s.executeApproved(id, request)
	return nil
}

func (s *confirmationService) executeApproved(id string, request CapabilityRequest) {
	result, err := s.runtime.invokeCapability(s.ctx, request, true)
	s.mu.Lock()
	record := s.records[id]
	if record == nil || record.info.Status != ConfirmationRunning {
		s.mu.Unlock()
		return
	}
	if err != nil {
		record.info.Status = ConfirmationFailed
		record.info.Error = err.Error()
	} else {
		record.info.Status = ConfirmationCompleted
		if len(result.Data) > 0 || result.DocumentID != "" {
			result.Data = append(json.RawMessage(nil), result.Data...)
			record.info.Result = &result
		}
	}
	info := cloneConfirmationInfo(record.info)
	requestID := record.request.RequestID
	s.mu.Unlock()
	s.publish(info, OperatorSystem, requestID)
}

func (s *confirmationService) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	var changed []ConfirmationInfo
	for _, record := range s.records {
		if record.info.Status == ConfirmationPending || record.info.Status == ConfirmationRunning {
			record.info.Status = ConfirmationCancelled
			changed = append(changed, cloneConfirmationInfo(record.info))
		}
	}
	s.mu.Unlock()
	s.cancel()
	for _, info := range changed {
		s.publish(info, OperatorSystem, "")
	}
}

func (s *confirmationService) publish(info ConfirmationInfo, source OperatorKind, requestID string) {
	data, err := json.Marshal(info)
	if err != nil || s.runtime == nil {
		return
	}
	eventType := EventConfirmationUpdated
	if info.Status == ConfirmationPending {
		eventType = EventConfirmationRequested
	}
	s.runtime.publishDomainEvent(Event{
		Type: eventType, Source: source, DocumentID: info.DocumentID,
		RequestID: requestID, CommandID: info.CapabilityID,
		Revision: info.ExpectedRevision, Name: info.CapabilityName,
		View: ViewContext{WindowID: info.WindowID, DocumentID: info.DocumentID, Revision: info.ExpectedRevision}, Data: data,
	})
}

func (s *confirmationService) expireAndTrim(now time.Time) {
	s.mu.Lock()
	expired := s.expireAndTrimLocked(now)
	s.mu.Unlock()
	for _, event := range expired {
		s.publish(event.info, OperatorSystem, event.requestID)
	}
}

func (s *confirmationService) expireAndTrimLocked(now time.Time) []expiredConfirmationEvent {
	var expired []expiredConfirmationEvent
	for _, record := range s.records {
		if record.info.Status == ConfirmationPending && !now.Before(record.info.ExpiresAt) {
			record.info.Status = ConfirmationExpired
			expired = append(expired, expiredConfirmationEvent{
				info: cloneConfirmationInfo(record.info), requestID: record.request.RequestID,
			})
		}
	}
	kept := s.order[:0]
	for _, id := range s.order {
		record := s.records[id]
		if record == nil || now.Sub(record.info.CreatedAt) >= confirmationRecordTTL && !isConfirmationActive(record.info.Status) {
			s.removeRecordLocked(id)
			continue
		}
		kept = append(kept, id)
	}
	s.order = kept
	return expired
}

func (s *confirmationService) trimRecordLimitLocked() {
	if len(s.records) < maxConfirmationRecords {
		return
	}
	kept := s.order[:0]
	for _, id := range s.order {
		record := s.records[id]
		if len(s.records) >= maxConfirmationRecords && record != nil && !isConfirmationActive(record.info.Status) {
			s.removeRecordLocked(id)
			continue
		}
		kept = append(kept, id)
	}
	s.order = kept
}

func (s *confirmationService) removeRecordLocked(id string) {
	delete(s.records, id)
	for key, value := range s.byKey {
		if value == id {
			delete(s.byKey, key)
		}
	}
	for key, value := range s.byCall {
		if value == id {
			delete(s.byCall, key)
		}
	}
}

func isConfirmationActive(status ConfirmationStatus) bool {
	return status == ConfirmationPending || status == ConfirmationRunning
}

func confirmationPayloadHash(request CapabilityRequest) (string, error) {
	request.RequestID = ""
	payload, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:]), nil
}

func newConfirmationID() (string, error) {
	var bytes [24]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func confirmationDocumentLabel(ctx context.Context, runtime *Runtime, documentID string) string {
	if runtime == nil || documentID == "" {
		return ""
	}
	info, err := runtime.GetDocument(ctx, documentID)
	if err != nil {
		return ""
	}
	if info.SourcePath != "" {
		return filepath.Base(info.SourcePath)
	}
	return fmt.Sprintf("未保存的 %s 文档", info.Format)
}

func confirmationImpactSummary(runtime *Runtime, descriptor CapabilityDescriptor, arguments json.RawMessage, view ViewContext) string {
	parts := []string{strings.TrimSpace(descriptor.Name), strings.TrimSpace(descriptor.Description)}
	if selection := view.Selection; selection != nil {
		selected := selection.TargetType
		if selection.TargetID != "" {
			selected += ":" + selection.TargetID
		}
		if selection.Sheet != "" {
			selected += " 工作表=" + selection.Sheet
		}
		if selection.Range != nil {
			if encoded, err := json.Marshal(selection.Range); err == nil {
				selected += " 区域=" + string(encoded)
			}
		}
		parts = append(parts, "当前选择="+selected)
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(arguments, &values) == nil {
		if descriptor.ID == "document.plan.apply" && runtime != nil && runtime.documentPlans != nil {
			var planInput struct {
				PlanID string `json:"plan_id"`
			}
			if json.Unmarshal(values["plan_id"], &planInput) == nil {
				if plan, err := runtime.documentPlans.get(planInput.PlanID); err == nil {
					capabilityNames := make([]string, 0, len(plan.Commands))
					for _, command := range plan.Commands {
						capabilityNames = append(capabilityNames, command.Name)
					}
					parts = append(parts, fmt.Sprintf("计划含 %d 项修改：%s", len(plan.Commands), strings.Join(capabilityNames, "、")))
				}
			}
		}
		for _, key := range []string{"target_id", "image_id", "run_id", "path", "target_path", "sheet", "cell", "range", "mode", "orientation"} {
			raw := values[key]
			if len(raw) == 0 {
				continue
			}
			var value any
			if json.Unmarshal(raw, &value) != nil {
				continue
			}
			encoded, err := json.Marshal(value)
			if err == nil {
				parts = append(parts, key+"="+string(encoded))
			}
		}
	}
	summary := strings.Join(parts, "；")
	runes := []rune(summary)
	if len(runes) > 400 {
		summary = string(runes[:400]) + "…"
	}
	return summary
}

func rejectAgentConfirmationFlag(request CapabilityRequest) error {
	if request.Operator.Kind == OperatorHuman {
		return nil
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(request.Arguments, &values) != nil || values == nil {
		return nil
	}
	if raw := values["confirmed"]; len(raw) > 0 {
		var confirmed bool
		if json.Unmarshal(raw, &confirmed) == nil && confirmed {
			return ErrInvalidCommandArguments
		}
	}
	return nil
}

func cloneConfirmationInfo(info ConfirmationInfo) ConfirmationInfo {
	if info.Result != nil {
		result := *info.Result
		result.Data = append(json.RawMessage(nil), result.Data...)
		info.Result = &result
	}
	return info
}

func cloneCapabilityRequest(request CapabilityRequest) CapabilityRequest {
	request.Arguments = append(json.RawMessage(nil), request.Arguments...)
	request.View = cloneViewContext(request.View)
	return request
}

func registerConfirmationCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "confirmation.get", Name: "查询操作确认状态",
			Description: "根据一次性确认 ID 查询高影响操作是否待批准、已拒绝或已完成；此工具不能批准操作。",
			Category:    CapabilityQuery, Risk: RiskLow,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"confirmation_id":{"type":"string","minLength":1}},"required":["confirmation_id"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"confirmation_id":{"type":"string"},"status":{"type":"string"},"capability_id":{"type":"string"},"capability_name":{"type":"string"},"document_id":{"type":"string"},"document_label":{"type":"string"},"expected_revision":{"type":"integer"},"impact_summary":{"type":"string"},"created_at":{"type":"string"},"expires_at":{"type":"string"},"resolved_by":{"type":"string"},"error":{"type":"string"},"result":{"type":"object"}},"required":["confirmation_id","status","capability_id","capability_name","impact_summary","created_at","expires_at"]}`),
		},
		Handler: func(ctx context.Context, call CapabilityContext, arguments json.RawMessage) (CapabilityResult, error) {
			var input struct {
				ID string `json:"confirmation_id"`
			}
			if err := json.Unmarshal(arguments, &input); err != nil {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			info, err := call.Runtime.GetConfirmation(ctx, input.ID)
			if err != nil {
				return CapabilityResult{}, err
			}
			data, err := json.Marshal(info)
			return CapabilityResult{Data: data}, err
		},
	})
}
