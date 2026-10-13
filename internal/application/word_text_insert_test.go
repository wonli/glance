package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

type testWordtextTargetRunIdArguments struct {
	Target           string `json:"target"`
	RunId            string `json:"run_id"`
	Text             string `json:"text"`
	AfterParagraphId string `json:"after_paragraph_id"`
}

type testWordtextTargetRunIdAfterParagraphIdArguments struct {
	Target           string `json:"target"`
	RunId            string `json:"run_id"`
	AfterParagraphId string `json:"after_paragraph_id"`
	Text             string `json:"text"`
}

type testWordtextTargetAfterParagraphIdArguments struct {
	Target           string `json:"target"`
	AfterParagraphId string `json:"after_paragraph_id"`
	Text             string `json:"text"`
}

type testWordtextTargetRunIdTextArguments struct {
	Target string `json:"target"`
	RunId  string `json:"run_id"`
	Text   string `json:"text"`
}

type testWordtextTextArguments struct {
	Text string `json:"text"`
}

func TestWordTextInsertCapabilitySupportsExplicitRunAndParagraphTargets(t *testing.T) {
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
	operator := Operator{ID: "dictation-agent", Kind: OperatorBuiltInAgent}
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "dictation-source", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: operator, Arguments: mustJSON(t, testWordtextTextArguments{Text: "已有正文"}),
	})
	if err != nil {
		t.Fatalf("append source paragraph: %v", err)
	}
	var source struct {
		ParagraphID string `json:"paragraph_id"`
		RunID       string `json:"run_id"`
	}
	if err := json.Unmarshal(appended.Data, &source); err != nil || source.ParagraphID == "" || source.RunID == "" {
		t.Fatalf("append result = %+v, error=%v, data=%s", source, err, appended.Data)
	}

	inserted, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "dictation-append-run", DocumentID: doc.DocumentID, CommandID: "word.text.insert",
		ExpectedRevision: appended.Revision, Operator: operator,
		Arguments: mustJSON(t, testWordtextTargetRunIdTextArguments{Target: "run", RunId: source.RunID, Text: "本周完成报表"}),
	})
	if err != nil {
		t.Fatalf("append dictated text to run: %v", err)
	}
	var result struct {
		Changed bool   `json:"changed"`
		Target  string `json:"target"`
		RunID   string `json:"run_id"`
		Text    string `json:"text"`
	}
	if err := json.Unmarshal(inserted.Data, &result); err != nil || !result.Changed || result.Target != "run" || result.RunID != source.RunID || result.Text != "本周完成报表" {
		t.Fatalf("run insertion result = %+v, error=%v, data=%s", result, err, inserted.Data)
	}

	textbox, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "dictation-insert-textbox", DocumentID: doc.DocumentID, CommandID: "word.text.insert",
		ExpectedRevision: inserted.Revision, Operator: operator,
		Arguments: mustJSON(t, testWordtextTargetAfterParagraphIdArguments{Target: "textbox", AfterParagraphId: source.ParagraphID, Text: "补充说明"}),
	})
	if err != nil {
		t.Fatalf("insert dictated text after paragraph: %v", err)
	}
	if !bytes.Contains(textbox.Data, []byte(`"target":"textbox"`)) || !bytes.Contains(textbox.Data, []byte("补充说明")) {
		t.Fatalf("text-box insertion result = %s", textbox.Data)
	}

	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 20})
	if err != nil {
		t.Fatalf("InspectDocument after dictation: %v", err)
	}
	if !bytes.Contains(snapshot.Content, []byte("已有正文 本周完成报表")) || !bytes.Contains(snapshot.Content, []byte("补充说明")) {
		t.Fatalf("dictation targets are not both present in the live document: %s", snapshot.Content)
	}

	invalid, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "dictation-invalid-target", DocumentID: doc.DocumentID, CommandID: "word.text.insert",
		ExpectedRevision: textbox.Revision, Operator: operator,
		Arguments: mustJSON(t, testWordtextTargetRunIdAfterParagraphIdArguments{Target: "run", RunId: source.RunID, AfterParagraphId: source.ParagraphID, Text: "invalid"}),
	})
	if !errors.Is(err, ErrInvalidCommandArguments) {
		t.Fatalf("run insertion with a second target returned result %+v, error=%v; want invalid arguments", invalid, err)
	}
	stateBeforeInvalidTargets := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	for index, arguments := range []testWordtextTargetRunIdArguments{
		{Target: "run", RunId: "missing-run", Text: "不应写入"},
		{Target: "textbox", AfterParagraphId: "missing-paragraph", Text: "不应写入"},
	} {
		if _, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: fmt.Sprintf("dictation-missing-target-%d", index), DocumentID: doc.DocumentID,
			CommandID: "word.text.insert", ExpectedRevision: stateBeforeInvalidTargets.Revision,
			Operator: operator, Arguments: mustJSON(t, arguments),
		}); err == nil {
			t.Errorf("dictation accepted an unknown stable target: %+v", arguments)
		}
	}
	if afterInvalidTargets := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterInvalidTargets.Revision != stateBeforeInvalidTargets.Revision || afterInvalidTargets.Dirty != stateBeforeInvalidTargets.Dirty {
		t.Fatalf("invalid dictation target changed document state: before=%+v after=%+v", stateBeforeInvalidTargets, afterInvalidTargets)
	}
	path := filepath.Join(t.TempDir(), "dictation.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil || saved.Dirty {
		t.Fatalf("save dictated DOCX = %+v, error=%v; want clean document", saved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close dictated DOCX: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen dictated DOCX: %v", err)
	}
	reopenedSnapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: reopened.DocumentID, Limit: 20})
	if err != nil || !bytes.Contains(reopenedSnapshot.Content, []byte("已有正文 本周完成报表")) || !bytes.Contains(reopenedSnapshot.Content, []byte("补充说明")) {
		t.Fatalf("dictated targets did not survive DOCX save/reopen: error=%v content=%s", err, reopenedSnapshot.Content)
	}
}

type testWordtextTextArguments144 struct {
	Text string `json:"text"`
}

func TestWordParagraphAppendUsesUnicodeCharacterLimitAtomically(t *testing.T) {
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
	operator := Operator{ID: "unicode-report-agent", Kind: OperatorExternalAgent}
	text := strings.Repeat("中", 6000)
	arguments := mustJSON(t, testWordtextTextArguments144{Text: text})
	descriptor, ok := runtime.Catalog().Descriptor("word.paragraph.append")
	if !ok {
		t.Fatal("word.paragraph.append is not registered")
	}
	if err := validateArguments(descriptor.InputSchema, arguments); err != nil {
		t.Fatalf("schema rejected %d-character Chinese paragraph: %v", len([]rune(text)), err)
	}
	result, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "unicode-report-paragraph", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("append %d-character Chinese paragraph: %v", len([]rune(text)), err)
	}
	var output struct {
		Changed     bool   `json:"changed"`
		ParagraphID string `json:"paragraph_id"`
		RunID       string `json:"run_id"`
		Text        string `json:"text"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil || !output.Changed || output.ParagraphID == "" || output.RunID == "" || output.Text != text {
		t.Fatalf("long paragraph result changed=%v paragraph=%q run=%q runes=%d error=%v", output.Changed, output.ParagraphID, output.RunID, len([]rune(output.Text)), err)
	}
	if result.Revision != doc.Revision+1 {
		t.Fatalf("long paragraph revision=%d; want %d", result.Revision, doc.Revision+1)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 20})
	if err != nil || !strings.Contains(string(snapshot.Content), text) {
		t.Fatalf("long paragraph is missing from live document: error=%v", err)
	}

	for name, invalidText := range map[string]string{
		"too-long":   strings.Repeat("中", 16001),
		"line-break": "第一行\n第二行",
	} {
		if _, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: "unicode-report-invalid-" + name, DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
			ExpectedRevision: result.Revision, Operator: operator,
			Arguments: mustJSON(t, testWordtextTextArguments144{Text: invalidText}),
		}); err == nil {
			t.Fatalf("append invalid paragraph %q succeeded", name)
		}
	}
	state := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if state.Revision != result.Revision || !state.Dirty {
		t.Fatalf("rejected paragraphs changed document state: revision=%d dirty=%v; want revision=%d dirty=true", state.Revision, state.Dirty, result.Revision)
	}
}

type testWordtextTargetRunIdTextArguments145 struct {
	Target string `json:"target"`
	RunId  string `json:"run_id"`
	Text   string `json:"text"`
}

type testWordtextTextArguments146 struct {
	Text string `json:"text"`
}

func TestWordTextInsertAcceptsUnicodeWithinSchemaLimit(t *testing.T) {
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
	operator := Operator{ID: "unicode-text-agent", Kind: OperatorExternalAgent}
	source, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "unicode-text-source", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: mustJSON(t, testWordtextTextArguments146{Text: "报告"}),
	})
	if err != nil {
		t.Fatalf("append source paragraph: %v", err)
	}
	var sourceData struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(source.Data, &sourceData); err != nil || sourceData.RunID == "" {
		t.Fatalf("source paragraph result=%s error=%v", source.Data, err)
	}
	text := strings.Repeat("周", 6000)
	arguments := mustJSON(t, testWordtextTargetRunIdTextArguments145{Target: "run", RunId: sourceData.RunID, Text: text})
	descriptor, ok := runtime.Catalog().Descriptor("word.text.insert")
	if !ok {
		t.Fatal("word.text.insert is not registered")
	}
	if err := validateArguments(descriptor.InputSchema, arguments); err != nil {
		t.Fatalf("schema rejected %d-character Chinese insertion: %v", len([]rune(text)), err)
	}
	result, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "unicode-text-insert", DocumentID: doc.DocumentID, CommandID: "word.text.insert",
		ExpectedRevision: source.Revision, Operator: operator, Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("insert %d-character Chinese text: %v", len([]rune(text)), err)
	}
	var output struct {
		Changed bool   `json:"changed"`
		RunID   string `json:"run_id"`
		Text    string `json:"text"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil || !output.Changed || output.RunID != sourceData.RunID || output.Text != text {
		t.Fatalf("long text insertion changed=%v run=%q runes=%d error=%v", output.Changed, output.RunID, len([]rune(output.Text)), err)
	}
	if result.Revision != source.Revision+1 {
		t.Fatalf("long text insertion revision=%d; want %d", result.Revision, source.Revision+1)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 20})
	if err != nil || !strings.Contains(string(snapshot.Content), text) {
		t.Fatalf("long text insertion is missing from live document: error=%v", err)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "unicode-text-insert-too-long", DocumentID: doc.DocumentID, CommandID: "word.text.insert",
		ExpectedRevision: result.Revision, Operator: operator,
		Arguments: mustJSON(t, testWordtextTargetRunIdTextArguments145{Target: "run", RunId: sourceData.RunID, Text: strings.Repeat("超", 16001)}),
	}); err == nil {
		t.Fatal("word.text.insert accepted more than 16000 Unicode characters")
	}
	state := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if state.Revision != result.Revision || !state.Dirty {
		t.Fatalf("rejected long text changed document state: revision=%d dirty=%v; want revision=%d dirty=true", state.Revision, state.Dirty, result.Revision)
	}
}
