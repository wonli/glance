package application

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestWordEquationCapabilityReplacesRunsAndInsertsTextBoxes(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	anchor, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "equation-anchor", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "E=mc"}),
	})
	if err != nil {
		t.Fatalf("append equation anchor: %v", err)
	}
	var target struct {
		ParagraphID string `json:"paragraph_id"`
		RunID       string `json:"run_id"`
	}
	if err := json.Unmarshal(anchor.Data, &target); err != nil || target.ParagraphID == "" || target.RunID == "" {
		t.Fatalf("decode equation anchor: %v, data=%s", err, anchor.Data)
	}

	runExpression := strings.Repeat("等", 6000)
	replaced, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "equation-replace-run", DocumentID: doc.DocumentID, CommandID: "word.equation.insert",
		ExpectedRevision: anchor.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"target": "run", "run_id": target.RunID, "expression": runExpression}),
	})
	if err != nil {
		t.Fatalf("insert equation in text run: %v", err)
	}
	textboxExpression := strings.Repeat("式", 6000)
	inserted, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "equation-insert-textbox", DocumentID: doc.DocumentID, CommandID: "word.equation.insert",
		ExpectedRevision: replaced.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"target": "textbox", "after_paragraph_id": target.ParagraphID, "expression": textboxExpression}),
	})
	if err != nil {
		t.Fatalf("insert equation text box: %v", err)
	}
	if inserted.Revision != replaced.Revision+1 {
		t.Fatalf("equation insertion revision = %d, want %d", inserted.Revision, replaced.Revision+1)
	}
	inspection, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 20})
	if err != nil {
		t.Fatalf("inspect equations: %v", err)
	}
	if !strings.Contains(string(inspection.Content), runExpression) || !strings.Contains(string(inspection.Content), textboxExpression) {
		t.Fatalf("equation results missing from Agent document inspection: %s", inspection.Content)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "equation-insert-too-long", DocumentID: doc.DocumentID, CommandID: "word.equation.insert",
		ExpectedRevision: inserted.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"target": "textbox", "expression": strings.Repeat("超", 16001)}),
	}); err == nil {
		t.Fatal("word.equation.insert accepted more than 16000 Unicode characters")
	}
	if state := mustGetDocumentInfo(t, runtime, doc.DocumentID); state.Revision != inserted.Revision || !state.Dirty {
		t.Fatalf("rejected long equation changed document state: %+v", state)
	}
	for index, arguments := range []map[string]any{
		{"target": "run", "run_id": "missing-run", "expression": "x²"},
		{"target": "textbox", "after_paragraph_id": "missing-paragraph", "expression": "x²"},
	} {
		if _, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: fmt.Sprintf("equation-missing-target-%d", index), DocumentID: doc.DocumentID,
			CommandID: "word.equation.insert", ExpectedRevision: inserted.Revision,
			Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent}, Arguments: mustJSON(t, arguments),
		}); err == nil {
			t.Errorf("equation insertion accepted invalid stable target: %+v", arguments)
		}
	}
	if state := mustGetDocumentInfo(t, runtime, doc.DocumentID); state.Revision != inserted.Revision || !state.Dirty {
		t.Fatalf("invalid equation targets changed document state: %+v", state)
	}
	_, err = runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "equation-invalid-target", DocumentID: doc.DocumentID, CommandID: "word.equation.insert",
		ExpectedRevision: inserted.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"target": "run", "run_id": target.RunID, "after_paragraph_id": target.ParagraphID, "expression": "invalid"}),
	})
	if err == nil {
		t.Fatal("equation insertion accepted conflicting run and paragraph targets")
	}
	if state := mustGetDocumentInfo(t, runtime, doc.DocumentID); state.Revision != inserted.Revision || !state.Dirty {
		t.Fatalf("conflicting equation target changed document state: %+v", state)
	}
	path := filepath.Join(t.TempDir(), "equations.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil || saved.Dirty {
		t.Fatalf("save equation DOCX = %+v, error=%v; want clean document", saved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close equation DOCX: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen equation DOCX: %v", err)
	}
	reopenedSnapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: reopened.DocumentID, Limit: 20})
	if err != nil || !strings.Contains(string(reopenedSnapshot.Content), runExpression) || !strings.Contains(string(reopenedSnapshot.Content), textboxExpression) {
		t.Fatalf("equation edits did not survive DOCX save/reopen: error=%v content=%s", err, reopenedSnapshot.Content)
	}
}
