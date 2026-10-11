package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"
)

func TestPDFEditCapabilitiesValidateAtomicityHistoryAndSaveReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "capabilities.pdf")
	writePDFEditCapabilityFixture(t, path)

	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	opened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	requestNumber := 0
	invoke := func(id string, args any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		info, getErr := runtime.GetDocument(ctx, opened.DocumentID)
		if getErr != nil {
			t.Fatalf("GetDocument before %s: %v", id, getErr)
		}
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("pdf-edit-%d", requestNumber), DocumentID: opened.DocumentID,
			CapabilityID: id, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "pdf-edit-test", Kind: OperatorHuman},
			Arguments: mustJSON(t, args),
		})
	}
	inspect := func() (DocumentSnapshot, pdfCapabilityQuery) {
		t.Helper()
		snapshot, inspectErr := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: opened.DocumentID})
		if inspectErr != nil {
			t.Fatalf("InspectDocument: %v", inspectErr)
		}
		var query pdfCapabilityQuery
		if err := json.Unmarshal(snapshot.Content, &query); err != nil {
			t.Fatalf("decode PDF query: %v; content=%s", err, snapshot.Content)
		}
		return snapshot, query
	}
	assertAtomicFailure := func(id string, args any) {
		t.Helper()
		before, _ := inspect()
		infoBefore := before.Info
		_, invokeErr := invoke(id, args)
		if invokeErr == nil {
			t.Fatalf("%s accepted invalid target: %v", id, args)
		}
		after, _ := inspect()
		if after.Info.Revision != infoBefore.Revision || after.Info.Dirty != infoBefore.Dirty || !bytes.Equal(after.Content, before.Content) {
			t.Fatalf("%s partially changed PDF: before=%+v after=%+v\nbefore content=%s\nafter content=%s", id, infoBefore, after.Info, before.Content, after.Content)
		}
	}
	assertChanged := func(id string, args any, wantChanged bool) CapabilityInvocationResult {
		t.Helper()
		before, _ := inspect()
		result, invokeErr := invoke(id, args)
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(%s): %v", id, invokeErr)
		}
		var output struct {
			Changed    bool `json:"changed"`
			PageNumber int  `json:"page_number"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode %s result: %v; data=%s", id, err, result.Data)
		}
		if output.Changed != wantChanged {
			t.Fatalf("%s changed=%v, want %v; data=%s", id, output.Changed, wantChanged, result.Data)
		}
		after, _ := inspect()
		wantRevision := before.Info.Revision
		if wantChanged {
			wantRevision++
		}
		if after.Info.Revision != wantRevision || after.Info.Dirty != (before.Info.Dirty || wantChanged) {
			t.Fatalf("%s session state: before=%+v after=%+v changed=%v", id, before.Info, after.Info, wantChanged)
		}
		return result
	}

	initial, initialQuery := inspect()
	if len(initialQuery.Pages) != 2 || len(initialQuery.Pages[0].Text) == 0 || initialQuery.Pages[0].Text[0].Text != "Hello PDF" {
		t.Fatalf("unexpected fixture query: %+v", initialQuery)
	}
	if initial.Info.Dirty {
		t.Fatalf("opened fixture is dirty: %+v", initial.Info)
	}

	t.Run("invalid targets preserve the live snapshot", func(t *testing.T) {
		assertAtomicFailure("pdf.page.rotate", map[string]any{"page_number": 99})
		assertAtomicFailure("pdf.annotation.note", map[string]any{"page_number": 1, "x": 100000, "y": 20, "text": "outside"})
		assertAtomicFailure("pdf.annotation.highlight", map[string]any{
			"page_number": 1,
			"bounds":      map[string]any{"x": 10, "y": 10, "w": 100000, "h": 20},
		})
		assertAtomicFailure("pdf.annotation.ink", map[string]any{
			"page_number": 1,
			"points":      []map[string]any{{"x": 20, "y": 20}, {"x": 100000, "y": 100000}},
		})
	})

	t.Run("empty history is a clean no-op", func(t *testing.T) {
		assertChanged("pdf.history.apply", map[string]any{"operation": "undo"}, false)
	})

	t.Run("replace is a revisioned edit with idempotent no-op", func(t *testing.T) {
		assertChanged("pdf.text.replace", map[string]any{"page_number": 1, "run": 0, "text": "Hello PDF"}, false)
		assertAtomicFailure("pdf.text.replace", map[string]any{"page_number": 1, "run": 99, "text": "invalid run"})
		assertChanged("pdf.text.replace", map[string]any{"page_number": 1, "run": 0, "text": "Updated PDF"}, true)
		_, query := inspect()
		if query.Pages[0].Text[0].Text != "Updated PDF" {
			t.Fatalf("replacement missing from live query: %+v", query.Pages[0].Text)
		}
		assertChanged("pdf.text.replace", map[string]any{"page_number": 1, "run": 0, "text": "Updated PDF"}, false)
	})

	t.Run("annotations and ink apply to explicit page geometry", func(t *testing.T) {
		page := initialQuery.Pages[0]
		assertChanged("pdf.annotation.note", map[string]any{"page_number": 1, "x": 120, "y": 160, "text": "Review this section"}, true)
		assertChanged("pdf.annotation.highlight", map[string]any{
			"page_number": 1,
			"bounds":      map[string]any{"x": 35, "y": 45, "w": 80, "h": 16},
			"text":        "Reviewed text",
		}, true)
		assertChanged("pdf.annotation.ink", map[string]any{
			"page_number": 1,
			"points":      []map[string]any{{"x": 180, "y": 180}, {"x": 210, "y": 210}, {"x": 240, "y": 180}},
		}, true)
		_, query := inspect()
		if len(query.Pages[0].Annotations) < 3 {
			t.Fatalf("note/highlight missing from PDF query: %+v", query.Pages[0].Annotations)
		}
		if page.Width <= 240 || page.Height <= 210 {
			t.Fatalf("fixture page too small for test geometry: %+v", page)
		}
		assertChanged("pdf.annotation.note", map[string]any{"page_number": 1, "x": 120, "y": 160, "text": ""}, false)
	})

	t.Run("history and rotation retain consistency through save and reopen", func(t *testing.T) {
		// Undo and redo target the last effective edit (the ink operation).
		assertChanged("pdf.history.apply", map[string]any{"operation": "undo"}, true)
		assertChanged("pdf.history.apply", map[string]any{"operation": "redo"}, true)
		assertChanged("pdf.page.rotate", map[string]any{"page_number": 1}, true)
		_, query := inspect()
		if query.Pages[0].Rotation != 90 || len(query.Pages[0].Annotations) < 3 {
			t.Fatalf("rotated page lost state: %+v", query.Pages[0])
		}
		if _, err := runtime.SaveDocument(ctx, opened.DocumentID); err != nil {
			t.Fatalf("SaveDocument: %v", err)
		}
		if info, err := runtime.GetDocument(ctx, opened.DocumentID); err != nil || info.Dirty {
			t.Fatalf("saved document state = %+v, error=%v", info, err)
		}
		if err := runtime.CloseDocument(ctx, opened.DocumentID); err != nil {
			t.Fatalf("CloseDocument after save: %v", err)
		}
		reopened, err := runtime.OpenDocument(ctx, path)
		if err != nil {
			t.Fatalf("OpenDocument after save: %v", err)
		}
		opened = reopened
		snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: opened.DocumentID})
		if err != nil {
			t.Fatalf("InspectDocument after reopen: %v", err)
		}
		var reopenedQuery pdfCapabilityQuery
		if err := json.Unmarshal(snapshot.Content, &reopenedQuery); err != nil {
			t.Fatalf("decode reopened PDF: %v", err)
		}
		if snapshot.Info.Dirty || reopenedQuery.Pages[0].Rotation != 90 || reopenedQuery.Pages[1].Text[0].Text != "untouched page" || len(reopenedQuery.Pages[0].Annotations) < 3 {
			t.Fatalf("reopened PDF did not preserve edits and untouched page: info=%+v query=%+v", snapshot.Info, reopenedQuery)
		}
		if !strings.Contains(reopenedQuery.Pages[0].Text[0].Text, "Updated PDF") {
			t.Fatalf("reopened PDF lost text replacement: %+v", reopenedQuery.Pages[0].Text)
		}
		reader, err := gopdf.Open(path)
		if err != nil || reader.Repaired() || reader.NumPages() != 2 {
			t.Fatalf("saved PDF validation: reader=%v error=%v", reader, err)
		}
	})
}

func TestPDFSearchCapabilityPaginatesWithoutMutation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "search.pdf")
	writePDFEditCapabilityFixture(t, path)
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	search := func(requestID, query string, startPage int) pdfSearchResult {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: doc.DocumentID, CapabilityID: "pdf.document.search",
			ExpectedRevision: doc.Revision, Operator: Operator{ID: "pdf-search-agent", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, map[string]any{
				"query": query, "start_page": startPage, "page_limit": 1, "max_results": 10,
			}),
		})
		if invokeErr != nil {
			t.Fatalf("PDF search %q from page %d: %v", query, startPage, invokeErr)
		}
		var output pdfSearchResult
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode PDF search result: %v data=%s", err, result.Data)
		}
		if result.Revision != doc.Revision {
			t.Fatalf("read-only PDF search revision=%d want=%d", result.Revision, doc.Revision)
		}
		return output
	}
	first := search("pdf-search-first-page", "hello", 1)
	if first.TotalPages != 2 || first.StartPage != 1 || first.PagesScanned != 1 || first.NextStartPage != 2 || len(first.Matches) != 1 {
		t.Fatalf("first PDF search page=%+v", first)
	}
	match := first.Matches[0]
	if match.PageNumber != 1 || match.Run != 0 || !match.Editable || !strings.Contains(match.Text, "Hello PDF") || match.Bounds.W <= 0 || match.Bounds.H <= 0 {
		t.Fatalf("first PDF search match=%+v", match)
	}
	second := search("pdf-search-second-page", "UNTOUCHED", first.NextStartPage)
	if second.TotalPages != 2 || second.StartPage != 2 || second.PagesScanned != 1 || second.NextStartPage != 0 || len(second.Matches) != 1 || second.Matches[0].PageNumber != 2 {
		t.Fatalf("continued PDF search page=%+v", second)
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pdf-search-past-end", DocumentID: doc.DocumentID, CapabilityID: "pdf.document.search",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "pdf-search-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"query": "none", "start_page": 3, "page_limit": 1, "max_results": 10}),
	}); err == nil {
		t.Fatal("PDF search accepted a start page beyond document end")
	}
	if after := mustGetDocumentInfo(t, runtime, doc.DocumentID); after.Revision != doc.Revision || after.Dirty {
		t.Fatalf("PDF search changed document state: opened=%+v after=%+v", doc, after)
	}
}

type pdfCapabilityQuery struct {
	Pages []struct {
		Index    int     `json:"index"`
		Rotation int     `json:"rotation"`
		Width    float64 `json:"width"`
		Height   float64 `json:"height"`
		Text     []struct {
			Run      int    `json:"run"`
			Text     string `json:"text"`
			Editable bool   `json:"editable"`
		} `json:"text"`
		Annotations []struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"annotations"`
	} `json:"pages"`
}

func writePDFEditCapabilityFixture(t *testing.T, path string) {
	t.Helper()
	doc := gopdf.New()
	page := doc.AddPage()
	page.SetFont(gopdf.Helvetica, 12)
	page.Text(40, 60, "Hello PDF")
	page.AddNote(90, 90, "existing note", gopdf.NoteOptions{})
	page = doc.AddPage()
	page.Text(40, 60, "untouched page")
	if err := os.WriteFile(path, mustPDFBytes(t, doc), 0o600); err != nil {
		t.Fatalf("write PDF fixture: %v", err)
	}
}

func mustPDFBytes(t *testing.T, doc *gopdf.Document) []byte {
	t.Helper()
	var output bytes.Buffer
	if _, err := doc.WriteTo(&output); err != nil {
		t.Fatalf("serialize PDF fixture: %v", err)
	}
	return output.Bytes()
}
