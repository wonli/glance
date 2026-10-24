package desktopui

import (
	"glance/internal/fontmanager/fontcatalog"
	"glance/internal/wordfont"
)

type wordTextShaper = wordfont.Shaper

func newWordTextShaper(snapshots ...fontcatalog.Snapshot) (*wordTextShaper, error) {
	if len(snapshots) > 0 && snapshots[0].Version > 0 && len(snapshots[0].Faces()) > 0 {
		return wordfont.NewWithSnapshot(snapshots[0])
	}
	// Fall back to system scan if no snapshots provided or the snapshot is unpopulated.
	return wordfont.NewSystem()
}

func newWordTextShaperWithCacheDir(cacheDir string) (*wordTextShaper, error) {
	return wordfont.NewWithCacheDir(cacheDir)
}
