package officeqa

import (
	"context"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestRunPersistsReportForInvalidInput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "run")
	report := Run(context.Background(), RunConfig{InputPath: filepath.Join(t.TempDir(), "missing"), OutputDir: output}, &fakeRenderer{})
	if report.RunError == "" {
		t.Fatal("invalid input did not produce a run error")
	}
	if _, err := os.Stat(filepath.Join(output, "report.json")); err != nil {
		t.Fatalf("invalid-input report missing: %v", err)
	}
}

func TestRunStopsWhenHeadlessRendererUnavailable(t *testing.T) {
	root, output := t.TempDir(), filepath.Join(t.TempDir(), "run")
	writeInput(t, root, "a.xlsx", []byte("first"))
	writeInput(t, root, "b.docx", []byte("second"))
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{"a.xlsx": {openErr: ErrRendererUnavailable}}}
	report := Run(context.Background(), RunConfig{InputPath: root, OutputDir: output}, renderer)
	if report.RunError == "" || len(renderer.opened) != 1 {
		t.Fatalf("headless failure was not run-level: error=%q opened=%v", report.RunError, renderer.opened)
	}
	if report.Files[0].Status != StatusNotRun || report.Files[1].Status != StatusNotRun || report.Counts.NotRun != 2 {
		t.Fatalf("unavailable renderer status = %+v, counts=%+v", report.Files, report.Counts)
	}
}

func TestRunRejectsOversizedInputBeforeOpening(t *testing.T) {
	root := t.TempDir()
	writeInput(t, root, "oversized.xlsx", []byte("12345"))
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{}}
	report := Run(context.Background(), RunConfig{InputPath: root, OutputDir: filepath.Join(t.TempDir(), "run"), MaxInputBytes: 4}, renderer)
	if len(renderer.opened) != 0 || report.Files[0].Status != StatusOpenFailed || report.FailedCount != 1 {
		t.Fatalf("oversized input was passed to renderer: opened=%v files=%+v", renderer.opened, report.Files)
	}
}

func TestRunUsesUniqueIDsWhenGeneratedIDConflictsWithManifest(t *testing.T) {
	root := t.TempDir()
	writeInput(t, root, "plain.xlsx", []byte("plain"))
	writeInput(t, root, "z-manifested.xlsx", []byte("manifested"))
	writeManifestFile(t, root, manifestCase(generatedCaseID("plain.xlsx"), "z-manifested.xlsx", "xlsx"))
	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{
		"plain.xlsx":        {targets: []Target{officeTarget("worksheet-1", color.NRGBA{R: 1, G: 2, B: 3, A: 255})}},
		"z-manifested.xlsx": {targets: []Target{officeTarget("worksheet-1", color.NRGBA{R: 4, G: 5, B: 6, A: 255})}},
	}}
	report := Run(context.Background(), RunConfig{InputPath: root, OutputDir: filepath.Join(t.TempDir(), "run"), Manifest: &manifest}, renderer)
	if report.Files[0].CaseID == report.Files[1].CaseID {
		t.Fatalf("case IDs collide: %+v", report.Files)
	}
	if report.Files[0].Targets[0].Path == report.Files[1].Targets[0].Path {
		t.Fatalf("screenshot paths collide: %+v", report.Files)
	}
}
