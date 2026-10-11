package application

import (
	"archive/zip"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestWordHeaderFooterUseUnicodeCharacterLimitsAndRoundTrip(t *testing.T) {
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
	operator := Operator{ID: "unicode-header-agent", Kind: OperatorExternalAgent}
	revision := doc.Revision
	headerText := strings.Repeat("页", 6000)
	footerText := strings.Repeat("脚", 6000)
	for _, item := range []struct {
		id   string
		text string
	}{
		{"word.header.set", headerText},
		{"word.footer.set", footerText},
	} {
		result, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: "unicode-" + item.id, DocumentID: doc.DocumentID, CommandID: item.id,
			ExpectedRevision: revision, Operator: operator,
			Arguments: mustJSON(t, map[string]string{"text": item.text}),
		})
		if err != nil {
			t.Fatalf("set %s with %d-character Chinese text: %v", item.id, len([]rune(item.text)), err)
		}
		if result.Revision != revision+1 {
			t.Fatalf("%s revision=%d; want %d", item.id, result.Revision, revision+1)
		}
		revision = result.Revision
	}

	path := filepath.Join(t.TempDir(), "unicode-header-footer.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open DOCX: %v", err)
	}
	defer archive.Close()
	parts := make(map[string]string)
	for _, name := range []string{"word/header1.xml", "word/footer1.xml"} {
		for _, file := range archive.File {
			if file.Name != name {
				continue
			}
			reader, err := file.Open()
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			contents, readErr := io.ReadAll(reader)
			closeErr := reader.Close()
			if readErr != nil || closeErr != nil {
				t.Fatalf("read %s: read=%v close=%v", name, readErr, closeErr)
			}
			parts[name] = string(contents)
			break
		}
	}
	if !strings.Contains(parts["word/header1.xml"], headerText) || !strings.Contains(parts["word/footer1.xml"], footerText) {
		t.Fatal("long Chinese header or footer text is missing after save")
	}

	stateBeforeInvalid := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	for _, command := range []string{"word.header.set", "word.footer.set"} {
		if _, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: "unicode-too-long-" + command, DocumentID: doc.DocumentID, CommandID: command,
			ExpectedRevision: revision, Operator: operator,
			Arguments: mustJSON(t, map[string]string{"text": strings.Repeat("超", 16001)}),
		}); err == nil {
			t.Fatalf("%s accepted more than 16000 Unicode characters", command)
		}
	}
	stateAfterInvalid := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if stateAfterInvalid.Revision != stateBeforeInvalid.Revision || stateAfterInvalid.Dirty != stateBeforeInvalid.Dirty {
		t.Fatalf("rejected header/footer changed document state: before=%+v after=%+v", stateBeforeInvalid, stateAfterInvalid)
	}
}
