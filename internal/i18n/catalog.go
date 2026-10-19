package i18n

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
	"text/template"
	"text/template/parse"
)

//go:embed locales/*.json
var localeFiles embed.FS

var messageIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._][a-z0-9]+)*$`)

type compiledEntry struct {
	forms map[string]compiledForm
}

// Literal forms need neither a template tree nor an execution buffer.
type compiledForm struct {
	literal  string
	template *template.Template
}

func (f compiledForm) arguments() (map[string]struct{}, error) {
	if f.template == nil {
		return nil, nil
	}
	return templateArguments(f.template)
}

type Catalog struct {
	locale    Locale
	entries   map[string]compiledEntry
	english   map[string]compiledEntry
	onMissing func(id string)
}

func NewCatalog(locale Locale, onMissing func(id string)) (*Catalog, error) {
	if locale != LocaleChinese && locale != LocaleEnglish {
		return nil, fmt.Errorf("unsupported catalog locale %q", locale)
	}
	englishData, err := readCatalog("en-US")
	if err != nil {
		return nil, err
	}
	selectedData := englishData
	if locale != LocaleEnglish {
		selectedData, err = readCatalog(string(locale))
		if err != nil {
			return nil, err
		}
	}
	return newCatalog(locale, selectedData, englishData, onMissing)
}

func readCatalog(locale string) (map[string]any, error) {
	data, err := localeFiles.ReadFile("locales/" + locale + ".json")
	if err != nil {
		return nil, fmt.Errorf("read embedded %s catalog: %w", locale, err)
	}
	return decodeCatalog(data)
}

func decodeCatalog(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode catalog: %w", err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("catalog must be a JSON object")
	}

	entries := make(map[string]any)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode catalog key: %w", err)
		}
		id, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("catalog key is not a string")
		}
		if _, duplicate := entries[id]; duplicate {
			return nil, fmt.Errorf("duplicate catalog message ID %q", id)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decode message %q: %w", id, err)
		}
		value, err := decodeEntry(raw)
		if err != nil {
			return nil, fmt.Errorf("message %q: %w", id, err)
		}
		entries[id] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("close catalog object: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("catalog has trailing JSON data")
		}
		return nil, fmt.Errorf("catalog trailing data: %w", err)
	}
	return entries, nil
}

func decodeEntry(raw json.RawMessage) (any, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	forms, err := decodePluralForms(raw)
	if err != nil {
		return nil, fmt.Errorf("entry must be a string or one/other plural object: %w", err)
	}
	return forms, nil
}

func decodePluralForms(raw json.RawMessage) (map[string]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, fmt.Errorf("plural entry must be an object")
	}
	forms := make(map[string]string)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("plural form name is not a string")
		}
		if _, duplicate := forms[name]; duplicate {
			return nil, fmt.Errorf("duplicate plural form %q", name)
		}
		var value string
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("plural form %q must be a string: %w", name, err)
		}
		forms[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if len(forms) != 2 || forms["one"] == "" || forms["other"] == "" {
		return nil, fmt.Errorf("plural entry must provide non-empty one and other forms")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("plural entry has trailing data")
	}
	return forms, nil
}

func newCatalog(locale Locale, selected, english map[string]any, onMissing func(id string)) (*Catalog, error) {
	if locale != LocaleChinese && locale != LocaleEnglish {
		return nil, fmt.Errorf("unsupported catalog locale %q", locale)
	}
	compiledEnglish, err := compileEntries(english)
	if err != nil {
		return nil, fmt.Errorf("compile English catalog: %w", err)
	}
	compiledSelected, err := compileEntries(selected)
	if err != nil {
		return nil, fmt.Errorf("compile %s catalog: %w", locale, err)
	}
	if err := validateCompiledPair(compiledEnglish, compiledSelected); err != nil {
		return nil, err
	}
	return &Catalog{
		locale:    locale,
		entries:   compiledSelected,
		english:   compiledEnglish,
		onMissing: onMissing,
	}, nil
}

func compileEntries(entries map[string]any) (map[string]compiledEntry, error) {
	compiled := make(map[string]compiledEntry, len(entries))
	for id, raw := range entries {
		if !messageIDPattern.MatchString(id) {
			return nil, fmt.Errorf("invalid message ID %q", id)
		}
		forms, err := entryForms(raw)
		if err != nil {
			return nil, fmt.Errorf("message %q: %w", id, err)
		}
		compiledForms := make(map[string]compiledForm, len(forms))
		for category, source := range forms {
			if !strings.Contains(source, "{{") {
				compiledForms[category] = compiledForm{literal: source}
				continue
			}
			t, err := template.New(id + "." + category).Option("missingkey=error").Parse(source)
			if err != nil {
				return nil, fmt.Errorf("message %q has invalid template: %w", id, err)
			}
			if _, err := templateArguments(t); err != nil {
				return nil, fmt.Errorf("message %q has unsupported template fields: %w", id, err)
			}
			compiledForms[category] = compiledForm{template: t}
		}
		compiled[id] = compiledEntry{forms: compiledForms}
	}
	return compiled, nil
}

func entryForms(value any) (map[string]string, error) {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil, fmt.Errorf("message text must not be empty")
		}
		return map[string]string{"text": typed}, nil
	case map[string]string:
		if len(typed) != 2 || typed["one"] == "" || typed["other"] == "" {
			return nil, fmt.Errorf("plural entry must provide non-empty one and other forms")
		}
		if _, ok := typed["one"]; !ok {
			return nil, fmt.Errorf("plural entry is missing one form")
		}
		if _, ok := typed["other"]; !ok {
			return nil, fmt.Errorf("plural entry is missing other form")
		}
		return typed, nil
	default:
		return nil, fmt.Errorf("entry has unsupported type %T", value)
	}
}

func templateArguments(t *template.Template) (map[string]struct{}, error) {
	args := make(map[string]struct{})
	var visit func(parse.Node) error
	visit = func(node parse.Node) error {
		if node == nil {
			return nil
		}
		value := reflect.ValueOf(node)
		if value.Kind() == reflect.Pointer && value.IsNil() {
			return nil
		}
		switch typed := node.(type) {
		case *parse.ListNode:
			for _, child := range typed.Nodes {
				if err := visit(child); err != nil {
					return err
				}
			}
		case *parse.ActionNode:
			return visit(typed.Pipe)
		case *parse.PipeNode:
			for _, command := range typed.Cmds {
				if err := visit(command); err != nil {
					return err
				}
			}
		case *parse.CommandNode:
			for _, arg := range typed.Args {
				if err := visit(arg); err != nil {
					return err
				}
			}
		case *parse.FieldNode:
			if len(typed.Ident) != 1 {
				return fmt.Errorf("only top-level named fields are supported: .%s", strings.Join(typed.Ident, "."))
			}
			args[typed.Ident[0]] = struct{}{}
		case *parse.IfNode:
			if err := visit(typed.Pipe); err != nil {
				return err
			}
			if err := visit(typed.List); err != nil {
				return err
			}
			return visit(typed.ElseList)
		case *parse.RangeNode:
			if err := visit(typed.Pipe); err != nil {
				return err
			}
			if err := visit(typed.List); err != nil {
				return err
			}
			return visit(typed.ElseList)
		case *parse.WithNode:
			if err := visit(typed.Pipe); err != nil {
				return err
			}
			if err := visit(typed.List); err != nil {
				return err
			}
			return visit(typed.ElseList)
		}
		return nil
	}
	if err := visit(t.Tree.Root); err != nil {
		return nil, err
	}
	return args, nil
}

func validateCatalogPair(english, localized map[string]any) error {
	compiledEnglish, err := compileEntries(english)
	if err != nil {
		return fmt.Errorf("compile English catalog: %w", err)
	}
	compiledLocalized, err := compileEntries(localized)
	if err != nil {
		return fmt.Errorf("compile localized catalog: %w", err)
	}
	return validateCompiledPair(compiledEnglish, compiledLocalized)
}

func validateCompiledPair(english, localized map[string]compiledEntry) error {
	for id, translated := range localized {
		fallback, exists := english[id]
		if !exists {
			return fmt.Errorf("message %q is missing English fallback", id)
		}
		if len(fallback.forms) != len(translated.forms) {
			return fmt.Errorf("message %q has mismatched plural forms", id)
		}
		for category, translatedTemplate := range translated.forms {
			englishTemplate, exists := fallback.forms[category]
			if !exists {
				return fmt.Errorf("message %q has mismatched plural form %q", id, category)
			}
			translatedArgs, err := translatedTemplate.arguments()
			if err != nil {
				return fmt.Errorf("message %q: %w", id, err)
			}
			englishArgs, err := englishTemplate.arguments()
			if err != nil {
				return fmt.Errorf("message %q: %w", id, err)
			}
			if !sameArguments(englishArgs, translatedArgs) {
				return fmt.Errorf("message %q has mismatched named arguments", id)
			}
		}
	}
	return nil
}

func sameArguments(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for arg := range left {
		if _, ok := right[arg]; !ok {
			return false
		}
	}
	return true
}

func ValidateEmbedded() error {
	english, err := readCatalog("en-US")
	if err != nil {
		return err
	}
	chinese, err := readCatalog("zh-CN")
	if err != nil {
		return err
	}
	if _, ok := english["common.translation_missing"]; !ok {
		return fmt.Errorf("English catalog is missing common.translation_missing")
	}
	if _, ok := chinese["common.translation_missing"]; !ok {
		return fmt.Errorf("Chinese catalog is missing common.translation_missing")
	}
	return validateCatalogPair(english, chinese)
}

func (c *Catalog) Text(message Message) string {
	entry, ok := c.entries[message.ID]
	if !ok {
		entry, ok = c.english[message.ID]
	}
	if !ok {
		if c.onMissing != nil {
			c.onMissing(message.ID)
		}
		fallbackID := "common.translation_missing"
		entry, ok = c.entries[fallbackID]
		if !ok {
			entry, ok = c.english[fallbackID]
		}
		if !ok {
			return "Missing translation: " + message.ID
		}
		message = Message{ID: fallbackID, Args: Args{"ID": message.ID}}
	}
	return c.render(entry, "text", message.Args, message.ID)
}

func (c *Catalog) Plural(message Message, count int64) string {
	entry, ok := c.entries[message.ID]
	entryLocale := c.locale
	if !ok {
		entry, ok = c.english[message.ID]
		entryLocale = LocaleEnglish
	}
	if !ok {
		return c.Text(message)
	}
	category := "other"
	if entryLocale == LocaleEnglish && count == 1 {
		category = "one"
	}
	args := make(Args, len(message.Args)+1)
	for key, value := range message.Args {
		args[key] = value
	}
	if _, exists := args["Count"]; !exists {
		args["Count"] = count
	}
	if _, isPlural := entry.forms["one"]; !isPlural {
		return c.render(entry, "text", args, message.ID)
	}
	return c.render(entry, category, args, message.ID)
}

func (c *Catalog) render(entry compiledEntry, category string, args Args, id string) string {
	t, ok := entry.forms[category]
	if !ok {
		t, ok = entry.forms["other"]
	}
	if !ok {
		if c.onMissing != nil {
			c.onMissing(id)
		}
		return ""
	}
	if t.template == nil {
		return t.literal
	}
	var output bytes.Buffer
	if err := t.template.Execute(&output, args); err != nil {
		if c.onMissing != nil {
			c.onMissing(id)
		}
		return ""
	}
	return output.String()
}
