package prompts

import (
	"strings"
	"testing"
)

func TestSystemPromptIncludesViewContext(t *testing.T) {
	view := `{"document_id":"doc-1","selection":{"cell":"A1"}}`
	got, err := System(SystemData{ViewContext: view})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, view) || !strings.Contains(got, "工具调用") {
		t.Fatalf("system prompt omitted data or instruction: %q", got)
	}
}

func TestConversationTitlePromptTreatsUserMessageAsLiteralData(t *testing.T) {
	message := "总结 {{.ViewContext}} 并检查 A1"
	got, err := ConversationTitle(TitleData{UserMessage: message})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, message) {
		t.Fatalf("title prompt omitted user message: %q", got)
	}
	if strings.Contains(got, "\"document_id\"") {
		t.Fatalf("user message was executed as template syntax: %q", got)
	}
}

type testPromptsEmptyArguments struct {
}

func TestRenderRejectsMissingTemplateData(t *testing.T) {
	if _, err := render("system.tmpl", testPromptsEmptyArguments{}); err == nil {
		t.Fatal("render succeeded with a missing required template key")
	}
}
