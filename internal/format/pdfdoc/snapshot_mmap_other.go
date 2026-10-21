//go:build !darwin && !linux

package pdfdoc

import (
	"fmt"
	"io"
	"os"
)

func mapSnapshotFile(file *os.File, size int64) (*snapshotBacking, error) {
	data, err := io.ReadAll(io.NewSectionReader(file, 0, size))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != size {
		return nil, fmt.Errorf("read PDF snapshot backing: got %d bytes, want %d", len(data), size)
	}
	return &snapshotBacking{
		data: data,
		release: func() error {
			data = nil
			return nil
		},
	}, nil
}
