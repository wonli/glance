package xls

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"glance/internal/format/xlsx"
)

func BenchmarkConvertLegacyXLSByPopulatedCellCount(b *testing.B) {
	for _, fixture := range []struct {
		name string
		path string
	}{
		{name: "sparse", path: "testdata/formula-style-multisheet.xls"},
		{name: "dense_100k", path: "testdata/benchmark-dense.xls"},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			reader, err := OpenImportReader(fixture.path)
			if err != nil {
				b.Fatal(err)
			}
			var cells uint64
			for _, sheet := range reader.SheetNames() {
				if err := reader.VisitRows(sheet, func(row ImportedRow) error {
					cells += uint64(len(row.Cells))
					return nil
				}); err != nil {
					_ = reader.Close()
					b.Fatal(err)
				}
			}
			if err := reader.Close(); err != nil {
				b.Fatal(err)
			}

			var peak atomic.Uint64
			samplePeak := func() {
				var stats runtime.MemStats
				runtime.ReadMemStats(&stats)
				value := stats.HeapAlloc
				for old := peak.Load(); value > old && !peak.CompareAndSwap(old, value); old = peak.Load() {
				}
			}
			b.ReportAllocs()
			runtime.GC()
			samplePeak()
			stop := make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer close(done)
				ticker := time.NewTicker(time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						samplePeak()
					case <-stop:
						samplePeak()
						return
					}
				}
			}()

			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				converted, err := ConvertToTempXLSX(context.Background(), fixture.path)
				if err != nil {
					b.Fatal(err)
				}
				workbook, err := xlsx.Open(converted.Path)
				if err != nil {
					_ = converted.Cleanup()
					b.Fatal(err)
				}
				if err := workbook.Close(); err != nil {
					_ = converted.Cleanup()
					b.Fatal(err)
				}
				if err := converted.Cleanup(); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			close(stop)
			<-done
			b.ReportMetric(float64(cells), "cells/op")
			b.ReportMetric(float64(peak.Load()), "peak_heap_B/op")
		})
	}
}
