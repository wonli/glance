package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"glance/internal/localaccess"
)

var ErrLocalAccessResultTooLarge = errors.New("local access result exceeds the Agent tool result limit")

const maxLocalAccessCapabilityResultBytes = (1 << 20) - 4096

func registerLocalAccessCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "local.request_directory_access", Name: "Request local directory access",
				Description: "Ask the user to choose a local directory and approve read-only access for this request. No access is granted until the user approves.",
				Category:    CapabilityWorkflow, Risk: RiskLow,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"purpose":{"type":"string","minLength":1,"maxLength":240,"description":"Why local files are needed for the current request."}},"required":["purpose"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"scope_id":{"type":"string","minLength":1,"maxLength":64},"folder_name":{"type":"string"},"operations":{"type":"array","items":{"type":"string"}}},"required":["scope_id","folder_name","operations"],"additionalProperties":false}`),
			},
			Handler: localAccessRequestDirectory,
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "local.list_directory", Name: "List local directory",
				Description: "List up to 500 direct children inside a directory approved for this request. Use relative paths; '.' is the approved directory root.",
				Category:    CapabilityQuery, Risk: RiskLow,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"scope_id":{"type":"string","minLength":1,"maxLength":64},"relative_path":{"type":"string","minLength":1,"maxLength":4096,"pattern":"^(\\.|[^/\\\\].*)$"}},"required":["scope_id","relative_path"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"entries":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"relative_path":{"type":"string"},"is_directory":{"type":"boolean"},"size":{"type":"integer","minimum":0}},"required":["name","relative_path","is_directory","size"],"additionalProperties":false}}},"required":["entries"],"additionalProperties":false}`),
			},
			Handler: localAccessListDirectory,
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "local.read_text_file", Name: "Read local text file",
				Description: "Read one UTF-8 text file of at most 1 MiB from a directory approved for this request. Binary, credential, and out-of-scope files are rejected.",
				Category:    CapabilityQuery, Risk: RiskLow,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"scope_id":{"type":"string","minLength":1,"maxLength":64},"relative_path":{"type":"string","minLength":1,"maxLength":4096,"pattern":"^(\\.|[^/\\\\].*)$"}},"required":["scope_id","relative_path"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"relative_path":{"type":"string"},"content":{"type":"string"},"size":{"type":"integer","minimum":0}},"required":["relative_path","content","size"],"additionalProperties":false}`),
			},
			Handler: localAccessReadTextFile,
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "local.list_git_commits", Name: "List local Git commits",
				Description: "List up to 200 commit summaries and changed relative paths for a Git repository root approved for this request. Does not return diffs, commit bodies, or author email addresses.",
				Category:    CapabilityQuery, Risk: RiskLow,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"scope_id":{"type":"string","minLength":1,"maxLength":64},"since":{"type":"string","maxLength":40,"format":"date-time","description":"Optional ISO date or RFC3339 start date/time."},"until":{"type":"string","maxLength":40,"format":"date-time","description":"Optional ISO date or RFC3339 end date/time."},"limit":{"type":"integer","minimum":1,"maximum":200,"default":200}},"required":["scope_id"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"commits":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"date":{"type":"string","format":"date-time"},"author":{"type":"string"},"subject":{"type":"string"},"paths":{"type":"array","items":{"type":"string"}}},"required":["id","date","author","subject","paths"],"additionalProperties":false}},"has_more":{"type":"boolean"}},"required":["commits","has_more"],"additionalProperties":false}`),
			},
			Handler: localAccessListGitCommits,
		},
	}
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	return nil
}

func localAccessRequestDirectory(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
	broker, err := localAccessForContext(capability)
	if err != nil {
		return CapabilityResult{}, err
	}
	var input struct {
		Purpose string `json:"purpose"`
	}
	if err := decodeLocalAccessInput(raw, &input); err != nil {
		return CapabilityResult{}, err
	}
	if strings.TrimSpace(input.Purpose) == "" || utf8.RuneCountInString(input.Purpose) > 240 {
		return CapabilityResult{}, localaccess.ErrInvalidPurpose
	}
	scope, err := broker.RequestDirectory(ctx, capability.AgentTurnID, capability.AgentWindowID, input.Purpose, capability.UserRequestExcerpt)
	if err != nil {
		return CapabilityResult{}, err
	}
	return marshalLocalAccessResult(scope)
}

func localAccessListDirectory(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
	broker, err := localAccessForContext(capability)
	if err != nil {
		return CapabilityResult{}, err
	}
	var input struct {
		ScopeID      string `json:"scope_id"`
		RelativePath string `json:"relative_path"`
	}
	if err := decodeLocalAccessInput(raw, &input); err != nil {
		return CapabilityResult{}, err
	}
	entries, err := broker.ListDirectory(ctx, capability.AgentTurnID, input.ScopeID, input.RelativePath)
	if err != nil {
		return CapabilityResult{}, err
	}
	if entries == nil {
		entries = []localaccess.Entry{}
	}
	return marshalLocalAccessResult(struct {
		Entries []localaccess.Entry `json:"entries"`
	}{Entries: entries})
}

func localAccessReadTextFile(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
	broker, err := localAccessForContext(capability)
	if err != nil {
		return CapabilityResult{}, err
	}
	var input struct {
		ScopeID      string `json:"scope_id"`
		RelativePath string `json:"relative_path"`
	}
	if err := decodeLocalAccessInput(raw, &input); err != nil {
		return CapabilityResult{}, err
	}
	file, err := broker.ReadTextFile(ctx, capability.AgentTurnID, input.ScopeID, input.RelativePath)
	if err != nil {
		return CapabilityResult{}, err
	}
	return marshalLocalAccessResult(file)
}

func localAccessListGitCommits(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
	broker, err := localAccessForContext(capability)
	if err != nil {
		return CapabilityResult{}, err
	}
	var input struct {
		ScopeID string `json:"scope_id"`
		Since   string `json:"since"`
		Until   string `json:"until"`
		Limit   int    `json:"limit"`
	}
	if err := decodeLocalAccessInput(raw, &input); err != nil {
		return CapabilityResult{}, err
	}
	limit := input.Limit
	if limit == 0 {
		limit = 200
	}
	since, err := parseLocalAccessDate(input.Since, false)
	if err != nil {
		return CapabilityResult{}, err
	}
	until, err := parseLocalAccessDate(input.Until, true)
	if err != nil {
		return CapabilityResult{}, err
	}
	page, err := broker.ListGitCommits(ctx, capability.AgentTurnID, input.ScopeID, since, until, limit)
	if err != nil {
		return CapabilityResult{}, err
	}
	return marshalLocalAccessResult(page)
}

func localAccessForContext(capability CapabilityContext) (*localaccess.Broker, error) {
	if capability.Operator.Kind != OperatorBuiltInAgent || strings.TrimSpace(capability.AgentTurnID) == "" || capability.AgentWindowID == 0 || capability.Runtime == nil || capability.Runtime.localAccessBroker == nil {
		return nil, localaccess.ErrInteractiveAuthorizationUnavailable
	}
	return capability.Runtime.localAccessBroker, nil
}

func decodeLocalAccessInput(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrInvalidCommandArguments
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return ErrInvalidCommandArguments
	}
	return nil
}

func parseLocalAccessDate(value string, endOfDay bool) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		if endOfDay {
			return parsed.Add(24*time.Hour - time.Nanosecond), nil
		}
		return parsed, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, localaccess.ErrInvalidDateRange
	}
	return parsed, nil
}

func marshalLocalAccessResult(value any) (CapabilityResult, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return CapabilityResult{}, ErrInvalidCommandResult
	}
	if len(data) > maxLocalAccessCapabilityResultBytes {
		return CapabilityResult{}, ErrLocalAccessResultTooLarge
	}
	return CapabilityResult{Data: data}, nil
}
