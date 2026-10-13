package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"
)

func TestPresentationThemesAndSlideDuplicateRoundTrip(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatPPTX)
	if err != nil {
		t.Fatalf("CreateDocument(PPTX): %v", err)
	}
	operator := Operator{ID: "pptx-effective-command-test", Kind: OperatorHuman}
	initial, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID})
	if err != nil {
		t.Fatalf("inspect new presentation: %v", err)
	}
	var before struct {
		SlideCount int `json:"slide_count"`
		Slides     []struct {
			ID uint32 `json:"slide_id"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(initial.Content, &before); err != nil || before.SlideCount != 1 || len(before.Slides) != 1 || before.Slides[0].ID == 0 {
		t.Fatalf("initial presentation = %s, error=%v", initial.Content, err)
	}
	sourceSlideID := before.Slides[0].ID
	revision := doc.Revision

	palette, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-palette-set", DocumentID: doc.DocumentID, CapabilityID: "presentation.theme.palette.set",
		ExpectedRevision: revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"palette_id": "ocean"}),
	})
	if err != nil || !containsJSONText(palette.Data, `"changed":true`) {
		t.Fatalf("set presentation palette: result=%s error=%v", palette.Data, err)
	}
	revision = palette.Revision
	paletteState, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument after palette set: %v", err)
	}
	paletteNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-palette-set-noop", DocumentID: doc.DocumentID, CapabilityID: "presentation.theme.palette.set",
		ExpectedRevision: revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"palette_id": "ocean"}),
	})
	if err != nil || paletteNoOp.Revision != revision || !containsJSONText(paletteNoOp.Data, `"changed":false`) {
		t.Fatalf("repeat presentation palette should be a no-op: result=%s revision=%d error=%v", paletteNoOp.Data, paletteNoOp.Revision, err)
	}
	paletteNoOpState, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || paletteNoOpState.Revision != paletteState.Revision || paletteNoOpState.Dirty != paletteState.Dirty {
		t.Fatalf("palette no-op changed revision/dirty state: before=%+v after=%+v error=%v", paletteState, paletteNoOpState, err)
	}
	theme, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-theme-set", DocumentID: doc.DocumentID, CapabilityID: "presentation.theme.set",
		ExpectedRevision: revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"theme_id": "glance-corporate"}),
	})
	if err != nil || !containsJSONText(theme.Data, `"changed":true`) {
		t.Fatalf("set presentation theme: result=%s error=%v", theme.Data, err)
	}
	revision = theme.Revision
	themeState, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument after theme set: %v", err)
	}
	themeNoOp, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-theme-set-noop", DocumentID: doc.DocumentID, CapabilityID: "presentation.theme.set",
		ExpectedRevision: revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"theme_id": "glance-corporate"}),
	})
	if err != nil || themeNoOp.Revision != revision || !containsJSONText(themeNoOp.Data, `"changed":false`) {
		t.Fatalf("repeat presentation theme should be a no-op: result=%s revision=%d error=%v", themeNoOp.Data, themeNoOp.Revision, err)
	}
	themeNoOpState, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || themeNoOpState.Revision != themeState.Revision || themeNoOpState.Dirty != themeState.Dirty {
		t.Fatalf("theme no-op changed revision/dirty state: before=%+v after=%+v error=%v", themeState, themeNoOpState, err)
	}

	beforeInvalidTheme, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid theme: %v", err)
	}
	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-invalid-theme", DocumentID: doc.DocumentID, CapabilityID: "presentation.theme.set",
		ExpectedRevision: revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"theme_id": "unknown-theme"}),
	})
	if err == nil {
		t.Fatal("unknown presentation theme was accepted")
	}
	state, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || state.Revision != beforeInvalidTheme.Revision || state.Dirty != beforeInvalidTheme.Dirty {
		t.Fatalf("invalid theme changed revision/dirty state: before=%+v after=%+v error=%v", beforeInvalidTheme, state, err)
	}

	duplicate, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-slide-duplicate", DocumentID: doc.DocumentID, CapabilityID: "presentation.slide.duplicate",
		ExpectedRevision: revision, Operator: operator,
		Arguments: mustJSON(t, map[string]uint32{"slide_id": sourceSlideID}),
	})
	if err != nil {
		t.Fatalf("duplicate presentation slide: %v", err)
	}
	var duplicateResult struct {
		SourceSlideID uint32 `json:"source_slide_id"`
		SlideID       uint32 `json:"slide_id"`
		SlideNumber   int    `json:"slide_number"`
		SlideCount    int    `json:"slide_count"`
	}
	if err := json.Unmarshal(duplicate.Data, &duplicateResult); err != nil || duplicateResult.SourceSlideID != sourceSlideID || duplicateResult.SlideID == 0 || duplicateResult.SlideID == sourceSlideID || duplicateResult.SlideNumber != 2 || duplicateResult.SlideCount != 2 {
		t.Fatalf("duplicate result=%s error=%v", duplicate.Data, err)
	}
	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-slide-duplicate-invalid-target", DocumentID: doc.DocumentID, CapabilityID: "presentation.slide.duplicate",
		ExpectedRevision: duplicate.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]uint32{"slide_id": 4294967295}),
	})
	if err == nil {
		t.Fatal("slide duplicate accepted a nonexistent stable slide_id")
	}
	state, err = runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || state.Revision != duplicate.Revision || !state.Dirty {
		t.Fatalf("invalid duplicate target changed document state: state=%+v error=%v", state, err)
	}

	path := filepath.Join(t.TempDir(), "presentation-themes-duplicate.pptx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil || saved.Dirty {
		t.Fatalf("save themed presentation: saved=%+v error=%v", saved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close before PPTX round trip: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen themed presentation: %v", err)
	}
	inspection, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: reopened.DocumentID})
	if err != nil {
		t.Fatalf("inspect reopened presentation: %v", err)
	}
	var after struct {
		SlideCount int `json:"slide_count"`
		Slides     []struct {
			ID uint32 `json:"slide_id"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(inspection.Content, &after); err != nil || after.SlideCount != 2 || len(after.Slides) != 2 || after.Slides[0].ID != sourceSlideID || after.Slides[1].ID != duplicateResult.SlideID {
		t.Fatalf("reopened duplicated slides=%s error=%v", inspection.Content, err)
	}
	themeNoOp, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-theme-persisted", DocumentID: reopened.DocumentID, CapabilityID: "presentation.theme.set",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"theme_id": "glance-corporate"}),
	})
	if err != nil || !containsJSONText(themeNoOp.Data, `"changed":false`) || themeNoOp.Revision != reopened.Revision {
		t.Fatalf("theme was not retained after save/reopen: result=%s revision=%d error=%v", themeNoOp.Data, themeNoOp.Revision, err)
	}
	cleanAfterThemeNoOp, err := runtime.GetDocument(ctx, reopened.DocumentID)
	if err != nil || cleanAfterThemeNoOp.Revision != reopened.Revision || cleanAfterThemeNoOp.Dirty {
		t.Fatalf("theme no-op after reopen changed clean state: state=%+v error=%v", cleanAfterThemeNoOp, err)
	}
	paletteNoOp, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-palette-persisted", DocumentID: reopened.DocumentID, CapabilityID: "presentation.theme.palette.set",
		ExpectedRevision: reopened.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"palette_id": "ocean"}),
	})
	if err != nil || !containsJSONText(paletteNoOp.Data, `"changed":true`) || paletteNoOp.Revision != reopened.Revision+1 {
		t.Fatalf("palette command did not apply after save/reopen: result=%s revision=%d error=%v", paletteNoOp.Data, paletteNoOp.Revision, err)
	}
	paletteNoOp, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-palette-persisted-noop", DocumentID: reopened.DocumentID, CapabilityID: "presentation.theme.palette.set",
		ExpectedRevision: paletteNoOp.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"palette_id": "ocean"}),
	})
	if err != nil || !containsJSONText(paletteNoOp.Data, `"changed":false`) || paletteNoOp.Revision != reopened.Revision+1 {
		t.Fatalf("repeated palette command after save/reopen should be a no-op: result=%s revision=%d error=%v", paletteNoOp.Data, paletteNoOp.Revision, err)
	}
	paletteNoOpState, err = runtime.GetDocument(ctx, reopened.DocumentID)
	if err != nil || paletteNoOpState.Revision != paletteNoOp.Revision || !paletteNoOpState.Dirty {
		t.Fatalf("palette no-op after reopen changed revision/dirty state: state=%+v error=%v", paletteNoOpState, err)
	}
}

type testPresentationeffectiveSlideIdElementIdsArguments struct {
	SlideId    uint32   `json:"slide_id"`
	ElementIds []uint32 `json:"element_ids"`
}

type testPresentationeffectiveSlideIdXArguments struct {
	SlideId uint32 `json:"slide_id"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Text    string `json:"text"`
}

func TestPresentationGroupCapabilityUsesExplicitAdjacentElementIDs(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatPPTX)
	if err != nil {
		t.Fatalf("CreateDocument(PPTX): %v", err)
	}
	inspection, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID})
	if err != nil {
		t.Fatalf("inspect blank slide: %v", err)
	}
	var slideData struct {
		Slides []struct {
			ID uint32 `json:"slide_id"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(inspection.Content, &slideData); err != nil || len(slideData.Slides) != 1 || slideData.Slides[0].ID == 0 {
		t.Fatalf("blank slide inspection=%s error=%v", inspection.Content, err)
	}
	slideID := slideData.Slides[0].ID
	operator := Operator{ID: "pptx-group-test", Kind: OperatorBuiltInAgent}
	revision := doc.Revision
	var elementIDs []uint32
	for index, text := range []string{"第一项", "第二项"} {
		created, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: "pptx-group-seed-" + string(rune('a'+index)), DocumentID: doc.DocumentID,
			CapabilityID: "presentation.element.textbox.create", ExpectedRevision: revision, Operator: operator,
			Arguments: mustJSON(t, testPresentationeffectiveSlideIdXArguments{SlideId: slideID, X: 80 + index*140, Y: 100, Width: 120, Height: 48, Text: text}),
		})
		if invokeErr != nil {
			t.Fatalf("create group child %d: %v", index, invokeErr)
		}
		var element struct {
			ElementID uint32 `json:"element_id"`
		}
		if err := json.Unmarshal(created.Data, &element); err != nil || element.ElementID == 0 {
			t.Fatalf("create element result=%s error=%v", created.Data, err)
		}
		elementIDs = append(elementIDs, element.ElementID)
		revision = created.Revision
	}
	beforeGroup := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	pending, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-group-set", DocumentID: doc.DocumentID, CapabilityID: "presentation.element.group.set",
		ExpectedRevision: revision, Operator: operator,
		Arguments: mustJSON(t, testPresentationeffectiveSlideIdElementIdsArguments{SlideId: slideID, ElementIds: elementIDs}),
	})
	if err != nil {
		t.Fatalf("stage grouping adjacent elements: %v", err)
	}
	var staged struct {
		Status         string `json:"status"`
		ConfirmationID string `json:"confirmation_id"`
		CapabilityID   string `json:"capability_id"`
	}
	if err := json.Unmarshal(pending.Data, &staged); err != nil || staged.Status != "confirmation_required" || staged.ConfirmationID == "" || staged.CapabilityID != "presentation.element.group.set" {
		t.Fatalf("grouping request was not staged for human review: data=%s error=%v", pending.Data, err)
	}
	if afterStage := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterStage.Revision != beforeGroup.Revision || afterStage.Dirty != beforeGroup.Dirty {
		t.Fatalf("grouping changed document before approval: before=%+v after=%+v", beforeGroup, afterStage)
	}
	if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, Operator{ID: "pptx-group-reviewer", Kind: OperatorHuman}); err != nil {
		t.Fatalf("approve grouping: %v", err)
	}
	completed := waitForConfirmationStatus(t, runtime, staged.ConfirmationID, ConfirmationCompleted)
	if completed.Result == nil {
		t.Fatalf("approved grouping has no command result: %+v", completed)
	}
	var groupResult struct {
		Changed    bool     `json:"changed"`
		GroupID    uint32   `json:"group_id"`
		ElementIDs []uint32 `json:"element_ids"`
	}
	if err := json.Unmarshal(completed.Result.Data, &groupResult); err != nil || !groupResult.Changed || groupResult.GroupID == 0 || len(groupResult.ElementIDs) != 2 || completed.Result.Revision != revision+1 {
		t.Fatalf("approved group result=%s revision=%d error=%v", completed.Result.Data, completed.Result.Revision, err)
	}
	_, err = runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-group-invalid-child", DocumentID: doc.DocumentID, CapabilityID: "presentation.element.group.set",
		ExpectedRevision: completed.Result.Revision, Operator: Operator{ID: "pptx-group-invalid-target-test", Kind: OperatorHuman},
		Arguments: mustJSON(t, testPresentationeffectiveSlideIdElementIdsArguments{SlideId: slideID, ElementIds: []uint32{elementIDs[0], 4294967295}}),
	})
	if err == nil {
		t.Fatal("group operation accepted a nonexistent element_id")
	}
	state, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || state.Revision != completed.Result.Revision || !state.Dirty {
		t.Fatalf("invalid group target changed document state: state=%+v error=%v", state, err)
	}

	path := filepath.Join(t.TempDir(), "presentation-group.pptx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("save grouped presentation: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close grouped presentation: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen grouped presentation: %v", err)
	}
	inspection, err = runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: reopened.DocumentID})
	if err != nil {
		t.Fatalf("inspect grouped presentation after reopen: %v", err)
	}
	var after struct {
		Slides []struct {
			Elements []struct {
				ID      uint32 `json:"id"`
				GroupID uint32 `json:"group_id"`
				Text    string `json:"text"`
			} `json:"elements"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(inspection.Content, &after); err != nil || len(after.Slides) != 1 {
		t.Fatalf("grouped presentation inspection=%s error=%v", inspection.Content, err)
	}
	var retained, retainedText bool
	for _, element := range after.Slides[0].Elements {
		retained = retained || element.ID == uint32(elementIDs[0]) && element.GroupID == groupResult.GroupID
		retainedText = retainedText || element.Text == "第一项"
	}
	if !retained || !retainedText {
		t.Fatalf("saved group or child text was not retained: %+v", after.Slides[0].Elements)
	}
}

type testPresentationeffectiveSlideIdXYArguments struct {
	SlideId uint32 `json:"slide_id"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Text    string `json:"text"`
}

func TestPresentationExportAndPrintCapabilitiesUseRealSnapshot(t *testing.T) {
	ctx := context.Background()
	printer := &capturingPrintService{}
	runtime, err := NewRuntime(ctx, RuntimeOptions{PrintService: printer})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatPPTX)
	if err != nil {
		t.Fatalf("CreateDocument(PPTX): %v", err)
	}
	inspection, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID})
	if err != nil {
		t.Fatalf("inspect presentation: %v", err)
	}
	var slideData struct {
		Slides []struct {
			ID uint32 `json:"slide_id"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(inspection.Content, &slideData); err != nil || len(slideData.Slides) != 1 {
		t.Fatalf("presentation inspection=%s error=%v", inspection.Content, err)
	}
	created, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-export-seed", DocumentID: doc.DocumentID, CapabilityID: "presentation.element.textbox.create",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "presentation-export-agent", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, testPresentationeffectiveSlideIdXYArguments{SlideId: slideData.Slides[0].ID, X: 72, Y: 72, Width: 360, Height: 64, Text: "季度报告导出验证"}),
	})
	if err != nil {
		t.Fatalf("seed presentation export: %v", err)
	}
	before, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before export: %v", err)
	}
	if before.Revision != created.Revision {
		t.Fatalf("live revision after export seed = %d, command returned %d", before.Revision, created.Revision)
	}
	path := filepath.Join(t.TempDir(), "presentation.pdf")
	exported, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-export-pdf", DocumentID: doc.DocumentID, CapabilityID: "presentation.document.export_pdf",
		ExpectedRevision: before.Revision, Operator: Operator{ID: "presentation-export-agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]string{"target_path": path, "orientation": "landscape"}),
	})
	if err != nil {
		t.Fatalf("export presentation PDF: %v", err)
	}
	var exportResult struct {
		Path         string   `json:"path"`
		BytesWritten int64    `json:"bytes_written"`
		PageCount    int      `json:"page_count"`
		Warnings     []string `json:"warnings"`
	}
	if err := json.Unmarshal(exported.Data, &exportResult); err != nil || exportResult.Path != canonicalPath(path) || exportResult.BytesWritten <= 0 || exportResult.PageCount != 1 {
		t.Fatalf("presentation PDF result=%s error=%v", exported.Data, err)
	}
	pdf, err := gopdf.Open(path)
	if err != nil || pdf.Repaired() || pdf.NumPages() != 1 {
		t.Fatalf("exported presentation PDF pages=%v repaired=%v error=%v", pdf, pdf != nil && pdf.Repaired(), err)
	}
	afterExport, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || afterExport.Revision != before.Revision || afterExport.Dirty != before.Dirty || afterExport.SourcePath != before.SourcePath {
		t.Fatalf("export changed presentation state: before=%+v after=%+v error=%v", before, afterExport, err)
	}

	printed, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-print", DocumentID: doc.DocumentID, CapabilityID: "presentation.document.print",
		ExpectedRevision: before.Revision, Operator: Operator{ID: "human-printer", Kind: OperatorHuman},
		Arguments: mustJSON(t, map[string]int{"copies": 2, "first_page": 1, "last_page": 1}),
	})
	if err != nil {
		t.Fatalf("print presentation snapshot: %v", err)
	}
	if printer.Calls != 1 || printer.Options != (PrintOptions{Copies: 2, FirstPage: 1, LastPage: 1}) || len(printer.Data) == 0 {
		t.Fatalf("presentation print service received calls=%d options=%+v PDF bytes=%d", printer.Calls, printer.Options, len(printer.Data))
	}
	var printResult struct {
		Status      string `json:"status"`
		PageCount   int    `json:"page_count"`
		Copies      int    `json:"copies"`
		FirstPage   int    `json:"first_page"`
		LastPage    int    `json:"last_page"`
		Orientation string `json:"orientation"`
	}
	if err := json.Unmarshal(printed.Data, &printResult); err != nil || printResult.Status != "sent_to_system_print_queue" || printResult.PageCount != 1 || printResult.Copies != 2 || printResult.FirstPage != 1 || printResult.LastPage != 1 {
		t.Fatalf("presentation print result=%s error=%v", printed.Data, err)
	}
}

func TestPDFPrintCapabilityUsesCapturedPrinterAndPreservesRevision(t *testing.T) {
	ctx := context.Background()
	printer := &capturingPrintService{}
	runtime, err := NewRuntime(ctx, RuntimeOptions{PrintService: printer})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatPDF)
	if err != nil {
		t.Fatalf("CreateDocument(PDF): %v", err)
	}
	before, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before print: %v", err)
	}
	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pdf-print-range", DocumentID: doc.DocumentID, CapabilityID: "pdf.document.print",
		ExpectedRevision: before.Revision, Operator: Operator{ID: "human-printer", Kind: OperatorHuman},
		Arguments: mustJSON(t, map[string]int{"copies": 3, "first_page": 1, "last_page": 1}),
	})
	if err != nil {
		t.Fatalf("print PDF document: %v", err)
	}
	var output struct {
		Status        string `json:"status"`
		PrintedRev    uint64 `json:"printed_revision"`
		Copies        int    `json:"copies"`
		PageSelection string `json:"page_selection"`
		FirstPage     int    `json:"first_page"`
		LastPage      int    `json:"last_page"`
	}
	if err := json.Unmarshal(result.Data, &output); err != nil || output.Status != "sent_to_system_print_queue" || output.PrintedRev != before.Revision || output.Copies != 3 || output.PageSelection != "range" || output.FirstPage != 1 || output.LastPage != 1 {
		t.Fatalf("PDF print result=%s error=%v", result.Data, err)
	}
	if printer.Calls != 1 || printer.Options != (PrintOptions{Copies: 3, FirstPage: 1, LastPage: 1}) || len(printer.Data) == 0 {
		t.Fatalf("PDF print service received calls=%d options=%+v bytes=%d", printer.Calls, printer.Options, len(printer.Data))
	}
	if err := os.WriteFile(filepath.Join(t.TempDir(), "captured.pdf"), printer.Data, 0o600); err != nil {
		t.Fatalf("write captured PDF: %v", err)
	}
	after, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || after.Revision != before.Revision || after.Dirty != before.Dirty {
		t.Fatalf("PDF print changed document: before=%+v after=%+v error=%v", before, after, err)
	}
}

type capturingPrintService struct {
	Calls   int
	Data    []byte
	Options PrintOptions
}

func (*capturingPrintService) Available() bool { return true }

func (printer *capturingPrintService) PrintPDF(_ context.Context, data []byte, options PrintOptions) error {
	printer.Calls++
	printer.Data = append([]byte(nil), data...)
	printer.Options = options
	return nil
}
