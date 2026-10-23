package fontcatalog

import (
	"bytes"
	"context"
	"golang.org/x/image/font/gofont/goregular"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheRestartReuse(t *testing.T) {
	path := testFont(t, "cached.ttf")
	cache := filepath.Join(t.TempDir(), "fonts.json")
	scanner := Scanner{Directories: []string{filepath.Dir(path)}}
	result, err := scanner.Scan(context.Background(), Snapshot{}, false)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSnapshot(4, result.Faces)
	s.CheckedAt = time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)
	if err := SaveCache(cache, s); err != nil {
		t.Fatal(err)
	}
	restored, err := LoadCache(cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Families()) != 1 || restored.Version != 4 || !restored.CheckedAt.Equal(s.CheckedAt) {
		t.Fatalf("cache = %+v", restored)
	}
	result, err = scanner.Scan(context.Background(), restored, false)
	if err != nil || result.Parsed != 0 {
		t.Fatalf("cache not reused: %+v, %v", result, err)
	}
}
func TestForceRefreshSameStatDifferentBytes(t *testing.T) {
	path := testFont(t, "replace.ttf")
	scanner := Scanner{Directories: []string{filepath.Dir(path)}}
	first, err := scanner.Scan(context.Background(), Snapshot{}, false)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	data, _ := os.ReadFile(path)
	old := []byte("Go Regular")
	at := bytes.Index(data, old)
	if at < 0 {
		t.Fatal("fixture name missing")
	}
	copy(data[at:], []byte("Go Changed"))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(path, info.ModTime(), info.ModTime())
	cached := NewSnapshot(1, first.Faces)
	result, err := scanner.Scan(context.Background(), cached, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Faces) != 1 || result.Faces[0].Digest == first.Faces[0].Digest || result.Parsed != 1 {
		t.Fatalf("stale refresh: %+v", result)
	}
	if r, e := cached.OpenFace(first.Faces[0].ID); e == nil {
		r.Close()
		t.Fatal("old snapshot silently read replaced content")
	}
}
func TestDeletedResourceUnavailable(t *testing.T) {
	path := testFont(t, "gone.ttf")
	scanner := Scanner{Directories: []string{filepath.Dir(path)}}
	first, _ := scanner.Scan(context.Background(), Snapshot{}, false)
	os.Remove(path)
	result, err := scanner.Scan(context.Background(), NewSnapshot(1, first.Faces), false)
	if err != nil || len(result.Faces) != 0 {
		t.Fatalf("removed font retained: %+v %v", result, err)
	}
}
func TestBrokenCacheRebuild(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(cache, []byte("{invalid"), 0600)
	if _, err := LoadCache(cache); err == nil {
		t.Fatal("accepted corrupt cache")
	}
}
func TestUnreadableScopeRetainsDiagnostics(t *testing.T) {
	path := testFont(t, "test.ttf")
	first, _ := Inspect(context.Background(), path, System)
	scanner := Scanner{Directories: []string{filepath.Join(filepath.Dir(path), "absent")}}
	first[0].Path = filepath.Join(scanner.Directories[0], "old.ttf")
	result, err := scanner.Scan(context.Background(), NewSnapshot(1, first), false)
	if err == nil || len(result.Diagnostics) == 0 || len(result.Faces) != 1 || result.Faces[0].Unavailable == "" {
		t.Fatalf("failed scope discarded: %+v %v", result, err)
	}
}
func TestCacheWriteFailureKeepsMemoryCatalog(t *testing.T) {
	faces, _ := Inspect(context.Background(), testFont(t, "test.ttf"), System)
	s := NewSnapshot(1, faces)
	if err := SaveCache(filepath.Join(t.TempDir(), "missing", "index"), s); err == nil {
		t.Fatal("expected write error")
	}
	if len(s.Families()) != 1 {
		t.Fatal("write failure destroyed memory snapshot")
	}
}

func TestPrivateScanIgnoresDownloadStaging(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".font-stage-complete"), goregular.TTF, 0600)
	result, err := (Scanner{PrivateDirectories: []string{dir}}).Scan(context.Background(), Snapshot{}, true)
	if err != nil || len(result.Faces) != 0 {
		t.Fatalf("published staging font: %+v %v", result, err)
	}
}
func TestSystemMetadataAcceptsLargeCollections(t *testing.T) {
	path := testFont(t, "large.ttf")
	f, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(96 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	faces, err := Inspect(context.Background(), path, System)
	if err != nil || len(faces) != 1 {
		t.Fatalf("large system font excluded: %v %v", faces, err)
	}
}
