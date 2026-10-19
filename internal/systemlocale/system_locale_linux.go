//go:build linux

package systemlocale

import (
	"os"

	"golang.org/x/text/language"
)

func PreferredLanguages() []language.Tag {
	return preferredLanguagesFromEnvironment(os.LookupEnv)
}
