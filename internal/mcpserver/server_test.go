package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPToolsExactlyMatchCatalog(t *testing.T) {
	runtime, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	server, err := New(runtime)
	if err != nil {
		t.Fatalf("New MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(context.Background())
	if err != nil {
		t.Fatalf("connect in-memory MCP client: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })
	listed, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list MCP tools: %v", err)
	}
	got := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		got = append(got, tool.Name)
		if tool.Description == "" || tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Errorf("MCP tool %q is missing its description or schema", tool.Name)
		}
	}
	want := make([]string, 0)
	for _, descriptor := range runtime.Catalog().Descriptors() {
		want = append(want, descriptor.ID)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MCP tools = %v; catalog capabilities = %v", got, want)
	}
}

type testMcpServerDocumentIdExpectedRevisionArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Operation        string `json:"operation"`
}

func TestMCPViewCapabilityWithoutAttachedWindowReturnsNoAttachedView(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	doc, err := runtime.CreateDocument(ctx, application.FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	server, err := New(runtime)
	if err != nil {
		t.Fatalf("New MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		t.Fatalf("connect in-memory MCP client: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "spreadsheet.zoom.apply", Arguments: testMcpServerDocumentIdExpectedRevisionArguments{DocumentId: doc.DocumentID, ExpectedRevision: doc.Revision, Operation: "zoom_in"}})
	if err != nil {
		t.Fatalf("view tool transport error: %v", err)
	}
	if !result.IsError || stringValue(t, mapValue(t, result.StructuredContent)["error"]) != "no_attached_view" {
		t.Fatalf("view capability without attached window = %+v; want no_attached_view", result)
	}
	current, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || current.Revision != doc.Revision {
		t.Fatalf("view request changed document revision: current=%+v error=%v", current, err)
	}
}

type testMcpServerEmptyArguments struct {
}

type testMcpServerDocumentIdExpectedRevisionArguments7 struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
}

type testMcpServerDocumentIdExpectedRevisionPathArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Path             string `json:"path"`
}

type testMcpServerDocumentIdExpectedRevisionTitleArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Title            string `json:"title"`
	Subtitle         string `json:"subtitle"`
	Author           string `json:"author"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	Text             string `json:"text"`
	TargetUrl        string `json:"target_url"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdNameArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	Name             string `json:"name"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdStartArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	Start            int    `json:"start"`
	End              int    `json:"end"`
}

type testMcpServerDocumentIdExpectedRevisionEnabledArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Enabled          bool   `json:"enabled"`
}

type testMcpServerDocumentIdExpectedRevisionTextArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Text             string `json:"text"`
}

type testMcpServerDocumentIdArguments struct {
	DocumentId string `json:"document_id"`
}

type testMcpServerDocumentIdExpectedRevisionColorArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Color            string `json:"color"`
}

type testMcpServerDocumentIdExpectedRevisionTitleCreatorArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Title            string `json:"title"`
	Creator          string `json:"creator"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdStartEndArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	Start            int    `json:"start"`
	End              int    `json:"end"`
	BreakType        string `json:"break_type"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdValueArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	Value            string `json:"value"`
}

type testMcpServerDocumentIdExpectedRevisionLimitArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Limit            int    `json:"limit"`
}

type testMcpServerDocumentIdStartBlockArguments struct {
	DocumentId string `json:"document_id"`
	StartBlock int    `json:"start_block"`
	MaxBlocks  int    `json:"max_blocks"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdLanguageCodeArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	LanguageCode     string `json:"language_code"`
}

type testMcpServerBorderTypeArguments struct {
	BorderType string `json:"border_type"`
}

type testMcpServerModeArguments struct {
	Mode string `json:"mode"`
}

type testMcpServerColumnsArguments struct {
	Columns int `json:"columns"`
}

type testMcpServerWidthPointsHeightPointsArguments struct {
	WidthPoints  int `json:"width_points"`
	HeightPoints int `json:"height_points"`
}

type testMcpServerTopPointsRightPointsArguments struct {
	TopPoints    int `json:"top_points"`
	RightPoints  int `json:"right_points"`
	BottomPoints int `json:"bottom_points"`
	LeftPoints   int `json:"left_points"`
}

type testMcpServerOrientationArguments struct {
	Orientation string `json:"orientation"`
}

type testMcpServerDocumentIdExpectedRevisionAlignmentArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Alignment        string `json:"alignment"`
}

type testMcpServerDocumentIdExpectedRevisionAfterParagraphIdArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	AfterParagraphId string `json:"after_paragraph_id"`
	Text             string `json:"text"`
}

type testMcpServerDocumentIdExpectedRevisionAfterParagraphIdRowsArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	AfterParagraphId string `json:"after_paragraph_id"`
	Rows             int    `json:"rows"`
	Columns          int    `json:"columns"`
}

type testMcpServerDocumentIdQueryArguments struct {
	DocumentId string `json:"document_id"`
	Query      string `json:"query"`
	Limit      int    `json:"limit"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdStartEndReplacementArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	Start            int    `json:"start"`
	End              int    `json:"end"`
	Replacement      string `json:"replacement"`
}

type testMcpServerDocumentIdExpectedRevisionParagraphIdArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	ParagraphId      string `json:"paragraph_id"`
	BeforePoints     int    `json:"before_points"`
	AfterPoints      int    `json:"after_points"`
}

type testMcpServerDocumentIdExpectedRevisionParagraphIdListTypeArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	ParagraphId      string `json:"paragraph_id"`
	ListType         string `json:"list_type"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdFontSizePointsArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	FontSizePoints   int    `json:"font_size_points"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdFontFamilyArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	FontFamily       string `json:"font_family"`
}

type testMcpServerDocumentIdExpectedRevisionParagraphIdLeftIndentPointsArguments struct {
	DocumentId        string `json:"document_id"`
	ExpectedRevision  uint64 `json:"expected_revision"`
	ParagraphId       string `json:"paragraph_id"`
	LeftIndentPoints  int    `json:"left_indent_points"`
	RightIndentPoints int    `json:"right_indent_points"`
}

type testMcpServerDocumentIdExpectedRevisionParagraphIdAlignmentArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	ParagraphId      string `json:"paragraph_id"`
	Alignment        string `json:"alignment"`
}

type testMcpServerDocumentIdExpectedRevisionIdempotencyKeyArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	IdempotencyKey   string `json:"idempotency_key"`
	Text             string `json:"text"`
}

type testMcpServerDocumentIdExpectedRevisionIdempotencyKeyRunIdArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	IdempotencyKey   string `json:"idempotency_key"`
	RunId            string `json:"run_id"`
	Value            string `json:"value"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdBoldArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	Bold             bool   `json:"bold"`
}

type testMcpServerFormatArguments struct {
	Format string `json:"format"`
}

func TestMCPCreateInspectEditSaveAndStaleRevision(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	server, err := New(runtime)
	if err != nil {
		t.Fatalf("New MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		t.Fatalf("connect in-memory MCP client: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })

	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: testMcpServerFormatArguments{Format: "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create Word tool result = %+v, %v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))

	inspected, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.inspect", Arguments: testMcpServerDocumentIdExpectedRevisionLimitArguments{DocumentId: documentID, ExpectedRevision: revision, Limit: 10}})
	if err != nil || inspected.IsError {
		t.Fatalf("inspect Word tool result = %+v, %v", inspected, err)
	}
	data := mapValue(t, mapValue(t, inspected.StructuredContent)["data"])
	content := mapValue(t, data["content"])
	blocks := rawArray(t, content["blocks"])
	if len(blocks) == 0 {
		t.Fatalf("Word inspection content = %#v", data["content"])
	}
	runs := rawArray(t, mapValue(t, blocks[0])["runs"])
	if len(runs) == 0 {
		t.Fatalf("blank Word document runs = %#v", blocks[0])
	}
	runID := stringValue(t, mapValue(t, runs[0])["id"])
	styled, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.style.set", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdBoldArguments{DocumentId: documentID, ExpectedRevision: revision, RunId: runID, Bold: true}})
	if err != nil || styled.IsError {
		t.Fatalf("style Word tool result = %+v, %v", styled, err)
	}
	styledRevision := uint64(numberValue(t, mapValue(t, styled.StructuredContent)["revision"]))
	if styledRevision != revision+1 {
		t.Fatalf("styled revision = %d; want %d", styledRevision, revision+1)
	}

	changed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.replace", Arguments: testMcpServerDocumentIdExpectedRevisionIdempotencyKeyRunIdArguments{DocumentId: documentID, ExpectedRevision: styledRevision, IdempotencyKey: "word-edit-once", RunId: runID, Value: "通过 MCP 写入的周报内容"}})
	if err != nil || changed.IsError {
		t.Fatalf("edit Word tool result = %+v, %v", changed, err)
	}
	updatedRevision := uint64(numberValue(t, mapValue(t, changed.StructuredContent)["revision"]))
	if updatedRevision != styledRevision+1 {
		t.Fatalf("updated revision = %d; want %d", updatedRevision, styledRevision+1)
	}
	stale, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.replace", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdValueArguments{DocumentId: documentID, ExpectedRevision: revision, RunId: runID, Value: "过期修改"}})
	if err != nil {
		t.Fatalf("stale tool call transport error: %v", err)
	}
	if !stale.IsError || stringValue(t, mapValue(t, stale.StructuredContent)["error"]) != "revision_conflict" {
		t.Fatalf("stale MCP response = %+v; want structured revision_conflict", stale)
	}
	appended, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: testMcpServerDocumentIdExpectedRevisionIdempotencyKeyArguments{DocumentId: documentID, ExpectedRevision: updatedRevision, IdempotencyKey: "word-report-paragraph-once", Text: "本周新增的报告段落"}})
	if err != nil || appended.IsError {
		t.Fatalf("append Word report paragraph result = %+v, %v", appended, err)
	}
	finalRevision := uint64(numberValue(t, mapValue(t, appended.StructuredContent)["revision"]))
	if finalRevision != updatedRevision+1 {
		t.Fatalf("appended revision = %d; want %d", finalRevision, updatedRevision+1)
	}
	appendedData := mapValue(t, mapValue(t, appended.StructuredContent)["data"])
	paragraphID := stringValue(t, appendedData["paragraph_id"])
	aligned, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.alignment.set", Arguments: testMcpServerDocumentIdExpectedRevisionParagraphIdAlignmentArguments{DocumentId: documentID, ExpectedRevision: finalRevision, ParagraphId: paragraphID, Alignment: "center"}})
	if err != nil || aligned.IsError {
		t.Fatalf("align Word paragraph tool result = %+v, %v", aligned, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, aligned.StructuredContent)["revision"]))
	indented, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.indent.set", Arguments: testMcpServerDocumentIdExpectedRevisionParagraphIdLeftIndentPointsArguments{DocumentId: documentID, ExpectedRevision: finalRevision, ParagraphId: paragraphID, LeftIndentPoints: 36, RightIndentPoints: 18}})
	if err != nil || indented.IsError {
		t.Fatalf("indent Word paragraph tool result = %+v, %v", indented, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, indented.StructuredContent)["revision"]))
	family, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.font_family.set", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdFontFamilyArguments{DocumentId: documentID, ExpectedRevision: finalRevision, RunId: stringValue(t, mapValue(t, appendedData)["run_id"]), FontFamily: "Arial"}})
	if err != nil || family.IsError {
		t.Fatalf("set Word font family tool result = %+v, %v", family, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, family.StructuredContent)["revision"]))
	sized, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.font_size.set", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdFontSizePointsArguments{DocumentId: documentID, ExpectedRevision: finalRevision, RunId: stringValue(t, mapValue(t, appendedData)["run_id"]), FontSizePoints: 16}})
	if err != nil || sized.IsError {
		t.Fatalf("set Word font size tool result = %+v, %v", sized, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, sized.StructuredContent)["revision"]))
	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.list.set", Arguments: testMcpServerDocumentIdExpectedRevisionParagraphIdListTypeArguments{DocumentId: documentID, ExpectedRevision: finalRevision, ParagraphId: paragraphID, ListType: "bullet"}})
	if err != nil || listed.IsError {
		t.Fatalf("set Word paragraph list tool result = %+v, %v", listed, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, listed.StructuredContent)["revision"]))
	spacing, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.spacing.set", Arguments: testMcpServerDocumentIdExpectedRevisionParagraphIdArguments{DocumentId: documentID, ExpectedRevision: finalRevision, ParagraphId: paragraphID, BeforePoints: 12, AfterPoints: 8}})
	if err != nil || spacing.IsError {
		t.Fatalf("set Word paragraph spacing tool result = %+v, %v", spacing, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, spacing.StructuredContent)["revision"]))
	ranged, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_range.replace", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdStartEndReplacementArguments{DocumentId: documentID, ExpectedRevision: finalRevision, RunId: stringValue(t, mapValue(t, appendedData)["run_id"]), Start: 2, End: 4, Replacement: "完成"}})
	if err != nil || ranged.IsError {
		t.Fatalf("replace Word text range tool result = %+v, %v", ranged, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, ranged.StructuredContent)["revision"]))
	searched, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text.search", Arguments: testMcpServerDocumentIdQueryArguments{DocumentId: documentID, Query: "完成的", Limit: 10}})
	if err != nil || searched.IsError {
		t.Fatalf("search Word text tool result = %+v, %v", searched, err)
	}
	searchData := mapValue(t, mapValue(t, searched.StructuredContent)["data"])
	searchMatches := rawArray(t, searchData["matches"])
	if len(searchMatches) != 1 {
		t.Fatalf("MCP text search matches = %#v", searchData)
	}
	searchMatch := mapValue(t, searchMatches[0])
	if numberValue(t, searchMatch["start"]) != 2 || numberValue(t, searchMatch["end"]) != 5 || stringValue(t, searchMatch["run_id"]) != stringValue(t, mapValue(t, appendedData)["run_id"]) {
		t.Fatalf("MCP text search returned wrong Unicode range: %#v", searchMatch)
	}
	if uint64(numberValue(t, mapValue(t, searched.StructuredContent)["revision"])) != finalRevision {
		t.Fatalf("read-only MCP search changed revision from %d", finalRevision)
	}
	table, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.table.insert", Arguments: testMcpServerDocumentIdExpectedRevisionAfterParagraphIdRowsArguments{DocumentId: documentID, ExpectedRevision: finalRevision, AfterParagraphId: paragraphID, Rows: 2, Columns: 3}})
	if err != nil || table.IsError {
		t.Fatalf("insert Word table tool result = %+v, %v", table, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, table.StructuredContent)["revision"]))
	if stringValue(t, mapValue(t, mapValue(t, table.StructuredContent)["data"])["table_id"]) == "" {
		t.Fatalf("MCP table insert returned no table ID: %+v", table.StructuredContent)
	}
	textBox, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_box.insert", Arguments: testMcpServerDocumentIdExpectedRevisionAfterParagraphIdArguments{DocumentId: documentID, ExpectedRevision: finalRevision, AfterParagraphId: paragraphID, Text: "本周重点：完成工资核算"}})
	if err != nil || textBox.IsError {
		t.Fatalf("insert Word text box tool result = %+v, %v", textBox, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, textBox.StructuredContent)["revision"]))
	header, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.header.set", Arguments: testMcpServerDocumentIdExpectedRevisionTextArguments{DocumentId: documentID, ExpectedRevision: finalRevision, Text: "周报"}})
	if err != nil || header.IsError {
		t.Fatalf("set Word header tool result = %+v, %v", header, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, header.StructuredContent)["revision"]))
	footer, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.footer.set", Arguments: testMcpServerDocumentIdExpectedRevisionTextArguments{DocumentId: documentID, ExpectedRevision: finalRevision, Text: "人力资源部"}})
	if err != nil || footer.IsError {
		t.Fatalf("set Word footer tool result = %+v, %v", footer, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, footer.StructuredContent)["revision"]))
	pageNumber, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.page_number.set", Arguments: testMcpServerDocumentIdExpectedRevisionAlignmentArguments{DocumentId: documentID, ExpectedRevision: finalRevision, Alignment: "right"}})
	if err != nil || pageNumber.IsError {
		t.Fatalf("set Word page number tool result = %+v, %v", pageNumber, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, pageNumber.StructuredContent)["revision"]))
	for _, command := range []struct {
		name string
		args any
	}{
		{"word.page_setup.orientation.set", testMcpServerOrientationArguments{Orientation: "landscape"}},
		{"word.page_setup.margins.set", testMcpServerTopPointsRightPointsArguments{TopPoints: 60, RightPoints: 60, BottomPoints: 60, LeftPoints: 60}},
		{"word.page_setup.size.set", testMcpServerWidthPointsHeightPointsArguments{WidthPoints: 792, HeightPoints: 612}},
		{"word.page_setup.columns.set", testMcpServerColumnsArguments{Columns: 2}},
		{"word.page_setup.line_numbers.set", testMcpServerModeArguments{Mode: "continuous"}},
		{"word.page_setup.borders.set", testMcpServerBorderTypeArguments{BorderType: "dashed"}},
	} {
		arguments := versionedMCPArguments(t, command.args, documentID, finalRevision)
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: command.name, Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("call %s tool result = %+v, %v", command.name, result, err)
		}
		finalRevision = uint64(numberValue(t, mapValue(t, result.StructuredContent)["revision"]))
	}
	language, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.proofing.language.set", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdLanguageCodeArguments{DocumentId: documentID, ExpectedRevision: finalRevision, RunId: stringValue(t, mapValue(t, appendedData)["run_id"]), LanguageCode: "zh-CN"}})
	if err != nil || language.IsError {
		t.Fatalf("set Word proofing language tool result = %+v, %v", language, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, language.StructuredContent)["revision"]))
	statistics, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text.statistics", Arguments: testMcpServerDocumentIdStartBlockArguments{DocumentId: documentID, StartBlock: 0, MaxBlocks: 20}})
	if err != nil || statistics.IsError {
		t.Fatalf("read Word statistics tool result = %+v, %v", statistics, err)
	}
	statisticsData := mapValue(t, mapValue(t, statistics.StructuredContent)["data"])
	if numberValue(t, statisticsData["characters_with_whitespace"]) < 1 || numberValue(t, statisticsData["total_blocks"]) < 1 {
		t.Fatalf("Word statistics MCP result is empty: %+v", statisticsData)
	}
	if uint64(numberValue(t, mapValue(t, statistics.StructuredContent)["revision"])) != finalRevision {
		t.Fatalf("read-only Word statistics changed revision %d", finalRevision)
	}
	tableSnapshot, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.inspect", Arguments: testMcpServerDocumentIdExpectedRevisionLimitArguments{DocumentId: documentID, ExpectedRevision: finalRevision, Limit: 10}})
	if err != nil || tableSnapshot.IsError {
		t.Fatalf("inspect MCP-inserted table = %+v, %v", tableSnapshot, err)
	}
	inspectedData := mapValue(t, mapValue(t, tableSnapshot.StructuredContent)["data"])
	inspectedContent := mapValue(t, inspectedData["content"])
	inspectedBlocks := rawArray(t, inspectedContent["blocks"])
	var cellRunID string
	for _, value := range inspectedBlocks {
		block := mapValue(t, value)
		if stringValue(t, block["kind"]) != "table" {
			continue
		}
		runs := rawArray(t, block["runs"])
		for _, runValue := range runs {
			run := mapValue(t, runValue)
			if boolValue(t, run["editable"]) {
				cellRunID = stringValue(t, run["id"])
				break
			}
		}
	}
	if cellRunID == "" {
		t.Fatalf("MCP table inspection did not expose an editable cell run: %#v", inspectedBlocks)
	}
	cellFilled, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.replace", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdValueArguments{DocumentId: documentID, ExpectedRevision: finalRevision, RunId: cellRunID, Value: "员工"}})
	if err != nil || cellFilled.IsError {
		t.Fatalf("fill MCP-inserted table cell = %+v, %v", cellFilled, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, cellFilled.StructuredContent)["revision"]))
	pageBreak, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.page_break.insert", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdStartEndArguments{DocumentId: documentID, ExpectedRevision: finalRevision, RunId: stringValue(t, mapValue(t, appendedData)["run_id"]), Start: 9, End: 9, BreakType: "column"}})
	if err != nil || pageBreak.IsError {
		t.Fatalf("insert Word column break tool result = %+v, %v", pageBreak, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, pageBreak.StructuredContent)["revision"]))
	pageBreakData := mapValue(t, mapValue(t, pageBreak.StructuredContent)["data"])
	if stringValue(t, pageBreakData["break_type"]) != "column" {
		t.Fatalf("Word break result type = %#v; want column", pageBreakData["break_type"])
	}
	propertiesSet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.document.properties.set", Arguments: testMcpServerDocumentIdExpectedRevisionTitleCreatorArguments{DocumentId: documentID, ExpectedRevision: finalRevision, Title: "通过 MCP 设置的周报", Creator: "Glance Agent"}})
	if err != nil || propertiesSet.IsError {
		t.Fatalf("set Word document properties tool result = %+v, %v", propertiesSet, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, propertiesSet.StructuredContent)["revision"]))
	propertiesGet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.document.properties.get", Arguments: testMcpServerDocumentIdArguments{DocumentId: documentID}})
	if err != nil || propertiesGet.IsError {
		t.Fatalf("get Word document properties tool result = %+v, %v", propertiesGet, err)
	}
	propertiesData := mapValue(t, mapValue(t, propertiesGet.StructuredContent)["data"])
	if stringValue(t, propertiesData["title"]) != "通过 MCP 设置的周报" || stringValue(t, propertiesData["creator"]) != "Glance Agent" {
		t.Fatalf("Word document properties = %#v", propertiesData)
	}
	backgroundSet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.page.background.set", Arguments: testMcpServerDocumentIdExpectedRevisionColorArguments{DocumentId: documentID, ExpectedRevision: finalRevision, Color: "#E3F2FD"}})
	if err != nil || backgroundSet.IsError {
		t.Fatalf("set Word page background tool result = %+v, %v", backgroundSet, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, backgroundSet.StructuredContent)["revision"]))
	backgroundGet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.page.background.get", Arguments: testMcpServerDocumentIdArguments{DocumentId: documentID}})
	if err != nil || backgroundGet.IsError {
		t.Fatalf("get Word page background tool result = %+v, %v", backgroundGet, err)
	}
	if stringValue(t, mapValue(t, mapValue(t, backgroundGet.StructuredContent)["data"])["color"]) != "E3F2FD" {
		t.Fatalf("Word page background result = %+v", backgroundGet.StructuredContent)
	}
	watermarkSet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.watermark.set", Arguments: testMcpServerDocumentIdExpectedRevisionTextArguments{DocumentId: documentID, ExpectedRevision: finalRevision, Text: "机密"}})
	if err != nil || watermarkSet.IsError {
		t.Fatalf("set Word watermark tool result = %+v, %v", watermarkSet, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, watermarkSet.StructuredContent)["revision"]))
	watermarkGet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.watermark.get", Arguments: testMcpServerDocumentIdExpectedRevisionArguments7{DocumentId: documentID, ExpectedRevision: finalRevision}})
	if err != nil || watermarkGet.IsError {
		t.Fatalf("get Word watermark tool result = %+v, %v", watermarkGet, err)
	}
	if stringValue(t, mapValue(t, mapValue(t, watermarkGet.StructuredContent)["data"])["text"]) != "机密" {
		t.Fatalf("Word watermark result = %+v", watermarkGet.StructuredContent)
	}
	trackingSet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.review.track.set", Arguments: testMcpServerDocumentIdExpectedRevisionEnabledArguments{DocumentId: documentID, ExpectedRevision: finalRevision, Enabled: true}})
	if err != nil || trackingSet.IsError {
		t.Fatalf("set Word revision tracking tool result = %+v, %v", trackingSet, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, trackingSet.StructuredContent)["revision"]))
	trackingGet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.review.track.get", Arguments: testMcpServerDocumentIdExpectedRevisionArguments7{DocumentId: documentID, ExpectedRevision: finalRevision}})
	if err != nil || trackingGet.IsError {
		t.Fatalf("get Word revision tracking tool result = %+v, %v", trackingGet, err)
	}
	if !boolValue(t, mapValue(t, mapValue(t, trackingGet.StructuredContent)["data"])["enabled"]) {
		t.Fatalf("Word revision tracking result = %+v", trackingGet.StructuredContent)
	}
	blankPage, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.blank_page.insert", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdStartArguments{DocumentId: documentID, ExpectedRevision: finalRevision, RunId: stringValue(t, pageBreakData["caret_run_id"]), Start: 0, End: 0}})
	if err != nil || blankPage.IsError {
		t.Fatalf("insert Word blank page tool result = %+v, %v", blankPage, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, blankPage.StructuredContent)["revision"]))
	blankPageData := mapValue(t, mapValue(t, blankPage.StructuredContent)["data"])
	bookmark, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.bookmark.insert", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdNameArguments{DocumentId: documentID, ExpectedRevision: finalRevision, RunId: stringValue(t, blankPageData["caret_run_id"]), Name: "ReportBody"}})
	if err != nil || bookmark.IsError {
		t.Fatalf("insert Word bookmark tool result = %+v, %v", bookmark, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, bookmark.StructuredContent)["revision"]))
	link, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.hyperlink.insert", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdArguments{DocumentId: documentID, ExpectedRevision: finalRevision, RunId: stringValue(t, blankPageData["caret_run_id"]), Text: "查看周报", TargetUrl: "https://example.com/report"}})
	if err != nil || link.IsError {
		t.Fatalf("insert Word hyperlink tool result = %+v, %v", link, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, link.StructuredContent)["revision"]))
	cover, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.cover_page.insert", Arguments: testMcpServerDocumentIdExpectedRevisionTitleArguments{DocumentId: documentID, ExpectedRevision: finalRevision, Title: "周报", Subtitle: "本周工作", Author: "运营部"}})
	if err != nil || cover.IsError {
		t.Fatalf("insert Word cover page tool result = %+v, %v", cover, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, cover.StructuredContent)["revision"]))

	path := filepath.Join(t.TempDir(), "周报.docx")
	saved, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.save_as", Arguments: testMcpServerDocumentIdExpectedRevisionPathArguments{DocumentId: documentID, ExpectedRevision: finalRevision, Path: path}})
	if err != nil || saved.IsError {
		t.Fatalf("save Word tool result = %+v, %v", saved, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("MCP did not create Word file: %v", err)
	}
	closed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.close", Arguments: testMcpServerDocumentIdExpectedRevisionArguments7{DocumentId: documentID, ExpectedRevision: finalRevision}})
	if err != nil || closed.IsError {
		t.Fatalf("close Word tool result = %+v, %v", closed, err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := session.CallTool(canceled, &mcp.CallToolParams{Name: "document.list", Arguments: testMcpServerEmptyArguments{}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled MCP call error = %v; want context canceled", err)
	}
}

type testMcpServerDocumentIdExpectedRevisionLimitArguments44 struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Limit            int    `json:"limit"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdStartEndReplacementArguments45 struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	Start            int    `json:"start"`
	End              int    `json:"end"`
	Replacement      string `json:"replacement"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdStartEndArguments46 struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	Start            int    `json:"start"`
	End              int    `json:"end"`
}

type testMcpServerDocumentIdExpectedRevisionTextArguments47 struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Text             string `json:"text"`
}

type testMcpServerFormatArguments48 struct {
	Format string `json:"format"`
}

func TestMCPWordTextRangeCopyCutAndPaste(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	server, err := New(runtime)
	if err != nil {
		t.Fatalf("New MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		t.Fatalf("connect in-memory MCP client: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })

	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: testMcpServerFormatArguments48{Format: "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create Word document: result=%+v error=%v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))

	appended, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: testMcpServerDocumentIdExpectedRevisionTextArguments47{DocumentId: documentID, ExpectedRevision: revision, Text: "ABC🙂DEF"}})
	if err != nil || appended.IsError {
		t.Fatalf("append source text: result=%+v error=%v", appended, err)
	}
	appendedData := mapValue(t, mapValue(t, appended.StructuredContent)["data"])
	runID := stringValue(t, appendedData["run_id"])
	revision = uint64(numberValue(t, mapValue(t, appended.StructuredContent)["revision"]))

	copied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_range.copy", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdStartEndArguments46{DocumentId: documentID, ExpectedRevision: revision, RunId: runID, Start: 3, End: 4}})
	if err != nil || copied.IsError {
		t.Fatalf("copy Unicode range: result=%+v error=%v", copied, err)
	}
	copyData := mapValue(t, mapValue(t, copied.StructuredContent)["data"])
	if stringValue(t, copyData["text"]) != "🙂" || uint64(numberValue(t, mapValue(t, copied.StructuredContent)["revision"])) != revision {
		t.Fatalf("copy result=%+v; want emoji and unchanged revision %d", copied.StructuredContent, revision)
	}

	cut, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_range.cut", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdStartEndArguments46{DocumentId: documentID, ExpectedRevision: revision, RunId: runID, Start: 3, End: 4}})
	if err != nil || cut.IsError {
		t.Fatalf("cut Unicode range: result=%+v error=%v", cut, err)
	}
	cutData := mapValue(t, mapValue(t, cut.StructuredContent)["data"])
	revision = uint64(numberValue(t, mapValue(t, cut.StructuredContent)["revision"]))
	if stringValue(t, cutData["text"]) != "🙂" || !boolValue(t, cutData["changed"]) || revision < 2 {
		t.Fatalf("cut result=%+v; want deleted emoji and revision advance", cut.StructuredContent)
	}

	pasted, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_range.replace", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdStartEndReplacementArguments45{DocumentId: documentID, ExpectedRevision: revision, RunId: runID, Start: 3, End: 3, Replacement: "🙂"}})
	if err != nil || pasted.IsError {
		t.Fatalf("paste into empty range: result=%+v error=%v", pasted, err)
	}

	inspected, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.inspect", Arguments: testMcpServerDocumentIdExpectedRevisionLimitArguments44{DocumentId: documentID, ExpectedRevision: uint64(numberValue(t, mapValue(t, pasted.StructuredContent)["revision"])), Limit: 10}})
	if err != nil || inspected.IsError {
		t.Fatalf("inspect pasted text: result=%+v error=%v", inspected, err)
	}
	inspectData := mapValue(t, mapValue(t, inspected.StructuredContent)["data"])
	content := mapValue(t, inspectData["content"])
	blocks := rawArray(t, content["blocks"])
	if len(blocks) < 1 {
		t.Fatalf("inspected blocks = %#v", content["blocks"])
	}
	var foundText string
	for _, block := range blocks {
		if text, ok := optionalStringValue(mapValue(t, block)["text"]); ok && text != "" {
			foundText = text
		}
	}
	if foundText != "ABC🙂DEF" {
		t.Fatalf("pasted text = %#v; want %q", foundText, "ABC🙂DEF")
	}
}

type testMcpServerDocumentIdExpectedRevisionLimitArguments49 struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Limit            int    `json:"limit"`
}

type testMcpServerDocumentIdExpectedRevisionSourceRunIdArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	SourceRunId      string `json:"source_run_id"`
	TargetRunId      string `json:"target_run_id"`
}

type testMcpServerDocumentIdExpectedRevisionRunIdBoldArguments51 struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	RunId            string `json:"run_id"`
	Bold             bool   `json:"bold"`
}

type testMcpServerDocumentIdExpectedRevisionTextArguments52 struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Text             string `json:"text"`
}

type testMcpServerFormatArguments53 struct {
	Format string `json:"format"`
}

func TestMCPWordFormatPainterUsesExplicitRuns(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	server, err := New(runtime)
	if err != nil {
		t.Fatalf("New MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		t.Fatalf("connect in-memory MCP client: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })
	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: testMcpServerFormatArguments53{Format: "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create Word document: result=%+v error=%v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))
	appendParagraph := func(text string) string {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: testMcpServerDocumentIdExpectedRevisionTextArguments52{DocumentId: documentID, ExpectedRevision: revision, Text: text}})
		if err != nil || result.IsError {
			t.Fatalf("append %q: result=%+v error=%v", text, result, err)
		}
		data := mapValue(t, mapValue(t, result.StructuredContent)["data"])
		runID := stringValue(t, data["run_id"])
		revision = uint64(numberValue(t, mapValue(t, result.StructuredContent)["revision"]))
		return runID
	}
	sourceRunID := appendParagraph("Source")
	targetRunID := appendParagraph("Target")
	styled, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.style.set", Arguments: testMcpServerDocumentIdExpectedRevisionRunIdBoldArguments51{DocumentId: documentID, ExpectedRevision: revision, RunId: sourceRunID, Bold: true}})
	if err != nil || styled.IsError {
		t.Fatalf("style source run: result=%+v error=%v", styled, err)
	}
	revision = uint64(numberValue(t, mapValue(t, styled.StructuredContent)["revision"]))
	painted, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_format.paint", Arguments: testMcpServerDocumentIdExpectedRevisionSourceRunIdArguments{DocumentId: documentID, ExpectedRevision: revision, SourceRunId: sourceRunID, TargetRunId: targetRunID}})
	if err != nil || painted.IsError {
		t.Fatalf("paint target run: result=%+v error=%v", painted, err)
	}
	paintData := mapValue(t, mapValue(t, painted.StructuredContent)["data"])
	if !boolValue(t, paintData["changed"]) || uint64(numberValue(t, mapValue(t, painted.StructuredContent)["revision"])) != revision+1 {
		t.Fatalf("paint result=%+v; want changed and one revision advance", painted.StructuredContent)
	}
	inspected, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.inspect", Arguments: testMcpServerDocumentIdExpectedRevisionLimitArguments49{DocumentId: documentID, ExpectedRevision: revision + 1, Limit: 10}})
	if err != nil || inspected.IsError {
		t.Fatalf("inspect painted document: result=%+v error=%v", inspected, err)
	}
	content := mapValue(t, mapValue(t, mapValue(t, inspected.StructuredContent)["data"])["content"])
	blocks := rawArray(t, content["blocks"])
	for _, block := range blocks {
		runs := rawArray(t, mapValue(t, block)["runs"])
		for _, run := range runs {
			runData := mapValue(t, run)
			style := mapValue(t, runData["style"])
			if stringValue(t, runData["id"]) == targetRunID && boolValue(t, style["bold"]) && stringValue(t, runData["text"]) == "Target" {
				return
			}
		}
	}
	t.Fatalf("MCP format painter did not preserve target text and copy bold style: %s", inspected.StructuredContent)
}

type testMcpServerDocumentIdExpectedRevisionImageIdArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	ImageId          string `json:"image_id"`
}

type testMcpServerDocumentIdExpectedRevisionImageIdWidthPointsArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	ImageId          string `json:"image_id"`
	WidthPoints      int    `json:"width_points"`
}

type testMcpServerDocumentIdExpectedRevisionImageIdDeltaDegreesArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	ImageId          string `json:"image_id"`
	DeltaDegrees     int    `json:"delta_degrees"`
}

type testMcpServerDocumentIdExpectedRevisionParagraphIdAfterInlineIdArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	ParagraphId      string `json:"paragraph_id"`
	AfterInlineId    string `json:"after_inline_id"`
	Path             string `json:"path"`
}

type testMcpServerDocumentIdExpectedRevisionBeforePointsArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	BeforePoints     int    `json:"before_points"`
	AfterPoints      int    `json:"after_points"`
}

type testMcpServerDocumentIdExpectedRevisionTextArguments59 struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Text             string `json:"text"`
}

type testMcpServerFormatArguments60 struct {
	Format string `json:"format"`
}

func TestMCPWordImageToolsMutateTheDocument(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	server, err := New(runtime)
	if err != nil {
		t.Fatalf("New MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		t.Fatalf("connect in-memory MCP client: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })
	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: testMcpServerFormatArguments60{Format: "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create Word document: %+v, %v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))
	appended, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: testMcpServerDocumentIdExpectedRevisionTextArguments59{DocumentId: documentID, ExpectedRevision: revision, Text: "图片说明"}})
	if err != nil || appended.IsError {
		t.Fatalf("append image target paragraph: %+v, %v", appended, err)
	}
	revision = uint64(numberValue(t, mapValue(t, appended.StructuredContent)["revision"]))
	paragraphData := mapValue(t, mapValue(t, appended.StructuredContent)["data"])
	spaced, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.spacing_all.set", Arguments: testMcpServerDocumentIdExpectedRevisionBeforePointsArguments{DocumentId: documentID, ExpectedRevision: revision, BeforePoints: 0, AfterPoints: 4}})
	if err != nil || spaced.IsError {
		t.Fatalf("set all paragraph spacing over MCP: %+v, %v", spaced, err)
	}
	revision = uint64(numberValue(t, mapValue(t, spaced.StructuredContent)["revision"]))
	imagePath := writeMCPWordImage(t)
	inserted, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.image.insert", Arguments: testMcpServerDocumentIdExpectedRevisionParagraphIdAfterInlineIdArguments{DocumentId: documentID, ExpectedRevision: revision, ParagraphId: stringValue(t, paragraphData["paragraph_id"]), AfterInlineId: stringValue(t, paragraphData["run_id"]), Path: imagePath}})
	if err != nil || inserted.IsError {
		t.Fatalf("insert MCP Word image: %+v, %v", inserted, err)
	}
	revision = uint64(numberValue(t, mapValue(t, inserted.StructuredContent)["revision"]))
	imageData := mapValue(t, mapValue(t, inserted.StructuredContent)["data"])
	imageID := stringValue(t, imageData["image_id"])
	rotated, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.image.rotate", Arguments: testMcpServerDocumentIdExpectedRevisionImageIdDeltaDegreesArguments{DocumentId: documentID, ExpectedRevision: revision, ImageId: imageID, DeltaDegrees: -90}})
	if err != nil || rotated.IsError {
		t.Fatalf("rotate MCP Word image: %+v, %v", rotated, err)
	}
	rotatedData := mapValue(t, mapValue(t, rotated.StructuredContent)["data"])
	if numberValue(t, rotatedData["rotation_degrees"]) != 270 {
		t.Fatalf("MCP image rotation result = %#v", rotatedData)
	}
	revision = uint64(numberValue(t, mapValue(t, rotated.StructuredContent)["revision"]))
	resized, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.image.resize", Arguments: testMcpServerDocumentIdExpectedRevisionImageIdWidthPointsArguments{DocumentId: documentID, ExpectedRevision: revision, ImageId: imageID, WidthPoints: 72}})
	if err != nil || resized.IsError {
		t.Fatalf("resize MCP Word image: %+v, %v", resized, err)
	}
	revision = uint64(numberValue(t, mapValue(t, resized.StructuredContent)["revision"]))
	deleted, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.image.delete", Arguments: testMcpServerDocumentIdExpectedRevisionImageIdArguments{DocumentId: documentID, ExpectedRevision: revision, ImageId: imageID}})
	if err != nil || deleted.IsError {
		t.Fatalf("delete MCP Word image: %+v, %v", deleted, err)
	}
	deleteData := mapValue(t, mapValue(t, deleted.StructuredContent)["data"])
	if stringValue(t, deleteData["status"]) != "confirmation_required" || stringValue(t, deleteData["confirmation_id"]) == "" {
		t.Fatalf("delete MCP response = %#v; want human confirmation request", deleteData)
	}
	if uint64(numberValue(t, mapValue(t, deleted.StructuredContent)["revision"])) != revision {
		t.Fatalf("delete MCP staged revision = %+v; want unchanged revision %d", deleted.StructuredContent, revision)
	}
	confirmationID := stringValue(t, deleteData["confirmation_id"])
	if err := runtime.ResolveConfirmation(ctx, confirmationID, true, application.Operator{ID: "human-reviewer", Kind: application.OperatorHuman}); err != nil {
		t.Fatalf("human approves MCP image deletion: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		confirmation, err := runtime.GetConfirmation(ctx, confirmationID)
		if err != nil {
			t.Fatalf("read image deletion confirmation: %v", err)
		}
		if confirmation.Status == application.ConfirmationCompleted {
			current, err := runtime.GetDocument(ctx, documentID)
			if err != nil || current.Revision != revision+1 {
				t.Fatalf("delete MCP revision after approval = %+v, error=%v; want %d", current, err, revision+1)
			}
			snapshot, err := runtime.Queries().InspectDocument(ctx, application.InspectRequest{
				DocumentID: documentID, Limit: 100, MaxBytes: application.MaxQueryResultBytes,
			})
			if err != nil || strings.Contains(string(snapshot.Content), imageID) {
				t.Fatalf("Word image remains after approved deletion: found=%v error=%v content=%s", strings.Contains(string(snapshot.Content), imageID), err, snapshot.Content)
			}
			return
		}
		if confirmation.Status == application.ConfirmationFailed {
			t.Fatalf("MCP image deletion failed after approval: %s", confirmation.Error)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for approved MCP image deletion")
}

func writeMCPWordImage(t *testing.T) string {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 1))
	canvas.Set(0, 0, color.NRGBA{R: 20, G: 100, B: 210, A: 255})
	canvas.Set(1, 0, color.NRGBA{R: 220, G: 80, B: 40, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatalf("encode PNG image fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mcp-image.png")
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatalf("write PNG image fixture: %v", err)
	}
	return path
}

type testMcpServerFormatArguments61 struct {
	Format string `json:"format"`
}

type testMcpServerEmptyArguments62 struct {
}

func TestIndependentMCPRuntimeDoesNotShareDesktopDocumentSessions(t *testing.T) {
	ctx := context.Background()
	desktopRuntime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("New desktop Runtime: %v", err)
	}
	t.Cleanup(func() { _ = desktopRuntime.Close(context.Background()) })
	desktopDocument, err := desktopRuntime.CreateDocument(ctx, application.FormatDocx)
	if err != nil {
		t.Fatalf("create desktop document: %v", err)
	}

	stdioRuntime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("New independent stdio Runtime: %v", err)
	}
	t.Cleanup(func() { _ = stdioRuntime.Close(context.Background()) })
	server, err := New(stdioRuntime)
	if err != nil {
		t.Fatalf("New independent MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		t.Fatalf("connect independent MCP client: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })
	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.list", Arguments: testMcpServerEmptyArguments62{}})
	if err != nil || listed.IsError {
		t.Fatalf("list independent Runtime documents = %+v, %v", listed, err)
	}
	items := rawArray(t, mapValue(t, listed.StructuredContent)["data"])
	if len(items) != 0 {
		t.Fatalf("independent Runtime saw desktop documents: %#v", listed.StructuredContent)
	}
	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: testMcpServerFormatArguments61{Format: "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create independent Word document = %+v, %v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	stdioDocumentID := stringValue(t, createdData["document_id"])
	if stdioDocumentID == desktopDocument.DocumentID {
		t.Fatal("independent Runtime reused the desktop document ID")
	}
	if _, err := desktopRuntime.GetDocument(ctx, desktopDocument.DocumentID); err != nil {
		t.Fatalf("independent Runtime changed desktop session state: %v", err)
	}
}

func mapValue(t *testing.T, value any) map[string]json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode structured value: %v", err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &result); err != nil || result == nil {
		t.Fatalf("value has type %T, want JSON object: %#v (error %v)", value, value, err)
	}
	return result
}

func rawArray(t *testing.T, value json.RawMessage) []json.RawMessage {
	t.Helper()
	var result []json.RawMessage
	if err := json.Unmarshal(value, &result); err != nil {
		t.Fatalf("value is not a JSON array: %s (%v)", value, err)
	}
	return result
}

func stringValue(t *testing.T, value json.RawMessage) string {
	t.Helper()
	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		t.Fatalf("value is not a JSON string: %s (%v)", value, err)
	}
	return result
}

func optionalStringValue(value json.RawMessage) (string, bool) {
	if len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return "", false
	}
	var result string
	if json.Unmarshal(value, &result) != nil {
		return "", false
	}
	return result, true
}

func numberValue(t *testing.T, value json.RawMessage) float64 {
	t.Helper()
	var result float64
	if err := json.Unmarshal(value, &result); err != nil {
		t.Fatalf("value is not a JSON number: %s (%v)", value, err)
	}
	return result
}

func boolValue(t *testing.T, value json.RawMessage) bool {
	t.Helper()
	var result bool
	if err := json.Unmarshal(value, &result); err != nil {
		t.Fatalf("value is not a JSON boolean: %s (%v)", value, err)
	}
	return result
}

func versionedMCPArguments(t *testing.T, payload any, documentID string, revision uint64) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode MCP arguments: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode MCP arguments: %v", err)
	}
	fields["document_id"], _ = json.Marshal(documentID)
	fields["expected_revision"], _ = json.Marshal(revision)
	encoded, err = json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode versioned MCP arguments: %v", err)
	}
	return encoded
}
