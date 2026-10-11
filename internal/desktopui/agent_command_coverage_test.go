package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"glance/internal/application"
)

type agentCommandCoverageManifest struct {
	Version               int                          `json:"version"`
	RibbonActions         []agentCommandCoverageAction `json:"ribbon_actions"`
	AgentOnlyCapabilities []agentOnlyCapability        `json:"agent_only_capabilities"`
}

type agentCommandCoverageAction struct {
	ActionID     string          `json:"action_id"`
	SourceKey    string          `json:"source_key"`
	Format       string          `json:"format"`
	Tab          string          `json:"tab"`
	Label        string          `json:"label"`
	Capability   string          `json:"capability_id"`
	Description  string          `json:"description"`
	Target       string          `json:"target_type"`
	Parameters   []string        `json:"parameters"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
	Impact       string          `json:"state_impact"`
	Risk         string          `json:"risk"`
	Status       string          `json:"agent_status"`
}

type agentOnlyCapability struct {
	ID          string `json:"capability_id"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

type ribbonActionSource struct {
	Key    string
	Format string
	Tab    string
	ID     string
	Label  string
}

func TestRibbonActionCoverageManifest(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	manifestPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "docs", "compatibility", "agent-command-coverage.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read Agent command coverage manifest: %v", err)
	}

	var manifest agentCommandCoverageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode Agent command coverage manifest: %v", err)
	}
	if manifest.Version != 1 {
		t.Fatalf("manifest version = %d, want 1", manifest.Version)
	}
	assertAction := func(sourceKey, target, impact string, requiredParameters ...string) {
		t.Helper()
		var action *agentCommandCoverageAction
		for index := range manifest.RibbonActions {
			if manifest.RibbonActions[index].SourceKey == sourceKey {
				action = &manifest.RibbonActions[index]
				break
			}
		}
		if action == nil {
			t.Fatalf("manifest is missing action %q", sourceKey)
		}
		if action.Target != target || action.Impact != impact {
			t.Errorf("%s target/impact = %q/%q, want %q/%q", sourceKey, action.Target, action.Impact, target, impact)
		}
		for _, required := range requiredParameters {
			if !slices.Contains(action.Parameters, required) {
				t.Errorf("%s parameters %v do not include %q", sourceKey, action.Parameters, required)
			}
		}
	}
	assertAction("markdown/main/open", "workspace", "workspace_lifecycle", "path")
	assertAction("json/main/import", "workspace", "workspace_lifecycle", "path")
	assertAction("pdf/main/open", "workspace", "workspace_lifecycle", "path")
	assertAction("pdf/main/zoom-in", "attached_window", "view_state", "document_id", "expected_revision", "view", "operation", "zoom_percent")
	assertAction("pdf/mode/print", "pdf_document", "external_output", "document_id", "expected_revision", "copies", "first_page", "last_page")
	assertAction("pdf/mode/download", "pdf_document_or_output_file", "file_output", "document_id", "expected_revision", "path")
	assertAction("presentation/slideshow/show_from_start", "attached_window", "window_view", "document_id", "expected_revision", "operation", "mode")
	assertAction("word/review/review-restrict", "word_document", "document_mutation", "document_id", "expected_revision", "enabled")
	assertAction("word/home/select", "attached_window", "view_state", "document_id", "expected_revision", "view", "selection")
	assertAction("word/home/add-ins", "word_document", "read_only", "document_id", "expected_revision", "category")
	assertAction("word/file/export-pdf", "document_or_output_file", "file_output", "document_id", "expected_revision", "target_path", "orientation")
	assertAction("markdown/main/export-pdf", "document_or_output_file", "file_output", "document_id", "expected_revision", "target_path", "orientation")
	assertAction("presentation/view/view_sorter", "attached_window", "window_view", "document_id", "expected_revision", "operation", "mode")
	assertAction("presentation/view/view_ruler", "attached_window", "window_view", "document_id", "expected_revision", "operation", "enabled")
	assertAction("word/file/share", "document_or_output_file", "file_output", "document_id", "expected_revision", "path")
	assertAction("word/file/autosave", "word_document", "document_settings", "document_id", "expected_revision", "view", "enabled")
	assertAction("word/home/editor", "word_document_or_block_page", "read_only", "document_id", "start_block", "max_blocks")
	assertAction("word/review/review-editor", "word_document_or_block_page", "read_only", "document_id", "start_block", "max_blocks")
	assertAction("word/review/review-compare", "two_word_documents", "read_only", "left_document_id", "right_document_id", "left_expected_revision", "right_expected_revision", "start", "limit")
	assertAction("word/view/view-macros", "word_document", "read_only", "document_id", "expected_revision")
	assertAction("word/insert/insert-shapes", "word_document", "document_mutation", "document_id", "expected_revision", "after_paragraph_id", "text")
	assertAction("word/insert/insert-wordart", "word_document", "document_mutation", "document_id", "expected_revision", "after_paragraph_id", "text")
	assertAction("word/insert/insert-chart", "word_document", "document_mutation", "document_id", "expected_revision", "after_paragraph_id", "rows", "columns")
	assertAction("word/insert/insert-equation", "word_document", "document_mutation", "document_id", "expected_revision", "target", "expression")
	assertAction("word/design/design-themes", "word_document", "document_mutation", "document_id", "expected_revision", "before_points", "after_points")
	assertAction("word/home/designer", "word_document", "document_mutation", "document_id", "expected_revision", "before_points", "after_points")
	assertAction("word/home/dictate", "word_text_run_or_paragraph", "document_mutation", "document_id", "expected_revision", "target", "text")
	assertAction("word/design/design-colors", "word_document", "document_mutation", "document_id", "expected_revision", "color")
	assertAction("word/design/design-fonts", "word_text_run", "document_mutation", "document_id", "expected_revision", "run_id", "font_family")
	assertAction("word/review/review-translate", "word_text_run", "document_mutation", "document_id", "expected_revision", "run_id", "value")
	assertAction("spreadsheet/file/slot-81", "spreadsheet_range", "file_output", "document_id", "expected_revision", "sheet", "bounds", "target_path", "orientation")
	assertAction("spreadsheet/review/slot-66", "spreadsheet_cells", "document_mutation", "document_id", "expected_revision", "sheet", "edits")
	assertAction("spreadsheet/review/slot-65", "spreadsheet_range", "read_only", "document_id", "expected_revision", "sheet", "bounds")
	assertAction("spreadsheet/formula/slot-59", "spreadsheet_workbook", "document_mutation", "document_id", "expected_revision")
	assertAction("spreadsheet/data/slot-60", "spreadsheet_workbook", "document_mutation", "document_id", "expected_revision")
	assertAction("spreadsheet/file/slot-82", "document_or_output_file", "file_output", "document_id", "expected_revision", "path")
	for _, action := range manifest.RibbonActions {
		if action.SourceKey == "word/review/review-compare" {
			if action.Capability != "document.compare" || action.Status != "implemented" || action.Risk != "low" {
				t.Errorf("Word compare manifest = capability %q, status %q, risk %q; want the shared low-risk document comparison capability", action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "word/view/view-macros" {
			if action.Capability != "word.macros.inspect" || action.Status != "implemented" || action.Risk != "low" {
				t.Errorf("Word macros manifest = capability %q, status %q, risk %q; want the shared low-risk macro payload inspection capability", action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "word/file/autosave" {
			if action.Capability != "word.autosave.set" || action.Status != "implemented" || action.Risk != "high" {
				t.Errorf("Word AutoSave manifest = capability %q, status %q, risk %q; want registered high-risk capability", action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "spreadsheet/formula/slot-59" || action.SourceKey == "spreadsheet/data/slot-60" {
			if action.Capability != "spreadsheet.workbook.recalculate" || action.Status != "implemented" || action.Risk != "low" {
				t.Errorf("%s manifest = capability %q, status %q, risk %q; want the shared low-risk XLSX calculation command", action.SourceKey, action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "word/file/share" || action.SourceKey == "spreadsheet/file/slot-82" {
			if action.Capability != "document.export_copy" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("%s manifest = capability %q, status %q, risk %q; want the shared medium-risk document-copy command", action.SourceKey, action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "word/file/export-pdf" {
			if action.Capability != "word.document.export_pdf" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("Word PDF export manifest = capability %q, status %q, risk %q; want the shared medium-risk PDF export capability", action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "markdown/main/export-pdf" {
			if action.Capability != "markdown.document.export_pdf" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("Markdown PDF export manifest = capability %q, status %q, risk %q; want the shared medium-risk PDF export capability", action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "spreadsheet/file/slot-81" {
			if action.Capability != "spreadsheet.document.export_pdf" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("Spreadsheet PDF export manifest = capability %q, status %q, risk %q; want the shared medium-risk bounded PDF export capability", action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "spreadsheet/review/slot-66" {
			if action.Capability != "spreadsheet.cells.set" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("Spreadsheet translation manifest = capability %q, status %q, risk %q; want the shared medium-risk bounded cell batch command", action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "spreadsheet/review/slot-65" {
			if action.Capability != "spreadsheet.text.proofread" || action.Status != "implemented" || action.Risk != "low" {
				t.Errorf("Spreadsheet proofread manifest = capability %q, status %q, risk %q; want the shared low-risk bounded text query", action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "word/home/select" {
			if action.Capability != "word.view.set" || action.Status != "implemented" || action.Risk != "low" {
				t.Errorf("Word Select manifest = capability %q, status %q, risk %q; want the shared low-risk Word view and text-selection workflow", action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "word/home/editor" || action.SourceKey == "word/review/review-editor" {
			if action.Capability != "word.review.editor.analyze" || action.Status != "implemented" || action.Risk != "low" {
				t.Errorf("%s manifest = capability %q, status %q, risk %q; want the shared low-risk bounded Word review query", action.SourceKey, action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "word/insert/insert-shapes" || action.SourceKey == "word/insert/insert-wordart" {
			if action.Capability != "word.text_box.insert" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("%s manifest = capability %q, status %q, risk %q; want the shared medium-risk Word text-box command", action.SourceKey, action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "word/insert/insert-chart" {
			if action.Capability != "word.table.insert" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("Word chart template manifest = capability %q, status %q, risk %q; want the shared medium-risk Word table insertion command", action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "word/insert/insert-equation" {
			if action.Capability != "word.equation.insert" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("Word equation manifest = capability %q, status %q, risk %q; want the shared medium-risk Word equation-text workflow", action.Capability, action.Status, action.Risk)
			}
		}
		switch action.SourceKey {
		case "word/design/design-themes":
			if action.Capability != "word.paragraph.spacing_all.set" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("Word themes manifest = capability %q, status %q, risk %q; want the shared medium-risk whole-document spacing capability", action.Capability, action.Status, action.Risk)
			}
		case "word/home/designer":
			if action.Capability != "word.paragraph.spacing_all.set" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("Word designer manifest = capability %q, status %q, risk %q; want the shared medium-risk whole-document spacing capability", action.Capability, action.Status, action.Risk)
			}
		case "word/home/dictate":
			if action.Capability != "word.text.insert" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("Word dictation manifest = capability %q, status %q, risk %q; want the shared medium-risk explicit-target text insertion capability", action.Capability, action.Status, action.Risk)
			}
		case "word/design/design-colors":
			if action.Capability != "word.page.background.set" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("Word colors manifest = capability %q, status %q, risk %q; want the shared medium-risk page-background capability", action.Capability, action.Status, action.Risk)
			}
		case "word/design/design-fonts":
			if action.Capability != "word.text_run.font_family.set" || action.Status != "implemented" || action.Risk != "medium" {
				t.Errorf("Word fonts manifest = capability %q, status %q, risk %q; want the shared medium-risk text-run font capability", action.Capability, action.Status, action.Risk)
			}
		case "word/review/review-translate":
			if action.Capability != "word.text_run.replace" || action.Status != "implemented" || action.Risk != "low" {
				t.Errorf("Word translate manifest = capability %q, status %q, risk %q; want the shared low-risk text-run replacement capability", action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "presentation/view/view_sorter" {
			if action.Capability != "presentation.view.set" || action.Status != "implemented" || action.Risk != "low" {
				t.Errorf("Presentation slide sorter manifest = capability %q, status %q, risk %q; want the shared low-risk presentation view workflow", action.Capability, action.Status, action.Risk)
			}
		}
		if action.SourceKey == "presentation/view/view_ruler" {
			if action.Capability != "presentation.view.set" || action.Status != "implemented" || action.Risk != "low" {
				t.Errorf("Presentation ruler manifest = capability %q, status %q, risk %q; want the shared low-risk presentation view workflow", action.Capability, action.Status, action.Risk)
			}
		}
	}

	manifestSources := make([]string, 0, len(manifest.RibbonActions))
	actionIDs := make(map[string]struct{}, len(manifest.RibbonActions))
	sourceKeys := make(map[string]struct{}, len(manifest.RibbonActions))
	capabilities := make(map[string]struct{}, len(manifest.RibbonActions))
	for _, action := range manifest.RibbonActions {
		if action.ActionID == "" || action.SourceKey == "" || action.Capability == "" || action.Description == "" {
			t.Errorf("ribbon action has missing identity or description: %+v", action)
		}
		if len(action.Parameters) == 0 || len(action.InputSchema) == 0 || !json.Valid(action.InputSchema) {
			t.Errorf("ribbon action %q has no valid input schema or declared parameters", action.ActionID)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(action.InputSchema, &schema); err == nil {
			for _, parameter := range action.Parameters {
				if _, ok := schema.Properties[parameter]; !ok {
					t.Errorf("ribbon action %q declares parameter %q outside its input schema", action.ActionID, parameter)
				}
			}
		}
		if len(action.OutputSchema) == 0 || !json.Valid(action.OutputSchema) {
			t.Errorf("ribbon action %q has no valid output schema", action.ActionID)
		}
		if action.Target == "" || action.Impact == "" || action.Risk == "" {
			t.Errorf("ribbon action %q is missing target, state impact, or risk", action.ActionID)
		}
		if action.Status != "planned" && action.Status != "implemented" {
			t.Errorf("ribbon action %q has unknown Agent status %q", action.ActionID, action.Status)
		}
		if _, duplicate := actionIDs[action.ActionID]; duplicate {
			t.Errorf("duplicate Ribbon action ID %q", action.ActionID)
		}
		if _, duplicate := sourceKeys[action.SourceKey]; duplicate {
			t.Errorf("duplicate Ribbon source key %q", action.SourceKey)
		}
		actionIDs[action.ActionID] = struct{}{}
		sourceKeys[action.SourceKey] = struct{}{}
		capabilities[action.Capability] = struct{}{}
		manifestSources = append(manifestSources, action.SourceKey)
	}

	currentSources := collectRibbonActionSources()
	actualSources := make([]string, 0, len(currentSources))
	for _, source := range currentSources {
		actualSources = append(actualSources, source.Key)
	}
	sort.Strings(manifestSources)
	sort.Strings(actualSources)
	if !reflect.DeepEqual(manifestSources, actualSources) {
		t.Errorf("manifest Ribbon source keys differ from UI definitions\nmanifest: %v\nUI: %v", manifestSources, actualSources)
	}

	appRuntime, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("create capability catalog for manifest check: %v", err)
	}
	t.Cleanup(func() { _ = appRuntime.Close(context.Background()) })
	registered := make(map[string]struct{})
	for _, descriptor := range appRuntime.Catalog().Descriptors() {
		registered[descriptor.ID] = struct{}{}
	}
	for _, action := range manifest.RibbonActions {
		if action.Status != "implemented" {
			continue
		}
		if _, ok := registered[action.Capability]; !ok {
			t.Errorf("Ribbon action %q is marked implemented but capability %q is not registered", action.ActionID, action.Capability)
		}
	}

	if len(manifest.AgentOnlyCapabilities) < 3 {
		t.Fatalf("Agent-only capabilities = %d, want at least 3", len(manifest.AgentOnlyCapabilities))
	}
	seenAgentOnly := make(map[string]struct{}, len(manifest.AgentOnlyCapabilities))
	hasQuery, hasWorkflow := false, false
	for _, capability := range manifest.AgentOnlyCapabilities {
		if capability.ID == "document.compare" {
			t.Error("document.compare is listed as Agent-only even though the Word Ribbon uses it")
		}
		if capability.ID == "" || capability.Description == "" {
			t.Errorf("Agent-only capability is missing an ID or description: %+v", capability)
		}
		if _, duplicate := seenAgentOnly[capability.ID]; duplicate {
			t.Errorf("duplicate Agent-only capability %q", capability.ID)
		}
		if _, isRibbon := capabilities[capability.ID]; isRibbon {
			t.Errorf("Agent-only capability %q is already a Ribbon capability", capability.ID)
		}
		seenAgentOnly[capability.ID] = struct{}{}
		hasQuery = hasQuery || capability.Category == "query"
		hasWorkflow = hasWorkflow || capability.Category == "workflow"
	}
	if !hasQuery || !hasWorkflow {
		t.Errorf("Agent-only catalog must include a query and a workflow; hasQuery=%v hasWorkflow=%v", hasQuery, hasWorkflow)
	}

	coverageDoc, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "..", "..", "docs", "compatibility", "agent-command-coverage.md"))
	if err != nil {
		t.Fatalf("read Agent command coverage document: %v", err)
	}
	countsByFormat := make(map[string]int)
	for _, source := range currentSources {
		countsByFormat[source.Format]++
	}
	for format, count := range countsByFormat {
		row := fmt.Sprintf("| `%s` | %d |", format, count)
		if !strings.Contains(string(coverageDoc), row) {
			t.Errorf("coverage document is missing current format count %q", row)
		}
	}
}

func collectRibbonActionSources() []ribbonActionSource {
	var sources []ribbonActionSource
	add := func(format, tab string, actions []ribbonAction) {
		for _, action := range actions {
			if format == "pdf" && action.id == "page-indicator" {
				continue // This toolbar element displays the current page; it is not an action.
			}
			id := action.id
			if id == "" {
				id = "slot-" + strconv.Itoa(action.slot)
			}
			sources = append(sources, ribbonActionSource{
				Key: fmt.Sprintf("%s/%s/%s", format, tab, id), Format: format,
				Tab: tab, ID: id, Label: action.label,
			})
		}
	}

	for _, tab := range []struct {
		name  string
		value wordToolbarTab
	}{
		{"file", wordTabFile}, {"home", wordTabHome}, {"insert", wordTabInsert},
		{"design", wordTabDesign}, {"layout", wordTabLayout}, {"review", wordTabReview},
		{"view", wordTabView}, {"picture", wordTabPicture},
	} {
		add("word", tab.name, wordToolbarActions(tab.value))
	}

	for _, tab := range []struct {
		name  string
		value spreadsheetToolbarTab
	}{
		{"file", toolbarTabFile}, {"home", toolbarTabHome}, {"insert", toolbarTabInsert},
		{"page-layout", toolbarTabPageLayout}, {"formula", toolbarTabFormula},
		{"data", toolbarTabData}, {"review", toolbarTabReview}, {"view", toolbarTabView},
		{"picture", toolbarTabPicture},
	} {
		add("spreadsheet", tab.name, toolbarActionsForTab(tab.value))
	}

	for _, tab := range []string{"file", "home", "insert", "design", "transitions", "animations", "slideshow", "review", "view", "picture", "shape"} {
		add("presentation", tab, pptxActionsForTab(tab))
	}
	add("presentation", "picture-crop", pptxPictureCropActions)
	add("json", "main", jsonToolbarActions)
	add("markdown", "main", markdownToolbarActions)
	add("pdf", "main", pdfToolbarActions)

	for _, item := range []struct{ id, label string }{
		{"mode-edit", "编辑"}, {"mode-preview", "预览"}, {"mode-split", "分屏"},
	} {
		sources = append(sources, ribbonActionSource{Key: "markdown/mode/" + item.id, Format: "markdown", Tab: "mode", ID: item.id, Label: item.label})
	}
	for _, item := range []struct{ id, label string }{
		{"read", "阅读"}, {"annotate", "批注"}, {"signature", "签名"}, {"print", "打印"}, {"download", "下载"},
	} {
		sources = append(sources, ribbonActionSource{Key: "pdf/mode/" + item.id, Format: "pdf", Tab: "mode", ID: item.id, Label: item.label})
	}
	return sources
}
