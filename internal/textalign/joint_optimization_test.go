package textalign

import (
	"context"
	"math/rand"
	"reflect"
	"testing"
)

func TestJointMatchesFullMatrix(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 300; trial++ {
		q, r := make([]rune, rng.Intn(40)), make([]rune, rng.Intn(40))
		for i := range q {
			q[i] = rune('甲' + rng.Intn(4))
		}
		for i := range r {
			r[i] = rune('甲' + rng.Intn(4))
		}
		rows := []int{-1, 0, len(q) / 2, len(q), len(q) + 1}
		if trial%3 == 0 {
			rows = nil
		}
		valid := func(j int) bool { return j%3 != 1 }
		got, err := joint(context.Background(), q, r, rows, valid)
		if err != nil {
			t.Fatal(err)
		}
		want, err := jointReference(context.Background(), q, r, rows, valid)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("trial %d q=%q r=%q got=%+v want=%+v", trial, q, r, got, want)
		}
	}
}

func benchmarkJoint(b *testing.B, reference bool) {
	q := []rune(uniqueText(0x5000, 1000))
	r := append([]rune(nil), q...)
	for i := 17; i < len(r); i += 37 {
		r[i] = '错'
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		if reference {
			_, err = jointReference(context.Background(), q, r, []int{333, 666}, nil)
		} else {
			_, err = joint(context.Background(), q, r, []int{333, 666}, nil)
		}
		if err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkJoint(b *testing.B)           { benchmarkJoint(b, false) }
func BenchmarkJointFullMatrix(b *testing.B) { benchmarkJoint(b, true) }

func TestJointAllocationBudget(t *testing.T) {
	result := testing.Benchmark(func(b *testing.B) { benchmarkJoint(b, false) })
	if used := result.AllocedBytesPerOp(); used > 1500000 {
		t.Fatalf("joint allocated %d bytes; want <= 1500000 for 1000x1000 with two cuts", used)
	}
}

// Full-matrix reference retained to check tie-breaking and all optimal cuts.
// joint uses global unit edit distance between fixed unique anchors. Prefix and
// suffix costs enumerate every optimal cut at requested query-page boundaries.
func jointReference(ctx context.Context, q, r []rune, rows []int, validCut func(int) bool) (jointAlignment, error) {
	if err := ctx.Err(); err != nil {
		return jointAlignment{}, err
	}
	n, m := len(q), len(r)
	cols := m + 1
	d := make([]int32, (n+1)*cols)
	for i := 0; i <= n; i++ {
		d[i*cols] = int32(i)
	}
	for j := 0; j <= m; j++ {
		d[j] = int32(j)
	}
	for i := 1; i <= n; i++ {
		if err := ctx.Err(); err != nil {
			return jointAlignment{}, err
		}
		for j := 1; j <= m; j++ {
			cost := int32(0)
			if q[i-1] != r[j-1] {
				cost = 1
			}
			d[i*cols+j] = min(d[(i-1)*cols+j]+1, d[i*cols+j-1]+1, d[(i-1)*cols+j-1]+cost)
		}
	}
	out := jointAlignment{cuts: map[int]Range{}}
	wanted := map[int]bool{}
	for _, row := range rows {
		wanted[row] = true
	}
	previous, current := make([]int32, cols), make([]int32, cols)
	for j := 0; j <= m; j++ {
		previous[j] = int32(m - j)
	}
	best := d[n*cols+m]
	collect := func(i int, suffix []int32) {
		if !wanted[i] {
			return
		}
		cut := Range{m + 1, -1}
		for j := 0; j <= m; j++ {
			if d[i*cols+j]+suffix[j] == best && (validCut == nil || validCut(j)) {
				cut.Start = min(cut.Start, j)
				cut.End = max(cut.End, j)
			}
		}
		out.cuts[i] = cut
	}
	collect(n, previous)
	for i := n - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return jointAlignment{}, err
		}
		current[m] = int32(n - i)
		for j := m - 1; j >= 0; j-- {
			cost := int32(0)
			if q[i] != r[j] {
				cost = 1
			}
			current[j] = min(previous[j]+1, current[j+1]+1, previous[j+1]+cost)
		}
		collect(i, current)
		previous, current = current, previous
	}
	for i, j := n, m; i > 0 || j > 0; {
		if i > 0 && j > 0 {
			cost := int32(0)
			if q[i-1] != r[j-1] {
				cost = 1
			}
			if d[i*cols+j] == d[(i-1)*cols+j-1]+cost {
				out.pairs = append(out.pairs, alignedPair{i - 1, j - 1, cost == 0})
				i--
				j--
				continue
			}
		}
		if i > 0 && d[i*cols+j] == d[(i-1)*cols+j]+1 {
			i--
		} else {
			j--
		}
	}
	return out, nil
}
