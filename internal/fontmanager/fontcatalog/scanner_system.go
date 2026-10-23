package fontcatalog

import (
	"io"
	"log"
	"os"

	"github.com/go-text/typesetting/fontscan"
)

func SystemDirectories() ([]string, error) {
	dirs, err := fontscan.DefaultFontDirectories(log.New(io.Discard, "", 0))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		out = append(out, dir)
	}
	return out, nil
}
