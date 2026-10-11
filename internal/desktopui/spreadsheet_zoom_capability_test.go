package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"glance/internal/application"
	"glance/internal/format/xlsx"
)

type spreadsheetZoomCapabilityTestViewHandler struct{ app *App }

func (h spreadsheetZoomCapabilityTestViewHandler) HandleViewIntent(_ context.Context, intent application.ViewIntent) (json.RawMessage, error) {
	return h.app.applyDesktopViewIntent(intent)
}

func TestSpreadsheetZoomCapabilitiesRouteToAttachedWindowWithoutDocumentMutation(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	book := xlsx.NewBlank()
	if err := app.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatalf("install workbook: %v", err)
	}
	desktop.attachWindow(7, app)
	documentID := app.documentSessionState.documentID
	initial, err := desktop.applicationRuntime.GetDocument(ctx, documentID)
	if err != nil {
		t.Fatalf("get initial document: %v", err)
	}

	requestID := 0
	invoke := func(windowID uint64, capabilityID, arguments string) (application.CapabilityInvocationResult, error) {
		requestID++
		return desktop.applicationRuntime.InvokeCapability(ctx, application.CapabilityRequest{
			RequestID: fmt.Sprintf("spreadsheet-zoom-%d", requestID), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: initial.Revision,
			Operator:  application.Operator{ID: "zoom-agent", Kind: application.OperatorExternalAgent},
			View:      application.ViewContext{WindowID: windowID, DocumentID: documentID, Revision: initial.Revision},
			Arguments: json.RawMessage(arguments),
		})
	}

	if _, err := invoke(7, "spreadsheet.zoom.get", `{}`); !errors.Is(err, application.ErrNoAttachedView) {
		t.Fatalf("zoom query without a registered attached view error = %v; want no attached view", err)
	}
	unregister := desktop.applicationRuntime.RegisterViewIntentHandler(7, spreadsheetZoomCapabilityTestViewHandler{app: app})
	defer unregister()
	if _, err := invoke(8, "spreadsheet.zoom.apply", `{"operation":"set","zoom_percent":125}`); !errors.Is(err, application.ErrNoAttachedView) {
		t.Fatalf("zoom mutation for another window error = %v; want no attached view", err)
	}

	type zoomResult struct {
		WindowID    uint64 `json:"window_id"`
		ZoomPercent int    `json:"zoom_percent"`
		Changed     bool   `json:"changed"`
	}
	decode := func(result application.CapabilityInvocationResult, err error) zoomResult {
		t.Helper()
		if err != nil {
			t.Fatalf("invoke zoom capability: %v", err)
		}
		var got zoomResult
		if err := json.Unmarshal(result.Data, &got); err != nil {
			t.Fatalf("decode zoom result %s: %v", result.Data, err)
		}
		return got
	}
	got := decode(invoke(7, "spreadsheet.zoom.get", `{}`))
	if got.WindowID != 7 || got.ZoomPercent != 100 || got.Changed {
		t.Fatalf("initial zoom = %+v; want window 7 at 100%% with changed=false", got)
	}
	if _, err := invoke(7, "spreadsheet.zoom.apply", `{"operation":"set","zoom_percent":201}`); !errors.Is(err, application.ErrInvalidCommandArguments) {
		t.Fatalf("out-of-range zoom error = %v; want invalid command arguments", err)
	}
	got = decode(invoke(7, "spreadsheet.zoom.apply", `{"operation":"set","zoom_percent":125}`))
	if got.ZoomPercent != 125 || !got.Changed || app.spreadsheetZoom() != 1.25 {
		t.Fatalf("set zoom result/state = %+v / %v; want 125%% and changed=true", got, app.spreadsheetZoom())
	}
	got = decode(invoke(7, "spreadsheet.zoom.apply", `{"operation":"set","zoom_percent":125}`))
	if got.ZoomPercent != 125 || got.Changed {
		t.Fatalf("repeated zoom set = %+v; want changed=false", got)
	}
	got = decode(invoke(7, "spreadsheet.zoom.apply", `{"operation":"zoom_in"}`))
	if got.ZoomPercent != 135 || !got.Changed {
		t.Fatalf("zoom in result = %+v; want 135%% and changed=true", got)
	}
	got = decode(invoke(7, "spreadsheet.zoom.apply", `{"operation":"reset"}`))
	if got.ZoomPercent != 100 || !got.Changed {
		t.Fatalf("zoom reset result = %+v; want 100%% and changed=true", got)
	}
	final, err := desktop.applicationRuntime.GetDocument(ctx, documentID)
	if err != nil {
		t.Fatalf("get final document: %v", err)
	}
	if final.Revision != initial.Revision || final.Dirty != initial.Dirty {
		t.Fatalf("view-only zoom changed document state: initial revision/dirty=%d/%v final=%d/%v", initial.Revision, initial.Dirty, final.Revision, final.Dirty)
	}
}
