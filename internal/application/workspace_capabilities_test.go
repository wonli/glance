package application

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"glance/internal/workspace"
)

func TestWorkspaceRecentListCapabilityFiltersAndBoundsResults(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{WorkspaceDBPath: filepath.Join(t.TempDir(), "workspace.sqlite")})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	openedAt := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	if err := runtime.WorkspaceStore().SaveRecents([]workspace.Recent{
		{Path: "/docs/上周周报.docx", Kind: "docx", OpenedAt: openedAt.Add(-2 * time.Hour)},
		{Path: "/docs/工资表.xlsx", Kind: "xlsx", OpenedAt: openedAt.Add(-time.Hour)},
		{Path: "/docs/本周周报.docx", Kind: "docx", OpenedAt: openedAt},
	}); err != nil {
		t.Fatalf("SaveRecents: %v", err)
	}

	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "recent-word-reports", CapabilityID: "workspace.recent.list",
		Operator:  Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"format": "docx", "query": "周报", "limit": 1}),
	})
	if err != nil {
		t.Fatalf("InvokeCapability(workspace.recent.list): %v", err)
	}
	var recent struct {
		Items []struct {
			Path     string    `json:"path"`
			Format   string    `json:"format"`
			OpenedAt time.Time `json:"opened_at"`
		} `json:"items"`
		HasMore    bool `json:"has_more"`
		NextOffset int  `json:"next_offset"`
	}
	if err := json.Unmarshal(result.Data, &recent); err != nil {
		t.Fatalf("decode recent list: %v", err)
	}
	if len(recent.Items) != 1 || recent.Items[0].Path != "/docs/本周周报.docx" || recent.Items[0].Format != "docx" || !recent.Items[0].OpenedAt.Equal(openedAt) || !recent.HasMore || recent.NextOffset != 1 {
		t.Fatalf("recent results = %+v", recent)
	}
	if result.DocumentID != "" {
		t.Fatalf("workspace query unexpectedly bound to document %q", result.DocumentID)
	}

	secondPage, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "recent-word-reports-page-2", CapabilityID: "workspace.recent.list",
		Operator:  Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"format": "docx", "query": "周报", "limit": 1, "offset": 1}),
	})
	if err != nil {
		t.Fatalf("InvokeCapability(workspace.recent.list offset=1): %v", err)
	}
	var page2 recentDocumentListResult
	if err := json.Unmarshal(secondPage.Data, &page2); err != nil {
		t.Fatalf("decode second recent page: %v", err)
	}
	if len(page2.Items) != 1 || page2.Items[0].Path != "/docs/上周周报.docx" || page2.HasMore {
		t.Fatalf("second recent page = %+v", page2)
	}

	descriptor, ok := runtime.Catalog().Descriptor("workspace.recent.list")
	if !ok || descriptor.Description == "" || descriptor.Category != CapabilityQuery {
		t.Fatalf("workspace recent capability descriptor = %+v, found=%v", descriptor, ok)
	}
}

func TestWorkspaceRecentListReportsMissingWorkspace(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "recent-no-workspace", CapabilityID: "workspace.recent.list",
		Operator: Operator{ID: "agent", Kind: OperatorExternalAgent}, Arguments: json.RawMessage(`{}`),
	})
	if err != ErrWorkspaceUnavailable {
		t.Fatalf("recent list without workspace error = %v; want ErrWorkspaceUnavailable", err)
	}
}

func TestWorkspaceRecentListRejectsUnboundedLimit(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{WorkspaceDBPath: filepath.Join(t.TempDir(), "workspace.sqlite")})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "recent-too-many", CapabilityID: "workspace.recent.list",
		Operator:  Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"limit": 101}),
	})
	if !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("unbounded recent list error = %v; want ErrInvalidCommandArguments", err)
	}
}
