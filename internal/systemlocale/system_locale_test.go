package systemlocale

import (
	"testing"

	"golang.org/x/text/language"
)

func TestParsePreferredLanguagesNormalizesLocaleNames(t *testing.T) {
	tags := parsePreferredLanguages([]string{"zh_CN.UTF-8", "en-GB", "C", "bad_locale_!"})
	want := []language.Tag{language.MustParse("zh-CN"), language.MustParse("en-GB")}
	if len(tags) != len(want) {
		t.Fatalf("parsePreferredLanguages returned %v, want %v", tags, want)
	}
	for index := range want {
		if tags[index] != want[index] {
			t.Errorf("tag %d = %q, want %q", index, tags[index], want[index])
		}
	}
}

func TestParsePreferredLanguagesReturnsEmptyForUnsupportedValues(t *testing.T) {
	if tags := parsePreferredLanguages([]string{"", "C", "POSIX", "not a locale"}); len(tags) != 0 {
		t.Fatalf("unsupported values parsed as %v", tags)
	}
}
