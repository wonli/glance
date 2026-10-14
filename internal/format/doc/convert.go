package doc

import (
	"context"
	"os"
	"sync"
)

// TempDOCX is a private on-disk DOCX staging package produced from a legacy
// Word document. The DOCX adapter must finish loading it before Cleanup.
type TempDOCX struct {
	Path      string
	directory string
	mu        sync.Mutex
	cleaned   bool
}

// ConvertToTempDOCX converts a legacy .doc file into an app-private DOCX file.
// The source file is never used as an output path or modified.
func ConvertToTempDOCX(ctx context.Context, sourcePath string) (*TempDOCX, error) {
	return convertToTempDOCX(ctx, sourcePath)
}

// Cleanup removes the private staging directory. It is safe to call twice.
func (t *TempDOCX) Cleanup() error {
	if t == nil || t.directory == "" {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cleaned {
		return nil
	}
	if err := os.RemoveAll(t.directory); err != nil {
		return err
	}
	t.cleaned = true
	return nil
}
