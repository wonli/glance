package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type agentCapabilityManifest struct {
	RibbonActions []struct {
		ActionID   string `json:"action_id"`
		Capability string `json:"capability_id"`
		Status     string `json:"agent_status"`
	} `json:"ribbon_actions"`
	AgentOnlyCapabilities []struct {
		ID string `json:"capability_id"`
	} `json:"agent_only_capabilities"`
}

func TestEveryManifestCapabilityIsRegisteredAndCallable(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	manifestPath := filepath.Join(filepath.Dir(sourcePath), "..", "..", "docs", "compatibility", "agent-command-coverage.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read capability manifest: %v", err)
	}
	var manifest agentCapabilityManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode capability manifest: %v", err)
	}
	if len(manifest.RibbonActions) == 0 || len(manifest.AgentOnlyCapabilities) == 0 {
		t.Fatalf("manifest coverage is empty: ribbon=%d agent-only=%d", len(manifest.RibbonActions), len(manifest.AgentOnlyCapabilities))
	}

	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	server, err := New(runtime)
	if err != nil {
		t.Fatalf("New MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })
	listed, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list MCP tools: %v", err)
	}
	tools := make(map[string]*mcp.Tool, len(listed.Tools))
	for _, tool := range listed.Tools {
		tools[tool.Name] = tool
	}
	registered := make(map[string]application.CapabilityDescriptor)
	for _, descriptor := range runtime.Catalog().Descriptors() {
		registered[descriptor.ID] = descriptor
	}

	capabilityIDs := make(map[string]string)
	for _, action := range manifest.RibbonActions {
		if action.ActionID == "" || action.Capability == "" {
			t.Errorf("Ribbon manifest action is missing its action or capability ID: %+v", action)
			continue
		}
		if action.Status != "implemented" {
			t.Errorf("Ribbon action %q is %q; Agent must cover every Ribbon action", action.ActionID, action.Status)
		}
		capabilityIDs[action.Capability] = "Ribbon action " + action.ActionID
	}
	for _, capability := range manifest.AgentOnlyCapabilities {
		if capability.ID == "" {
			t.Error("Agent-only manifest entry is missing its capability ID")
			continue
		}
		if _, alsoRibbon := capabilityIDs[capability.ID]; alsoRibbon {
			t.Errorf("Agent-only capability %q also appears as a Ribbon capability", capability.ID)
		}
		capabilityIDs[capability.ID] = "Agent-only capability"
	}

	for capabilityID, source := range capabilityIDs {
		descriptor, ok := registered[capabilityID]
		if !ok {
			t.Errorf("%s references unregistered capability %q", source, capabilityID)
			continue
		}
		tool, ok := tools[capabilityID]
		if !ok {
			t.Errorf("registered capability %q is not exposed as an MCP tool", capabilityID)
			continue
		}
		if tool.Description == "" || tool.InputSchema == nil || tool.OutputSchema == nil || descriptor.Description == "" {
			t.Errorf("MCP capability %q is missing its operation instructions or schemas", capabilityID)
			continue
		}

		// Use a missing document for document-scoped capabilities so the call
		// reaches the registered handler but cannot mutate a real document.
		arguments := map[string]any{}
		if descriptor.RequiresDocument {
			arguments["document_id"] = "manifest-conformance-missing-document"
			if descriptor.RequiresRevision {
				arguments["expected_revision"] = 1
			}
			if descriptor.RequiresView {
				arguments["view"] = application.ViewContext{
					WindowID: 1, DocumentID: "manifest-conformance-missing-document", Revision: 1,
				}
			}
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: capabilityID, Arguments: arguments})
		if err != nil {
			t.Errorf("MCP tool %q could not be dispatched: %v", capabilityID, err)
			continue
		}
		if result == nil || len(result.Content) == 0 {
			t.Errorf("MCP tool %q returned no structured response", capabilityID)
		}
	}
}
