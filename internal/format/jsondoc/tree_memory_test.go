package jsondoc

import (
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// Catch oversized per-node records, excessive capacity and repeated array growth.
func TestLargeTreeAllocationBudget(t *testing.T) {
	const records = 25000
	dense := "[" + strings.Repeat(`{"a":1,"b":false,"c":"x"},`, records-1) + `{"a":1,"b":false,"c":"x"}]`
	sparse := `{"payload":"` + strings.Repeat(`{}[],:x`, 100000) + `","tail":[-12.5e+3,true,null,"a\"b\\c"]}`
	for _, tc := range []struct {
		name, source string
		nodes        int
	}{
		{"dense", dense, 100001},
		{"sparse", sparse, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkTreeAllocationBudget(t, tc.source, tc.nodes)
		})
	}
}

func checkTreeAllocationBudget(t *testing.T, source string, nodes int) {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	tree, err := ParseTree(source)
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if len(tree.Nodes) != nodes {
		t.Fatalf("nodes=%d, want %d", len(tree.Nodes), nodes)
	}
	budget := uint64(nodes*144 + 256*1024)
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > budget {
		t.Fatalf("tree allocated %d bytes, budget %d", allocated, budget)
	}
	runtime.KeepAlive(tree)
}

func TestTreePathsAndChildrenAcrossSourceSizes(t *testing.T) {
	for _, padding := range []int{0, 600000} {
		source := `{"a/b~c":{"点.键":["林墨",true,null]},"a/b~c":2,"padding":"` + strings.Repeat("x", padding) + `"}`
		tree, err := ParseTree(source)
		if err != nil {
			t.Fatal(err)
		}
		if got := tree.Children(0); len(got) != 3 || got[0] != 1 || got[1] != 6 || got[2] != 7 {
			t.Fatalf("root children=%v", got)
		}
		if tree.ChildCount(2) != 3 || tree.NodePointer(3) != "/a~1b~0c/点.键/0" || tree.NodeJSONPath(3) != `$["a/b~c"]["点.键"][0]` || tree.NodeDotPath(3) != `root["a/b~c"]["点.键"][0]` {
			t.Fatal("child count or escaped path changed")
		}
		if tree.FindNodeByPointer("/a~1b~0c/点.键/0") != 3 {
			t.Fatal("pointer lookup changed")
		}
		updated, err := ReplaceValue(tree, 3, `"中文输入"`)
		if err != nil || updated != strings.Replace(source, `"林墨"`, `"中文输入"`, 1) {
			t.Fatal("replacement changed untouched source")
		}
	}
}

func TestRealJSONMemory(t *testing.T) {
	path := os.Getenv("GLANCE_JSON_LARGE_FILE")
	if path == "" {
		t.Skip("set GLANCE_JSON_LARGE_FILE to measure a real file")
	}
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	debug.FreeOSMemory()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	tree, err := ParseTree(doc.Text())
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	debug.FreeOSMemory()
	runtime.ReadMemStats(&after)
	t.Logf("source=%d nodes=%d capacity=%d parse=%v heap=%.1f MiB treeHeap=%.1f MiB totalAllocated=%.1f MiB", len(doc.Text()), len(tree.Nodes), cap(tree.Nodes), elapsed, float64(after.HeapAlloc)/1048576, float64(after.HeapAlloc-before.HeapAlloc)/1048576, float64(after.TotalAlloc-before.TotalAlloc)/1048576)
	runtime.KeepAlive(tree)
	runtime.KeepAlive(doc)
}
