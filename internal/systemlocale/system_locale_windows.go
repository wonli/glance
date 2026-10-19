//go:build windows

package systemlocale

import (
	"golang.org/x/sys/windows"
	"golang.org/x/text/language"
)

func PreferredLanguages() []language.Tag {
	values, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil {
		return nil
	}
	return parsePreferredLanguages(values)
}
