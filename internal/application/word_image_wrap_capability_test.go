package application

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordImageWrapCapabilityRequiresHumanAndPreservesImage(t *testing.T) {
	ctx := context.Background()
	fixturePath, imageID, originalImage, originalBytes := makeFloatingWordImageFixture(t)

	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, fixturePath)
	if err != nil {
		t.Fatalf("OpenDocument floating-image fixture: %v", err)
	}
	documentID := doc.DocumentID
	requestNumber := 0
	stageWrap := func() string {
		t.Helper()
		requestNumber++
		info := mustGetDocumentInfo(t, runtime, documentID)
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("word-image-wrap-%d", requestNumber), DocumentID: documentID,
			CapabilityID: "word.image.wrap.set", ExpectedRevision: info.Revision,
			Operator:  Operator{ID: "wrap-agent-test", Kind: OperatorExternalAgent},
			Arguments: mustJSON(t, map[string]any{"image_id": imageID, "wrap_mode": "inline"}),
		})
		if invokeErr != nil {
			t.Fatalf("stage Word image wrap: %v", invokeErr)
		}
		var staged struct {
			Status         string `json:"status"`
			ConfirmationID string `json:"confirmation_id"`
		}
		if err := json.Unmarshal(result.Data, &staged); err != nil || staged.Status != "confirmation_required" || staged.ConfirmationID == "" {
			t.Fatalf("Word image wrap did not require human confirmation: %+v data=%s err=%v", staged, result.Data, err)
		}
		if afterStage := mustGetDocumentInfo(t, runtime, documentID); afterStage.Revision != info.Revision || afterStage.Dirty != info.Dirty {
			t.Fatalf("image changed before wrap approval: before=%+v after stage=%+v", info, afterStage)
		}
		return staged.ConfirmationID
	}
	resolve := func(id string, approve bool) ConfirmationInfo {
		t.Helper()
		if err := runtime.ResolveConfirmation(ctx, id, approve, Operator{ID: "human-reviewer", Kind: OperatorHuman}); err != nil {
			t.Fatalf("resolve image wrap: %v", err)
		}
		status := ConfirmationRejected
		if approve {
			status = ConfirmationCompleted
		}
		return waitForConfirmationStatus(t, runtime, id, status)
	}
	inspectImage := func() document.WordImage {
		t.Helper()
		inspection, inspectErr := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: documentID, Limit: 1000})
		if inspectErr != nil {
			t.Fatalf("InspectDocument wrapped image: %v", inspectErr)
		}
		var output struct {
			Blocks []struct {
				ParagraphID string `json:"paragraph_id"`
				Images      []struct {
					ID           string `json:"image_id"`
					ParagraphID  string `json:"paragraph_id"`
					WidthEMU     int64  `json:"width_emu"`
					HeightEMU    int64  `json:"height_emu"`
					WrapMode     string `json:"wrap_mode"`
					WrapEditable bool   `json:"wrap_editable"`
				} `json:"images"`
			} `json:"blocks"`
		}
		if err := json.Unmarshal(inspection.Content, &output); err != nil {
			t.Fatalf("decode wrapped image inspection: %v; content=%s", err, inspection.Content)
		}
		for _, block := range output.Blocks {
			for _, image := range block.Images {
				if image.ID == imageID {
					return document.WordImage{ID: image.ID, WidthEMU: image.WidthEMU, HeightEMU: image.HeightEMU, WrapMode: image.WrapMode, WrapEditable: image.WrapEditable}
				}
			}
		}
		t.Fatalf("image %q missing from document.inspect: %s", imageID, inspection.Content)
		return document.WordImage{}
	}
	if current := inspectImage(); current.WrapMode == "inline" || !current.WrapEditable {
		t.Fatalf("fixture image should be an editable floating image: %+v", current)
	}

	rejectID := stageWrap()
	if rejected := resolve(rejectID, false); rejected.Result != nil {
		t.Fatalf("rejected wrap returned a result: %+v", rejected)
	}
	if current := inspectImage(); current.WrapMode == "inline" {
		t.Fatal("rejected image wrap changed the document")
	}
	beforeApprove := mustGetDocumentInfo(t, runtime, documentID)
	approveID := stageWrap()
	completed := resolve(approveID, true)
	if completed.Result == nil || completed.Result.Revision != beforeApprove.Revision+1 {
		t.Fatalf("approved wrap result=%+v; want one revision", completed)
	}
	var result struct {
		Changed          bool   `json:"changed"`
		ImageID          string `json:"image_id"`
		PreviousWrapMode string `json:"previous_wrap_mode"`
		WrapMode         string `json:"wrap_mode"`
	}
	if err := json.Unmarshal(completed.Result.Data, &result); err != nil || !result.Changed || result.ImageID != imageID || result.PreviousWrapMode == "inline" || result.WrapMode != "inline" {
		t.Fatalf("approved wrap result=%+v data=%s err=%v", result, completed.Result.Data, err)
	}
	wrapped := inspectImage()
	if wrapped.WrapMode != "inline" || wrapped.WidthEMU != originalImage.WidthEMU || wrapped.HeightEMU != originalImage.HeightEMU {
		t.Fatalf("wrapped image metadata=%+v original=%+v", wrapped, originalImage)
	}

	beforeNoop := mustGetDocumentInfo(t, runtime, documentID)
	noopID := stageWrap()
	noop := resolve(noopID, true)
	if noop.Result == nil {
		t.Fatalf("repeated inline wrap returned no result: %+v", noop)
	}
	var noopResult struct {
		Changed          bool   `json:"changed"`
		PreviousWrapMode string `json:"previous_wrap_mode"`
	}
	if err := json.Unmarshal(noop.Result.Data, &noopResult); err != nil || noopResult.Changed || noopResult.PreviousWrapMode != "inline" {
		t.Fatalf("repeated inline wrap result=%+v data=%s err=%v", noopResult, noop.Result.Data, err)
	}
	if afterNoop := mustGetDocumentInfo(t, runtime, documentID); afterNoop.Revision != beforeNoop.Revision || afterNoop.Dirty != beforeNoop.Dirty || noop.Result.Revision != beforeNoop.Revision {
		t.Fatalf("inline no-op changed document state: before=%+v after=%+v result=%+v", beforeNoop, afterNoop, noop.Result)
	}

	path := filepath.Join(t.TempDir(), "inline-image.docx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, path); err != nil {
		t.Fatalf("SaveDocumentAs converted image: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("CloseDocument converted image: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen converted image: %v", err)
	}
	defer reopened.Close()
	var reopenedImage *document.WordImage
	for _, block := range reopened.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Image != nil && inline.Image.ID == imageID {
				reopenedImage = inline.Image
			}
		}
	}
	if reopenedImage == nil || reopenedImage.WrapMode != "inline" || reopenedImage.WidthEMU != originalImage.WidthEMU || reopenedImage.HeightEMU != originalImage.HeightEMU {
		t.Fatalf("converted image after reopen=%+v original=%+v", reopenedImage, originalImage)
	}
	reopenedBytes, err := reopened.WordImageBytes(imageID)
	if err != nil || !bytes.Equal(reopenedBytes, originalBytes) {
		t.Fatalf("image bytes changed during wrap conversion: size=%d want=%d err=%v", len(reopenedBytes), len(originalBytes), err)
	}
}

func makeFloatingWordImageFixture(t *testing.T) (string, string, document.WordImage, []byte) {
	t.Helper()
	imagePath := writeApplicationWordImageFixture(t, "floating.png", "png")
	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("read PNG fixture: %v", err)
	}
	source, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create blank DOCX fixture: %v", err)
	}
	paragraphID, runID, err := source.AppendBodyTextParagraph("Floating image fixture")
	if err != nil {
		_ = source.Close()
		t.Fatalf("append fixture paragraph: %v", err)
	}
	imageID, err := source.InsertInlineImage(paragraphID, runID, "image/png", imageBytes)
	if err != nil {
		_ = source.Close()
		t.Fatalf("insert fixture image: %v", err)
	}
	var metadata document.WordImage
	for _, block := range source.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Image != nil && inline.Image.ID == imageID {
				metadata = *inline.Image
			}
		}
	}
	packageBytes, err := source.Snapshot()
	_ = source.Close()
	if err != nil {
		t.Fatalf("snapshot inline fixture: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(packageBytes), int64(len(packageBytes)))
	if err != nil {
		t.Fatalf("open inline DOCX fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "floating-image.docx")
	output, err := os.Create(path)
	if err != nil {
		t.Fatalf("create floating DOCX fixture: %v", err)
	}
	writer := zip.NewWriter(output)
	for _, file := range reader.File {
		entry, openErr := file.Open()
		if openErr != nil {
			t.Fatalf("open fixture part %s: %v", file.Name, openErr)
		}
		data, readErr := io.ReadAll(entry)
		closeErr := entry.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read fixture part %s: read=%v close=%v", file.Name, readErr, closeErr)
		}
		if file.Name == "word/document.xml" {
			data = anchorWordImageXML(t, data)
		}
		part, createErr := writer.Create(file.Name)
		if createErr != nil {
			t.Fatalf("create fixture part %s: %v", file.Name, createErr)
		}
		if _, writeErr := part.Write(data); writeErr != nil {
			t.Fatalf("write fixture part %s: %v", file.Name, writeErr)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close floating DOCX archive: %v", err)
	}
	if err := output.Close(); err != nil {
		t.Fatalf("close floating DOCX fixture: %v", err)
	}
	check, err := docx.Open(path)
	if err != nil {
		t.Fatalf("validate floating DOCX fixture: %v", err)
	}
	defer check.Close()
	for _, block := range check.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Image != nil && inline.Image.ID == imageID {
				if !inline.Image.WrapEditable || inline.Image.WrapMode == "inline" {
					t.Fatalf("generated fixture image is not an editable float: %+v", inline.Image)
				}
				metadata.WrapMode = inline.Image.WrapMode
				metadata.WrapEditable = inline.Image.WrapEditable
				return path, imageID, metadata, imageBytes
			}
		}
	}
	t.Fatalf("generated floating DOCX is missing image %q", imageID)
	return "", "", document.WordImage{}, nil
}

func anchorWordImageXML(t *testing.T, data []byte) []byte {
	t.Helper()
	openingStart := bytes.Index(data, []byte("<wp:inline"))
	if openingStart < 0 {
		t.Fatal("fixture document XML has no wp:inline image")
	}
	openingEndRelative := bytes.IndexByte(data[openingStart:], '>')
	if openingEndRelative < 0 {
		t.Fatal("fixture wp:inline opening tag is incomplete")
	}
	openingEnd := openingStart + openingEndRelative + 1
	closingRelative := bytes.Index(data[openingEnd:], []byte("</wp:inline>"))
	if closingRelative < 0 {
		t.Fatal("fixture wp:inline has no closing tag")
	}
	closingStart := openingEnd + closingRelative
	closingEnd := closingStart + len("</wp:inline>")
	inner := data[openingEnd:closingStart]
	anchor := []byte(`<wp:anchor distT="0" distB="0" distL="0" distR="0" simplePos="0" relativeHeight="0" behindDoc="0" locked="0" layoutInCell="1" allowOverlap="1"><wp:simplePos x="0" y="0"/><wp:positionH relativeFrom="column"><wp:posOffset>0</wp:posOffset></wp:positionH><wp:positionV relativeFrom="paragraph"><wp:posOffset>0</wp:posOffset></wp:positionV>`)
	anchor = append(anchor, inner...)
	anchor = append(anchor, []byte(`<wp:wrapSquare wrapText="bothSides"/></wp:anchor>`)...)
	updated := make([]byte, 0, len(data)+len(anchor)-(closingEnd-openingStart))
	updated = append(updated, data[:openingStart]...)
	updated = append(updated, anchor...)
	updated = append(updated, data[closingEnd:]...)
	if bytes.Contains(updated, []byte("<wp:inline")) || strings.Count(string(updated), "<wp:anchor") != 1 {
		t.Fatal("fixture inline image was not converted to exactly one floating anchor")
	}
	return updated
}
