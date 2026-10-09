package desktopui

import (
	"image"
	"image/color"

	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/format/xlsx"
	"glance/internal/workspace"
)

func TestPlusMenuDoesNotExposeSaveActions(t *testing.T) {
	a := New(nil, "")
	for _, state := range []string{"empty", "draft", "saved"} {
		if state == "draft" {
			a.documentSessionState = newDocumentSessionState("")
			a.book = xlsx.NewBlank()
			a.book.(*xlsx.Workbook).SetDirty(true)
		} else if state == "saved" {
			a.path = "/tmp/book.xlsx"
		}
		got := a.createActionLabels()
		if containsLabel(got, "保存") || containsLabel(got, "保存…") || containsLabel(got, "另存为…") {
			t.Fatalf("save actions visible for %s document: %v", state, got)
		}
		if !containsLabel(got, "打开本地文件") || !containsLabel(got, "打开文件夹") {
			t.Fatalf("open actions missing for %s document: %v", state, got)
		}
	}
}

func TestPlusTabMatchesPenDocumentTabSpacing(t *testing.T) {
	a := New(nil, "")
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(500, 42)},
	}
	dims := a.layoutPlusTab(gtx)
	if dims.Size != image.Pt(38, int(workspaceTabHeight)) {
		t.Fatalf("plus tab size=%v; want 38x%v (32px button plus 6px left gap)", dims.Size, workspaceTabHeight)
	}
}

func TestPlusMenuAnchorsAtPenClpgmTabPosition(t *testing.T) {
	a := New(nil, "")
	a.clientTitlebar = false
	a.tabMeta["doc-a"] = workspace.Tab{ID: "doc-a", Position: 0}
	a.tabMeta["doc-b"] = workspace.Tab{ID: "doc-b", Position: 1}
	gtx := layout.Context{
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1440, 900)},
	}
	if got := a.plusMenuOffset(gtx); got != 526 {
		t.Fatalf("plus menu x=%d, want Pen Clpgm x=526", got)
	}
}

func TestPlusMenuConnectorGeometry(t *testing.T) {
	bounds := image.Rect(0, 0, 480, 390)
	radius := 12
	plusWidth := 32
	seamCover := 1

	t.Run("left connection", func(t *testing.T) {
		conn := calculateCreateMenuConnector(bounds, 0, plusWidth, radius, seamCover)
		if conn.NW != 0 {
			t.Errorf("expected sharp NW (0), got %d", conn.NW)
		}
		if conn.NE != radius {
			t.Errorf("expected rounded NE (%d), got %d", radius, conn.NE)
		}
		if conn.MinX != 0 || conn.MaxX != plusWidth+seamCover {
			t.Errorf("connector range = [%d, %d], want [0, %d]", conn.MinX, conn.MaxX, plusWidth+seamCover)
		}
	})

	t.Run("middle connection", func(t *testing.T) {
		plusRelX := 200
		conn := calculateCreateMenuConnector(bounds, plusRelX, plusWidth, radius, seamCover)
		if conn.NW != radius {
			t.Errorf("expected rounded NW (%d), got %d", radius, conn.NW)
		}
		if conn.NE != radius {
			t.Errorf("expected rounded NE (%d), got %d", radius, conn.NE)
		}
		if conn.MinX != plusRelX || conn.MaxX != plusRelX+plusWidth {
			t.Errorf("connector range = [%d, %d], want [%d, %d]", conn.MinX, conn.MaxX, plusRelX, plusRelX+plusWidth)
		}
	})

	t.Run("right connection", func(t *testing.T) {
		plusRelX := 480 - plusWidth
		conn := calculateCreateMenuConnector(bounds, plusRelX, plusWidth, radius, seamCover)
		if conn.NW != radius {
			t.Errorf("expected rounded NW (%d), got %d", radius, conn.NW)
		}
		if conn.NE != 0 {
			t.Errorf("expected sharp NE (0), got %d", conn.NE)
		}
		if conn.MinX != plusRelX-seamCover || conn.MaxX != 480 {
			t.Errorf("connector range = [%d, %d], want [%d, 480]", conn.MinX, conn.MaxX, plusRelX-seamCover)
		}
	})

	t.Run("near left transition", func(t *testing.T) {
		conn := calculateCreateMenuConnector(bounds, 5, plusWidth, radius, seamCover)
		if conn.NW != 5 {
			t.Errorf("expected clamped NW (5), got %d", conn.NW)
		}
		if conn.NE != radius {
			t.Errorf("expected rounded NE (%d), got %d", radius, conn.NE)
		}
	})

	t.Run("near right transition", func(t *testing.T) {
		conn := calculateCreateMenuConnector(bounds, 480-plusWidth-4, plusWidth, radius, seamCover)
		if conn.NW != radius {
			t.Errorf("expected rounded NW (%d), got %d", radius, conn.NW)
		}
		if conn.NE != 4 {
			t.Errorf("expected clamped NE (4), got %d", conn.NE)
		}
	})
}

func TestPlusMenuDynamicOffsetAndSeamAlignment(t *testing.T) {
	a := New(nil, "")
	a.clientTitlebar = false
	a.tabMeta["doc-a"] = workspace.Tab{ID: "doc-a", Position: 0}
	a.tabMeta["doc-b"] = workspace.Tab{ID: "doc-b", Position: 1}

	gtxWide := layout.Context{
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1440, 900)},
	}
	plusX := a.plusButtonOffset(gtxWide)
	menuLeft := a.plusMenuOffset(gtxWide)
	if plusX != 526 {
		t.Fatalf("plusX = %d, want 526", plusX)
	}
	if menuLeft != 526 {
		t.Fatalf("menuLeft = %d, want 526", menuLeft)
	}
	relX := plusX - menuLeft
	if relX != 0 {
		t.Fatalf("relX = %d, want 0 (left connection)", relX)
	}

	gtxNarrow := layout.Context{
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(800, 600)},
	}
	plusXNarrow := a.plusButtonOffset(gtxNarrow)
	menuLeftNarrow := a.plusMenuOffset(gtxNarrow)
	if plusXNarrow != 526 {
		t.Fatalf("plusXNarrow = %d, want 526", plusXNarrow)
	}
	if menuLeftNarrow != 312 {
		t.Fatalf("menuLeftNarrow = %d, want 312", menuLeftNarrow)
	}
	relXNarrow := plusXNarrow - menuLeftNarrow
	if relXNarrow != 214 {
		t.Fatalf("relXNarrow = %d, want 214 (middle connection)", relXNarrow)
	}

	a.tabMeta["doc-c"] = workspace.Tab{ID: "doc-c", Position: 2}
	a.tabMeta["doc-d"] = workspace.Tab{ID: "doc-d", Position: 3}
	plusXMany := a.plusButtonOffset(gtxNarrow)
	if plusXMany != 918 {
		t.Fatalf("plusXMany = %d, want 918", plusXMany)
	}
	menuLeftMany := a.plusMenuOffset(gtxNarrow)
	if menuLeftMany != 320 {
		t.Fatalf("menuLeftMany = %d, want 320", menuLeftMany)
	}
}

func TestPlusMenuMiddleConnectionSeamRender(t *testing.T) {
	app := New(nil, "")
	app.clientTitlebar = false
	app.activeTabID = "doc-a"
	app.tabMeta["doc-a"] = workspace.Tab{ID: "doc-a", Title: "test.docx", Position: 0}
	app.tabMeta["doc-b"] = workspace.Tab{ID: "doc-b", Title: "notes.docx", Position: 1}
	app.showNewMenu = true

	const (
		width  = 800
		height = 300
		stripY = 42
	)

	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:    ops,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Max: image.Pt(width, height),
		},
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(app.layoutWorkspaceTabs),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return app.layoutCreateMenuPopover(gtx)
		}),
	)
	if dims.Size.X <= 0 {
		t.Fatal("empty layout dimensions")
	}

	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	if err := window.Frame(ops); err != nil {
		t.Fatalf("render popover: %v", err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(screenshot); err != nil {
		t.Fatalf("capture popover screenshot: %v", err)
	}

	gotSeam := color.NRGBAModel.Convert(screenshot.At(540, stripY)).(color.NRGBA)
	gotSeamAbove := color.NRGBAModel.Convert(screenshot.At(540, stripY-1)).(color.NRGBA)
	if gotSeam != contentSurface {
		t.Errorf("pixel under '+' tab at (540, %d) = %v, want contentSurface %v", stripY, gotSeam, contentSurface)
	}
	if gotSeamAbove != contentSurface {
		t.Errorf("pixel under '+' tab bottom edge at (540, %d) = %v, want contentSurface %v", stripY-1, gotSeamAbove, contentSurface)
	}

	gotBorder := color.NRGBAModel.Convert(screenshot.At(350, stripY)).(color.NRGBA)
	if gotBorder == contentSurface {
		t.Errorf("pixel on top border at (350, %d) unexpectedly covered with contentSurface", stripY)
	}
	if gotBorder.A != 255 || gotBorder.R >= 250 {
		t.Errorf("pixel on top border at (350, %d) = %v, want visible border stroke", stripY, gotBorder)
	}
}

func TestCreateMenuItemsHaveMonochromeIconsAndASectionDivider(t *testing.T) {
	a := New(nil, "")
	items := a.createActionItems()
	if len(items) != 8 || !items[2].separator {
		t.Fatalf("new-document menu items=%+v; want open actions, one divider, and five create actions", items)
	}
	for _, item := range items {
		if item.separator {
			continue
		}
		if item.icon == "" {
			t.Errorf("menu action %q has no icon", item.label)
			continue
		}
		if _, ok := toolbarIconData(item.icon); !ok {
			t.Errorf("menu action %q has no monochrome sprite icon %q", item.label, item.icon)
		}
	}
}
