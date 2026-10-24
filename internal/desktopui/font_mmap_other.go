//go:build !darwin && !linux && !windows

package desktopui

import "errors"

func mapFontFile(path string) ([]byte, func(), error) {
	return nil, nil, errors.New("mmap font not supported on this platform")
}
