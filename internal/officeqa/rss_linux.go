//go:build linux

package officeqa

import "golang.org/x/sys/unix"

func processPeakRSS() (uint64, bool, error) {
	var usage unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &usage); err != nil {
		return 0, false, err
	}
	if usage.Maxrss < 0 {
		return 0, false, nil
	}
	return uint64(usage.Maxrss) * 1024, true, nil
}
