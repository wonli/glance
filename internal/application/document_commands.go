package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"glance/internal/document"
)

var (
	ErrPendingEditorDraft        = errors.New("document has pending editor drafts")
	ErrPresentationNotesConflict = errors.New("presentation notes changed since the editor draft began")
)

// SpreadsheetCellSetter is an optional neutral capability implemented by
// writable spreadsheet adapters.
type SpreadsheetCellSetter interface {
	SetCellText(context.Context, string, string, string) (bool, error)
}

type SpreadsheetCellValueSetter interface {
	SetSpreadsheetCellValue(context.Context, string, string, document.CellValue) (bool, error)
}

func decodeSpreadsheetCellValue(raw json.RawMessage) (document.CellValue, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return document.CellValue{}, ErrInvalidCommandArguments
	}
	switch typed := value.(type) {
	case string:
		if utf8.RuneCountInString(typed) > 32767 {
			return document.CellValue{}, ErrInvalidCommandArguments
		}
		return document.CellValue{Kind: document.CellValueText, Text: typed}, nil
	case json.Number:
		number, err := strconv.ParseFloat(typed.String(), 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return document.CellValue{}, ErrInvalidCommandArguments
		}
		return document.CellValue{Kind: document.CellValueNumber, Number: number}, nil
	case bool:
		return document.CellValue{Kind: document.CellValueBoolean, Boolean: typed}, nil
	default:
		return document.CellValue{}, ErrInvalidCommandArguments
	}
}

type SpreadsheetCellFormulaSetter interface {
	SetSpreadsheetCellFormula(context.Context, string, string, string) (bool, error)
}

type SpreadsheetCellsSetter interface {
	SetSpreadsheetCells(context.Context, string, []document.CellEdit) (bool, error)
}

type SpreadsheetRangeFormatter interface {
	SetSpreadsheetCellFormats(context.Context, string, document.CellRange, document.CellFormatPatch) (bool, error)
}

type SpreadsheetRangeSorter interface {
	SortSpreadsheetRange(context.Context, document.SortRequest) (bool, error)
}

type SpreadsheetHistoryController interface {
	ApplySpreadsheetHistory(context.Context, bool) (bool, error)
}

type SpreadsheetSearchCapability interface {
	FindSpreadsheetCells(context.Context, string, string, int) ([]document.CellMatch, bool, error)
}

type SpreadsheetRangeReaderCapability interface {
	ReadSpreadsheetRange(context.Context, string, document.CellRange) ([][]document.Cell, error)
}

// SpreadsheetImageController exposes neutral over-cell image operations to
// registered capabilities. Implementations keep format-library values hidden.
type SpreadsheetImageController interface {
	ListSpreadsheetImages(context.Context, string) ([]document.SpreadsheetImage, error)
	InsertSpreadsheetImage(context.Context, document.SpreadsheetImagePlacement, []byte) (string, error)
	ReplaceSpreadsheetImage(context.Context, string, string, string, []byte) (bool, error)
	MoveSpreadsheetImage(context.Context, string, string, int, int, int, int) (bool, error)
	ResizeSpreadsheetImage(context.Context, string, string, int) (bool, error)
	DeleteSpreadsheetImage(context.Context, string, string) (bool, error)
}

type SpreadsheetPageLayoutPatch struct {
	Operation    string   `json:"operation"`
	Orientation  *string  `json:"orientation,omitempty"`
	PaperSize    *int     `json:"paper_size,omitempty"`
	MarginTop    *float64 `json:"margin_top,omitempty"`
	MarginRight  *float64 `json:"margin_right,omitempty"`
	MarginBottom *float64 `json:"margin_bottom,omitempty"`
	MarginLeft   *float64 `json:"margin_left,omitempty"`
	PrintArea    *string  `json:"print_area,omitempty"`
	PrintTitles  *string  `json:"print_titles,omitempty"`
}

type SpreadsheetPageLayoutController interface {
	GetSpreadsheetPageLayout(context.Context, string) (document.SpreadsheetPageLayout, error)
	SetSpreadsheetPageLayout(context.Context, string, SpreadsheetPageLayoutPatch) (document.SpreadsheetPageLayout, bool, error)
}

type SpreadsheetGridlinesController interface {
	GetSpreadsheetGridlines(context.Context, string) (bool, error)
	SetSpreadsheetGridlines(context.Context, string, bool) (bool, error)
}

// SpreadsheetProtectionController exposes sheet and workbook protection
// without leaking format-library types or returning the supplied password.
type SpreadsheetProtectionController interface {
	GetSpreadsheetProtection(context.Context, string, string) (bool, error)
	SetSpreadsheetProtection(context.Context, string, string, bool, string) (bool, error)
}

type SpreadsheetDataValidationController interface {
	SetSpreadsheetDataValidation(context.Context, document.SpreadsheetDataValidationSpec, bool) (bool, error)
}

type SpreadsheetConditionalFormatController interface {
	SetSpreadsheetConditionalFormat(context.Context, document.SpreadsheetConditionalFormatSpec, bool) (bool, error)
}

type SpreadsheetTableController interface {
	CreateSpreadsheetTable(context.Context, document.SpreadsheetTableSpec) error
}

type SpreadsheetPivotTableController interface {
	CreateSpreadsheetPivotTable(context.Context, document.SpreadsheetPivotTableSpec) error
}

type SpreadsheetChartController interface {
	CreateSpreadsheetChart(context.Context, document.SpreadsheetChartSpec) error
}

type SpreadsheetShapeController interface {
	CreateSpreadsheetShape(context.Context, document.SpreadsheetShapeSpec) error
}

type SpreadsheetDefinedNameController interface {
	GetSpreadsheetDefinedNames(context.Context) ([]document.SpreadsheetDefinedName, error)
	SetSpreadsheetDefinedName(context.Context, document.SpreadsheetDefinedName) (bool, error)
	DeleteSpreadsheetDefinedName(context.Context, string, string) (bool, error)
}

type SpreadsheetTextToColumnsController interface {
	SplitSpreadsheetTextToColumns(context.Context, string, document.CellRange, int, string) (document.SpreadsheetTextToColumnsResult, error)
}

type SpreadsheetDuplicatesController interface {
	RemoveSpreadsheetDuplicates(context.Context, string, document.CellRange, int) (document.SpreadsheetDuplicatesResult, error)
}

type SpreadsheetPanesController interface {
	GetSpreadsheetPanes(context.Context, string) (document.SpreadsheetPaneFreeze, error)
	SetSpreadsheetPanes(context.Context, string, document.SpreadsheetPaneFreeze) (bool, error)
}

type SpreadsheetViewController interface {
	GetSpreadsheetView(context.Context, string) (document.SpreadsheetSheetView, error)
	SetSpreadsheetViewMode(context.Context, string, string) (bool, error)
	SetSpreadsheetViewRuler(context.Context, string, bool) (bool, error)
}

type SpreadsheetGridWindowController interface {
	ReadSpreadsheetGridWindow(context.Context, string, document.CellRange) (document.SheetWindow, error)
}

// SpreadsheetPDFController reads an explicit bounded worksheet window and
// its over-cell images for the shared desktop/MCP PDF export capability.
type SpreadsheetPDFController interface {
	ReadSpreadsheetPDFWindow(context.Context, string, document.CellRange) (document.SheetWindow, error)
	ListSpreadsheetImages(context.Context, string) ([]document.SpreadsheetImage, error)
	ReadSpreadsheetImageBytes(context.Context, string) ([]byte, error)
}

type SpreadsheetCommentController interface {
	GetSpreadsheetComments(context.Context, string) ([]document.SpreadsheetComment, error)
	AddSpreadsheetComment(context.Context, string, document.SpreadsheetComment) (bool, error)
	DeleteSpreadsheetComment(context.Context, string, string) (bool, error)
}

type SpreadsheetFlashFillController interface {
	FlashFillSpreadsheet(context.Context, string, document.CellRange) (document.SpreadsheetFlashFillResult, error)
}

type SpreadsheetHyperlinkController interface {
	GetSpreadsheetHyperlink(context.Context, string, string) (document.CellHyperlink, bool, error)
	SetSpreadsheetHyperlink(context.Context, string, string, string, string, bool) (bool, error)
}

type SpreadsheetStructureController interface {
	ApplySpreadsheetStructure(context.Context, document.StructureEdit) (bool, error)
}

// WordTextRunReplacer is an optional neutral capability implemented by
// writable Word adapters.
type WordTextRunReplacer interface {
	ReplaceWordTextRun(context.Context, string, string) (bool, error)
}

type WordTextRangeReplacer interface {
	ReplaceWordTextRange(context.Context, string, int, int, string) (bool, error)
}

type WordFormatPainterCapability interface {
	ApplyWordFormatting(context.Context, string, string) (bool, error)
}

type WordTableInserter interface {
	InsertWordTable(context.Context, string, int, int) (string, error)
}

type WordCoverPageInserter interface {
	InsertWordCoverPage(context.Context, string, string, string) error
}

type WordBreakInserter interface {
	InsertWordBreak(context.Context, string, int, int, document.WordBreakType) (paragraphID, caretRunID string, err error)
}

type WordBlankPageInserter interface {
	InsertWordBlankPage(context.Context, string, int, int) (paragraphID, caretRunID string, err error)
}

type WordBookmarkInserter interface {
	InsertWordBookmark(context.Context, string, string) error
}

type WordHyperlinkInserter interface {
	InsertWordHyperlink(context.Context, string, string, string) error
}

type WordTextBoxInserter interface {
	InsertWordTextBox(context.Context, string, string) error
}

type WordHeaderFooterSetter interface {
	SetWordHeader(context.Context, string) (bool, error)
	SetWordFooter(context.Context, string) (bool, error)
	SetWordPageNumber(context.Context, string) (bool, error)
}

type WordPageSetupSetter interface {
	SetWordPageOrientation(context.Context, string) (bool, error)
	SetWordPageMargins(context.Context, float64, float64, float64, float64) (bool, error)
	SetWordPageSize(context.Context, float64, float64) (bool, error)
	SetWordPageColumns(context.Context, int) (bool, error)
	SetWordPageLineNumbers(context.Context, string) (bool, error)
	SetWordPageBorders(context.Context, string) (bool, error)
}

type WordBodyTextParagraphAppender interface {
	AppendBodyTextParagraph(context.Context, string) (paragraphID, runID string, err error)
}

// CommandRevisionObserver lets a live host refresh its own views after a
// command has committed and Runtime has advanced the session revision.
type CommandRevisionObserver interface {
	CommandRevisionChanged(uint64)
}

func registerCoreCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "spreadsheet.cell.set", Name: "设置单元格内容",
				Description: "将指定工作表中的单元格设置为文本、数字或布尔值，并保留 JSON 值类型；字符串（例如 \"00123\"）按文本原样写入。公式需要使用专门的公式能力。请先通过文档查询确认工作表名、单元格地址和当前版本。",
				Category:    CapabilityCommand, Formats: []Format{FormatXLSX}, Risk: RiskLow, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"sheet":{"type":"string"},"cell":{"type":"string"},"value":{"anyOf":[{"type":"string","maxLength":32767},{"type":"number"},{"type":"boolean"}]}},"required":["sheet","cell","value"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"sheet":{"type":"string"},"cell":{"type":"string"},"value":{"anyOf":[{"type":"string"},{"type":"number"},{"type":"boolean"}]}},"required":["changed","sheet","cell","value"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Sheet string          `json:"sheet"`
					Cell  string          `json:"cell"`
					Value json.RawMessage `json:"value"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode spreadsheet.cell.set: %w", err)
				}
				value, err := decodeSpreadsheetCellValue(input.Value)
				if err != nil {
					return CapabilityResult{}, err
				}
				if value.Kind == document.CellValueText && strings.HasPrefix(strings.TrimSpace(value.Text), "=") {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				setter, ok := capability.Adapter.(SpreadsheetCellValueSetter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := setter.SetSpreadsheetCellValue(ctx, input.Sheet, input.Cell, value)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool            `json:"changed"`
					Sheet   string          `json:"sheet"`
					Cell    string          `json:"cell"`
					Value   json.RawMessage `json:"value"`
				}{changed, input.Sheet, input.Cell, input.Value}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.paragraph.append", Name: "向 Word 追加段落",
				Description: "在文档正文末尾新增一个纯文本段落，返回段落和可编辑文本片段 ID。每次调用只写一个段落；文本最多 16000 个 Unicode 字符且不能包含换行符，长报告应拆成多次调用。先查询并使用当前文档修订号。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskLow, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","maxLength":16000,"description":"纯文本段落，最多 16000 个 Unicode 字符，不得包含换行符。"}},"required":["text"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"paragraph_id":{"type":"string"},"run_id":{"type":"string"},"text":{"type":"string"}},"required":["changed","paragraph_id","run_id","text"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode word.paragraph.append: %w", err)
				}
				if utf8.RuneCountInString(input.Text) > 16000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				appender, ok := capability.Adapter.(WordBodyTextParagraphAppender)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				paragraphID, runID, err := appender.AppendBodyTextParagraph(ctx, input.Text)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed     bool   `json:"changed"`
					ParagraphID string `json:"paragraph_id"`
					RunID       string `json:"run_id"`
					Text        string `json:"text"`
				}{true, paragraphID, runID, input.Text}, true)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.text_run.replace", Name: "替换 Word 文本片段",
				Description: "用新文本替换一个可编辑文本片段。run_id 必须来自当前文档的查询结果；此操作保留该片段原有格式。请先查询文档并使用返回的当前版本。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskLow, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string"},"value":{"type":"string"}},"required":["run_id","value"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"run_id":{"type":"string"},"value":{"type":"string"}},"required":["changed","run_id","value"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					RunID string `json:"run_id"`
					Value string `json:"value"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode word.text_run.replace: %w", err)
				}
				replacer, ok := capability.Adapter.(WordTextRunReplacer)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := replacer.ReplaceWordTextRun(ctx, input.RunID, input.Value)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool   `json:"changed"`
					RunID   string `json:"run_id"`
					Value   string `json:"value"`
				}{changed, input.RunID, input.Value}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.text_range.replace", Name: "替换 Word 文本范围",
				Description: "在一个可编辑文本运行内替换 Unicode 字符范围，start 和 end 使用从 0 开始的 rune 索引，end 不包含在范围内。可用空范围插入文本。先通过 document.inspect 获取 run_id 和文本，并使用当前文档修订。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string"},"start":{"type":"integer","minimum":0},"end":{"type":"integer","minimum":0},"replacement":{"type":"string","maxLength":1048576,"description":"替换文本，最多 1,048,576 个 Unicode 字符。"}},"required":["run_id","start","end","replacement"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"run_id":{"type":"string"},"start":{"type":"integer"},"end":{"type":"integer"},"replacement":{"type":"string"}},"required":["changed","run_id","start","end","replacement"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					RunID       string `json:"run_id"`
					Start       int    `json:"start"`
					End         int    `json:"end"`
					Replacement string `json:"replacement"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode word.text_range.replace: %w", err)
				}
				if input.RunID == "" || input.Start < 0 || input.End < input.Start || utf8.RuneCountInString(input.Replacement) > 1<<20 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				replacer, ok := capability.Adapter.(WordTextRangeReplacer)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := replacer.ReplaceWordTextRange(ctx, input.RunID, input.Start, input.End, input.Replacement)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed     bool   `json:"changed"`
					RunID       string `json:"run_id"`
					Start       int    `json:"start"`
					End         int    `json:"end"`
					Replacement string `json:"replacement"`
				}{changed, input.RunID, input.Start, input.End, input.Replacement}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.table.insert", Name: "插入 Word 表格",
				Description: "在正文末尾或指定正文段落之后插入空白表格。rows 为 1 至 100，columns 为 1 至 50；返回 table_id。插入后通过 document.inspect 读取单元格文本运行 ID，再用 Word 文本能力填入内容。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"after_paragraph_id":{"type":"string"},"rows":{"type":"integer","minimum":1,"maximum":100},"columns":{"type":"integer","minimum":1,"maximum":50}},"required":["rows","columns"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"table_id":{"type":"string"},"rows":{"type":"integer"},"columns":{"type":"integer"}},"required":["changed","table_id","rows","columns"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					AfterParagraphID string `json:"after_paragraph_id"`
					Rows             int    `json:"rows"`
					Columns          int    `json:"columns"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode word.table.insert: %w", err)
				}
				if input.Rows < 1 || input.Rows > 100 || input.Columns < 1 || input.Columns > 50 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				inserter, ok := capability.Adapter.(WordTableInserter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				tableID, err := inserter.InsertWordTable(ctx, input.AfterParagraphID, input.Rows, input.Columns)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool   `json:"changed"`
					TableID string `json:"table_id"`
					Rows    int    `json:"rows"`
					Columns int    `json:"columns"`
				}{true, tableID, input.Rows, input.Columns}, true)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.cover_page.insert", Name: "插入 Word 封面",
				Description: "在 Word 正文开头插入一页封面，并在封面后添加分页符。可提供 title、subtitle、author；未提供时使用中文默认值。每项最多 512 个 Unicode 字符。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","maxLength":512},"subtitle":{"type":"string","maxLength":512},"author":{"type":"string","maxLength":512}},"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"title":{"type":"string"},"subtitle":{"type":"string"},"author":{"type":"string"}},"required":["changed","title","subtitle","author"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Title    string `json:"title"`
					Subtitle string `json:"subtitle"`
					Author   string `json:"author"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode word.cover_page.insert: %w", err)
				}
				input.Title = strings.TrimSpace(input.Title)
				input.Subtitle = strings.TrimSpace(input.Subtitle)
				input.Author = strings.TrimSpace(input.Author)
				if input.Title == "" {
					input.Title = "文档标题"
				}
				if input.Subtitle == "" {
					input.Subtitle = "文档副标题"
				}
				if input.Author == "" {
					input.Author = "作者"
				}
				if len([]rune(input.Title)) > 512 || len([]rune(input.Subtitle)) > 512 || len([]rune(input.Author)) > 512 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				inserter, ok := capability.Adapter.(WordCoverPageInserter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				if err := inserter.InsertWordCoverPage(ctx, input.Title, input.Subtitle, input.Author); err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed  bool   `json:"changed"`
					Title    string `json:"title"`
					Subtitle string `json:"subtitle"`
					Author   string `json:"author"`
				}{true, input.Title, input.Subtitle, input.Author}, true)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.page_break.insert", Name: "插入 Word 分页符",
				Description: "在明确可编辑文本运行的 Unicode 字符范围处插入分页符或分栏符。break_type 默认为 page，也可设为 column。start/end 从 0 开始且 end 不包含在范围内；相同值表示在插入点，非空范围会被所选分隔符替换。仅支持正文段落。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string"},"start":{"type":"integer","minimum":0},"end":{"type":"integer","minimum":0},"break_type":{"type":"string","enum":["page","column"]}},"required":["run_id","start","end"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"paragraph_id":{"type":"string"},"caret_run_id":{"type":"string"},"break_type":{"type":"string","enum":["page","column"]}},"required":["changed","paragraph_id","caret_run_id","break_type"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					RunID     string                 `json:"run_id"`
					Start     int                    `json:"start"`
					End       int                    `json:"end"`
					BreakType document.WordBreakType `json:"break_type"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode word.page_break.insert: %w", err)
				}
				if input.RunID == "" || input.Start < 0 || input.End < input.Start {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if input.BreakType == "" {
					input.BreakType = document.WordBreakPage
				}
				if input.BreakType != document.WordBreakPage && input.BreakType != document.WordBreakColumn {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				inserter, ok := capability.Adapter.(WordBreakInserter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				paragraphID, caretRunID, err := inserter.InsertWordBreak(ctx, input.RunID, input.Start, input.End, input.BreakType)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed     bool   `json:"changed"`
					ParagraphID string `json:"paragraph_id"`
					CaretRunID  string `json:"caret_run_id"`
					BreakType   string `json:"break_type"`
				}{true, paragraphID, caretRunID, string(input.BreakType)}, true)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.blank_page.insert", Name: "插入 Word 空白页",
				Description: "在明确可编辑文本运行的 Unicode 字符位置插入一个空白页，保留原有文字，并返回后续编辑位置。start/end 从 0 开始；非空范围会被空白页替换。仅支持正文段落。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string"},"start":{"type":"integer","minimum":0},"end":{"type":"integer","minimum":0}},"required":["run_id","start","end"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"paragraph_id":{"type":"string"},"caret_run_id":{"type":"string"}},"required":["changed","paragraph_id","caret_run_id"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					RunID string `json:"run_id"`
					Start int    `json:"start"`
					End   int    `json:"end"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode word.blank_page.insert: %w", err)
				}
				if input.RunID == "" || input.Start < 0 || input.End < input.Start {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				inserter, ok := capability.Adapter.(WordBlankPageInserter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				paragraphID, caretRunID, err := inserter.InsertWordBlankPage(ctx, input.RunID, input.Start, input.End)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed     bool   `json:"changed"`
					ParagraphID string `json:"paragraph_id"`
					CaretRunID  string `json:"caret_run_id"`
				}{true, paragraphID, caretRunID}, true)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.bookmark.insert", Name: "插入 Word 书签",
				Description: "在明确可编辑文本运行周围插入命名书签。书签名必须以英文字母开头，后续只允许英文字母、数字和下划线，最多 40 个字符。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string"},"name":{"type":"string","minLength":1,"maxLength":40,"pattern":"^[A-Za-z][A-Za-z0-9_]*$"}},"required":["run_id","name"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"run_id":{"type":"string"},"name":{"type":"string"}},"required":["changed","run_id","name"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					RunID string `json:"run_id"`
					Name  string `json:"name"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode word.bookmark.insert: %w", err)
				}
				if input.RunID == "" || !validWordBookmarkName(input.Name) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				inserter, ok := capability.Adapter.(WordBookmarkInserter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				if err := inserter.InsertWordBookmark(ctx, input.RunID, input.Name); err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed bool   `json:"changed"`
					RunID   string `json:"run_id"`
					Name    string `json:"name"`
				}{true, input.RunID, input.Name}, true)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.hyperlink.insert", Name: "插入 Word 超链接",
				Description: "将明确可编辑文本运行替换为超链接文本。地址只允许 http、https、mailto 或 #书签锚点；最长 2048 个字符。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string"},"text":{"type":"string","maxLength":10000},"target_url":{"type":"string","minLength":1,"maxLength":2048}},"required":["run_id","target_url"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"run_id":{"type":"string"},"text":{"type":"string"},"target_url":{"type":"string"}},"required":["changed","run_id","text","target_url"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					RunID     string `json:"run_id"`
					Text      string `json:"text"`
					TargetURL string `json:"target_url"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode word.hyperlink.insert: %w", err)
				}
				input.Text = strings.TrimSpace(input.Text)
				input.TargetURL = strings.TrimSpace(input.TargetURL)
				if input.RunID == "" || len([]rune(input.Text)) > 10000 || !validWordHyperlinkTarget(input.TargetURL) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				inserter, ok := capability.Adapter.(WordHyperlinkInserter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				if err := inserter.InsertWordHyperlink(ctx, input.RunID, input.Text, input.TargetURL); err != nil {
					return CapabilityResult{}, err
				}
				if input.Text == "" {
					input.Text = input.TargetURL
				}
				return capabilityJSONResult(struct {
					Changed   bool   `json:"changed"`
					RunID     string `json:"run_id"`
					Text      string `json:"text"`
					TargetURL string `json:"target_url"`
				}{true, input.RunID, input.Text, input.TargetURL}, true)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.text_box.insert", Name: "插入 Word 文本框",
				Description: "在正文末尾或指定正文段落之后插入一个有边框和底色的 callout 文本框。文本最多 16000 个 Unicode 字符；插入后可通过 document.inspect 定位其表格单元格文本运行。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"after_paragraph_id":{"type":"string","description":"可选目标段落 ID；省略时插入到正文末尾。"},"text":{"type":"string","minLength":1,"maxLength":16000,"description":"文本框正文，最多 16000 个 Unicode 字符。"}},"required":["text"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"after_paragraph_id":{"type":"string"},"text":{"type":"string"}},"required":["changed","text"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					AfterParagraphID string `json:"after_paragraph_id"`
					Text             string `json:"text"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode word.text_box.insert: %w", err)
				}
				if input.Text == "" || utf8.RuneCountInString(input.Text) > 16000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				inserter, ok := capability.Adapter.(WordTextBoxInserter)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				if err := inserter.InsertWordTextBox(ctx, input.AfterParagraphID, input.Text); err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					Changed          bool   `json:"changed"`
					AfterParagraphID string `json:"after_paragraph_id,omitempty"`
					Text             string `json:"text"`
				}{true, input.AfterParagraphID, input.Text}, true)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "word.equation.insert", Name: "插入 Word 公式文本",
				Description: "将公式表达式作为普通文本写入指定可编辑文本运行，或作为文本框插入到正文段落后。此命令不创建 Office 数学公式对象；表达式最多 16000 个 Unicode 字符。",
				Category:    CapabilityCommand, Formats: []Format{FormatDocx}, Risk: RiskMedium, RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"target":{"type":"string","enum":["run","textbox"],"description":"run 替换可编辑文本片段；textbox 在正文插入文本框。"},"run_id":{"type":"string","minLength":1,"description":"target=run 时来自当前文档查询的可编辑文本运行 ID。"},"after_paragraph_id":{"type":"string","description":"target=textbox 时可选的插入位置段落 ID。"},"expression":{"type":"string","minLength":1,"maxLength":16000,"description":"公式表达式按普通文本保存，最多 16000 个 Unicode 字符。"}},"required":["target","expression"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"changed":{"type":"boolean"},"target":{"type":"string","enum":["run","textbox"]},"run_id":{"type":"string"},"after_paragraph_id":{"type":"string"},"expression":{"type":"string"}},"required":["changed","target","expression"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					Target           string `json:"target"`
					RunID            string `json:"run_id"`
					AfterParagraphID string `json:"after_paragraph_id"`
					Expression       string `json:"expression"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return CapabilityResult{}, fmt.Errorf("decode word.equation.insert: %w", err)
				}
				if input.Expression == "" || utf8.RuneCountInString(input.Expression) > 16000 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				changed := false
				switch input.Target {
				case "run":
					if input.RunID == "" || input.AfterParagraphID != "" {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
					replacer, ok := capability.Adapter.(WordTextRunReplacer)
					if !ok {
						return CapabilityResult{}, ErrCapabilityUnsupported
					}
					var err error
					changed, err = replacer.ReplaceWordTextRun(ctx, input.RunID, input.Expression)
					if err != nil {
						return CapabilityResult{}, err
					}
				case "textbox":
					if input.RunID != "" {
						return CapabilityResult{}, ErrInvalidCommandArguments
					}
					inserter, ok := capability.Adapter.(WordTextBoxInserter)
					if !ok {
						return CapabilityResult{}, ErrCapabilityUnsupported
					}
					if err := inserter.InsertWordTextBox(ctx, input.AfterParagraphID, input.Expression); err != nil {
						return CapabilityResult{}, err
					}
					changed = true
				default:
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				return capabilityJSONResult(struct {
					Changed          bool   `json:"changed"`
					Target           string `json:"target"`
					RunID            string `json:"run_id,omitempty"`
					AfterParagraphID string `json:"after_paragraph_id,omitempty"`
					Expression       string `json:"expression"`
				}{changed, input.Target, input.RunID, input.AfterParagraphID, input.Expression}, changed)
			},
		},
	}
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	return nil
}

func validWordBookmarkName(name string) bool {
	if len(name) == 0 || len(name) > 40 || name[0] < 'A' || name[0] > 'Z' && name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, char := range name[1:] {
		if char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func validWordHyperlinkTarget(target string) bool {
	if target == "" || len(target) > 2048 {
		return false
	}
	if strings.HasPrefix(target, "#") {
		return len(strings.TrimSpace(strings.TrimPrefix(target, "#"))) > 0
	}
	parsed, err := url.ParseRequestURI(target)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	case "mailto":
		return parsed.Opaque != "" || parsed.Path != ""
	default:
		return false
	}
}

func capabilityJSONResult(value any, changed bool) (CapabilityResult, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return CapabilityResult{}, err
	}
	return CapabilityResult{Data: data, Changed: changed}, nil
}

func editableWordRun(blocks []document.WordBlock, runID string) (string, bool) {
	for _, block := range blocks {
		if block.Paragraph != nil {
			if value, ok := editableWordRunInParagraph(block.Paragraph, runID); ok {
				return value, true
			}
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					for index := range cell.Paragraphs {
						if value, ok := editableWordRunInParagraph(&cell.Paragraphs[index], runID); ok {
							return value, true
						}
					}
				}
			}
		}
	}
	return "", false
}

func editableWordRunInParagraph(paragraph *document.WordParagraph, runID string) (string, bool) {
	if paragraph == nil || runID == "" {
		return "", false
	}
	for _, inline := range paragraph.Inlines {
		if inline.Text != nil && inline.Text.ID == runID && inline.Text.Editable {
			return inline.Text.Text, true
		}
	}
	return "", false
}
