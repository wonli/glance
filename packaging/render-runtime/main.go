// Build-time verified fetch/staging. This command is never invoked by the app.
package main

import (
	"archive/zip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed manifest.json
var defaultManifest []byte

type manifest struct {
	Version string            `json:"version"`
	Targets map[string]target `json:"targets"`
}
type target struct{ URL, SHA256, Platform string }

func main() {
	t := flag.String("target", "", "target OS-architecture")
	out := flag.String("output", "dist/render-runtime", "output directory")
	m := flag.String("manifest", "", "manifest path (default embedded)")
	archive := flag.String("archive", "", "already downloaded archive (still hash verified)")
	check := flag.Bool("check", false, "validate an existing staged runtime bundle")
	flag.Parse()
	data := defaultManifest
	var err error
	if *m != "" {
		data, err = os.ReadFile(*m)
	}
	if err == nil {
		if *check {
			err = validateBundle(data, *t, *out)
		} else {
			err = stage(data, *t, *out, *archive)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func stage(data []byte, key, output, archive string) error {
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	t, ok := m.Targets[key]
	if !ok || m.Version == "" {
		return fmt.Errorf("unknown runtime target %q", key)
	}
	expected, err := hex.DecodeString(t.SHA256)
	if err != nil || len(expected) != 32 {
		return errors.New("invalid pinned SHA-256")
	}
	if !strings.HasPrefix(t.URL, "https://storage.googleapis.com/chrome-for-testing-public/") {
		return errors.New("untrusted runtime download URL")
	}
	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".render-runtime-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if archive == "" {
		archive = filepath.Join(tmp, "runtime.zip")
		if err := download(t.URL, archive); err != nil {
			return err
		}
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), t.SHA256) {
		return errors.New("runtime SHA-256 mismatch")
	}
	dest := filepath.Join(tmp, "content")
	if err := extractArchive(archive, dest); err != nil {
		return err
	}
	meta, _ := json.Marshal(struct{ Version, Target string }{m.Version, key})
	if err := os.WriteFile(filepath.Join(dest, "runtime.json"), meta, 0644); err != nil {
		return err
	}
	if err := validateBundle(data, key, dest); err != nil {
		return err
	}
	// Refuse to replace an existing bundle. Callers stage to a new release path;
	// a failed build must never damage a usable installed runtime.
	if _, err := os.Stat(output); err == nil {
		return errors.New("runtime output already exists; stage to a fresh directory")
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(dest, output)
}

// validateBundle is run both after staging and against the exact copied runtime
// directory that a portable package is about to publish.
func validateBundle(data []byte, key, dir string) error {
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	t, ok := m.Targets[key]
	if !ok || m.Version == "" || t.Platform == "" || filepath.Base(t.Platform) != t.Platform {
		return fmt.Errorf("unknown or invalid runtime target %q", key)
	}
	metaPath := filepath.Join(dir, "runtime.json")
	metaInfo, err := os.Lstat(metaPath)
	if err != nil || !metaInfo.Mode().IsRegular() || metaInfo.Size() == 0 {
		if err == nil {
			err = fmt.Errorf("not a nonempty regular file")
		}
		return fmt.Errorf("incomplete runtime: runtime.json: %w", err)
	}
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("incomplete runtime: runtime.json: %w", err)
	}
	var meta struct{ Version, Target string }
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return fmt.Errorf("invalid runtime metadata: %w", err)
	}
	if meta.Version != m.Version || meta.Target != key {
		return fmt.Errorf("runtime metadata mismatch: got %s/%s, want %s/%s", meta.Version, meta.Target, m.Version, key)
	}
	executable := "chrome-headless-shell"
	if key == "windows-amd64" {
		executable += ".exe"
	}
	base := filepath.Join(dir, "chrome-headless-shell-"+t.Platform)
	baseInfo, err := os.Lstat(base)
	if err != nil || !baseInfo.IsDir() {
		if err == nil {
			err = fmt.Errorf("not a runtime directory")
		}
		return fmt.Errorf("incomplete runtime: %s: %w", filepath.Base(base), err)
	}
	for _, name := range []string{executable, "icudtl.dat", "headless_lib_data.pak", "headless_command_resources.pak", "LICENSE.headless_shell"} {
		path := filepath.Join(base, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			if err == nil {
				err = fmt.Errorf("not a nonempty regular file")
			}
			return fmt.Errorf("incomplete runtime: %s: %w", name, err)
		}
	}
	return nil
}

func download(url, path string) error {
	client := http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" || req.URL.Host != "storage.googleapis.com" {
			return errors.New("untrusted redirect")
		}
		return nil
	}}
	r, err := client.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("runtime download HTTP %d", r.StatusCode)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, e := io.Copy(f, io.LimitReader(r.Body, 512<<20+1))
	ce := f.Close()
	if e != nil {
		return e
	}
	if n > 512<<20 {
		return errors.New("runtime archive too large")
	}
	return ce
}
func extractArchive(archive, dest string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	var total uint64
	for _, f := range z.File {
		name := f.Name
		clean := filepath.Clean(filepath.FromSlash(name))
		if strings.Contains(name, "\\") || strings.Contains(name, ":") || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe archive path %q", name)
		}
		if f.UncompressedSize64 > 1<<30 || total > (1<<30)-f.UncompressedSize64 {
			return errors.New("runtime expands beyond 1 GiB")
		}
		total += f.UncompressedSize64
		if total > 1<<30 {
			return errors.New("runtime expands beyond 1 GiB")
		}
		path := filepath.Join(dest, clean)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		src, e := f.Open()
		if e != nil {
			return e
		}
		mode := os.FileMode(0644)
		if f.Mode()&0111 != 0 {
			mode = 0755
		}
		out, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if e != nil {
			src.Close()
			return e
		}
		_, e = io.Copy(out, io.LimitReader(src, 1<<30+1))
		ce := out.Close()
		src.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
	return nil
}
