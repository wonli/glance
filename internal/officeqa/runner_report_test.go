package officeqa

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRunReportSerializesMemoryMetricAvailability(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(map[bool]string{true: "available", false: "unavailable"}[available], func(t *testing.T) {
			report := RunReport{
				Files: []DocumentResult{{
					GoTotalAllocDeltaBytes: 7,
					GoHeapAllocBeforeBytes: 10,
					GoHeapAllocAfterBytes:  12,
				}},
				ProcessPeakRSS: ProcessPeakRSS{Bytes: 123, Available: available},
			}
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"go_total_alloc_delta_bytes", "go_heap_alloc_before_bytes", "go_heap_alloc_after_bytes", "process_peak_rss_bytes", "process_peak_rss_available"} {
				if !strings.Contains(string(data), field) {
					t.Errorf("report JSON lacks metric field %q: %s", field, data)
				}
			}
		})
	}
}
