package desktopui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-text/typesetting/fontscan"
)

func defaultWordFontCacheDir() (string, error) {
	processWordFontCache.once.Do(func() {
		preferred := ""
		if userCache, err := os.UserCacheDir(); err == nil {
			preferred = filepath.Join(userCache, "glance", "fonts")
		}
		processWordFontCache.dir, processWordFontCache.err = writableWordFontCacheDir(preferred)
	})
	return processWordFontCache.dir, processWordFontCache.err
}

func prepareWordFontScanner() error {
	cacheDir, err := defaultWordFontCacheDir()
	if err != nil {
		return fmt.Errorf("prepare Word font cache: %w", err)
	}
	if _, err := fontscan.SystemFonts(nil, cacheDir); err != nil {
		return fmt.Errorf("initialize system font scanner: %w", err)
	}
	return nil
}

func writableWordFontCacheDir(preferred string) (string, error) {
	if preferred != "" {
		if err := ensureWordFontCacheWritable(preferred); err == nil {
			return preferred, nil
		}
	}
	return os.MkdirTemp("", "glance-word-fonts-")
}

func ensureWordFontCacheWritable(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	probe, err := os.CreateTemp(directory, ".write-probe-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	closeErr := probe.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}
