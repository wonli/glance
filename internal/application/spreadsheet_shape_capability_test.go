package application

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

type testSpreadsheetshapeSheetCellArguments struct {
	Sheet string `json:"sheet"`
	Cell  string `json:"cell"`
	Type  string `json:"type"`
	Text  string `json:"text"`
}

type testSpreadsheetshapeSheetCellTypeArguments struct {
	Sheet    string `json:"sheet"`
	Cell     string `json:"cell"`
	Type     string `json:"type"`
	WidthPx  int    `json:"width_px"`
	HeightPx int    `json:"height_px"`
	Text     string `json:"text"`
}

type testSpreadsheetshapeSheetCellTypeArguments111 struct {
	Sheet string `json:"sheet"`
	Cell  string `json:"cell"`
	Type  string `json:"type"`
}

func TestSpreadsheetShapeCapabilityUsesExplicitTargetsAndRoundTrips(t *testing.T) {
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
	invoke := func(arguments any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		info := mustGetDocumentInfo(t, runtime, documentID)
		return runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("xlsx-shape-%d", requestNumber), DocumentID: documentID,
			CapabilityID: "spreadsheet.shape.create", ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "shape-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}
	assertFailedAtomic := func(arguments any) {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, documentID)
		if _, err := invoke(arguments); err == nil {
			t.Fatal("invalid shape request unexpectedly succeeded")
		}
		if after := mustGetDocumentInfo(t, runtime, documentID); after.Revision != before.Revision || after.Dirty != before.Dirty {
			t.Fatalf("invalid shape request partially changed workbook: before=%+v after=%+v", before, after)
		}
	}
	assertFailedAtomic(testSpreadsheetshapeSheetCellTypeArguments111{Sheet: "Sheet1", Cell: "A0", Type: "rect"})
	assertFailedAtomic(testSpreadsheetshapeSheetCellTypeArguments111{Sheet: "Missing", Cell: "B2", Type: "rect"})

	before := mustGetDocumentInfo(t, runtime, documentID)
	shapeResult, err := invoke(testSpreadsheetshapeSheetCellTypeArguments{Sheet: "Sheet1", Cell: "D4", Type: "rightArrow", WidthPx: 240, HeightPx: 72, Text: "Agent status"})
	if err != nil {
		t.Fatalf("InvokeCapability(spreadsheet.shape.create): %v", err)
	}
	var shape struct {
		Sheet   string `json:"sheet"`
		Cell    string `json:"cell"`
		Type    string `json:"type"`
		Width   uint   `json:"width_px"`
		Height  uint   `json:"height_px"`
		Text    string `json:"text"`
		Changed bool   `json:"changed"`
	}
	if err := json.Unmarshal(shapeResult.Data, &shape); err != nil {
		t.Fatalf("decode shape result: %v; data=%s", err, shapeResult.Data)
	}
	if shape.Sheet != "Sheet1" || shape.Cell != "D4" || shape.Type != "rightArrow" || shape.Width != 240 || shape.Height != 72 || shape.Text != "Agent status" || !shape.Changed {
		t.Fatalf("shape result = %+v; data=%s", shape, shapeResult.Data)
	}
	after := mustGetDocumentInfo(t, runtime, documentID)
	if shapeResult.Revision != before.Revision+1 || after.Revision != shapeResult.Revision || !after.Dirty {
		t.Fatalf("shape creation state: before=%+v result=%+v after=%+v", before, shapeResult, after)
	}

	textBoxResult, err := invoke(testSpreadsheetshapeSheetCellArguments{Sheet: "Sheet1", Cell: "G7", Type: "rect", Text: "Text box"})
	if err != nil {
		t.Fatalf("create default-sized text box: %v", err)
	}
	var textBox struct {
		Cell   string `json:"cell"`
		Width  uint   `json:"width_px"`
		Height uint   `json:"height_px"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(textBoxResult.Data, &textBox); err != nil {
		t.Fatalf("decode text box result: %v; data=%s", err, textBoxResult.Data)
	}
	if textBox.Cell != "G7" || textBox.Width != 160 || textBox.Height != 90 || textBox.Text != "Text box" {
		t.Fatalf("default-sized text box result = %+v; data=%s", textBox, textBoxResult.Data)
	}

	path := filepath.Join(t.TempDir(), "agent-shapes.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after save: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after shape creation: %v", err)
	}
	if reopened.Dirty {
		t.Fatalf("reopened workbook is dirty: %+v", reopened)
	}

	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open saved XLSX: %v", err)
	}
	defer archive.Close()
	var drawings string
	for _, file := range archive.File {
		if !strings.HasPrefix(file.Name, "xl/drawings/drawing") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read %s: read=%v close=%v", file.Name, readErr, closeErr)
		}
		drawings += string(data)
	}
	for _, want := range []string{"Agent status", "Text box", "rightArrow", "<xdr:col>3</xdr:col>", "<xdr:row>3</xdr:row>", "<xdr:col>6</xdr:col>", "<xdr:row>6</xdr:row>"} {
		if !strings.Contains(drawings, want) {
			t.Fatalf("saved drawing parts are missing %q; drawing XML=%s", want, drawings)
		}
	}
}
