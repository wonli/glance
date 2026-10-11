package mcpserver

import (
	"bytes"
	"context"
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

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "spreadsheet.zoom.apply", Arguments: map[string]any{
		"document_id": doc.DocumentID, "expected_revision": doc.Revision, "operation": "zoom_in",
	}})
	if err != nil {
		t.Fatalf("view tool transport error: %v", err)
	}
	if !result.IsError || mapValue(t, result.StructuredContent)["error"] != "no_attached_view" {
		t.Fatalf("view capability without attached window = %+v; want no_attached_view", result)
	}
	current, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || current.Revision != doc.Revision {
		t.Fatalf("view request changed document revision: current=%+v error=%v", current, err)
	}
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

	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: map[string]any{"format": "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create Word tool result = %+v, %v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))

	inspected, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.inspect", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "limit": 10,
	}})
	if err != nil || inspected.IsError {
		t.Fatalf("inspect Word tool result = %+v, %v", inspected, err)
	}
	data := mapValue(t, mapValue(t, inspected.StructuredContent)["data"])
	content := mapValue(t, data["content"])
	blocks, ok := content["blocks"].([]any)
	if !ok || len(blocks) == 0 {
		t.Fatalf("Word inspection content = %#v", data["content"])
	}
	runs, ok := mapValue(t, blocks[0])["runs"].([]any)
	if !ok || len(runs) == 0 {
		t.Fatalf("blank Word document runs = %#v", blocks[0])
	}
	runID := stringValue(t, mapValue(t, runs[0])["id"])
	styled, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.style.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "run_id": runID, "bold": true,
	}})
	if err != nil || styled.IsError {
		t.Fatalf("style Word tool result = %+v, %v", styled, err)
	}
	styledRevision := uint64(numberValue(t, mapValue(t, styled.StructuredContent)["revision"]))
	if styledRevision != revision+1 {
		t.Fatalf("styled revision = %d; want %d", styledRevision, revision+1)
	}

	changed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.replace", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": styledRevision,
		"idempotency_key": "word-edit-once", "run_id": runID, "value": "通过 MCP 写入的周报内容",
	}})
	if err != nil || changed.IsError {
		t.Fatalf("edit Word tool result = %+v, %v", changed, err)
	}
	updatedRevision := uint64(numberValue(t, mapValue(t, changed.StructuredContent)["revision"]))
	if updatedRevision != styledRevision+1 {
		t.Fatalf("updated revision = %d; want %d", updatedRevision, styledRevision+1)
	}
	stale, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.replace", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "run_id": runID, "value": "过期修改",
	}})
	if err != nil {
		t.Fatalf("stale tool call transport error: %v", err)
	}
	if !stale.IsError || mapValue(t, stale.StructuredContent)["error"] != "revision_conflict" {
		t.Fatalf("stale MCP response = %+v; want structured revision_conflict", stale)
	}
	appended, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": updatedRevision,
		"idempotency_key": "word-report-paragraph-once", "text": "本周新增的报告段落",
	}})
	if err != nil || appended.IsError {
		t.Fatalf("append Word report paragraph result = %+v, %v", appended, err)
	}
	finalRevision := uint64(numberValue(t, mapValue(t, appended.StructuredContent)["revision"]))
	if finalRevision != updatedRevision+1 {
		t.Fatalf("appended revision = %d; want %d", finalRevision, updatedRevision+1)
	}
	appendedData := mapValue(t, mapValue(t, appended.StructuredContent)["data"])
	paragraphID := stringValue(t, appendedData["paragraph_id"])
	aligned, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.alignment.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "paragraph_id": paragraphID, "alignment": "center",
	}})
	if err != nil || aligned.IsError {
		t.Fatalf("align Word paragraph tool result = %+v, %v", aligned, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, aligned.StructuredContent)["revision"]))
	indented, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.indent.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "paragraph_id": paragraphID,
		"left_indent_points": 36, "right_indent_points": 18,
	}})
	if err != nil || indented.IsError {
		t.Fatalf("indent Word paragraph tool result = %+v, %v", indented, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, indented.StructuredContent)["revision"]))
	family, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.font_family.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "run_id": stringValue(t, mapValue(t, appendedData)["run_id"]), "font_family": "Arial",
	}})
	if err != nil || family.IsError {
		t.Fatalf("set Word font family tool result = %+v, %v", family, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, family.StructuredContent)["revision"]))
	sized, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.font_size.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "run_id": stringValue(t, mapValue(t, appendedData)["run_id"]), "font_size_points": 16,
	}})
	if err != nil || sized.IsError {
		t.Fatalf("set Word font size tool result = %+v, %v", sized, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, sized.StructuredContent)["revision"]))
	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.list.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "paragraph_id": paragraphID, "list_type": "bullet",
	}})
	if err != nil || listed.IsError {
		t.Fatalf("set Word paragraph list tool result = %+v, %v", listed, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, listed.StructuredContent)["revision"]))
	spacing, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.spacing.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "paragraph_id": paragraphID, "before_points": 12, "after_points": 8,
	}})
	if err != nil || spacing.IsError {
		t.Fatalf("set Word paragraph spacing tool result = %+v, %v", spacing, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, spacing.StructuredContent)["revision"]))
	ranged, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_range.replace", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "run_id": stringValue(t, mapValue(t, appendedData)["run_id"]),
		"start": 2, "end": 4, "replacement": "完成",
	}})
	if err != nil || ranged.IsError {
		t.Fatalf("replace Word text range tool result = %+v, %v", ranged, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, ranged.StructuredContent)["revision"]))
	searched, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text.search", Arguments: map[string]any{
		"document_id": documentID, "query": "完成的", "limit": 10,
	}})
	if err != nil || searched.IsError {
		t.Fatalf("search Word text tool result = %+v, %v", searched, err)
	}
	searchData := mapValue(t, mapValue(t, searched.StructuredContent)["data"])
	searchMatches, ok := searchData["matches"].([]any)
	if !ok || len(searchMatches) != 1 {
		t.Fatalf("MCP text search matches = %#v", searchData)
	}
	searchMatch := mapValue(t, searchMatches[0])
	if numberValue(t, searchMatch["start"]) != 2 || numberValue(t, searchMatch["end"]) != 5 || searchMatch["run_id"] != mapValue(t, appendedData)["run_id"] {
		t.Fatalf("MCP text search returned wrong Unicode range: %#v", searchMatch)
	}
	if uint64(numberValue(t, mapValue(t, searched.StructuredContent)["revision"])) != finalRevision {
		t.Fatalf("read-only MCP search changed revision from %d", finalRevision)
	}
	table, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.table.insert", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "after_paragraph_id": paragraphID, "rows": 2, "columns": 3,
	}})
	if err != nil || table.IsError {
		t.Fatalf("insert Word table tool result = %+v, %v", table, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, table.StructuredContent)["revision"]))
	if stringValue(t, mapValue(t, mapValue(t, table.StructuredContent)["data"])["table_id"]) == "" {
		t.Fatalf("MCP table insert returned no table ID: %+v", table.StructuredContent)
	}
	textBox, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_box.insert", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "after_paragraph_id": paragraphID, "text": "本周重点：完成工资核算",
	}})
	if err != nil || textBox.IsError {
		t.Fatalf("insert Word text box tool result = %+v, %v", textBox, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, textBox.StructuredContent)["revision"]))
	header, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.header.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "text": "周报",
	}})
	if err != nil || header.IsError {
		t.Fatalf("set Word header tool result = %+v, %v", header, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, header.StructuredContent)["revision"]))
	footer, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.footer.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "text": "人力资源部",
	}})
	if err != nil || footer.IsError {
		t.Fatalf("set Word footer tool result = %+v, %v", footer, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, footer.StructuredContent)["revision"]))
	pageNumber, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.page_number.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "alignment": "right",
	}})
	if err != nil || pageNumber.IsError {
		t.Fatalf("set Word page number tool result = %+v, %v", pageNumber, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, pageNumber.StructuredContent)["revision"]))
	for _, command := range []struct {
		name string
		args map[string]any
	}{
		{"word.page_setup.orientation.set", map[string]any{"orientation": "landscape"}},
		{"word.page_setup.margins.set", map[string]any{"top_points": 60, "right_points": 60, "bottom_points": 60, "left_points": 60}},
		{"word.page_setup.size.set", map[string]any{"width_points": 792, "height_points": 612}},
		{"word.page_setup.columns.set", map[string]any{"columns": 2}},
		{"word.page_setup.line_numbers.set", map[string]any{"mode": "continuous"}},
		{"word.page_setup.borders.set", map[string]any{"border_type": "dashed"}},
	} {
		arguments := make(map[string]any, len(command.args)+2)
		arguments["document_id"] = documentID
		arguments["expected_revision"] = finalRevision
		for key, value := range command.args {
			arguments[key] = value
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: command.name, Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("call %s tool result = %+v, %v", command.name, result, err)
		}
		finalRevision = uint64(numberValue(t, mapValue(t, result.StructuredContent)["revision"]))
	}
	language, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.proofing.language.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision,
		"run_id": stringValue(t, mapValue(t, appendedData)["run_id"]), "language_code": "zh-CN",
	}})
	if err != nil || language.IsError {
		t.Fatalf("set Word proofing language tool result = %+v, %v", language, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, language.StructuredContent)["revision"]))
	statistics, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text.statistics", Arguments: map[string]any{
		"document_id": documentID, "start_block": 0, "max_blocks": 20,
	}})
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
	tableSnapshot, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.inspect", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "limit": 10,
	}})
	if err != nil || tableSnapshot.IsError {
		t.Fatalf("inspect MCP-inserted table = %+v, %v", tableSnapshot, err)
	}
	inspectedData := mapValue(t, mapValue(t, tableSnapshot.StructuredContent)["data"])
	inspectedContent := mapValue(t, inspectedData["content"])
	inspectedBlocks, ok := inspectedContent["blocks"].([]any)
	if !ok {
		t.Fatalf("MCP table inspection blocks = %#v", inspectedContent["blocks"])
	}
	var cellRunID string
	for _, value := range inspectedBlocks {
		block := mapValue(t, value)
		if block["kind"] != "table" {
			continue
		}
		runs, _ := block["runs"].([]any)
		for _, runValue := range runs {
			run := mapValue(t, runValue)
			if run["editable"] == true {
				cellRunID = stringValue(t, run["id"])
				break
			}
		}
	}
	if cellRunID == "" {
		t.Fatalf("MCP table inspection did not expose an editable cell run: %#v", inspectedBlocks)
	}
	cellFilled, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.replace", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "run_id": cellRunID, "value": "员工",
	}})
	if err != nil || cellFilled.IsError {
		t.Fatalf("fill MCP-inserted table cell = %+v, %v", cellFilled, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, cellFilled.StructuredContent)["revision"]))
	pageBreak, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.page_break.insert", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision,
		"run_id": stringValue(t, mapValue(t, appendedData)["run_id"]), "start": 9, "end": 9, "break_type": "column",
	}})
	if err != nil || pageBreak.IsError {
		t.Fatalf("insert Word column break tool result = %+v, %v", pageBreak, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, pageBreak.StructuredContent)["revision"]))
	pageBreakData := mapValue(t, mapValue(t, pageBreak.StructuredContent)["data"])
	if stringValue(t, pageBreakData["break_type"]) != "column" {
		t.Fatalf("Word break result type = %#v; want column", pageBreakData["break_type"])
	}
	propertiesSet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.document.properties.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision,
		"title": "通过 MCP 设置的周报", "creator": "Glance Agent",
	}})
	if err != nil || propertiesSet.IsError {
		t.Fatalf("set Word document properties tool result = %+v, %v", propertiesSet, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, propertiesSet.StructuredContent)["revision"]))
	propertiesGet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.document.properties.get", Arguments: map[string]any{
		"document_id": documentID,
	}})
	if err != nil || propertiesGet.IsError {
		t.Fatalf("get Word document properties tool result = %+v, %v", propertiesGet, err)
	}
	propertiesData := mapValue(t, mapValue(t, propertiesGet.StructuredContent)["data"])
	if stringValue(t, propertiesData["title"]) != "通过 MCP 设置的周报" || stringValue(t, propertiesData["creator"]) != "Glance Agent" {
		t.Fatalf("Word document properties = %#v", propertiesData)
	}
	backgroundSet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.page.background.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "color": "#E3F2FD",
	}})
	if err != nil || backgroundSet.IsError {
		t.Fatalf("set Word page background tool result = %+v, %v", backgroundSet, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, backgroundSet.StructuredContent)["revision"]))
	backgroundGet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.page.background.get", Arguments: map[string]any{
		"document_id": documentID,
	}})
	if err != nil || backgroundGet.IsError {
		t.Fatalf("get Word page background tool result = %+v, %v", backgroundGet, err)
	}
	if stringValue(t, mapValue(t, mapValue(t, backgroundGet.StructuredContent)["data"])["color"]) != "E3F2FD" {
		t.Fatalf("Word page background result = %+v", backgroundGet.StructuredContent)
	}
	watermarkSet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.watermark.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "text": "机密",
	}})
	if err != nil || watermarkSet.IsError {
		t.Fatalf("set Word watermark tool result = %+v, %v", watermarkSet, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, watermarkSet.StructuredContent)["revision"]))
	watermarkGet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.watermark.get", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision,
	}})
	if err != nil || watermarkGet.IsError {
		t.Fatalf("get Word watermark tool result = %+v, %v", watermarkGet, err)
	}
	if stringValue(t, mapValue(t, mapValue(t, watermarkGet.StructuredContent)["data"])["text"]) != "机密" {
		t.Fatalf("Word watermark result = %+v", watermarkGet.StructuredContent)
	}
	trackingSet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.review.track.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "enabled": true,
	}})
	if err != nil || trackingSet.IsError {
		t.Fatalf("set Word revision tracking tool result = %+v, %v", trackingSet, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, trackingSet.StructuredContent)["revision"]))
	trackingGet, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.review.track.get", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision,
	}})
	if err != nil || trackingGet.IsError {
		t.Fatalf("get Word revision tracking tool result = %+v, %v", trackingGet, err)
	}
	if mapValue(t, mapValue(t, trackingGet.StructuredContent)["data"])["enabled"] != true {
		t.Fatalf("Word revision tracking result = %+v", trackingGet.StructuredContent)
	}
	blankPage, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.blank_page.insert", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision,
		"run_id": stringValue(t, pageBreakData["caret_run_id"]), "start": 0, "end": 0,
	}})
	if err != nil || blankPage.IsError {
		t.Fatalf("insert Word blank page tool result = %+v, %v", blankPage, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, blankPage.StructuredContent)["revision"]))
	blankPageData := mapValue(t, mapValue(t, blankPage.StructuredContent)["data"])
	bookmark, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.bookmark.insert", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision,
		"run_id": stringValue(t, blankPageData["caret_run_id"]), "name": "ReportBody",
	}})
	if err != nil || bookmark.IsError {
		t.Fatalf("insert Word bookmark tool result = %+v, %v", bookmark, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, bookmark.StructuredContent)["revision"]))
	link, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.hyperlink.insert", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision,
		"run_id": stringValue(t, blankPageData["caret_run_id"]), "text": "查看周报", "target_url": "https://example.com/report",
	}})
	if err != nil || link.IsError {
		t.Fatalf("insert Word hyperlink tool result = %+v, %v", link, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, link.StructuredContent)["revision"]))
	cover, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.cover_page.insert", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision,
		"title": "周报", "subtitle": "本周工作", "author": "运营部",
	}})
	if err != nil || cover.IsError {
		t.Fatalf("insert Word cover page tool result = %+v, %v", cover, err)
	}
	finalRevision = uint64(numberValue(t, mapValue(t, cover.StructuredContent)["revision"]))

	path := filepath.Join(t.TempDir(), "周报.docx")
	saved, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.save_as", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision, "path": path,
	}})
	if err != nil || saved.IsError {
		t.Fatalf("save Word tool result = %+v, %v", saved, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("MCP did not create Word file: %v", err)
	}
	closed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.close", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": finalRevision,
	}})
	if err != nil || closed.IsError {
		t.Fatalf("close Word tool result = %+v, %v", closed, err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := session.CallTool(canceled, &mcp.CallToolParams{Name: "document.list", Arguments: map[string]any{}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled MCP call error = %v; want context canceled", err)
	}
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

	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: map[string]any{"format": "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create Word document: result=%+v error=%v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))

	appended, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "text": "ABC🙂DEF",
	}})
	if err != nil || appended.IsError {
		t.Fatalf("append source text: result=%+v error=%v", appended, err)
	}
	appendedData := mapValue(t, mapValue(t, appended.StructuredContent)["data"])
	runID := stringValue(t, appendedData["run_id"])
	revision = uint64(numberValue(t, mapValue(t, appended.StructuredContent)["revision"]))

	copied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_range.copy", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "run_id": runID, "start": 3, "end": 4,
	}})
	if err != nil || copied.IsError {
		t.Fatalf("copy Unicode range: result=%+v error=%v", copied, err)
	}
	copyData := mapValue(t, mapValue(t, copied.StructuredContent)["data"])
	if copyData["text"] != "🙂" || uint64(numberValue(t, mapValue(t, copied.StructuredContent)["revision"])) != revision {
		t.Fatalf("copy result=%+v; want emoji and unchanged revision %d", copied.StructuredContent, revision)
	}

	cut, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_range.cut", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "run_id": runID, "start": 3, "end": 4,
	}})
	if err != nil || cut.IsError {
		t.Fatalf("cut Unicode range: result=%+v error=%v", cut, err)
	}
	cutData := mapValue(t, mapValue(t, cut.StructuredContent)["data"])
	revision = uint64(numberValue(t, mapValue(t, cut.StructuredContent)["revision"]))
	if cutData["text"] != "🙂" || cutData["changed"] != true || revision < 2 {
		t.Fatalf("cut result=%+v; want deleted emoji and revision advance", cut.StructuredContent)
	}

	pasted, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_range.replace", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "run_id": runID, "start": 3, "end": 3, "replacement": "🙂",
	}})
	if err != nil || pasted.IsError {
		t.Fatalf("paste into empty range: result=%+v error=%v", pasted, err)
	}

	inspected, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.inspect", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": uint64(numberValue(t, mapValue(t, pasted.StructuredContent)["revision"])), "limit": 10,
	}})
	if err != nil || inspected.IsError {
		t.Fatalf("inspect pasted text: result=%+v error=%v", inspected, err)
	}
	inspectData := mapValue(t, mapValue(t, inspected.StructuredContent)["data"])
	content := mapValue(t, inspectData["content"])
	blocks, ok := content["blocks"].([]any)
	if !ok || len(blocks) < 1 {
		t.Fatalf("inspected blocks = %#v", content["blocks"])
	}
	var foundText string
	for _, block := range blocks {
		if text, ok := mapValue(t, block)["text"].(string); ok && text != "" {
			foundText = text
		}
	}
	if foundText != "ABC🙂DEF" {
		t.Fatalf("pasted text = %#v; want %q", foundText, "ABC🙂DEF")
	}
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
	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: map[string]any{"format": "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create Word document: result=%+v error=%v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))
	appendParagraph := func(text string) string {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: map[string]any{
			"document_id": documentID, "expected_revision": revision, "text": text,
		}})
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
	styled, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_run.style.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "run_id": sourceRunID, "bold": true,
	}})
	if err != nil || styled.IsError {
		t.Fatalf("style source run: result=%+v error=%v", styled, err)
	}
	revision = uint64(numberValue(t, mapValue(t, styled.StructuredContent)["revision"]))
	painted, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.text_format.paint", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "source_run_id": sourceRunID, "target_run_id": targetRunID,
	}})
	if err != nil || painted.IsError {
		t.Fatalf("paint target run: result=%+v error=%v", painted, err)
	}
	paintData := mapValue(t, mapValue(t, painted.StructuredContent)["data"])
	if paintData["changed"] != true || uint64(numberValue(t, mapValue(t, painted.StructuredContent)["revision"])) != revision+1 {
		t.Fatalf("paint result=%+v; want changed and one revision advance", painted.StructuredContent)
	}
	inspected, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.inspect", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision + 1, "limit": 10,
	}})
	if err != nil || inspected.IsError {
		t.Fatalf("inspect painted document: result=%+v error=%v", inspected, err)
	}
	content := mapValue(t, mapValue(t, mapValue(t, inspected.StructuredContent)["data"])["content"])
	blocks, ok := content["blocks"].([]any)
	if !ok {
		t.Fatalf("inspected blocks = %#v", content["blocks"])
	}
	for _, block := range blocks {
		runs, _ := mapValue(t, block)["runs"].([]any)
		for _, run := range runs {
			runData := mapValue(t, run)
			style := mapValue(t, runData["style"])
			if runData["id"] == targetRunID && style["bold"] == true && runData["text"] == "Target" {
				return
			}
		}
	}
	t.Fatalf("MCP format painter did not preserve target text and copy bold style: %s", inspected.StructuredContent)
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
	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: map[string]any{"format": "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create Word document: %+v, %v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))
	appended, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "text": "图片说明",
	}})
	if err != nil || appended.IsError {
		t.Fatalf("append image target paragraph: %+v, %v", appended, err)
	}
	revision = uint64(numberValue(t, mapValue(t, appended.StructuredContent)["revision"]))
	paragraphData := mapValue(t, mapValue(t, appended.StructuredContent)["data"])
	spaced, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.spacing_all.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "before_points": 0, "after_points": 4,
	}})
	if err != nil || spaced.IsError {
		t.Fatalf("set all paragraph spacing over MCP: %+v, %v", spaced, err)
	}
	revision = uint64(numberValue(t, mapValue(t, spaced.StructuredContent)["revision"]))
	imagePath := writeMCPWordImage(t)
	inserted, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.image.insert", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision,
		"paragraph_id":    stringValue(t, paragraphData["paragraph_id"]),
		"after_inline_id": stringValue(t, paragraphData["run_id"]), "path": imagePath,
	}})
	if err != nil || inserted.IsError {
		t.Fatalf("insert MCP Word image: %+v, %v", inserted, err)
	}
	revision = uint64(numberValue(t, mapValue(t, inserted.StructuredContent)["revision"]))
	imageData := mapValue(t, mapValue(t, inserted.StructuredContent)["data"])
	imageID := stringValue(t, imageData["image_id"])
	rotated, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.image.rotate", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "image_id": imageID, "delta_degrees": -90,
	}})
	if err != nil || rotated.IsError {
		t.Fatalf("rotate MCP Word image: %+v, %v", rotated, err)
	}
	rotatedData := mapValue(t, mapValue(t, rotated.StructuredContent)["data"])
	if numberValue(t, rotatedData["rotation_degrees"]) != 270 {
		t.Fatalf("MCP image rotation result = %#v", rotatedData)
	}
	revision = uint64(numberValue(t, mapValue(t, rotated.StructuredContent)["revision"]))
	resized, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.image.resize", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "image_id": imageID, "width_points": 72,
	}})
	if err != nil || resized.IsError {
		t.Fatalf("resize MCP Word image: %+v, %v", resized, err)
	}
	revision = uint64(numberValue(t, mapValue(t, resized.StructuredContent)["revision"]))
	deleted, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.image.delete", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "image_id": imageID,
	}})
	if err != nil || deleted.IsError {
		t.Fatalf("delete MCP Word image: %+v, %v", deleted, err)
	}
	deleteData := mapValue(t, mapValue(t, deleted.StructuredContent)["data"])
	if deleteData["status"] != "confirmation_required" || stringValue(t, deleteData["confirmation_id"]) == "" {
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
	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.list", Arguments: map[string]any{}})
	if err != nil || listed.IsError {
		t.Fatalf("list independent Runtime documents = %+v, %v", listed, err)
	}
	items, ok := mapValue(t, listed.StructuredContent)["data"].([]any)
	if !ok || len(items) != 0 {
		t.Fatalf("independent Runtime saw desktop documents: %#v", listed.StructuredContent)
	}
	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: map[string]any{"format": "docx"}})
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

func mapValue(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value has type %T, want object: %#v", value, value)
	}
	return result
}

func stringValue(t *testing.T, value any) string {
	t.Helper()
	result, ok := value.(string)
	if !ok {
		t.Fatalf("value has type %T, want string: %#v", value, value)
	}
	return result
}

func numberValue(t *testing.T, value any) float64 {
	t.Helper()
	result, ok := value.(float64)
	if !ok {
		t.Fatalf("value has type %T, want number: %#v", value, value)
	}
	return result
}
