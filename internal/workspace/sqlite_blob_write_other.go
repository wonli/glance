//go:build !darwin || !cgo

package workspace

import "io"

func (s *Store) saveDocumentStreamPlatform(doc Document, payload io.Reader, payloadSize int64) error {
	return s.saveDocumentStreamTranslated(doc, payload, payloadSize)
}
