package textintern

import (
	"fmt"
	"sync"
	"testing"
	"unsafe"
)

func TestInternBasic(t *testing.T) {
	s1 := fmt.Sprintf("font-%s", "SimSun")
	s2 := fmt.Sprintf("font-%s", "SimSun")

	p1 := unsafe.StringData(s1)
	p2 := unsafe.StringData(s2)
	if p1 == p2 {
		t.Fatal("expected distinct string allocations before interning")
	}

	i1 := String(s1)
	i2 := String(s2)

	if i1 != i2 {
		t.Fatalf("expected equal strings, got %q vs %q", i1, i2)
	}
	if unsafe.StringData(i1) != unsafe.StringData(i2) {
		t.Fatal("expected interned strings to share identical underlying pointer")
	}

	if String("") != "" {
		t.Fatal("expected empty string handling")
	}
}

func TestInternConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("family-%d", j%5)
				_ = String(key)
			}
		}(i)
	}
	wg.Wait()
}
