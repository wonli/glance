package officeqa

import (
	"context"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestRunRejectsScreenshotDirectorySymlink(t *testing.T) {
	root, output, outside := t.TempDir(), filepath.Join(t.TempDir(), "run"), t.TempDir()
	writeInput(t, root, "one.xlsx", []byte("xlsx"))
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "target-001.png")
	if err := os.WriteFile(sentinel, []byte("preserve external file"), 0o600); err != nil {
		t.Fatal(err)
	}
	caseDir := filepath.Join(output, generatedCaseID("one.xlsx"))
	if err := os.Symlink(outside, caseDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{
		"one.xlsx": {targets: []Target{officeTarget("worksheet-1", color.NRGBA{R: 30, G: 40, B: 50, A: 255})}},
	}}
	report := Run(context.Background(), RunConfig{InputPath: root, OutputDir: output}, renderer)
	contents, err := os.ReadFile(sentinel)
	if err != nil || string(contents) != "preserve external file" {
		t.Fatalf("renderer followed output symlink and changed external file: %q, error=%v", contents, err)
	}
	if report.Files[0].Status != StatusRenderFailed {
		t.Fatalf("symlinked screenshot path status = %+v", report.Files[0])
	}
}

func TestRunRejectsScreenshotFileSymlink(t *testing.T) {
	root, output, outside := t.TempDir(), filepath.Join(t.TempDir(), "run"), t.TempDir()
	writeInput(t, root, "one.xlsx", []byte("xlsx"))
	caseID := generatedCaseID("one.xlsx")
	caseDir := filepath.Join(output, caseID)
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "preserve.png")
	if err := os.WriteFile(sentinel, []byte("preserve external file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sentinel, filepath.Join(caseDir, "target-001.png")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{
		"one.xlsx": {targets: []Target{officeTarget("worksheet-1", color.NRGBA{R: 30, G: 40, B: 50, A: 255})}},
	}}
	report := Run(context.Background(), RunConfig{InputPath: root, OutputDir: output}, renderer)
	contents, err := os.ReadFile(sentinel)
	if err != nil || string(contents) != "preserve external file" {
		t.Fatalf("renderer followed screenshot symlink and changed external file: %q, error=%v", contents, err)
	}
	if report.Files[0].Status != StatusRenderFailed {
		t.Fatalf("symlinked screenshot file status = %+v", report.Files[0])
	}
}
