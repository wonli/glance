package i18n

import (
	"testing"

	"golang.org/x/text/language"
)

func TestNormalizePreferenceDefaultsAndRejectsInvalid(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		found      bool
		want       Selection
		wantRepair bool
	}{
		{name: "missing uses system", found: false, want: SelectionSystem},
		{name: "system", value: "system", found: true, want: SelectionSystem},
		{name: "Chinese", value: "zh-CN", found: true, want: SelectionChinese},
		{name: "English", value: "en-US", found: true, want: SelectionEnglish},
		{name: "invalid falls back to English", value: "fr-FR", found: true, want: SelectionEnglish, wantRepair: true},
		{name: "empty stored value falls back to English", value: "", found: true, want: SelectionEnglish, wantRepair: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, repair := NormalizePreference(test.value, test.found)
			if got != test.want || repair != test.wantRepair {
				t.Fatalf("NormalizePreference(%q, %t) = (%q, %t), want (%q, %t)", test.value, test.found, got, repair, test.want, test.wantRepair)
			}
		})
	}
}

func TestValidateEmbeddedCatalogs(t *testing.T) {
	if err := ValidateEmbedded(); err != nil {
		t.Fatal(err)
	}
}

func TestResolveExplicitSelection(t *testing.T) {
	preferred := []language.Tag{language.MustParse("zh-CN")}
	if got := Resolve(SelectionEnglish, preferred); got != LocaleEnglish {
		t.Fatalf("explicit English resolved to %q", got)
	}
	if got := Resolve(SelectionChinese, nil); got != LocaleChinese {
		t.Fatalf("explicit Chinese resolved to %q", got)
	}
}

func TestResolveChineseSystemTags(t *testing.T) {
	for _, raw := range []string{"zh", "zh-CN", "zh-TW", "zh-Hans-SG"} {
		t.Run(raw, func(t *testing.T) {
			if got := Resolve(SelectionSystem, []language.Tag{language.MustParse(raw)}); got != LocaleChinese {
				t.Fatalf("Resolve(system, %s) = %q, want %q", raw, got, LocaleChinese)
			}
		})
	}
}

func TestResolveEnglishSystemTags(t *testing.T) {
	for _, raw := range []string{"en", "en-US", "en-GB"} {
		t.Run(raw, func(t *testing.T) {
			if got := Resolve(SelectionSystem, []language.Tag{language.MustParse(raw)}); got != LocaleEnglish {
				t.Fatalf("Resolve(system, %s) = %q, want %q", raw, got, LocaleEnglish)
			}
		})
	}
}

func TestResolveUnsupportedOrEmptySystemTags(t *testing.T) {
	if got := Resolve(SelectionSystem, []language.Tag{language.MustParse("fr-FR")}); got != LocaleEnglish {
		t.Fatalf("unsupported system language resolved to %q, want %q", got, LocaleEnglish)
	}
	if got := Resolve(SelectionSystem, nil); got != LocaleEnglish {
		t.Fatalf("empty system language list resolved to %q, want %q", got, LocaleEnglish)
	}
}
