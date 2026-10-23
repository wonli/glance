//go:build darwin && cgo

package markdowndoc

import (
	"os"
	"runtime"
	"testing"
	"time"

	"glance/internal/nativewebview"
)

func TestMain(m *testing.M) {
	runtime.LockOSThread()
	exitCode := make(chan int, 1)
	go func() {
		exitCode <- m.Run()
	}()

	for {
		select {
		case code := <-exitCode:
			os.Exit(code)
		default:
			nativewebview.PumpMainRunLoop(0.01)
			time.Sleep(1 * time.Millisecond)
		}
	}
}
