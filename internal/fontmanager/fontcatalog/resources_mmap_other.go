//go:build !darwin && !linux && !windows

package fontcatalog

import (
	"errors"
	"os"
)

func mapFaceFile(file *os.File, size int64) ([]byte, func(), error) {
	return nil, nil, errors.New("mmap not supported on this platform")
}
