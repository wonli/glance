package officeqa

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	manifestVersion           = 1
	maxManifestBytes          = 2 << 20
	maxCaseIDLength           = 128
	maxManifestCases          = 10000
	maxFeatureTagSize         = 64
	maxExpectedDiagnosticSize = 128
)

var caseIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Case describes one checked-in Office document without per-file provenance.
type Case struct {
	ID                  string            `json:"id"`
	Path                string            `json:"path"`
	Format              string            `json:"format"`
	Features            []string          `json:"features"`
	Expected            string            `json:"expected"`
	ExpectedDiagnostics []string          `json:"expected_diagnostics,omitempty"`
	Notes               string            `json:"notes,omitempty"`
	Baselines           map[string]string `json:"baselines,omitempty"`
}

// Manifest is the curated, repository-local Office test corpus index.
type Manifest struct {
	Version int    `json:"version"`
	Cases   []Case `json:"cases"`
	Root    string `json:"-"`
}

// LoadManifest reads and validates root/manifest.json. Document paths must
// resolve to regular files within root; baseline paths are validated but may
// be absent so the run can report a missing configured baseline explicitly.
func LoadManifest(root string) (Manifest, error) {
	var manifest Manifest
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return manifest, fmt.Errorf("resolve corpus root: %w", err)
	}
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return manifest, fmt.Errorf("resolve corpus root: %w", err)
	}
	rootInfo, err := os.Stat(rootPath)
	if err != nil {
		return manifest, fmt.Errorf("stat corpus root: %w", err)
	}
	if !rootInfo.IsDir() {
		return manifest, fmt.Errorf("corpus root %q is not a directory", root)
	}

	manifestPath := filepath.Join(rootPath, "manifest.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return manifest, fmt.Errorf("inspect corpus manifest: %w", err)
	}
	if manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
		return manifest, errors.New("corpus manifest must be a regular file, not a symlink")
	}
	manifestFile, err := os.Open(manifestPath)
	if err != nil {
		return manifest, fmt.Errorf("read corpus manifest: %w", err)
	}
	manifestBytes, readErr := io.ReadAll(io.LimitReader(manifestFile, maxManifestBytes+1))
	closeErr := manifestFile.Close()
	if readErr != nil || closeErr != nil {
		return manifest, fmt.Errorf("read corpus manifest: %w", errors.Join(readErr, closeErr))
	}
	if len(manifestBytes) > maxManifestBytes {
		return manifest, fmt.Errorf("corpus manifest exceeds %d bytes", maxManifestBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode corpus manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, errors.New("corpus manifest contains trailing JSON data")
		}
		return Manifest{}, fmt.Errorf("decode trailing manifest data: %w", err)
	}
	if manifest.Version != manifestVersion {
		return Manifest{}, fmt.Errorf("unsupported corpus manifest version %d", manifest.Version)
	}
	if len(manifest.Cases) == 0 {
		return Manifest{}, errors.New("corpus manifest has no cases")
	}
	if len(manifest.Cases) > maxManifestCases {
		return Manifest{}, fmt.Errorf("corpus manifest has more than %d cases", maxManifestCases)
	}
	manifest.Root = rootPath

	ids := make(map[string]struct{}, len(manifest.Cases))
	paths := make(map[string]struct{}, len(manifest.Cases))
	for index := range manifest.Cases {
		caseEntry := &manifest.Cases[index]
		if err := validateCaseID(caseEntry.ID); err != nil {
			return Manifest{}, fmt.Errorf("case %d: %w", index+1, err)
		}
		if _, duplicate := ids[caseEntry.ID]; duplicate {
			return Manifest{}, fmt.Errorf("duplicate case ID %q", caseEntry.ID)
		}
		ids[caseEntry.ID] = struct{}{}

		caseEntry.Format = strings.ToLower(strings.TrimSpace(caseEntry.Format))
		extension := strings.ToLower(path.Ext(caseEntry.Path))
		if !supportedFormatExtension(caseEntry.Format, extension) {
			return Manifest{}, fmt.Errorf("case %q has unsupported or mismatched format %q and path %q", caseEntry.ID, caseEntry.Format, caseEntry.Path)
		}
		cleanPath, err := validateCorpusRelativePath(caseEntry.Path)
		if err != nil {
			return Manifest{}, fmt.Errorf("case %q document path: %w", caseEntry.ID, err)
		}
		caseEntry.Path = cleanPath
		if _, duplicate := paths[cleanPath]; duplicate {
			return Manifest{}, fmt.Errorf("duplicate case path %q", cleanPath)
		}
		paths[cleanPath] = struct{}{}

		if err := validateExistingCorpusFile(rootPath, cleanPath); err != nil {
			return Manifest{}, fmt.Errorf("case %q document path: %w", caseEntry.ID, err)
		}
		if !validExpectedOutcome(caseEntry.Expected) {
			return Manifest{}, fmt.Errorf("case %q has unsupported expected outcome %q", caseEntry.ID, caseEntry.Expected)
		}
		for _, feature := range caseEntry.Features {
			if feature == "" || len(feature) > maxFeatureTagSize || strings.TrimSpace(feature) != feature {
				return Manifest{}, fmt.Errorf("case %q has invalid feature tag %q", caseEntry.ID, feature)
			}
		}
		expectedDiagnostics := make(map[string]struct{}, len(caseEntry.ExpectedDiagnostics))
		for _, diagnostic := range caseEntry.ExpectedDiagnostics {
			if diagnostic == "" || len(diagnostic) > maxExpectedDiagnosticSize || strings.TrimSpace(diagnostic) != diagnostic {
				return Manifest{}, fmt.Errorf("case %q has invalid expected diagnostic %q", caseEntry.ID, diagnostic)
			}
			if _, duplicate := expectedDiagnostics[diagnostic]; duplicate {
				return Manifest{}, fmt.Errorf("case %q has duplicate expected diagnostic %q", caseEntry.ID, diagnostic)
			}
			expectedDiagnostics[diagnostic] = struct{}{}
		}
		for target, baseline := range caseEntry.Baselines {
			if strings.TrimSpace(target) == "" {
				return Manifest{}, fmt.Errorf("case %q has an empty baseline target", caseEntry.ID)
			}
			baselinePath, err := validateCorpusRelativePath(baseline)
			if err != nil {
				return Manifest{}, fmt.Errorf("case %q baseline path: %w", caseEntry.ID, err)
			}
			if !strings.EqualFold(path.Ext(baselinePath), ".png") {
				return Manifest{}, fmt.Errorf("case %q baseline %q is not a PNG path", caseEntry.ID, baseline)
			}
			if err := validateOptionalBaseline(rootPath, baselinePath); err != nil {
				return Manifest{}, fmt.Errorf("case %q baseline path: %w", caseEntry.ID, err)
			}
			caseEntry.Baselines[target] = baselinePath
		}
	}
	return manifest, nil
}

func validateOptionalBaseline(root, relative string) error {
	current := root
	parts := strings.Split(relative, "/")
	for index, part := range parts {
		candidate := filepath.Join(current, part)
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect %q: %w", relative, err)
		}
		resolved := candidate
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err = filepath.EvalSymlinks(candidate)
			if err != nil {
				return fmt.Errorf("resolve symlink in %q: %w", relative, err)
			}
			resolvedRelative, err := filepath.Rel(root, resolved)
			if err != nil || !isContainedRelative(resolvedRelative) {
				return fmt.Errorf("path %q resolves outside the corpus root", relative)
			}
			info, err = os.Stat(resolved)
			if err != nil {
				return fmt.Errorf("stat resolved baseline %q: %w", relative, err)
			}
		}
		if index < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("parent of baseline %q is not a directory", relative)
		}
		if index == len(parts)-1 && !info.Mode().IsRegular() {
			return fmt.Errorf("baseline %q is not a regular file", relative)
		}
		current = resolved
	}
	return nil
}

func validateCaseID(id string) error {
	if len(id) == 0 || len(id) > maxCaseIDLength || !caseIDPattern.MatchString(id) || id == "." || id == ".." {
		return fmt.Errorf("invalid case ID %q", id)
	}
	return nil
}

func validateCorpusRelativePath(value string) (string, error) {
	if value == "" || strings.Contains(value, `\`) || strings.TrimSpace(value) != value {
		return "", fmt.Errorf("path %q must be a clean repository-relative path using forward slashes", value)
	}
	if !fs.ValidPath(value) || path.Clean(value) != value || path.IsAbs(value) {
		return "", fmt.Errorf("path %q is not a safe relative path", value)
	}
	return value, nil
}

func validateExistingCorpusFile(root, relative string) error {
	current := root
	parts := strings.Split(relative, "/")
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("stat %q: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path %q contains a symlink", relative)
		}
		if index < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("parent of %q is not a directory", relative)
		}
		if index == len(parts)-1 && !info.Mode().IsRegular() {
			return fmt.Errorf("path %q is not a regular file", relative)
		}
	}
	return nil
}

func isContainedRelative(relative string) bool {
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func supportedFormatExtension(format, extension string) bool {
	switch format {
	case "xlsx":
		return extension == ".xlsx"
	case "docx":
		return extension == ".docx"
	case "pptx":
		return extension == ".pptx"
	default:
		return false
	}
}

func validExpectedOutcome(value string) bool {
	switch value {
	case "rendered", "completed_with_diagnostics", "open_failure", "render_failure":
		return true
	default:
		return false
	}
}
