package desktopui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"glance/internal/agent"
	"glance/internal/agent/provider"
	"glance/internal/agent/provider/deepseek"
)

type catalogDiagnosticTransport func(*http.Request) (*http.Response, error)

func (f catalogDiagnosticTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDeepSeekCompleteDesktopCatalogHasCompatibleWireNames(t *testing.T) {
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	ctx := context.Background()
	client, err := agent.ConnectMCP(ctx, runtime.applicationRuntime)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	listed, err := client.Session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if listed.NextCursor != "" {
		t.Fatal("test must include all catalog pages")
	}
	var tools []provider.ModelTool
	for _, tool := range listed.Tools {
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		tools = append(tools, provider.ModelTool{Type: "function", Function: provider.ModelToolFunction{Name: tool.Name, Description: tool.Description, Parameters: schema}})
	}
	t.Logf("complete local catalog: %d tools", len(tools))
	if len(tools) == 0 {
		t.Fatal("missing desktop catalog")
	}
	transport := catalogDiagnosticTransport(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Tools []provider.ModelTool `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		if len(body.Tools) != len(tools) {
			t.Fatal("adapter dropped desktop capabilities")
		}
		seen := make(map[string]bool)
		pattern := regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
		for _, tool := range body.Tools {
			name := tool.Function.Name
			if !pattern.MatchString(name) || seen[name] {
				t.Errorf("invalid or duplicate wire name %q", name)
			}
			seen[name] = true
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\ndata: [DONE]\n\n")), Request: r}, nil
	})
	model := deepseek.New(deepseek.Config{HTTPClient: &http.Client{Transport: transport}, Model: "test-model", APIKey: "synthetic-key"})
	if err := model.Stream(ctx, provider.ModelRequest{Tools: tools, Messages: []provider.ModelMessage{{Role: "user", Content: "synthetic probe"}}}, func(provider.ModelEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
}
