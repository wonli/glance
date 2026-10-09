package officeqa

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"sort"
)

type FileMetricDelta struct {
	CaseID                   string `json:"case_id"`
	Path                     string `json:"path"`
	OpenDurationDeltaMS      int64  `json:"open_duration_delta_ms"`
	TimeToFirstTargetDeltaMS *int64 `json:"time_to_first_target_delta_ms,omitempty"`
	RenderDurationDeltaMS    int64  `json:"render_duration_delta_ms"`
	TotalDurationDeltaMS     int64  `json:"total_duration_delta_ms"`
	GoTotalAllocDeltaBytes   int64  `json:"go_total_alloc_delta_bytes"`
}

type AggregateMetricDelta struct {
	OpenDurationDeltaMS    int64  `json:"open_duration_delta_ms"`
	FirstTargetDeltaMS     *int64 `json:"time_to_first_target_delta_ms,omitempty"`
	RenderDurationDeltaMS  int64  `json:"render_duration_delta_ms"`
	TotalDurationDeltaMS   int64  `json:"total_duration_delta_ms"`
	GoTotalAllocDeltaBytes int64  `json:"go_total_alloc_delta_bytes"`
}

type ReportComparison struct {
	OlderVersion             string               `json:"older_version"`
	NewerVersion             string               `json:"newer_version"`
	CorpusFingerprint        string               `json:"corpus_fingerprint"`
	Files                    []FileMetricDelta    `json:"files"`
	Aggregate                AggregateMetricDelta `json:"aggregate"`
	ProcessPeakRSSDeltaBytes *int64               `json:"process_peak_rss_delta_bytes,omitempty"`
}

type BaselineStatus string

const (
	BaselinePassed        BaselineStatus = "passed"
	BaselineChanged       BaselineStatus = "changed"
	BaselineSizeMismatch  BaselineStatus = "size_mismatch"
	BaselineMissing       BaselineStatus = "missing_baseline"
	BaselineTargetMissing BaselineStatus = "target_missing"
	BaselineError         BaselineStatus = "error"
	BaselineNotConfigured BaselineStatus = "not_configured"
)

type BaselineResult struct {
	Target        string         `json:"target"`
	Path          string         `json:"path,omitempty"`
	Status        BaselineStatus `json:"status"`
	ChangedPixels int64          `json:"changed_pixels"`
	TotalPixels   int64          `json:"total_pixels"`
	ChangedRatio  float64        `json:"changed_ratio"`
	Error         string         `json:"error,omitempty"`
}

type PixelComparison struct {
	Status        BaselineStatus `json:"status"`
	ChangedPixels int64          `json:"changed_pixels"`
	TotalPixels   int64          `json:"total_pixels"`
	ChangedRatio  float64        `json:"changed_ratio"`
}

func (result BaselineResult) IsFailure() bool {
	switch result.Status {
	case BaselineChanged, BaselineSizeMismatch, BaselineMissing, BaselineTargetMissing, BaselineError:
		return true
	default:
		return false
	}
}

func CompareReports(older, newer RunReport) (ReportComparison, error) {
	if older.CorpusFingerprint == "" || newer.CorpusFingerprint == "" {
		return ReportComparison{}, errors.New("reports do not contain a corpus fingerprint")
	}
	if older.CorpusFingerprint != newer.CorpusFingerprint {
		return ReportComparison{}, errors.New("reports use different corpus fingerprints")
	}
	if older.GoVersion != newer.GoVersion || older.GOOS != newer.GOOS || older.GOARCH != newer.GOARCH {
		return ReportComparison{}, errors.New("reports use different Go or operating-system environments")
	}
	if older.Viewport != newer.Viewport {
		return ReportComparison{}, errors.New("reports use different viewport dimensions or scale")
	}
	olderByID := make(map[string]DocumentResult, len(older.Files))
	for _, file := range older.Files {
		if file.CaseID == "" {
			return ReportComparison{}, errors.New("older report contains a file without a case ID")
		}
		if _, duplicate := olderByID[file.CaseID]; duplicate {
			return ReportComparison{}, fmt.Errorf("older report contains duplicate case ID %q", file.CaseID)
		}
		if file.Status == StatusNotRun {
			return ReportComparison{}, fmt.Errorf("older report did not run case %q", file.CaseID)
		}
		olderByID[file.CaseID] = file
	}
	newerByID := make(map[string]DocumentResult, len(newer.Files))
	for _, file := range newer.Files {
		if file.CaseID == "" {
			return ReportComparison{}, errors.New("newer report contains a file without a case ID")
		}
		if _, duplicate := newerByID[file.CaseID]; duplicate {
			return ReportComparison{}, fmt.Errorf("newer report contains duplicate case ID %q", file.CaseID)
		}
		if file.Status == StatusNotRun {
			return ReportComparison{}, fmt.Errorf("newer report did not run case %q", file.CaseID)
		}
		newerByID[file.CaseID] = file
	}
	if len(olderByID) != len(newerByID) {
		return ReportComparison{}, errors.New("reports contain different file sets")
	}

	ids := make([]string, 0, len(olderByID))
	for id := range olderByID {
		if _, ok := newerByID[id]; !ok {
			return ReportComparison{}, fmt.Errorf("newer report is missing case %q", id)
		}
		if olderByID[id].Path != newerByID[id].Path {
			return ReportComparison{}, fmt.Errorf("case %q has different paths in reports", id)
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return olderByID[ids[i]].Path < olderByID[ids[j]].Path })
	comparison := ReportComparison{
		OlderVersion:      older.GlanceVersion,
		NewerVersion:      newer.GlanceVersion,
		CorpusFingerprint: older.CorpusFingerprint,
		Files:             make([]FileMetricDelta, 0, len(ids)),
	}
	var olderFirst, newerFirst int64
	allFirstTargets := true
	for _, id := range ids {
		oldFile, newFile := olderByID[id], newerByID[id]
		firstDelta := pointerDelta(oldFile.FirstTargetMS, newFile.FirstTargetMS)
		if oldFile.FirstTargetMS == nil || newFile.FirstTargetMS == nil {
			allFirstTargets = false
		} else {
			olderFirst += *oldFile.FirstTargetMS
			newerFirst += *newFile.FirstTargetMS
		}
		comparison.Files = append(comparison.Files, FileMetricDelta{
			CaseID:                   id,
			Path:                     oldFile.Path,
			OpenDurationDeltaMS:      newFile.OpenDurationMS - oldFile.OpenDurationMS,
			TimeToFirstTargetDeltaMS: firstDelta,
			RenderDurationDeltaMS:    newFile.RenderDurationMS - oldFile.RenderDurationMS,
			TotalDurationDeltaMS:     newFile.TotalDurationMS - oldFile.TotalDurationMS,
			GoTotalAllocDeltaBytes:   signedUintDelta(oldFile.GoTotalAllocDeltaBytes, newFile.GoTotalAllocDeltaBytes),
		})
		comparison.Aggregate.OpenDurationDeltaMS += newFile.OpenDurationMS - oldFile.OpenDurationMS
		comparison.Aggregate.RenderDurationDeltaMS += newFile.RenderDurationMS - oldFile.RenderDurationMS
		comparison.Aggregate.TotalDurationDeltaMS += newFile.TotalDurationMS - oldFile.TotalDurationMS
		comparison.Aggregate.GoTotalAllocDeltaBytes += signedUintDelta(oldFile.GoTotalAllocDeltaBytes, newFile.GoTotalAllocDeltaBytes)
	}
	if allFirstTargets {
		delta := newerFirst - olderFirst
		comparison.Aggregate.FirstTargetDeltaMS = &delta
	}
	if older.ProcessPeakRSS.Available && newer.ProcessPeakRSS.Available {
		delta := signedUintDelta(older.ProcessPeakRSS.Bytes, newer.ProcessPeakRSS.Bytes)
		comparison.ProcessPeakRSSDeltaBytes = &delta
	}
	return comparison, nil
}

func CompareBaselineImages(actual, baseline image.Image) PixelComparison {
	if actual == nil || baseline == nil {
		return PixelComparison{Status: BaselineError}
	}
	actualBounds, baselineBounds := actual.Bounds(), baseline.Bounds()
	width, height := actualBounds.Dx(), actualBounds.Dy()
	if width < 1 || height < 1 || width != baselineBounds.Dx() || height != baselineBounds.Dy() {
		return PixelComparison{Status: BaselineSizeMismatch}
	}
	total := int64(width) * int64(height)
	changed := int64(0)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			actualPixel := color.NRGBAModel.Convert(actual.At(actualBounds.Min.X+x, actualBounds.Min.Y+y)).(color.NRGBA)
			baselinePixel := color.NRGBAModel.Convert(baseline.At(baselineBounds.Min.X+x, baselineBounds.Min.Y+y)).(color.NRGBA)
			if actualPixel != baselinePixel {
				changed++
			}
		}
	}
	status := BaselinePassed
	if changed != 0 {
		status = BaselineChanged
	}
	return PixelComparison{Status: status, ChangedPixels: changed, TotalPixels: total, ChangedRatio: float64(changed) / float64(total)}
}

func compareBaselineFiles(actualPath, baselinePath string) BaselineResult {
	result := BaselineResult{Status: BaselineError, Path: baselinePath}
	actualFile, err := os.Open(actualPath)
	if err != nil {
		result.Error = fmt.Sprintf("open rendered PNG: %v", err)
		return result
	}
	actual, decodeErr := decodeBoundedPNG(actualFile)
	closeErr := actualFile.Close()
	if decodeErr != nil || closeErr != nil {
		result.Error = errors.Join(decodeErr, closeErr).Error()
		return result
	}
	baselineFile, err := os.Open(baselinePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.Status = BaselineMissing
			result.Error = "configured baseline file does not exist"
		} else {
			result.Error = fmt.Sprintf("open configured baseline: %v", err)
		}
		return result
	}
	baseline, decodeErr := decodeBoundedPNG(baselineFile)
	closeErr = baselineFile.Close()
	if decodeErr != nil || closeErr != nil {
		result.Error = errors.Join(decodeErr, closeErr).Error()
		return result
	}
	comparison := CompareBaselineImages(actual, baseline)
	result.Status = comparison.Status
	result.ChangedPixels = comparison.ChangedPixels
	result.TotalPixels = comparison.TotalPixels
	result.ChangedRatio = comparison.ChangedRatio
	return result
}

func decodeBoundedPNG(file *os.File) (image.Image, error) {
	config, err := png.DecodeConfig(file)
	if err != nil {
		return nil, err
	}
	if config.Width < 1 || config.Height < 1 || config.Width > maxRenderDimension || config.Height > maxRenderDimension || int64(config.Width)*int64(config.Height) > maxRenderPixels {
		return nil, fmt.Errorf("PNG dimensions %dx%d exceed supported limits", config.Width, config.Height)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind PNG: %w", err)
	}
	return png.Decode(file)
}

func pointerDelta(older, newer *int64) *int64 {
	if older == nil || newer == nil {
		return nil
	}
	delta := *newer - *older
	return &delta
}

func signedUintDelta(older, newer uint64) int64 {
	if newer >= older {
		delta := newer - older
		if delta > math.MaxInt64 {
			return math.MaxInt64
		}
		return int64(delta)
	}
	delta := older - newer
	if delta > math.MaxInt64 {
		return math.MinInt64
	}
	return -int64(delta)
}
