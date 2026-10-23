package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractRuntimeRejectsTraversalAndSymlinkEscape(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "C:/escape"} {
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		w, _ := z.Create(name)
		w.Write([]byte("x"))
		z.Close()
		f := filepath.Join(t.TempDir(), "bad.zip")
		os.WriteFile(f, b.Bytes(), 0600)
		if err := extractArchive(f, t.TempDir()); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}

func TestRuntimeDigestMismatchDoesNotPublish(t *testing.T) {
	d := t.TempDir()
	archive := filepath.Join(d, "input.zip")
	os.WriteFile(archive, []byte("wrong archive"), 0600)
	out := filepath.Join(d, "output")
	os.Mkdir(out, 0755)
	sentinel := filepath.Join(out, "usable")
	os.WriteFile(sentinel, []byte("old"), 0600)
	data := []byte(`{"version":"1","targets":{"darwin-arm64":{"url":"https://storage.googleapis.com/chrome-for-testing-public/pinned.zip","sha256":"0000000000000000000000000000000000000000000000000000000000000000","platform":"mac-arm64"}}}`)
	if e := stage(data, "darwin-arm64", out, archive); e == nil {
		t.Fatal("bad digest accepted")
	}
	b, _ := os.ReadFile(sentinel)
	if string(b) != "old" {
		t.Fatal("previous runtime damaged")
	}
}
func TestPackageFailsWhenRuntimeIsIncomplete(t *testing.T) {
	d := t.TempDir()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.Create("chrome-headless-shell-mac-arm64/chrome-headless-shell")
	w.Write([]byte("fake"))
	z.Close()
	archive := filepath.Join(d, "runtime.zip")
	os.WriteFile(archive, b.Bytes(), 0600)
	sum := fmt.Sprintf("%x", sha256.Sum256(b.Bytes()))
	data := []byte(fmt.Sprintf(`{"version":"1","targets":{"darwin-arm64":{"url":"https://storage.googleapis.com/chrome-for-testing-public/pinned.zip","sha256":%q,"platform":"mac-arm64"}}}`, sum))
	out := filepath.Join(d, "out")
	if e := stage(data, "darwin-arm64", out, archive); e == nil {
		t.Fatal("incomplete runtime accepted")
	}
	if _, e := os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("published incomplete runtime")
	}
}

func TestCachedRuntimeValidationDetectsMissingRequiredFile(t *testing.T) {
	const target = "darwin-arm64"
	const version = "156.0.8078.47"
	platform := "mac-arm64"
	dir := filepath.Join(t.TempDir(), "runtime")
	chromeDir := filepath.Join(dir, "chrome-headless-shell-"+platform)
	if err := os.MkdirAll(chromeDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"chrome-headless-shell", "icudtl.dat", "headless_lib_data.pak", "headless_command_resources.pak", "LICENSE.headless_shell"} {
		if err := os.WriteFile(filepath.Join(chromeDir, name), []byte("runtime"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	meta := []byte(fmt.Sprintf(`{"Version":%q,"Target":%q}`, version, target))
	if err := os.WriteFile(filepath.Join(dir, "runtime.json"), meta, 0644); err != nil {
		t.Fatal(err)
	}
	manifestData := []byte(fmt.Sprintf(`{"version":%q,"targets":{"%s":{"url":"https://storage.googleapis.com/chrome-for-testing-public/pinned.zip","sha256":"%064d","platform":%q}}}`, version, target, 0, platform))
	if err := validateBundle(manifestData, target, dir); err != nil {
		t.Fatalf("valid staged runtime rejected: %v", err)
	}
	if err := os.Remove(filepath.Join(chromeDir, "icudtl.dat")); err != nil {
		t.Fatal(err)
	}
	if err := validateBundle(manifestData, target, dir); err == nil {
		t.Fatal("cached runtime missing icudtl.dat was accepted")
	}
}

func TestExtractRejectsSymlink(t *testing.T) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "link"}
	h.SetMode(os.ModeSymlink | 0777)
	w, _ := z.CreateHeader(h)
	w.Write([]byte("../escape"))
	z.Close()
	p := filepath.Join(t.TempDir(), "symlink.zip")
	os.WriteFile(p, b.Bytes(), 0600)
	if e := extractArchive(p, t.TempDir()); e == nil {
		t.Fatal("escaping symlink accepted")
	}
}
