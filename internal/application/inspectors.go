package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/markdowndoc"
	"glance/internal/format/pdfdoc"
	"glance/internal/format/pptx"
)

const defaultSpreadsheetColumns = 16

type textPage struct {
	Start      int      `json:"start"`
	TotalLines int      `json:"total_lines"`
	Lines      []string `json:"lines"`
}

type jsonArrayPage struct {
	Kind  string            `json:"kind"`
	Start int               `json:"start"`
	Total int               `json:"total"`
	Items []json.RawMessage `json:"items"`
}

type jsonObjectEntry struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type jsonObjectPage struct {
	Kind  string            `json:"kind"`
	Start int               `json:"start"`
	Total int               `json:"total"`
	Items []jsonObjectEntry `json:"items"`
}

type wordQueryBlock struct {
	Index             int                    `json:"index"`
	Kind              string                 `json:"kind"`
	ParagraphID       string                 `json:"paragraph_id,omitempty"`
	Alignment         document.WordAlignment `json:"alignment,omitempty"`
	LeftIndentPoints  float64                `json:"left_indent_points"`
	RightIndentPoints float64                `json:"right_indent_points"`
	ListType          document.WordListKind  `json:"list_type,omitempty"`
	ListLevel         int                    `json:"list_level,omitempty"`
	BeforeSpacingPT   float64                `json:"before_spacing_points"`
	AfterSpacingPT    float64                `json:"after_spacing_points"`
	BeforeSpacingSet  bool                   `json:"before_spacing_set"`
	AfterSpacingSet   bool                   `json:"after_spacing_set"`
	Text              string                 `json:"text,omitempty"`
	Runs              []wordQueryRun         `json:"runs,omitempty"`
	Images            []wordQueryImage       `json:"images,omitempty"`
	Rows              [][]string             `json:"rows,omitempty"`
	Warning           string                 `json:"warning,omitempty"`
}

type wordQueryImage struct {
	ID           string `json:"image_id"`
	ParagraphID  string `json:"paragraph_id"`
	MediaType    string `json:"media_type,omitempty"`
	WidthEMU     int64  `json:"width_emu"`
	HeightEMU    int64  `json:"height_emu"`
	Editable     bool   `json:"editable"`
	WrapMode     string `json:"wrap_mode,omitempty"`
	WrapEditable bool   `json:"wrap_editable"`
}

type wordQueryRun struct {
	ParagraphID string             `json:"paragraph_id"`
	ID          string             `json:"id"`
	Text        string             `json:"text"`
	Editable    bool               `json:"editable"`
	Style       wordQueryTextStyle `json:"style"`
	StyleSet    []string           `json:"style_set,omitempty"`
}

type wordQueryTextStyle struct {
	Bold       bool    `json:"bold"`
	Italic     bool    `json:"italic"`
	Underline  bool    `json:"underline"`
	Strike     bool    `json:"strike"`
	FontFamily string  `json:"font_family,omitempty"`
	FontSizePT float64 `json:"font_size_points,omitempty"`
	FontColor  string  `json:"font_color,omitempty"`
	Highlight  string  `json:"highlight,omitempty"`
}

type presentationQueryElement struct {
	ID                int64                        `json:"id"`
	GroupID           uint32                       `json:"group_id,omitempty"`
	Name              string                       `json:"name,omitempty"`
	Kind              string                       `json:"kind"`
	Text              string                       `json:"text,omitempty"`
	AltText           string                       `json:"alt_text,omitempty"`
	Decorative        bool                         `json:"decorative,omitempty"`
	Bounds            presentationQueryRect        `json:"bounds"`
	RotationDeg       float64                      `json:"rotation_deg"`
	FillColorHex      string                       `json:"fill_color_hex,omitempty"`
	BorderColor       string                       `json:"border_color,omitempty"`
	BorderWidthPT     float64                      `json:"border_width_pt,omitempty"`
	CanEditTextStyle  bool                         `json:"can_edit_text_style"`
	CanEditShapeStyle bool                         `json:"can_edit_shape_style"`
	CanEditPicture    bool                         `json:"can_edit_picture"`
	Paragraphs        []presentationQueryParagraph `json:"paragraphs,omitempty"`
}

type presentationQueryRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type presentationQueryParagraph struct {
	Text        string                        `json:"text"`
	Alignment   string                        `json:"alignment,omitempty"`
	Bullet      bool                          `json:"bullet,omitempty"`
	Runs        []presentationQueryTextRun    `json:"runs,omitempty"`
	LineSpacing *document.PresentationSpacing `json:"line_spacing,omitempty"`
	SpaceBefore *document.PresentationSpacing `json:"space_before,omitempty"`
	SpaceAfter  *document.PresentationSpacing `json:"space_after,omitempty"`
}

type presentationQueryTextRun struct {
	Text      string  `json:"text"`
	Font      string  `json:"font,omitempty"`
	SizePT    float64 `json:"size_pt,omitempty"`
	Bold      bool    `json:"bold,omitempty"`
	Italic    bool    `json:"italic,omitempty"`
	Underline bool    `json:"underline,omitempty"`
	ColorHex  string  `json:"color_hex,omitempty"`
}

type presentationQuerySlide struct {
	Index              int                        `json:"index"`
	ID                 uint32                     `json:"slide_id"`
	Title              string                     `json:"title"`
	Notes              string                     `json:"notes,omitempty"`
	BackgroundColorHex string                     `json:"background_color_hex,omitempty"`
	BackgroundImageSet bool                       `json:"background_image_present,omitempty"`
	Elements           []presentationQueryElement `json:"elements,omitempty"`
}

type spreadsheetQueryCell struct {
	Value   string `json:"value"`
	Formula string `json:"formula,omitempty"`
}

type spreadsheetQueryResult struct {
	Sheet       string                   `json:"sheet"`
	StartRow    int                      `json:"start_row"`
	StartColumn int                      `json:"start_column"`
	Rows        [][]spreadsheetQueryCell `json:"rows"`
}

type pdfQueryText struct {
	Run      int          `json:"run"`
	Text     string       `json:"text"`
	Bounds   pdfQueryRect `json:"bounds"`
	Editable bool         `json:"editable"`
}

type pdfQueryRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"width"`
	H float64 `json:"height"`
}

type pdfQueryAnnotation struct {
	Index  int          `json:"index"`
	Kind   string       `json:"kind"`
	Text   string       `json:"text"`
	Author string       `json:"author,omitempty"`
	Bounds pdfQueryRect `json:"bounds"`
}

type pdfQueryPage struct {
	Index       int                  `json:"index"`
	Revision    uint64               `json:"revision"`
	Width       float64              `json:"width"`
	Height      float64              `json:"height"`
	Rotation    int                  `json:"rotation"`
	Text        []pdfQueryText       `json:"text"`
	Annotations []pdfQueryAnnotation `json:"annotations"`
}

func InspectJSON(ctx context.Context, d *jsondoc.Document, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if request.Sheet != "" || request.Range != nil {
		return nil, nil, ErrQueryFilterNotSupported
	}
	text := d.Text()
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
		content, err := inspectJSONContainer(text, request)
		if err == nil {
			return content, nil, nil
		}
	}
	raw := json.RawMessage(text)
	if json.Valid(raw) {
		content, _, err := marshalQuery(struct {
			Kind  string          `json:"kind"`
			Value json.RawMessage `json:"value"`
		}{Kind: "scalar", Value: append(json.RawMessage(nil), raw...)})
		return content, nil, err
	}
	page := linePage(text, request.Start, request.Limit)
	content, err := json.Marshal(struct {
		Kind string   `json:"kind"`
		Text textPage `json:"text"`
	}{Kind: "invalid_json_text", Text: page})
	return content, []Diagnostic{{Code: "invalid_json", Message: "当前编辑内容不是有效 JSON，已按原文返回"}}, err
}

func inspectJSONContainer(text string, request InspectRequest) (json.RawMessage, error) {
	decoder := json.NewDecoder(strings.NewReader(text))
	first, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch first {
	case json.Delim('['):
		items := make([]json.RawMessage, 0, min(request.Limit, 32))
		total := 0
		for decoder.More() {
			var value json.RawMessage
			if err := decoder.Decode(&value); err != nil {
				return nil, err
			}
			if total >= request.Start && total-request.Start < request.Limit {
				items = append(items, append(json.RawMessage(nil), value...))
			}
			total++
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		if err := ensureJSONEOF(decoder); err != nil {
			return nil, err
		}
		start := min(request.Start, total)
		return json.Marshal(jsonArrayPage{Kind: "array", Start: start, Total: total, Items: items})
	case json.Delim('{'):
		items := make([]jsonObjectEntry, 0, min(request.Limit, 32))
		total := 0
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, ErrInvalidQuery
			}
			var value json.RawMessage
			if err := decoder.Decode(&value); err != nil {
				return nil, err
			}
			if total >= request.Start && total-request.Start < request.Limit {
				items = append(items, jsonObjectEntry{Key: key, Value: append(json.RawMessage(nil), value...)})
			}
			total++
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		if err := ensureJSONEOF(decoder); err != nil {
			return nil, err
		}
		start := min(request.Start, total)
		return json.Marshal(jsonObjectPage{Kind: "object", Start: start, Total: total, Items: items})
	default:
		return nil, ErrInvalidQuery
	}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return ErrInvalidQuery
		}
		return err
	}
	return nil
}

func InspectMarkdown(ctx context.Context, d *markdowndoc.Document, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if request.Sheet != "" || request.Range != nil {
		return nil, nil, ErrQueryFilterNotSupported
	}
	return marshalQuery(linePage(d.Text(), request.Start, request.Limit))
}

func InspectSpreadsheet(ctx context.Context, spreadsheet document.Spreadsheet, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	sheets := spreadsheet.SheetNames()
	sheet := request.Sheet
	if sheet == "" {
		if len(sheets) == 0 {
			return marshalQuery(struct {
				Sheet string            `json:"sheet"`
				Rows  [][]document.Cell `json:"rows"`
			}{Sheet: "", Rows: [][]document.Cell{}})
		}
		sheet = sheets[0]
	}
	if !containsString(sheets, sheet) {
		return nil, nil, fmt.Errorf("worksheet %q does not exist", sheet)
	}
	var bounds document.CellRange
	if request.Range != nil {
		if request.Start != 0 {
			return nil, nil, ErrInvalidQuery
		}
		bounds = *request.Range
	} else {
		rows := request.Limit
		if rows > MaxQueryCells/defaultSpreadsheetColumns {
			rows = MaxQueryCells / defaultSpreadsheetColumns
		}
		startRow := request.Start + 1
		if startRow > 1_048_576 || rows > 1_048_576-startRow+1 {
			return nil, nil, ErrInvalidQuery
		}
		bounds = document.CellRange{
			StartRow: startRow, StartColumn: 1,
			EndRow: startRow + rows - 1, EndColumn: defaultSpreadsheetColumns,
		}
	}
	rowCount := bounds.EndRow - bounds.StartRow + 1
	columnCount := bounds.EndColumn - bounds.StartColumn + 1
	if grid, ok := spreadsheet.(document.GridReader); ok {
		window, err := grid.Window(sheet, bounds.StartRow, bounds.StartColumn, rowCount, columnCount)
		if err != nil {
			return nil, nil, err
		}
		return marshalQuery(spreadsheetQueryResult{
			Sheet: sheet, StartRow: window.StartRow, StartColumn: window.StartColumn,
			Rows: spreadsheetCells(window.Cells),
		})
	}
	all, err := spreadsheet.Range(sheet, bounds.EndRow, bounds.EndColumn)
	if err != nil {
		return nil, nil, err
	}
	rows := make([][]document.Cell, 0, rowCount)
	for row := bounds.StartRow - 1; row < bounds.EndRow && row < len(all); row++ {
		source := all[row]
		if len(source) < bounds.EndColumn {
			padded := make([]document.Cell, bounds.EndColumn)
			copy(padded, source)
			source = padded
		}
		rows = append(rows, append([]document.Cell(nil), source[bounds.StartColumn-1:bounds.EndColumn]...))
	}
	return marshalQuery(spreadsheetQueryResult{
		Sheet: sheet, StartRow: bounds.StartRow, StartColumn: bounds.StartColumn,
		Rows: spreadsheetCells(rows),
	})
}

func spreadsheetCells(cells [][]document.Cell) [][]spreadsheetQueryCell {
	rows := make([][]spreadsheetQueryCell, len(cells))
	for rowIndex, row := range cells {
		rows[rowIndex] = make([]spreadsheetQueryCell, len(row))
		for columnIndex, cell := range row {
			rows[rowIndex][columnIndex] = spreadsheetQueryCell{Value: cell.Value, Formula: cell.Formula}
		}
	}
	return rows
}

func InspectWord(ctx context.Context, d *docx.Document, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if request.Sheet != "" || request.Range != nil {
		return nil, nil, ErrQueryFilterNotSupported
	}
	blocks := d.WordBlocks()
	start := min(request.Start, len(blocks))
	end := min(start+request.Limit, len(blocks))
	items := make([]wordQueryBlock, 0, end-start)
	var diagnostics []Diagnostic
	for index, block := range blocks[start:end] {
		item := wordQueryBlock{Index: start + index}
		switch block.Kind {
		case document.WordParagraphBlock:
			item.Kind = "paragraph"
			if block.Paragraph != nil {
				item.ParagraphID = block.Paragraph.ID
				item.Alignment = block.Paragraph.Alignment
				item.LeftIndentPoints = float64(block.Paragraph.LeftIndentEMU) / 12700
				item.RightIndentPoints = float64(block.Paragraph.RightIndentEMU) / 12700
				item.ListType = block.Paragraph.ListKind
				item.ListLevel = block.Paragraph.ListLevel
				item.BeforeSpacingPT = block.Paragraph.Layout.BeforePT
				item.AfterSpacingPT = block.Paragraph.Layout.AfterPT
				item.BeforeSpacingSet = block.Paragraph.Layout.Set&document.WordParagraphBeforeSet != 0
				item.AfterSpacingSet = block.Paragraph.Layout.Set&document.WordParagraphAfterSet != 0
				item.Warning = block.Paragraph.PreviewWarning
				for _, inline := range block.Paragraph.Inlines {
					if inline.Text != nil {
						item.Text += inline.Text.Text
						item.Runs = append(item.Runs, queryWordRun(*inline.Text, block.Paragraph.ID))
					} else if inline.Image != nil {
						item.Images = append(item.Images, queryWordImage(*inline.Image, block.Paragraph.ID))
					} else if inline.Kind == document.WordPageBreakInline {
						item.Text += "\n"
					}
				}
			}
		case document.WordTableBlock:
			item.Kind = "table"
			if block.Table != nil {
				for _, row := range block.Table.Rows {
					cells := make([]string, 0, len(row.Cells))
					for _, cell := range row.Cells {
						var text string
						for _, paragraph := range cell.Paragraphs {
							for _, inline := range paragraph.Inlines {
								if inline.Text != nil {
									text += inline.Text.Text
									item.Runs = append(item.Runs, queryWordRun(*inline.Text, paragraph.ID))
								} else if inline.Image != nil {
									item.Images = append(item.Images, queryWordImage(*inline.Image, paragraph.ID))
								}
							}
						}
						cells = append(cells, text)
					}
					item.Rows = append(item.Rows, cells)
				}
			}
		default:
			item.Kind = "unsupported"
			if block.Unsupported != nil {
				item.Warning = block.Unsupported.Description
			}
			diagnostics = append(diagnostics, Diagnostic{Code: "unsupported_content", Message: "文档包含无法提取为文本的内容", Path: fmt.Sprintf("blocks[%d]", start+index)})
		}
		items = append(items, item)
	}
	content, err := json.Marshal(struct {
		Start int              `json:"start"`
		Total int              `json:"total_blocks"`
		Items []wordQueryBlock `json:"blocks"`
	}{Start: start, Total: len(blocks), Items: items})
	return content, diagnostics, err
}

func queryWordImage(image document.WordImage, paragraphID string) wordQueryImage {
	return wordQueryImage{
		ID: image.ID, ParagraphID: paragraphID, MediaType: image.MediaType,
		WidthEMU: image.WidthEMU, HeightEMU: image.HeightEMU,
		Editable: image.Editable, WrapMode: image.WrapMode, WrapEditable: image.WrapEditable,
	}
}

func queryWordRun(run document.WordTextRun, paragraphID string) wordQueryRun {
	var styleSet []string
	for _, field := range []struct {
		mask document.WordTextStyleMask
		name string
	}{
		{document.WordTextBoldSet, "bold"},
		{document.WordTextItalicSet, "italic"},
		{document.WordTextUnderlineSet, "underline"},
		{document.WordTextStrikeSet, "strike"},
		{document.WordTextFontFamilySet, "font_family"},
		{document.WordTextFontSizeSet, "font_size_points"},
		{document.WordTextFontColorSet, "font_color"},
		{document.WordTextHighlightSet, "highlight"},
	} {
		if run.StyleSet&field.mask != 0 {
			styleSet = append(styleSet, field.name)
		}
	}
	return wordQueryRun{
		ParagraphID: paragraphID, ID: run.ID, Text: run.Text, Editable: run.Editable,
		Style: wordQueryTextStyle{
			Bold: run.Style.Bold, Italic: run.Style.Italic, Underline: run.Style.Underline,
			Strike: run.Style.Strike, FontFamily: run.Style.FontFamily, FontSizePT: run.Style.FontSize,
			FontColor: run.Style.FontColor, Highlight: run.Style.Highlight,
		},
		StyleSet: styleSet,
	}
}

func InspectPresentation(ctx context.Context, d *pptx.Document, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if request.Sheet != "" || request.Range != nil {
		return nil, nil, ErrQueryFilterNotSupported
	}
	snapshot, elementsTruncated := d.SnapshotWindow(request.Start, request.Limit, MaxQueryItems)
	start := min(request.Start, snapshot.SlideCount)
	slides := make([]presentationQuerySlide, 0, len(snapshot.Slides))
	var diagnostics []Diagnostic
	for _, slide := range snapshot.Slides {
		item := presentationQuerySlide{
			Index: slide.Index, ID: slide.ID, Title: slide.Title, Notes: slide.Notes,
			BackgroundColorHex: slide.BgColorHex, BackgroundImageSet: slide.BackgroundImage != nil,
		}
		for _, element := range slide.Elements {
			item.Elements = append(item.Elements, presentationQueryElement{
				ID: element.ID, GroupID: element.GroupID, Name: element.Name, Kind: string(element.Kind), Text: element.PlainText(),
				AltText: element.AltText, Decorative: element.Decorative,
				Bounds:      presentationQueryRect{X: element.Bounds.X, Y: element.Bounds.Y, Width: element.Bounds.W, Height: element.Bounds.H},
				RotationDeg: element.RotationDeg, FillColorHex: element.FillColorHex, BorderColor: element.BorderColor,
				BorderWidthPT: element.BorderWidth, CanEditTextStyle: element.CanEditTextStyle,
				CanEditShapeStyle: element.CanEditShapeStyle, CanEditPicture: element.CanEditPicture,
				Paragraphs: presentationQueryParagraphs(element.Paragraphs),
			})
		}
		slides = append(slides, item)
	}
	if elementsTruncated {
		diagnostics = append(diagnostics, Diagnostic{Code: "elements_truncated", Message: "幻灯片元素总量达到查询上限"})
	}
	if start+len(snapshot.Slides) < snapshot.SlideCount {
		diagnostics = append(diagnostics, Diagnostic{Code: "slides_truncated", Message: "结果按分页限制返回，可使用 start 继续读取"})
	}
	content, err := json.Marshal(struct {
		Title         string                          `json:"title"`
		SlideWidthPT  float64                         `json:"slide_width_pt"`
		SlideHeightPT float64                         `json:"slide_height_pt"`
		SlideCount    int                             `json:"slide_count"`
		Start         int                             `json:"start"`
		Slides        []presentationQuerySlide        `json:"slides"`
		Accessibility presentationAccessibilityReport `json:"accessibility"`
	}{Title: snapshot.Title, SlideWidthPT: snapshot.WidthPT, SlideHeightPT: snapshot.HeightPT, SlideCount: snapshot.SlideCount, Start: start, Slides: slides,
		Accessibility: inspectPresentationAccessibility(slides, snapshot.SlideCount, start, elementsTruncated),
	})
	return content, diagnostics, err
}

type presentationAccessibilityReport struct {
	CheckedSlides int                              `json:"checked_slides"`
	TotalSlides   int                              `json:"total_slides"`
	Complete      bool                             `json:"complete"`
	Issues        []presentationAccessibilityIssue `json:"issues"`
}

type presentationAccessibilityIssue struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	SlideID     uint32 `json:"slide_id"`
	SlideNumber int    `json:"slide_number"`
	ElementID   int64  `json:"element_id,omitempty"`
}

func inspectPresentationAccessibility(slides []presentationQuerySlide, totalSlides, start int, elementsTruncated bool) presentationAccessibilityReport {
	report := presentationAccessibilityReport{
		CheckedSlides: len(slides), TotalSlides: totalSlides,
		Complete: !elementsTruncated && start+len(slides) >= totalSlides,
		Issues:   make([]presentationAccessibilityIssue, 0),
	}
	for _, slide := range slides {
		if strings.TrimSpace(slide.Title) == "" {
			report.Issues = append(report.Issues, presentationAccessibilityIssue{
				Code: "no_readable_text", Severity: "warning",
				Message: "幻灯片没有可读取的文本；请确认是否需要补充标题或说明。", SlideID: slide.ID, SlideNumber: slide.Index + 1,
			})
		}
		for _, element := range slide.Elements {
			kind := element.Kind
			if (kind != string(document.ElementKindPicture) && kind != string(document.ElementKindVideo)) || element.Decorative || strings.TrimSpace(element.AltText) != "" {
				continue
			}
			message := "图片没有替代文本，也未标记为装饰性图片。"
			code := "missing_alt_text"
			if kind == string(document.ElementKindVideo) {
				message = "视频没有替代文本，也未标记为装饰性视频。"
				code = "missing_video_alt_text"
			}
			report.Issues = append(report.Issues, presentationAccessibilityIssue{
				Code: code, Severity: "warning",
				Message: message,
				SlideID: slide.ID, SlideNumber: slide.Index + 1, ElementID: element.ID,
			})
		}
	}
	return report
}

func presentationQueryParagraphs(paragraphs []document.PresentationParagraph) []presentationQueryParagraph {
	if len(paragraphs) == 0 {
		return nil
	}
	result := make([]presentationQueryParagraph, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		item := presentationQueryParagraph{
			Text: paragraph.PlainText(), Alignment: paragraph.Align, Bullet: paragraph.Bullet,
			LineSpacing: paragraph.LineSpacing, SpaceBefore: paragraph.SpaceBefore, SpaceAfter: paragraph.SpaceAfter,
		}
		for _, run := range paragraph.Runs {
			item.Runs = append(item.Runs, presentationQueryTextRun{
				Text: run.Text, Font: run.Font, SizePT: run.SizePT, Bold: run.Bold,
				Italic: run.Italic, Underline: run.Underline, ColorHex: run.ColorHex,
			})
		}
		result = append(result, item)
	}
	return result
}

func InspectPDF(ctx context.Context, d *pdfdoc.Document, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if request.Sheet != "" || request.Range != nil {
		return nil, nil, ErrQueryFilterNotSupported
	}
	info := d.Info()
	start := min(request.Start, info.Pages)
	end := min(start+request.Limit, info.Pages)
	pages := make([]document.PDFPage, 0, end-start)
	remainingItems := MaxQueryItems
	truncated := false
	for pageIndex := start; pageIndex < end; pageIndex++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		page, pageTruncated, err := d.PageWindow(pageIndex, 0, remainingItems)
		if err != nil {
			return nil, nil, err
		}
		truncated = truncated || pageTruncated
		remainingItems -= len(page.Text) + len(page.Annotations)
		if remainingItems < 0 {
			remainingItems = 0
		}
		pages = append(pages, page)
	}
	queryPages := make([]pdfQueryPage, 0, len(pages))
	for _, page := range pages {
		item := pdfQueryPage{
			Index: page.Index, Revision: page.Revision, Width: page.Width, Height: page.Height,
			Rotation: page.Rotation,
		}
		for _, text := range page.Text {
			item.Text = append(item.Text, pdfQueryText{
				Run: text.Run, Text: text.Text, Bounds: pdfRect(text.Bounds), Editable: text.Editable,
			})
		}
		for _, annotation := range page.Annotations {
			item.Annotations = append(item.Annotations, pdfQueryAnnotation{
				Index: annotation.Index, Kind: annotation.Kind, Text: annotation.Text,
				Author: annotation.Author, Bounds: pdfRect(annotation.Bounds),
			})
		}
		queryPages = append(queryPages, item)
	}
	content, diagnostics, err := marshalQuery(struct {
		TotalPages int            `json:"total_pages"`
		Start      int            `json:"start"`
		Pages      []pdfQueryPage `json:"pages"`
	}{TotalPages: info.Pages, Start: start, Pages: queryPages})
	if truncated {
		diagnostics = append(diagnostics, Diagnostic{Code: "page_content_truncated", Message: "PDF 文本与批注总量达到查询上限"})
	}
	return content, diagnostics, err
}

func pdfRect(bounds document.PDFRect) pdfQueryRect {
	return pdfQueryRect{X: bounds.X, Y: bounds.Y, W: bounds.W, H: bounds.H}
}

func linePage(text string, start, limit int) textPage {
	lines := strings.Split(text, "\n")
	start = min(start, len(lines))
	end := min(start+limit, len(lines))
	return textPage{Start: start, TotalLines: len(lines), Lines: append([]string(nil), lines[start:end]...)}
}

func marshalQuery(value any) (json.RawMessage, []Diagnostic, error) {
	content, err := json.Marshal(value)
	return content, nil, err
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
