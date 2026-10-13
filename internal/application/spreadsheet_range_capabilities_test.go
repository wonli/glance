package application

import (
	"archive/zip"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
)

type spreadsheetRangeFormatSetPayload struct {
	Sheet  string             `json:"sheet"`
	Bounds document.CellRange `json:"bounds"`
	Patch  any                `json:"patch"`
}

type testSpreadsheetrangeSheetSourceCellArguments struct {
	Sheet        string             `json:"sheet"`
	SourceCell   string             `json:"source_cell"`
	TargetBounds document.CellRange `json:"target_bounds"`
}

type testSpreadsheetrangeSheetBoundsArguments struct {
	Sheet  string             `json:"sheet"`
	Bounds document.CellRange `json:"bounds"`
	Patch  any                `json:"patch"`
}

type testSpreadsheetrangeBoldFillColorArguments struct {
	Bold      bool   `json:"bold"`
	FillColor string `json:"fill_color"`
	FontColor string `json:"font_color"`
}

type testSpreadsheetrangeSheetQueryArguments struct {
	Sheet      string `json:"sheet"`
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

type testSpreadsheetrangeSheetBoundsArguments99 struct {
	Sheet  string             `json:"sheet"`
	Bounds document.CellRange `json:"bounds"`
}

type spreadsheetRangeSearchPayload struct {
	Sheet string `json:"sheet"`
	Query string `json:"query"`
}

type spreadsheetRangeSearchPayloadSheet struct {
	Sheet      string `json:"sheet"`
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

type spreadsheetRangeCopyPayload struct {
	Sheet  string             `json:"sheet"`
	Bounds document.CellRange `json:"bounds"`
}

func TestSpreadsheetRangeQueryAndFormattingCapabilitiesUseExplicitTargets(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument(XLSX): %v", err)
	}
	documentID := doc.DocumentID
	requestNumber := 0
	invoke := func(capabilityID string, arguments any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		info := mustGetDocumentInfo(t, runtime, documentID)
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("xlsx-range-%d", requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "range-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}
	assertChanged := func(capabilityID string, arguments any, wantChanged bool) CapabilityInvocationResult {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, documentID)
		result, invokeErr := invoke(capabilityID, arguments)
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(%s): %v", capabilityID, invokeErr)
		}
		var output struct {
			Changed bool `json:"changed"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode %s result: %v; data=%s", capabilityID, err, result.Data)
		}
		if output.Changed != wantChanged {
			t.Fatalf("%s changed=%v, want %v; data=%s", capabilityID, output.Changed, wantChanged, result.Data)
		}
		after := mustGetDocumentInfo(t, runtime, documentID)
		wantRevision := before.Revision
		if wantChanged {
			wantRevision++
		}
		if result.Revision != wantRevision || after.Revision != wantRevision || after.Dirty != (before.Dirty || wantChanged) {
			t.Fatalf("%s state: before=%+v result=%+v after=%+v", capabilityID, before, result, after)
		}
		return result
	}
	seed := []spreadsheetCellEditFixture{
		{Row: 1, Column: 1, Value: "Item"}, {Row: 1, Column: 2, Value: "Value"},
		{Row: 2, Column: 1, Value: "apple"}, {Row: 2, Column: 2, Value: "120"},
		{Row: 3, Column: 1, Value: "apple pie"}, {Row: 3, Column: 2, Value: "150"},
	}
	if _, err := invoke("spreadsheet.cells.set", spreadsheetCellsSetPayload{Sheet: "Sheet1", Edits: seed}); err != nil {
		t.Fatalf("seed range: %v", err)
	}
	beforeQuery := mustGetDocumentInfo(t, runtime, documentID)
	copyResult := assertChanged("spreadsheet.range.copy", spreadsheetRangeCopyPayload{Sheet: "Sheet1", Bounds: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 2}}, false)
	var copied struct {
		Rows [][]string `json:"rows"`
	}
	if err := json.Unmarshal(copyResult.Data, &copied); err != nil {
		t.Fatalf("decode range copy result: %v; data=%s", err, copyResult.Data)
	}
	if len(copied.Rows) != 3 || strings.Join(copied.Rows[1], ",") != "apple,120" || strings.Join(copied.Rows[2], ",") != "apple pie,150" {
		t.Fatalf("range copy rows=%v; want the explicit A1:B3 values", copied.Rows)
	}
	searchResult := assertChanged("spreadsheet.range.search", spreadsheetRangeSearchPayloadSheet{Sheet: "Sheet1", Query: "APPLE", MaxResults: 1}, false)
	var found struct {
		Matches []struct {
			Row    int    `json:"row"`
			Column int    `json:"column"`
			Value  string `json:"value"`
		} `json:"matches"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(searchResult.Data, &found); err != nil {
		t.Fatalf("decode range search result: %v; data=%s", err, searchResult.Data)
	}
	if len(found.Matches) != 1 || found.Matches[0].Row != 2 || found.Matches[0].Column != 1 || found.Matches[0].Value != "apple" || !found.Truncated {
		t.Fatalf("range search result=%+v; want first case-insensitive match and truncation", found)
	}
	unicodeQuery := strings.Repeat("查", 100)
	unicodeSearch := assertChanged("spreadsheet.range.search", spreadsheetRangeSearchPayload{Sheet: "Sheet1", Query: unicodeQuery}, false)
	var unicodeFound struct {
		Matches []json.RawMessage `json:"matches"`
	}
	if err := json.Unmarshal(unicodeSearch.Data, &unicodeFound); err != nil || len(unicodeFound.Matches) != 0 {
		t.Fatalf("Unicode search result=%s error=%v; want no matches", unicodeSearch.Data, err)
	}
	if afterQuery := mustGetDocumentInfo(t, runtime, documentID); afterQuery.Revision != beforeQuery.Revision || afterQuery.Dirty != beforeQuery.Dirty {
		t.Fatalf("range queries changed workbook state: before=%+v after=%+v", beforeQuery, afterQuery)
	}
	invalidQueries := []struct {
		capabilityID string
		arguments    any
	}{
		{
			capabilityID: "spreadsheet.range.copy",
			arguments:    testSpreadsheetrangeSheetBoundsArguments99{Sheet: "MissingSheet", Bounds: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}},
		},
		{
			capabilityID: "spreadsheet.range.copy",
			arguments:    testSpreadsheetrangeSheetBoundsArguments99{Sheet: "Sheet1", Bounds: document.CellRange{StartRow: 2, StartColumn: 1, EndRow: 1, EndColumn: 1}},
		},
		{
			capabilityID: "spreadsheet.range.copy",
			arguments:    testSpreadsheetrangeSheetBoundsArguments99{Sheet: "Sheet1", Bounds: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 101, EndColumn: 100}},
		},
		{
			capabilityID: "spreadsheet.range.search",
			arguments:    testSpreadsheetrangeSheetQueryArguments{Sheet: "MissingSheet", Query: "apple", MaxResults: 10},
		},
		{
			capabilityID: "spreadsheet.range.search",
			arguments:    testSpreadsheetrangeSheetQueryArguments{Sheet: "Sheet1", Query: "apple", MaxResults: 1001},
		},
	}
	for _, invalid := range invalidQueries {
		before := mustGetDocumentInfo(t, runtime, documentID)
		if _, err := invoke(invalid.capabilityID, invalid.arguments); err == nil {
			t.Fatalf("%s accepted invalid query: %+v", invalid.capabilityID, invalid.arguments)
		}
		after := mustGetDocumentInfo(t, runtime, documentID)
		if after.Revision != before.Revision || after.Dirty != before.Dirty {
			t.Fatalf("invalid read query %s changed workbook state: before=%+v after=%+v", invalid.capabilityID, before, after)
		}
	}

	formatPatch := testSpreadsheetrangeBoldFillColorArguments{Bold: true, FillColor: "FFFF00", FontColor: "112233"}
	formatArgs := testSpreadsheetrangeSheetBoundsArguments{Sheet: "Sheet1", Bounds: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, Patch: formatPatch}
	assertChanged("spreadsheet.range.format.set", formatArgs, true)
	assertChanged("spreadsheet.range.format.set", formatArgs, false)
	paintArgs := testSpreadsheetrangeSheetSourceCellArguments{Sheet: "Sheet1", SourceCell: "A1", TargetBounds: document.CellRange{StartRow: 1, StartColumn: 2, EndRow: 3, EndColumn: 2}}
	assertChanged("spreadsheet.format.paint", paintArgs, true)
	assertChanged("spreadsheet.format.paint", paintArgs, false)
	beforeInvalid := mustGetDocumentInfo(t, runtime, documentID)
	if _, err := invoke("spreadsheet.range.format.set", spreadsheetRangeFormatSetPayload{Sheet: "Missing", Bounds: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, Patch: formatPatch}); err == nil {
		t.Fatal("range formatting accepted a missing worksheet")
	}
	if afterInvalid := mustGetDocumentInfo(t, runtime, documentID); afterInvalid.Revision != beforeInvalid.Revision || afterInvalid.Dirty != beforeInvalid.Dirty {
		t.Fatalf("invalid format target partially changed workbook: before=%+v after=%+v", beforeInvalid, afterInvalid)
	}

	path := filepath.Join(t.TempDir(), "formatted-range.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after formatting: %v", err)
	}
	documentID = reopened.DocumentID
	if reopened.Dirty {
		t.Fatalf("reopened workbook is dirty: %+v", reopened)
	}
	assertChanged("spreadsheet.range.format.set", formatArgs, false)
	assertChanged("spreadsheet.format.paint", paintArgs, false)

	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open saved XLSX: %v", err)
	}
	defer archive.Close()
	var worksheetXML string
	for _, file := range archive.File {
		if file.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open worksheet XML: %v", err)
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read worksheet XML: read=%v close=%v", readErr, closeErr)
		}
		worksheetXML = string(data)
		break
	}
	var cells []struct {
		Ref     string `xml:"r,attr"`
		StyleID int    `xml:"s,attr"`
	}
	decoder := xml.NewDecoder(strings.NewReader(worksheetXML))
	for {
		token, tokenErr := decoder.Token()
		if tokenErr == io.EOF {
			break
		}
		if tokenErr != nil {
			t.Fatalf("parse worksheet XML: %v", tokenErr)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "c" {
			continue
		}
		var cell struct {
			Ref     string `xml:"r,attr"`
			StyleID int    `xml:"s,attr"`
		}
		if err := decoder.DecodeElement(&cell, &start); err != nil {
			t.Fatalf("decode cell style: %v", err)
		}
		cells = append(cells, cell)
	}
	styleIDs := map[string]int{}
	for _, cell := range cells {
		styleIDs[cell.Ref] = cell.StyleID
	}
	if styleIDs["A1"] == 0 || styleIDs["B1"] != styleIDs["A1"] || styleIDs["B2"] != styleIDs["A1"] || styleIDs["B3"] != styleIDs["A1"] {
		t.Fatalf("painted cells did not retain source style after save: styles=%v XML=%s", styleIDs, worksheetXML)
	}
}
