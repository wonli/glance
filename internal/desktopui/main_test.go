package desktopui

import (
	"fmt"
	"os"
	"testing"

	"github.com/go-text/typesetting/fontscan"
)

func TestMain(m *testing.M) {
	if os.Getenv("GLANCE_TEST_SKIP_FONTSCAN_PRIME") == "1" {
		os.Exit(m.Run())
	}
	cacheDir, err := os.MkdirTemp("", "glance-desktopui-fonts-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare test font cache: %v\n", err)
		os.Exit(1)
	}
	if _, err := fontscan.SystemFonts(nil, cacheDir); err != nil {
		_ = os.RemoveAll(cacheDir)
		fmt.Fprintf(os.Stderr, "initialize test system fonts: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(cacheDir)
	os.Exit(code)
}
