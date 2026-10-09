package officeqa

import (
	"bytes"
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDispatchQARenderParsesInputAndOutput(t *testing.T) {
	inputRoot, output := t.TempDir(), filepath.Join(t.TempDir(), "render-output")
	writeInput(t, inputRoot, "one.xlsx", []byte("xlsx"))
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{
		"one.xlsx": {targets: []Target{officeTarget("worksheet-1", color.NRGBA{R: 45, G: 60, B: 75, A: 255})}},
	}}
	var stdout, stderr bytes.Buffer
	handled, code := DispatchCLI([]string{"qa", "render", "--input", inputRoot, "--out", output, "--width", "64", "--height", "48", "--scale", "1.5"}, renderer, &stdout, &stderr)
	if !handled || code != 0 {
		t.Fatalf("dispatch = handled %v, exit %d; stderr=%s", handled, code, stderr.String())
	}
	var report RunReport
	data, err := os.ReadFile(filepath.Join(output, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Viewport != (Viewport{Width: 64, Height: 48, Scale: 1.5}) || report.Counts.Rendered != 1 {
		t.Fatalf("render options/results = %+v / %+v", report.Viewport, report.Counts)
	}
	if !strings.Contains(stdout.String(), "report.json") {
		t.Fatalf("render summary omitted report path: %s", stdout.String())
	}
}

func TestDispatchRenderPrintsPerFileDiagnostics(t *testing.T) {
	input, output := t.TempDir(), filepath.Join(t.TempDir(), "render-output")
	writeInput(t, input, "drawing-shapes.docx", []byte("docx"))
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{
		"drawing-shapes.docx": {
			targets:     []Target{officeTarget("viewport-1", color.NRGBA{R: 22, G: 30, B: 40, A: 255})},
			diagnostics: []string{"error:nested-table", "warning:unsupported-image"},
		},
	}}
	var stdout, stderr bytes.Buffer
	handled, code := DispatchCLI([]string{"qa", "render", "--input", input, "--out", output}, renderer, &stdout, &stderr)
	if !handled || code != 0 {
		t.Fatalf("dispatch = handled %v, exit %d; stderr=%s", handled, code, stderr.String())
	}
	outputText := stdout.String()
	for _, want := range []string{"drawing-shapes.docx", "error:nested-table", "warning:unsupported-image"} {
		if !strings.Contains(outputText, want) {
			t.Fatalf("render summary omitted diagnostic %q: %s", want, outputText)
		}
	}
}

func TestDispatchRenderFailsWhenManifestExpectationIsUnmet(t *testing.T) {
	input, output := t.TempDir(), filepath.Join(t.TempDir(), "render-output")
	writeInput(t, input, "one.xlsx", []byte("xlsx"))
	writeManifestFile(t, input, []map[string]any{{
		"id":       "expected-diagnostic",
		"path":     "one.xlsx",
		"format":   "xlsx",
		"features": []string{"basic"},
		"expected": "completed_with_diagnostics",
	}})
	renderer := &fakeRenderer{results: map[string]fakeRenderResult{
		"one.xlsx": {targets: []Target{officeTarget("worksheet-1", color.NRGBA{R: 45, G: 60, B: 75, A: 255})}},
	}}
	var stdout, stderr bytes.Buffer
	handled, code := DispatchCLI([]string{"qa", "render", "--input", input, "--out", output}, renderer, &stdout, &stderr)
	if !handled || code != 1 {
		t.Fatalf("unmet manifest expectation dispatch = handled %v, exit %d; stderr=%s", handled, code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 unexpected") {
		t.Fatalf("render summary omitted unmet expectation count: %s", stdout.String())
	}
}

func TestDispatchQACompareLoadsTwoReports(t *testing.T) {
	older, newer := comparableReport("v1"), comparableReport("v2")
	newer.Files[0].OpenDurationMS = 12
	olderPath, newerPath := writeComparableReport(t, older), writeComparableReport(t, newer)
	var stdout, stderr bytes.Buffer
	handled, code := DispatchCLI([]string{"qa", "compare", olderPath, newerPath}, nil, &stdout, &stderr)
	if !handled || code != 0 {
		t.Fatalf("dispatch = handled %v, exit %d; stderr=%s", handled, code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "v1 -> v2") || !strings.Contains(stdout.String(), "docx/one.docx") || !strings.Contains(stdout.String(), "+2 ms") {
		t.Fatalf("comparison output lacks version/file/delta details: %s", stdout.String())
	}
}

func TestDispatchInvalidQAUsageReturnsTwo(t *testing.T) {
	tests := [][]string{{"qa"}, {"qa", "unknown"}, {"qa", "render", "--input", "only-input"}, {"qa", "compare", "one.json"}}
	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			handled, code := DispatchCLI(args, &fakeRenderer{}, &stdout, &stderr)
			if !handled || code != 2 {
				t.Fatalf("dispatch = handled %v, exit %d; stdout=%s stderr=%s", handled, code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestDispatchDoesNotConsumeInteractiveArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	handled, code := DispatchCLI([]string{"document.docx"}, &fakeRenderer{}, &stdout, &stderr)
	if handled || code != 0 {
		t.Fatalf("normal document argument was consumed: handled=%v exit=%d", handled, code)
	}
}

func TestManifestForInputIgnoresUnrelatedWorkingDirectoryCorpus(t *testing.T) {
	workingDirectory := t.TempDir()
	inputDirectory := filepath.Join(workingDirectory, "unrelated")
	if err := os.MkdirAll(inputDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	input := writeInput(t, inputDirectory, "single.xlsx", []byte("xlsx"))
	brokenCorpus := filepath.Join(workingDirectory, "testdata", "office-corpus")
	writeManifestFile(t, brokenCorpus, []map[string]any{manifestCase("stale", "missing.docx", "docx")})
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	manifest, err := manifestForInput(input)
	if err != nil {
		t.Fatalf("unrelated corpus blocked single-file input: %v", err)
	}
	if manifest != nil {
		t.Fatal("single-file input unexpectedly loaded a manifest outside its path")
	}
}

func writeComparableReport(t *testing.T, report RunReport) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.json")
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
