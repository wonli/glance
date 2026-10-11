package application

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordImageCapabilitiesRoundTripAndDelete(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	operator := Operator{ID: "image-agent", Kind: OperatorBuiltInAgent}
	assertState := func(wantRevision uint64, wantDirty bool) {
		t.Helper()
		state, err := runtime.GetDocument(ctx, doc.DocumentID)
		if err != nil || state.Revision != wantRevision || state.Dirty != wantDirty {
			t.Fatalf("Word image document state=%+v error=%v; want revision=%d dirty=%v", state, err, wantRevision, wantDirty)
		}
	}
	invokeConfirmed := func(requestID, commandID string, revision uint64, arguments any) CapabilityInvocationResult {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, doc.DocumentID)
		pending, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: doc.DocumentID, CapabilityID: commandID,
			ExpectedRevision: revision, Operator: operator, Arguments: mustJSON(t, arguments),
		})
		if err != nil {
			t.Fatalf("stage %s: %v", commandID, err)
		}
		var staged struct {
			Status         string `json:"status"`
			ConfirmationID string `json:"confirmation_id"`
			CapabilityID   string `json:"capability_id"`
		}
		if err := json.Unmarshal(pending.Data, &staged); err != nil || staged.Status != "confirmation_required" || staged.ConfirmationID == "" || staged.CapabilityID != commandID {
			t.Fatalf("%s did not require human confirmation: data=%s error=%v", commandID, pending.Data, err)
		}
		if afterStage := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterStage.Revision != before.Revision || afterStage.Dirty != before.Dirty {
			t.Fatalf("%s changed the document before approval: before=%+v after=%+v", commandID, before, afterStage)
		}
		if err := runtime.ResolveConfirmation(ctx, staged.ConfirmationID, true, Operator{ID: "image-human-reviewer", Kind: OperatorHuman}); err != nil {
			t.Fatalf("approve %s: %v", commandID, err)
		}
		completed := waitForConfirmationStatus(t, runtime, staged.ConfirmationID, ConfirmationCompleted)
		if completed.Result == nil {
			t.Fatalf("approved %s has no command result: %+v", commandID, completed)
		}
		return *completed.Result
	}
	assertInvalidImageRequest := func(requestID, capabilityID string, revision uint64, arguments any) {
		t.Helper()
		before := mustGetDocumentInfo(t, runtime, doc.DocumentID)
		_, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: doc.DocumentID, CapabilityID: capabilityID,
			ExpectedRevision: revision, Operator: Operator{ID: "image-target-reviewer", Kind: OperatorHuman},
			Arguments: mustJSON(t, arguments),
		})
		if err == nil {
			t.Fatalf("%s accepted invalid image target: %+v", capabilityID, arguments)
		}
		after := mustGetDocumentInfo(t, runtime, doc.DocumentID)
		if after.Revision != before.Revision || after.Dirty != before.Dirty {
			t.Fatalf("%s invalid target changed document state: before=%+v after=%+v error=%v", capabilityID, before, after, err)
		}
	}
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "append-image-target", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: operator, Arguments: mustJSON(t, map[string]any{"text": "图片说明"}),
	})
	if err != nil {
		t.Fatalf("append image target paragraph: %v", err)
	}
	var paragraph struct {
		ParagraphID string `json:"paragraph_id"`
		RunID       string `json:"run_id"`
	}
	if err := json.Unmarshal(appended.Data, &paragraph); err != nil || paragraph.ParagraphID == "" || paragraph.RunID == "" {
		t.Fatalf("decode image target IDs: %v, data=%s", err, appended.Data)
	}
	destination, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "append-image-destination", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: appended.Revision, Operator: operator, Arguments: mustJSON(t, map[string]any{"text": "目标段落"}),
	})
	if err != nil {
		t.Fatalf("append image destination paragraph: %v", err)
	}
	var destinationParagraph struct {
		ParagraphID string `json:"paragraph_id"`
		RunID       string `json:"run_id"`
	}
	if err := json.Unmarshal(destination.Data, &destinationParagraph); err != nil || destinationParagraph.ParagraphID == "" || destinationParagraph.RunID == "" {
		t.Fatalf("decode destination paragraph IDs: %v, data=%s", err, destination.Data)
	}
	pngPath := writeApplicationWordImageFixture(t, "insert.png", "png")
	jpegPath := writeApplicationWordImageFixture(t, "replacement.jpeg", "jpeg")
	inserted, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-agent-image", DocumentID: doc.DocumentID, CommandID: "word.image.insert",
		ExpectedRevision: destination.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"paragraph_id": paragraph.ParagraphID, "after_inline_id": paragraph.RunID, "path": pngPath}),
	})
	if err != nil {
		t.Fatalf("insert Agent image: %v", err)
	}
	assertState(inserted.Revision, true)
	var insertedImage struct {
		ImageID string `json:"image_id"`
	}
	if err := json.Unmarshal(inserted.Data, &insertedImage); err != nil || insertedImage.ImageID == "" {
		t.Fatalf("decode inserted image ID: %v, data=%s", err, inserted.Data)
	}
	moved, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "move-agent-image", DocumentID: doc.DocumentID, CommandID: "word.image.move",
		ExpectedRevision: inserted.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"image_id": insertedImage.ImageID, "paragraph_id": destinationParagraph.ParagraphID, "after_inline_id": destinationParagraph.RunID}),
	})
	if err != nil {
		t.Fatalf("move Agent image: %v", err)
	}
	assertState(moved.Revision, true)
	var moveResult struct {
		Changed       bool   `json:"changed"`
		ImageID       string `json:"image_id"`
		ParagraphID   string `json:"paragraph_id"`
		AfterInlineID string `json:"after_inline_id"`
	}
	if err := json.Unmarshal(moved.Data, &moveResult); err != nil || !moveResult.Changed || moveResult.ImageID != insertedImage.ImageID || moveResult.ParagraphID != destinationParagraph.ParagraphID || moveResult.AfterInlineID != destinationParagraph.RunID {
		t.Fatalf("decode moved image result: %v, data=%s", err, moved.Data)
	}
	if got := runtimeWordImageParagraph(t, runtime, doc.DocumentID, insertedImage.ImageID); got != destinationParagraph.ParagraphID {
		t.Fatalf("moved image paragraph=%q; want %q", got, destinationParagraph.ParagraphID)
	}
	moveNoop, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "move-agent-image-noop", DocumentID: doc.DocumentID, CommandID: "word.image.move",
		ExpectedRevision: moved.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"image_id": insertedImage.ImageID, "paragraph_id": destinationParagraph.ParagraphID, "after_inline_id": destinationParagraph.RunID}),
	})
	var moveNoopResult struct {
		Changed bool `json:"changed"`
	}
	if decodeErr := json.Unmarshal(moveNoop.Data, &moveNoopResult); decodeErr != nil || moveNoopResult.Changed || moveNoop.Revision != moved.Revision {
		t.Fatalf("repeating image move should be a no-op: result=%+v decoded=%+v err=%v decodeErr=%v", moveNoop, moveNoopResult, err, decodeErr)
	}
	assertState(moved.Revision, true)
	stateBeforeInvalidMove := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "move-agent-image-invalid-target", DocumentID: doc.DocumentID, CommandID: "word.image.move",
		ExpectedRevision: moved.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"image_id": insertedImage.ImageID, "paragraph_id": "missing-paragraph"}),
	}); err == nil {
		t.Fatal("moving image to a missing paragraph should fail")
	}
	if stateAfterInvalidMove := mustGetDocumentInfo(t, runtime, doc.DocumentID); stateAfterInvalidMove.Revision != stateBeforeInvalidMove.Revision || stateAfterInvalidMove.Dirty != stateBeforeInvalidMove.Dirty {
		t.Fatalf("invalid image move changed document state: before=%+v after=%+v", stateBeforeInvalidMove, stateAfterInvalidMove)
	}
	if got := runtimeWordImageParagraph(t, runtime, doc.DocumentID, insertedImage.ImageID); got != destinationParagraph.ParagraphID {
		t.Fatalf("failed move changed image paragraph to %q", got)
	}
	assertInvalidImageRequest("insert-image-missing-paragraph", "word.image.insert", moved.Revision, map[string]any{"paragraph_id": "missing-paragraph", "path": pngPath})
	assertInvalidImageRequest("rotate-image-missing-id", "word.image.rotate", moved.Revision, map[string]any{"image_id": "missing-image", "delta_degrees": 90})
	assertInvalidImageRequest("resize-image-missing-id", "word.image.resize", moved.Revision, map[string]any{"image_id": "missing-image", "width_points": 72})
	assertInvalidImageRequest("replace-image-missing-id", "word.image.replace", moved.Revision, map[string]any{"image_id": "missing-image", "path": jpegPath})
	assertInvalidImageRequest("delete-image-missing-id", "word.image.delete", moved.Revision, map[string]any{"image_id": "missing-image"})
	rotated, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "rotate-agent-image", DocumentID: doc.DocumentID, CommandID: "word.image.rotate",
		ExpectedRevision: moved.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"image_id": insertedImage.ImageID, "delta_degrees": 90}),
	})
	if err != nil {
		t.Fatalf("rotate Agent image: %v", err)
	}
	assertState(rotated.Revision, true)
	resized, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "resize-agent-image", DocumentID: doc.DocumentID, CommandID: "word.image.resize",
		ExpectedRevision: rotated.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"image_id": insertedImage.ImageID, "width_points": 72}),
	})
	if err != nil {
		t.Fatalf("resize Agent image: %v", err)
	}
	assertState(resized.Revision, true)
	replaced := invokeConfirmed("replace-agent-image", "word.image.replace", resized.Revision, map[string]any{"image_id": insertedImage.ImageID, "path": jpegPath})
	assertState(replaced.Revision, true)
	path := filepath.Join(t.TempDir(), "agent-image.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil {
		t.Fatalf("SaveDocumentAs image document: %v", err)
	}
	if saved.Dirty {
		t.Fatalf("saved Word image document remains dirty: %+v", saved)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen image DOCX: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	imageData := findApplicationWordImage(reopened.WordBlocks(), insertedImage.ImageID)
	if imageData == nil || imageData.MediaType != "image/jpeg" || imageData.WidthEMU != 914400 || imageData.RotationDegrees != 90 {
		t.Fatalf("Agent image edits did not round-trip: %+v", imageData)
	}
	if paragraphID := findApplicationWordImageParagraph(reopened.WordBlocks(), insertedImage.ImageID); paragraphID != destinationParagraph.ParagraphID {
		t.Fatalf("moved image paragraph after reopen=%q; want %q", paragraphID, destinationParagraph.ParagraphID)
	}
	imageBytes, err := reopened.WordImageBytes(insertedImage.ImageID)
	if err != nil || len(imageBytes) == 0 {
		t.Fatalf("read replaced image after reopen: bytes=%d error=%v", len(imageBytes), err)
	}
	if !bytes.HasPrefix(imageBytes, []byte{0xff, 0xd8}) {
		t.Fatal("replacement image bytes are not JPEG")
	}
	deleted := invokeConfirmed("delete-agent-image", "word.image.delete", replaced.Revision, map[string]any{"image_id": insertedImage.ImageID})
	if deleted.Revision != replaced.Revision+1 {
		t.Fatalf("delete revision = %d; want %d", deleted.Revision, replaced.Revision+1)
	}
	assertState(deleted.Revision, true)
	deletedPath := filepath.Join(t.TempDir(), "agent-image-deleted.docx")
	deletedSaved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, deletedPath)
	if err != nil {
		t.Fatalf("SaveDocumentAs after delete: %v", err)
	}
	if deletedSaved.Dirty {
		t.Fatalf("saved deleted-image document remains dirty: %+v", deletedSaved)
	}
	deletedDoc, err := docx.Open(deletedPath)
	if err != nil {
		t.Fatalf("reopen deleted image DOCX: %v", err)
	}
	t.Cleanup(func() { _ = deletedDoc.Close() })
	if findApplicationWordImage(deletedDoc.WordBlocks(), insertedImage.ImageID) != nil {
		t.Fatal("deleted image remained after save and reopen")
	}
}

func writeApplicationWordImageFixture(t *testing.T, name, format string) string {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 1))
	canvas.Set(0, 0, color.NRGBA{R: 40, G: 110, B: 210, A: 255})
	canvas.Set(1, 0, color.NRGBA{R: 210, G: 80, B: 50, A: 255})
	var encoded bytes.Buffer
	switch format {
	case "png":
		if err := png.Encode(&encoded, canvas); err != nil {
			t.Fatalf("encode PNG fixture: %v", err)
		}
	case "jpeg":
		if err := jpeg.Encode(&encoded, canvas, &jpeg.Options{Quality: 90}); err != nil {
			t.Fatalf("encode JPEG fixture: %v", err)
		}
	default:
		t.Fatalf("unsupported fixture format %q", format)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}
	return path
}

func findApplicationWordImage(blocks []document.WordBlock, id string) *document.WordImage {
	for _, block := range blocks {
		var paragraphs []document.WordParagraph
		if block.Paragraph != nil {
			paragraphs = append(paragraphs, *block.Paragraph)
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					paragraphs = append(paragraphs, cell.Paragraphs...)
				}
			}
		}
		for _, paragraph := range paragraphs {
			for _, inline := range paragraph.Inlines {
				if inline.Image != nil && inline.Image.ID == id {
					return inline.Image
				}
			}
		}
	}
	return nil
}

func findApplicationWordImageParagraph(blocks []document.WordBlock, id string) string {
	for _, block := range blocks {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Image != nil && inline.Image.ID == id {
				return block.Paragraph.ID
			}
		}
	}
	return ""
}

func runtimeWordImageParagraph(t *testing.T, runtime *Runtime, documentID, imageID string) string {
	t.Helper()
	snapshot, err := runtime.Queries().InspectDocument(context.Background(), InspectRequest{DocumentID: documentID, Limit: 1000})
	if err != nil {
		t.Fatalf("inspect Word image paragraph: %v", err)
	}
	var output struct {
		Blocks []struct {
			ParagraphID string `json:"paragraph_id"`
			Images      []struct {
				ID string `json:"image_id"`
			} `json:"images"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(snapshot.Content, &output); err != nil {
		t.Fatalf("decode Word image paragraph: %v; content=%s", err, snapshot.Content)
	}
	for _, block := range output.Blocks {
		for _, image := range block.Images {
			if image.ID == imageID {
				return block.ParagraphID
			}
		}
	}
	return ""
}
