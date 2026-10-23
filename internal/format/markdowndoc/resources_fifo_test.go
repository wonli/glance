//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package markdowndoc

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestLoadImageRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.fifo")
	if err := unix.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := LoadImage(context.Background(), path, "")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "普通文件") {
			t.Fatalf("FIFO load error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("loading a FIFO blocked instead of rejecting it")
	}
}
