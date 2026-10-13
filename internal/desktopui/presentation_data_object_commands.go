package desktopui

import (
	"encoding/json"
	"fmt"

	"glance/internal/application"
	"glance/internal/document"
)

type presentationChartSeries struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
}

type presentationDataObjectArguments struct {
	SlideID    uint32                    `json:"slide_id"`
	X          float64                   `json:"x"`
	Y          float64                   `json:"y"`
	Width      float64                   `json:"width"`
	Height     float64                   `json:"height"`
	Rows       [][]string                `json:"rows,omitempty"`
	FirstRow   bool                      `json:"first_row,omitempty"`
	BandRows   bool                      `json:"band_rows,omitempty"`
	ChartType  string                    `json:"chart_type,omitempty"`
	Title      string                    `json:"title,omitempty"`
	Categories []string                  `json:"categories,omitempty"`
	Series     []presentationChartSeries `json:"series,omitempty"`
}

func (a *App) showPresentationTableInsertOptions(view *presentationViewState) {
	if view == nil {
		return
	}
	a.modal.Show("插入表格", "选择空白表格尺寸。单元格数据可由 Agent 通过 MCP 填入。", func(action string) {
		a.modal.Hide()
		rows, columns := 0, 0
		switch action {
		case "2x2":
			rows, columns = 2, 2
		case "3x3":
			rows, columns = 3, 3
		case "4x4":
			rows, columns = 4, 4
		default:
			return
		}
		grid := make([][]string, rows)
		for row := range grid {
			grid[row] = make([]string, columns)
		}
		if !a.startPresentationDataObject(view, "presentation.element.table.create", presentationDataObjectArguments{
			Rows: grid, FirstRow: true, BandRows: true,
		}, false) {
			a.setStatus("无法启动表格插入")
		}
	},
		modalDialogActionSpec{ID: "2x2", Label: "2 × 2"},
		modalDialogActionSpec{ID: "3x3", Label: "3 × 3"},
		modalDialogActionSpec{ID: "4x4", Label: "4 × 4"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showPresentationChartInsertOptions(view *presentationViewState) {
	if view == nil {
		return
	}
	a.modal.Show("插入图表", "选择图表类型。模板含三个示例类别和数值 1、2、3；Agent 可按实际数据创建正式图表。", func(action string) {
		a.modal.Hide()
		if action != "bar" && action != "line" && action != "pie" {
			return
		}
		if !a.startPresentationDataObject(view, "presentation.element.chart.create", presentationDataObjectArguments{
			ChartType: action, Title: "示例数据",
			Categories: []string{"类别 1", "类别 2", "类别 3"},
			Series:     []presentationChartSeries{{Name: "系列 1", Values: []float64{1, 2, 3}}},
		}, true) {
			a.setStatus("无法启动图表插入")
		}
	},
		modalDialogActionSpec{ID: "bar", Label: "柱状图"},
		modalDialogActionSpec{ID: "line", Label: "折线图"},
		modalDialogActionSpec{ID: "pie", Label: "饼图"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) startPresentationDataObject(view *presentationViewState, capabilityID string, arguments presentationDataObjectArguments, chart bool) bool {
	state := a.documentSessionState
	if view == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if view.commandPending {
		return true
	}
	if err := a.flushPresentationNotesDraft(view); err != nil {
		return true
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return true
	}
	slide := view.currentSlide()
	if slide == nil {
		a.setStatus("未找到当前幻灯片")
		return true
	}
	bounds := presentationObjectBounds(slide)
	arguments.SlideID = slide.ID
	arguments.X = bounds.X
	arguments.Y = bounds.Y
	arguments.Width = bounds.W
	arguments.Height = bounds.H
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatus("演示文稿正在处理其他操作")
		return true
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		command, err := a.executeRibbonApplicationCommand(token, capabilityID, "presentation_slide", fmt.Sprint(slide.ID), "", arguments)
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: view, generation: generation, err: err}
		if chart {
			out.createdChart = true
		} else {
			out.createdTable = true
		}
		if err == nil {
			var data struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
			}
			if json.Unmarshal(command.Data, &data) != nil || data.SlideID != slide.ID || data.ElementID <= 0 {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.createdSlideID = data.SlideID
				out.createdElementID = data.ElementID
			}
		}
		select {
		case view.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

func presentationObjectBounds(slide *document.PresentationSlide) document.PresentationRect {
	if slide == nil {
		return document.PresentationRect{X: 96, Y: 81, W: 768, H: 378}
	}
	width, height := slide.WidthPT, slide.HeightPT
	if width <= 0 {
		width = 960
	}
	if height <= 0 {
		height = 540
	}
	return document.PresentationRect{X: width * 0.1, Y: height * 0.15, W: width * 0.8, H: height * 0.7}
}
