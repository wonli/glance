package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsPortableIncludesRuntimeAndNotices(t *testing.T) {
	d := t.TempDir()
	root := filepath.Join(d, "Glance")
	for _, p := range []string{"Glance.exe", "render-runtime/runtime.json", "render-runtime/LICENSE.headless_shell"} {
		f := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(f), 0755)
		os.WriteFile(f, []byte("fixture"), 0644)
	}
	out := filepath.Join(d, "portable.zip")
	if e := pack(root, out); e != nil {
		t.Fatal(e)
	}
	z, e := zip.OpenReader(out)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	for _, name := range []string{"Glance/Glance.exe", "Glance/render-runtime/runtime.json", "Glance/render-runtime/LICENSE.headless_shell"} {
		found := false
		for _, f := range z.File {
			if f.Name == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s", name)
		}
	}
}
