package officeqa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

var ErrRendererUnavailable = errors.New("headless renderer unavailable")

type Viewport struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Scale  float32 `json:"scale"`
}

type Target struct {
	Name  string
	Image image.Image
}

type Document interface {
	Render(context.Context, Viewport) ([]Target, error)
	Close() error
}

type Renderer interface {
	Open(context.Context, string) (Document, error)
}

// TargetStreamer lets renderers yield large multi-page documents one image at
// a time. Documents that do not implement it are collected through Render.
type TargetStreamer interface {
	RenderEach(context.Context, Viewport, func(Target) error) error
}

type DiagnosticDocument interface {
	Diagnostics() []string
}

type RunConfig struct {
	InputPath     string
	OutputDir     string
	Viewport      Viewport
	Manifest      *Manifest
	MaxInputBytes int64
	BuildVersion  string
}

type FileStatus string

const (
	StatusRendered                 FileStatus = "rendered"
	StatusCompletedWithDiagnostics FileStatus = "completed_with_diagnostics"
	StatusOpenFailed               FileStatus = "open_failed"
	StatusRenderFailed             FileStatus = "render_failed"
	StatusNotRun                   FileStatus = "not_run"
)

type DiscoveredFile struct {
	Path         string
	RelativePath string
	Format       string
	SizeBytes    int64
}

type TargetResult struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type DocumentResult struct {
	CaseID                 string           `json:"case_id"`
	Path                   string           `json:"path"`
	Format                 string           `json:"format"`
	FileSizeBytes          int64            `json:"file_size_bytes"`
	Features               []string         `json:"features,omitempty"`
	Expected               string           `json:"expected"`
	ExpectedDiagnostics    []string         `json:"expected_diagnostics,omitempty"`
	ExpectationMet         bool             `json:"expectation_met"`
	Status                 FileStatus       `json:"status"`
	Error                  string           `json:"error,omitempty"`
	Diagnostics            []string         `json:"diagnostics,omitempty"`
	Targets                []TargetResult   `json:"targets,omitempty"`
	Baselines              []BaselineResult `json:"baselines,omitempty"`
	TargetCount            int              `json:"target_count"`
	OpenDurationMS         int64            `json:"open_duration_ms"`
	FirstTargetMS          *int64           `json:"time_to_first_target_ms,omitempty"`
	RenderDurationMS       int64            `json:"render_duration_ms"`
	TotalDurationMS        int64            `json:"total_duration_ms"`
	GoTotalAllocDeltaBytes uint64           `json:"go_total_alloc_delta_bytes"`
	GoHeapAllocBeforeBytes uint64           `json:"go_heap_alloc_before_bytes"`
	GoHeapAllocAfterBytes  uint64           `json:"go_heap_alloc_after_bytes"`
}

type ResultCounts struct {
	Total           int `json:"total"`
	Rendered        int `json:"rendered"`
	WithDiagnostics int `json:"completed_with_diagnostics"`
	OpenFailed      int `json:"open_failed"`
	RenderFailed    int `json:"render_failed"`
	NotRun          int `json:"not_run"`
	BaselineFailed  int `json:"baseline_failed"`
	Unexpected      int `json:"unexpected"`
}

type ProcessPeakRSS struct {
	Bytes     uint64 `json:"process_peak_rss_bytes"`
	Available bool   `json:"process_peak_rss_available"`
}

type RunReport struct {
	SchemaVersion       int              `json:"schema_version"`
	StartedAt           time.Time        `json:"started_at"`
	FinishedAt          time.Time        `json:"finished_at"`
	DurationMS          int64            `json:"duration_ms"`
	GlanceVersion       string           `json:"glance_version"`
	Revision            string           `json:"revision,omitempty"`
	Dirty               bool             `json:"dirty,omitempty"`
	GoVersion           string           `json:"go_version"`
	GOOS                string           `json:"goos"`
	GOARCH              string           `json:"goarch"`
	InputRoot           string           `json:"input_root"`
	OutputDir           string           `json:"output_dir"`
	Viewport            Viewport         `json:"viewport"`
	MaxInputBytes       int64            `json:"max_input_bytes"`
	CorpusFingerprint   string           `json:"corpus_fingerprint,omitempty"`
	Counts              ResultCounts     `json:"counts"`
	FailedCount         int              `json:"failed_count"`
	BaselineFailedCount int              `json:"baseline_failed_count"`
	ProcessPeakRSS      ProcessPeakRSS   `json:"process_peak_rss"`
	RunError            string           `json:"run_error,omitempty"`
	Files               []DocumentResult `json:"files"`
}

func completeFileMetrics(result *DocumentResult, started time.Time, before runtime.MemStats, finished time.Time) DocumentResult {
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	result.TotalDurationMS = finished.Sub(started).Milliseconds()
	result.GoHeapAllocBeforeBytes = before.HeapAlloc
	result.GoHeapAllocAfterBytes = after.HeapAlloc
	if after.TotalAlloc >= before.TotalAlloc {
		result.GoTotalAllocDeltaBytes = after.TotalAlloc - before.TotalAlloc
	}
	result.ExpectationMet = expectationMatches(result.Expected, result.Status) && len(missingExpectedDiagnostics(result.ExpectedDiagnostics, result.Diagnostics)) == 0
	return *result
}

func missingExpectedDiagnostics(expected, actual []string) []string {
	if len(expected) == 0 {
		return nil
	}
	present := make(map[string]struct{}, len(actual))
	for _, diagnostic := range actual {
		present[diagnostic] = struct{}{}
	}
	missing := make([]string, 0, len(expected))
	for _, diagnostic := range expected {
		if _, ok := present[diagnostic]; !ok {
			missing = append(missing, diagnostic)
		}
	}
	return missing
}

func expectationMatches(expected string, status FileStatus) bool {
	switch expected {
	case "rendered":
		return status == StatusRendered || status == StatusCompletedWithDiagnostics
	case "completed_with_diagnostics":
		return status == StatusCompletedWithDiagnostics
	case "open_failure":
		return status == StatusOpenFailed
	case "render_failure":
		return status == StatusRenderFailed
	default:
		return false
	}
}

func finishRunReport(report *RunReport) {
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	counts := ResultCounts{Total: len(report.Files)}
	failed := 0
	for _, result := range report.Files {
		switch result.Status {
		case StatusRendered:
			counts.Rendered++
		case StatusCompletedWithDiagnostics:
			counts.WithDiagnostics++
		case StatusOpenFailed:
			counts.OpenFailed++
			failed++
		case StatusRenderFailed:
			counts.RenderFailed++
			failed++
		case StatusNotRun:
			counts.NotRun++
		}
		if !result.ExpectationMet && result.Status != StatusNotRun {
			counts.Unexpected++
		}
		for _, baseline := range result.Baselines {
			if baseline.IsFailure() {
				counts.BaselineFailed++
				counts.Unexpected++
			}
		}
	}
	report.Counts = counts
	report.FailedCount = failed
	report.BaselineFailedCount = counts.BaselineFailed
	if bytes, available, err := processPeakRSS(); err == nil {
		report.ProcessPeakRSS = ProcessPeakRSS{Bytes: bytes, Available: available}
	} else {
		report.ProcessPeakRSS = ProcessPeakRSS{Available: false}
	}
}

func writeReport(outputDir string, report RunReport) error {
	if outputDir == "" {
		return errors.New("report output directory is empty")
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run report: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(outputDir, ".report-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary report: %w", err)
	}
	temporaryPath := temporary.Name()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("write temporary report: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("close temporary report: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(outputDir, "report.json")); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("publish run report: %w", err)
	}
	return nil
}

func buildVersion(override string) (version, revision string, dirty bool) {
	version = strings.TrimSpace(override)
	if version == "" {
		version = "devel"
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if version == "devel" && info.Main.Version != "" {
			version = info.Main.Version
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				dirty = setting.Value == "true"
			}
		}
	}
	return version, revision, dirty
}
