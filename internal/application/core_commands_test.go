package application

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestRuntimeRegistersCoreAgentEditCapabilities(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	var ids []string
	for _, descriptor := range runtime.Catalog().Descriptors() {
		ids = append(ids, descriptor.ID)
	}
	want := []string{
		"confirmation.get", "document.close", "document.compare", "document.create", "document.export_copy", "document.get", "document.inspect", "document.list", "document.open", "document.plan.apply", "document.plan.preview", "document.plan.validate", "document.save", "document.save_as",
		"json.document.read_text", "json.document.search", "json.document.validate", "json.text.transform", "json.view.set", "markdown.document.export_pdf", "markdown.text.edit", "markdown.view.set",
		"pdf.annotation.highlight", "pdf.annotation.ink", "pdf.annotation.note", "pdf.document.print", "pdf.document.search", "pdf.history.apply", "pdf.page.rotate", "pdf.text.replace", "pdf.view.get", "pdf.view.set",
		"presentation.animations.list", "presentation.comment.add", "presentation.comment.delete", "presentation.comments.list", "presentation.document.export_pdf", "presentation.document.print", "presentation.element.animation.add", "presentation.element.animation.delete", "presentation.element.animation.duration.set", "presentation.element.audio.create", "presentation.element.chart.create", "presentation.element.group.set", "presentation.element.picture.create", "presentation.element.picture.crop.set", "presentation.element.picture.replace", "presentation.element.shape.create", "presentation.element.style.set", "presentation.element.table.create", "presentation.element.text.copy", "presentation.element.text.set", "presentation.element.textbox.create", "presentation.element.transform.set", "presentation.element.video.create", "presentation.layouts.list", "presentation.slide.add", "presentation.slide.background.set", "presentation.slide.delete", "presentation.slide.duplicate", "presentation.slide.layout.set", "presentation.slide.notes.set", "presentation.slide.size.set", "presentation.text.proofread", "presentation.theme.palette.set", "presentation.theme.set", "presentation.transition.get", "presentation.transition.set", "presentation.view.get", "presentation.view.set",
		"spreadsheet.cell.set", "spreadsheet.cells.set", "spreadsheet.chart.create", "spreadsheet.comment.add", "spreadsheet.comment.delete", "spreadsheet.comments.get", "spreadsheet.conditional_format.set", "spreadsheet.data_validation.set", "spreadsheet.defined_names.delete", "spreadsheet.defined_names.get", "spreadsheet.defined_names.set", "spreadsheet.document.export_pdf", "spreadsheet.document.print", "spreadsheet.duplicates.remove", "spreadsheet.filter.apply", "spreadsheet.flash_fill.apply", "spreadsheet.format.paint", "spreadsheet.formula.set", "spreadsheet.gridlines.get", "spreadsheet.gridlines.set", "spreadsheet.history.apply", "spreadsheet.hyperlink.get", "spreadsheet.hyperlink.remove", "spreadsheet.hyperlink.set", "spreadsheet.image.delete", "spreadsheet.image.insert", "spreadsheet.image.move", "spreadsheet.image.replace", "spreadsheet.image.resize", "spreadsheet.images.list", "spreadsheet.page_layout.get", "spreadsheet.page_layout.set", "spreadsheet.panes.get", "spreadsheet.panes.set", "spreadsheet.pivot.create", "spreadsheet.protection.get", "spreadsheet.protection.set", "spreadsheet.range.copy", "spreadsheet.range.format.set", "spreadsheet.range.search", "spreadsheet.range.sort", "spreadsheet.shape.create", "spreadsheet.structure.edit", "spreadsheet.table.create", "spreadsheet.text.proofread", "spreadsheet.text_to_columns.split", "spreadsheet.view.get", "spreadsheet.view.mode.set", "spreadsheet.view.ruler.set", "spreadsheet.workbook.recalculate", "spreadsheet.zoom.apply", "spreadsheet.zoom.get",
		"word.addins.list", "word.autosave.set", "word.blank_page.insert", "word.bookmark.insert", "word.comment.add", "word.comment.delete", "word.comments.list", "word.cover_page.insert", "word.document.export_pdf", "word.document.print", "word.document.properties.get", "word.document.properties.set", "word.document.protection.get", "word.document.protection.set", "word.equation.insert", "word.footer.set", "word.header.set", "word.hyperlink.insert", "word.image.delete", "word.image.insert", "word.image.move", "word.image.replace", "word.image.resize", "word.image.rotate", "word.image.wrap.set", "word.macros.inspect", "word.page.background.get", "word.page.background.set", "word.page_break.insert", "word.page_number.set", "word.page_setup.borders.set", "word.page_setup.columns.set", "word.page_setup.get", "word.page_setup.line_numbers.set", "word.page_setup.margins.set", "word.page_setup.orientation.set", "word.page_setup.size.set", "word.paragraph.alignment.set", "word.paragraph.append", "word.paragraph.indent.set", "word.paragraph.list.set", "word.paragraph.spacing.set", "word.paragraph.spacing_all.set", "word.proofing.language.set", "word.review.editor.analyze", "word.review.revisions.apply", "word.review.track.get", "word.review.track.set", "word.table.insert", "word.text.insert", "word.text.search", "word.text.statistics", "word.text_box.insert", "word.text_format.paint", "word.text_range.copy", "word.text_range.cut", "word.text_range.replace", "word.text_run.font_family.set", "word.text_run.font_size.set", "word.text_run.replace", "word.text_run.style.set", "word.theme.effects.set", "word.version.list", "word.view.get", "word.view.new_window", "word.view.set", "word.view.side_by_side", "word.watermark.get", "word.watermark.set", "word.zoom.dialog", "workbook.audit", "workspace.recent.list",
	}
	if len(ids) != len(want) {
		t.Fatalf("registered capabilities = %v; want %v", ids, want)
	}
	for index := range want {
		if ids[index] != want[index] {
			t.Fatalf("registered capabilities = %v; want %v", ids, want)
		}
	}
}

func TestWordHeaderFooterCapabilitiesRoundTrip(t *testing.T) {
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
	operator := Operator{ID: "agent-1", Kind: OperatorBuiltInAgent}
	revision := doc.Revision
	commands := []struct {
		id   string
		args map[string]any
	}{
		{"word.header.set", map[string]any{"text": "季度经营报告"}},
		{"word.footer.set", map[string]any{"text": "仅供内部使用"}},
		{"word.page_number.set", map[string]any{"alignment": "center"}},
	}
	for index, command := range commands {
		result, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: "word-header-footer-" + command.id, DocumentID: doc.DocumentID, CommandID: command.id,
			ExpectedRevision: revision, Operator: operator, Arguments: mustJSON(t, command.args),
		})
		if err != nil {
			t.Fatalf("execute %s: %v", command.id, err)
		}
		if result.Revision != revision+1 {
			t.Fatalf("%s revision = %d; want %d", command.id, result.Revision, revision+1)
		}
		revision = result.Revision
		var data struct {
			Changed bool `json:"changed"`
		}
		if err := json.Unmarshal(result.Data, &data); err != nil || !data.Changed {
			t.Fatalf("%s result = %s, error=%v", command.id, result.Data, err)
		}
		repeated, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: "word-header-footer-noop-" + command.id, DocumentID: doc.DocumentID, CommandID: command.id,
			ExpectedRevision: revision, Operator: operator, Arguments: mustJSON(t, command.args),
		})
		if err != nil {
			t.Fatalf("repeat %s: %v", command.id, err)
		}
		var repeatedData struct {
			Changed bool `json:"changed"`
		}
		if err := json.Unmarshal(repeated.Data, &repeatedData); err != nil || repeatedData.Changed || repeated.Revision != revision {
			t.Fatalf("repeat %s was not a no-op: result=%s revision=%d error=%v want=%d", command.id, repeated.Data, repeated.Revision, err, revision)
		}
		state, err := runtime.GetDocument(ctx, doc.DocumentID)
		if err != nil || state.Revision != revision || !state.Dirty {
			t.Fatalf("%s change/no-op state=%+v error=%v; want dirty revision %d", command.id, state, err, revision)
		}
		if index == 0 && revision != doc.Revision+1 {
			t.Fatalf("first header edit revision = %d; document started at %d", revision, doc.Revision)
		}
	}

	path := filepath.Join(t.TempDir(), "header-footer.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if saved.Dirty {
		t.Fatalf("saved header/footer document remains dirty: %+v", saved)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open saved DOCX: %v", err)
	}
	defer archive.Close()
	parts := make(map[string]string, len(archive.File))
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open DOCX part %s: %v", file.Name, err)
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read DOCX part %s: read=%v close=%v", file.Name, readErr, closeErr)
		}
		parts[file.Name] = string(data)
	}
	if !strings.Contains(parts["word/header1.xml"], "季度经营报告") {
		t.Fatalf("saved DOCX header is missing: %s", parts["word/header1.xml"])
	}
	if !strings.Contains(parts["word/footer1.xml"], "仅供内部使用") || !strings.Contains(parts["word/footer1.xml"], "PAGE") {
		t.Fatalf("saved DOCX footer did not preserve text with page number: %s", parts["word/footer1.xml"])
	}
}

func TestWordPageSetupCapabilitiesRoundTrip(t *testing.T) {
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
	commands := []struct {
		id   string
		args map[string]any
	}{
		{"word.page_setup.orientation.set", map[string]any{"orientation": "landscape"}},
		{"word.page_setup.margins.set", map[string]any{"top_points": 54, "right_points": 60, "bottom_points": 66, "left_points": 48}},
		{"word.page_setup.size.set", map[string]any{"width_points": 792, "height_points": 612}},
		{"word.page_setup.columns.set", map[string]any{"columns": 2}},
		{"word.page_setup.line_numbers.set", map[string]any{"mode": "continuous"}},
		{"word.page_setup.borders.set", map[string]any{"border_type": "dashed"}},
	}
	revision := doc.Revision
	for _, command := range commands {
		result, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: "page-setup-" + command.id, DocumentID: doc.DocumentID, CommandID: command.id,
			ExpectedRevision: revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent}, Arguments: mustJSON(t, command.args),
		})
		if err != nil {
			t.Fatalf("execute %s: %v", command.id, err)
		}
		if result.Revision != revision+1 {
			t.Fatalf("%s revision = %d; want %d", command.id, result.Revision, revision+1)
		}
		revision = result.Revision
	}
	stateAfterEdits, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterEdits.Revision != revision || !stateAfterEdits.Dirty {
		t.Fatalf("page setup state after edits=%+v revision=%d error=%v", stateAfterEdits, revision, err)
	}
	for _, command := range commands {
		result, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: "page-setup-noop-" + command.id, DocumentID: doc.DocumentID, CommandID: command.id,
			ExpectedRevision: revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent}, Arguments: mustJSON(t, command.args),
		})
		if err != nil {
			t.Fatalf("repeat %s: %v", command.id, err)
		}
		var data struct {
			Changed bool `json:"changed"`
		}
		if err := json.Unmarshal(result.Data, &data); err != nil || data.Changed || result.Revision != revision {
			t.Fatalf("repeat %s was not a no-op: result=%s revision=%d error=%v want=%d", command.id, result.Data, result.Revision, err, revision)
		}
	}
	invalid := []struct {
		id   string
		args map[string]any
	}{
		{"word.page_setup.orientation.set", map[string]any{"orientation": "diagonal"}},
		{"word.page_setup.margins.set", map[string]any{"top_points": 1001, "right_points": 60, "bottom_points": 66, "left_points": 48}},
		{"word.page_setup.size.set", map[string]any{"width_points": 3001, "height_points": 612}},
		{"word.page_setup.columns.set", map[string]any{"columns": 5}},
		{"word.page_setup.line_numbers.set", map[string]any{"mode": "sometimes"}},
		{"word.page_setup.borders.set", map[string]any{"border_type": "unknown"}},
	}
	for index, command := range invalid {
		if _, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: fmt.Sprintf("page-setup-invalid-%d", index), DocumentID: doc.DocumentID, CommandID: command.id,
			ExpectedRevision: revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent}, Arguments: mustJSON(t, command.args),
		}); err == nil {
			t.Fatalf("%s accepted invalid parameters: %+v", command.id, command.args)
		}
	}
	if afterNoopsAndInvalid := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterNoopsAndInvalid.Revision != stateAfterEdits.Revision || afterNoopsAndInvalid.Dirty != stateAfterEdits.Dirty {
		t.Fatalf("no-op or invalid page setup changed document state: before=%+v after=%+v", stateAfterEdits, afterNoopsAndInvalid)
	}

	path := filepath.Join(t.TempDir(), "page-setup.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen page setup DOCX: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	layout := reopened.WordLayoutData()
	if len(layout.Sections) != 1 {
		t.Fatalf("page setup sections = %d; want 1", len(layout.Sections))
	}
	setup := layout.Sections[0].PageSetup
	if setup.Orientation != "landscape" || setup.WidthPT < 791 || setup.WidthPT > 793 || setup.HeightPT < 611 || setup.HeightPT > 613 || setup.Columns != 2 {
		t.Fatalf("page setup did not round-trip: %+v", setup)
	}
	if setup.MarginTopPT != 54 || setup.MarginRightPT != 60 || setup.MarginBottomPT != 66 || setup.MarginLeftPT != 48 {
		t.Fatalf("page margins did not round-trip: %+v", setup)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open page setup package: %v", err)
	}
	defer archive.Close()
	var documentXML string
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open document.xml: %v", err)
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read document.xml: read=%v close=%v", readErr, closeErr)
		}
		documentXML = string(data)
		break
	}
	if !strings.Contains(documentXML, "lnNumType") || !strings.Contains(documentXML, `restart="continuous"`) || !strings.Contains(documentXML, "pgBorders") || !strings.Contains(documentXML, `val="dashed"`) {
		t.Fatalf("line numbers or page borders did not round-trip: %s", documentXML)
	}
}

func TestWordLanguageAndStatisticsCapabilities(t *testing.T) {
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
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "append-language-test", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "你好 👋 Glance"}),
	})
	if err != nil {
		t.Fatalf("append language fixture: %v", err)
	}
	var appendedData struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(appended.Data, &appendedData); err != nil || appendedData.RunID == "" {
		t.Fatalf("decode language fixture: %v, data=%s", err, appended.Data)
	}
	language, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "set-proofing-language", DocumentID: doc.DocumentID, CommandID: "word.proofing.language.set",
		ExpectedRevision: appended.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"run_id": appendedData.RunID, "language_code": "zh-CN"}),
	})
	if err != nil {
		t.Fatalf("set Word proofing language: %v", err)
	}
	var languageData struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(language.Data, &languageData); err != nil || !languageData.Changed || language.Revision != appended.Revision+1 {
		t.Fatalf("initial proofing language result=%s revision=%d error=%v", language.Data, language.Revision, err)
	}
	noopLanguage, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "set-proofing-language-noop", DocumentID: doc.DocumentID, CommandID: "word.proofing.language.set",
		ExpectedRevision: language.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"run_id": appendedData.RunID, "language_code": "zh-CN"}),
	})
	var noopLanguageData struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noopLanguage.Data, &noopLanguageData) != nil || noopLanguageData.Changed || noopLanguage.Revision != language.Revision {
		t.Fatalf("repeat proofing language was not a no-op: data=%s revision=%d error=%v", noopLanguage.Data, noopLanguage.Revision, err)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "set-proofing-language-invalid-target", DocumentID: doc.DocumentID, CommandID: "word.proofing.language.set",
		ExpectedRevision: language.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"run_id": "missing-run", "language_code": "zh-CN"}),
	}); err == nil {
		t.Fatal("proofing language accepted a missing text run")
	}
	if afterInvalid := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterInvalid.Revision != language.Revision || !afterInvalid.Dirty {
		t.Fatalf("invalid proofing language target changed document state: %+v", afterInvalid)
	}
	statistics, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "read-word-statistics", DocumentID: doc.DocumentID, CapabilityID: "word.text.statistics",
		Operator:  Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]int{"start_block": 0, "max_blocks": 10}),
	})
	if err != nil {
		t.Fatalf("read Word text statistics: %v", err)
	}
	var result struct {
		CharactersWithWhitespace    int  `json:"characters_with_whitespace"`
		CharactersWithoutWhitespace int  `json:"characters_without_whitespace"`
		Paragraphs                  int  `json:"paragraphs"`
		Tables                      int  `json:"tables"`
		TotalBlocks                 int  `json:"total_blocks"`
		HasMore                     bool `json:"has_more"`
	}
	if err := json.Unmarshal(statistics.Data, &result); err != nil {
		t.Fatalf("decode Word statistics: %v, data=%s", err, statistics.Data)
	}
	if result.CharactersWithWhitespace != 11 || result.CharactersWithoutWhitespace != 9 || result.Paragraphs < 1 || result.Tables != 0 || result.HasMore {
		t.Fatalf("Word statistics = %+v; want rune counts 11/9 and complete paragraph page", result)
	}
	if statistics.Revision != language.Revision {
		t.Fatalf("read-only statistics revision = %d; want %d", statistics.Revision, language.Revision)
	}

	path := filepath.Join(t.TempDir(), "language.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open language DOCX: %v", err)
	}
	defer archive.Close()
	var documentXML string
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("open document.xml: %v", err)
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read document.xml: read=%v close=%v", readErr, closeErr)
		}
		documentXML = string(data)
		break
	}
	if !strings.Contains(documentXML, `w:lang w:val="zh-CN"`) {
		t.Fatalf("proofing language was not saved: %s", documentXML)
	}
}

func TestWordColumnBreakCapabilityRoundTrips(t *testing.T) {
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
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "column-break-text", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "左侧右侧"}),
	})
	if err != nil {
		t.Fatalf("append column break fixture: %v", err)
	}
	var paragraph struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(appended.Data, &paragraph); err != nil || paragraph.RunID == "" {
		t.Fatalf("decode column break fixture: %v, data=%s", err, appended.Data)
	}
	inserted, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-column-break", DocumentID: doc.DocumentID, CommandID: "word.page_break.insert",
		ExpectedRevision: appended.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]any{"run_id": paragraph.RunID, "start": 2, "end": 2, "break_type": "column"}),
	})
	if err != nil {
		t.Fatalf("insert Word column break: %v", err)
	}
	var result struct {
		BreakType string `json:"break_type"`
	}
	if err := json.Unmarshal(inserted.Data, &result); err != nil || result.BreakType != "column" {
		t.Fatalf("column break result = %+v, error=%v, data=%s", result, err, inserted.Data)
	}
	path := filepath.Join(t.TempDir(), "column-break.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen column break DOCX: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	for _, block := range reopened.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.PageBreak != nil && inline.PageBreak.Type == document.WordBreakColumn {
				return
			}
		}
	}
	t.Fatal("column break did not survive DOCX save and reopen")
}

func TestWordDocumentPropertiesCapabilitiesRoundTrip(t *testing.T) {
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
	updated, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "set-word-properties", DocumentID: doc.DocumentID, CommandID: "word.document.properties.set",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"title": "人力资源周报", "subject": "员工薪资核算", "creator": "人力资源部", "keywords": "周报,工资", "description": "第 42 周"}),
	})
	if err != nil {
		t.Fatalf("set Word document properties: %v", err)
	}
	var updateResult struct {
		Changed    bool                            `json:"changed"`
		Properties document.WordDocumentProperties `json:"properties"`
	}
	if err := json.Unmarshal(updated.Data, &updateResult); err != nil || !updateResult.Changed || updated.Revision != doc.Revision+1 {
		t.Fatalf("set Word properties result=%+v revision=%d error=%v data=%s", updateResult, updated.Revision, err, updated.Data)
	}
	noop, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "set-word-properties-noop", DocumentID: doc.DocumentID, CommandID: "word.document.properties.set",
		ExpectedRevision: updated.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"title": "人力资源周报", "subject": "员工薪资核算", "creator": "人力资源部", "keywords": "周报,工资", "description": "第 42 周"}),
	})
	var noopResult struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noop.Data, &noopResult) != nil || noopResult.Changed || noop.Revision != updated.Revision {
		t.Fatalf("repeat Word properties was not a no-op: result=%+v changed=%v error=%v", noop, noopResult.Changed, err)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "set-word-properties-invalid", DocumentID: doc.DocumentID, CommandID: "word.document.properties.set",
		ExpectedRevision: updated.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"title": strings.Repeat("甲", 4097)}),
	}); err == nil {
		t.Fatal("Word document properties accepted a title longer than the declared limit")
	}
	if afterInvalid := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterInvalid.Revision != updated.Revision || !afterInvalid.Dirty {
		t.Fatalf("invalid Word properties changed document state: %+v", afterInvalid)
	}
	queried, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "get-word-properties", DocumentID: doc.DocumentID, CapabilityID: "word.document.properties.get",
		Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent}, Arguments: mustJSON(t, map[string]any{}),
	})
	if err != nil {
		t.Fatalf("get Word document properties: %v", err)
	}
	var result struct {
		Title       string `json:"title"`
		Subject     string `json:"subject"`
		Creator     string `json:"creator"`
		Keywords    string `json:"keywords"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(queried.Data, &result); err != nil {
		t.Fatalf("decode Word properties: %v, data=%s", err, queried.Data)
	}
	if result.Title != "人力资源周报" || result.Subject != "员工薪资核算" || result.Creator != "人力资源部" || result.Keywords != "周报,工资" || result.Description != "第 42 周" {
		t.Fatalf("Word properties = %+v", result)
	}
	if queried.Revision != updated.Revision {
		t.Fatalf("property query revision = %d; want %d", queried.Revision, updated.Revision)
	}
	path := filepath.Join(t.TempDir(), "properties.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen property DOCX: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	properties, err := reopened.GetWordDocumentProperties()
	if err != nil || properties.Title != result.Title || properties.Description != result.Description {
		t.Fatalf("reopened Word properties = %+v, error=%v", properties, err)
	}
}

func TestWordPageBackgroundCapabilitySetsAndClears(t *testing.T) {
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
	set, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "set-word-page-background", DocumentID: doc.DocumentID, CommandID: "word.page.background.set",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"color": "#E3F2FD"}),
	})
	if err != nil {
		t.Fatalf("set Word page background: %v", err)
	}
	var setResult struct {
		Changed bool   `json:"changed"`
		Color   string `json:"color"`
	}
	if err := json.Unmarshal(set.Data, &setResult); err != nil || !setResult.Changed || setResult.Color != "E3F2FD" {
		t.Fatalf("set background result = %+v, error=%v, data=%s", setResult, err, set.Data)
	}
	stateAfterSet := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if stateAfterSet.Revision != set.Revision || !stateAfterSet.Dirty {
		t.Fatalf("Word page background set state = %+v; want dirty revision %d", stateAfterSet, set.Revision)
	}
	noopBackground, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "set-word-page-background-noop", DocumentID: doc.DocumentID, CommandID: "word.page.background.set",
		ExpectedRevision: set.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"color": "e3f2fd"}),
	})
	var noopResult struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noopBackground.Data, &noopResult) != nil || noopResult.Changed || noopBackground.Revision != set.Revision {
		t.Fatalf("repeat Word page background was not a no-op: result=%+v changed=%v error=%v", noopBackground, noopResult.Changed, err)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "set-word-page-background-invalid", DocumentID: doc.DocumentID, CommandID: "word.page.background.set",
		ExpectedRevision: set.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"color": "GHIJKL"}),
	}); err == nil {
		t.Fatal("Word page background accepted invalid hexadecimal color")
	}
	stateAfterNoopAndInvalid := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if stateAfterNoopAndInvalid.Revision != set.Revision || stateAfterNoopAndInvalid.Dirty != stateAfterSet.Dirty {
		t.Fatalf("Word page background no-op/invalid request changed state: before=%+v after=%+v", stateAfterSet, stateAfterNoopAndInvalid)
	}
	cleared, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "clear-word-page-background", DocumentID: doc.DocumentID, CommandID: "word.page.background.set",
		ExpectedRevision: set.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"color": "none"}),
	})
	if err != nil {
		t.Fatalf("clear Word page background: %v", err)
	}
	stateAfterClear := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if stateAfterClear.Revision != cleared.Revision || stateAfterClear.Revision != set.Revision+1 || !stateAfterClear.Dirty {
		t.Fatalf("Word page background clear state = %+v; want dirty revision %d", stateAfterClear, set.Revision+1)
	}
	path := filepath.Join(t.TempDir(), "page-background.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if saved.Dirty {
		t.Fatalf("saved page background document remains dirty: %+v", saved)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen page background DOCX: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	background, err := reopened.GetPageBackgroundColor()
	if err != nil || background != "" || cleared.Revision != set.Revision+1 {
		t.Fatalf("cleared page background = %q, revision=%d, error=%v", background, cleared.Revision, err)
	}
}

func TestWordFontListAndSpacingCapabilitiesRoundTrip(t *testing.T) {
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
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "word-format-paragraph", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]string{"text": "本周工作成果"}),
	})
	if err != nil {
		t.Fatalf("append report paragraph: %v", err)
	}
	var target struct {
		ParagraphID string `json:"paragraph_id"`
		RunID       string `json:"run_id"`
	}
	if err := json.Unmarshal(appended.Data, &target); err != nil || target.ParagraphID == "" || target.RunID == "" {
		t.Fatalf("decode appended identities: %v, data=%s", err, appended.Data)
	}
	request := CommandRequest{
		DocumentID: doc.DocumentID, Operator: Operator{ID: "agent", Kind: OperatorExternalAgent},
	}
	request.RequestID, request.CommandID, request.ExpectedRevision = "word-font-family", "word.text_run.font_family.set", appended.Revision
	request.Arguments = mustJSON(t, map[string]string{"run_id": target.RunID, "font_family": "Arial"})
	fontFamily, err := runtime.Commands().Execute(ctx, request)
	if err != nil {
		t.Fatalf("set Word font family: %v", err)
	}
	request.RequestID, request.ExpectedRevision = "word-font-family-noop", fontFamily.Revision
	noopFontFamily, err := runtime.Commands().Execute(ctx, request)
	var noopFontFamilyData struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noopFontFamily.Data, &noopFontFamilyData) != nil || noopFontFamilyData.Changed || noopFontFamily.Revision != fontFamily.Revision {
		t.Fatalf("repeat font family result=%+v data=%s error=%v; want no-op", noopFontFamily, noopFontFamily.Data, err)
	}
	request.RequestID, request.CommandID, request.ExpectedRevision = "word-font-size", "word.text_run.font_size.set", fontFamily.Revision
	request.Arguments = mustJSON(t, map[string]any{"run_id": target.RunID, "font_size_points": 16})
	fontSize, err := runtime.Commands().Execute(ctx, request)
	if err != nil {
		t.Fatalf("set Word font size: %v", err)
	}
	request.RequestID, request.ExpectedRevision = "word-font-size-noop", fontSize.Revision
	noopFontSize, err := runtime.Commands().Execute(ctx, request)
	var noopFontSizeData struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noopFontSize.Data, &noopFontSizeData) != nil || noopFontSizeData.Changed || noopFontSize.Revision != fontSize.Revision {
		t.Fatalf("repeat font size result=%+v data=%s error=%v; want no-op", noopFontSize, noopFontSize.Data, err)
	}
	request.RequestID, request.CommandID, request.ExpectedRevision = "word-paragraph-list", "word.paragraph.list.set", fontSize.Revision
	request.Arguments = mustJSON(t, map[string]string{"paragraph_id": target.ParagraphID, "list_type": "bullet"})
	list, err := runtime.Commands().Execute(ctx, request)
	if err != nil {
		t.Fatalf("set Word paragraph list: %v", err)
	}
	request.RequestID, request.ExpectedRevision = "word-paragraph-list-noop", list.Revision
	noopList, err := runtime.Commands().Execute(ctx, request)
	var noopListData struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noopList.Data, &noopListData) != nil || noopListData.Changed || noopList.Revision != list.Revision {
		t.Fatalf("repeat paragraph list result=%+v data=%s error=%v; want no-op", noopList, noopList.Data, err)
	}
	request.RequestID, request.CommandID, request.ExpectedRevision = "word-paragraph-spacing", "word.paragraph.spacing.set", list.Revision
	request.Arguments = mustJSON(t, map[string]any{"paragraph_id": target.ParagraphID, "before_points": 12.0, "after_points": 8.0})
	spacing, err := runtime.Commands().Execute(ctx, request)
	if err != nil {
		t.Fatalf("set Word paragraph spacing: %v", err)
	}
	if spacing.Revision != list.Revision+1 {
		t.Fatalf("spacing revision = %d, want %d", spacing.Revision, list.Revision+1)
	}
	request.RequestID, request.ExpectedRevision = "word-paragraph-spacing-noop", spacing.Revision
	noopSpacing, err := runtime.Commands().Execute(ctx, request)
	var noopSpacingData struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noopSpacing.Data, &noopSpacingData) != nil || noopSpacingData.Changed || noopSpacing.Revision != spacing.Revision {
		t.Fatalf("repeat paragraph spacing result=%+v data=%s error=%v; want no-op", noopSpacing, noopSpacing.Data, err)
	}
	stateBeforeInvalidTargets := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if stateBeforeInvalidTargets.Revision != spacing.Revision || !stateBeforeInvalidTargets.Dirty {
		t.Fatalf("Word font/list/spacing no-ops changed revision or dirty state: state=%+v", stateBeforeInvalidTargets)
	}
	for _, invalid := range []struct {
		commandID string
		arguments any
	}{
		{"word.text_run.font_family.set", map[string]any{"run_id": "missing-run", "font_family": "Arial"}},
		{"word.text_run.font_size.set", map[string]any{"run_id": "missing-run", "font_size_points": 16}},
		{"word.paragraph.list.set", map[string]any{"paragraph_id": "missing-paragraph", "list_type": "bullet"}},
		{"word.paragraph.spacing.set", map[string]any{"paragraph_id": "missing-paragraph", "before_points": 12, "after_points": 8}},
	} {
		if _, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: "word-format-invalid-" + invalid.commandID, DocumentID: doc.DocumentID,
			CommandID: invalid.commandID, ExpectedRevision: stateBeforeInvalidTargets.Revision,
			Operator: Operator{ID: "agent", Kind: OperatorExternalAgent}, Arguments: mustJSON(t, invalid.arguments),
		}); err == nil {
			t.Errorf("%s accepted an unknown stable target", invalid.commandID)
		}
	}
	if afterInvalidTargets := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterInvalidTargets.Revision != stateBeforeInvalidTargets.Revision || afterInvalidTargets.Dirty != stateBeforeInvalidTargets.Dirty {
		t.Fatalf("invalid Word formatting targets changed document state: before=%+v after=%+v", stateBeforeInvalidTargets, afterInvalidTargets)
	}
	inspected, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("inspect formatted Word document: %v", err)
	}
	var query struct {
		Blocks []struct {
			ParagraphID     string  `json:"paragraph_id"`
			ListType        string  `json:"list_type"`
			BeforeSpacingPT float64 `json:"before_spacing_points"`
			AfterSpacingPT  float64 `json:"after_spacing_points"`
			Runs            []struct {
				ID    string `json:"id"`
				Style struct {
					FontFamily string  `json:"font_family"`
					FontSizePT float64 `json:"font_size_points"`
				} `json:"style"`
			} `json:"runs"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(inspected.Content, &query); err != nil {
		t.Fatalf("decode Word inspection: %v", err)
	}
	var foundParagraph, foundRun bool
	for _, block := range query.Blocks {
		if block.ParagraphID == target.ParagraphID {
			foundParagraph = block.ListType == "bullet" && block.BeforeSpacingPT == 12 && block.AfterSpacingPT == 8
		}
		for _, run := range block.Runs {
			if run.ID == target.RunID {
				foundRun = run.Style.FontFamily == "Arial" && run.Style.FontSizePT == 16
			}
		}
	}
	if !foundParagraph || !foundRun {
		t.Fatalf("Word inspection omits formatting needed by Agent: %+v", query)
	}

	path := filepath.Join(t.TempDir(), "formatted.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("CloseDocument: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("docx.Open: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var gotParagraph *document.WordParagraph
	var gotRun *document.WordTextRun
	for _, block := range reopened.WordBlocks() {
		if block.Paragraph == nil || block.Paragraph.ID != target.ParagraphID {
			continue
		}
		gotParagraph = block.Paragraph
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == target.RunID {
				gotRun = inline.Text
			}
		}
	}
	if gotParagraph == nil || gotParagraph.ListKind != document.WordListBullet || gotParagraph.Layout.BeforePT != 12 || gotParagraph.Layout.AfterPT != 8 {
		t.Fatalf("paragraph formatting did not survive save/reopen: %+v", gotParagraph)
	}
	if gotRun == nil || gotRun.Style.FontFamily != "Arial" || gotRun.Style.FontSize != 16 {
		t.Fatalf("run formatting did not survive save/reopen: %+v", gotRun)
	}
}

func TestWordParagraphAlignmentAndIndentCapabilitiesRoundTrip(t *testing.T) {
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
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "word-paragraph-insert", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]string{"text": "汇报结论"}),
	})
	if err != nil {
		t.Fatalf("append report paragraph: %v", err)
	}
	var paragraph struct {
		ParagraphID string `json:"paragraph_id"`
	}
	if err := json.Unmarshal(appended.Data, &paragraph); err != nil || paragraph.ParagraphID == "" {
		t.Fatalf("decode paragraph ID: %v, data=%s", err, appended.Data)
	}

	aligned, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "word-paragraph-align", DocumentID: doc.DocumentID, CommandID: "word.paragraph.alignment.set",
		ExpectedRevision: appended.Revision, Operator: Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]string{"paragraph_id": paragraph.ParagraphID, "alignment": "center"}),
	})
	if err != nil {
		t.Fatalf("set paragraph alignment: %v", err)
	}
	if aligned.Revision != appended.Revision+1 {
		t.Fatalf("alignment revision = %d, want %d", aligned.Revision, appended.Revision+1)
	}
	noopAlignment, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "word-paragraph-align-noop", DocumentID: doc.DocumentID, CommandID: "word.paragraph.alignment.set",
		ExpectedRevision: aligned.Revision, Operator: Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]string{"paragraph_id": paragraph.ParagraphID, "alignment": "center"}),
	})
	var noopAlignmentData struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noopAlignment.Data, &noopAlignmentData) != nil || noopAlignmentData.Changed || noopAlignment.Revision != aligned.Revision {
		t.Fatalf("repeat alignment result=%+v data=%s error=%v; want no-op", noopAlignment, noopAlignment.Data, err)
	}

	indented, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "word-paragraph-indent", DocumentID: doc.DocumentID, CommandID: "word.paragraph.indent.set",
		ExpectedRevision: aligned.Revision, Operator: Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"paragraph_id": paragraph.ParagraphID, "left_indent_points": 36.0, "right_indent_points": 18.0}),
	})
	if err != nil {
		t.Fatalf("set paragraph indents: %v", err)
	}
	if indented.Revision != aligned.Revision+1 {
		t.Fatalf("indent revision = %d, want %d", indented.Revision, aligned.Revision+1)
	}
	noopIndent, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "word-paragraph-indent-noop", DocumentID: doc.DocumentID, CommandID: "word.paragraph.indent.set",
		ExpectedRevision: indented.Revision, Operator: Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"paragraph_id": paragraph.ParagraphID, "left_indent_points": 36.0, "right_indent_points": 18.0}),
	})
	var noopIndentData struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noopIndent.Data, &noopIndentData) != nil || noopIndentData.Changed || noopIndent.Revision != indented.Revision {
		t.Fatalf("repeat indent result=%+v data=%s error=%v; want no-op", noopIndent, noopIndent.Data, err)
	}
	stateBeforeInvalidTargets := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if stateBeforeInvalidTargets.Revision != indented.Revision || !stateBeforeInvalidTargets.Dirty {
		t.Fatalf("Word alignment/indent no-ops changed revision or dirty state: state=%+v", stateBeforeInvalidTargets)
	}
	for _, invalid := range []struct {
		commandID string
		arguments any
	}{
		{"word.paragraph.alignment.set", map[string]any{"paragraph_id": "missing-paragraph", "alignment": "center"}},
		{"word.paragraph.indent.set", map[string]any{"paragraph_id": "missing-paragraph", "left_indent_points": 36, "right_indent_points": 18}},
	} {
		if _, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: "word-layout-invalid-" + invalid.commandID, DocumentID: doc.DocumentID,
			CommandID: invalid.commandID, ExpectedRevision: stateBeforeInvalidTargets.Revision,
			Operator: Operator{ID: "agent", Kind: OperatorExternalAgent}, Arguments: mustJSON(t, invalid.arguments),
		}); err == nil {
			t.Errorf("%s accepted an unknown paragraph ID", invalid.commandID)
		}
	}
	if afterInvalidTargets := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterInvalidTargets.Revision != stateBeforeInvalidTargets.Revision || afterInvalidTargets.Dirty != stateBeforeInvalidTargets.Dirty {
		t.Fatalf("invalid Word paragraph targets changed document state: before=%+v after=%+v", stateBeforeInvalidTargets, afterInvalidTargets)
	}
	inspected, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("InspectDocument after layout edits: %v", err)
	}
	var layoutView struct {
		Blocks []struct {
			ParagraphID       string  `json:"paragraph_id"`
			Alignment         string  `json:"alignment"`
			LeftIndentPoints  float64 `json:"left_indent_points"`
			RightIndentPoints float64 `json:"right_indent_points"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(inspected.Content, &layoutView); err != nil {
		t.Fatalf("unmarshal Word paragraph layout view: %v", err)
	}
	foundLayout := false
	for _, block := range layoutView.Blocks {
		if block.ParagraphID != paragraph.ParagraphID {
			continue
		}
		foundLayout = true
		if block.Alignment != "center" || block.LeftIndentPoints != 36 || block.RightIndentPoints != 18 {
			t.Fatalf("Word query returned stale paragraph layout: %+v", block)
		}
	}
	if !foundLayout {
		t.Fatalf("Word query omitted paragraph identity %q: %s", paragraph.ParagraphID, inspected.Content)
	}
	path := filepath.Join(t.TempDir(), "laid-out.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("CloseDocument: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("docx.Open: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var got *document.WordParagraph
	for _, block := range reopened.WordBlocks() {
		if block.Paragraph != nil && block.Paragraph.ID == paragraph.ParagraphID {
			got = block.Paragraph
			break
		}
	}
	if got == nil || got.Alignment != document.WordAlignCenter || got.LeftIndentEMU != 36*12700 || got.RightIndentEMU != 18*12700 {
		t.Fatalf("paragraph layout did not survive save/reopen: %+v", got)
	}
}

func TestWordTextRunStyleSetIsIdempotentAndRoundTrips(t *testing.T) {
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
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("InspectDocument: %v", err)
	}
	var content struct {
		Blocks []struct {
			Runs []struct {
				ID       string `json:"id"`
				Editable bool   `json:"editable"`
			} `json:"runs"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(snapshot.Content, &content); err != nil {
		t.Fatalf("unmarshal Word query: %v", err)
	}
	var runID string
	for _, block := range content.Blocks {
		for _, run := range block.Runs {
			if run.Editable {
				runID = run.ID
				break
			}
		}
	}
	if runID == "" {
		t.Fatalf("Word query did not expose an editable run: %s", snapshot.Content)
	}
	request := CommandRequest{
		RequestID: "word-style-set", DocumentID: doc.DocumentID, CommandID: "word.text_run.style.set",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{
			"run_id": runID, "bold": true, "italic": true, "font_color": "#2A54C7", "highlight": "yellow",
		}),
	}
	first, err := runtime.Commands().Execute(ctx, request)
	if err != nil {
		t.Fatalf("Execute Word style set: %v", err)
	}
	if first.Revision != doc.Revision+1 {
		t.Fatalf("style revision = %d, want %d", first.Revision, doc.Revision+1)
	}
	request.RequestID = "word-style-noop"
	request.ExpectedRevision = first.Revision
	second, err := runtime.Commands().Execute(ctx, request)
	if err != nil {
		t.Fatalf("Execute repeated Word style set: %v", err)
	}
	if second.Revision != first.Revision {
		t.Fatalf("no-op style command advanced revision from %d to %d", first.Revision, second.Revision)
	}
	stateBeforeInvalid := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if stateBeforeInvalid.Revision != second.Revision || !stateBeforeInvalid.Dirty {
		t.Fatalf("Word style no-op changed revision or dirty state: state=%+v", stateBeforeInvalid)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "word-style-invalid-run", DocumentID: doc.DocumentID, CommandID: "word.text_run.style.set",
		ExpectedRevision: stateBeforeInvalid.Revision, Operator: Operator{ID: "agent", Kind: OperatorExternalAgent},
		Arguments: mustJSON(t, map[string]any{"run_id": "missing-run", "bold": true}),
	}); err == nil {
		t.Fatal("Word style setter accepted an unknown run ID")
	}
	if stateAfterInvalid := mustGetDocumentInfo(t, runtime, doc.DocumentID); stateAfterInvalid.Revision != stateBeforeInvalid.Revision || stateAfterInvalid.Dirty != stateBeforeInvalid.Dirty {
		t.Fatalf("invalid style target changed document state: before=%+v after=%+v", stateBeforeInvalid, stateAfterInvalid)
	}

	path := filepath.Join(t.TempDir(), "styled.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("CloseDocument: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("docx.Open after save: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var styled *document.WordTextRun
	for _, block := range reopened.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == runID {
				styled = inline.Text
				break
			}
		}
	}
	if styled == nil || !styled.Style.Bold || !styled.Style.Italic || styled.Style.FontColor != "2A54C7" || styled.Style.Highlight != "yellow" {
		t.Fatalf("Word run style did not survive save/reopen: %+v", styled)
	}
}

func TestWordParagraphAppendGeneratesAndReopensReportContent(t *testing.T) {
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

	result, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "req-report-paragraph-1", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "本周完成项目总结"}),
	})
	if err != nil {
		t.Fatalf("Execute Word paragraph append: %v", err)
	}
	if result.Revision != doc.Revision+1 {
		t.Fatalf("Word append revision = %d, want %d", result.Revision, doc.Revision+1)
	}
	var appended struct {
		Changed     bool   `json:"changed"`
		ParagraphID string `json:"paragraph_id"`
		RunID       string `json:"run_id"`
		Text        string `json:"text"`
	}
	if err := json.Unmarshal(result.Data, &appended); err != nil {
		t.Fatalf("unmarshal append result: %v", err)
	}
	if !appended.Changed || appended.ParagraphID == "" || appended.RunID == "" || appended.Text != "本周完成项目总结" {
		t.Fatalf("unexpected append result: %+v", appended)
	}

	path := filepath.Join(t.TempDir(), "weekly-report.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("CloseDocument: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after save: %v", err)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: reopened.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("InspectDocument after reopen: %v", err)
	}
	var content struct {
		Blocks []struct {
			Text string `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(snapshot.Content, &content); err != nil {
		t.Fatalf("unmarshal reopened Word query: %v", err)
	}
	found := false
	for _, block := range content.Blocks {
		if block.Text == "本周完成项目总结" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("saved report paragraph missing after reopen: %s", snapshot.Content)
	}
}

func TestSpreadsheetCellSetCommandMutatesAndRequeriesLiveDocument(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatXLSX)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	request := CommandRequest{
		RequestID: "req-cell-1", DocumentID: doc.DocumentID, CommandID: "spreadsheet.cell.set",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorExternalAgent},
		Arguments: json.RawMessage(`{"sheet":"Sheet1","cell":"A1","value":"人员"}`),
	}
	first, err := runtime.Commands().Execute(ctx, request)
	if err != nil {
		t.Fatalf("Execute cell set: %v", err)
	}
	if first.Revision != doc.Revision+1 {
		t.Fatalf("cell set revision = %d, want %d", first.Revision, doc.Revision+1)
	}
	var firstData struct {
		Changed bool   `json:"changed"`
		Sheet   string `json:"sheet"`
		Cell    string `json:"cell"`
		Value   string `json:"value"`
	}
	if err := json.Unmarshal(first.Data, &firstData); err != nil || !firstData.Changed || firstData.Sheet != "Sheet1" || firstData.Cell != "A1" || firstData.Value != "人员" {
		t.Fatalf("cell set result=%+v data=%s error=%v", firstData, first.Data, err)
	}
	query, err := runtime.Queries().InspectDocument(ctx, InspectRequest{
		DocumentID: doc.DocumentID, Sheet: "Sheet1",
		Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1},
	})
	if err != nil {
		t.Fatalf("InspectDocument: %v", err)
	}
	var sheet struct {
		Rows [][]struct {
			Value string `json:"value"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(query.Content, &sheet); err != nil {
		t.Fatalf("unmarshal cell query: %v", err)
	}
	if len(sheet.Rows) != 1 || len(sheet.Rows[0]) != 1 || sheet.Rows[0][0].Value != "人员" {
		t.Fatalf("cell query returned different live content: %s", query.Content)
	}
	request.RequestID = "req-cell-noop"
	request.ExpectedRevision = first.Revision
	second, err := runtime.Commands().Execute(ctx, request)
	if err != nil {
		t.Fatalf("Execute idempotent cell set: %v", err)
	}
	var secondData struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(second.Data, &secondData); err != nil || secondData.Changed || second.Revision != first.Revision {
		t.Fatalf("no-op cell set result=%s revision=%d error=%v; want unchanged revision %d", second.Data, second.Revision, err, first.Revision)
	}
	stateAfterNoop, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterNoop.Revision != first.Revision || !stateAfterNoop.Dirty {
		t.Fatalf("no-op cell set changed document state: state=%+v error=%v", stateAfterNoop, err)
	}
	for _, invalid := range []struct {
		requestID string
		arguments map[string]string
	}{
		{requestID: "req-cell-missing-sheet", arguments: map[string]string{"sheet": "MissingSheet", "cell": "A1", "value": "不应写入"}},
		{requestID: "req-cell-invalid-address", arguments: map[string]string{"sheet": "Sheet1", "cell": "A0", "value": "不应写入"}},
	} {
		request.RequestID = invalid.requestID
		request.ExpectedRevision = first.Revision
		request.Arguments = mustJSON(t, invalid.arguments)
		if _, err := runtime.Commands().Execute(ctx, request); err == nil {
			t.Fatalf("cell set accepted invalid target: %+v", invalid.arguments)
		}
	}
	stateAfterInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterInvalid.Revision != stateAfterNoop.Revision || stateAfterInvalid.Dirty != stateAfterNoop.Dirty {
		t.Fatalf("invalid cell set changed document state: before=%+v after=%+v error=%v", stateAfterNoop, stateAfterInvalid, err)
	}

	path := filepath.Join(t.TempDir(), "cell-set.xlsx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("CloseDocument before reopen: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after save: %v", err)
	}
	reopenedQuery, err := runtime.Queries().InspectDocument(ctx, InspectRequest{
		DocumentID: reopened.DocumentID, Sheet: "Sheet1",
		Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1},
	})
	if err != nil {
		t.Fatalf("InspectDocument after reopen: %v", err)
	}
	var reopenedSheet struct {
		Rows [][]struct {
			Value string `json:"value"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(reopenedQuery.Content, &reopenedSheet); err != nil || len(reopenedSheet.Rows) != 1 || len(reopenedSheet.Rows[0]) != 1 || reopenedSheet.Rows[0][0].Value != "人员" {
		t.Fatalf("cell value did not survive XLSX save/reopen: content=%s error=%v", reopenedQuery.Content, err)
	}
}

func TestWordTextRunReplaceCommandUsesNeutralRunIdentity(t *testing.T) {
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
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("InspectDocument: %v", err)
	}
	var content struct {
		Blocks []struct {
			Runs []struct {
				ID       string `json:"id"`
				Text     string `json:"text"`
				Editable bool   `json:"editable"`
			} `json:"runs"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(snapshot.Content, &content); err != nil {
		t.Fatalf("unmarshal Word query: %v", err)
	}
	var runID string
	for _, block := range content.Blocks {
		for _, run := range block.Runs {
			if run.Editable {
				runID = run.ID
				break
			}
		}
	}
	if runID == "" {
		t.Fatalf("Word query did not expose an editable run identity: %s", snapshot.Content)
	}
	result, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "req-word-1", DocumentID: doc.DocumentID, CommandID: "word.text_run.replace",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"run_id": runID, "value": "本周完成项目总结"}),
	})
	if err != nil {
		t.Fatalf("Execute Word text replacement: %v", err)
	}
	if result.Revision != doc.Revision+1 {
		t.Fatalf("Word edit revision = %d, want %d", result.Revision, doc.Revision+1)
	}
	var resultData struct {
		Changed bool   `json:"changed"`
		RunID   string `json:"run_id"`
		Value   string `json:"value"`
	}
	if err := json.Unmarshal(result.Data, &resultData); err != nil || !resultData.Changed || resultData.RunID != runID || resultData.Value != "本周完成项目总结" {
		t.Fatalf("Word replacement result=%+v data=%s error=%v", resultData, result.Data, err)
	}
	noop, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "req-word-1-noop", DocumentID: doc.DocumentID, CommandID: "word.text_run.replace",
		ExpectedRevision: result.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"run_id": runID, "value": "本周完成项目总结"}),
	})
	var noopData struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noop.Data, &noopData) != nil || noopData.Changed || noop.Revision != result.Revision {
		t.Fatalf("repeat Word text replacement=%s revision=%d error=%v; want no-op at %d", noop.Data, noop.Revision, err, result.Revision)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "req-word-1-invalid-run", DocumentID: doc.DocumentID, CommandID: "word.text_run.replace",
		ExpectedRevision: result.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"run_id": "missing-run", "value": "不应写入"}),
	}); err == nil {
		t.Fatal("Word text replacement accepted a missing run ID")
	}
	stateAfterNoopAndInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterNoopAndInvalid.Revision != result.Revision || !stateAfterNoopAndInvalid.Dirty {
		t.Fatalf("no-op or invalid replacement changed document state: state=%+v error=%v", stateAfterNoopAndInvalid, err)
	}
	updated, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("InspectDocument after edit: %v", err)
	}
	if string(updated.Content) == string(snapshot.Content) {
		t.Fatal("Word query returned the original content after a successful edit")
	}
	if !strings.Contains(string(updated.Content), "本周完成项目总结") {
		t.Fatalf("Word query omitted replacement text: %s", updated.Content)
	}

	path := filepath.Join(t.TempDir(), "replaced-run.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("CloseDocument before reopen: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after save: %v", err)
	}
	reopenedSnapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: reopened.DocumentID, Limit: 10})
	if err != nil || !strings.Contains(string(reopenedSnapshot.Content), "本周完成项目总结") {
		t.Fatalf("replacement did not survive save/reopen: content=%s error=%v", reopenedSnapshot.Content, err)
	}
}

func TestWordTextRangeReplaceUsesUnicodeRuneOffsets(t *testing.T) {
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
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "range-replace-paragraph", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "甲🌺&乙"}),
	})
	if err != nil {
		t.Fatalf("append source paragraph: %v", err)
	}
	var target struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(appended.Data, &target); err != nil || target.RunID == "" {
		t.Fatalf("decode appended run ID: %v, data=%s", err, appended.Data)
	}
	replaced, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "range-replace-emoji", DocumentID: doc.DocumentID, CommandID: "word.text_range.replace",
		ExpectedRevision: appended.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]any{"run_id": target.RunID, "start": 1, "end": 2, "replacement": "丁丙"}),
	})
	if err != nil {
		t.Fatalf("replace Unicode range: %v", err)
	}
	if replaced.Revision != appended.Revision+1 {
		t.Fatalf("range replacement revision = %d, want %d", replaced.Revision, appended.Revision+1)
	}
	noop, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "range-replace-same-text", DocumentID: doc.DocumentID, CommandID: "word.text_range.replace",
		ExpectedRevision: replaced.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]any{"run_id": target.RunID, "start": 1, "end": 3, "replacement": "丁丙"}),
	})
	var noopData struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noop.Data, &noopData) != nil || noopData.Changed || noop.Revision != replaced.Revision {
		t.Fatalf("repeat equivalent range replacement=%s revision=%d error=%v; want no-op at %d", noop.Data, noop.Revision, err, replaced.Revision)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "range-replace-invalid-range", DocumentID: doc.DocumentID, CommandID: "word.text_range.replace",
		ExpectedRevision: replaced.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]any{"run_id": target.RunID, "start": 4, "end": 2, "replacement": "不应写入"}),
	}); err == nil {
		t.Fatal("Word range replacement accepted an end offset before its start")
	}
	stateAfterNoopAndInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterNoopAndInvalid.Revision != replaced.Revision || !stateAfterNoopAndInvalid.Dirty {
		t.Fatalf("no-op or invalid range replacement changed document state: state=%+v error=%v", stateAfterNoopAndInvalid, err)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("inspect replaced Word document: %v", err)
	}
	var inspected struct {
		Blocks []struct {
			Text string `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(snapshot.Content, &inspected); err != nil {
		t.Fatalf("decode inspected Word content: %v", err)
	}
	var got string
	for _, block := range inspected.Blocks {
		if strings.Contains(block.Text, "甲丁丙") {
			got = block.Text
		}
	}
	if got != "甲丁丙&乙" {
		t.Fatalf("Unicode range replacement text = %q; want %q (content=%s)", got, "甲丁丙&乙", snapshot.Content)
	}
	path := filepath.Join(t.TempDir(), "unicode-range.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("CloseDocument before reopen: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument after save: %v", err)
	}
	reopenedSnapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: reopened.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("InspectDocument after reopen: %v", err)
	}
	var reopenedContent struct {
		Blocks []struct {
			Text string `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(reopenedSnapshot.Content, &reopenedContent); err != nil {
		t.Fatalf("decode reopened Word document: %v", err)
	}
	for _, block := range reopenedContent.Blocks {
		if block.Text == "甲丁丙&乙" {
			return
		}
	}
	t.Fatalf("Unicode range replacement did not survive save/reopen: content=%s", reopenedSnapshot.Content)
}

func TestWordTextRangeReplaceUsesSchemaUnicodeLength(t *testing.T) {
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
	operator := Operator{ID: "unicode-range-agent", Kind: OperatorExternalAgent}
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "unicode-range-source", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: operator, Arguments: mustJSON(t, map[string]string{"text": "初始"}),
	})
	if err != nil {
		t.Fatalf("append source paragraph: %v", err)
	}
	var source struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(appended.Data, &source); err != nil || source.RunID == "" {
		t.Fatalf("decode source run: data=%s error=%v", appended.Data, err)
	}
	replacement := strings.Repeat("中", 350000) // 1,050,000 UTF-8 bytes, within the 1,048,576-rune schema limit.
	arguments := mustJSON(t, map[string]any{"run_id": source.RunID, "start": 0, "end": 0, "replacement": replacement})
	descriptor, ok := runtime.Catalog().Descriptor("word.text_range.replace")
	if !ok {
		t.Fatal("word.text_range.replace is not registered")
	}
	if err := validateArguments(descriptor.InputSchema, arguments); err != nil {
		t.Fatalf("schema rejected %d-rune replacement: %v", len([]rune(replacement)), err)
	}
	result, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "unicode-range-long-replacement", DocumentID: doc.DocumentID, CommandID: "word.text_range.replace",
		ExpectedRevision: appended.Revision, Operator: operator, Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("execute long Unicode range replacement: %v", err)
	}
	if result.Revision != appended.Revision+1 || !strings.Contains(string(result.Data), replacement[:64]) {
		t.Fatalf("long Unicode replacement result revision=%d data length=%d", result.Revision, len(result.Data))
	}
	collapsed, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "unicode-range-long-replacement-collapse", DocumentID: doc.DocumentID, CommandID: "word.text_range.replace",
		ExpectedRevision: result.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"run_id": source.RunID, "start": 0, "end": 350000, "replacement": "已验证"}),
	})
	if err != nil || collapsed.Revision != result.Revision+1 {
		t.Fatalf("replace long Unicode range: revision=%d error=%v", collapsed.Revision, err)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 10})
	if err != nil || !strings.Contains(string(snapshot.Content), "已验证初始") {
		t.Fatalf("long Unicode range replacement was not present in the live document: content=%s error=%v", snapshot.Content, err)
	}
}

func TestWordTextRangeCopyCutAndPasteUseExplicitUnicodeRanges(t *testing.T) {
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
	operator := Operator{ID: "agent-1", Kind: OperatorBuiltInAgent}
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "range-clipboard-append", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"text": "ABC🙂DEF"}),
	})
	if err != nil {
		t.Fatalf("append source paragraph: %v", err)
	}
	var target struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(appended.Data, &target); err != nil || target.RunID == "" {
		t.Fatalf("decode appended run ID: %v, data=%s", err, appended.Data)
	}

	copied, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "range-clipboard-copy", DocumentID: doc.DocumentID, CapabilityID: "word.text_range.copy",
		ExpectedRevision: appended.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"run_id": target.RunID, "start": 3, "end": 4}),
	})
	if err != nil {
		t.Fatalf("copy Unicode range: %v", err)
	}
	var copiedText struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(copied.Data, &copiedText); err != nil || copiedText.Text != "🙂" {
		t.Fatalf("copied range = %q; want emoji (data=%s, err=%v)", copiedText.Text, copied.Data, err)
	}
	if copied.Revision != appended.Revision {
		t.Fatalf("copy changed revision to %d; want %d", copied.Revision, appended.Revision)
	}
	emptyCut, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "range-clipboard-empty-cut", DocumentID: doc.DocumentID, CommandID: "word.text_range.cut",
		ExpectedRevision: appended.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"run_id": target.RunID, "start": 3, "end": 3}),
	})
	if err != nil {
		t.Fatalf("cut empty range: %v", err)
	}
	var emptyCutData struct {
		Changed bool   `json:"changed"`
		Text    string `json:"text"`
	}
	if err := json.Unmarshal(emptyCut.Data, &emptyCutData); err != nil || emptyCutData.Changed || emptyCutData.Text != "" || emptyCut.Revision != appended.Revision {
		t.Fatalf("empty cut result = %+v, revision=%d, error=%v; want no-op", emptyCutData, emptyCut.Revision, err)
	}
	stateAfterEmptyCut := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if stateAfterEmptyCut.Revision != appended.Revision || !stateAfterEmptyCut.Dirty {
		t.Fatalf("empty cut changed revision or dirty state: %+v", stateAfterEmptyCut)
	}
	for index, arguments := range []map[string]any{
		{"run_id": "missing-run", "start": 0, "end": 1},
		{"run_id": target.RunID, "start": 3, "end": 99},
	} {
		if _, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: fmt.Sprintf("range-clipboard-invalid-cut-%d", index), DocumentID: doc.DocumentID,
			CommandID: "word.text_range.cut", ExpectedRevision: appended.Revision, Operator: operator,
			Arguments: mustJSON(t, arguments),
		}); err == nil {
			t.Errorf("Word cut accepted an invalid range target: %+v", arguments)
		}
	}
	if afterInvalidCut := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterInvalidCut.Revision != stateAfterEmptyCut.Revision || afterInvalidCut.Dirty != stateAfterEmptyCut.Dirty {
		t.Fatalf("invalid Word cut changed document state: before=%+v after=%+v", stateAfterEmptyCut, afterInvalidCut)
	}

	cut, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "range-clipboard-cut", DocumentID: doc.DocumentID, CapabilityID: "word.text_range.cut",
		ExpectedRevision: appended.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"run_id": target.RunID, "start": 3, "end": 4}),
	})
	if err != nil {
		t.Fatalf("cut Unicode range: %v", err)
	}
	var cutText struct {
		Changed bool   `json:"changed"`
		Text    string `json:"text"`
	}
	if err := json.Unmarshal(cut.Data, &cutText); err != nil || !cutText.Changed || cutText.Text != "🙂" {
		t.Fatalf("cut result = %+v; want changed emoji (data=%s, err=%v)", cutText, cut.Data, err)
	}
	if cut.Revision != appended.Revision+1 {
		t.Fatalf("cut revision = %d; want %d", cut.Revision, appended.Revision+1)
	}

	pasted, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "range-clipboard-paste", DocumentID: doc.DocumentID, CommandID: "word.text_range.replace",
		ExpectedRevision: cut.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"run_id": target.RunID, "start": 3, "end": 3, "replacement": "🙂"}),
	})
	if err != nil {
		t.Fatalf("paste at empty range: %v", err)
	}
	if pasted.Revision != cut.Revision+1 {
		t.Fatalf("paste revision = %d; want %d", pasted.Revision, cut.Revision+1)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("inspect pasted Word document: %v", err)
	}
	var inspected struct {
		Blocks []struct {
			Text string `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(snapshot.Content, &inspected); err != nil {
		t.Fatalf("decode inspected Word content: %v", err)
	}
	var got string
	for _, block := range inspected.Blocks {
		if strings.Contains(block.Text, "ABC") {
			got = block.Text
		}
	}
	if got != "ABC🙂DEF" {
		t.Fatalf("clipboard round trip text = %q; want %q", got, "ABC🙂DEF")
	}
	path := filepath.Join(t.TempDir(), "clipboard.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil || saved.Dirty {
		t.Fatalf("save clipboard-edited DOCX = %+v, error=%v; want clean document", saved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close clipboard-edited DOCX: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen clipboard-edited DOCX: %v", err)
	}
	reopenedSnapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: reopened.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("inspect clipboard-edited DOCX after reopen: %v", err)
	}
	if !strings.Contains(string(reopenedSnapshot.Content), "ABC🙂DEF") {
		t.Fatalf("cut/paste text did not survive DOCX save/reopen: %s", reopenedSnapshot.Content)
	}
}

func TestWordFormatPainterUsesExplicitSourceAndTargetRuns(t *testing.T) {
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
	operator := Operator{ID: "agent-1", Kind: OperatorBuiltInAgent}
	appendParagraph := func(requestID, value string, revision uint64) (CommandResult, string) {
		t.Helper()
		result, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: requestID, DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
			ExpectedRevision: revision, Operator: operator, Arguments: mustJSON(t, map[string]string{"text": value}),
		})
		if err != nil {
			t.Fatalf("append %s paragraph: %v", requestID, err)
		}
		var data struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal(result.Data, &data); err != nil || data.RunID == "" {
			t.Fatalf("decode %s run ID: %v, data=%s", requestID, err, result.Data)
		}
		return result, data.RunID
	}
	source, sourceRunID := appendParagraph("format-painter-source", "Source", doc.Revision)
	target, targetRunID := appendParagraph("format-painter-target", "Target", source.Revision)
	styled, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "format-painter-style-source", DocumentID: doc.DocumentID, CommandID: "word.text_run.style.set",
		ExpectedRevision: target.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]any{"run_id": sourceRunID, "bold": true}),
	})
	if err != nil {
		t.Fatalf("style format painter source: %v", err)
	}
	painted, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "format-painter-apply", DocumentID: doc.DocumentID, CommandID: "word.text_format.paint",
		ExpectedRevision: styled.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"source_run_id": sourceRunID, "target_run_id": targetRunID}),
	})
	if err != nil {
		t.Fatalf("apply format painter capability: %v", err)
	}
	if painted.Revision != styled.Revision+1 {
		t.Fatalf("paint revision = %d; want %d", painted.Revision, styled.Revision+1)
	}
	repeated, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "format-painter-repeat", DocumentID: doc.DocumentID, CommandID: "word.text_format.paint",
		ExpectedRevision: painted.Revision, Operator: operator,
		Arguments: mustJSON(t, map[string]string{"source_run_id": sourceRunID, "target_run_id": targetRunID}),
	})
	if err != nil {
		t.Fatalf("repeat format painter: %v", err)
	}
	var repeatedData struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(repeated.Data, &repeatedData); err != nil || repeatedData.Changed || repeated.Revision != painted.Revision {
		t.Fatalf("repeat format painter result = %+v, revision=%d, error=%v; want no-op", repeatedData, repeated.Revision, err)
	}

	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("inspect painted document: %v", err)
	}
	var inspected struct {
		Blocks []struct {
			Runs []struct {
				ID    string `json:"id"`
				Style struct {
					Bold bool `json:"bold"`
				} `json:"style"`
			} `json:"runs"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(snapshot.Content, &inspected); err != nil {
		t.Fatalf("decode painted Word content: %v", err)
	}
	foundPaintedRun := false
	for _, block := range inspected.Blocks {
		for _, run := range block.Runs {
			if run.ID == targetRunID && run.Style.Bold {
				foundPaintedRun = true
			}
		}
	}
	if !foundPaintedRun {
		t.Fatalf("format painter did not copy bold style to run %q: %s", targetRunID, snapshot.Content)
	}
	stateBeforeInvalidTargets := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if stateBeforeInvalidTargets.Revision != repeated.Revision || !stateBeforeInvalidTargets.Dirty {
		t.Fatalf("format-painter no-op changed revision or dirty state: %+v", stateBeforeInvalidTargets)
	}
	for _, invalid := range []map[string]string{
		{"source_run_id": "missing-source", "target_run_id": targetRunID},
		{"source_run_id": sourceRunID, "target_run_id": "missing-target"},
	} {
		if _, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID:  "format-painter-invalid-" + invalid["source_run_id"] + "-" + invalid["target_run_id"],
			DocumentID: doc.DocumentID, CommandID: "word.text_format.paint", ExpectedRevision: stateBeforeInvalidTargets.Revision,
			Operator: operator, Arguments: mustJSON(t, invalid),
		}); err == nil {
			t.Errorf("format painter accepted invalid source/target: %+v", invalid)
		}
	}
	if afterInvalidTargets := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterInvalidTargets.Revision != stateBeforeInvalidTargets.Revision || afterInvalidTargets.Dirty != stateBeforeInvalidTargets.Dirty {
		t.Fatalf("invalid format-painter target changed document state: before=%+v after=%+v", stateBeforeInvalidTargets, afterInvalidTargets)
	}
	path := filepath.Join(t.TempDir(), "format-painted.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil || saved.Dirty {
		t.Fatalf("save format-painted DOCX = %+v, error=%v; want clean document", saved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close format-painted DOCX: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen format-painted DOCX: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	for _, block := range reopened.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == targetRunID {
				if inline.Text.Style.Bold {
					return
				}
				t.Fatalf("saved format painter target lost bold style: %+v", inline.Text.Style)
			}
		}
	}
	t.Fatalf("saved format painter target run %q was not found", targetRunID)
}

func TestWordWatermarkCapabilitiesRoundTripThroughRuntime(t *testing.T) {
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
	operator := Operator{ID: "agent-1", Kind: OperatorBuiltInAgent}
	set, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "watermark-set", DocumentID: doc.DocumentID, CommandID: "word.watermark.set",
		ExpectedRevision: doc.Revision, Operator: operator, Arguments: mustJSON(t, map[string]string{"text": "机密"}),
	})
	if err != nil {
		t.Fatalf("set watermark: %v", err)
	}
	if set.Revision != doc.Revision+1 {
		t.Fatalf("watermark set revision = %d; want %d", set.Revision, doc.Revision+1)
	}
	got, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "watermark-get", DocumentID: doc.DocumentID, CapabilityID: "word.watermark.get",
		ExpectedRevision: set.Revision, Operator: operator, Arguments: mustJSON(t, map[string]any{}),
	})
	if err != nil {
		t.Fatalf("get watermark: %v", err)
	}
	var watermark struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(got.Data, &watermark); err != nil || watermark.Text != "机密" {
		t.Fatalf("watermark query = %+v, error=%v, data=%s", watermark, err, got.Data)
	}
	repeated, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "watermark-set-noop", DocumentID: doc.DocumentID, CommandID: "word.watermark.set",
		ExpectedRevision: set.Revision, Operator: operator, Arguments: mustJSON(t, map[string]string{"text": "机密"}),
	})
	if err != nil {
		t.Fatalf("repeat watermark: %v", err)
	}
	var repeatedData struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(repeated.Data, &repeatedData); err != nil || repeatedData.Changed || repeated.Revision != set.Revision {
		t.Fatalf("repeated watermark = %+v, revision=%d, err=%v; want no-op", repeatedData, repeated.Revision, err)
	}
	stateAfterSet, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterSet.Revision != set.Revision || !stateAfterSet.Dirty {
		t.Fatalf("watermark state after set/no-op = %+v, error=%v; want dirty revision %d", stateAfterSet, err, set.Revision)
	}
	path := filepath.Join(t.TempDir(), "watermarked.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil {
		t.Fatalf("save watermarked DOCX: %v", err)
	}
	if saved.Dirty {
		t.Fatalf("watermarked DOCX remains dirty after save: %+v", saved)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen watermarked DOCX: %v", err)
	}
	defer reopened.Close()
	watermarkEditor := any(reopened).(document.WordWatermarkEditor)
	if value, err := watermarkEditor.GetWatermark(); err != nil || value != "机密" {
		t.Fatalf("reopened watermark = %q, error=%v; want 机密", value, err)
	}
}

func TestWordTrackRevisionsCapabilitiesUseAbsoluteState(t *testing.T) {
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
	operator := Operator{ID: "agent-1", Kind: OperatorBuiltInAgent}
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "track-revisions-append", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: operator, Arguments: mustJSON(t, map[string]string{"text": "审阅内容"}),
	})
	if err != nil {
		t.Fatalf("append Word content: %v", err)
	}
	query := func(requestID string, revision uint64) bool {
		t.Helper()
		result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
			RequestID: requestID, DocumentID: doc.DocumentID, CapabilityID: "word.review.track.get",
			ExpectedRevision: revision, Operator: operator, Arguments: mustJSON(t, map[string]any{}),
		})
		if err != nil {
			t.Fatalf("query revision tracking: %v", err)
		}
		var data struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.Unmarshal(result.Data, &data); err != nil {
			t.Fatalf("decode revision tracking result: %v, data=%s", err, result.Data)
		}
		return data.Enabled
	}
	if query("track-revisions-get-initial", appended.Revision) {
		t.Fatal("revision tracking is enabled in a new document")
	}
	set, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "track-revisions-enable", DocumentID: doc.DocumentID, CommandID: "word.review.track.set",
		ExpectedRevision: appended.Revision, Operator: operator, Arguments: mustJSON(t, map[string]bool{"enabled": true}),
	})
	if err != nil {
		t.Fatalf("enable revision tracking: %v", err)
	}
	enabled := query("track-revisions-get-enabled", set.Revision)
	if set.Revision != appended.Revision+1 || !enabled {
		t.Fatalf("enabled revision state/revision = %v/%d; want true/%d", enabled, set.Revision, appended.Revision+1)
	}
	noop, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "track-revisions-enable-noop", DocumentID: doc.DocumentID, CommandID: "word.review.track.set",
		ExpectedRevision: set.Revision, Operator: operator, Arguments: mustJSON(t, map[string]bool{"enabled": true}),
	})
	if err != nil {
		t.Fatalf("repeat enabled revision tracking: %v", err)
	}
	var noopData struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(noop.Data, &noopData); err != nil || noopData.Changed || noop.Revision != set.Revision {
		t.Fatalf("repeat tracking result = %+v, revision=%d, err=%v; want no-op", noopData, noop.Revision, err)
	}
	stateAfterNoop, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterNoop.Revision != set.Revision || !stateAfterNoop.Dirty {
		t.Fatalf("revision tracking state after no-op = %+v, error=%v; want dirty revision %d", stateAfterNoop, err, set.Revision)
	}
	path := filepath.Join(t.TempDir(), "tracked-revisions.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil || saved.Dirty {
		t.Fatalf("save tracked revisions DOCX = %+v, error=%v; want clean document", saved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close tracked revisions DOCX: %v", err)
	}
	doc, err = runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen tracked revisions DOCX: %v", err)
	}
	if !query("track-revisions-get-reopened-enabled", doc.Revision) {
		t.Fatal("revision tracking did not survive DOCX save/reopen")
	}
	cleared, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "track-revisions-disable", DocumentID: doc.DocumentID, CommandID: "word.review.track.set",
		ExpectedRevision: doc.Revision, Operator: operator, Arguments: mustJSON(t, map[string]bool{"enabled": false}),
	})
	if err != nil {
		t.Fatalf("disable revision tracking: %v", err)
	}
	if cleared.Revision != doc.Revision+1 || query("track-revisions-get-disabled", cleared.Revision) {
		t.Fatalf("revision tracking was not disabled at revision %d", cleared.Revision)
	}
	stateAfterDisable, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterDisable.Revision != cleared.Revision || !stateAfterDisable.Dirty {
		t.Fatalf("revision tracking state after disable = %+v, error=%v; want dirty revision %d", stateAfterDisable, err, cleared.Revision)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "track-revisions-invalid-type", DocumentID: doc.DocumentID, CommandID: "word.review.track.set",
		ExpectedRevision: cleared.Revision, Operator: operator, Arguments: mustJSON(t, map[string]any{"enabled": "false"}),
	}); err == nil {
		t.Fatal("revision tracking accepted a string instead of a boolean")
	}
	stateAfterInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterInvalid.Revision != stateAfterDisable.Revision || stateAfterInvalid.Dirty != stateAfterDisable.Dirty {
		t.Fatalf("invalid revision tracking request changed document state: before=%+v after=%+v error=%v", stateAfterDisable, stateAfterInvalid, err)
	}
	noopDisabled, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "track-revisions-disable-noop", DocumentID: doc.DocumentID, CommandID: "word.review.track.set",
		ExpectedRevision: cleared.Revision, Operator: operator, Arguments: mustJSON(t, map[string]bool{"enabled": false}),
	})
	var noopDisabledData struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(noopDisabled.Data, &noopDisabledData) != nil || noopDisabledData.Changed || noopDisabled.Revision != cleared.Revision {
		t.Fatalf("repeat disabled tracking = %+v, error=%v; want no-op at revision %d", noopDisabledData, err, cleared.Revision)
	}
	saved, err = runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil || saved.Dirty {
		t.Fatalf("save disabled tracking DOCX = %+v, error=%v; want clean document", saved, err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("close disabled tracking DOCX: %v", err)
	}
	doc, err = runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("reopen disabled tracking DOCX: %v", err)
	}
	if query("track-revisions-get-reopened-disabled", doc.Revision) {
		t.Fatal("disabled revision tracking did not survive DOCX save/reopen")
	}
}

func TestWordTextSearchReturnsBoundedUnicodeMatches(t *testing.T) {
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
	first, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "search-paragraph-one", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "甲🌺乙"}),
	})
	if err != nil {
		t.Fatalf("append first search paragraph: %v", err)
	}
	second, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "search-paragraph-two", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: first.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "🌺乙甲"}),
	})
	if err != nil {
		t.Fatalf("append second search paragraph: %v", err)
	}
	result, err := runtime.InvokeCapability(ctx, CapabilityRequest{
		RequestID: "search-document", DocumentID: doc.DocumentID, CapabilityID: "word.text.search",
		Operator:  Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]any{"query": "🌺乙", "start_block": 1, "max_blocks": 2, "limit": 1}),
	})
	if err != nil {
		t.Fatalf("search Word document: %v", err)
	}
	var matches struct {
		Matches []struct {
			ParagraphID string `json:"paragraph_id"`
			RunID       string `json:"run_id"`
			Start       int    `json:"start"`
			End         int    `json:"end"`
			Text        string `json:"text"`
		} `json:"matches"`
		HasMore bool `json:"has_more"`
	}
	if err := json.Unmarshal(result.Data, &matches); err != nil {
		t.Fatalf("decode search result: %v, data=%s", err, result.Data)
	}
	if len(matches.Matches) != 1 || matches.Matches[0].Start != 1 || matches.Matches[0].End != 3 || matches.Matches[0].Text != "🌺乙" || !matches.HasMore {
		t.Fatalf("bounded Unicode search matches = %+v; want one rune-indexed match and has_more", matches)
	}
	if result.Revision != second.Revision {
		t.Fatalf("read-only search revision = %d, want %d", result.Revision, second.Revision)
	}
}

func TestWordTableInsertCapabilityCreatesEditableTable(t *testing.T) {
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
	paragraph, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "table-anchor-paragraph", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "人员名单"}),
	})
	if err != nil {
		t.Fatalf("append table anchor: %v", err)
	}
	var anchor struct {
		ParagraphID string `json:"paragraph_id"`
	}
	if err := json.Unmarshal(paragraph.Data, &anchor); err != nil || anchor.ParagraphID == "" {
		t.Fatalf("decode table anchor ID: %v, data=%s", err, paragraph.Data)
	}
	inserted, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-agent-table", DocumentID: doc.DocumentID, CommandID: "word.table.insert",
		ExpectedRevision: paragraph.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]any{"after_paragraph_id": anchor.ParagraphID, "rows": 2, "columns": 3}),
	})
	if err != nil {
		t.Fatalf("insert Word table: %v", err)
	}
	if inserted.Revision != paragraph.Revision+1 {
		t.Fatalf("table insert revision = %d, want %d", inserted.Revision, paragraph.Revision+1)
	}
	var result struct {
		TableID string `json:"table_id"`
	}
	if err := json.Unmarshal(inserted.Data, &result); err != nil || result.TableID == "" {
		t.Fatalf("decode table ID: %v, data=%s", err, inserted.Data)
	}
	stateBeforeInvalidTarget := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-agent-table-invalid-target", DocumentID: doc.DocumentID, CommandID: "word.table.insert",
		ExpectedRevision: inserted.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]any{"after_paragraph_id": "missing-paragraph", "rows": 1, "columns": 1}),
	}); err == nil {
		t.Fatal("word.table.insert accepted an unknown paragraph ID")
	}
	if state := mustGetDocumentInfo(t, runtime, doc.DocumentID); state.Revision != stateBeforeInvalidTarget.Revision || state.Dirty != stateBeforeInvalidTarget.Dirty {
		t.Fatalf("rejected table target changed document state: before=%+v after=%+v", stateBeforeInvalidTarget, state)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("inspect inserted Word table: %v", err)
	}
	var content struct {
		Blocks []struct {
			Kind string     `json:"kind"`
			Rows [][]string `json:"rows"`
			Runs []struct {
				ParagraphID string `json:"paragraph_id"`
				ID          string `json:"id"`
				Editable    bool   `json:"editable"`
			} `json:"runs"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(snapshot.Content, &content); err != nil {
		t.Fatalf("decode Word table inspection: %v", err)
	}
	var foundTable bool
	var firstCellRunID string
	for _, block := range content.Blocks {
		if block.Kind != "table" {
			continue
		}
		foundTable = len(block.Rows) == 2 && len(block.Rows[0]) == 3 && len(block.Runs) == 6
		for _, run := range block.Runs {
			if run.ID == "" || run.ParagraphID == "" {
				foundTable = false
			}
			if firstCellRunID == "" && run.Editable {
				firstCellRunID = run.ID
			}
		}
	}
	if !foundTable || firstCellRunID == "" {
		t.Fatalf("inserted Word table is not inspectable and editable: %s", snapshot.Content)
	}
	filled, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "fill-agent-table-cell", DocumentID: doc.DocumentID, CommandID: "word.text_run.replace",
		ExpectedRevision: inserted.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"run_id": firstCellRunID, "value": "姓名"}),
	})
	if err != nil {
		t.Fatalf("fill first Word table cell: %v", err)
	}

	path := filepath.Join(t.TempDir(), "table.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("save Word table: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen Word table: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var persisted bool
	for _, block := range reopened.WordBlocks() {
		if block.Table != nil && block.ID == result.TableID && len(block.Table.Rows) == 2 && len(block.Table.Rows[0].Cells) == 3 {
			firstCell := block.Table.Rows[0].Cells[0]
			for _, paragraph := range firstCell.Paragraphs {
				for _, inline := range paragraph.Inlines {
					if inline.Text != nil && inline.Text.ID == firstCellRunID && inline.Text.Text == "姓名" {
						persisted = true
					}
				}
			}
		}
	}
	if !persisted {
		t.Fatalf("inserted table and first-cell edit did not survive save/reopen (fill revision=%d)", filled.Revision)
	}
}

func TestWordCoverPageCapabilityRoundTrips(t *testing.T) {
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
	result, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-agent-cover", DocumentID: doc.DocumentID, CommandID: "word.cover_page.insert",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"title": "月度经营报告", "subtitle": "2026 年 10 月", "author": "运营部"}),
	})
	if err != nil {
		t.Fatalf("insert Word cover page: %v", err)
	}
	if result.Revision != doc.Revision+1 {
		t.Fatalf("cover page revision = %d, want %d", result.Revision, doc.Revision+1)
	}
	stateAfterInsert := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if stateAfterInsert.Revision != result.Revision || !stateAfterInsert.Dirty {
		t.Fatalf("cover page insertion state = %+v; want dirty revision %d", stateAfterInsert, result.Revision)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("inspect Word cover page: %v", err)
	}
	for _, want := range []string{"月度经营报告", "2026 年 10 月", "运营部"} {
		if !strings.Contains(string(snapshot.Content), want) {
			t.Errorf("cover page inspection omits %q: %s", want, snapshot.Content)
		}
	}
	path := filepath.Join(t.TempDir(), "cover.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil {
		t.Fatalf("save Word cover page: %v", err)
	}
	if saved.Dirty {
		t.Fatalf("saved Word cover page remains dirty: %+v", saved)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen Word cover page: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var text string
	for _, block := range reopened.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil {
				text += inline.Text.Text
			}
		}
	}
	for _, want := range []string{"月度经营报告", "2026 年 10 月", "运营部"} {
		if !strings.Contains(text, want) {
			t.Errorf("saved cover page omits %q: %s", want, text)
		}
	}
}

func TestWordPageBreakCapabilitySplitsUnicodeRun(t *testing.T) {
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
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "page-break-source", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "甲🌺乙"}),
	})
	if err != nil {
		t.Fatalf("append page-break source: %v", err)
	}
	var run struct {
		RunID       string `json:"run_id"`
		ParagraphID string `json:"paragraph_id"`
	}
	if err := json.Unmarshal(appended.Data, &run); err != nil || run.RunID == "" || run.ParagraphID == "" {
		t.Fatalf("decode source run ID: %v, data=%s", err, appended.Data)
	}
	stateBeforeInvalidTargets := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if stateBeforeInvalidTargets.Revision != appended.Revision || !stateBeforeInvalidTargets.Dirty {
		t.Fatalf("page-break invalid-target precondition = %+v; want appended dirty document", stateBeforeInvalidTargets)
	}
	for index, arguments := range []map[string]any{
		{"run_id": "missing-run", "start": 0, "end": 0},
		{"run_id": run.RunID, "start": 1, "end": 99},
	} {
		if _, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: fmt.Sprintf("insert-page-break-invalid-%d", index), DocumentID: doc.DocumentID, CommandID: "word.page_break.insert",
			ExpectedRevision: stateBeforeInvalidTargets.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
			Arguments: mustJSON(t, arguments),
		}); err == nil {
			t.Errorf("page-break insertion accepted invalid target: %+v", arguments)
		}
	}
	if afterInvalidTargets := mustGetDocumentInfo(t, runtime, doc.DocumentID); afterInvalidTargets.Revision != stateBeforeInvalidTargets.Revision || afterInvalidTargets.Dirty != stateBeforeInvalidTargets.Dirty {
		t.Fatalf("invalid page-break target changed document state: before=%+v after=%+v", stateBeforeInvalidTargets, afterInvalidTargets)
	}
	inserted, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-page-break", DocumentID: doc.DocumentID, CommandID: "word.page_break.insert",
		ExpectedRevision: appended.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]any{"run_id": run.RunID, "start": 1, "end": 1}),
	})
	if err != nil {
		t.Fatalf("insert page break: %v", err)
	}
	var output struct {
		ParagraphID string `json:"paragraph_id"`
		CaretRunID  string `json:"caret_run_id"`
	}
	if err := json.Unmarshal(inserted.Data, &output); err != nil || output.ParagraphID == "" || output.CaretRunID == "" {
		t.Fatalf("decode page-break caret: %v, data=%s", err, inserted.Data)
	}
	if inserted.Revision != appended.Revision+1 {
		t.Fatalf("page break revision = %d, want %d", inserted.Revision, appended.Revision+1)
	}
	path := filepath.Join(t.TempDir(), "page-break.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("save Word page break: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen Word page break: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var before, after string
	var foundBreak, foundCaret bool
	blocks := reopened.WordBlocks()
	for _, block := range blocks {
		if block.Paragraph == nil || block.Paragraph.ID != run.ParagraphID {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.PageBreak != nil {
				foundBreak = true
				continue
			}
			if inline.Text != nil {
				if foundBreak {
					after += inline.Text.Text
					foundCaret = foundCaret || inline.Text.ID == output.CaretRunID
				} else {
					before += inline.Text.Text
				}
			}
		}
	}
	if output.ParagraphID != run.ParagraphID || before != "甲" || after != "🌺乙" || !foundBreak || !foundCaret {
		t.Fatalf("page-break projection = paragraph %q, split %q / %q, break=%v caret=%v; want same paragraph, %q / %q, break and caret (blocks=%+v)", output.ParagraphID, before, after, foundBreak, foundCaret, "甲", "🌺乙", blocks)
	}
}

func TestWordBlankPageCapabilityInsertsTwoPageBreaks(t *testing.T) {
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
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "blank-page-source", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "正文内容"}),
	})
	if err != nil {
		t.Fatalf("append blank-page source: %v", err)
	}
	var run struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(appended.Data, &run); err != nil || run.RunID == "" {
		t.Fatalf("decode source run ID: %v, data=%s", err, appended.Data)
	}
	stateBeforeInvalid, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid blank-page requests: %v", err)
	}
	for index, arguments := range []map[string]any{
		{"run_id": "missing-run", "start": 0, "end": 0},
		{"run_id": run.RunID, "start": 1, "end": 99},
	} {
		if _, err := runtime.Commands().Execute(ctx, CommandRequest{
			RequestID: fmt.Sprintf("insert-blank-page-invalid-%d", index), DocumentID: doc.DocumentID, CommandID: "word.blank_page.insert",
			ExpectedRevision: stateBeforeInvalid.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
			Arguments: mustJSON(t, arguments),
		}); err == nil {
			t.Fatalf("blank-page insertion accepted invalid target: %+v", arguments)
		}
	}
	if afterInvalid, err := runtime.GetDocument(ctx, doc.DocumentID); err != nil || afterInvalid.Revision != stateBeforeInvalid.Revision || afterInvalid.Dirty != stateBeforeInvalid.Dirty {
		t.Fatalf("invalid blank-page request changed document state: before=%+v after=%+v error=%v", stateBeforeInvalid, afterInvalid, err)
	}
	inserted, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-blank-page", DocumentID: doc.DocumentID, CommandID: "word.blank_page.insert",
		ExpectedRevision: stateBeforeInvalid.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]any{"run_id": run.RunID, "start": 1, "end": 3}),
	})
	if err != nil {
		t.Fatalf("insert blank page: %v", err)
	}
	var output struct {
		ParagraphID string `json:"paragraph_id"`
		CaretRunID  string `json:"caret_run_id"`
	}
	if err := json.Unmarshal(inserted.Data, &output); err != nil || output.ParagraphID == "" || output.CaretRunID == "" {
		t.Fatalf("decode blank-page caret: %v, data=%s", err, inserted.Data)
	}
	if inserted.Revision != stateBeforeInvalid.Revision+1 {
		t.Fatalf("blank-page revision=%d; want %d", inserted.Revision, stateBeforeInvalid.Revision+1)
	}
	stateAfterInsert, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil || stateAfterInsert.Revision != inserted.Revision || !stateAfterInsert.Dirty {
		t.Fatalf("document state after blank-page insertion=%+v error=%v", stateAfterInsert, err)
	}
	path := filepath.Join(t.TempDir(), "blank-page.docx")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil || saved.Dirty {
		t.Fatalf("save Word blank page: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen Word blank page: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	blocks := reopened.WordBlocks()
	var breaks int
	var foundCaret bool
	var before, after string
	for _, block := range blocks {
		if block.Paragraph == nil || block.Paragraph.ID != output.ParagraphID {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.PageBreak != nil {
				breaks++
				continue
			}
			if inline.Text != nil {
				if breaks > 0 {
					after += inline.Text.Text
				} else {
					before += inline.Text.Text
				}
				if inline.Text.ID == output.CaretRunID {
					foundCaret = true
				}
			}
		}
	}
	if breaks != 2 || !foundCaret || before != "正" || after != "容" {
		t.Fatalf("blank-page operation has %d page breaks, caret=%v and split text %q/%q; want two page breaks, editable caret and preserved prefix/suffix (blocks=%+v)", breaks, foundCaret, before, after, blocks)
	}
}

func TestWordBookmarkAndHyperlinkCapabilitiesRoundTrip(t *testing.T) {
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
	appended, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "link-source", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "章节入口"}),
	})
	if err != nil {
		t.Fatalf("append link source: %v", err)
	}
	var target struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(appended.Data, &target); err != nil || target.RunID == "" {
		t.Fatalf("decode source run ID: %v, data=%s", err, appended.Data)
	}
	stateBeforeInvalidBookmark, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument before invalid bookmark target: %v", err)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-bookmark-missing-run", DocumentID: doc.DocumentID, CommandID: "word.bookmark.insert",
		ExpectedRevision: stateBeforeInvalidBookmark.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"run_id": "missing-run", "name": "Ghost"}),
	}); err == nil {
		t.Fatal("bookmark insertion accepted a missing run ID")
	}
	if afterInvalidBookmark, err := runtime.GetDocument(ctx, doc.DocumentID); err != nil || afterInvalidBookmark.Revision != stateBeforeInvalidBookmark.Revision || afterInvalidBookmark.Dirty != stateBeforeInvalidBookmark.Dirty {
		t.Fatalf("invalid bookmark target changed document state: before=%+v after=%+v error=%v", stateBeforeInvalidBookmark, afterInvalidBookmark, err)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-hyperlink-missing-run", DocumentID: doc.DocumentID, CommandID: "word.hyperlink.insert",
		ExpectedRevision: stateBeforeInvalidBookmark.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"run_id": "missing-run", "text": "Ghost", "target_url": "https://example.com/ghost"}),
	}); err == nil {
		t.Fatal("hyperlink insertion accepted a missing run ID")
	}
	if afterInvalidHyperlink, err := runtime.GetDocument(ctx, doc.DocumentID); err != nil || afterInvalidHyperlink.Revision != stateBeforeInvalidBookmark.Revision || afterInvalidHyperlink.Dirty != stateBeforeInvalidBookmark.Dirty {
		t.Fatalf("invalid hyperlink target changed document state: before=%+v after=%+v error=%v", stateBeforeInvalidBookmark, afterInvalidHyperlink, err)
	}
	bookmark, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-bookmark", DocumentID: doc.DocumentID, CommandID: "word.bookmark.insert",
		ExpectedRevision: appended.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"run_id": target.RunID, "name": "Overview"}),
	})
	if err != nil {
		t.Fatalf("insert Word bookmark: %v", err)
	}
	linked, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-hyperlink", DocumentID: doc.DocumentID, CommandID: "word.hyperlink.insert",
		ExpectedRevision: bookmark.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"run_id": target.RunID, "text": "点击查看报告", "target_url": "https://example.com/report"}),
	})
	if err != nil {
		t.Fatalf("insert Word hyperlink: %v", err)
	}
	if linked.Revision != bookmark.Revision+1 {
		t.Fatalf("hyperlink revision = %d, want %d", linked.Revision, bookmark.Revision+1)
	}
	path := filepath.Join(t.TempDir(), "links.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("save Word links: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen Word links: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var text string
	for _, block := range reopened.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil {
				text += inline.Text.Text
			}
		}
	}
	if !strings.Contains(text, "点击查看报告") {
		t.Fatalf("hyperlink text did not survive save/reopen: %q", text)
	}
}

func TestWordTextBoxCapabilityRoundTrips(t *testing.T) {
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
	anchor, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "textbox-anchor", DocumentID: doc.DocumentID, CommandID: "word.paragraph.append",
		ExpectedRevision: doc.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"text": "季度经营结果"}),
	})
	if err != nil {
		t.Fatalf("append text box anchor: %v", err)
	}
	var target struct {
		ParagraphID string `json:"paragraph_id"`
	}
	if err := json.Unmarshal(anchor.Data, &target); err != nil || target.ParagraphID == "" {
		t.Fatalf("decode text box anchor: %v, data=%s", err, anchor.Data)
	}
	textboxText := strings.Repeat("营", 6000)
	inserted, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-report-textbox", DocumentID: doc.DocumentID, CommandID: "word.text_box.insert",
		ExpectedRevision: anchor.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"after_paragraph_id": target.ParagraphID, "text": textboxText}),
	})
	if err != nil {
		t.Fatalf("insert Word text box: %v", err)
	}
	snapshot, err := runtime.Queries().InspectDocument(ctx, InspectRequest{DocumentID: doc.DocumentID, Limit: 10})
	if err != nil {
		t.Fatalf("inspect Word text box: %v", err)
	}
	if !strings.Contains(string(snapshot.Content), textboxText) || inserted.Revision != anchor.Revision+1 {
		t.Fatalf("text box is not present in Agent inspection or revision: result=%+v content=%s", inserted, snapshot.Content)
	}
	path := filepath.Join(t.TempDir(), "textbox.docx")
	if _, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, path); err != nil {
		t.Fatalf("save Word text box: %v", err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatalf("reopen Word text box: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var text string
	for _, block := range reopened.WordBlocks() {
		if block.Table == nil {
			continue
		}
		for _, row := range block.Table.Rows {
			for _, cell := range row.Cells {
				for _, paragraph := range cell.Paragraphs {
					for _, inline := range paragraph.Inlines {
						if inline.Text != nil {
							text += inline.Text.Text
						}
					}
				}
			}
		}
	}
	if !strings.Contains(text, textboxText) {
		t.Fatalf("text box content did not survive save/reopen: %q", text)
	}
	stateBeforeInvalid := mustGetDocumentInfo(t, runtime, doc.DocumentID)
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-report-textbox-invalid-target", DocumentID: doc.DocumentID, CommandID: "word.text_box.insert",
		ExpectedRevision: inserted.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"after_paragraph_id": "missing-paragraph", "text": "不会插入"}),
	}); err == nil {
		t.Fatal("word.text_box.insert accepted an unknown paragraph ID")
	}
	if state := mustGetDocumentInfo(t, runtime, doc.DocumentID); state.Revision != stateBeforeInvalid.Revision || state.Dirty != stateBeforeInvalid.Dirty {
		t.Fatalf("rejected text box target changed document state: before=%+v after=%+v", stateBeforeInvalid, state)
	}
	if _, err := runtime.Commands().Execute(ctx, CommandRequest{
		RequestID: "insert-report-textbox-too-long", DocumentID: doc.DocumentID, CommandID: "word.text_box.insert",
		ExpectedRevision: inserted.Revision, Operator: Operator{ID: "agent-1", Kind: OperatorBuiltInAgent},
		Arguments: mustJSON(t, map[string]string{"after_paragraph_id": target.ParagraphID, "text": strings.Repeat("超", 16001)}),
	}); err == nil {
		t.Fatal("word.text_box.insert accepted more than 16000 Unicode characters")
	}
	if state := mustGetDocumentInfo(t, runtime, doc.DocumentID); state.Revision != stateBeforeInvalid.Revision || state.Dirty != stateBeforeInvalid.Dirty {
		t.Fatalf("rejected long text box changed document state: before=%+v after=%+v", stateBeforeInvalid, state)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return data
}
