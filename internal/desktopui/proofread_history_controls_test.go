package desktopui

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/proofhistory"
)

func TestProofreadingHistoryPageCountSharesTitleCenter(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		w := a.ensureProofreadingWorkbench()
		w.historyPages = []proofhistory.PageSummary{{Page: 1}, {Page: 2}}
		var ops op.Ops
		size := image.Pt(int(700*scale), int(300*scale))
		g := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		a.layoutProofreadingHistoryPages(g)
		shot := renderPresentationTestOps(t, &ops, size)
		title := historyLayoutPixelBounds(shot, image.Rect(int(20*scale), 0, int(220*scale), int(65*scale)), func(c color.NRGBA) bool { return c.A > 240 && c.R < 80 && c.G < 90 && c.B < 100 })
		count := historyLayoutPixelBounds(shot, image.Rect(int(540*scale), 0, int(680*scale), int(60*scale)), func(c color.NRGBA) bool { return c.R < 180 && c.G > c.R+3 && c.B > c.G+3 })
		if title.Empty() || count.Empty() {
			t.Fatal("missing title or count ink")
		}
		delta := float32(count.Min.Y+count.Max.Y-title.Min.Y-title.Max.Y) / (2 * scale)
		if delta < -2 || delta > 2 {
			t.Fatalf("%gx: count is %.1fdp away from title center: title=%v count=%v", scale, delta, title, count)
		}
	}
}

func TestProofreadingHistoryGitDirectoryButtonUsesTaskDirectory(t *testing.T) {
	a := New(nil, "")
	defer a.stopAndWaitManagedBackground()
	w := a.ensureProofreadingWorkbench()
	w.historyProject.Directory = t.TempDir()
	called := make(chan fileCommandResult, 1)
	a.fileCommandRunner = func(kind fileCommandKind, path string) fileCommandResult {
		r := fileCommandResult{kind: kind, path: path}
		called <- r
		return r
	}
	var ops op.Ops
	var router input.Router
	frame := func() {
		ops.Reset()
		g := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(259, 40))}
		a.layoutProofreadingHistoryOpenGitDirectory(g)
		router.Frame(&ops)
	}
	frame()
	point := f32.Pt(100, 20)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
	frame()
	frame()
	select {
	case got := <-called:
		if got.kind != commandOpenDirectory || got.path != w.historyProject.Directory {
			t.Fatalf("opened wrong location: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("Git directory button did not open current task directory")
	}
}

func TestProofreadingHistoryGitDirectoryFailureCanRetry(t *testing.T) {
	a := New(nil, "")
	defer a.stopAndWaitManagedBackground()
	path := t.TempDir()
	a.fileCommandRunner = func(kind fileCommandKind, target string) fileCommandResult {
		return fileCommandResult{kind: kind, path: target, err: os.ErrPermission}
	}
	for attempt := 0; attempt < 2; attempt++ {
		a.startFileCommand(commandOpenDirectory, path)
		select {
		case result := <-a.fileCommandResults:
			a.fileCommandResults <- result
		case <-time.After(time.Second):
			t.Fatal("open folder result did not arrive")
		}
		a.processAsyncResults()
		if a.fileCommandRunning || a.statusRef.ID != "file.command.open_directory_failed" || !strings.Contains(a.feedback.detail, "permission") {
			t.Fatal("directory failure blocked retry or hid its reason")
		}
	}
}

func TestOpenLocalDirectoryRejectsMissingOrEmptyPath(t *testing.T) {
	for _, path := range []string{"", filepath.Join(t.TempDir(), "missing")} {
		if err := openLocalDirectory(path); err == nil {
			t.Fatal("invalid folder was passed to the OS opener")
		}
	}
}
