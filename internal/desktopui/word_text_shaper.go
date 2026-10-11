package desktopui

import "glance/internal/wordfont"

type wordTextShaper = wordfont.Shaper

func newWordTextShaper() (*wordTextShaper, error) {
	return wordfont.NewSystem()
}

func newWordTextShaperWithCacheDir(cacheDir string) (*wordTextShaper, error) {
	return wordfont.NewWithCacheDir(cacheDir)
}
