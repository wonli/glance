package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"glance/internal/document"
)

func TestSpreadsheetProofreadUsesExplicitRangeAndOptionalDictionary(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	for _, cell := range []struct{ address, value string }{
		{"A1", "Helo  world"},
		{"A2", "safe\u200bvalue"},
		{"B1", "未纳入的区域"},
	} {
		if _, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: "proofread-seed-" + cell.address, DocumentID: doc.DocumentID,
			CommandID: "spreadsheet.cell.set", ExpectedRevision: doc.Revision,
			Operator:  Operator{ID: "test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, map[string]any{"sheet": "Sheet1", "cell": cell.address, "value": cell.value}),
		}); err != nil {
			t.Fatalf("seed %s: %v", cell.address, err)
		}
		doc, err = runtime.GetDocument(ctx, doc.DocumentID)
		if err != nil {
			t.Fatalf("GetDocument after seed: %v", err)
		}
	}
	before := doc
	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "proofread-explicit-range", DocumentID: doc.DocumentID,
		CapabilityID: "spreadsheet.text.proofread", ExpectedRevision: doc.Revision,
		Operator: Operator{ID: "test", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{
			"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 1},
			"dictionary": []string{"hello", "world", "safe", "value"},
		}),
	})
	if err != nil {
		t.Fatalf("InvokeCapability: %v", err)
	}
	var report struct {
		Sheet             string `json:"sheet"`
		CellsScanned      int    `json:"cells_scanned"`
		DictionaryChecked bool   `json:"dictionary_checked"`
		Issues            []struct {
			Cell     string `json:"cell"`
			Category string `json:"category"`
			Token    string `json:"token"`
		} `json:"issues"`
		Truncated bool `json:"issues_truncated"`
	}
	if err := json.Unmarshal(result.Data, &report); err != nil {
		t.Fatalf("decode proofread result %s: %v", result.Data, err)
	}
	if report.Sheet != "Sheet1" || report.CellsScanned != 2 || !report.DictionaryChecked || report.Truncated {
		t.Fatalf("unexpected proofread result: %+v", report)
	}
	foundUnknown, foundWhitespace, foundInvisible := false, false, false
	for _, issue := range report.Issues {
		switch {
		case issue.Cell == "A1" && issue.Category == "unknown_dictionary_word" && issue.Token == "Helo":
			foundUnknown = true
		case issue.Cell == "A1" && issue.Category == "repeated_whitespace":
			foundWhitespace = true
		case issue.Cell == "A2" && issue.Category == "invisible_format_character":
			foundInvisible = true
		}
	}
	if !foundUnknown || !foundWhitespace || !foundInvisible {
		t.Fatalf("proofread issues = %+v; want dictionary, whitespace, and invisible-character findings", report.Issues)
	}
	after, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument after proofread: %v", err)
	}
	if result.Revision != before.Revision || after.Revision != before.Revision || after.Dirty != before.Dirty {
		t.Fatalf("proofread changed document state: before=%+v result_revision=%d after=%+v", before, result.Revision, after)
	}
}

func TestSpreadsheetProofreadWithoutDictionaryDoesNotClaimSpellingCheck(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "proofread-seed-no-dictionary", DocumentID: doc.DocumentID, CommandID: "spreadsheet.cell.set",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "test", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"sheet": "Sheet1", "cell": "A1", "value": "definately"}),
	}); err != nil {
		t.Fatalf("seed cell: %v", err)
	}
	doc, err = runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "proofread-no-dictionary", DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.text.proofread",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "test", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}}),
	})
	if err != nil {
		t.Fatalf("InvokeCapability: %v", err)
	}
	var report struct {
		DictionaryChecked bool `json:"dictionary_checked"`
		Issues            []struct {
			Category string `json:"category"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(result.Data, &report); err != nil {
		t.Fatalf("decode proofread result %s: %v", result.Data, err)
	}
	if report.DictionaryChecked || len(report.Issues) != 0 {
		t.Fatalf("without a dictionary, proofread must not guess spelling: %+v", report)
	}
}

func TestSpreadsheetProofreadRejectsOutOfBoundsAndMalformedDictionary(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	for name, arguments := range map[string]any{
		"too many cells":          map[string]any{"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 101, EndColumn: 100}},
		"invalid dictionary term": map[string]any{"sheet": "Sheet1", "bounds": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, "dictionary": []string{"two words"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := runtime.InvokeCapability(ctx, CapabilityRequest{
				RequestID: "proofread-invalid-" + name, DocumentID: doc.DocumentID, CapabilityID: "spreadsheet.text.proofread",
				ExpectedRevision: doc.Revision, Operator: Operator{ID: "test", Kind: OperatorExternalAgent},
				Arguments: mustJSON(t, arguments),
			})
			if !errors.Is(err, ErrInvalidCommandArguments) {
				t.Fatalf("InvokeCapability error = %v; want ErrInvalidCommandArguments", err)
			}
		})
	}
}
