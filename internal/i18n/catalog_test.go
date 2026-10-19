package i18n

import (
	"strings"
	"testing"
)

func TestCatalogsHaveIdenticalCompleteKeySets(t *testing.T) {
	english, err := readCatalog("en-US")
	if err != nil {
		t.Fatal(err)
	}
	chinese, err := readCatalog("zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateEmbedded(); err != nil {
		t.Fatalf("embedded catalogs are incomplete or invalid: %v", err)
	}
	if len(english) != len(chinese) {
		t.Fatalf("catalog key counts differ: en-US=%d zh-CN=%d", len(english), len(chinese))
	}
	for id := range english {
		if _, ok := chinese[id]; !ok {
			t.Errorf("zh-CN catalog is missing %q", id)
		}
	}
	for id := range chinese {
		if _, ok := english[id]; !ok {
			t.Errorf("en-US catalog is missing %q", id)
		}
	}
}

func TestCatalogUsesEnglishFallback(t *testing.T) {
	catalog, err := newCatalog(LocaleChinese, map[string]any{
		"greeting": "你好，{{.Name}}",
	}, map[string]any{
		"greeting":  "Hello, {{.Name}}",
		"save.done": "Saved",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	got := catalog.Text(Message{ID: "save.done"})
	if got != "Saved" {
		t.Fatalf("Text(save.done) = %q, want English fallback %q", got, "Saved")
	}
}

func TestCatalogInterpolatesNamedArguments(t *testing.T) {
	catalog, err := newCatalog(LocaleEnglish, map[string]any{
		"workspace.opened": "Opened {{.FileName}} in {{.FolderName}}",
	}, map[string]any{
		"workspace.opened": "Opened {{.FileName}} in {{.FolderName}}",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	got := catalog.Text(Message{ID: "workspace.opened", Args: Args{
		"FileName":   "report.docx",
		"FolderName": "Recent files",
	}})
	if got != "Opened report.docx in Recent files" {
		t.Fatalf("Text() = %q", got)
	}
}

func TestCatalogRejectsMissingEnglishAndMismatchedArguments(t *testing.T) {
	err := validateCatalogPair(
		map[string]any{"shared": "Hello {{.Name}}"},
		map[string]any{"localized": "Localized"},
	)
	if err == nil || !strings.Contains(err.Error(), "missing English") {
		t.Fatalf("missing English entry error = %v, want a missing-English error", err)
	}

	err = validateCatalogPair(
		map[string]any{"shared": "Hello {{.Name}}"},
		map[string]any{"shared": "你好，{{.User}}"},
	)
	if err == nil || !strings.Contains(err.Error(), "argument") {
		t.Fatalf("argument mismatch error = %v, want an argument mismatch", err)
	}
}

func TestCatalogPluralFormsForChineseAndEnglish(t *testing.T) {
	entries := map[string]any{
		"files.count": map[string]string{
			"one":   "{{.Count}} file",
			"other": "{{.Count}} files",
		},
	}
	english, err := newCatalog(LocaleEnglish, entries, entries, nil)
	if err != nil {
		t.Fatal(err)
	}
	chinese, err := newCatalog(LocaleChinese, entries, entries, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got := english.Plural(Message{ID: "files.count"}, 1); got != "1 file" {
		t.Errorf("English singular = %q", got)
	}
	if got := english.Plural(Message{ID: "files.count"}, 3); got != "3 files" {
		t.Errorf("English plural = %q", got)
	}
	if got := chinese.Plural(Message{ID: "files.count"}, 1); got != "1 files" {
		t.Errorf("Chinese uses the other form for count 1 = %q", got)
	}
}

func TestCatalogUsesEnglishPluralRulesForFallbackEntries(t *testing.T) {
	englishEntries := map[string]any{
		"files.count": map[string]string{
			"one":   "{{.Count}} file",
			"other": "{{.Count}} files",
		},
	}
	catalog, err := newCatalog(LocaleChinese, map[string]any{}, englishEntries, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Plural(Message{ID: "files.count"}, 1); got != "1 file" {
		t.Fatalf("English fallback singular = %q, want %q", got, "1 file")
	}
}

func TestCatalogRejectsInvalidTemplate(t *testing.T) {
	_, err := newCatalog(LocaleEnglish, map[string]any{
		"broken": "Hello {{.Name",
	}, map[string]any{
		"broken": "Hello {{.Name",
	}, nil)
	if err == nil {
		t.Fatal("newCatalog accepted an invalid template")
	}
}
