//go:build darwin && cgo

package diagnostics

import "testing"

func assertNativeMemory(t *testing.T, resident uint64, failure string) {
	t.Helper()
	if failure != "" || resident == 0 {
		t.Fatalf("native process snapshot: resident=%d error=%q", resident, failure)
	}
}
