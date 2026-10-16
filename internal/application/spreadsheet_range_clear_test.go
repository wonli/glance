package application

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSpreadsheetRangeClearCommandIsAtomicAndIdempotent(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(ctx)
	doc, err := r.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(id string, revision uint64, command, args string) CommandResult {
		t.Helper()
		got, err := r.Commands().Execute(ctx, CommandRequest{RequestID: id, DocumentID: doc.DocumentID, ExpectedRevision: revision, CommandID: command, Operator: Operator{ID: "range-clear-test", Kind: OperatorExternalAgent}, Arguments: json.RawMessage(args)})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	set := invoke("set", doc.Revision, "spreadsheet.cells.set", `{"sheet":"Sheet1","edits":[{"row":1,"column":2,"value":"remove"},{"row":2,"column":3,"value":"keep"}]}`)
	bounds := `{"sheet":"Sheet1","bounds":{"start_row":1,"end_row":1048576,"start_column":2,"end_column":2}}`
	cleared := invoke("clear", set.Revision, "spreadsheet.range.clear", bounds)
	if cleared.Revision != set.Revision+1 {
		t.Fatal("clear revision did not advance")
	}
	repeated := invoke("repeat", cleared.Revision, "spreadsheet.range.clear", bounds)
	if repeated.Revision != cleared.Revision {
		t.Fatal("no-op clear advanced revision")
	}
	family := `{"sheet":"Sheet1","bounds":{"start_row":1,"end_row":1048576,"start_column":2,"end_column":2},"patch":{"font_family":"Synthetic Font"}}`
	invoke("format", repeated.Revision, "spreadsheet.range.format.set", family)
}
