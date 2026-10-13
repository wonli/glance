package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

type spreadsheetGridlinesSetPayload struct {
	Sheet string `json:"sheet"`
	Show  bool   `json:"show"`
}

type spreadsheetPanesSetFreezeAtPayload struct {
	Sheet     string `json:"sheet"`
	Operation string `json:"operation"`
	Rows      int    `json:"rows"`
	Columns   int    `json:"columns"`
}

type spreadsheetPageLayoutSetPayload struct {
	Sheet       string `json:"sheet"`
	Operation   string `json:"operation"`
	Orientation string `json:"orientation"`
}

type spreadsheetViewRulerSetPayload struct {
	Sheet     string `json:"sheet"`
	ShowRuler bool   `json:"show_ruler"`
}

func TestSpreadsheetViewAndPageSetupCapabilitiesRoundTripAndNoop(t *testing.T) {
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
	operator := Operator{ID: "xlsx-view-capabilities-test", Kind: OperatorHuman}
	const sheet = "Sheet1"
	revision := doc.Revision
	requestIndex := 0
	invoke := func(capabilityID string, args any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestIndex++
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID:  "xlsx-view-capabilities-" + string(rune('a'+requestIndex)),
			DocumentID: doc.DocumentID, CapabilityID: capabilityID, ExpectedRevision: revision,
			Operator: operator, Arguments: mustJSON(t, args),
		})
		if invokeErr == nil && result.Revision > revision {
			revision = result.Revision
		}
		return result, invokeErr
	}

	readView := func(documentID string, currentRevision uint64, requestID string) (spreadsheetViewTestState, error) {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: documentID, CapabilityID: "spreadsheet.view.get",
			ExpectedRevision: currentRevision, Operator: operator,
			Arguments: mustJSON(t, map[string]string{"sheet": sheet}),
		})
		if invokeErr != nil {
			return spreadsheetViewTestState{}, invokeErr
		}
		var state spreadsheetViewTestState
		if err := json.Unmarshal(result.Data, &state); err != nil {
			return spreadsheetViewTestState{}, err
		}
		return state, nil
	}
	readPanes := func(documentID string, currentRevision uint64, requestID string) (spreadsheetPanesTestState, error) {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: documentID, CapabilityID: "spreadsheet.panes.get",
			ExpectedRevision: currentRevision, Operator: operator,
			Arguments: mustJSON(t, map[string]string{"sheet": sheet}),
		})
		if invokeErr != nil {
			return spreadsheetPanesTestState{}, invokeErr
		}
		var state spreadsheetPanesTestState
		if err := json.Unmarshal(result.Data, &state); err != nil {
			return spreadsheetPanesTestState{}, err
		}
		return state, nil
	}
	readGridlines := func(documentID string, currentRevision uint64, requestID string) (bool, error) {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: documentID, CapabilityID: "spreadsheet.gridlines.get",
			ExpectedRevision: currentRevision, Operator: operator,
			Arguments: mustJSON(t, map[string]string{"sheet": sheet}),
		})
		if invokeErr != nil {
			return false, invokeErr
		}
		var state struct {
			Show bool `json:"show"`
		}
		if err := json.Unmarshal(result.Data, &state); err != nil {
			return false, err
		}
		return state.Show, nil
	}
	readLayout := func(documentID string, currentRevision uint64, requestID string) (spreadsheetPageLayoutTestState, error) {
		t.Helper()
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: documentID, CapabilityID: "spreadsheet.page_layout.get",
			ExpectedRevision: currentRevision, Operator: operator,
			Arguments: mustJSON(t, map[string]string{"sheet": sheet}),
		})
		if invokeErr != nil {
			return spreadsheetPageLayoutTestState{}, invokeErr
		}
		var state spreadsheetPageLayoutTestState
		if err := json.Unmarshal(result.Data, &state); err != nil {
			return spreadsheetPageLayoutTestState{}, err
		}
		return state, nil
	}

	view, err := readView(doc.DocumentID, revision, "xlsx-view-initial")
	if err != nil || view.ViewMode != "normal" {
		t.Fatalf("initial worksheet view=%+v error=%v", view, err)
	}
	wantedRuler := !view.ShowRuler
	mode, err := invoke("spreadsheet.view.mode.set", map[string]string{"sheet": sheet, "mode": "pageBreakPreview"})
	if err != nil || mode.Revision != revision || !containsJSONText(mode.Data, `"changed":true`) {
		t.Fatalf("set worksheet view mode: result=%s revision=%d error=%v", mode.Data, mode.Revision, err)
	}
	modeNoOp, err := invoke("spreadsheet.view.mode.set", map[string]string{"sheet": sheet, "mode": "pageBreakPreview"})
	if err != nil || modeNoOp.Revision != revision || !containsJSONText(modeNoOp.Data, `"changed":false`) {
		t.Fatalf("repeated worksheet view mode should be a no-op: result=%s revision=%d error=%v", modeNoOp.Data, modeNoOp.Revision, err)
	}
	ruler, err := invoke("spreadsheet.view.ruler.set", spreadsheetViewRulerSetPayload{Sheet: sheet, ShowRuler: wantedRuler})
	if err != nil || !containsJSONText(ruler.Data, `"changed":true`) {
		t.Fatalf("show worksheet ruler: result=%s error=%v", ruler.Data, err)
	}
	rulerNoOp, err := invoke("spreadsheet.view.ruler.set", spreadsheetViewRulerSetPayload{Sheet: sheet, ShowRuler: wantedRuler})
	if err != nil || rulerNoOp.Revision != revision || !containsJSONText(rulerNoOp.Data, `"changed":false`) {
		t.Fatalf("repeated ruler setting should be a no-op: result=%s revision=%d error=%v", rulerNoOp.Data, rulerNoOp.Revision, err)
	}
	view, err = readView(doc.DocumentID, revision, "xlsx-view-after-set")
	if err != nil || view.ViewMode != "pageBreakPreview" || view.ShowRuler != wantedRuler {
		t.Fatalf("worksheet view after set=%+v error=%v", view, err)
	}

	panes, err := readPanes(doc.DocumentID, revision, "xlsx-panes-initial")
	if err != nil || panes.Frozen {
		t.Fatalf("initial panes=%+v error=%v", panes, err)
	}
	freeze, err := invoke("spreadsheet.panes.set", spreadsheetPanesSetFreezeAtPayload{Sheet: sheet, Operation: "freeze_at", Rows: 1, Columns: 2})
	if err != nil || !containsJSONText(freeze.Data, `"changed":true`) {
		t.Fatalf("freeze worksheet panes: result=%s error=%v", freeze.Data, err)
	}
	freezeNoOp, err := invoke("spreadsheet.panes.set", spreadsheetPanesSetFreezeAtPayload{Sheet: sheet, Operation: "freeze_at", Rows: 1, Columns: 2})
	if err != nil || freezeNoOp.Revision != revision || !containsJSONText(freezeNoOp.Data, `"changed":false`) {
		t.Fatalf("repeated pane freeze should be a no-op: result=%s revision=%d error=%v", freezeNoOp.Data, freezeNoOp.Revision, err)
	}
	panes, err = readPanes(doc.DocumentID, revision, "xlsx-panes-after-freeze")
	if err != nil || !panes.Frozen || panes.XSplit != 2 || panes.YSplit != 1 {
		t.Fatalf("panes after freeze=%+v error=%v", panes, err)
	}

	gridlinesBefore, err := readGridlines(doc.DocumentID, revision, "xlsx-gridlines-initial")
	if err != nil {
		t.Fatalf("read initial gridlines: %v", err)
	}
	gridlineSet, err := invoke("spreadsheet.gridlines.set", spreadsheetGridlinesSetPayload{Sheet: sheet, Show: !gridlinesBefore})
	if err != nil || !containsJSONText(gridlineSet.Data, `"changed":true`) {
		t.Fatalf("toggle gridlines: result=%s error=%v", gridlineSet.Data, err)
	}
	gridlineNoOp, err := invoke("spreadsheet.gridlines.set", spreadsheetGridlinesSetPayload{Sheet: sheet, Show: !gridlinesBefore})
	if err != nil || gridlineNoOp.Revision != revision || !containsJSONText(gridlineNoOp.Data, `"changed":false`) {
		t.Fatalf("repeated gridline setting should be a no-op: result=%s revision=%d error=%v", gridlineNoOp.Data, gridlineNoOp.Revision, err)
	}
	gridlinesAfter, err := readGridlines(doc.DocumentID, revision, "xlsx-gridlines-after-set")
	if err != nil || gridlinesAfter == gridlinesBefore {
		t.Fatalf("gridline state after set=%v before=%v error=%v", gridlinesAfter, gridlinesBefore, err)
	}

	layoutBefore, err := readLayout(doc.DocumentID, revision, "xlsx-page-layout-initial")
	if err != nil || layoutBefore.Layout.Orientation == "" {
		t.Fatalf("initial page layout=%+v error=%v", layoutBefore, err)
	}
	wantedOrientation := "landscape"
	if layoutBefore.Layout.Orientation == wantedOrientation {
		wantedOrientation = "portrait"
	}
	pageLayout, err := invoke("spreadsheet.page_layout.set", spreadsheetPageLayoutSetPayload{Sheet: sheet, Operation: "orientation", Orientation: wantedOrientation})
	if err != nil || !containsJSONText(pageLayout.Data, `"changed":true`) {
		t.Fatalf("set page orientation: result=%s error=%v", pageLayout.Data, err)
	}
	pageLayoutNoOp, err := invoke("spreadsheet.page_layout.set", spreadsheetPageLayoutSetPayload{Sheet: sheet, Operation: "orientation", Orientation: wantedOrientation})
	if err != nil || pageLayoutNoOp.Revision != revision || !containsJSONText(pageLayoutNoOp.Data, `"changed":false`) {
		t.Fatalf("repeated page orientation should be a no-op: result=%s revision=%d error=%v", pageLayoutNoOp.Data, pageLayoutNoOp.Revision, err)
	}
	layoutAfter, err := readLayout(doc.DocumentID, revision, "xlsx-page-layout-after-set")
	if err != nil || layoutAfter.Layout.Orientation != wantedOrientation {
		t.Fatalf("page layout after set=%+v error=%v", layoutAfter, err)
	}

	stateBeforeInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid view target: %v", err)
	}
	if _, err := invoke("spreadsheet.page_layout.set", spreadsheetPageLayoutSetPayload{Sheet: sheet, Operation: "orientation", Orientation: "diagonal"}); err == nil {
		t.Fatal("page layout accepted an invalid orientation")
	}
	if _, err := invoke("spreadsheet.panes.set", spreadsheetPanesSetFreezeAtPayload{Sheet: sheet, Operation: "freeze_at", Rows: 0, Columns: 0}); err == nil {
		t.Fatal("pane freeze accepted an empty frozen region")
	}
	if _, err := invoke("spreadsheet.gridlines.set", spreadsheetGridlinesSetPayload{Sheet: "MissingSheet", Show: false}); err == nil {
		t.Fatal("gridline update accepted a missing worksheet")
	}
	stateAfterInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterInvalid.Revision != stateBeforeInvalid.Revision || stateAfterInvalid.Dirty != stateBeforeInvalid.Dirty {
		t.Fatalf("invalid worksheet view operation changed state: before=%+v after=%+v error=%v", stateBeforeInvalid, stateAfterInvalid, err)
	}

	path := filepath.Join(t.TempDir(), "spreadsheet-view-settings.xlsx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil || saved.SourcePath != canonicalPath(path) || saved.Dirty {
		t.Fatalf("save worksheet view settings: saved=%+v error=%v", saved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close workbook before settings reopen: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen workbook with view settings: %v", err)
	}
	reopenedView, err := readView(reopened.DocumentID, reopened.Revision, "xlsx-view-after-reopen")
	if err != nil || reopenedView.ViewMode != "pageBreakPreview" || reopenedView.ShowRuler != wantedRuler {
		t.Fatalf("reopened view state=%+v error=%v", reopenedView, err)
	}
	reopenedPanes, err := readPanes(reopened.DocumentID, reopened.Revision, "xlsx-panes-after-reopen")
	if err != nil || !reopenedPanes.Frozen || reopenedPanes.XSplit != 2 || reopenedPanes.YSplit != 1 {
		t.Fatalf("reopened panes=%+v error=%v", reopenedPanes, err)
	}
	reopenedGridlines, err := readGridlines(reopened.DocumentID, reopened.Revision, "xlsx-gridlines-after-reopen")
	if err != nil || reopenedGridlines == gridlinesBefore {
		t.Fatalf("reopened gridlines=%v initial=%v error=%v", reopenedGridlines, gridlinesBefore, err)
	}
	reopenedLayout, err := readLayout(reopened.DocumentID, reopened.Revision, "xlsx-page-layout-after-reopen")
	if err != nil || reopenedLayout.Layout.Orientation != wantedOrientation {
		t.Fatalf("reopened page layout=%+v error=%v", reopenedLayout, err)
	}
}

type spreadsheetViewTestState struct {
	ViewMode  string `json:"view_mode"`
	ShowRuler bool   `json:"show_ruler"`
}

type spreadsheetPanesTestState struct {
	Frozen bool `json:"frozen"`
	XSplit int  `json:"x_split"`
	YSplit int  `json:"y_split"`
}

type spreadsheetPageLayoutTestState struct {
	Layout struct {
		Orientation string `json:"orientation"`
	} `json:"layout"`
}
