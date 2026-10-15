package fontcatalog

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Scanner struct {
	DiscoverSystem     bool
	Directories        []string
	PrivateDirectories []string
}
type ScanResult struct {
	Faces       []Face
	Diagnostics []Diagnostic
	Parsed      int
}

func (s Scanner) Scan(ctx context.Context, previous Snapshot, force bool) (ScanResult, error) {
	out := ScanResult{}
	var systemFiles []string
	var systemFamilies map[string]bool
	if s.DiscoverSystem {
		files, fileErr := availableSystemFontFiles()
		if fileErr == nil && len(files) > 0 {
			systemFiles = files
			if names, err := availableSystemFontFamilyNames(); err == nil && len(names) > 0 {
				systemFamilies = make(map[string]bool, len(names))
				for _, name := range names {
					systemFamilies[strings.ToLower(name)] = true
				}
			}
		} else {
			dirs, e := SystemDirectories()
			if e != nil {
				return out, e
			}
			s.Directories = append(s.Directories, dirs...)
		}
	}
	old := map[string][]Face{}
	for _, f := range previous.faces {
		old[f.Path] = append(old[f.Path], f)
	}
	seen := map[string]bool{}
	var failures []error
	appendFaces := func(faces []Face, source Source) {
		for _, face := range faces {
			if source == System && len(systemFamilies) > 0 && !systemFamilyAvailable(face, systemFamilies) {
				continue
			}
			out.Faces = append(out.Faces, face)
		}
	}
	scanFile := func(path string, source Source) {
		if seen[path] {
			return
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".ttf" && ext != ".otf" && ext != ".ttc" && ext != ".otc" && source != Private {
			return
		}
		declaredFamily := ""
		if source == Private {
			var e error
			declaredFamily, e = privateDeclaration(path)
			if e != nil {
				return
			}
		}
		info, err := os.Stat(path)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, Diagnostic{path, err.Error()})
			return
		}
		if !info.Mode().IsRegular() {
			return
		}
		seen[path] = true
		if cached := old[path]; !force && len(cached) > 0 && cached[0].Size == info.Size() && cached[0].ModTime == info.ModTime().UnixNano() && cached[0].Unavailable == "" {
			appendFaces(cached, source)
			return
		}
		faces, err := Inspect(ctx, path, source)
		out.Parsed++
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, Diagnostic{path, err.Error()})
			return
		}
		if source == Private && (len(faces) == 0 || faces[0].Digest != filepath.Base(path) || !strings.EqualFold(faces[0].Family, declaredFamily)) {
			out.Diagnostics = append(out.Diagnostics, Diagnostic{path, "private font identity mismatch"})
			return
		}
		appendFaces(faces, source)
	}
	roots := append(append([]string(nil), s.Directories...), s.PrivateDirectories...)
	for _, root := range roots {
		source := System
		for _, p := range s.PrivateDirectories {
			if p == root {
				source = Private
			}
		}
		complete := true
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				complete = false
				out.Diagnostics = append(out.Diagnostics, Diagnostic{path, walkErr.Error()})
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			scanFile(path, source)
			return nil
		})
		if err != nil {
			return out, err
		}
		if !complete {
			failures = append(failures, fmt.Errorf("font directory could not be completely scanned: %s", root))
			for path, faces := range old {
				rel, e := filepath.Rel(root, path)
				if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || seen[path] {
					continue
				}
				for _, f := range faces {
					f.Unavailable = "font directory could not be verified"
					appendFaces([]Face{f}, source)
				}
				seen[path] = true
			}
		}
	}
	for _, path := range systemFiles {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		scanFile(path, System)
	}
	return out, errors.Join(failures...)
}

func systemFamilyAvailable(face Face, available map[string]bool) bool {
	if available[strings.ToLower(face.Family)] {
		return true
	}
	for _, alias := range face.Aliases {
		if available[strings.ToLower(alias)] {
			return true
		}
	}
	return false
}
