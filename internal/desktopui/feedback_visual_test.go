package desktopui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func TestFeedbackVisualSnapshot(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := New(nil, "")
	size := image.Pt(430, 460)
	var ops op.Ops
	paint.FillShape(&ops, color.NRGBA{R: 245, G: 247, B: 249, A: 255}, clip.Rect{Max: size}.Op())
	for i, tc := range []struct {
		kind    feedbackKind
		message string
	}{
		{feedbackSuccess, "更改已同步到本地文件。"},
		{feedbackWarning, "部分样式可能无法完整保留。"},
		{feedbackError, "请检查文件权限后重试。"},
		{feedbackInfo, "正在打开 data.json。"},
	} {
		a.feedback.kind = tc.kind
		a.feedback.message = tc.message
		shift := op.Offset(image.Pt(20, 20+i*108)).Push(&ops)
		gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(390, 88))}
		a.layoutFeedbackCard(gtx)
		shift.Pop()
		if _, ok := svgIconTint("feedback", feedbackAppearance(tc.kind).icon, feedbackAppearance(tc.kind).ink); !ok {
			t.Fatal("missing feedback icon")
		}
	}
	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skipf("headless unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	shot := image.NewRGBA(image.Rectangle{Max: size})
	if err := window.Screenshot(shot); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, _, ok := darkInkVerticalBounds(shot, image.Rect(82, 20+i*108, 330, 108+i*108)); !ok {
			t.Errorf("feedback %d missing text", i)
		}
	}
	if path := os.Getenv("GLANCE_FEEDBACK_SCREENSHOT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, shot)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}
