package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/localaccess"
)

type localAccessTestPrompt struct {
	folder   string
	selects  int
	confirms int
}

func (p *localAccessTestPrompt) SelectDirectory(context.Context, localaccess.SelectionRequest) (string, error) {
	p.selects++
	return p.folder, nil
}

func (p *localAccessTestPrompt) ConfirmDirectory(context.Context, localaccess.ConsentRequest) (bool, error) {
	p.confirms++
	return true, nil
}

func TestLocalAccessCapabilitiesRegisteredWithBoundedSchemas(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	descriptors := make(map[string]CapabilityDescriptor)
	for _, descriptor := range runtime.Catalog().Descriptors() {
		if len(descriptor.ID) >= len("local.") && descriptor.ID[:len("local.")] == "local." {
			descriptors[descriptor.ID] = descriptor
		}
	}
	want := map[string]CapabilityCategory{
		"local.request_directory_access": CapabilityWorkflow,
		"local.list_directory":           CapabilityQuery,
		"local.read_text_file":           CapabilityQuery,
		"local.list_git_commits":         CapabilityQuery,
	}
	if len(descriptors) != len(want) {
		t.Fatalf("registered local access capability count=%d descriptors=%v", len(descriptors), descriptors)
	}
	for id, category := range want {
		descriptor, found := descriptors[id]
		if !found || descriptor.Category != category || descriptor.Risk != RiskLow || descriptor.Description == "" {
			t.Fatalf("capability %q = %#v; want category=%q risk=low and description", id, descriptor, category)
		}
	}
	var requestSchema struct {
		Properties map[string]struct {
			MaxLength *int `json:"maxLength"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(descriptors["local.request_directory_access"].InputSchema, &requestSchema); err != nil {
		t.Fatal(err)
	}
	if requestSchema.Properties["purpose"].MaxLength == nil || *requestSchema.Properties["purpose"].MaxLength != 240 || len(requestSchema.Required) != 1 || requestSchema.Required[0] != "purpose" {
		t.Fatalf("directory authorization input schema = %s; want required purpose <= 240 runes", descriptors["local.request_directory_access"].InputSchema)
	}
	for _, id := range []string{"local.list_directory", "local.read_text_file"} {
		var schema struct {
			Properties map[string]struct {
				Pattern   *string `json:"pattern"`
				MaxLength *int    `json:"maxLength"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(descriptors[id].InputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		path := schema.Properties["relative_path"]
		if path.Pattern == nil || path.MaxLength == nil || *path.MaxLength > 4096 {
			t.Fatalf("%s path schema = %s; want a bounded relative path pattern", id, descriptors[id].InputSchema)
		}
	}
	var gitSchema struct {
		Properties map[string]struct {
			Maximum *int   `json:"maximum"`
			Format  string `json:"format"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(descriptors["local.list_git_commits"].InputSchema, &gitSchema); err != nil {
		t.Fatal(err)
	}
	if gitSchema.Properties["limit"].Maximum == nil || *gitSchema.Properties["limit"].Maximum != 200 || gitSchema.Properties["since"].Format != "date-time" || gitSchema.Properties["until"].Format != "date-time" {
		t.Fatalf("Git query schema = %s; want RFC3339 dates and limit <= 200", descriptors["local.list_git_commits"].InputSchema)
	}
}

func TestLocalAccessCapabilitiesRequireBuiltInTurnAndWindow(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("private text"), 0o600); err != nil {
		t.Fatal(err)
	}
	prompt := &localAccessTestPrompt{folder: root}
	broker := localaccess.NewBroker(prompt)
	runtime, err := NewRuntime(ctx, RuntimeOptions{LocalAccessBroker: broker})
	if err != nil {
		t.Fatalf("NewRuntime with local access broker: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	request := func(operator Operator, turnID string, windowID uint64) error {
		_, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "request-local-directory", CapabilityID: "local.request_directory_access",
			Operator: operator, AgentTurnID: turnID, AgentWindowID: windowID,
			Arguments: json.RawMessage(`{"purpose":"summarize this folder"}`),
		})
		return err
	}
	if err := request(Operator{ID: "external", Kind: OperatorExternalAgent}, "forged", 17); !errors.Is(err, localaccess.ErrInteractiveAuthorizationUnavailable) {
		t.Fatalf("external operator error=%v; want interactive authorization unavailable", err)
	}
	if err := request(Operator{ID: "builtin", Kind: OperatorBuiltInAgent}, "", 17); !errors.Is(err, localaccess.ErrInteractiveAuthorizationUnavailable) {
		t.Fatalf("missing turn ID error=%v; want interactive authorization unavailable", err)
	}
	if err := request(Operator{ID: "builtin", Kind: OperatorBuiltInAgent}, "turn", 0); !errors.Is(err, localaccess.ErrInteractiveAuthorizationUnavailable) {
		t.Fatalf("missing window ID error=%v; want interactive authorization unavailable", err)
	}
	if prompt.selects != 0 || prompt.confirms != 0 {
		t.Fatalf("unauthorized requests reached Prompt: selections=%d confirmations=%d", prompt.selects, prompt.confirms)
	}
	noBroker, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime without broker: %v", err)
	}
	t.Cleanup(func() { _ = noBroker.Close(ctx) })
	_, err = noBroker.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "request-no-broker", CapabilityID: "local.request_directory_access",
		Operator: Operator{ID: "builtin", Kind: OperatorBuiltInAgent}, AgentTurnID: "turn", AgentWindowID: 17,
		Arguments: json.RawMessage(`{"purpose":"summarize this folder"}`),
	})
	if !errors.Is(err, localaccess.ErrInteractiveAuthorizationUnavailable) {
		t.Fatalf("missing broker error=%v; want interactive authorization unavailable", err)
	}
}

func TestLocalAccessCapabilitiesRequestReadAndReleaseScope(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("private project note"), 0o600); err != nil {
		t.Fatal(err)
	}
	prompt := &localAccessTestPrompt{folder: root}
	broker := localaccess.NewBroker(prompt)
	runtime, err := NewRuntime(ctx, RuntimeOptions{LocalAccessBroker: broker})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	operator := Operator{ID: "builtin", Kind: OperatorBuiltInAgent}
	request, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "request-folder", CapabilityID: "local.request_directory_access", Operator: operator,
		AgentTurnID: "turn-read", AgentWindowID: 32, UserRequestExcerpt: "summarize these notes",
		Arguments: json.RawMessage(`{"purpose":"summarize selected work files"}`),
	})
	if err != nil {
		t.Fatalf("request directory capability: %v", err)
	}
	var scope struct {
		ID         string   `json:"scope_id"`
		FolderName string   `json:"folder_name"`
		Operations []string `json:"operations"`
	}
	if err := json.Unmarshal(request.Data, &scope); err != nil {
		t.Fatal(err)
	}
	if scope.ID == "" || scope.FolderName != filepath.Base(root) || strings.Contains(string(request.Data), root) {
		t.Fatalf("directory capability output leaked or omitted scope data: %s", request.Data)
	}
	read, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "read-folder-file", CapabilityID: "local.read_text_file", Operator: operator,
		AgentTurnID: "turn-read", AgentWindowID: 32,
		Arguments: mustJSON(t, struct {
			ScopeID      string `json:"scope_id"`
			RelativePath string `json:"relative_path"`
		}{scope.ID, "notes.txt"}),
	})
	if err != nil {
		t.Fatalf("read local file capability: %v", err)
	}
	if !strings.Contains(string(read.Data), "private project note") || strings.Contains(string(read.Data), root) {
		t.Fatalf("local file result = %s; want content without absolute root", read.Data)
	}
	runtime.ReleaseAgentTurn("turn-read")
	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "read-after-release", CapabilityID: "local.read_text_file", Operator: operator,
		AgentTurnID: "turn-read", AgentWindowID: 32,
		Arguments: mustJSON(t, struct {
			ScopeID      string `json:"scope_id"`
			RelativePath string `json:"relative_path"`
		}{scope.ID, "notes.txt"}),
	})
	if !errors.Is(err, localaccess.ErrTurnExpired) {
		t.Fatalf("reading after turn release error=%v; want ErrTurnExpired", err)
	}
}
