package i18n

import (
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

func FormatDateTime(locale Locale, value time.Time) string {
	if locale == LocaleChinese {
		return value.Format("2006-01-02 15:04")
	}
	return value.Format("Jan 2, 2006, 3:04 PM")
}

func FormatNumber(locale Locale, value float64) string {
	tag := language.AmericanEnglish
	if locale == LocaleChinese {
		tag = language.SimplifiedChinese
	}
	printer := message.NewPrinter(tag)
	return printer.Sprintf("%v", number.Decimal(value, number.MaxFractionDigits(6)))
}
