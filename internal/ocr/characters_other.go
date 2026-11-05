//go:build !windows && (!darwin || !cgo)

package ocr

import "context"

func recognizeCharacterImage(_ context.Context, _ []byte) ([]detectedTextLine, error) {
	return nil, ErrCharacterLocalizationUnavailable
}
