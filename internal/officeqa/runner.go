package officeqa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	defaultViewportWidth  = 1280
	defaultViewportHeight = 800
	defaultViewportScale  = 1
	defaultMaxInputBytes  = 256 << 20
	maxDiscoveredFiles    = 10_000
	maxRenderDimension    = 4096
	maxRenderPixels       = 16 << 20
)

type discovery struct {
	Root  string
	Files []DiscoveredFile
}

type caseMetadata struct {
	id                  string
	features            []string
	expected            string
	expectedDiagnostics []string
	baselineRoot        string
	baselines           map[string]string
}

func discoverInput(input string) (discovery, error) {
	if strings.TrimSpace(input) == "" {
		return discovery{}, errors.New("input path is empty")
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return discovery{}, fmt.Errorf("resolve input path: %w", err)
	}
	linkInfo, err := os.Lstat(abs)
	if err != nil {
		return discovery{}, fmt.Errorf("stat input path: %w", err)
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		return discovery{}, errors.New("input path must not be a symlink")
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return discovery{}, fmt.Errorf("resolve input path: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return discovery{}, fmt.Errorf("stat input path: %w", err)
	}
	if info.IsDir() {
		found := discovery{Root: root}
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			format := formatForExtension(filepath.Ext(entry.Name()))
			if format == "" {
				return nil
			}
			fileInfo, err := entry.Info()
			if err != nil {
				return fmt.Errorf("stat discovered file %q: %w", path, err)
			}
			if !fileInfo.Mode().IsRegular() {
				return nil
			}
			if len(found.Files) >= maxDiscoveredFiles {
				return fmt.Errorf("input directory exceeds the %d document limit", maxDiscoveredFiles)
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			found.Files = append(found.Files, DiscoveredFile{
				Path:         path,
				RelativePath: filepath.ToSlash(relative),
				Format:       format,
				SizeBytes:    fileInfo.Size(),
			})
			return nil
		})
		if err != nil {
			return discovery{}, fmt.Errorf("walk input directory: %w", err)
		}
		sort.Slice(found.Files, func(i, j int) bool { return found.Files[i].RelativePath < found.Files[j].RelativePath })
		if len(found.Files) == 0 {
			return discovery{}, errors.New("input directory contains no supported Office documents (.xlsx, .docx, .pptx)")
		}
		return found, nil
	}
	if !info.Mode().IsRegular() {
		return discovery{}, errors.New("input path is not a regular Office document or directory")
	}
	format := formatForExtension(filepath.Ext(root))
	if format == "" {
		return discovery{}, fmt.Errorf("unsupported Office document extension %q (supported: .xlsx, .docx, .pptx)", filepath.Ext(root))
	}
	directory := filepath.Dir(root)
	return discovery{Root: directory, Files: []DiscoveredFile{{
		Path:         root,
		RelativePath: filepath.Base(root),
		Format:       format,
		SizeBytes:    info.Size(),
	}}}, nil
}

func formatForExtension(extension string) string {
	switch strings.ToLower(extension) {
	case ".xlsx":
		return "xlsx"
	case ".docx":
		return "docx"
	case ".pptx":
		return "pptx"
	default:
		return ""
	}
}

func corpusFingerprint(files []DiscoveredFile) (string, error) {
	return corpusFingerprintWithLimit(files, defaultMaxInputBytes)
}

func corpusFingerprintWithLimit(files []DiscoveredFile, maxInputBytes int64) (string, error) {
	ordered := append([]DiscoveredFile(nil), files...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].RelativePath < ordered[j].RelativePath })
	hash := sha256.New()
	for _, file := range ordered {
		if _, err := fmt.Fprintf(hash, "%d:%s\n", len(file.RelativePath), file.RelativePath); err != nil {
			return "", err
		}
		info, err := os.Lstat(file.Path)
		if err != nil {
			return "", fmt.Errorf("stat %q for corpus fingerprint: %w", file.RelativePath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", fmt.Errorf("input %q is not a regular file", file.RelativePath)
		}
		if info.Size() > maxInputBytes {
			// Oversized inputs are rejected before rendering; bind their identity to
			// path and size without reading unbounded document content.
			if _, err := fmt.Fprintf(hash, "oversized:%d\n", info.Size()); err != nil {
				return "", err
			}
			continue
		}
		input, err := os.Open(file.Path)
		if err != nil {
			return "", fmt.Errorf("open %q for corpus fingerprint: %w", file.RelativePath, err)
		}
		written, copyErr := io.Copy(hash, io.LimitReader(input, maxInputBytes))
		closeErr := input.Close()
		if copyErr != nil {
			return "", fmt.Errorf("read %q for corpus fingerprint: %w", file.RelativePath, copyErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close %q after fingerprint: %w", file.RelativePath, closeErr)
		}
		if _, err := fmt.Fprintf(hash, "\n%d\n", written); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func Run(ctx context.Context, config RunConfig, renderer Renderer) RunReport {
	started := time.Now().UTC()
	viewport := config.Viewport
	if viewport.Width == 0 {
		viewport.Width = defaultViewportWidth
	}
	if viewport.Height == 0 {
		viewport.Height = defaultViewportHeight
	}
	if viewport.Scale == 0 {
		viewport.Scale = defaultViewportScale
	}
	maxInputBytes := config.MaxInputBytes
	if maxInputBytes == 0 {
		maxInputBytes = defaultMaxInputBytes
	}
	version, revision, dirty := buildVersion(config.BuildVersion)
	report := RunReport{
		SchemaVersion: 1,
		StartedAt:     started,
		GlanceVersion: version,
		Revision:      revision,
		Dirty:         dirty,
		GoVersion:     runtime.Version(),
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		Viewport:      viewport,
		MaxInputBytes: maxInputBytes,
		Files:         []DocumentResult{},
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var found discovery
	if config.OutputDir == "" {
		report.RunError = "output directory is empty"
	} else if err := os.MkdirAll(config.OutputDir, 0o755); err != nil {
		report.RunError = fmt.Sprintf("create output directory: %v", err)
	} else if outputDir, err := filepath.Abs(config.OutputDir); err != nil {
		report.RunError = fmt.Sprintf("resolve output directory: %v", err)
	} else if outputDir, err := filepath.EvalSymlinks(outputDir); err != nil {
		report.RunError = fmt.Sprintf("resolve output directory symlinks: %v", err)
	} else {
		report.OutputDir = outputDir
	}
	if report.RunError == "" && renderer == nil {
		report.RunError = "renderer is nil"
	} else if report.RunError == "" && (viewport.Width < 1 || viewport.Height < 1 || viewport.Width > maxRenderDimension || viewport.Height > maxRenderDimension || int64(viewport.Width)*int64(viewport.Height) > maxRenderPixels || math.IsNaN(float64(viewport.Scale)) || math.IsInf(float64(viewport.Scale), 0) || viewport.Scale < 0.25 || viewport.Scale > 4) {
		report.RunError = "viewport is outside supported render limits"
	} else if report.RunError == "" && maxInputBytes < 1 {
		report.RunError = "maximum input size must be positive"
	} else if report.RunError == "" {
		var err error
		found, err = discoverInput(config.InputPath)
		if err != nil {
			report.RunError = err.Error()
		} else {
			report.InputRoot = found.Root
			fingerprint, err := corpusFingerprintWithLimit(found.Files, maxInputBytes)
			if err != nil {
				report.RunError = err.Error()
			} else {
				report.CorpusFingerprint = fingerprint
			}
		}
	}

	if report.RunError == "" {
		metadata := buildCaseMetadata(found, config.Manifest)
		for index, file := range found.Files {
			if err := ctx.Err(); err != nil {
				report.RunError = err.Error()
				for _, pending := range found.Files[index:] {
					result := fileResultFor(pending, metadata[pending.Path], StatusNotRun)
					result.Error = "not run: " + err.Error()
					report.Files = append(report.Files, result)
				}
				break
			}
			result, unavailable := runOne(ctx, file, metadata[file.Path], report.OutputDir, viewport, maxInputBytes, renderer)
			report.Files = append(report.Files, result)
			if unavailable {
				report.RunError = result.Error
				for _, pending := range found.Files[index+1:] {
					pendingResult := fileResultFor(pending, metadata[pending.Path], StatusNotRun)
					pendingResult.Error = "not run: " + result.Error
					report.Files = append(report.Files, pendingResult)
				}
				break
			}
		}
	}

	finishRunReport(&report)
	if report.OutputDir != "" {
		if err := writeReport(report.OutputDir, report); err != nil && report.RunError == "" {
			report.RunError = err.Error()
			finishRunReport(&report)
			_ = writeReport(report.OutputDir, report)
		}
	}
	return report
}

func runOne(ctx context.Context, file DiscoveredFile, metadata caseMetadata, outputDir string, viewport Viewport, maxInputBytes int64, renderer Renderer) (DocumentResult, bool) {
	result := fileResultFor(file, metadata, StatusOpenFailed)
	started := time.Now()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	info, err := os.Lstat(file.Path)
	if err != nil {
		result.Error = fmt.Sprintf("stat input: %v", err)
		return completeFileMetrics(&result, started, before, time.Now()), false
	}
	result.FileSizeBytes = info.Size()
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		result.Error = "input is not a regular file"
		return completeFileMetrics(&result, started, before, time.Now()), false
	}
	if info.Size() > maxInputBytes {
		result.Error = fmt.Sprintf("input size %d bytes exceeds limit %d bytes", info.Size(), maxInputBytes)
		return completeFileMetrics(&result, started, before, time.Now()), false
	}
	openStarted := time.Now()
	doc, err := renderer.Open(ctx, file.Path)
	result.OpenDurationMS = time.Since(openStarted).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		if errors.Is(err, ErrRendererUnavailable) {
			result.Status = StatusNotRun
			return completeFileMetrics(&result, started, before, time.Now()), true
		}
		return completeFileMetrics(&result, started, before, time.Now()), false
	}
	if doc == nil {
		result.Error = "renderer returned a nil document without an error"
		return completeFileMetrics(&result, started, before, time.Now()), false
	}

	renderStarted := time.Now()
	streamStart := started
	var renderErr error
	writeTarget := func(target Target) error {
		if result.FirstTargetMS == nil {
			elapsed := time.Since(streamStart).Milliseconds()
			result.FirstTargetMS = &elapsed
		}
		if err := saveTarget(outputDir, metadata.id, len(result.Targets)+1, target, &result); err != nil {
			return err
		}
		return nil
	}
	if streamer, ok := doc.(TargetStreamer); ok {
		renderErr = streamer.RenderEach(ctx, viewport, writeTarget)
	} else {
		var targets []Target
		targets, renderErr = doc.Render(ctx, viewport)
		for _, target := range targets {
			if err := writeTarget(target); err != nil {
				renderErr = errors.Join(renderErr, err)
				break
			}
		}
	}
	result.RenderDurationMS = time.Since(renderStarted).Milliseconds()
	if diagnosticDoc, ok := doc.(DiagnosticDocument); ok {
		result.Diagnostics = cleanDiagnostics(diagnosticDoc.Diagnostics())
	}
	closeErr := doc.Close()
	if renderErr != nil || closeErr != nil {
		result.Status = StatusRenderFailed
		result.Error = errors.Join(renderErr, closeErr).Error()
	} else if result.TargetCount == 0 {
		result.Status = StatusRenderFailed
		result.Error = "renderer produced no screenshot targets"
	} else if len(result.Diagnostics) > 0 {
		result.Status = StatusCompletedWithDiagnostics
	} else {
		result.Status = StatusRendered
	}
	if errors.Is(renderErr, ErrRendererUnavailable) {
		result.Status = StatusNotRun
		return completeFileMetrics(&result, started, before, time.Now()), true
	}
	result = completeFileMetrics(&result, started, before, time.Now())
	result.Baselines = compareRenderedBaselines(outputDir, metadata, result.Targets)
	return result, false
}
