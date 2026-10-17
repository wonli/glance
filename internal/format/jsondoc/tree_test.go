package jsondoc

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTreePreservesOrderNumbersAndEscapedPaths(t *testing.T) {
	source := ` {"z":9007199254740993,"a/b~c":{"点.键":["林墨",true,null]},"z":2} `
	tree, err := ParseTree(source)
	if err != nil {
		t.Fatal(err)
	}
	if tree.KeyCount != 4 || tree.MaxDepth != 3 {
		t.Fatalf("stats: keys=%d depth=%d", tree.KeyCount, tree.MaxDepth)
	}
	if len(tree.Nodes) != 8 {
		t.Fatalf("nodes=%d", len(tree.Nodes))
	}
	if tree.Nodes[1].Raw != "9007199254740993" || tree.Nodes[7].Key != "z" {
		t.Fatal("number precision or duplicate key/order lost")
	}
	n := tree.Nodes[4]
	if tree.NodePointer(4) != "/a~1b~0c/点.键/0" || tree.NodeJSONPath(4) != `$["a/b~c"]["点.键"][0]` || n.Display != "林墨" {
		t.Fatalf("node=%+v", n)
	}
	updated, err := ReplaceValue(tree, 4, `"中文输入"`)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(source, `"林墨"`, `"中文输入"`, 1)
	if updated != want {
		t.Fatalf("replacement changed untouched bytes: %q", updated)
	}
	if _, err := ReplaceValue(tree, 4, `true false`); err == nil {
		t.Fatal("multiple values accepted")
	}
}

func TestTreeRejectsInvalidAndTrailingJSON(t *testing.T) {
	for _, text := range []string{"", `{"name":`, `{} []`, `{"x":NaN}`} {
		if _, err := ParseTree(text); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
	for _, text := range []string{`null`, `[]`, `"中文"`, `123`} {
		if _, err := ParseTree(text); err != nil {
			t.Fatalf("rejected %s: %v", text, err)
		}
	}
}

func TestMeasureLargeJSONMemory(t *testing.T) {
	var b strings.Builder
	b.WriteString("[\n")
	for i := 0; b.Len() < 100*1024*1024; i++ {
		if i > 0 {
			b.WriteString(",\n")
		}
		b.WriteString(`{"id": 123456, "name": "test_user_name_string", "active": true, "score": 98.5, "email": "user@example.com", "role": "developer"}`)
	}
	b.WriteString("\n]")
	jsonStr := b.String()
	t.Logf("Generated JSON size: %d MB", len(jsonStr)/(1024*1024))

	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	tree, err := ParseTree(jsonStr)
	if err != nil {
		t.Fatal(err)
	}
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	t.Logf("Nodes: %d, KeyCount: %d", len(tree.Nodes), tree.KeyCount)
	t.Logf("Alloc: %d MB, TotalAlloc: %d MB, HeapSys: %d MB, Sys: %d MB, NumGC: %d",
		(m2.Alloc-m1.Alloc)/(1024*1024),
		(m2.TotalAlloc-m1.TotalAlloc)/(1024*1024),
		(m2.HeapSys-m1.HeapSys)/(1024*1024),
		(m2.Sys-m1.Sys)/(1024*1024),
		m2.NumGC-m1.NumGC)
}

func BenchmarkParseTree(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("[\n")
	for i := 0; sb.Len() < 5*1024*1024; i++ {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString(`{"id": 123456, "name": "test_user_name_string", "active": true, "score": 98.5, "email": "user@example.com", "role": "developer"}`)
	}
	sb.WriteString("\n]")
	jsonStr := sb.String()

	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(jsonStr)))

	for i := 0; i < b.N; i++ {
		tree, err := ParseTree(jsonStr)
		if err != nil {
			b.Fatal(err)
		}
		if len(tree.Nodes) == 0 {
			b.Fatal("no nodes")
		}
	}
}

func TestParseTreeCancellation(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("[\n")
	for i := 0; sb.Len() < 5*1024*1024; i++ {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString(`{"id": 123456, "name": "test_user_name_string", "active": true, "score": 98.5, "email": "user@example.com", "role": "developer"}`)
	}
	sb.WriteString("\n]")
	jsonStr := sb.String()

	// 1. Pre-cancelled
	done := make(chan struct{})
	close(done)
	_, err := ParseTreeWithCancel(jsonStr, done)
	if err == nil {
		t.Fatal("expected error for pre-cancelled parse, got nil")
	}

	// 2. In-flight cancellation
	cancelDone := make(chan struct{})
	go func() {
		time.Sleep(2 * time.Millisecond)
		close(cancelDone)
	}()
	_, err = ParseTreeWithCancel(jsonStr, cancelDone)
	if err == nil {
		t.Fatal("expected error for in-flight cancelled parse, got nil")
	}
}
