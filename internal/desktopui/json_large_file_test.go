package desktopui

import (
	"image"
	"image/png"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/format/jsondoc"
)

func TestJSONSourceSegmentsBoundShapingAndKeepUTF8(t *testing.T) {
	text := strings.Repeat("中文🙂", 100000) + "\nlast line"
	segments := jsonSourceSegments(text)
	if len(segments) < 2 {
		t.Fatal("large minified source must be segmented")
	}
	rebuilt := strings.Builder{}
	for i, segment := range segments {
		if segment.end-segment.start > 1024 || !utf8.ValidString(text[segment.start:segment.end]) {
			t.Fatal("unbounded or broken UTF-8 source segment")
		}
		if i > 0 {
			rebuilt.WriteString(text[segments[i-1].end:segment.start])
		}
		rebuilt.WriteString(text[segment.start:segment.end])
	}
	if rebuilt.String() != text {
		t.Fatal("source segments lost text")
	}
	if segments[len(segments)-1].line != 2 {
		t.Fatal("source line number lost")
	}
}

func TestJSONSourceSegmentsMalformedUTF8StillMakesProgress(t *testing.T) {
	text := strings.Repeat(string([]byte{0x80}), jsonSourceEditLimit+1)
	segments := jsonSourceSegments(text)
	if len(segments) == 0 || segments[len(segments)-1].end != len(text) {
		t.Fatal("malformed source did not finish segmentation")
	}
	for _, segment := range segments {
		if segment.end <= segment.start {
			t.Fatal("source segmentation did not advance")
		}
	}
}

func TestJSONSourceGrowthSwitchesToBoundedPreview(t *testing.T) {
	app := newTestAppWithJSON(t, `{}`)
	v := app.ensureJSONView()
	text := `"` + strings.Repeat("x", jsonSourceEditLimit) + `"`
	app.jsonEditor.SetText(text)
	app.commitJSONSourceDraft()
	if v.sourceLoaded {
		t.Fatal("source-editor growth bypassed the bounded preview")
	}
	if app.jsonDoc.Text() != text {
		t.Fatal("switching to preview lost the source draft")
	}
}

func TestJSONSourcePreviewKeepsSnapshotAfterShorterReplacement(t *testing.T) {
	old := `"` + strings.Repeat("x", 2*jsonSourceEditLimit) + `"`
	app := newTestAppWithJSON(t, old)
	v := app.ensureJSONView()
	tree, _ := jsondoc.ParseTree(old)
	v.accept(jsonTreeResult{text: old, tree: tree, segments: jsonSourceSegments(old)})
	v.mode = jsonModeSource
	v.sourceList.List.ScrollTo(len(v.sourceSegments) - 1)
	app.applyJSONTextMutation(`"`+strings.Repeat("y", jsonSourceEditLimit+1)+`"`, "changed")
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 600))}
	app.layoutJSONEditor(gtx) // Must never slice the shorter source with the old offsets.
}

func TestJSONLargeFileInteractiveLayout(t *testing.T) {
	path := os.Getenv("GLANCE_JSON_LARGE_FILE")
	if path == "" {
		t.Skip("set GLANCE_JSON_LARGE_FILE to exercise a real large JSON file")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	doc, err := jsondoc.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	text := doc.Text()
	started := time.Now()
	tree, err := jsondoc.ParseTree(text)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("parsed %d bytes into %d nodes in %v", len(text), len(tree.Nodes), time.Since(started))
	app := New(nil, "")
	state := newDocumentSessionState(path)
	state.jsonDoc = doc
	app.documentSessionState = state
	v := app.ensureJSONView()
	segments := jsonSourceSegments(text)
	started = time.Now()
	v.accept(jsonTreeResult{text: text, tree: tree, segments: segments, baselineSegments: segments})
	t.Logf("published tree in %v; %d visible rows", time.Since(started), len(v.rows))
	if v.sourceLoaded || len(v.value.Text()) > 4096 || !v.value.ReadOnly {
		t.Fatal("large source was loaded into an editable text widget")
	}
	const width, height = 1440, 858
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless unavailable: %v", err)
	}
	defer window.Release()
	ops := new(op.Ops)
	for _, mode := range []jsonViewMode{jsonModeTree, jsonModeSource, jsonModeCompare} {
		v.mode = mode
		ops.Reset()
		started = time.Now()
		gtx := layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(width, height))}
		dims := app.layoutJSON(gtx)
		elapsed := time.Since(started)
		if dims.Size != image.Pt(width, height) {
			t.Fatal("large JSON layout changed window dimensions")
		}
		if elapsed > time.Second {
			t.Fatalf("mode %d layout blocks the window for %v", mode, elapsed)
		}
		if err := window.Frame(ops); err != nil {
			t.Fatal(err)
		}
		t.Logf("mode %d layout took %v", mode, elapsed)
		if mode == jsonModeTree && os.Getenv("GLANCE_JSON_LARGE_SCREENSHOT") != "" {
			screenshot := image.NewRGBA(image.Rect(0, 0, width, height))
			if err := window.Screenshot(screenshot); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(os.Getenv("GLANCE_JSON_LARGE_SCREENSHOT"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, screenshot)
			_ = file.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if doc.Dirty() || doc.Text() != text {
		t.Fatal("viewing large JSON modified the original source")
	}
}
