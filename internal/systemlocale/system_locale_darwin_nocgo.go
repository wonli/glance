//go:build darwin && !cgo

package systemlocale

import "golang.org/x/text/language"

func PreferredLanguages() []language.Tag {
	return nil
}
