package officeqa

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestRunWritesScreenshotsAndRelativeReportPaths(t *testing.T) {
	root, output := t.TempDir(), filepath.Join(t.TempDir(), "run")
	writeInput(t, root, "xlsx/book.xlsx", []byte("xlsx source"))
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{
		"book.xlsx": {targets: []Target{officeTarget("worksheet-1", color.NRGBA{R: 25, G: 90, B: 160, A: 255})}},
	}}
	report := Run(context.Background(), RunConfig{InputPath: root, OutputDir: output}, renderer)
	if report.RunError != "" {
		t.Fatalf("run error = %q", report.RunError)
	}
	if len(report.Files) != 1 || report.Files[0].Path != "xlsx/book.xlsx" {
		t.Fatalf("report paths = %+v", report.Files)
	}
	if report.Files[0].Status != StatusRendered || report.Files[0].TargetCount != 1 {
		t.Fatalf("file result = %+v", report.Files[0])
	}
	if len(report.CorpusFingerprint) != 64 {
		t.Fatalf("corpus fingerprint = %q, want SHA-256 hex", report.CorpusFingerprint)
	}
	if _, err := os.Stat(filepath.Join(output, "report.json")); err != nil {
		t.Fatalf("report.json missing: %v", err)
	}
	imagePath := filepath.Join(output, report.Files[0].Targets[0].Path)
	file, err := os.Open(imagePath)
	if err != nil {
		t.Fatalf("screenshot missing: %v", err)
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil || decoded.Bounds().Dx() != 16 || decoded.Bounds().Dy() != 12 {
		t.Fatalf("screenshot decode = %v, error = %v", decoded, err)
	}
}

func TestRunBlankScreenshotFails(t *testing.T) {
	root, output := t.TempDir(), filepath.Join(t.TempDir(), "run")
	writeInput(t, root, "blank.docx", []byte("docx"))
	blank := image.NewRGBA(image.Rect(0, 0, 16, 12))
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{
		"blank.docx": {targets: []Target{{Name: "viewport-1", Image: blank}}},
	}}
	report := Run(context.Background(), RunConfig{InputPath: root, OutputDir: output}, renderer)
	if report.Files[0].Status != StatusRenderFailed || report.FailedCount != 1 {
		t.Fatalf("blank screenshot result = %+v; counts=%+v", report.Files[0], report.Counts)
	}
	if report.Files[0].Error == "" {
		t.Fatal("blank screenshot failure has no diagnostic")
	}
}

func TestRunContinuesAfterOpenAndRenderFailures(t *testing.T) {
	root, output := t.TempDir(), filepath.Join(t.TempDir(), "run")
	writeInput(t, root, "a-open.docx", []byte("bad docx"))
	writeInput(t, root, "b-render.xlsx", []byte("bad render"))
	writeInput(t, root, "c-ok.pptx", []byte("pptx"))
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{
		"a-open.docx":   {openErr: errors.New("parse failure")},
		"b-render.xlsx": {renderErr: errors.New("layout failure")},
		"c-ok.pptx":     {targets: []Target{officeTarget("slide-1", color.NRGBA{R: 20, G: 100, B: 30, A: 255})}},
	}}
	report := Run(context.Background(), RunConfig{InputPath: root, OutputDir: output}, renderer)
	if len(report.Files) != 3 || len(renderer.opened) != 3 {
		t.Fatalf("runner stopped early: files=%d opened=%v", len(report.Files), renderer.opened)
	}
	wantStatuses := []FileStatus{StatusOpenFailed, StatusRenderFailed, StatusRendered}
	for index, want := range wantStatuses {
		if report.Files[index].Status != want {
			t.Errorf("file %d status = %q, want %q", index, report.Files[index].Status, want)
		}
	}
	if report.FailedCount != 2 {
		t.Fatalf("failed count = %d, want 2", report.FailedCount)
	}
	if _, err := os.Stat(filepath.Join(output, "report.json")); err != nil {
		t.Fatalf("partial report was not persisted: %v", err)
	}
}

func TestRunRecordsDiagnosticsSeparatelyFromRenderFailure(t *testing.T) {
	root := t.TempDir()
	writeInput(t, root, "warning.docx", []byte("docx"))
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{
		"warning.docx": {
			targets:     []Target{officeTarget("viewport-1", color.NRGBA{R: 22, G: 30, B: 40, A: 255})},
			diagnostics: []string{"page-setup-defaulted"},
		},
	}}
	report := Run(context.Background(), RunConfig{InputPath: root, OutputDir: filepath.Join(t.TempDir(), "run")}, renderer)
	if report.Files[0].Status != StatusCompletedWithDiagnostics || len(report.Files[0].Diagnostics) != 1 || report.FailedCount != 0 {
		t.Fatalf("diagnostic result = %+v; counts=%+v", report.Files[0], report.Counts)
	}
}

func TestRunRequiresManifestExpectedDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name        string
		diagnostics []string
		wantMet     bool
	}{
		{name: "present with additional diagnostics", diagnostics: []string{"error:nested-table", "warning:other"}, wantMet: true},
		{name: "missing", diagnostics: []string{"warning:other"}, wantMet: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, output := t.TempDir(), filepath.Join(t.TempDir(), "run")
			writeInput(t, root, "drawing-shapes.docx", []byte("docx"))
			entry := manifestCase("nested-table", "drawing-shapes.docx", "docx")
			entry["expected_diagnostics"] = []string{"error:nested-table"}
			writeManifestFile(t, root, []map[string]any{entry})
			manifest, err := LoadManifest(root)
			if err != nil {
				t.Fatal(err)
			}
			renderer := &fakeRenderer{results: map[string]fakeRenderResult{
				"drawing-shapes.docx": {
					targets:     []Target{officeTarget("viewport-1", color.NRGBA{R: 22, G: 30, B: 40, A: 255})},
					diagnostics: test.diagnostics,
				},
			}}
			report := Run(context.Background(), RunConfig{InputPath: root, OutputDir: output, Manifest: &manifest}, renderer)
			result := report.Files[0]
			if result.ExpectationMet != test.wantMet {
				t.Fatalf("expectation met = %v, want %v; result=%+v", result.ExpectationMet, test.wantMet, result)
			}
			wantUnexpected := 0
			if !test.wantMet {
				wantUnexpected = 1
			}
			if got := report.Counts.Unexpected; got != wantUnexpected {
				t.Fatalf("unexpected count = %d, want %d; result=%+v", got, wantUnexpected, result)
			}
		})
	}
}

func TestRunMatchesManifestForASubdirectoryAndKeepsSourceReadOnly(t *testing.T) {
	root, output := t.TempDir(), filepath.Join(t.TempDir(), "run")
	path := writeInput(t, root, "xlsx/expected-failure.xlsx", []byte("unchanged source"))
	writeManifestFile(t, root, []map[string]any{
		caseWithExpected(manifestCase("known-render-limit", "xlsx/expected-failure.xlsx", "xlsx"), "render_failure"),
	})
	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{
		"expected-failure.xlsx": {renderErr: errors.New("known renderer gap"), stream: true, targets: []Target{officeTarget("worksheet-1", color.NRGBA{R: 1, G: 2, B: 3, A: 255})}},
	}}
	report := Run(context.Background(), RunConfig{InputPath: filepath.Join(root, "xlsx"), OutputDir: output, Manifest: &manifest}, renderer)
	result := report.Files[0]
	if result.CaseID != "known-render-limit" || result.Path != "expected-failure.xlsx" || len(result.Features) != 1 || result.Features[0] != "basic" {
		t.Fatalf("manifest metadata was not applied: %+v", result)
	}
	if result.Status != StatusRenderFailed || !result.ExpectationMet || result.TargetCount != 1 {
		t.Fatalf("expected render failure not captured as partial expected result: %+v", result)
	}
	if report.Counts.Unexpected != 0 || report.FailedCount != 1 {
		t.Fatalf("failure expectation counts = %+v, failed=%d", report.Counts, report.FailedCount)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "unchanged source" {
		t.Fatalf("source document changed: %q, error=%v", contents, err)
	}
	if _, err := os.Stat(filepath.Join(output, result.Targets[0].Path)); err != nil {
		t.Fatalf("partial streamed screenshot was not saved: %v", err)
	}
}
