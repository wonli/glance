//go:build windows

package officeqa

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var psapi = windows.NewLazySystemDLL("psapi.dll")
var getProcessMemoryInfo = psapi.NewProc("GetProcessMemoryInfo")

type processMemoryCounters struct {
	Size                    uint32
	PageFaultCount          uint32
	PeakWorkingSetSize      uintptr
	WorkingSetSize          uintptr
	QuotaPeakPagedPoolUsage uintptr
	QuotaPagedPoolUsage     uintptr
	QuotaPeakNonPagedUsage  uintptr
	QuotaNonPagedUsage      uintptr
	PagefileUsage           uintptr
	PeakPagefileUsage       uintptr
}

func processPeakRSS() (uint64, bool, error) {
	if err := getProcessMemoryInfo.Find(); err != nil {
		return 0, false, err
	}
	counters := processMemoryCounters{Size: uint32(unsafe.Sizeof(processMemoryCounters{}))}
	result, _, callErr := getProcessMemoryInfo.Call(
		uintptr(windows.CurrentProcess()),
		uintptr(unsafe.Pointer(&counters)),
		uintptr(counters.Size),
	)
	if result == 0 {
		return 0, false, fmt.Errorf("GetProcessMemoryInfo: %w", callErr)
	}
	return uint64(counters.PeakWorkingSetSize), true, nil
}
