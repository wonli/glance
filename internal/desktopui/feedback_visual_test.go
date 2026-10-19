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
	"glance/internal/i18n"
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
		message i18n.Message
	}{
		{feedbackSuccess, i18n.Message{ID: "spreadsheet.status.operation.completed"}},
		{feedbackWarning, i18n.Message{ID: "spreadsheet.error.operation_failed"}},
		{feedbackError, i18n.Message{ID: "common.feedback.operation_failed"}},
		{feedbackInfo, i18n.Message{ID: "status.initial.open_document"}},
	} {
		a.feedback.kind = tc.kind
		a.feedback.messageRef = tc.message
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
