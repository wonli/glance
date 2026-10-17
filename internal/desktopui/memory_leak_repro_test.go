package desktopui

import (
	"image"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestReproMemoryLeakAfterClosingJSONAndCSV(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "sample.json")
	csvPath := filepath.Join(tempDir, "sample.csv")

	if err := os.WriteFile(jsonPath, []byte(`{"name":"test","items":[1,2,3,4,5],"nested":{"a":1,"b":2}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(csvPath, []byte("col1,col2,col3\nval1,val2,val3\nval4,val5,val6\n"), 0644); err != nil {
		t.Fatal(err)
	}

	app := New(nil, "")
	defer close(app.shutdown)

	// Ensure desktop runtime and collection
	app.ensureDesktopRuntime()

	// 1. Open JSON
	loadedJSON, err := loadDocument(jsonPath)
	if err != nil {
		t.Fatalf("loadDocument JSON: %v", err)
	}
	if err := app.installDocument(loadedJSON); err != nil {
		t.Fatalf("install JSON: %v", err)
	}
	jsonTabID := app.activeTabID
	t.Logf("Opened JSON tab: %s", jsonTabID)

	// Render a frame
	var ops op.Ops
	gtx := layout.Context{
		Ops: &ops,
		Now: time.Now(),
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	app.layout(gtx)

	// 2. Open CSV
	loadedCSV, err := loadDocument(csvPath)
	if err != nil {
		t.Fatalf("loadDocument CSV: %v", err)
	}
	if err := app.installDocument(loadedCSV); err != nil {
		t.Fatalf("install CSV: %v", err)
	}
	csvTabID := app.activeTabID
	t.Logf("Opened CSV tab: %s", csvTabID)

	// Render a frame
	ops.Reset()
	gtx.Now = time.Now()
	app.layout(gtx)

	// Check tabs
	t.Logf("Current active tab: %s, total tabs: %v", app.activeTabID, app.workspaceTabIDs())

	// 3. Close JSON tab
	app.closeWorkspaceTabNow(jsonTabID)
	t.Logf("Closed JSON tab. Active tab now: %s", app.activeTabID)

	// 4. Close CSV tab
	app.closeWorkspaceTabNow(csvTabID)
	t.Logf("Closed CSV tab. Active tab now: %s", app.activeTabID)

	// Both tabs closed. Now we are on "recent" tab.
	// Let's monitor memory and goroutines over 20 iterations or 2 seconds.
	var initialMem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&initialMem)
	t.Logf("Initial HeapAlloc: %d KB, NumGoroutine: %d", initialMem.HeapAlloc/1024, runtime.NumGoroutine())

	// Check goroutines
	pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)

	// Simulate event loop / frames on the Recent page
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		ops.Reset()
		gtx.Now = time.Now()
		app.layout(gtx)
	}

	var finalMem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&finalMem)
	t.Logf("After 50 frames: HeapAlloc: %d KB, NumGoroutine: %d, TotalAlloc Delta: %d KB",
		finalMem.HeapAlloc/1024, runtime.NumGoroutine(), (finalMem.TotalAlloc-initialMem.TotalAlloc)/1024)

	pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
}
