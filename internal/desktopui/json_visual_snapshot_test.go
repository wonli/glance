package desktopui

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/format/jsondoc"
	"glance/internal/workspace"
)

func TestJSONWorkspaceVisualSnapshot(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	source := `{"user":{"id":10482,"name":"林墨","active":true,"profile":{"email":"linmo@example.com","avatar":"https://cdn.example.com/a.png","bio":null},"roles":["admin","editor","viewer"]},"projects":[{"name":"Glance"},{}],"metadata":{"version":1,"created":"2026-10-06","encoding":"UTF-8"}}`
	app := newTestAppWithJSON(t, source)
	app.activeTabID = "data-json"
	app.tabMeta["data-json"] = workspace.Tab{ID: "data-json", Title: "data.json"}
	app.sessions["data-json"] = app.documentSessionState
	v := app.ensureJSONView()
	tree, err := jsondoc.ParseTree(source)
	if err != nil {
		t.Fatal(err)
	}
	v.accept(jsonTreeResult{text: source, tree: tree})
	for id, n := range tree.Nodes {
		if n.Key == "roles" || n.Key == "projects" || n.Key == "metadata" {
			v.collapsed[id] = true
		}
		if n.Key == "avatar" {
			v.selectNode(id)
		}
	}
	v.rebuildRows()
	for _, size := range []image.Point{image.Pt(1440, 900), image.Pt(900, 600)} {
		ops := new(op.Ops)
		gtx := layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
		dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(app.layoutWorkspaceTabs), layout.Flexed(1, app.layoutJSON))
		if dims.Size != size {
			t.Fatalf("layout=%v want %v", dims.Size, size)
		}
		window, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			t.Skipf("headless unavailable: %v", err)
		}
		if err := window.Frame(ops); err != nil {
			window.Release()
			t.Fatal(err)
		}
		screenshot := image.NewRGBA(image.Rectangle{Max: size})
		if err := window.Screenshot(screenshot); err != nil {
			window.Release()
			t.Fatal(err)
		}
		window.Release()
		if size.X == 1440 {
			// The third 38px tree row follows the tabs, command strip and breadcrumb.
			dy := jsonToolbarHeightDP - 54
			top, bottom, ok := darkInkVerticalBounds(screenshot, image.Rect(346, 223+dy, 380, 259+dy))
			if !ok || math.Abs(float64(top+bottom)/2-(240.5+float64(dy))) > 3 {
				t.Errorf("tree key ink y=%d..%d; want centered in the third row", top, bottom)
			}
			for _, tc := range []struct {
				point image.Point
				want  color.NRGBA
			}{{image.Pt(5, 150), jsonSidebarColor}, {image.Pt(1100, 800), jsonInspectorColor}, {image.Pt(850, 850), contentSurface}} {
				got := color.NRGBAModel.Convert(screenshot.At(tc.point.X, tc.point.Y)).(color.NRGBA)
				if got != tc.want {
					t.Errorf("surface at %v=%v want %v", tc.point, got, tc.want)
				}
			}
		}
		path := os.Getenv("GLANCE_JSON_SCREENSHOT")
		if size.X == 900 {
			path = os.Getenv("GLANCE_JSON_NARROW_SCREENSHOT")
		}
		if path != "" {
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, screenshot)
			_ = file.Close()
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("wrote %s", path)
		}
	}
}
