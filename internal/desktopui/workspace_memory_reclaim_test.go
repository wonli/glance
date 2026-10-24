package desktopui

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/wordlayout"
	"glance/internal/workspace"
)

func TestJSONAndCSVCloseMemoryReclaim(t *testing.T) {
	jsonPath := "/Users/mima1234/Downloads/data_100mb.json"
	if _, err := os.Stat(jsonPath); err != nil {
		tmp := t.TempDir()
		jsonPath = filepath.Join(tmp, "synthetic.json")
		var sb strings.Builder
		sb.WriteString("[\n")
		for i := 0; sb.Len() < 5*1024*1024; i++ {
			if i > 0 {
				sb.WriteString(",\n")
			}
			sb.WriteString(fmt.Sprintf(`{"id": %d, "name": "user_%d", "active": true, "score": 98.5}`, i, i))
		}
		sb.WriteString("\n]")
		if err := os.WriteFile(jsonPath, []byte(sb.String()), 0644); err != nil {
			t.Fatalf("write synthetic json: %v", err)
		}
	}

	csvPath := "/Users/mima1234/Downloads/customers-2000000.csv"
	if _, err := os.Stat(csvPath); err != nil {
		tmp := t.TempDir()
		csvPath = filepath.Join(tmp, "synthetic.csv")
		var sb strings.Builder
		sb.WriteString("id,first_name,last_name,email,gender,ip_address,country\n")
		for i := 1; i <= 20000; i++ {
			sb.WriteString(fmt.Sprintf("%d,First%d,Last%d,user%d@example.com,Female,192.168.1.%d,China\n", i, i, i, i, i%255))
		}
		if err := os.WriteFile(csvPath, []byte(sb.String()), 0644); err != nil {
			t.Fatalf("write synthetic csv: %v", err)
		}
	}

	app := New(nil, "")
	defer close(app.shutdown)
	app.ensureDesktopRuntime()

	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	warmupDoc, _ := loadDocument(csvPath)
	if err := app.installDocument(warmupDoc); err == nil {
		app.layout(gtx)
		app.closeWorkspaceTabNow(app.activeTabID)
		time.Sleep(200 * time.Millisecond)
	}

	runtime.GC()
	debug.FreeOSMemory()
	var mBaseline runtime.MemStats
	runtime.ReadMemStats(&mBaseline)
	t.Logf("Baseline with document UI and fonts loaded: HeapAlloc=%d MB, HeapSys=%d MB",
		mBaseline.HeapAlloc/1024/1024, mBaseline.HeapSys/1024/1024)

	// 1. Open JSON tab
	loadedJSON, err := loadDocument(jsonPath)
	if err != nil {
		t.Fatalf("loadDocument JSON: %v", err)
	}
	if err := app.installDocument(loadedJSON); err != nil {
		t.Fatalf("install JSON: %v", err)
	}
	jsonTabID := app.activeTabID
	loadedJSON = loadedDocument{}

	ops.Reset()
	app.layout(gtx)

	// 2. Open CSV tab
	loadedCSV, err := loadDocument(csvPath)
	if err != nil {
		t.Fatalf("loadDocument CSV: %v", err)
	}
	if err := app.installDocument(loadedCSV); err != nil {
		t.Fatalf("install CSV: %v", err)
	}
	csvTabID := app.activeTabID
	loadedCSV = loadedDocument{}

	ops.Reset()
	app.layout(gtx)

	var mBoth runtime.MemStats
	runtime.ReadMemStats(&mBoth)
	t.Logf("Both open: HeapAlloc=%d MB, HeapSys=%d MB", mBoth.HeapAlloc/1024/1024, mBoth.HeapSys/1024/1024)

	// 3. Close both tabs
	app.closeWorkspaceTabNow(csvTabID)
	app.closeWorkspaceTabNow(jsonTabID)

	// 4. Wait for debounced OS memory release (150ms + margin)
	time.Sleep(300 * time.Millisecond)

	var mAfterClose runtime.MemStats
	runtime.ReadMemStats(&mAfterClose)
	t.Logf("300ms after closing both: HeapAlloc=%d MB, HeapSys=%d MB, NextGC=%d MB",
		mAfterClose.HeapAlloc/1024/1024, mAfterClose.HeapSys/1024/1024, mAfterClose.NextGC/1024/1024)

	diffMB := int64(mAfterClose.HeapAlloc-mBaseline.HeapAlloc) / (1024 * 1024)
	if diffMB > 30 {
		t.Fatalf("expected HeapAlloc to return near baseline (<= 30 MB above baseline), got +%d MB (alloc=%d MB, baseline=%d MB)",
			diffMB, mAfterClose.HeapAlloc/1024/1024, mBaseline.HeapAlloc/1024/1024)
	}

	// 5. Render frames on Recent page and verify memory stays bounded
	for frame := 1; frame <= 60; frame++ {
		time.Sleep(16 * time.Millisecond)
		ops.Reset()
		gtx.Now = time.Now()
		app.layout(gtx)

		if frame%20 == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			t.Logf("Frame %2d on Recent page: HeapAlloc=%d MB, NextGC=%d MB",
				frame, m.HeapAlloc/1024/1024, m.NextGC/1024/1024)
			runtime.GC()
			var mClean runtime.MemStats
			runtime.ReadMemStats(&mClean)
			cleanDiffMB := int64(mClean.HeapAlloc-mBaseline.HeapAlloc) / (1024 * 1024)
			if cleanDiffMB > 30 {
				t.Fatalf("unreleased memory after GC on Recent page: +%d MB above baseline", cleanDiffMB)
			}
		}
	}
}

func Test100MBJSONFullOpenAndCloseMemory(t *testing.T) {
	jsonPath := "/Users/mima1234/Downloads/data_100mb.json"
	if _, err := os.Stat(jsonPath); err != nil {
		t.Skip("100mb json not found")
	}

	app := New(nil, "")
	defer close(app.shutdown)
	app.ensureDesktopRuntime()

	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	runtime.GC()
	debug.FreeOSMemory()
	var mBaseline runtime.MemStats
	runtime.ReadMemStats(&mBaseline)
	t.Logf("Baseline: HeapAlloc=%d MB, HeapSys=%d MB", mBaseline.HeapAlloc/1024/1024, mBaseline.HeapSys/1024/1024)

	mOpen := openAndClose100MBJSON(t, app, jsonPath, gtx, &ops)

	time.Sleep(500 * time.Millisecond)
	runtime.GC()
	runtime.GC()
	debug.FreeOSMemory()

	var mClosed runtime.MemStats
	runtime.ReadMemStats(&mClosed)
	t.Logf("After close, layout Recent, and GC: HeapAlloc=%d MB, HeapSys=%d MB, NextGC=%d MB",
		mClosed.HeapAlloc/1024/1024, mClosed.HeapSys/1024/1024, mClosed.NextGC/1024/1024)
	reclaimedMB := int64(mOpen.HeapAlloc-mClosed.HeapAlloc) / 1024 / 1024
	if reclaimedMB < 450 {
		t.Fatalf("expected at least 450 MB reclaimed after closing JSON, got %d MB (open=%d MB, closed=%d MB)",
			reclaimedMB, mOpen.HeapAlloc/1024/1024, mClosed.HeapAlloc/1024/1024)
	}
	netRetainedMB := int64(mClosed.HeapAlloc-mBaseline.HeapAlloc) / 1024 / 1024
	if netRetainedMB > 80 {
		t.Fatalf("memory not reclaimed after closing 100MB JSON: net retained=%d MB (baseline=%d MB, closed=%d MB), want <= 80 MB",
			netRetainedMB, mBaseline.HeapAlloc/1024/1024, mClosed.HeapAlloc/1024/1024)
	}

	// Now open CSV
	csvPath := "/Users/mima1234/Downloads/customers-2000000.csv"
	if err := app.open(csvPath); err != nil {
		t.Fatalf("open CSV: %v", err)
	}
	ops.Reset()
	gtx.Now = time.Now()
	app.layout(gtx)

	var mAfterCSV runtime.MemStats
	runtime.ReadMemStats(&mAfterCSV)
	t.Logf("After opening CSV: HeapAlloc=%d MB, HeapSys=%d MB, NextGC=%d MB",
		mAfterCSV.HeapAlloc/1024/1024, mAfterCSV.HeapSys/1024/1024, mAfterCSV.NextGC/1024/1024)
	netCSVMB := int64(mAfterCSV.HeapAlloc-mBaseline.HeapAlloc) / 1024 / 1024
	if netCSVMB > 250 {
		t.Fatalf("memory exploded after opening CSV following closed JSON: net=%d MB, want <= 250 MB", netCSVMB)
	}

	// Now close CSV and open Excel (XLSX)
	csvTabID := app.activeTabID
	app.closeWorkspaceTabNow(csvTabID)
	ops.Reset()
	gtx.Now = time.Now()
	app.layout(gtx)
	runtime.GC()

	xlsxPath := filepath.Join("..", "..", "testdata", "office-corpus", "xlsx", "cell-styles.xlsx")
	if err := app.open(xlsxPath); err != nil {
		t.Fatalf("open XLSX: %v", err)
	}
	ops.Reset()
	gtx.Now = time.Now()
	app.layout(gtx)

	var mAfterXLSX runtime.MemStats
	runtime.ReadMemStats(&mAfterXLSX)
	t.Logf("After opening XLSX: HeapAlloc=%d MB, HeapSys=%d MB, NextGC=%d MB",
		mAfterXLSX.HeapAlloc/1024/1024, mAfterXLSX.HeapSys/1024/1024, mAfterXLSX.NextGC/1024/1024)
	netXLSXMB := int64(mAfterXLSX.HeapAlloc-mBaseline.HeapAlloc) / 1024 / 1024
	if netXLSXMB > 150 {
		t.Fatalf("memory exploded after opening Excel following closed JSON: net=%d MB, want <= 150 MB", netXLSXMB)
	}
}

func waitForJSONTree(t *testing.T, app *App, gtx layout.Context, ops *op.Ops) {
	start := time.Now()
	gtx.Ops = ops
	for {
		ops.Reset()
		gtx.Now = time.Now()
		app.layout(gtx)

		v := app.ensureJSONView()
		if v.tree != nil {
			t.Logf("JSON tree parsed and accepted in %v, node count: %d", time.Since(start), len(v.tree.Nodes))
			// Render 10 frames with the tree displayed
			for f := 0; f < 10; f++ {
				ops.Reset()
				gtx.Now = time.Now()
				app.layout(gtx)
				time.Sleep(16 * time.Millisecond)
			}
			return
		}
		if time.Since(start) > 30*time.Second {
			t.Fatal("timeout waiting for JSON tree to parse")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func openAndClose100MBJSON(t *testing.T, app *App, jsonPath string, gtx layout.Context, ops *op.Ops) runtime.MemStats {
	if err := app.open(jsonPath); err != nil {
		t.Fatalf("open JSON: %v", err)
	}
	jsonTabID := app.activeTabID

	waitForJSONTree(t, app, gtx, ops)

	var mOpen runtime.MemStats
	runtime.ReadMemStats(&mOpen)
	t.Logf("JSON fully open: HeapAlloc=%d MB, HeapSys=%d MB", mOpen.HeapAlloc/1024/1024, mOpen.HeapSys/1024/1024)

	app.closeWorkspaceTabNow(jsonTabID)

	ops.Reset()
	gtx.Now = time.Now()
	app.layout(gtx)

	return mOpen
}

func TestAllFormatsOpenCloseAndHomePageIdleMemory(t *testing.T) {
	app := New(nil, "")
	defer close(app.shutdown)
	app.ensureDesktopRuntime()

	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	warmupPath := filepath.Join("..", "..", "testdata", "office-corpus", "docx", "long-table.docx")
	if err := app.open(warmupPath); err == nil {
		app.layout(gtx)
		app.closeWorkspaceTabNow(app.activeTabID)
		time.Sleep(200 * time.Millisecond)
	}

	runtime.GC()
	debug.FreeOSMemory()
	var mBaseline runtime.MemStats
	runtime.ReadMemStats(&mBaseline)
	t.Logf("=== Baseline with fonts and UI loaded === HeapAlloc=%d MB, HeapSys=%d MB",
		mBaseline.HeapAlloc/1024/1024, mBaseline.HeapSys/1024/1024)

	formats := []struct {
		name string
		path string
	}{
		{"100MB JSON", "/Users/mima1234/Downloads/data_100mb.json"},
		{"333MB CSV", "/Users/mima1234/Downloads/customers-2000000.csv"},
		{"XLSX", filepath.Join("..", "..", "testdata", "office-corpus", "xlsx", "cell-styles.xlsx")},
		{"XLS", "/Users/mima1234/Downloads/正常工资薪金所得.xls"},
		{"DOCX", filepath.Join("..", "..", "testdata", "office-corpus", "docx", "long-table.docx")},
		{"DOC", "/Users/mima1234/Downloads/税收征收管理电子数据互信协议书.doc"},
		{"PPTX", filepath.Join("..", "..", "testdata", "office-corpus", "pptx", "sample-show.pptx")},
		{"PDF", "/Users/mima1234/Downloads/古玩商城系统v0.1报价.pdf"},
		{"Markdown", filepath.Join("..", "..", "testdata", "office-corpus", "README.md")},
	}

	for _, tc := range formats {
		if _, err := os.Stat(tc.path); err != nil {
			t.Logf("Skipping %s: %s not found", tc.name, tc.path)
			continue
		}

		t.Logf(">>> Opening %s (%s) ...", tc.name, tc.path)
		if err := app.open(tc.path); err != nil {
			t.Fatalf("open %s: %v", tc.name, err)
		}
		tabID := app.activeTabID

		// If it's JSON, wait for parse tree to be accepted
		if strings.HasSuffix(strings.ToLower(tc.path), ".json") {
			waitForJSONTree(t, app, gtx, &ops)
		} else {
			// Render 5 frames with document displayed
			for f := 0; f < 5; f++ {
				ops.Reset()
				gtx.Now = time.Now()
				app.layout(gtx)
				time.Sleep(10 * time.Millisecond)
			}
		}

		var mOpen runtime.MemStats
		runtime.ReadMemStats(&mOpen)
		t.Logf("[%s Open] HeapAlloc=%d MB, HeapSys=%d MB, NextGC=%d MB",
			tc.name, mOpen.HeapAlloc/1024/1024, mOpen.HeapSys/1024/1024, mOpen.NextGC/1024/1024)

		// Close the tab
		app.closeWorkspaceTabNow(tabID)

		// Render frame on Recent
		ops.Reset()
		gtx.Now = time.Now()
		app.layout(gtx)

		time.Sleep(200 * time.Millisecond)
		runtime.GC()
		debug.FreeOSMemory()

		var mAfterClose runtime.MemStats
		runtime.ReadMemStats(&mAfterClose)
		t.Logf("[%s Closed] HeapAlloc=%d MB, HeapSys=%d MB, NextGC=%d MB",
			tc.name, mAfterClose.HeapAlloc/1024/1024, mAfterClose.HeapSys/1024/1024, mAfterClose.NextGC/1024/1024)

		netRetainedMB := int64(mAfterClose.HeapAlloc-mBaseline.HeapAlloc) / 1024 / 1024
		if netRetainedMB > 120 {
			f, _ := os.Create("/tmp/docx_leak.pprof")
			_ = pprof.WriteHeapProfile(f)
			_ = f.Close()
			t.Fatalf("[%s] memory retained after close too high: +%d MB above baseline (%d MB vs baseline %d MB)",
				tc.name, netRetainedMB, mAfterClose.HeapAlloc/1024/1024, mBaseline.HeapAlloc/1024/1024)
		}
	}

	// Now all documents have been opened and closed.
	// We are idling on the home page (Recent / 首页).
	// Run 500 frames and verify heap memory does not creep up or explode.
	t.Logf("=== Starting 500 idle frames on Recent page ===")
	runtime.GC()
	debug.FreeOSMemory()
	var mRecentStart runtime.MemStats
	runtime.ReadMemStats(&mRecentStart)
	t.Logf("Recent start: HeapAlloc=%d MB, HeapSys=%d MB, NextGC=%d MB",
		mRecentStart.HeapAlloc/1024/1024, mRecentStart.HeapSys/1024/1024, mRecentStart.NextGC/1024/1024)

	for frame := 1; frame <= 500; frame++ {
		ops.Reset()
		gtx.Now = time.Now()
		app.layout(gtx)

		if frame%100 == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			t.Logf("Recent frame %d: HeapAlloc=%d MB, NextGC=%d MB",
				frame, m.HeapAlloc/1024/1024, m.NextGC/1024/1024)
		}
	}

	runtime.GC()
	debug.FreeOSMemory()
	var mRecentEnd runtime.MemStats
	runtime.ReadMemStats(&mRecentEnd)
	t.Logf("Recent after 500 frames & GC: HeapAlloc=%d MB, HeapSys=%d MB, NextGC=%d MB",
		mRecentEnd.HeapAlloc/1024/1024, mRecentEnd.HeapSys/1024/1024, mRecentEnd.NextGC/1024/1024)

	creepMB := int64(mRecentEnd.HeapAlloc-mRecentStart.HeapAlloc) / 1024 / 1024
	if creepMB > 30 {
		t.Fatalf("memory crept up during Recent page idle: +%d MB (start=%d MB, end=%d MB)",
			creepMB, mRecentStart.HeapAlloc/1024/1024, mRecentEnd.HeapAlloc/1024/1024)
	}
}

func TestInactiveWordTabReleasesLayoutEngine(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	col, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}

	app := New(nil, "")
	defer close(app.shutdown)
	app.ensureDesktopRuntime()
	app.workspaceStore = store
	app.collection = col

	var router input.Router
	var ops op.Ops
	renderFrame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Constraints: layout.Exact(image.Pt(1024, 768)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		}
		app.layout(gtx)
		router.Frame(&ops)
	}

	// 1. Open Word document
	docxPath := filepath.Join("..", "..", "testdata", "office-corpus", "docx", "long-table.docx")
	if err := app.open(docxPath); err != nil {
		t.Fatalf("open DOCX: %v", err)
	}
	wordTabID := app.activeTabID
	if wordTabID == "" || wordTabID == workspace.RecentTabID {
		t.Fatalf("expected active word tab, got %q", wordTabID)
	}

	// Render frames to ensure pagination runs and engine is created
	for i := 0; i < 5; i++ {
		renderFrame()
		time.Sleep(10 * time.Millisecond)
	}

	if !app.hasActiveWordDocument() {
		t.Fatal("expected hasActiveWordDocument to be true while Word tab is active")
	}
	if app.wordLayoutEngine == nil {
		t.Fatal("expected wordLayoutEngine to be instantiated for active Word tab")
	}

	// 2. Open an Excel document (now Excel is active, Word tab is inactive)
	xlsxPath := filepath.Join("..", "..", "testdata", "office-corpus", "xlsx", "cell-styles.xlsx")
	if err := app.open(xlsxPath); err != nil {
		t.Fatalf("open XLSX: %v", err)
	}
	excelTabID := app.activeTabID
	if excelTabID == "" || excelTabID == wordTabID {
		t.Fatalf("expected new active excel tab, got %q", excelTabID)
	}

	// Render frame with Excel active
	renderFrame()

	// Verify Word tab is still open in sessions, but Word engine is released!
	if app.hasActiveWordDocument() {
		t.Fatal("hasActiveWordDocument should be false while Excel tab is active")
	}
	if app.wordLayoutEngine != nil {
		t.Fatal("wordLayoutEngine should be nil (released) when Word tab is inactive")
	}
	if app.wordPDFFontSource != nil {
		t.Fatal("wordPDFFontSource should be nil (released) when Word tab is inactive")
	}

	// 3. Switch back to the Word tab
	app.activateWorkspaceTab(wordTabID)
	if app.activeTabID != wordTabID {
		t.Fatalf("expected activeTabID to be wordTabID %q, got %q", wordTabID, app.activeTabID)
	}

	// Render frames to trigger pagination on reactivation
	for i := 0; i < 5; i++ {
		renderFrame()
		time.Sleep(10 * time.Millisecond)
	}

	if !app.hasActiveWordDocument() {
		t.Fatal("expected hasActiveWordDocument to be true after reactivating Word tab")
	}
	state := app.sessions[wordTabID]
	if state == nil || state.wordLayoutResult == nil {
		t.Fatal("expected cached wordLayoutResult to be available on reactivation")
	}

	// Trigger pagination by bumping draft epoch (simulating user typing/editing), verifying engine is re-created on demand
	state.bumpWordDraftEpoch()
	for i := 0; i < 5; i++ {
		renderFrame()
		time.Sleep(10 * time.Millisecond)
	}
	if app.wordLayoutEngine == nil {
		t.Fatal("wordLayoutEngine should be re-instantiated on demand when pagination runs")
	}

	// 4. Switch to Recent (Home page)
	app.activateRecent()
	renderFrame()

	if app.hasActiveWordDocument() {
		t.Fatal("hasActiveWordDocument should be false on Recent page")
	}
	if app.wordLayoutEngine != nil {
		t.Fatal("wordLayoutEngine should be nil (released) on Recent page")
	}
	if app.wordPDFFontSource != nil {
		t.Fatal("wordPDFFontSource should be nil (released) on Recent page")
	}

	// 5. Verify Word session is evicted from heap memory on Recent page, but tab metadata is retained
	if _, ok := app.tabMeta[wordTabID]; !ok {
		t.Fatal("tab metadata must be retained in memory on Recent page")
	}
	if app.sessions[wordTabID] != nil {
		t.Fatal("inactive Word document session should be evicted on Recent page")
	}

	// 6. Switch back to Word tab from Recent and verify it restores seamlessly
	app.activateWorkspaceTab(wordTabID)
	if app.activeTabID != wordTabID {
		t.Fatalf("expected activeTabID %q, got %q", wordTabID, app.activeTabID)
	}
	if app.sessions[wordTabID] == nil || app.sessions[wordTabID].word == nil {
		t.Fatal("Word document session should be restored on demand when tab is reactivated")
	}
}

func TestWorkspaceInitializationLazyLoadsInactiveDocuments(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	col, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}

	r1 := newDesktopRuntime()
	app1 := newAppWithRuntime(nil, r1)
	app1.workspaceStore, app1.collection = store, col

	// Open DOCX
	docxPath := filepath.Join("..", "..", "testdata", "office-corpus", "docx", "long-table.docx")
	if err := app1.open(docxPath); err != nil {
		t.Fatalf("open docx: %v", err)
	}
	docID1 := app1.activeTabID

	// Open XLSX
	xlsxPath := filepath.Join("..", "..", "testdata", "office-corpus", "xlsx", "cell-styles.xlsx")
	if err := app1.open(xlsxPath); err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	docID2 := app1.activeTabID

	// Switch to Recent so active tab in collection is Recent
	app1.activateRecent()
	r1.drainWorkspaceWriter()
	r1.shutdown()

	// Reopen workspace in a fresh app instance
	store2, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	col2, err := workspace.OpenCollection(store2)
	if err != nil {
		t.Fatal(err)
	}

	r2 := newDesktopRuntime()
	app2 := newAppWithRuntime(nil, r2)
	app2.workspaceStore, app2.collection = store2, col2
	app2.initializeWorkspace()
	defer r2.shutdown()

	// Verify tab metadata is loaded for all tabs in workspace
	if len(app2.tabMeta) != 2 {
		t.Fatalf("expected 2 tabs in tabMeta, got %d", len(app2.tabMeta))
	}
	if _, ok := app2.tabMeta[docID1]; !ok {
		t.Fatalf("expected %q in tabMeta", docID1)
	}
	if _, ok := app2.tabMeta[docID2]; !ok {
		t.Fatalf("expected %q in tabMeta", docID2)
	}

	// Verify NO heavy document sessions are loaded at startup when active tab is Recent!
	if len(app2.sessions) != 0 {
		t.Fatalf("expected 0 document sessions loaded in memory at startup on Recent, got %d", len(app2.sessions))
	}
	r2.mu.Lock()
	sessionCount := len(r2.sessions)
	r2.mu.Unlock()
	if sessionCount != 0 {
		t.Fatalf("expected 0 shared document sessions in runtime at startup, got %d", sessionCount)
	}

	// Now activate the XLSX tab on demand
	app2.activateWorkspaceTab(docID2)
	if app2.activeTabID != docID2 {
		t.Fatalf("expected activeTabID %q, got %q", docID2, app2.activeTabID)
	}
	if app2.sessions[docID2] == nil || app2.sessions[docID2].book == nil {
		t.Fatal("expected XLSX document to be restored on demand")
	}

	// Verify DOCX is STILL not loaded in memory!
	if app2.sessions[docID1] != nil {
		t.Fatal("DOCX document should NOT be loaded while XLSX is active")
	}
}

func TestInactiveSpreadsheetRetainsScrollPositionAndReleasesMemory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	col, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}

	app := New(nil, "")
	defer close(app.shutdown)
	app.ensureDesktopRuntime()
	app.workspaceStore = store
	app.collection = col

	var router input.Router
	var ops op.Ops
	renderFrame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Now:         time.Now(),
			Constraints: layout.Exact(image.Pt(1024, 768)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		}
		app.layout(gtx)
		router.Frame(&ops)
	}

	// 1. Open spreadsheet
	xlsxPath := filepath.Join("..", "..", "testdata", "office-corpus", "xlsx", "cell-styles.xlsx")
	if err := app.open(xlsxPath); err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	xlsxTabID := app.activeTabID
	renderFrame()

	// Simulate scrolling down and right
	if app.virtualGrid == nil {
		t.Fatal("expected virtualGrid to be initialized")
	}
	app.virtualGrid.X = 120
	app.virtualGrid.Y = 400
	app.virtualGrid.SelectedRow = 15
	app.virtualGrid.SelectedColumn = 4
	app.selectedCell = "D15"
	renderFrame()

	expectedX := app.virtualGrid.scrollWorldX()
	expectedY := app.virtualGrid.scrollWorldY()

	// 2. Switch to Recent
	app.activateRecent()
	renderFrame()

	// Verify spreadsheet session is evicted from heap memory on Recent page
	if app.sessions[xlsxTabID] != nil {
		t.Fatal("expected spreadsheet session to be evicted from memory on Recent page")
	}
	// Verify tab metadata is still preserved in memory
	if _, ok := app.tabMeta[xlsxTabID]; !ok {
		t.Fatal("tab metadata must be retained in memory")
	}

	// 3. Reactivate the spreadsheet tab
	app.activateWorkspaceTab(xlsxTabID)
	renderFrame()

	if app.activeTabID != xlsxTabID {
		t.Fatalf("expected activeTabID %q, got %q", xlsxTabID, app.activeTabID)
	}
	state := app.sessions[xlsxTabID]
	if state == nil || state.book == nil {
		t.Fatal("expected spreadsheet to be restored on reactivation")
	}
	if state.virtualGrid == nil {
		t.Fatal("expected virtualGrid to be recreated on reactivation")
	}
	if state.virtualGrid.X != expectedX || state.virtualGrid.Y != expectedY {
		t.Fatalf("expected scroll position (%v, %v) restored, got (%v, %v)", expectedX, expectedY, state.virtualGrid.X, state.virtualGrid.Y)
	}
	if state.selectedCell != "D15" {
		t.Fatalf("expected selectedCell D15 restored, got %q", state.selectedCell)
	}
}

func TestMultipleTabsOpenEvictsOldestInactiveSession(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	col, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}

	app := New(nil, "")
	defer close(app.shutdown)
	app.ensureDesktopRuntime()
	app.workspaceStore = store
	app.collection = col

	// 1. Open DOCX (Tab 1)
	docxPath := filepath.Join("..", "..", "testdata", "office-corpus", "docx", "long-table.docx")
	if err := app.open(docxPath); err != nil {
		t.Fatalf("open DOCX: %v", err)
	}
	tab1 := app.activeTabID

	// 2. Open XLSX (Tab 2)
	xlsxPath := filepath.Join("..", "..", "testdata", "office-corpus", "xlsx", "cell-styles.xlsx")
	if err := app.open(xlsxPath); err != nil {
		t.Fatalf("open XLSX: %v", err)
	}
	tab2 := app.activeTabID

	// Both Tab 1 and Tab 2 should be in sessions (capacity = 2)
	if app.sessions[tab1] == nil {
		t.Fatal("expected tab1 to still be in sessions after tab2 open")
	}
	if app.sessions[tab2] == nil {
		t.Fatal("expected tab2 to be in sessions")
	}

	// 3. Open PPTX (Tab 3)
	pptxPath := filepath.Join("..", "..", "testdata", "office-corpus", "pptx", "sample-show.pptx")
	if err := app.open(pptxPath); err != nil {
		t.Fatalf("open PPTX: %v", err)
	}
	tab3 := app.activeTabID

	// Now Tab 1 (oldest inactive) must have been evicted from RAM!
	if app.sessions[tab1] != nil {
		t.Fatal("expected tab1 (oldest inactive) to be evicted after opening tab3")
	}
	// Tab 2 and Tab 3 must still be in sessions
	if app.sessions[tab2] == nil {
		t.Fatal("expected tab2 to be retained in sessions")
	}
	if app.sessions[tab3] == nil {
		t.Fatal("expected tab3 to be in sessions")
	}

	// Tab 1 metadata must still be preserved in tabMeta
	if _, ok := app.tabMeta[tab1]; !ok {
		t.Fatal("tab1 metadata must be preserved in tabMeta")
	}

	// 4. Switch back to Tab 1: Tab 1 must be restored, and Tab 2 should now be evicted!
	if state, err := app.viewForTabWithError(tab1); err != nil {
		t.Fatalf("viewForTabWithError(tab1): %v", err)
	} else if state == nil {
		t.Fatal("expected state from viewForTabWithError")
	}
	app.activateWorkspaceTab(tab1)
	if app.activeTabID != tab1 {
		t.Fatalf("expected activeTabID to be tab1 %q, got %q", tab1, app.activeTabID)
	}
	if app.sessions[tab1] == nil || app.sessions[tab1].word == nil {
		t.Fatal("expected tab1 (Word) to be seamlessly rehydrated on demand")
	}
}

type dummyWordLayoutEngine struct{}

func (d *dummyWordLayoutEngine) Layout(ctx context.Context, input wordlayout.Input) (wordlayout.Result, error) {
	return wordlayout.Result{}, nil
}

func TestBackgroundIdleMemoryReclaim(t *testing.T) {
	origDelay := backgroundIdleReclaimDelay
	backgroundIdleReclaimDelay = 20 * time.Millisecond
	defer func() {
		backgroundIdleReclaimDelay = origDelay
	}()

	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	col, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}

	app := New(nil, "")
	defer close(app.shutdown)
	app.ensureDesktopRuntime()
	app.workspaceStore = store
	app.collection = col

	// Open DOCX (Tab 1)
	docxPath := filepath.Join("..", "..", "testdata", "office-corpus", "docx", "long-table.docx")
	if err := app.open(docxPath); err != nil {
		t.Fatalf("open DOCX: %v", err)
	}
	tab1 := app.activeTabID

	// Open XLSX (Tab 2)
	xlsxPath := filepath.Join("..", "..", "testdata", "office-corpus", "xlsx", "cell-styles.xlsx")
	if err := app.open(xlsxPath); err != nil {
		t.Fatalf("open XLSX: %v", err)
	}
	tab2 := app.activeTabID

	// Both tabs should be in sessions
	if app.sessions[tab1] == nil || app.sessions[tab2] == nil {
		t.Fatal("expected both tab1 and tab2 to be in sessions")
	}

	app.wordLayoutEngine = &dummyWordLayoutEngine{}
	app.wordLayoutEngineFontVersion = 42

	// Transition window to unfocused
	app.handleWindowFocusConfig(false)

	// Wait for background reclaim to fire
	time.Sleep(60 * time.Millisecond)

	// Inactive session tab1 must have been evicted, retaining only active tab2
	if app.sessions[tab1] != nil {
		t.Fatal("expected inactive tab1 to be evicted after background idle reclaim")
	}
	if app.sessions[tab2] == nil {
		t.Fatal("expected active tab2 to remain in sessions")
	}
	if app.wordLayoutEngine != nil {
		t.Fatal("expected wordLayoutEngine to be cleared after background idle reclaim")
	}
	if app.wordLayoutEngineFontVersion != 0 {
		t.Fatalf("expected wordLayoutEngineFontVersion to be reset to 0, got %d", app.wordLayoutEngineFontVersion)
	}

	// Verify refocusing cancels a pending timer
	app.handleWindowFocusConfig(true)
	app.wordLayoutEngine = &dummyWordLayoutEngine{}
	app.wordLayoutEngineFontVersion = 100
	app.handleWindowFocusConfig(false)
	// Immediately refocus before timer fires
	app.handleWindowFocusConfig(true)
	time.Sleep(60 * time.Millisecond)
	if app.wordLayoutEngine == nil || app.wordLayoutEngineFontVersion != 100 {
		t.Fatal("expected wordLayoutEngine to NOT be cleared when focus is restored promptly")
	}
}

func TestMemoryShrinkHookCalledOnRelease(t *testing.T) {
	hookCalled := make(chan struct{}, 1)
	registerMemoryShrinkHook(func() error {
		select {
		case hookCalled <- struct{}{}:
		default:
		}
		return nil
	})

	requestMemoryRelease()

	select {
	case <-hookCalled:
		// success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for memory shrink hook to be called on release")
	}
}
