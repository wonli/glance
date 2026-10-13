package application

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type markdownEditArguments struct {
	Action      string `json:"action"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
	Destination string `json:"destination,omitempty"`
	Label       string `json:"label,omitempty"`
	Display     bool   `json:"display,omitempty"`
}

type markdownEditOptions struct {
	Destination string
	Label       string
	Display     bool
}

func TestMarkdownEditCapabilityUsesRuneRangesAndRoundTrips(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	source := "本周工资表已核对。\n待办：生成周报\n整理资料\n1 + 1\n"
	path := filepath.Join(t.TempDir(), "weekly-report.md")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write Markdown fixture: %v", err)
	}
	doc, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument(Markdown): %v", err)
	}
	documentID := doc.DocumentID
	revision := doc.Revision
	requestNumber := 0
	invoke := func(action, selected string, extra markdownEditOptions) (CapabilityInvocationResult, error) {
		t.Helper()
		content := readMarkdownCapabilityText(t, runtime, documentID)
		start, end, ok := markdownRuneRange(content, selected)
		if !ok {
			t.Fatalf("selected Markdown text %q missing from %q", selected, content)
		}
		args := markdownEditArguments{
			Action: action, Start: start, End: end,
			Destination: extra.Destination, Label: extra.Label, Display: extra.Display,
		}
		requestNumber++
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("markdown-edit-%d", requestNumber), DocumentID: documentID,
			CapabilityID: "markdown.text.edit", ExpectedRevision: revision,
			Operator:  Operator{ID: "markdown-agent", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, args),
		})
		if invokeErr == nil {
			revision = result.Revision
		}
		return result, invokeErr
	}

	for _, operation := range []struct {
		action   string
		selected string
		extra    markdownEditOptions
	}{
		{action: "bold", selected: "工资表"},
		{action: "italic", selected: "周报"},
		{action: "code", selected: "已核对"},
		{action: "link", selected: "周报", extra: markdownEditOptions{Destination: "https://example.test/周报?week=42"}},
		{action: "image", selected: "工资表", extra: markdownEditOptions{Destination: "file:///tmp/payroll.png", Label: "工资统计图"}},
		{action: "heading", selected: "本周"},
		{action: "todo", selected: "周报"},
		{action: "quote", selected: "待办："},
		{action: "list", selected: "整理资料"},
		{action: "math", selected: "1 + 1", extra: markdownEditOptions{Label: "salary_total/5", Display: true}},
	} {
		before := readMarkdownCapabilityText(t, runtime, documentID)
		result, invokeErr := invoke(operation.action, operation.selected, operation.extra)
		if invokeErr != nil {
			t.Fatalf("Markdown action %q: %v", operation.action, invokeErr)
		}
		var output struct {
			Changed      bool `json:"changed"`
			ContentBytes int  `json:"content_bytes"`
			Selection    struct {
				Start int `json:"start"`
				End   int `json:"end"`
			} `json:"selection"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil || !output.Changed || output.ContentBytes <= len(before) || output.Selection.Start < 0 || output.Selection.End < output.Selection.Start {
			t.Fatalf("Markdown %s result=%s error=%v before_bytes=%d", operation.action, result.Data, err, len(before))
		}
		if current := readMarkdownCapabilityText(t, runtime, documentID); len(current) != output.ContentBytes {
			t.Fatalf("Markdown %s content bytes=%d want=%d", operation.action, len(current), output.ContentBytes)
		}
	}
	finalText := readMarkdownCapabilityText(t, runtime, documentID)
	for _, fragment := range []string{"# 本周", "![工资统计图](file:///tmp/payroll.png)", "*[周报](https://example.test/周报?week=42)*", "`已核对`", "$$\nsalary_total/5\n$$", "> - [ ] 待办：生成", "- 整理资料"} {
		if !strings.Contains(finalText, fragment) {
			t.Fatalf("Markdown result missing %q: %s", fragment, finalText)
		}
	}

	beforeInvalid := mustGetDocumentInfo(t, runtime, documentID)
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "markdown-edit-invalid-range", DocumentID: documentID, CapabilityID: "markdown.text.edit",
		ExpectedRevision: revision, Operator: Operator{ID: "markdown-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, markdownEditArguments{Action: "bold", Start: 0, End: len([]rune(finalText)) + 1}),
	}); err == nil {
		t.Fatal("Markdown edit accepted a rune range beyond document end")
	}
	if afterInvalid := mustGetDocumentInfo(t, runtime, documentID); afterInvalid.Revision != beforeInvalid.Revision || afterInvalid.Dirty != beforeInvalid.Dirty {
		t.Fatalf("invalid Markdown range changed document state: before=%+v after=%+v", beforeInvalid, afterInvalid)
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "markdown-edit-invalid-destination", DocumentID: documentID, CapabilityID: "markdown.text.edit",
		ExpectedRevision: revision, Operator: Operator{ID: "markdown-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, markdownEditArguments{Action: "link", Start: 0, End: 0}),
	}); err == nil {
		t.Fatal("Markdown link edit accepted a missing destination")
	}
	if afterInvalid := mustGetDocumentInfo(t, runtime, documentID); afterInvalid.Revision != beforeInvalid.Revision || afterInvalid.Dirty != beforeInvalid.Dirty {
		t.Fatalf("invalid Markdown destination changed document state: before=%+v after=%+v", beforeInvalid, afterInvalid)
	}

	savedPath := filepath.Join(t.TempDir(), "weekly-report-edited.md")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, savedPath); err != nil {
		t.Fatalf("SaveDocumentAs Markdown: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("close Markdown: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, savedPath)
	if err != nil {
		t.Fatalf("reopen Markdown: %v", err)
	}
	if got := readMarkdownCapabilityText(t, runtime, reopened.DocumentID); got != finalText {
		t.Fatalf("Markdown after save/reopen differs:\n got: %q\nwant: %q", got, finalText)
	}
}

type testAgenttextEmptyArguments struct {
}

type testAgenttextQueryArguments struct {
	Query string `json:"query"`
}

type testAgenttextQueryCaseSensitiveArguments struct {
	Query         string `json:"query"`
	CaseSensitive bool   `json:"case_sensitive"`
	Limit         int    `json:"limit"`
}

func TestJSONSearchCapabilityReturnsEscapedPointersWithoutMutation(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	source := "{\n  \"a/b~c\": \"Budget approved\",\n  \"Rows\": [\n    {\"name\": \"Alice\", \"amount\": 12000},\n    {\"name\": \"alice\", \"amount\": 9000}\n  ],\n  \"ready\": true\n}"
	path := filepath.Join(t.TempDir(), "budget.json")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write JSON fixture: %v", err)
	}
	doc, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument(JSON): %v", err)
	}
	before := doc
	search := func(requestID, query string, caseSensitive bool, limit int) jsonSearchTestResult {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: doc.DocumentID, CapabilityID: "json.document.search",
			ExpectedRevision: doc.Revision, Operator: Operator{ID: "json-agent", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, testAgenttextQueryCaseSensitiveArguments{Query: query, CaseSensitive: caseSensitive, Limit: limit}),
		})
		if invokeErr != nil {
			t.Fatalf("JSON search %q: %v", query, invokeErr)
		}
		var output jsonSearchTestResult
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode JSON search %q: %v data=%s", query, err, result.Data)
		}
		return output
	}
	escaped := search("json-search-escaped-key", "Budget", false, 10)
	if len(escaped.Matches) != 1 || escaped.Matches[0].Pointer != "/a~1b~0c" || escaped.Matches[0].MatchedIn != "value" || escaped.Matches[0].Kind != "string" {
		t.Fatalf("escaped-key search=%+v", escaped)
	}
	limited := search("json-search-limited", "amount", false, 1)
	if len(limited.Matches) != 1 || limited.Matches[0].Pointer != "/Rows/0/amount" || !limited.Truncated {
		t.Fatalf("limited JSON search=%+v", limited)
	}
	caseSensitive := search("json-search-case-sensitive", "Alice", true, 10)
	if len(caseSensitive.Matches) != 1 || caseSensitive.Matches[0].Pointer != "/Rows/0/name" {
		t.Fatalf("case-sensitive JSON search=%+v", caseSensitive)
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "json-search-empty", DocumentID: doc.DocumentID, CapabilityID: "json.document.search",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "json-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, testAgenttextQueryArguments{Query: "  "}),
	}); err == nil {
		t.Fatal("JSON search accepted a blank query")
	}
	read, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "json-read-text-after-search", DocumentID: doc.DocumentID, CapabilityID: "json.document.read_text",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "json-agent", Kind: OperatorExternalAgent}, Arguments: mustJSON(t, testAgenttextEmptyArguments{}),
	})
	var readOutput struct {
		Text string `json:"text"`
	}
	if err == nil {
		err = json.Unmarshal(read.Data, &readOutput)
	}
	if err != nil || readOutput.Text != source {
		t.Fatalf("JSON source changed through read-only search: data=%s error=%v", read.Data, err)
	}
	if after, err := runtime.GetDocument(ctx, doc.DocumentID); err != nil || after.Revision != before.Revision || after.Dirty != before.Dirty {
		t.Fatalf("JSON search changed document state: before=%+v after=%+v error=%v", before, after, err)
	}
}

type jsonSearchTestResult struct {
	Matches []struct {
		Pointer   string `json:"pointer"`
		Kind      string `json:"kind"`
		MatchedIn string `json:"matched_in"`
		Preview   string `json:"preview"`
	} `json:"matches"`
	VisitedNodes int  `json:"visited_nodes"`
	Truncated    bool `json:"truncated"`
}

func readMarkdownCapabilityText(t *testing.T, runtime *Runtime, documentID string) string {
	t.Helper()
	snapshot, err := runtime.Queries().InspectDocument(context.Background(), InspectRequest{DocumentID: documentID, Limit: 1000})
	if err != nil {
		t.Fatalf("inspect Markdown text: %v", err)
	}
	var output struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(snapshot.Content, &output); err != nil {
		t.Fatalf("decode Markdown lines: %v content=%s", err, snapshot.Content)
	}
	return strings.Join(output.Lines, "\n")
}

func markdownRuneRange(source, selected string) (int, int, bool) {
	byteOffset := strings.Index(source, selected)
	if byteOffset < 0 {
		return 0, 0, false
	}
	start := len([]rune(source[:byteOffset]))
	return start, start + len([]rune(selected)), true
}
