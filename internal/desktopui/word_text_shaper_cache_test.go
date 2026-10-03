package desktopui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWordTextShaperFallsBackWhenFontCachePathIsUnavailable(t *testing.T) {
	root := t.TempDir()
	blockedPath := filepath.Join(root, "blocked-cache")
	if err := os.WriteFile(blockedPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	shaper, err := newWordTextShaperWithCacheDir(blockedPath)
	if err != nil {
		t.Fatal(err)
	}
	if shaper.cacheDir != blockedPath {
		t.Cleanup(func() { _ = os.RemoveAll(shaper.cacheDir) })
	}
	if shaper.cacheDir == blockedPath {
		t.Fatalf("font cache kept unusable path %q", shaper.cacheDir)
	}
	info, err := os.Stat(shaper.cacheDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("fallback font cache %q is not a directory: info=%v err=%v", shaper.cacheDir, info, err)
	}
}
