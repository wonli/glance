//go:build !darwin && !windows && !linux

package systemlocale

import "golang.org/x/text/language"

func PreferredLanguages() []language.Tag {
	return nil
}
