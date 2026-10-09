package officeqa

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const maxReportBytes = 16 << 20

func DispatchCLI(args []string, renderer Renderer, stdout, stderr io.Writer) (handled bool, exitCode int) {
	if len(args) == 0 || args[0] != "qa" {
		return false, 0
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) < 2 {
		writeQAUsage(stderr)
		return true, 2
	}
	switch args[1] {
	case "render":
		return true, dispatchRender(args[2:], renderer, stdout, stderr)
	case "compare":
		return true, dispatchCompare(args[2:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown qa command %q\n", args[1])
		writeQAUsage(stderr)
		return true, 2
	}
}

func dispatchRender(args []string, renderer Renderer, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("qa render", flag.ContinueOnError)
	flags.SetOutput(stderr)
	input := flags.String("input", "", "Office document file or directory")
	output := flags.String("out", "", "directory for screenshots and report.json")
	width := flags.Int("width", defaultViewportWidth, "render viewport width in pixels")
	height := flags.Int("height", defaultViewportHeight, "render viewport height in pixels")
	scale := flags.Float64("scale", defaultViewportScale, "render scale")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: glance qa render --input PATH --out DIR [--width 1280 --height 800 --scale 1]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || strings.TrimSpace(*input) == "" || strings.TrimSpace(*output) == "" {
		fmt.Fprintln(stderr, "qa render requires --input PATH and --out DIR and accepts no positional arguments")
		flags.Usage()
		return 2
	}
	if *width < 1 || *height < 1 || *width > maxRenderDimension || *height > maxRenderDimension || int64(*width)*int64(*height) > maxRenderPixels || math.IsNaN(*scale) || math.IsInf(*scale, 0) || *scale < 0.25 || *scale > 4 {
		fmt.Fprintln(stderr, "viewport must be within 1..4096 pixels, 16 MP, and scale 0.25..4")
		return 2
	}
	manifest, err := manifestForInput(*input)
	if err != nil {
		fmt.Fprintf(stderr, "load Office corpus manifest: %v\n", err)
		return 1
	}
	report := Run(nil, RunConfig{
		InputPath: *input,
		OutputDir: *output,
		Viewport:  Viewport{Width: *width, Height: *height, Scale: float32(*scale)},
		Manifest:  manifest,
	}, renderer)
	fmt.Fprintf(stdout,
		"Render run: %d files; %d rendered; %d with diagnostics; %d open failures; %d render failures; %d baseline failures; %d unexpected\n",
		report.Counts.Total,
		report.Counts.Rendered,
		report.Counts.WithDiagnostics,
		report.Counts.OpenFailed,
		report.Counts.RenderFailed,
		report.Counts.BaselineFailed,
		report.Counts.Unexpected,
	)
	for _, file := range report.Files {
		fmt.Fprintf(stdout, "%s  %s  (%d targets)\n", file.Status, file.Path, file.TargetCount)
		if len(file.Diagnostics) > 0 {
			fmt.Fprintf(stdout, "  diagnostics: %s\n", strings.Join(file.Diagnostics, ", "))
		}
		if missing := missingExpectedDiagnostics(file.ExpectedDiagnostics, file.Diagnostics); len(missing) > 0 {
			fmt.Fprintf(stdout, "  missing expected diagnostics: %s\n", strings.Join(missing, ", "))
		}
	}
	if report.OutputDir != "" {
		fmt.Fprintf(stdout, "Report: %s\n", filepath.Join(report.OutputDir, "report.json"))
	}
	if report.RunError != "" {
		fmt.Fprintf(stderr, "Render run error: %s\n", report.RunError)
		return 1
	}
	if report.FailedCount > 0 || report.BaselineFailedCount > 0 || report.Counts.Unexpected > 0 {
		return 1
	}
	return 0
}

func dispatchCompare(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "Usage: glance qa compare OLDER.json NEWER.json")
		return 2
	}
	older, err := readRunReport(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "read older report: %v\n", err)
		return 2
	}
	newer, err := readRunReport(args[1])
	if err != nil {
		fmt.Fprintf(stderr, "read newer report: %v\n", err)
		return 2
	}
	comparison, err := CompareReports(older, newer)
	if err != nil {
		fmt.Fprintf(stderr, "reports are not comparable: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "Performance comparison: %s -> %s\n", comparison.OlderVersion, comparison.NewerVersion)
	fmt.Fprintf(stdout, "Corpus: %s\n", comparison.CorpusFingerprint)
	for _, file := range comparison.Files {
		firstTarget := "n/a"
		if file.TimeToFirstTargetDeltaMS != nil {
			firstTarget = formatMilliseconds(*file.TimeToFirstTargetDeltaMS)
		}
		fmt.Fprintf(stdout,
			"%s  open %s  first target %s  render %s  total %s  Go allocation %s\n",
			file.Path,
			formatMilliseconds(file.OpenDurationDeltaMS),
			firstTarget,
			formatMilliseconds(file.RenderDurationDeltaMS),
			formatMilliseconds(file.TotalDurationDeltaMS),
			formatBytes(file.GoTotalAllocDeltaBytes),
		)
	}
	fmt.Fprintf(stdout,
		"Aggregate  open %s  first target %s  render %s  total %s  Go allocation %s\n",
		formatMilliseconds(comparison.Aggregate.OpenDurationDeltaMS),
		formatOptionalMilliseconds(comparison.Aggregate.FirstTargetDeltaMS),
		formatMilliseconds(comparison.Aggregate.RenderDurationDeltaMS),
		formatMilliseconds(comparison.Aggregate.TotalDurationDeltaMS),
		formatBytes(comparison.Aggregate.GoTotalAllocDeltaBytes),
	)
	if comparison.ProcessPeakRSSDeltaBytes != nil {
		fmt.Fprintf(stdout, "Process peak RSS delta: %s\n", formatBytes(*comparison.ProcessPeakRSSDeltaBytes))
	} else {
		fmt.Fprintln(stdout, "Process peak RSS delta: unavailable")
	}
	return 0
}

func readRunReport(path string) (RunReport, error) {
	var report RunReport
	info, err := os.Stat(path)
	if err != nil {
		return report, err
	}
	if !info.Mode().IsRegular() {
		return report, errors.New("report path is not a regular file")
	}
	if info.Size() > maxReportBytes {
		return report, fmt.Errorf("report exceeds the %d-byte limit", maxReportBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return report, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&report); err != nil {
		return report, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return report, errors.New("report contains trailing JSON data")
	} else if !errors.Is(err, io.EOF) {
		return report, fmt.Errorf("decode trailing report data: %w", err)
	}
	if report.SchemaVersion != 1 {
		return report, fmt.Errorf("unsupported report schema version %d", report.SchemaVersion)
	}
	return report, nil
}

func manifestForInput(input string) (*Manifest, error) {
	abs, err := filepath.Abs(input)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	root := abs
	if err == nil && !info.IsDir() || errors.Is(err, os.ErrNotExist) {
		root = filepath.Dir(abs)
	} else if err != nil {
		return nil, err
	}
	candidates := make([]string, 0, 18)
	for depth := 0; depth < 16; depth++ {
		candidates = append(candidates, root)
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate, err = filepath.Abs(candidate)
		if err != nil || seen[candidate] {
			continue
		}
		seen[candidate] = true
		manifestPath := filepath.Join(candidate, "manifest.json")
		if _, err := os.Stat(manifestPath); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("inspect %q: %w", manifestPath, err)
		}
		manifest, err := LoadManifest(candidate)
		if err != nil {
			return nil, err
		}
		return &manifest, nil
	}
	return nil, nil
}

func formatMilliseconds(delta int64) string { return fmt.Sprintf("%+d ms", delta) }

func formatOptionalMilliseconds(delta *int64) string {
	if delta == nil {
		return "n/a"
	}
	return formatMilliseconds(*delta)
}

func formatBytes(delta int64) string { return fmt.Sprintf("%+d B", delta) }

func writeQAUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: glance qa render --input PATH --out DIR [options]")
	fmt.Fprintln(out, "       glance qa compare OLDER.json NEWER.json")
}
