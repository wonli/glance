package fontcatalog

import (
	"context"
	"encoding/json"
	"golang.org/x/image/font/gofont/goregular"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultResourcesAfterCacheRemoval(t *testing.T) {
	root := t.TempDir()
	objects := filepath.Join(root, "objects")
	os.MkdirAll(objects, 0700)
	os.MkdirAll(filepath.Join(root, "licenses"), 0700)
	p := filepath.Join(objects, "font.ttf")
	os.WriteFile(p, goregular.TTF, 0600)
	faces, _ := Inspect(context.Background(), p, Private)
	digest := faces[0].Digest
	dest := filepath.Join(objects, digest)
	os.Rename(p, dest)
	record, _ := json.Marshal(map[string]any{"SHA256": digest, "Family": "Go", "License": "test", "Version": "1"})
	os.WriteFile(filepath.Join(root, "licenses", digest+".json"), record, 0600)
	s, err := LoadResources(context.Background(), filepath.Join(t.TempDir(), "missing.json"), objects, nil)
	if err != nil || len(s.Search("Go")) == 0 {
		t.Fatalf("private resources lost with cache: %v %v", s.Families(), err)
	}
}
