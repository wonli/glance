package desktopui

import (
	"image"
	"image/color"
	"reflect"
	"runtime"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/i18n"
)

func TestDialogIDSurvivesLocaleChange(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	app.modal.ShowInputMessage("word.rename", i18n.Message{ID: "common.dialog.confirm.title"},
		i18n.Message{ID: "common.dialog.confirm.body"}, "draft name",
		i18n.Message{ID: "common.dialog.field.name"}, nil,
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.confirm"}},
	)
	app.modal.Input.SetText("unsaved title")
	if got := app.text(app.modal.TitleRef); got != "Confirm" {
		t.Fatalf("English dialog title = %q; want Confirm", got)
	}

	app.applyLocaleSelection(i18n.SelectionChinese)
	if app.modal.DialogID != "word.rename" {
		t.Fatalf("dialog ID after locale switch = %q", app.modal.DialogID)
	}
	if got := app.text(app.modal.TitleRef); got != "确认" {
		t.Fatalf("Chinese dialog title = %q; want 确认", got)
	}
	if got := app.modal.Input.Text(); got != "unsaved title" {
		t.Fatalf("locale switch discarded the input draft: %q", got)
	}
	if len(app.modal.Actions) != 1 || app.text(app.modal.Actions[0].LabelRef) != "确定" {
		t.Fatalf("dialog action label did not resolve in the new locale: %+v", app.modal.Actions)
	}
}

func TestDialogActionStyleUsesActionID(t *testing.T) {
	cases := []struct {
		id    string
		label string
		want  modalDialogActionStyle
	}{
		{"confirm", "删除", modalDialogActionPrimary},
		{"custom", "取消", modalDialogActionPrimary},
		{"cancel", "Continue", modalDialogActionSecondary},
		{"delete", "Delete", modalDialogActionDanger},
	}
	for _, tc := range cases {
		if got := resolveDialogActionStyle(modalDialogActionSpec{ID: tc.id, Label: tc.label}); got != tc.want {
			t.Errorf("action id=%q label=%q resolved style=%v; want %v", tc.id, tc.label, got, tc.want)
		}
	}
}

func TestModalDialogActionStyleResolution(t *testing.T) {
	tests := []struct {
		id, label string
		explicit  modalDialogActionStyle
		want      modalDialogActionStyle
	}{
		{id: "cancel", label: "取消", want: modalDialogActionSecondary},
		{id: "close", label: "关闭", want: modalDialogActionSecondary},
		{id: "delete", label: "删除", want: modalDialogActionDanger},
		{id: "discard", label: "不保存", want: modalDialogActionDanger},
		{id: "confirm", label: "确定", want: modalDialogActionPrimary},
		{id: "save", label: "保存", want: modalDialogActionPrimary},
		{id: "replace", label: "替换", want: modalDialogActionPrimary},
		{id: "custom", label: "自定义", explicit: modalDialogActionDanger, want: modalDialogActionDanger},
		{id: "custom", label: "自定义", explicit: modalDialogActionSecondary, want: modalDialogActionSecondary},
	}

	for _, tt := range tests {
		spec := modalDialogActionSpec{ID: tt.id, Label: tt.label, Style: tt.explicit}
		got := resolveDialogActionStyle(spec)
		if got != tt.want {
			t.Errorf("resolveDialogActionStyle(%+v) = %v; want %v", spec, got, tt.want)
		}
	}
}

func TestModalConfirmStyleUsesExplicitMetadata(t *testing.T) {
	app := &App{desktopRuntime: newDesktopRuntime()}
	app.applyLocaleSelection(i18n.SelectionEnglish)
	app.modal.Show("保存更改？", "内容未保存。", nil)
	if app.modal.HasConfirmStyle {
		t.Fatal("raw dialog wording selected a confirmation style")
	}
	style := modalDialogConfirmStyle{Icon: "alert", IconBackground: color.NRGBA{R: 20, G: 30, B: 40, A: 255}}
	app.modal.ShowConfirmMessage("document.save.confirm", i18n.Message{ID: "common.dialog.confirm.title"}, i18n.Message{ID: "common.dialog.confirm.body"}, style, nil)
	if !app.modal.HasConfirmStyle || app.modal.ConfirmStyle != style {
		t.Fatalf("explicit confirmation style was not applied: %+v", app.modal)
	}
	app.applyLocaleSelection(i18n.SelectionChinese)
	if got := app.text(app.modal.TitleRef); got != "确认" || app.modal.ConfirmStyle != style {
		t.Fatalf("locale change altered title/style: title=%q style=%+v", got, app.modal.ConfirmStyle)
	}
}

func TestModalDialogUnifiedLayoutRenders(t *testing.T) {
	app := &App{theme: material.NewTheme()}
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	// 1. Confirmation dialog
	app.modal.Show("保存更改？", "当前内容尚未保存。保存后再继续，或取消并返回编辑。", nil,
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
		modalDialogActionSpec{ID: "save", Label: "保存"},
	)
	dims := app.layoutModalDialog(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("expected non-empty dims for confirmation modal, got %v", dims.Size)
	}

	// 2. Input dialog
	app.modal.ShowInput("设置字体", "输入字体名称", "Arial", "字体名称", nil,
		modalDialogActionSpec{ID: "confirm", Label: "应用"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
	dims = app.layoutModalDialog(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("expected non-empty dims for input modal, got %v", dims.Size)
	}

	// 3. Dual input dialog
	app.modal.ShowDualInput("替换", "在当前文档中查找和替换文字", "hello", "查找内容", "world", "替换为", nil,
		modalDialogActionSpec{ID: "find-next", Label: "查找下一个"},
		modalDialogActionSpec{ID: "replace", Label: "替换"},
		modalDialogActionSpec{ID: "close", Label: "关闭"},
	)
	dims = app.layoutModalDialog(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("expected non-empty dims for dual input modal, got %v", dims.Size)
	}

	// 4. Form dialog
	app.modal.ShowForm("文档信息", "编辑 DOCX 核心属性", []string{"标题", "作者"}, []string{"输入标题", "输入作者"}, []string{"Doc1", "User"}, nil,
		modalDialogActionSpec{ID: "confirm", Label: "应用"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
	dims = app.layoutModalDialog(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("expected non-empty dims for form modal, got %v", dims.Size)
	}
}

func TestPDFPreviewDialogFitsWindow(t *testing.T) {
	app := &App{theme: material.NewTheme()}
	app.pdfExportPreview = pdfExportPreviewState{Active: true, Format: "spreadsheet", Orientation: "portrait", TotalPages: 1}
	app.modal.ShowCustom("导出 PDF 预览", "设置页面方向并预览导出效果", 680, app.layoutPDFExportPreview, nil,
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
		modalDialogActionSpec{ID: "confirm", Label: "导出 PDF"},
	)
	app.modal.CustomActionsInLayout = true
	gtx := layout.Context{
		Ops: new(op.Ops), Constraints: layout.Constraints{Max: image.Pt(800, 600)},
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	dims := app.layoutConfirmDialogCard(gtx)
	if dims.Size.X > 680 || dims.Size.Y > 500 || dims.Size.Y < 300 {
		t.Fatalf("PDF preview card should fit the window, got %v", dims.Size)
	}
}

func TestModalActionRowsWrapWithinAvailableWidth(t *testing.T) {
	widths := []int{93, 88, 93, 88, 88, 80}
	rows := planDialogActionRows(widths, 392, 10)
	want := [][]int{{0, 1, 2, 3}, {4, 5}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("planDialogActionRows(%v, 392, 10) = %v; want %v", widths, rows, want)
	}
	for rowIndex, row := range rows {
		rowWidth := 0
		for index, actionIndex := range row {
			rowWidth += widths[actionIndex]
			if index > 0 {
				rowWidth += 10
			}
		}
		if rowWidth > 392 {
			t.Errorf("action row %d width = %d; exceeds available width 392", rowIndex, rowWidth)
		}
	}
}

func TestModalActionBeyondAvailableWidthIsNotClickable(t *testing.T) {
	app := &App{theme: material.NewTheme()}
	var clicked string
	app.modal.Show("条件格式", "为选区 H7:H7 选择条件格式规则：", func(id string) { clicked = id },
		modalDialogActionSpec{ID: "data_bar", Label: "渐变数据条"},
		modalDialogActionSpec{ID: "3_color", Label: "三色色阶"},
		modalDialogActionSpec{ID: "duplicate", Label: "高亮重复值"},
		modalDialogActionSpec{ID: "above_avg", Label: "高于平均值"},
		modalDialogActionSpec{ID: "clear", Label: "清除规则"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
	router := new(input.Router)
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Source: router.Source(), Ops: &ops,
			Constraints: layout.Constraints{Max: image.Pt(392, 200)},
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		}
		app.layoutConfirmDialogActions(gtx)
		router.Frame(&ops)
	}
	frame()
	point := f32.Pt(410, 18) // outside the 392px action area, over the old fifth button.
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 71, Buttons: pointer.ButtonPrimary, Position: point})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 71, Position: point})
	frame()
	if clicked != "" {
		t.Fatalf("click outside the action area triggered %q", clicked)
	}
}

func TestModalActionsStayWithinCardAtRetinaScale(t *testing.T) {
	app := &App{theme: material.NewTheme()}
	var clicked string
	app.modal.Show("条件格式", "为选区 H7:H7 选择条件格式规则：", func(id string) { clicked = id },
		modalDialogActionSpec{ID: "data_bar", Label: "渐变数据条"},
		modalDialogActionSpec{ID: "3_color", Label: "三色色阶"},
		modalDialogActionSpec{ID: "duplicate", Label: "高亮重复值"},
		modalDialogActionSpec{ID: "above_avg", Label: "高于平均值"},
		modalDialogActionSpec{ID: "clear", Label: "清除规则"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
	router := new(input.Router)
	var ops op.Ops
	const windowWidth, windowHeight = 960, 620
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Source: router.Source(), Ops: &ops,
			Constraints: layout.Exact(image.Pt(windowWidth, windowHeight)),
			Metric:      unit.Metric{PxPerDp: 2, PxPerSp: 2},
		}
		app.layoutModalDialog(gtx)
		router.Frame(&ops)
	}
	frame()
	// On the screenshot's old single-row layout, the fifth button started just
	// outside the 880px-wide card at this point. The wrapped layout keeps it in
	// the next row and leaves this outside-card position inert.
	point := f32.Pt(930, 455)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 72, Buttons: pointer.ButtonPrimary, Position: point})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 72, Position: point})
	frame()
	if clicked != "" {
		t.Fatalf("click outside the modal card triggered %q", clicked)
	}
}

func TestModalActionButtonsDoNotPaintOutsideTheDialogCard(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, tc := range []struct {
		name                 string
		width, height, scale int
	}{
		{name: "1x", width: 800, height: 600, scale: 1},
		{name: "2x screenshot geometry", width: 960, height: 900, scale: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := &App{theme: material.NewTheme()}
			var clicked string
			app.modal.Show("条件格式", "为选区 H7:H7 选择条件格式规则：", func(id string) { clicked = id },
				modalDialogActionSpec{ID: "data_bar", Label: "渐变数据条"},
				modalDialogActionSpec{ID: "3_color", Label: "三色色阶"},
				modalDialogActionSpec{ID: "duplicate", Label: "高亮重复值"},
				modalDialogActionSpec{ID: "above_avg", Label: "高于平均值"},
				modalDialogActionSpec{ID: "clear", Label: "清除规则"},
				modalDialogActionSpec{ID: "cancel", Label: "取消"},
			)
			size := image.Pt(tc.width, tc.height)
			var ops op.Ops
			metric := unit.Metric{PxPerDp: float32(tc.scale), PxPerSp: float32(tc.scale)}
			gtx := layout.Context{Ops: &ops, Now: time.Now(), Constraints: layout.Exact(size), Metric: metric}
			if got := app.layoutModalDialog(gtx).Size; got != size {
				t.Fatalf("modal root size = %v; want %v", got, size)
			}
			window, err := headless.NewWindow(tc.width, tc.height)
			if err != nil {
				t.Skipf("headless GPU is unavailable: %v", err)
			}
			defer window.Release()
			if err := window.Frame(&ops); err != nil {
				t.Fatalf("render modal: %v", err)
			}
			screenshot := image.NewRGBA(image.Rect(0, 0, tc.width, tc.height))
			if err := window.Screenshot(screenshot); err != nil {
				t.Fatalf("capture modal: %v", err)
			}
			cardWidth := min(gtx.Dp(unit.Dp(440)), tc.width-gtx.Dp(unit.Dp(32)))
			cardBounds := image.Rect((tc.width-cardWidth)/2, 0, (tc.width+cardWidth)/2, tc.height)
			primaryBlue := color.NRGBA{R: 52, G: 120, B: 209, A: 255}
			outsideButtonPixels := 0
			for y := 0; y < tc.height; y++ {
				for x := 0; x < cardBounds.Min.X; x++ {
					pixel := color.NRGBAModel.Convert(screenshot.At(x, y)).(color.NRGBA)
					if pixel == primaryBlue {
						outsideButtonPixels++
					}
				}
				for x := cardBounds.Max.X; x < tc.width; x++ {
					pixel := color.NRGBAModel.Convert(screenshot.At(x, y)).(color.NRGBA)
					if pixel == primaryBlue {
						outsideButtonPixels++
					}
				}
			}
			if outsideButtonPixels != 0 {
				t.Fatalf("found %d primary-button pixels outside dialog card bounds %v", outsideButtonPixels, cardBounds)
			}

			minButtonWidth := gtx.Dp(unit.Dp(72))
			minButtonHeight := gtx.Dp(unit.Dp(24))
			var blueButtons []image.Rectangle
			for _, bounds := range exactColorComponents(screenshot, cardBounds, primaryBlue) {
				if bounds.Dx() >= minButtonWidth && bounds.Dy() >= minButtonHeight {
					blueButtons = append(blueButtons, bounds)
				}
			}
			if len(blueButtons) != 5 {
				t.Fatalf("found %d complete primary button regions inside card; want 5: %v", len(blueButtons), blueButtons)
			}
			rowTops := make(map[int]struct{})
			for _, bounds := range blueButtons {
				if bounds.Dx() < minButtonWidth {
					t.Errorf("primary button is clipped to %dpx inside card; want at least %dpx: %v", bounds.Dx(), minButtonWidth, bounds)
				}
				rowTops[bounds.Min.Y] = struct{}{}
			}
			if len(rowTops) != 2 {
				t.Errorf("primary buttons occupy %d rows inside card; want 2: %v", len(rowTops), blueButtons)
			}

			bottomRowTop := -1
			var rightmostBottomButton image.Rectangle
			for _, bounds := range blueButtons {
				if bounds.Min.Y > bottomRowTop {
					bottomRowTop = bounds.Min.Y
					rightmostBottomButton = bounds
				} else if bounds.Min.Y == bottomRowTop && bounds.Max.X > rightmostBottomButton.Max.X {
					rightmostBottomButton = bounds
				}
			}
			if bottomRowTop >= 0 {
				router := new(input.Router)
				var clickOps op.Ops
				frame := func() {
					clickOps.Reset()
					clickGtx := layout.Context{
						Source: router.Source(), Ops: &clickOps,
						Constraints: layout.Exact(size),
						Metric:      metric,
					}
					app.layoutModalDialog(clickGtx)
					router.Frame(&clickOps)
				}
				frame()
				point := f32.Pt(
					float32(rightmostBottomButton.Max.X+gtx.Dp(unit.Dp(50))),
					float32(bottomRowTop+gtx.Dp(unit.Dp(18))),
				)
				router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 73, Buttons: pointer.ButtonPrimary, Position: point})
				frame()
				router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 73, Position: point})
				frame()
				if clicked != "cancel" {
					t.Errorf("clicking the expected in-card cancel button triggered %q; want cancel", clicked)
				}
			}
		})
	}
}

func exactColorComponents(img *image.RGBA, bounds image.Rectangle, target color.NRGBA) []image.Rectangle {
	bounds = bounds.Intersect(img.Bounds())
	if bounds.Empty() {
		return nil
	}
	visited := make([]bool, bounds.Dx()*bounds.Dy())
	indexAt := func(point image.Point) int {
		return (point.Y-bounds.Min.Y)*bounds.Dx() + point.X - bounds.Min.X
	}
	var components []image.Rectangle
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			start := image.Pt(x, y)
			if visited[indexAt(start)] || color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA) != target {
				continue
			}
			visited[indexAt(start)] = true
			queue := []image.Point{start}
			component := image.Rectangle{Min: start, Max: start.Add(image.Pt(1, 1))}
			for len(queue) > 0 {
				point := queue[len(queue)-1]
				queue = queue[:len(queue)-1]
				component.Min.X = min(component.Min.X, point.X)
				component.Min.Y = min(component.Min.Y, point.Y)
				component.Max.X = max(component.Max.X, point.X+1)
				component.Max.Y = max(component.Max.Y, point.Y+1)
				for _, next := range [...]image.Point{
					point.Add(image.Pt(-1, 0)), point.Add(image.Pt(1, 0)),
					point.Add(image.Pt(0, -1)), point.Add(image.Pt(0, 1)),
				} {
					if !next.In(bounds) || visited[indexAt(next)] || color.NRGBAModel.Convert(img.At(next.X, next.Y)).(color.NRGBA) != target {
						continue
					}
					visited[indexAt(next)] = true
					queue = append(queue, next)
				}
			}
			components = append(components, component)
		}
	}
	return components
}
