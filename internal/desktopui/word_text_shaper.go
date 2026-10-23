package desktopui

import (
	"glance/internal/fontmanager/fontcatalog"
	"glance/internal/wordfont"
)

type wordTextShaper = wordfont.Shaper

func newWordTextShaper(snapshots ...fontcatalog.Snapshot) (*wordTextShaper, error) {
	if len(snapshots) > 0 {
		return wordfont.NewWithSnapshot(snapshots[0])
	}
	// The implicit system scan is retained only for windowless test utilities.
	return wordfont.NewSystem()
}

func newWordTextShaperWithCacheDir(cacheDir string) (*wordTextShaper, error) {
	return wordfont.NewWithCacheDir(cacheDir)
}
