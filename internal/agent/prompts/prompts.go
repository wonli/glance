package prompts

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed *.tmpl
var files embed.FS

type SystemData struct {
	ViewContext string
}

type TitleData struct {
	UserMessage string
}

func System(data SystemData) (string, error) {
	return render("system.tmpl", data)
}

func ConversationTitle(data TitleData) (string, error) {
	return render("conversation_title.tmpl", data)
}

func render(name string, data any) (string, error) {
	contents, err := files.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("read Agent prompt %s: %w", name, err)
	}
	parsed, err := template.New(name).Option("missingkey=error").Parse(string(contents))
	if err != nil {
		return "", fmt.Errorf("parse Agent prompt %s: %w", name, err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return "", fmt.Errorf("render Agent prompt %s: %w", name, err)
	}
	return output.String(), nil
}
