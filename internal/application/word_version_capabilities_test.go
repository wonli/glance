package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"glance/internal/workspace"
)

func TestWordVersionListCapabilityReturnsBoundedMetadata(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{WorkspaceDBPath: filepath.Join(t.TempDir(), "workspace.sqlite")})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	path := filepath.Join(t.TempDir(), "report.docx")
	doc, err = runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	pathKey := canonicalPath(path)
	createdAt := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	for index, value := range []struct {
		source workspace.WordVersionSource
		bytes  string
	}{
		{source: workspace.WordVersionOpened, bytes: "baseline"},
		{source: workspace.WordVersionManualSave, bytes: "latest"},
	} {
		_, added, err := runtime.WorkspaceStore().AddWordVersion(workspace.WordVersionInput{
			PathKey: pathKey, DisplayPath: path, Source: value.source,
			CapturedAt: createdAt.Add(time.Duration(index) * time.Hour), Snapshot: []byte(value.bytes),
		})
		if err != nil || !added {
			t.Fatalf("AddWordVersion(%s) = added %v, err %v", value.source, added, err)
		}
	}

	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "list-word-versions", DocumentID: doc.DocumentID, CapabilityID: "word.version.list",
		Operator: Operator{ID: "agent", Kind: OperatorExternalAgent}, Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("InvokeCapability(word.version.list): %v", err)
	}
	var data struct {
		Versions []struct {
			ID        int64     `json:"id"`
			Source    string    `json:"source"`
			CreatedAt time.Time `json:"created_at"`
			SizeBytes int64     `json:"size_bytes"`
			SHA256    string    `json:"sha256"`
		} `json:"versions"`
		HasMore bool `json:"has_more"`
	}
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("decode version list: %v", err)
	}
	if len(data.Versions) != 2 || data.Versions[0].Source != string(workspace.WordVersionManualSave) || data.Versions[1].Source != string(workspace.WordVersionOpened) || data.HasMore {
		t.Fatalf("version list = %+v", data)
	}
	if data.Versions[0].SizeBytes != int64(len("latest")) || data.Versions[0].SHA256 == "" || result.Revision != doc.Revision {
		t.Fatalf("version metadata or live revision = %+v, revision=%d want %d", data.Versions[0], result.Revision, doc.Revision)
	}
	encoded, _ := json.Marshal(data)
	if string(encoded) == "" {
		t.Fatal("version list did not encode")
	}
}

func TestWordVersionListRejectsUnsavedNewDocument(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{WorkspaceDBPath: filepath.Join(t.TempDir(), "workspace.sqlite")})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "list-unsaved-word-versions", DocumentID: doc.DocumentID, CapabilityID: "word.version.list",
		Operator: Operator{ID: "agent", Kind: OperatorExternalAgent}, Arguments: json.RawMessage(`{}`),
	})
	if err != ErrNoSavePath {
		t.Fatalf("version list without saved path error = %v; want ErrNoSavePath", err)
	}
}
