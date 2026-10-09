//go:build !darwin && !linux && !windows

package officeqa

func processPeakRSS() (uint64, bool, error) {
	return 0, false, nil
}
