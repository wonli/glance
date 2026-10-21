//go:build !unix && !windows

package localaccess

import "os"

func openFileNoBlock(path string) (*os.File, error) {
	return os.Open(path)
}
