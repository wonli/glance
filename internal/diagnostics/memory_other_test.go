//go:build !darwin || !cgo

package diagnostics

import "testing"

func assertNativeMemory(t *testing.T, resident uint64, failure string) {
	t.Helper()
	if resident == 0 && failure == "" {
		t.Fatal("missing process stats must report an error")
	}
}
