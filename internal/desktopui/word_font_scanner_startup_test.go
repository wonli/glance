package desktopui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
)

func TestWordStartupPrimesFontScannerBeforeGioShaper(t *testing.T) {
	if os.Getenv("GLANCE_FONT_SCAN_STARTUP_CHILD") == "1" {
		New(nil, "")
		shaper, err := newWordTextShaper()
		if err != nil {
			t.Fatal(err)
		}
		shaped, err := shaper.Shape("A中B", document.WordTextStyle{FontSize: 11})
		if err != nil {
			t.Fatal(err)
		}
		if len(shaped.Glyphs) == 0 {
			t.Fatal("Word startup returned no glyphs")
		}
		return
	}

	home := t.TempDir()
	cachePath := filepath.Join(home, "Library", "Caches")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte("not a cache directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestWordStartupPrimesFontScannerBeforeGioShaper$")
	cmd.Env = replaceTestEnv(os.Environ(), "HOME", home)
	cmd.Env = replaceTestEnv(cmd.Env, "XDG_CACHE_HOME", cachePath)
	cmd.Env = replaceTestEnv(cmd.Env, "LOCALAPPDATA", cachePath)
	cmd.Env = replaceTestEnv(cmd.Env, "GLANCE_TEST_SKIP_FONTSCAN_PRIME", "1")
	cmd.Env = replaceTestEnv(cmd.Env, "GLANCE_FONT_SCAN_STARTUP_CHILD", "1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("startup with unavailable user font cache failed: %v\n%s", err, output)
	}
}

func replaceTestEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
