//go:build darwin && cgo

package nativewebview

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	runtime.LockOSThread()
	exitCode := make(chan int, 1)
	go func() {
		exitCode <- m.Run()
	}()

	for {
		select {
		case code := <-exitCode:
			os.Exit(code)
		default:
			PumpMainRunLoop(0.01)
			time.Sleep(1 * time.Millisecond)
		}
	}
}

func TestNativeWebViewEvaluateAndSnapshot(t *testing.T) {
	eng, err := Default()
	if err != nil {
		t.Fatalf("Default() failed: %v", err)
	}

	req := RenderRequest{
		HTML: `<!doctype html><html><head><meta charset="utf-8"><style>body{margin:0;background:transparent;}#box{width:120px;height:60px;background:#3b82f6;border-radius:8px;}</style></head><body><div id="box"></div></body></html>`,
		Script: `
			const box = document.getElementById('box');
			const rect = box.getBoundingClientRect();
			return { width: Math.ceil(rect.width), height: Math.ceil(rect.height) };
		`,
		Density: 2,
	}

	res, err := eng.EvaluateAndSnapshot(context.Background(), req)
	if err != nil {
		t.Fatalf("EvaluateAndSnapshot failed: %v", err)
	}

	if res.Image == nil {
		t.Fatal("expected non-nil image")
	}
	if res.LogicalSize.X != 120 || res.LogicalSize.Y != 60 {
		t.Fatalf("unexpected logical size: %+v", res.LogicalSize)
	}
	if res.Image.Bounds().Dx() != 240 || res.Image.Bounds().Dy() != 120 {
		t.Fatalf("unexpected image bounds for density 2: %+v", res.Image.Bounds())
	}
}
