//go:build !darwin || !cgo

package ocr

func recognizeCharacterImage(_ []byte) ([]detectedTextLine, error) {
	return nil, ErrCharacterLocalizationUnavailable
}
