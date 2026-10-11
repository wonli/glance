package application

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestWordMacroInspectionReportsExecutionBoundary(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	defer runtime.Close(ctx)
	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "word-macro-inspection", DocumentID: doc.DocumentID,
		CapabilityID: "word.macros.inspect", ExpectedRevision: doc.Revision,
		Operator:  Operator{ID: "macro-inspection-test", Kind: OperatorBuiltInAgent},
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("Invoke word.macros.inspect: %v", err)
	}
	var data struct {
		MacroPayloadDetected bool   `json:"macro_payload_detected"`
		ExecutionSupported   bool   `json:"execution_supported"`
		Summary              string `json:"summary"`
	}
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("decode macro inspection: %v", err)
	}
	if data.MacroPayloadDetected || data.ExecutionSupported || data.Summary == "" {
		t.Fatalf("macro inspection = %+v; blank DOCX should have no macro payload and Glance should not execute macros", data)
	}
}

func TestDetectWordMacroPayload(t *testing.T) {
	tests := []struct {
		name     string
		parts    map[string][]byte
		expected bool
	}{
		{name: "ordinary docx", parts: map[string][]byte{"[Content_Types].xml": []byte(`<Types/>`)}, expected: false},
		{name: "vba project part", parts: map[string][]byte{"word/vbaProject.bin": []byte{1, 2, 3}}, expected: true},
		{name: "macro enabled content type", parts: map[string][]byte{"[Content_Types].xml": []byte(`<Types ContentType="application/vnd.ms-word.document.macroEnabled.main+xml"/>`)}, expected: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := wordMacroTestSnapshot(t, test.parts)
			got, err := detectWordMacroPayload(snapshot)
			if err != nil {
				t.Fatalf("detectWordMacroPayload: %v", err)
			}
			if got != test.expected {
				t.Fatalf("macro payload detected = %v, want %v", got, test.expected)
			}
		})
	}
}

func wordMacroTestSnapshot(t *testing.T, parts map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, data := range parts {
		part, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create ZIP part %s: %v", name, err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatalf("write ZIP part %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close ZIP snapshot: %v", err)
	}
	return buffer.Bytes()
}
