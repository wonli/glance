package officeqa

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var invalidCaseIDChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func saveTarget(outputDir, caseID string, index int, target Target, result *DocumentResult) error {
	if target.Image == nil {
		return fmt.Errorf("target %d (%q) has no image", index, target.Name)
	}
	bounds := target.Image.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width < 1 || height < 1 || width > maxRenderDimension || height > maxRenderDimension || int64(width)*int64(height) > maxRenderPixels {
		return fmt.Errorf("target %d (%q) has invalid or oversized image bounds %v", index, target.Name, bounds)
	}
	if imageIsBlank(target.Image) {
		return fmt.Errorf("target %d (%q) is entirely blank", index, target.Name)
	}
	relativePath := filepath.ToSlash(filepath.Join(caseID, fmt.Sprintf("target-%03d.png", index)))
	caseDir, err := ensureCaseOutputDirectory(outputDir, caseID)
	if err != nil {
		return err
	}
	absolutePath := filepath.Join(caseDir, filepath.Base(filepath.FromSlash(relativePath)))
	if info, err := os.Lstat(absolutePath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("screenshot path %q is a symlink", relativePath)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("screenshot path %q is not a regular file", relativePath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect screenshot path %q: %w", relativePath, err)
	}
	output, err := os.OpenFile(absolutePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create screenshot %q: %w", relativePath, err)
	}
	encodeErr := png.Encode(output, target.Image)
	closeErr := output.Close()
	if encodeErr != nil || closeErr != nil {
		_ = os.Remove(absolutePath)
		return errors.Join(encodeErr, closeErr)
	}
	result.Targets = append(result.Targets, TargetResult{
		Name:   target.Name,
		Path:   relativePath,
		Width:  width,
		Height: height,
	})
	result.TargetCount++
	return nil
}

func ensureCaseOutputDirectory(outputDir, caseID string) (string, error) {
	if err := validateCaseID(caseID); err != nil {
		return "", fmt.Errorf("invalid screenshot case ID: %w", err)
	}
	caseDir := filepath.Join(outputDir, caseID)
	if _, err := os.Lstat(caseDir); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(caseDir, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("create screenshot directory: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("inspect screenshot directory: %w", err)
	}
	info, err := os.Lstat(caseDir)
	if err != nil {
		return "", fmt.Errorf("inspect screenshot directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("screenshot case directory must not be a symlink")
	}
	if !info.IsDir() {
		return "", errors.New("screenshot case path is not a directory")
	}
	resolved, err := filepath.EvalSymlinks(caseDir)
	if err != nil {
		return "", fmt.Errorf("resolve screenshot directory: %w", err)
	}
	relative, err := filepath.Rel(outputDir, resolved)
	if err != nil || !isContainedRelative(relative) {
		return "", errors.New("screenshot directory resolves outside the output directory")
	}
	return resolved, nil
}

func imageIsBlank(img image.Image) bool {
	bounds := img.Bounds()
	first := color.NRGBAModel.Convert(img.At(bounds.Min.X, bounds.Min.Y)).(color.NRGBA)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA) != first {
				return false
			}
		}
	}
	return true
}

func fileResultFor(file DiscoveredFile, metadata caseMetadata, status FileStatus) DocumentResult {
	if metadata.expected == "" {
		metadata.expected = "rendered"
	}
	if metadata.id == "" {
		metadata.id = generatedCaseID(file.RelativePath)
	}
	return DocumentResult{
		CaseID:              metadata.id,
		Path:                file.RelativePath,
		Format:              file.Format,
		FileSizeBytes:       file.SizeBytes,
		Features:            append([]string(nil), metadata.features...),
		Expected:            metadata.expected,
		ExpectedDiagnostics: append([]string(nil), metadata.expectedDiagnostics...),
		Status:              status,
	}
}

func buildCaseMetadata(found discovery, manifest *Manifest) map[string]caseMetadata {
	metadata := make(map[string]caseMetadata, len(found.Files))
	manifestCases := map[string]Case{}
	usedIDs := map[string]bool{}
	if manifest != nil && manifest.Root != "" {
		for _, entry := range manifest.Cases {
			manifestCases[filepath.Clean(filepath.Join(manifest.Root, filepath.FromSlash(entry.Path)))] = entry
			usedIDs[entry.ID] = true
		}
	}
	for _, file := range found.Files {
		if entry, ok := manifestCases[filepath.Clean(file.Path)]; ok {
			metadata[file.Path] = caseMetadata{
				id:                  entry.ID,
				features:            entry.Features,
				expected:            entry.Expected,
				expectedDiagnostics: entry.ExpectedDiagnostics,
				baselineRoot:        manifest.Root,
				baselines:           entry.Baselines,
			}
			continue
		}
		id := generatedCaseID(file.RelativePath)
		for usedIDs[id] {
			id += "x"
		}
		usedIDs[id] = true
		metadata[file.Path] = caseMetadata{id: id, expected: "rendered"}
	}
	return metadata
}

func baselineResultForTarget(outputDir string, metadata caseMetadata, targetName, renderedPath string) BaselineResult {
	baselineRelative, configured := metadata.baselines[targetName]
	if !configured {
		return BaselineResult{Target: targetName, Status: BaselineNotConfigured}
	}
	result := compareBaselineFiles(
		filepath.Join(outputDir, filepath.FromSlash(renderedPath)),
		filepath.Join(metadata.baselineRoot, filepath.FromSlash(baselineRelative)),
	)
	result.Target = targetName
	result.Path = baselineRelative
	return result
}

func compareRenderedBaselines(outputDir string, metadata caseMetadata, targets []TargetResult) []BaselineResult {
	results := make([]BaselineResult, 0, len(targets)+len(metadata.baselines))
	seen := make(map[string]bool, len(targets))
	for _, target := range targets {
		seen[target.Name] = true
		results = append(results, baselineResultForTarget(outputDir, metadata, target.Name, target.Path))
	}
	missingTargets := make([]string, 0, len(metadata.baselines))
	for target := range metadata.baselines {
		if !seen[target] {
			missingTargets = append(missingTargets, target)
		}
	}
	sort.Strings(missingTargets)
	for _, target := range missingTargets {
		results = append(results, BaselineResult{
			Target: target,
			Path:   metadata.baselines[target],
			Status: BaselineTargetMissing,
			Error:  "configured render target was not produced",
		})
	}
	return results
}

func generatedCaseID(relative string) string {
	withoutExtension := strings.TrimSuffix(relative, filepath.Ext(relative))
	base := invalidCaseIDChars.ReplaceAllString(filepath.ToSlash(withoutExtension), "-")
	base = strings.Trim(base, "-._")
	if base == "" {
		base = "document"
	}
	if len(base) > 72 {
		base = base[:72]
	}
	digest := sha256.Sum256([]byte(filepath.ToSlash(relative)))
	return fmt.Sprintf("%s-%x", base, digest[:4])
}

func cleanDiagnostics(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	sort.Strings(result)
	return result
}
