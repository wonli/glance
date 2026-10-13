package application

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
	"glance/internal/format/pptx"
)

type testPresentationobjectSlideIdElementIdArguments struct {
	SlideId     uint32 `json:"slide_id"`
	ElementId   int64  `json:"element_id"`
	MediaType   string `json:"media_type"`
	ImageBase64 []byte `json:"image_base64"`
}

type presentationElementPictureReplacePayload struct {
	SlideId     uint32 `json:"slide_id"`
	ElementId   int    `json:"element_id"`
	MediaType   string `json:"media_type"`
	ImageBase64 []byte `json:"image_base64"`
}

type presentationElementPictureCropSetPayload struct {
	SlideId   uint32 `json:"slide_id"`
	ElementId int    `json:"element_id"`
	Left      int    `json:"left"`
	Top       int    `json:"top"`
	Right     int    `json:"right"`
	Bottom    int    `json:"bottom"`
}

type testPresentationobjectSlideIdElementIdLeftArguments struct {
	SlideId   uint32 `json:"slide_id"`
	ElementId int64  `json:"element_id"`
	Left      int    `json:"left"`
	Top       int    `json:"top"`
	Right     int    `json:"right"`
	Bottom    int    `json:"bottom"`
}

type presentationElementPictureCreatePayload struct {
	SlideId     uint32 `json:"slide_id"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Fit         string `json:"fit"`
	MediaType   string `json:"media_type"`
	ImageBase64 []byte `json:"image_base64"`
}

type testPresentationobjectSlideIdXArguments struct {
	SlideId     uint32 `json:"slide_id"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Fit         string `json:"fit"`
	MediaType   string `json:"media_type"`
	ImageBase64 []byte `json:"image_base64"`
	AltText     string `json:"alt_text"`
}

type presentationElementChartCreatePayload struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
}

type testPresentationobjectSlideIdChartTypeArguments struct {
	SlideId    uint32                                  `json:"slide_id"`
	ChartType  string                                  `json:"chart_type"`
	Title      string                                  `json:"title"`
	X          int                                     `json:"x"`
	Y          int                                     `json:"y"`
	Width      int                                     `json:"width"`
	Height     int                                     `json:"height"`
	Categories []string                                `json:"categories"`
	Series     []presentationElementChartCreatePayload `json:"series"`
}

type presentationElementTableCreatePayload struct {
	SlideId uint32     `json:"slide_id"`
	X       int        `json:"x"`
	Y       int        `json:"y"`
	Width   int        `json:"width"`
	Height  int        `json:"height"`
	Rows    [][]string `json:"rows"`
}

type testPresentationobjectSlideIdXYArguments struct {
	SlideId  uint32     `json:"slide_id"`
	X        int        `json:"x"`
	Y        int        `json:"y"`
	Width    int        `json:"width"`
	Height   int        `json:"height"`
	FirstRow bool       `json:"first_row"`
	BandRows bool       `json:"band_rows"`
	Rows     [][]string `json:"rows"`
}

type presentationElementStyleSetPayload struct {
	SlideId       uint32  `json:"slide_id"`
	ElementId     int64   `json:"element_id"`
	FillColorHex  string  `json:"fill_color_hex"`
	BorderColor   string  `json:"border_color"`
	BorderWidthPt float64 `json:"border_width_pt"`
}

type presentationElementShapeCreatePayload struct {
	SlideId  uint32 `json:"slide_id"`
	Geometry string `json:"geometry"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Text     string `json:"text"`
}

type presentationElementTransformSetPayload struct {
	SlideId     uint32 `json:"slide_id"`
	ElementId   int    `json:"element_id"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	RotationDeg int    `json:"rotation_deg"`
}

type testPresentationobjectSlideIdElementIdXArguments struct {
	SlideId     uint32 `json:"slide_id"`
	ElementId   int64  `json:"element_id"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	RotationDeg int    `json:"rotation_deg"`
}

type testPresentationobjectSlideIdElementIdFontFamilyArguments struct {
	SlideId            uint32 `json:"slide_id"`
	ElementId          int64  `json:"element_id"`
	FontFamily         string `json:"font_family"`
	FontSizePt         int    `json:"font_size_pt"`
	Bold               bool   `json:"bold"`
	TextColorHex       string `json:"text_color_hex"`
	ParagraphAlignment string `json:"paragraph_alignment"`
}

type presentationElementTextSetPayload struct {
	SlideId   uint32 `json:"slide_id"`
	ElementId int64  `json:"element_id"`
	Text      string `json:"text"`
}

type presentationElementTextSetPayloadSlideId struct {
	SlideId   uint32 `json:"slide_id"`
	ElementId int    `json:"element_id"`
	Text      string `json:"text"`
}

type presentationElementTextCopyPayload struct {
	SlideId   uint32 `json:"slide_id"`
	ElementId int64  `json:"element_id"`
}

type testPresentationobjectSlideIdGeometryArguments struct {
	SlideId  uint32 `json:"slide_id"`
	Geometry string `json:"geometry"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

func TestPresentationObjectCapabilitiesUseStableTargetsAndRoundTrip(t *testing.T) {
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
	operator := Operator{ID: "pptx-object-agent", Kind: OperatorExternalAgent}
	documentID := doc.DocumentID
	revision := doc.Revision
	requestNumber := 0
	invoke := func(capabilityID string, args any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID:        fmt.Sprintf("pptx-object-%s-%d", capabilityID, requestNumber),
			DocumentID:       documentID,
			CapabilityID:     capabilityID,
			ExpectedRevision: revision,
			Operator:         operator,
			Arguments:        mustJSON(t, args),
		})
		if invokeErr == nil {
			revision = result.Revision
		}
		return result, invokeErr
	}
	inspectSlideIDs := func() []uint32 {
		t.Helper()
		result, inspectErr := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: documentID, Limit: 1000})
		if inspectErr != nil {
			t.Fatalf("inspect presentation: %v", inspectErr)
		}
		var snapshot struct {
			Slides []struct {
				ID uint32 `json:"slide_id"`
			} `json:"slides"`
		}
		if err := json.Unmarshal(result.Content, &snapshot); err != nil {
			t.Fatalf("decode presentation inspection: %v; content=%s", err, result.Content)
		}
		ids := make([]uint32, len(snapshot.Slides))
		for index, slide := range snapshot.Slides {
			ids[index] = slide.ID
		}
		return ids
	}
	state := func() DocumentInfo {
		t.Helper()
		info, stateErr := runtime.GetDocument(ctx, documentID)
		if stateErr != nil {
			t.Fatalf("GetDocument: %v", stateErr)
		}
		return info
	}
	initialSlideIDs := inspectSlideIDs()
	if len(initialSlideIDs) != 1 || initialSlideIDs[0] == 0 {
		t.Fatalf("new presentation slide IDs=%+v", initialSlideIDs)
	}
	slideID := initialSlideIDs[0]

	beforeInvalid := state()
	if _, err := invoke("presentation.element.shape.create", testPresentationobjectSlideIdGeometryArguments{SlideId: slideID + 10000, Geometry: "ellipse", X: 30, Y: 40, Width: 120, Height: 80}); err == nil {
		t.Fatal("shape creation accepted a missing slide_id")
	}
	if after := state(); after.Revision != beforeInvalid.Revision || after.Dirty != beforeInvalid.Dirty {
		t.Fatalf("invalid slide target changed presentation: before=%+v after=%+v", beforeInvalid, after)
	}

	textBox, err := invoke("presentation.element.textbox.create", presentationElementTextboxCreatePayload{SlideId: slideID, X: 72, Y: 72, Width: 240, Height: 56, Text: "季度分析草稿"})
	if err != nil {
		t.Fatalf("create text box: %v", err)
	}
	var textBoxResult struct {
		ElementID int64 `json:"element_id"`
		Changed   bool  `json:"changed"`
	}
	if err := json.Unmarshal(textBox.Data, &textBoxResult); err != nil || textBoxResult.ElementID <= 0 || !textBoxResult.Changed {
		t.Fatalf("text box result=%s error=%v", textBox.Data, err)
	}

	copied, err := invoke("presentation.element.text.copy", presentationElementTextCopyPayload{SlideId: slideID, ElementId: textBoxResult.ElementID})
	if err != nil || !containsJSONText(copied.Data, `"text":"季度分析草稿"`) {
		t.Fatalf("copy text result=%s error=%v", copied.Data, err)
	}
	beforeBadText := state()
	if _, err := invoke("presentation.element.text.set", presentationElementTextSetPayloadSlideId{SlideId: slideID, ElementId: 999999, Text: "无效"}); err == nil {
		t.Fatal("text.set accepted a missing element_id")
	}
	if after := state(); after.Revision != beforeBadText.Revision || after.Dirty != beforeBadText.Dirty {
		t.Fatalf("invalid text target changed presentation: before=%+v after=%+v", beforeBadText, after)
	}
	setText, err := invoke("presentation.element.text.set", presentationElementTextSetPayload{SlideId: slideID, ElementId: textBoxResult.ElementID, Text: "季度分析结论"})
	assertPresentationChanged(t, setText, err, true)
	textNoop, err := invoke("presentation.element.text.set", presentationElementTextSetPayload{SlideId: slideID, ElementId: textBoxResult.ElementID, Text: "季度分析结论"})
	assertPresentationChanged(t, textNoop, err, false)

	styleArgs := testPresentationobjectSlideIdElementIdFontFamilyArguments{SlideId: slideID, ElementId: textBoxResult.ElementID, FontFamily: "Noto Sans CJK SC", FontSizePt: 22, Bold: true, TextColorHex: "#2255AA", ParagraphAlignment: "center"}
	styled, err := invoke("presentation.element.style.set", styleArgs)
	assertPresentationChanged(t, styled, err, true)
	styleNoop, err := invoke("presentation.element.style.set", styleArgs)
	assertPresentationChanged(t, styleNoop, err, false)

	transformArgs := testPresentationobjectSlideIdElementIdXArguments{SlideId: slideID, ElementId: textBoxResult.ElementID, X: 91, Y: 105, Width: 260, Height: 64, RotationDeg: 12}
	transformed, err := invoke("presentation.element.transform.set", transformArgs)
	assertPresentationChanged(t, transformed, err, true)
	transformNoop, err := invoke("presentation.element.transform.set", transformArgs)
	assertPresentationChanged(t, transformNoop, err, false)
	beforeBadTransform := state()
	if _, err := invoke("presentation.element.transform.set", presentationElementTransformSetPayload{SlideId: slideID, ElementId: 999999, X: 91, Y: 105, Width: 260, Height: 64, RotationDeg: 12}); err == nil {
		t.Fatal("transform.set accepted a missing element_id")
	}
	if after := state(); after.Revision != beforeBadTransform.Revision || after.Dirty != beforeBadTransform.Dirty {
		t.Fatalf("invalid transform target changed presentation: before=%+v after=%+v", beforeBadTransform, after)
	}

	shape, err := invoke("presentation.element.shape.create", presentationElementShapeCreatePayload{SlideId: slideID, Geometry: "ellipse", X: 380, Y: 80, Width: 130, Height: 90, Text: "重点"})
	if err != nil {
		t.Fatalf("create shape: %v", err)
	}
	var shapeResult struct {
		ElementID int64  `json:"element_id"`
		Geometry  string `json:"geometry"`
	}
	if err := json.Unmarshal(shape.Data, &shapeResult); err != nil || shapeResult.ElementID <= 0 || shapeResult.Geometry != "ellipse" {
		t.Fatalf("shape result=%s error=%v", shape.Data, err)
	}
	shapeStyle, err := invoke("presentation.element.style.set", presentationElementStyleSetPayload{SlideId: slideID, ElementId: shapeResult.ElementID, FillColorHex: "#FFCC33", BorderColor: "#774400", BorderWidthPt: 2.5})
	assertPresentationChanged(t, shapeStyle, err, true)

	table, err := invoke("presentation.element.table.create", testPresentationobjectSlideIdXYArguments{SlideId: slideID, X: 70, Y: 230, Width: 300, Height: 100, FirstRow: true, BandRows: true, Rows: [][]string{{"季度", "收入"}, {"第一季度", "128"}}})
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	var tableResult struct {
		ElementID   int64 `json:"element_id"`
		RowCount    int   `json:"row_count"`
		ColumnCount int   `json:"column_count"`
	}
	if err := json.Unmarshal(table.Data, &tableResult); err != nil || tableResult.ElementID <= 0 || tableResult.RowCount != 2 || tableResult.ColumnCount != 2 {
		t.Fatalf("table result=%s error=%v", table.Data, err)
	}
	beforeBadTable := state()
	if _, err := invoke("presentation.element.table.create", presentationElementTableCreatePayload{SlideId: slideID, X: 70, Y: 230, Width: 300, Height: 100, Rows: [][]string{{"a", "b"}, {"c"}}}); err == nil {
		t.Fatal("table creation accepted a ragged matrix")
	}
	if after := state(); after.Revision != beforeBadTable.Revision || after.Dirty != beforeBadTable.Dirty {
		t.Fatalf("invalid table changed presentation: before=%+v after=%+v", beforeBadTable, after)
	}

	chart, err := invoke("presentation.element.chart.create", testPresentationobjectSlideIdChartTypeArguments{SlideId: slideID, ChartType: "bar", Title: "季度收入", X: 400, Y: 230, Width: 300, Height: 180, Categories: []string{"第一季度", "第二季度"}, Series: []presentationElementChartCreatePayload{{Name: "收入", Values: []float64{128, 156}}}})
	if err != nil {
		t.Fatalf("create chart: %v", err)
	}
	var chartResult struct {
		ElementID     int64  `json:"element_id"`
		ChartType     string `json:"chart_type"`
		CategoryCount int    `json:"category_count"`
	}
	if err := json.Unmarshal(chart.Data, &chartResult); err != nil || chartResult.ElementID <= 0 || chartResult.ChartType != "bar" || chartResult.CategoryCount != 2 {
		t.Fatalf("chart result=%s error=%v", chart.Data, err)
	}

	originalImage := presentationPNG(t, color.NRGBA{R: 40, G: 110, B: 210, A: 255}, color.NRGBA{R: 210, G: 80, B: 50, A: 255})
	replacementImage := presentationPNG(t, color.NRGBA{R: 30, G: 180, B: 90, A: 255}, color.NRGBA{R: 210, G: 180, B: 30, A: 255})
	picture, err := invoke("presentation.element.picture.create", testPresentationobjectSlideIdXArguments{SlideId: slideID, X: 80, Y: 370, Width: 180, Height: 90, Fit: "contain", MediaType: "image/png", ImageBase64: originalImage, AltText: "季度数据示意图"})
	if err != nil {
		t.Fatalf("create picture: %v", err)
	}
	var pictureResult struct {
		ElementID int64  `json:"element_id"`
		MediaType string `json:"media_type"`
		Bounds    struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"bounds"`
	}
	if err := json.Unmarshal(picture.Data, &pictureResult); err != nil || pictureResult.ElementID <= 0 || pictureResult.MediaType != "image/png" || pictureResult.Bounds.Width <= 0 || pictureResult.Bounds.Height <= 0 {
		t.Fatalf("picture result=%s error=%v", picture.Data, err)
	}
	badMedia := state()
	if _, err := invoke("presentation.element.picture.create", presentationElementPictureCreatePayload{SlideId: slideID, X: 80, Y: 370, Width: 180, Height: 90, Fit: "stretch", MediaType: "image/jpeg", ImageBase64: originalImage}); err == nil {
		t.Fatal("picture creation accepted PNG bytes declared as JPEG")
	}
	if after := state(); after.Revision != badMedia.Revision || after.Dirty != badMedia.Dirty {
		t.Fatalf("invalid picture data changed presentation: before=%+v after=%+v", badMedia, after)
	}

	cropArgs := testPresentationobjectSlideIdElementIdLeftArguments{SlideId: slideID, ElementId: pictureResult.ElementID, Left: 1000, Top: 2000, Right: 3000, Bottom: 4000}
	crop, err := invoke("presentation.element.picture.crop.set", cropArgs)
	assertPresentationChanged(t, crop, err, true)
	cropNoop, err := invoke("presentation.element.picture.crop.set", cropArgs)
	assertPresentationChanged(t, cropNoop, err, false)
	beforeBadCrop := state()
	if _, err := invoke("presentation.element.picture.crop.set", presentationElementPictureCropSetPayload{SlideId: slideID, ElementId: 999999, Left: 1000, Top: 2000, Right: 3000, Bottom: 4000}); err == nil {
		t.Fatal("picture crop accepted a missing element_id")
	}
	if after := state(); after.Revision != beforeBadCrop.Revision || after.Dirty != beforeBadCrop.Dirty {
		t.Fatalf("invalid crop target changed presentation: before=%+v after=%+v", beforeBadCrop, after)
	}

	firstPath := filepath.Join(t.TempDir(), "presentation-cropped.pptx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, firstPath); err != nil {
		t.Fatalf("save cropped presentation: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("close cropped presentation: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, firstPath)
	if err != nil {
		t.Fatalf("reopen cropped presentation: %v", err)
	}
	documentID, revision = reopened.DocumentID, reopened.Revision
	loaded, err := pptx.Open(firstPath)
	if err != nil {
		t.Fatalf("open cropped PPTX projection: %v", err)
	}
	loadedSnapshot := loaded.Snapshot()
	_ = loaded.Close()
	if len(loadedSnapshot.Slides) != 1 {
		t.Fatalf("reopened PPTX slides=%d; want 1", len(loadedSnapshot.Slides))
	}
	for _, item := range []struct {
		name string
		id   int64
	}{{"text box", textBoxResult.ElementID}, {"shape", shapeResult.ElementID}, {"table", tableResult.ElementID}, {"chart", chartResult.ElementID}} {
		if findPresentationElement(loadedSnapshot, item.id) == nil {
			t.Fatalf("%s element %d did not survive save/reopen", item.name, item.id)
		}
	}
	loadedPicture := findPresentationElement(loadedSnapshot, pictureResult.ElementID)
	if loadedPicture == nil || loadedPicture.AltText != "季度数据示意图" || !bytes.Equal(loadedPicture.ImageBytes, originalImage) || loadedPicture.Crop != (document.PresentationCrop{Left: 1000, Top: 2000, Right: 3000, Bottom: 4000}) {
		t.Fatalf("picture after cropped save/reopen=%+v bytes=%d", loadedPicture, len(loadedPicture.ImageBytes))
	}
	loadedText := findPresentationElement(loadedSnapshot, textBoxResult.ElementID)
	if loadedText == nil || loadedText.PlainText() != "季度分析结论" || loadedText.Bounds != (document.PresentationRect{X: 91, Y: 105, W: 260, H: 64}) || loadedText.RotationDeg != 12 || len(loadedText.Paragraphs) == 0 || len(loadedText.Paragraphs[0].Runs) == 0 {
		t.Fatalf("edited text element after reopen=%+v", loadedText)
	}
	loadedRun := loadedText.Paragraphs[0].Runs[0]
	if !loadedRun.Bold || loadedRun.Font != "Noto Sans CJK SC" || loadedRun.SizePT != 22 || loadedRun.ColorHex != "#2255AA" || loadedText.Paragraphs[0].Align != "center" {
		t.Fatalf("text style after reopen: run=%+v paragraph=%+v", loadedRun, loadedText.Paragraphs[0])
	}

	beforeBadReplace := state()
	if _, err := invoke("presentation.element.picture.replace", presentationElementPictureReplacePayload{SlideId: slideID, ElementId: 999999, MediaType: "image/png", ImageBase64: replacementImage}); err == nil {
		t.Fatal("picture replacement accepted a missing element_id")
	}
	if after := state(); after.Revision != beforeBadReplace.Revision || after.Dirty != beforeBadReplace.Dirty {
		t.Fatalf("invalid picture replacement changed presentation: before=%+v after=%+v", beforeBadReplace, after)
	}
	replaceArgs := testPresentationobjectSlideIdElementIdArguments{SlideId: slideID, ElementId: pictureResult.ElementID, MediaType: "image/png", ImageBase64: replacementImage}
	replaced, err := invoke("presentation.element.picture.replace", replaceArgs)
	assertPresentationChanged(t, replaced, err, true)
	replaceNoop, err := invoke("presentation.element.picture.replace", replaceArgs)
	assertPresentationChanged(t, replaceNoop, err, false)

	finalPath := filepath.Join(t.TempDir(), "presentation-objects.pptx")
	if _, err := runtime.SaveDocumentAs(ctx, documentID, finalPath); err != nil {
		t.Fatalf("save final presentation: %v", err)
	}
	if err := runtime.CloseDocument(ctx, documentID); err != nil {
		t.Fatalf("close final presentation: %v", err)
	}
	final, err := pptx.Open(finalPath)
	if err != nil {
		t.Fatalf("reopen final PPTX: %v", err)
	}
	finalSnapshot := final.Snapshot()
	_ = final.Close()
	finalPicture := findPresentationElement(finalSnapshot, pictureResult.ElementID)
	if finalPicture == nil || !bytes.Equal(finalPicture.ImageBytes, replacementImage) || finalPicture.Crop != (document.PresentationCrop{}) {
		t.Fatalf("replaced picture did not preserve bytes/reset crop: %+v bytes=%d", finalPicture, len(finalPicture.ImageBytes))
	}
	assertPresentationPackageContains(t, finalPath, "ppt/charts/chart1.xml", "季度收入", "第一季度", "156")
	assertPresentationPackageContains(t, finalPath, "ppt/slides/slide1.xml", "季度", "收入", "第一季度")
}

type presentationSlideNotesSetPayload struct {
	SlideId       uint32 `json:"slide_id"`
	ExpectedNotes string `json:"expected_notes"`
	Notes         string `json:"notes"`
}

func TestPresentationSlideNotesAndDeleteCapabilitiesUseStableIDsAndConfirmation(t *testing.T) {
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
	operator := Operator{ID: "pptx-lifecycle-agent", Kind: OperatorExternalAgent}
	info := doc
	requestNumber := 0
	invoke := func(capabilityID string, args any) (CapabilityInvocationResult, error) {
		t.Helper()
		requestNumber++
		result, invokeErr := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: fmt.Sprintf("pptx-slide-lifecycle-%d", requestNumber), DocumentID: info.DocumentID,
			CapabilityID: capabilityID, ExpectedRevision: info.Revision, Operator: operator, Arguments: mustJSON(t, args),
		})
		if invokeErr == nil {
			info.Revision = result.Revision
		}
		return result, invokeErr
	}
	added, err := invoke("presentation.slide.add", presentationSlideAddPayload{})
	if err != nil {
		t.Fatalf("add presentation slide: %v", err)
	}
	var addedResult struct {
		SlideNumber int `json:"slide_number"`
		SlideCount  int `json:"slide_count"`
	}
	if err := json.Unmarshal(added.Data, &addedResult); err != nil || addedResult.SlideNumber != 2 || addedResult.SlideCount != 2 {
		t.Fatalf("add slide result=%s error=%v", added.Data, err)
	}
	inspection, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: info.DocumentID})
	if err != nil {
		t.Fatalf("inspect added slides: %v", err)
	}
	var slides struct {
		Slides []struct {
			ID    uint32 `json:"slide_id"`
			Notes string `json:"notes"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(inspection.Content, &slides); err != nil || len(slides.Slides) != 2 || slides.Slides[0].ID == 0 || slides.Slides[1].ID == 0 || slides.Slides[0].ID == slides.Slides[1].ID {
		t.Fatalf("added slide inspection=%s error=%v", inspection.Content, err)
	}
	firstSlideID, secondSlideID := slides.Slides[0].ID, slides.Slides[1].ID
	notes, err := invoke("presentation.slide.notes.set", presentationSlideNotesSetPayload{SlideId: secondSlideID, ExpectedNotes: "", Notes: "先核对人员名单，再提交本周日报。\n检查工资汇总。"})
	assertPresentationChanged(t, notes, err, true)
	staleNotes, err := invoke("presentation.slide.notes.set", presentationSlideNotesSetPayload{SlideId: secondSlideID, ExpectedNotes: "旧备注", Notes: "不能覆盖新备注"})
	if err == nil {
		t.Fatalf("notes command accepted a stale expected_notes value: %s", staleNotes.Data)
	}
	stateAfterConflict, err := runtime.GetDocument(ctx, info.DocumentID)
	if err != nil || stateAfterConflict.Revision != info.Revision || !stateAfterConflict.Dirty {
		t.Fatalf("notes conflict changed document state: state=%+v info=%+v error=%v", stateAfterConflict, info, err)
	}
	notesNoop, err := invoke("presentation.slide.notes.set", presentationSlideNotesSetPayload{SlideId: secondSlideID, ExpectedNotes: "先核对人员名单，再提交本周日报。\n检查工资汇总。", Notes: "先核对人员名单，再提交本周日报。\n检查工资汇总。"})
	assertPresentationChanged(t, notesNoop, err, false)

	path := filepath.Join(t.TempDir(), "presentation-slide-notes.pptx")
	if _, err := runtime.SaveDocumentAs(ctx, info.DocumentID, path); err != nil {
		t.Fatalf("save presentation notes: %v", err)
	}
	if err := runtime.CloseDocument(ctx, info.DocumentID); err != nil {
		t.Fatalf("close presentation notes: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen presentation notes: %v", err)
	}
	info = reopened
	reopenedInspection, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: info.DocumentID})
	if err != nil {
		t.Fatalf("inspect reopened notes: %v", err)
	}
	var reopenedSlides struct {
		Slides []struct {
			ID    uint32 `json:"slide_id"`
			Notes string `json:"notes"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(reopenedInspection.Content, &reopenedSlides); err != nil || len(reopenedSlides.Slides) != 2 || reopenedSlides.Slides[1].ID != secondSlideID || reopenedSlides.Slides[1].Notes != "先核对人员名单，再提交本周日报。\n检查工资汇总。" {
		t.Fatalf("reopened slide notes=%s error=%v", reopenedInspection.Content, err)
	}

	deleteRequest, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-slide-delete-agent", DocumentID: info.DocumentID, CapabilityID: "presentation.slide.delete",
		ExpectedRevision: info.Revision, Operator: operator, Arguments: mustJSON(t, map[string]uint32{"slide_id": secondSlideID}),
	})
	if err != nil {
		t.Fatalf("stage slide deletion: %v", err)
	}
	var pending struct {
		Status         string `json:"status"`
		ConfirmationID string `json:"confirmation_id"`
	}
	if err := json.Unmarshal(deleteRequest.Data, &pending); err != nil || pending.Status != "confirmation_required" || pending.ConfirmationID == "" {
		t.Fatalf("slide deletion did not require confirmation: result=%s error=%v", deleteRequest.Data, err)
	}
	if afterStage, err := runtime.GetDocument(ctx, info.DocumentID); err != nil || afterStage.Revision != info.Revision || afterStage.Dirty {
		t.Fatalf("slide changed before approval: state=%+v error=%v", afterStage, err)
	}
	if err := runtime.ResolveConfirmation(ctx, pending.ConfirmationID, true, Operator{ID: "human-reviewer", Kind: OperatorHuman}); err != nil {
		t.Fatalf("approve slide deletion: %v", err)
	}
	completed := waitForConfirmationStatus(t, runtime, pending.ConfirmationID, ConfirmationCompleted)
	if completed.Result == nil || completed.Result.Revision != info.Revision+1 {
		t.Fatalf("approved slide deletion result=%+v; want one revision", completed)
	}
	deletedInfo, err := runtime.GetDocument(ctx, info.DocumentID)
	if err != nil || deletedInfo.Revision != completed.Result.Revision || !deletedInfo.Dirty {
		t.Fatalf("deleted presentation state=%+v error=%v", deletedInfo, err)
	}
	deletedSlides, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: info.DocumentID})
	if err != nil {
		t.Fatalf("inspect after deleting slide: %v", err)
	}
	var afterDelete struct {
		Slides []struct {
			ID uint32 `json:"slide_id"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(deletedSlides.Content, &afterDelete); err != nil || len(afterDelete.Slides) != 1 || afterDelete.Slides[0].ID != firstSlideID {
		t.Fatalf("slide delete result=%s error=%v", deletedSlides.Content, err)
	}
	if _, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "pptx-slide-delete-last-slide", DocumentID: info.DocumentID,
		CapabilityID: "presentation.slide.delete", ExpectedRevision: deletedInfo.Revision,
		Operator:  Operator{ID: "human-reviewer", Kind: OperatorHuman},
		Arguments: mustJSON(t, map[string]uint32{"slide_id": firstSlideID}),
	}); err == nil {
		t.Fatal("slide.delete removed the final slide")
	}
	lastSlideState, err := runtime.GetDocument(ctx, info.DocumentID)
	if err != nil || lastSlideState.Revision != deletedInfo.Revision || lastSlideState.Dirty != deletedInfo.Dirty {
		t.Fatalf("rejecting final-slide deletion changed state: before=%+v after=%+v error=%v", deletedInfo, lastSlideState, err)
	}
}

func assertPresentationChanged(t *testing.T, result CapabilityInvocationResult, err error, want bool) {
	t.Helper()
	if err != nil {
		t.Fatalf("presentation capability failed: %v", err)
	}
	var value struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(result.Data, &value); err != nil || value.Changed != want {
		t.Fatalf("presentation capability changed=%t want=%t data=%s error=%v", value.Changed, want, result.Data, err)
	}
}

func findPresentationElement(snapshot document.PresentationSnapshot, id int64) *document.PresentationElement {
	for slideIndex := range snapshot.Slides {
		for elementIndex := range snapshot.Slides[slideIndex].Elements {
			if snapshot.Slides[slideIndex].Elements[elementIndex].ID == id {
				return &snapshot.Slides[slideIndex].Elements[elementIndex]
			}
		}
	}
	return nil
}

func presentationPNG(t *testing.T, colors ...color.NRGBA) []byte {
	t.Helper()
	canvas := image.NewNRGBA(image.Rect(0, 0, len(colors), 1))
	for index, pixel := range colors {
		canvas.SetNRGBA(index, 0, pixel)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatalf("encode presentation PNG fixture: %v", err)
	}
	return encoded.Bytes()
}

func assertPresentationPackageContains(t *testing.T, path, part string, fragments ...string) {
	t.Helper()
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open PPTX package %q: %v", path, err)
	}
	defer archive.Close()
	for _, file := range archive.File {
		if file.Name != part {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open PPTX part %q: %v", part, err)
		}
		content, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read PPTX part %q: read=%v close=%v", part, readErr, closeErr)
		}
		for _, fragment := range fragments {
			if !strings.Contains(string(content), fragment) {
				t.Fatalf("PPTX part %q does not contain %q", part, fragment)
			}
		}
		return
	}
	t.Fatalf("PPTX package is missing part %q", part)
}
