//go:build !darwin || !cgo

package fontcatalog

func availableSystemFontFiles() ([]string, error) {
	return nil, nil
}

func availableSystemFontFamilyNames() ([]string, error) {
	return nil, nil
}
