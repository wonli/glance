package application

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type wordRevisionCommandTestAdapter struct {
	*testDocumentAdapter
	callsMu sync.Mutex
	counts  map[string]int
	actions []string
}

func (a *wordRevisionCommandTestAdapter) ApplyWordRevisionChanges(ctx context.Context, action string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	a.callsMu.Lock()
	defer a.callsMu.Unlock()
	a.actions = append(a.actions, action)
	count := a.counts[action]
	a.counts[action] = 0
	if count > 0 {
		a.testDocumentAdapter.mu.Lock()
		a.dirty = true
		a.testDocumentAdapter.mu.Unlock()
	}
	return count, nil
}

func TestWordReviewRevisionApplyRequiresHumanConfirmationAndReturnsCount(t *testing.T) {
	ctx := context.Background()
	adapter := &wordRevisionCommandTestAdapter{
		testDocumentAdapter: &testDocumentAdapter{format: FormatDocx},
		counts:              map[string]int{"accept_all": 2},
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{
		&testFormatFactory{format: FormatDocx, openDoc: adapter},
	}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "review.docx"))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	request := CapabilityRequest{
		RequestID: "word-review-accept-all", DocumentID: doc.DocumentID,
		CapabilityID: "word.review.revisions.apply", ExpectedRevision: doc.Revision,
		Operator:  Operator{ID: "agent-1", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]string{"action": "accept_all"}),
	}
	staged, err := runtime.InvokeCapability(ctx, request)
	if err != nil {
		t.Fatalf("stage review operation: %v", err)
	}
	var confirmation struct {
		Status         string `json:"status"`
		ConfirmationID string `json:"confirmation_id"`
	}
	if err := json.Unmarshal(staged.Data, &confirmation); err != nil {
		t.Fatalf("decode staged result: %v; data=%s", err, staged.Data)
	}
	if confirmation.Status != "confirmation_required" || confirmation.ConfirmationID == "" {
		t.Fatalf("staged review result = %+v; want human confirmation", confirmation)
	}
	adapter.callsMu.Lock()
	callsBeforeApproval := len(adapter.actions)
	adapter.callsMu.Unlock()
	if callsBeforeApproval != 0 {
		t.Fatalf("revision adapter ran before approval: %v", adapter.actions)
	}
	if err := runtime.ResolveConfirmation(ctx, confirmation.ConfirmationID, true, Operator{ID: "human-1", Kind: OperatorHuman}); err != nil {
		t.Fatalf("approve review operation: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		info, err := runtime.GetConfirmation(ctx, confirmation.ConfirmationID)
		if err != nil {
			t.Fatalf("get confirmation result: %v", err)
		}
		if info.Status == ConfirmationCompleted {
			if info.Result == nil {
				t.Fatal("completed confirmation has no result")
			}
			var result struct {
				Action        string `json:"action"`
				RevisionCount int    `json:"revision_count"`
				Changed       bool   `json:"changed"`
			}
			if err := json.Unmarshal(info.Result.Data, &result); err != nil {
				t.Fatalf("decode applied result: %v; data=%s", err, info.Result.Data)
			}
			if result.Action != "accept_all" || result.RevisionCount != 2 || !result.Changed {
				t.Fatalf("applied result = %+v; want accepted 2 revisions", result)
			}
			current, err := runtime.GetDocument(ctx, doc.DocumentID)
			if err != nil || current.Revision != doc.Revision+1 || !current.Dirty {
				t.Fatalf("document after approval = %+v, error=%v; want one dirty revision", current, err)
			}
			adapter.callsMu.Lock()
			defer adapter.callsMu.Unlock()
			if len(adapter.actions) != 1 || adapter.actions[0] != "accept_all" {
				t.Fatalf("revision adapter calls = %v; want one accept_all", adapter.actions)
			}
			return
		}
		if info.Status == ConfirmationFailed {
			t.Fatalf("review operation failed after approval: %s", info.Error)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for approved review operation")
}

func TestWordRevisionApplyUsesLiveDOCXAndRoundTrips(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	for _, test := range []struct {
		action       string
		wantInserted bool
		wantDeleted  bool
	}{
		{action: "accept_all", wantInserted: true},
		{action: "reject_all", wantDeleted: true},
	} {
		t.Run(test.action, func(t *testing.T) {
			sourcePath := writeWordRevisionCommandFixture(t)
			doc, err := runtime.OpenDocument(ctx, sourcePath)
			if err != nil {
				t.Fatalf("OpenDocument: %v", err)
			}
			operator := Operator{ID: "review-agent", Kind: OperatorExternalAgent}
			invoke := func(requestID string, revision uint64) CapabilityInvocationResult {
				t.Helper()
				result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
					RequestID: requestID, DocumentID: doc.DocumentID,
					CapabilityID: "word.review.revisions.apply", ExpectedRevision: revision,
					Operator: operator, Arguments: mustJSON(t, map[string]string{"action": test.action}),
				})
				if invokeErr != nil {
					t.Fatalf("InvokeCapability(word.review.revisions.apply): %v", invokeErr)
				}
				return result
			}
			approve := func(staged CapabilityInvocationResult, wantCount int, wantChanged bool) CapabilityInvocationResult {
				t.Helper()
				var pending struct {
					Status         string `json:"status"`
					ConfirmationID string `json:"confirmation_id"`
				}
				if err := json.Unmarshal(staged.Data, &pending); err != nil || pending.Status != "confirmation_required" || pending.ConfirmationID == "" {
					t.Fatalf("revision action did not require human confirmation: data=%s error=%v", staged.Data, err)
				}
				if err := runtime.ResolveConfirmation(ctx, pending.ConfirmationID, true, Operator{ID: "review-human", Kind: OperatorHuman}); err != nil {
					t.Fatalf("approve revision action: %v", err)
				}
				completed := waitForConfirmationStatus(t, runtime, pending.ConfirmationID, ConfirmationCompleted)
				if completed.Result == nil {
					t.Fatalf("approved revision action has no result: %+v", completed)
				}
				var result struct {
					Action        string `json:"action"`
					RevisionCount int    `json:"revision_count"`
					Changed       bool   `json:"changed"`
				}
				if err := json.Unmarshal(completed.Result.Data, &result); err != nil || result.Action != test.action || result.RevisionCount != wantCount || result.Changed != wantChanged {
					t.Fatalf("applied revision result=%+v error=%v data=%s", result, err, completed.Result.Data)
				}
				return *completed.Result
			}

			beforeInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
			if err != nil {
				t.Fatalf("GetDocument before invalid action: %v", err)
			}
			if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
				RequestID: "word-revisions-invalid-" + test.action, DocumentID: doc.DocumentID,
				CapabilityID: "word.review.revisions.apply", ExpectedRevision: beforeInvalid.Revision,
				Operator: operator, Arguments: mustJSON(t, map[string]string{"action": "accept_one"}),
			}); err == nil {
				t.Fatal("revision command accepted an unsupported action")
			}
			if afterInvalid, err := runtime.GetDocument(ctx, doc.DocumentID); err != nil || afterInvalid.Revision != beforeInvalid.Revision || afterInvalid.Dirty != beforeInvalid.Dirty {
				t.Fatalf("invalid action changed document state: before=%+v after=%+v error=%v", beforeInvalid, afterInvalid, err)
			}

			declined := invoke("word-revisions-decline-"+test.action, beforeInvalid.Revision)
			var declinedRequest struct {
				ConfirmationID string `json:"confirmation_id"`
			}
			if err := json.Unmarshal(declined.Data, &declinedRequest); err != nil || declinedRequest.ConfirmationID == "" {
				t.Fatalf("decode declined request: data=%s error=%v", declined.Data, err)
			}
			if err := runtime.ResolveConfirmation(ctx, declinedRequest.ConfirmationID, true, operator); err != ErrHumanApprovalRequired {
				t.Fatalf("Agent self-approval error=%v; want ErrHumanApprovalRequired", err)
			}
			if err := runtime.ResolveConfirmation(ctx, declinedRequest.ConfirmationID, false, Operator{ID: "review-human", Kind: OperatorHuman}); err != nil {
				t.Fatalf("human rejection: %v", err)
			}
			waitForConfirmationStatus(t, runtime, declinedRequest.ConfirmationID, ConfirmationRejected)
			if afterDecline, err := runtime.GetDocument(ctx, doc.DocumentID); err != nil || afterDecline.Revision != beforeInvalid.Revision || afterDecline.Dirty != beforeInvalid.Dirty {
				t.Fatalf("declined revision action changed document state: before=%+v after=%+v error=%v", beforeInvalid, afterDecline, err)
			}

			staged := invoke("word-revisions-apply-"+test.action, beforeInvalid.Revision)
			var pending struct {
				Status         string `json:"status"`
				ConfirmationID string `json:"confirmation_id"`
			}
			if err := json.Unmarshal(staged.Data, &pending); err != nil || pending.Status != "confirmation_required" || pending.ConfirmationID == "" {
				t.Fatalf("revision action staging=%s error=%v", staged.Data, err)
			}
			if afterStage, err := runtime.GetDocument(ctx, doc.DocumentID); err != nil || afterStage.Revision != beforeInvalid.Revision || afterStage.Dirty != beforeInvalid.Dirty {
				t.Fatalf("revision action ran before confirmation: before=%+v after=%+v error=%v", beforeInvalid, afterStage, err)
			}
			applied := approve(staged, 2, true)
			if applied.Revision != beforeInvalid.Revision+1 {
				t.Fatalf("applied revision number=%d; want %d", applied.Revision, beforeInvalid.Revision+1)
			}
			stateAfterApply, err := runtime.GetDocument(ctx, doc.DocumentID)
			if err != nil || stateAfterApply.Revision != applied.Revision || !stateAfterApply.Dirty {
				t.Fatalf("document state after revision action=%+v error=%v", stateAfterApply, err)
			}

			outputPath := filepath.Join(t.TempDir(), test.action+".docx")
			if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, outputPath); err != nil {
				t.Fatalf("SaveDocumentAs: %v", err)
			}
			if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
				t.Fatalf("CloseDocument: %v", err)
			}
			reopened, err := runtime.OpenDocument(ctx, outputPath)
			if err != nil {
				t.Fatalf("OpenDocument after save: %v", err)
			}
			doc = reopened
			documentXML := readWordRevisionCommandDocumentXML(t, outputPath)
			if strings.Contains(documentXML, "<w:ins") || strings.Contains(documentXML, "<w:del") {
				t.Fatalf("revision markup survived %s: %s", test.action, documentXML)
			}
			if strings.Contains(documentXML, "Inserted") != test.wantInserted || strings.Contains(documentXML, "Deleted") != test.wantDeleted {
				t.Fatalf("saved %s content has inserted=%v deleted=%v; XML=%s", test.action, strings.Contains(documentXML, "Inserted"), strings.Contains(documentXML, "Deleted"), documentXML)
			}

			beforeNoop, err := runtime.GetDocument(ctx, doc.DocumentID)
			if err != nil || beforeNoop.Dirty {
				t.Fatalf("reopened document state=%+v error=%v; want clean", beforeNoop, err)
			}
			noOp := invoke("word-revisions-noop-"+test.action, beforeNoop.Revision)
			result := approve(noOp, 0, false)
			afterNoop, err := runtime.GetDocument(ctx, doc.DocumentID)
			if err != nil || result.Revision != beforeNoop.Revision || afterNoop.Revision != beforeNoop.Revision || afterNoop.Dirty {
				t.Fatalf("empty revision action changed state: before=%+v result=%+v after=%+v error=%v", beforeNoop, result, afterNoop, err)
			}
			if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
				t.Fatalf("CloseDocument after no-op: %v", err)
			}
		})
	}
}

func writeWordRevisionCommandFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tracked-revisions.docx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create DOCX fixture: %v", err)
	}
	archive := zip.NewWriter(file)
	parts := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Start </w:t></w:r><w:ins w:id="1" w:author="Agent"><w:r><w:t>Inserted </w:t></w:r></w:ins><w:del w:id="2" w:author="Agent"><w:r><w:delText>Deleted </w:delText></w:r></w:del><w:r><w:t>End</w:t></w:r></w:p></w:body></w:document>`,
	}
	for name, content := range parts {
		writer, err := archive.Create(name)
		if err != nil {
			_ = archive.Close()
			_ = file.Close()
			t.Fatalf("create DOCX part %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			_ = archive.Close()
			_ = file.Close()
			t.Fatalf("write DOCX part %s: %v", name, err)
		}
	}
	if err := archive.Close(); err != nil {
		_ = file.Close()
		t.Fatalf("close DOCX archive: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close DOCX fixture: %v", err)
	}
	return path
}

func readWordRevisionCommandDocumentXML(t *testing.T, path string) string {
	t.Helper()
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open DOCX package: %v", err)
	}
	defer archive.Close()
	for _, part := range archive.File {
		if part.Name != "word/document.xml" {
			continue
		}
		reader, err := part.Open()
		if err != nil {
			t.Fatalf("open word/document.xml: %v", err)
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read word/document.xml: read=%v close=%v", readErr, closeErr)
		}
		return string(data)
	}
	t.Fatal("DOCX package has no word/document.xml")
	return ""
}
