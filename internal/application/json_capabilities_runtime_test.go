package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONTransformAndValidationCapabilitiesPreserveStateAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	validPath := filepath.Join(dir, "valid.json")
	const original = `{"z":1,"a":{"d":4,"b":2}}`
	if err := os.WriteFile(validPath, []byte(original), 0o600); err != nil {
		t.Fatalf("write JSON fixture: %v", err)
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	opened, err := runtime.OpenDocument(ctx, validPath)
	if err != nil {
		t.Fatalf("OpenDocument valid JSON: %v", err)
	}
	documentID := opened.DocumentID
	requestNumber := 0
	invoke := func(capabilityID string, arguments any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		info, getErr := runtime.GetDocument(ctx, documentID)
		if getErr != nil {
			t.Fatalf("GetDocument before %s: %v", capabilityID, getErr)
		}
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmtJSONRequestID(requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "json-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}
	getInfo := func() DocumentInfo {
		t.Helper()
		info, getErr := runtime.GetDocument(ctx, documentID)
		if getErr != nil {
			t.Fatalf("GetDocument: %v", getErr)
		}
		return info
	}
	readText := func() string {
		t.Helper()
		result, invokeErr := invoke("json.document.read_text", map[string]any{})
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(json.document.read_text): %v", invokeErr)
		}
		var output struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode JSON text result: %v; data=%s", err, result.Data)
		}
		return output.Text
	}
	transform := func(operation string, wantChanged bool) {
		t.Helper()
		before := getInfo()
		result, invokeErr := invoke("json.text.transform", map[string]any{"operation": operation})
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(json.text.transform %s): %v", operation, invokeErr)
		}
		var output struct {
			Changed      bool   `json:"changed"`
			ContentBytes int    `json:"content_bytes"`
			Operation    string `json:"operation"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode %s result: %v; data=%s", operation, err, result.Data)
		}
		if output.Operation != operation || output.Changed != wantChanged {
			t.Fatalf("transform result = %+v, want operation=%q changed=%v", output, operation, wantChanged)
		}
		text := readText()
		if output.ContentBytes != len(text) {
			t.Fatalf("content_bytes=%d, actual source bytes=%d", output.ContentBytes, len(text))
		}
		after := getInfo()
		wantRevision := before.Revision
		if wantChanged {
			wantRevision++
		}
		if after.Revision != wantRevision || after.Dirty != (before.Dirty || wantChanged) {
			t.Fatalf("%s state: before=%+v after=%+v changed=%v", operation, before, after, wantChanged)
		}
	}
	assertInvalidTransformIsAtomic := func(operation string) {
		t.Helper()
		beforeInfo := getInfo()
		beforeText := readText()
		_, invokeErr := invoke("json.text.transform", map[string]any{"operation": operation})
		if !errors.Is(invokeErr, ErrInvalidCommandArguments) {
			t.Fatalf("invalid operation error = %v, want ErrInvalidCommandArguments", invokeErr)
		}
		if after := getInfo(); after.Revision != beforeInfo.Revision || after.Dirty != beforeInfo.Dirty || readText() != beforeText {
			t.Fatalf("invalid operation partially changed JSON: before=%+v after=%+v", beforeInfo, after)
		}
	}
	validate := func(wantValid bool) string {
		t.Helper()
		before := getInfo()
		result, invokeErr := invoke("json.document.validate", map[string]any{})
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(json.document.validate): %v", invokeErr)
		}
		var output struct {
			Valid      bool   `json:"valid"`
			Diagnostic string `json:"diagnostic"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode validation result: %v; data=%s", err, result.Data)
		}
		if output.Valid != wantValid || !wantValid && output.Diagnostic == "" {
			t.Fatalf("validation result = %+v, want valid=%v", output, wantValid)
		}
		if after := getInfo(); after.Revision != before.Revision || after.Dirty != before.Dirty {
			t.Fatalf("read-only validation changed document state: before=%+v after=%+v", before, after)
		}
		return output.Diagnostic
	}

	if opened.Dirty {
		t.Fatalf("opened JSON unexpectedly dirty: %+v", opened)
	}
	validate(true)
	transform("format", true)
	transform("format", false)
	assertInvalidTransformIsAtomic("reverse")
	transform("minify", true)
	transform("sort_keys", true)
	if got, want := readText(), "{\n  \"a\": {\n    \"b\": 2,\n    \"d\": 4\n  },\n  \"z\": 1\n}\n"; got != want {
		t.Fatalf("sorted JSON = %q, want %q", got, want)
	}

	if _, err := runtime.SaveDocument(ctx, documentID); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	if info := getInfo(); info.Dirty {
		t.Fatalf("saved JSON remains dirty: %+v", info)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, validPath)
	if err != nil {
		t.Fatalf("OpenDocument after save: %v", err)
	}
	documentID = reopened.DocumentID
	if reopened.Dirty || readText() != "{\n  \"a\": {\n    \"b\": 2,\n    \"d\": 4\n  },\n  \"z\": 1\n}\n" {
		t.Fatalf("reopened JSON state/content = %+v, text=%q", reopened, readText())
	}
	transform("sort_keys", false) // A no-op on a clean reopened file must remain clean.

	invalidPath := filepath.Join(dir, "invalid.json")
	const invalidText = `{"broken":`
	if err := os.WriteFile(invalidPath, []byte(invalidText), 0o600); err != nil {
		t.Fatalf("write invalid JSON fixture: %v", err)
	}
	invalid, err := runtime.OpenDocument(ctx, invalidPath)
	if err != nil {
		t.Fatalf("OpenDocument invalid JSON editor text: %v", err)
	}
	documentID = invalid.DocumentID
	if diagnostic := validate(false); diagnostic == "" {
		t.Fatal("invalid JSON validation returned no diagnostic")
	}
	beforeInfo, beforeText := getInfo(), readText()
	if _, err := invoke("json.text.transform", map[string]any{"operation": "format"}); err == nil {
		t.Fatal("format accepted invalid JSON")
	}
	if after := getInfo(); after.Revision != beforeInfo.Revision || after.Dirty != beforeInfo.Dirty || readText() != beforeText {
		t.Fatalf("invalid JSON transform partially changed document: before=%+v after=%+v text=%q", beforeInfo, after, readText())
	}
}

func fmtJSONRequestID(number int) string { return fmt.Sprintf("json-capability-%d", number) }
