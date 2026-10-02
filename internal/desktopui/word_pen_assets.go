package desktopui

import (
	"embed"
	"image"
	"image/png"
	"io/fs"
	"path"
	"sync"
)

//go:embed assets/word-pen/*.png
var wordPenAssetFiles embed.FS

var (
	wordPenImageOnce sync.Once
	wordPenImages    map[string]image.Image
)

func wordPenAsset(name string) image.Image {
	wordPenImageOnce.Do(func() {
		wordPenImages = make(map[string]image.Image)
		_ = fs.WalkDir(wordPenAssetFiles, "assets/word-pen", func(file string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || path.Ext(file) != ".png" {
				return err
			}
			asset, err := wordPenAssetFiles.Open(file)
			if err != nil {
				return nil
			}
			defer asset.Close()
			img, err := png.Decode(asset)
			if err == nil {
				wordPenImages[path.Base(file[:len(file)-len(path.Ext(file))])] = img
			}
			return nil
		})
	})
	return wordPenImages[name]
}
