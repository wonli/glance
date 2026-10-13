package officeqa

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func comparableReport(version string) RunReport {
	return RunReport{
		SchemaVersion:     1,
		GlanceVersion:     version,
		GoVersion:         "go1.test",
		GOOS:              "testos",
		GOARCH:            "testarch",
		CorpusFingerprint: "same-corpus",
		Viewport:          Viewport{Width: 1280, Height: 800, Scale: 1},
		Files: []DocumentResult{
			{CaseID: "one", Path: "docx/one.docx", OpenDurationMS: 10, FirstTargetMS: int64Pointer(20), RenderDurationMS: 15, TotalDurationMS: 30, GoTotalAllocDeltaBytes: 100},
			{CaseID: "two", Path: "xlsx/two.xlsx", OpenDurationMS: 30, FirstTargetMS: int64Pointer(50), RenderDurationMS: 40, TotalDurationMS: 70, GoTotalAllocDeltaBytes: 300},
		},
	}
}

func int64Pointer(value int64) *int64 { return &value }

func TestCompareReportsReportsPerFileAndAggregateDeltas(t *testing.T) {
	older := comparableReport("v1")
	newer := comparableReport("v2")
	newer.Files[0].OpenDurationMS = 12
	newer.Files[0].FirstTargetMS = int64Pointer(24)
	newer.Files[0].RenderDurationMS = 18
	newer.Files[0].TotalDurationMS = 35
	newer.Files[0].GoTotalAllocDeltaBytes = 150
	newer.Files[1].OpenDurationMS = 25
	newer.Files[1].FirstTargetMS = int64Pointer(43)
	newer.Files[1].RenderDurationMS = 35
	newer.Files[1].TotalDurationMS = 60
	newer.Files[1].GoTotalAllocDeltaBytes = 250
	newer.ProcessPeakRSS = ProcessPeakRSS{Bytes: 900, Available: true}
	older.ProcessPeakRSS = ProcessPeakRSS{Bytes: 1000, Available: true}

	comparison, err := CompareReports(older, newer)
	if err != nil {
		t.Fatal(err)
	}
	if len(comparison.Files) != 2 || comparison.Files[0].OpenDurationDeltaMS != 2 || comparison.Files[1].TotalDurationDeltaMS != -10 {
		t.Fatalf("per-file deltas = %+v", comparison.Files)
	}
	if comparison.Aggregate.OpenDurationDeltaMS != -3 || comparison.Aggregate.TotalDurationDeltaMS != -5 || comparison.Aggregate.GoTotalAllocDeltaBytes != 0 {
		t.Fatalf("aggregate deltas = %+v", comparison.Aggregate)
	}
	if comparison.ProcessPeakRSSDeltaBytes == nil || *comparison.ProcessPeakRSSDeltaBytes != -100 {
		t.Fatalf("RSS delta = %v", comparison.ProcessPeakRSSDeltaBytes)
	}
}

func TestCompareReportsRejectsCorpusOrEnvironmentMismatch(t *testing.T) {
	tests := map[string]func(*RunReport){
		"corpus":       func(report *RunReport) { report.CorpusFingerprint = "different" },
		"Go version":   func(report *RunReport) { report.GoVersion = "different" },
		"OS":           func(report *RunReport) { report.GOOS = "different" },
		"architecture": func(report *RunReport) { report.GOARCH = "different" },
		"viewport":     func(report *RunReport) { report.Viewport.Width++ },
		"scale":        func(report *RunReport) { report.Viewport.Scale = 1.25 },
	}
	for name, alter := range tests {
		t.Run(name, func(t *testing.T) {
			older, newer := comparableReport("v1"), comparableReport("v2")
			alter(&newer)
			if _, err := CompareReports(older, newer); err == nil {
				t.Fatal("incompatible reports were compared")
			}
		})
	}
}

func TestCompareBaselineDetectsChangedPixels(t *testing.T) {
	actual := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	baseline := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	actual.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	baseline.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	actual.SetNRGBA(1, 0, color.NRGBA{G: 100, A: 255})
	baseline.SetNRGBA(1, 0, color.NRGBA{G: 101, A: 255})
	comparison := CompareBaselineImages(actual, baseline)
	if comparison.Status != BaselineChanged || comparison.ChangedPixels != 1 || comparison.TotalPixels != 2 || comparison.ChangedRatio != 0.5 {
		t.Fatalf("baseline comparison = %+v", comparison)
	}
}

func TestMissingBaselineIsNotVisualPass(t *testing.T) {
	root := t.TempDir()
	writeInput(t, root, "one.xlsx", []byte("xlsx"))
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{
		"one.xlsx": {targets: []Target{officeTarget("worksheet-1", color.NRGBA{B: 30, A: 255})}},
	}}
	report := Run(context.Background(), RunConfig{InputPath: root, OutputDir: filepath.Join(t.TempDir(), "run")}, renderer)
	if len(report.Files[0].Baselines) != 1 || report.Files[0].Baselines[0].Status != BaselineNotConfigured {
		t.Fatalf("unbaselined screenshot was treated as a visual pass: %+v", report.Files[0].Baselines)
	}
}

func TestRunComparesConfiguredBaseline(t *testing.T) {
	for _, mode := range []string{"pass", "changed", "missing"} {
		t.Run(mode, func(t *testing.T) {
			root, output := t.TempDir(), filepath.Join(t.TempDir(), "run")
			writeInput(t, root, "one.xlsx", []byte("xlsx"))
			baselineRelative := "baselines/worksheet.png"
			if mode != "missing" {
				baselineImage := officeTarget("baseline", color.NRGBA{R: 20, G: 40, B: 60, A: 255}).Image
				if mode == "changed" {
					baselineImage = officeTarget("baseline", color.NRGBA{R: 80, G: 40, B: 60, A: 255}).Image
				}
				writePNGImage(t, filepath.Join(root, filepath.FromSlash(baselineRelative)), baselineImage)
			}
			entry := manifestCase("baseline-case", "one.xlsx", "xlsx")
			entry = setManifestField(t, entry, "baselines", map[string]string{"worksheet-1": baselineRelative})
			writeManifestFile(t, root, entry)
			manifest, err := LoadManifest(root)
			if err != nil {
				t.Fatal(err)
			}
			renderer := &fakeRenderer{results: map[string]fakeRenderResult{
				"one.xlsx": {targets: []Target{officeTarget("worksheet-1", color.NRGBA{R: 20, G: 40, B: 60, A: 255})}},
			}}
			report := Run(context.Background(), RunConfig{InputPath: root, OutputDir: output, Manifest: &manifest}, renderer)
			if len(report.Files[0].Baselines) != 1 {
				t.Fatalf("baseline results = %+v", report.Files[0].Baselines)
			}
			want := map[string]BaselineStatus{"pass": BaselinePassed, "changed": BaselineChanged, "missing": BaselineMissing}[mode]
			if report.Files[0].Baselines[0].Status != want {
				t.Fatalf("baseline status = %q, want %q: %+v", report.Files[0].Baselines[0].Status, want, report.Files[0].Baselines[0])
			}
			if (mode == "pass") != (report.BaselineFailedCount == 0) {
				t.Fatalf("baseline failure count = %d", report.BaselineFailedCount)
			}
		})
	}
}

func writePNGImage(t *testing.T, path string, img image.Image) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBaselineDimensionMismatchFails(t *testing.T) {
	actual := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	baseline := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	comparison := CompareBaselineImages(actual, baseline)
	if comparison.Status != BaselineSizeMismatch {
		t.Fatalf("dimension mismatch status = %q", comparison.Status)
	}
}

func TestConfiguredBaselineRejectsOversizedPNGDimensions(t *testing.T) {
	root := t.TempDir()
	actualPath := filepath.Join(root, "actual.png")
	baselinePath := filepath.Join(root, "baseline.png")
	writePNGImage(t, actualPath, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	writePNGImage(t, baselinePath, image.NewNRGBA(image.Rect(0, 0, maxRenderDimension+1, 1)))
	result := compareBaselineFiles(actualPath, baselinePath)
	if result.Status != BaselineError || !strings.Contains(result.Error, "dimensions") {
		t.Fatalf("oversized baseline status = %+v, want a dimensions error", result)
	}
}
