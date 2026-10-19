//go:build darwin && cgo

package diagnostics

/*
#include <libproc.h>
#include <sys/resource.h>
#include <unistd.h>

static int glance_memory_info(struct rusage_info_v2 *info) {
 return proc_pid_rusage(getpid(), RUSAGE_INFO_V2, (rusage_info_t *)info);
}
*/
import "C"

func readProcessMemory() processMemory {
	var info C.struct_rusage_info_v2
	if C.glance_memory_info(&info) != 0 {
		return processMemory{Error: "proc_pid_rusage failed"}
	}
	return processMemory{
		ResidentBytes:          uint64(info.ri_resident_size),
		PhysicalFootprintBytes: uint64(info.ri_phys_footprint),
	}
}
