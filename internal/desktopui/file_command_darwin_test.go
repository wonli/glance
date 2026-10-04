//go:build darwin

package desktopui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartFileCommandUsesSavePickerForPDFExport(t *testing.T) {
	pickerDir := t.TempDir()
	wantPath := filepath.Join(t.TempDir(), "export.pdf")
	shim := "#!/bin/sh\nprintf '%s\\n' '" + wantPath + "'\n"
	if err := os.WriteFile(filepath.Join(pickerDir, "osascript"), []byte(shim), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pickerDir)

	app := &App{
		fileCommandResults: make(chan fileCommandResult, 1),
		shutdown:           make(chan struct{}),
	}
	app.startFileCommand(commandExportPDF, "pdf")

	select {
	case result := <-app.fileCommandResults:
		if result.err != nil {
			t.Fatalf("PDF save picker error: %v", result.err)
		}
		if result.kind != commandExportPDF || result.path != wantPath {
			t.Fatalf("PDF file command result = %#v, want kind=%v path=%q", result, commandExportPDF, wantPath)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("PDF export did not invoke the native save-path picker")
	}
}
