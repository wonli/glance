package desktopui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

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

func TestModalDialogDefaultConfirmStyleForTitle(t *testing.T) {
	titles := []string{
		"保存更改？",
		"恢复历史版本？",
		"字数统计",
		"替换",
		"查找",
		"设置字体",
		"调整图片大小",
		"缩放",
		"文档信息",
		"重命名工作表",
	}

	for _, title := range titles {
		style, ok := defaultConfirmStyleForTitle(title)
		if !ok || style.Icon == "" {
			t.Errorf("expected default confirm style with icon for %q", title)
		}
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
