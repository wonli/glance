package i18n

import (
	"testing"
	"time"
)

func TestFormatDateTimeUsesLocaleConventions(t *testing.T) {
	value := time.Date(2026, time.October, 18, 15, 4, 0, 0, time.UTC)
	if got, want := FormatDateTime(LocaleChinese, value), "2026-10-18 15:04"; got != want {
		t.Errorf("Chinese FormatDateTime() = %q, want %q", got, want)
	}
	if got, want := FormatDateTime(LocaleEnglish, value), "Oct 18, 2026, 3:04 PM"; got != want {
		t.Errorf("English FormatDateTime() = %q, want %q", got, want)
	}
}

func TestFormatNumberUsesLocaleDecimalSeparator(t *testing.T) {
	value := 1234.5
	if got, want := FormatNumber(LocaleChinese, value), "1,234.5"; got != want {
		t.Errorf("Chinese FormatNumber() = %q, want %q", got, want)
	}
	if got, want := FormatNumber(LocaleEnglish, value), "1,234.5"; got != want {
		t.Errorf("English FormatNumber() = %q, want %q", got, want)
	}
}
