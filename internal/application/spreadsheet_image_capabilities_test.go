package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"testing"
)

type testSpreadsheetimageSheetImageIdArguments struct {
	Sheet   string `json:"sheet"`
	ImageId string `json:"image_id"`
}

type testSpreadsheetimageSheetImageIdMediaTypeArguments struct {
	Sheet       string `json:"sheet"`
	ImageId     string `json:"image_id"`
	MediaType   string `json:"media_type"`
	ImageBase64 string `json:"image_base64"`
}

type spreadsheetImageResizePayload struct {
	Sheet   string `json:"sheet"`
	ImageId string `json:"image_id"`
	WidthPx int    `json:"width_px"`
}

type spreadsheetImageMovePayload struct {
	Sheet   string `json:"sheet"`
	ImageId string `json:"image_id"`
	Row     int    `json:"row"`
	Column  int    `json:"column"`
	OffsetX int    `json:"offset_x"`
	OffsetY int    `json:"offset_y"`
}

type spreadsheetImageInsertPayload struct {
	Sheet       string `json:"sheet"`
	Row         int    `json:"row"`
	Column      int    `json:"column"`
	OffsetX     int    `json:"offset_x"`
	OffsetY     int    `json:"offset_y"`
	WidthPx     int    `json:"width_px"`
	HeightPx    int    `json:"height_px"`
	MediaType   string `json:"media_type"`
	Name        string `json:"name"`
	AltText     string `json:"alt_text"`
	ImageBase64 string `json:"image_base64"`
}

type spreadsheetImagesListPayload struct {
	Sheet string `json:"sheet"`
}

func TestSpreadsheetImageCapabilitiesRequireHumanForDestructiveEditsAndRoundTrip(t *testing.T) {
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
			RequestID: fmt.Sprintf("xlsx-image-%d", requestNumber), DocumentID: documentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "image-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, arguments),
		})
	}
	listImages := func() []struct {
		ImageID   string `json:"image_id"`
		Cell      string `json:"cell"`
		Row       int    `json:"row"`
		Column    int    `json:"column"`
		WidthPx   int    `json:"width_px"`
		HeightPx  int    `json:"height_px"`
		MediaType string `json:"media_type"`
	} {
		t.Helper()
		result, invokeErr := invoke("spreadsheet.images.list", spreadsheetImagesListPayload{Sheet: "Sheet1"})
		if invokeErr != nil {
			t.Fatalf("InvokeCapability(spreadsheet.images.list): %v", invokeErr)
		}
		var output struct {
			Images []struct {
				ImageID   string `json:"image_id"`
				Cell      string `json:"cell"`
				Row       int    `json:"row"`
				Column    int    `json:"column"`
				WidthPx   int    `json:"width_px"`
				HeightPx  int    `json:"height_px"`
				MediaType string `json:"media_type"`
			} `json:"images"`
			Truncated bool `json:"truncated"`
		}
		if err := json.Unmarshal(result.Data, &output); err != nil {
			t.Fatalf("decode image list: %v; data=%s", err, result.Data)
		}
		if output.Truncated {
			t.Fatal("small image fixture unexpectedly truncated")
		}
		return output.Images
	}
	assertChanged := func(capabilityID string, arguments any, want bool) CapabilityInvocationResult {
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
		after := mustGetDocumentInfo(t, runtime, documentID)
		if output.Changed != want {
			t.Fatalf("%s changed=%v; want %v; data=%s", capabilityID, output.Changed, want, result.Data)
		}
		if want {
			if result.Revision != before.Revision+1 || after.Revision != result.Revision || !after.Dirty {
				t.Fatalf("%s mutation state: before=%+v result=%+v after=%+v", capabilityID, before, result, after)
			}
		} else if result.Revision != before.Revision || after.Revision != before.Revision || after.Dirty != before.Dirty {
			t.Fatalf("%s no-op changed document state: before=%+v result=%+v after=%+v", capabilityID, before, result, after)
		}
		return result
	}
	stageHighRisk := func(capabilityID string, arguments any) (string, DocumentInfo) {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, documentID)
		result, invokeErr := invoke(capabilityID, arguments)
		if invokeErr != nil {
			t.Fatalf("stage %s: %v", capabilityID, invokeErr)
		}
		var staged struct {
			Status         string `json:"status"`
			ConfirmationID string `json:"confirmation_id"`
		}
		if err := json.Unmarshal(result.Data, &staged); err != nil {
			t.Fatalf("decode staged %s: %v; data=%s", capabilityID, err, result.Data)
		}
		if staged.Status != "confirmation_required" || staged.ConfirmationID == "" {
			t.Fatalf("%s did not require human confirmation: %+v; data=%s", capabilityID, staged, result.Data)
		}
		if pending := mustGetDocumentInfo(t, runtime, documentID); pending.Revision != before.Revision || pending.Dirty != before.Dirty {
			t.Fatalf("%s mutated before confirmation: before=%+v pending=%+v", capabilityID, before, pending)
		}
		return staged.ConfirmationID, before
	}
	resolve := func(id string, approve bool) ConfirmationInfo {
		t.Helper()
		if err := runtime.ResolveConfirmation(ctx, id, approve, Operator{ID: "human-reviewer", Kind: OperatorHuman}); err != nil {
			t.Fatalf("ResolveConfirmation(%s, %v): %v", id, approve, err)
		}
		status := ConfirmationRejected
		if approve {
			status = ConfirmationCompleted
		}
		return waitForConfirmationStatus(t, runtime, id, status)
	}

	if got := listImages(); len(got) != 0 {
		t.Fatalf("new workbook images = %d; want 0", len(got))
	}
	initial := mustGetDocumentInfo(t, runtime, documentID)
	pngBytes := spreadsheetCapabilityPNG(t, 20, 10, color.NRGBA{R: 20, G: 90, B: 180, A: 255})
	inserted := assertChanged("spreadsheet.image.insert", spreadsheetImageInsertPayload{Sheet: "Sheet1", Row: 2, Column: 2, OffsetX: 3, OffsetY: 4, WidthPx: 40, HeightPx: 20, MediaType: "image/png", Name: "Agent chart", AltText: "quarterly chart", ImageBase64: base64.StdEncoding.EncodeToString(pngBytes)}, true)
	var insertedResult struct {
		ImageID string `json:"image_id"`
		Cell    string `json:"cell"`
	}
	if err := json.Unmarshal(inserted.Data, &insertedResult); err != nil || insertedResult.ImageID == "" || insertedResult.Cell != "B2" {
		t.Fatalf("insert result=%+v data=%s err=%v", insertedResult, inserted.Data, err)
	}
	if initial.Revision+1 != inserted.Revision {
		t.Fatalf("insert revision=%d; initial=%d", inserted.Revision, initial.Revision)
	}
	images := listImages()
	if len(images) != 1 || images[0].ImageID != insertedResult.ImageID || images[0].Cell != "B2" || images[0].WidthPx != 40 || images[0].HeightPx != 20 || images[0].MediaType != "image/png" {
		t.Fatalf("inserted image metadata = %+v; command result=%+v", images, insertedResult)
	}

	imageID := images[0].ImageID
	beforeInvalidTarget := mustGetDocumentInfo(t, runtime, documentID)
	if _, err := invoke("spreadsheet.image.move", spreadsheetImageMovePayload{Sheet: "Sheet1", ImageId: "stale-image-id", Row: 5, Column: 4, OffsetX: 0, OffsetY: 0}); err == nil {
		t.Fatal("move accepted a stale image ID")
	}
	if afterInvalidTarget := mustGetDocumentInfo(t, runtime, documentID); afterInvalidTarget.Revision != beforeInvalidTarget.Revision || afterInvalidTarget.Dirty != beforeInvalidTarget.Dirty {
		t.Fatalf("invalid image target partially changed workbook: before=%+v after=%+v", beforeInvalidTarget, afterInvalidTarget)
	}
	assertChanged("spreadsheet.image.move", spreadsheetImageMovePayload{Sheet: "Sheet1", ImageId: imageID, Row: 2, Column: 2, OffsetX: 3, OffsetY: 4}, false)
	assertChanged("spreadsheet.image.resize", spreadsheetImageResizePayload{Sheet: "Sheet1", ImageId: imageID, WidthPx: 40}, false)
	assertChanged("spreadsheet.image.move", spreadsheetImageMovePayload{Sheet: "Sheet1", ImageId: imageID, Row: 5, Column: 4, OffsetX: 5, OffsetY: 7}, true)
	images = listImages()
	if len(images) != 1 || images[0].Cell != "D5" || images[0].WidthPx != 40 || images[0].HeightPx != 20 {
		t.Fatalf("moved image metadata = %+v", images)
	}
	imageID = images[0].ImageID
	assertChanged("spreadsheet.image.resize", spreadsheetImageResizePayload{Sheet: "Sheet1", ImageId: imageID, WidthPx: 80}, true)
	images = listImages()
	if len(images) != 1 || images[0].Cell != "D5" || images[0].WidthPx != 80 || images[0].HeightPx != 40 {
		t.Fatalf("resized image metadata = %+v; want 80x40 with preserved aspect ratio", images)
	}
	imageID = images[0].ImageID
	assertChanged("spreadsheet.image.resize", spreadsheetImageResizePayload{Sheet: "Sheet1", ImageId: imageID, WidthPx: 80}, false)

	replacement := spreadsheetCapabilityJPEG(t, 16, 8, color.NRGBA{R: 210, G: 60, B: 30, A: 255})
	replacementArgs := testSpreadsheetimageSheetImageIdMediaTypeArguments{Sheet: "Sheet1", ImageId: imageID, MediaType: "image/jpeg", ImageBase64: base64.StdEncoding.EncodeToString(replacement)}
	rejectID, beforeReject := stageHighRisk("spreadsheet.image.replace", replacementArgs)
	if info := resolve(rejectID, false); info.Result != nil {
		t.Fatalf("rejected replace unexpectedly returned result: %+v", info)
	}
	if rejected := mustGetDocumentInfo(t, runtime, documentID); rejected.Revision != beforeReject.Revision || rejected.Dirty != beforeReject.Dirty {
		t.Fatalf("rejected replace mutated workbook: before=%+v after=%+v", beforeReject, rejected)
	}
	if afterReject := listImages(); len(afterReject) != 1 || afterReject[0].MediaType != "image/png" {
		t.Fatalf("rejected replacement changed image: %+v", afterReject)
	}

	approveReplaceID, beforeReplace := stageHighRisk("spreadsheet.image.replace", replacementArgs)
	completedReplace := resolve(approveReplaceID, true)
	if completedReplace.Result == nil || completedReplace.Result.Revision != beforeReplace.Revision+1 {
		t.Fatalf("approved replace result=%+v; want one revision", completedReplace)
	}
	if replaced := listImages(); len(replaced) != 1 || replaced[0].MediaType != "image/jpeg" || replaced[0].Cell != "D5" || replaced[0].WidthPx != 80 || replaced[0].HeightPx != 40 {
		t.Fatalf("approved replacement metadata = %+v", replaced)
	}

	path := filepath.Join(t.TempDir(), "agent-images.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs after image replacement: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument after image replacement: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after image replacement: %v", err)
	}
	documentID = reopened.DocumentID
	if reopened.Dirty {
		t.Fatalf("reopened workbook is dirty: %+v", reopened)
	}
	reopenedImages := listImages()
	if len(reopenedImages) != 1 || reopenedImages[0].MediaType != "image/jpeg" || reopenedImages[0].Cell != "D5" || reopenedImages[0].WidthPx != 80 || reopenedImages[0].HeightPx != 40 {
		t.Fatalf("image metadata after save/reopen = %+v", reopenedImages)
	}
	session, err := runtime.session(documentID)
	if err != nil {
		t.Fatalf("get reopened workbook session: %v", err)
	}
	bytesReader, ok := session.adapter.(interface {
		ReadSpreadsheetImageBytes(context.Context, string) ([]byte, error)
	})
	if !ok {
		t.Fatal("reopened spreadsheet adapter does not expose image bytes")
	}
	reopenedBytes, err := bytesReader.ReadSpreadsheetImageBytes(ctx, reopenedImages[0].ImageID)
	if err != nil || !bytes.Equal(reopenedBytes, replacement) {
		t.Fatalf("replacement image bytes after save/reopen differ: err=%v size=%d want=%d", err, len(reopenedBytes), len(replacement))
	}

	deleteArgs := testSpreadsheetimageSheetImageIdArguments{Sheet: "Sheet1", ImageId: reopenedImages[0].ImageID}
	deleteID, beforeDelete := stageHighRisk("spreadsheet.image.delete", deleteArgs)
	completedDelete := resolve(deleteID, true)
	if completedDelete.Result == nil || completedDelete.Result.Revision != beforeDelete.Revision+1 {
		t.Fatalf("approved delete result=%+v; want one revision", completedDelete)
	}
	if remaining := listImages(); len(remaining) != 0 {
		t.Fatalf("images after approved delete = %+v; want none", remaining)
	}
}

func spreadsheetCapabilityPNG(t *testing.T, width, height int, fill color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, fill)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatalf("encode PNG fixture: %v", err)
	}
	return data.Bytes()
}

func spreadsheetCapabilityJPEG(t *testing.T, width, height int, fill color.NRGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, fill)
		}
	}
	var data bytes.Buffer
	if err := jpeg.Encode(&data, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode JPEG fixture: %v", err)
	}
	return data.Bytes()
}
