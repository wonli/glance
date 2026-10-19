package diagnostics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMemorySnapshotAndExplicitRelease(t *testing.T) {
	mux := http.NewServeMux()
	registerMemoryHandlers(mux)
	request := func(method, path string) *http.Response {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
		return recorder.Result()
	}
	response := request(http.MethodGet, "/debug/memory")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("snapshot status=%d", response.StatusCode)
	}
	var snapshot struct {
		PID int `json:"pid"`
		Go  struct {
			HeapAlloc             uint64 `json:"heap_alloc"`
			RuntimeMappedEstimate uint64 `json:"runtime_mapped_estimate"`
		} `json:"go"`
		Process struct {
			ResidentBytes uint64 `json:"resident_bytes"`
			Error         string `json:"error"`
		} `json:"process"`
	}
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.PID <= 0 || snapshot.Go.HeapAlloc == 0 || snapshot.Go.RuntimeMappedEstimate == 0 {
		t.Fatalf("missing accounting: %+v", snapshot)
	}
	assertNativeMemory(t, snapshot.Process.ResidentBytes, snapshot.Process.Error)
	// Reading a URL must not trigger the intrusive release operation.
	response = request(http.MethodGet, "/debug/memory/release")
	response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET release status=%d", response.StatusCode)
	}
	response = request(http.MethodPost, "/debug/memory/release")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST release status=%d", response.StatusCode)
	}
	var release struct {
		Before, After struct {
			PID int
			Go  struct {
				NumGC uint32 `json:"num_gc"`
			} `json:"go"`
		}
		Duration int64 `json:"duration_ms"`
	}
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		t.Fatal(err)
	}
	if release.Before.PID != snapshot.PID || release.After.PID != snapshot.PID || release.After.Go.NumGC <= release.Before.Go.NumGC {
		t.Fatalf("release did not collect same process: %+v", release)
	}
}
