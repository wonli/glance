//go:build !darwin || !cgo

package diagnostics

func readProcessMemory() processMemory {
	return processMemory{Error: "native process accounting currently requires macOS with cgo"}
}
