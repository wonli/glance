package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/i18n"
	"strconv"
	"strings"
)

func (a *App) runSpreadsheetPageLayoutCommand(operation string, arguments any, edit func(document.SpreadsheetPageLayoutEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	sheet, generation, window := state.sheet, a.documentGeneration, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.page_layout.set", "spreadsheet_sheet", sheet, sheet, arguments)
			err = commandErr
			if err == nil {
				var outcome struct {
					Changed bool `json:"changed"`
				}
				err = json.Unmarshal(result.Data, &outcome)
				changed = outcome.Changed
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			err = edit(editor)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	})
}

type showSpreadsheetOrientationDialogArguments struct {
	Sheet       string `json:"sheet"`
	Operation   string `json:"operation"`
	Orientation string `json:"orientation"`
}

func (a *App) showSpreadsheetOrientationDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	currentOrient := "portrait"
	if layout, err := editor.GetSheetPageLayout(state.sheet); err == nil && layout.Orientation != "" {
		currentOrient = layout.Orientation
	}
	label := a.text(i18n.Message{ID: "spreadsheet.page_layout.orientation.portrait"})
	if currentOrient == "landscape" {
		label = a.text(i18n.Message{ID: "spreadsheet.page_layout.orientation.landscape"})
	}
	sheet := state.sheet
	a.modal.ShowMessage("spreadsheet.page_layout.orientation", i18n.Message{ID: "spreadsheet.page_layout.orientation.dialog.title"}, i18n.Message{ID: "spreadsheet.page_layout.orientation.dialog.body", Args: i18n.Args{"Orientation": label}}, func(action string) {
		a.modal.Hide()
		if action == "portrait" || action == "landscape" {
			a.runSpreadsheetPageLayoutCommand("修改纸张方向", showSpreadsheetOrientationDialogArguments{Sheet: sheet, Operation: "orientation", Orientation: action}, func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPageOrientation(sheet, action)
			})
		}
	},
		modalDialogActionSpec{ID: "portrait", LabelRef: i18n.Message{ID: "spreadsheet.page_layout.orientation.portrait"}},
		modalDialogActionSpec{ID: "landscape", LabelRef: i18n.Message{ID: "spreadsheet.page_layout.orientation.landscape"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type showSpreadsheetMarginsDialogArguments struct {
	Sheet        string  `json:"sheet"`
	Operation    string  `json:"operation"`
	MarginTop    float64 `json:"margin_top"`
	MarginRight  float64 `json:"margin_right"`
	MarginBottom float64 `json:"margin_bottom"`
	MarginLeft   float64 `json:"margin_left"`
}

func (a *App) showSpreadsheetMarginsDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	top, right, bottom, left := 0.75, 0.7, 0.75, 0.7
	if layout, err := editor.GetSheetPageLayout(state.sheet); err == nil {
		if layout.MarginTop > 0 {
			top = layout.MarginTop
		}
		if layout.MarginRight > 0 {
			right = layout.MarginRight
		}
		if layout.MarginBottom > 0 {
			bottom = layout.MarginBottom
		}
		if layout.MarginLeft > 0 {
			left = layout.MarginLeft
		}
	}
	labels := []i18n.Message{{ID: "spreadsheet.page_layout.margin.top"}, {ID: "spreadsheet.page_layout.margin.right"}, {ID: "spreadsheet.page_layout.margin.bottom"}, {ID: "spreadsheet.page_layout.margin.left"}}
	hints := []i18n.Message{{ID: "spreadsheet.page_layout.margin.default_top"}, {ID: "spreadsheet.page_layout.margin.default_right"}, {ID: "spreadsheet.page_layout.margin.default_bottom"}, {ID: "spreadsheet.page_layout.margin.default_left"}}
	initial := []string{
		strconv.FormatFloat(top, 'f', -1, 64),
		strconv.FormatFloat(right, 'f', -1, 64),
		strconv.FormatFloat(bottom, 'f', -1, 64),
		strconv.FormatFloat(left, 'f', -1, 64),
	}
	sheet := state.sheet
	a.modal.ShowFormMessage("spreadsheet.page_layout.margins", i18n.Message{ID: "spreadsheet.page_layout.margins.dialog.title"}, i18n.Message{ID: "spreadsheet.page_layout.margins.dialog.body"},
		labels, hints, initial,
		func(action string) {
			switch action {
			case "normal":
				a.modal.Hide()
				a.runSpreadsheetPageLayoutCommand("设置普通页边距", showSpreadsheetMarginsDialogArguments{Sheet: sheet, Operation: "margins", MarginTop: 0.75, MarginRight: 0.7, MarginBottom: 0.75, MarginLeft: 0.7}, func(ed document.SpreadsheetPageLayoutEditor) error {
					return ed.SetSheetPageMargins(sheet, 0.75, 0.7, 0.75, 0.7)
				})
			case "wide":
				a.modal.Hide()
				a.runSpreadsheetPageLayoutCommand("设置宽页边距", showSpreadsheetMarginsDialogArguments{Sheet: sheet, Operation: "margins", MarginTop: 1.0, MarginRight: 1.0, MarginBottom: 1.0, MarginLeft: 1.0}, func(ed document.SpreadsheetPageLayoutEditor) error {
					return ed.SetSheetPageMargins(sheet, 1.0, 1.0, 1.0, 1.0)
				})
			case "narrow":
				a.modal.Hide()
				a.runSpreadsheetPageLayoutCommand("设置窄页边距", showSpreadsheetMarginsDialogArguments{Sheet: sheet, Operation: "margins", MarginTop: 0.75, MarginRight: 0.25, MarginBottom: 0.75, MarginLeft: 0.25}, func(ed document.SpreadsheetPageLayoutEditor) error {
					return ed.SetSheetPageMargins(sheet, 0.75, 0.25, 0.75, 0.25)
				})
			case "confirm":
				tVal, err1 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[0].Text()), 64)
				rVal, err2 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[1].Text()), 64)
				bVal, err3 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[2].Text()), 64)
				lVal, err4 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[3].Text()), 64)
				if err1 != nil || err2 != nil || err3 != nil || err4 != nil ||
					tVal < 0 || rVal < 0 || bVal < 0 || lVal < 0 ||
					tVal > 20 || rVal > 20 || bVal > 20 || lVal > 20 {
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.margins_must_be_between_0_71fe5b"}, feedbackError, notificationToast)
					if a.window != nil {
						a.window.Invalidate()
					}
					return
				}
				a.modal.Hide()
				a.runSpreadsheetPageLayoutCommand("设置页边距", showSpreadsheetMarginsDialogArguments{Sheet: sheet, Operation: "margins", MarginTop: tVal, MarginRight: rVal, MarginBottom: bVal, MarginLeft: lVal}, func(ed document.SpreadsheetPageLayoutEditor) error {
					return ed.SetSheetPageMargins(sheet, tVal, rVal, bVal, lVal)
				})
			case "cancel":
				a.modal.Hide()
			}
		},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.apply"}},
		modalDialogActionSpec{ID: "normal", LabelRef: i18n.Message{ID: "spreadsheet.page_layout.margins.preset.normal"}},
		modalDialogActionSpec{ID: "narrow", LabelRef: i18n.Message{ID: "spreadsheet.page_layout.margins.preset.narrow"}},
		modalDialogActionSpec{ID: "wide", LabelRef: i18n.Message{ID: "spreadsheet.page_layout.margins.preset.wide"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type spreadsheetpageSheetOperationArguments struct {
	Sheet     string `json:"sheet"`
	Operation string `json:"operation"`
	PaperSize int    `json:"paper_size"`
}

func (a *App) showSpreadsheetPageSizeDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	currentSize := 9
	if layout, err := editor.GetSheetPageLayout(state.sheet); err == nil && layout.PaperSize > 0 {
		currentSize = layout.PaperSize
	}
	sizeName := a.text(i18n.Message{ID: "spreadsheet.page_layout.paper_size.a4"})
	if currentSize == 1 {
		sizeName = a.text(i18n.Message{ID: "spreadsheet.page_layout.paper_size.letter"})
	} else if currentSize == 8 {
		sizeName = a.text(i18n.Message{ID: "spreadsheet.page_layout.paper_size.a3"})
	}
	sheet := state.sheet
	a.modal.ShowMessage("spreadsheet.page_layout.paper_size", i18n.Message{ID: "spreadsheet.page_layout.paper_size.dialog.title"}, i18n.Message{ID: "spreadsheet.page_layout.paper_size.dialog.body", Args: i18n.Args{"Size": sizeName}}, func(action string) {
		a.modal.Hide()
		var paperSize int
		switch action {
		case "a4":
			paperSize = 9
		case "letter":
			paperSize = 1
		case "a3":
			paperSize = 8
		default:
			return
		}
		a.runSpreadsheetPageLayoutCommand("设置纸张大小", spreadsheetpageSheetOperationArguments{Sheet: sheet, Operation: "paper_size", PaperSize: paperSize}, func(ed document.SpreadsheetPageLayoutEditor) error {
			return ed.SetSheetPageSize(sheet, paperSize)
		})
	},
		modalDialogActionSpec{ID: "a4", LabelRef: i18n.Message{ID: "spreadsheet.page_layout.paper_size.a4"}},
		modalDialogActionSpec{ID: "letter", LabelRef: i18n.Message{ID: "spreadsheet.page_layout.paper_size.letter"}},
		modalDialogActionSpec{ID: "a3", LabelRef: i18n.Message{ID: "spreadsheet.page_layout.paper_size.a3"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type spreadsheetpageSheetOperationPrintAreaArguments struct {
	Sheet     string `json:"sheet"`
	Operation string `json:"operation"`
	PrintArea string `json:"print_area"`
}

func (a *App) showSpreadsheetPrintAreaDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	sheet := state.sheet
	layout, _ := editor.GetSheetPageLayout(sheet)
	var selRef string
	if state.selectionSet {
		bounds := state.selection.Bounds()
		selRef = fmt.Sprintf("$%s$%d:$%s$%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	}

	message := i18n.Message{ID: "spreadsheet.page_layout.print_area.dialog.body.empty"}
	if layout.PrintArea != "" {
		message = i18n.Message{ID: "spreadsheet.page_layout.print_area.dialog.body.set", Args: i18n.Args{"Range": layout.PrintArea}}
	}

	actions := make([]modalDialogActionSpec, 0, 3)
	if selRef != "" {
		actions = append(actions, modalDialogActionSpec{ID: "set", LabelRef: i18n.Message{ID: "spreadsheet.page_layout.print_area.action.set", Args: i18n.Args{"Range": selRef}}})
	}
	if layout.PrintArea != "" {
		actions = append(actions, modalDialogActionSpec{ID: "clear", LabelRef: i18n.Message{ID: "spreadsheet.page_layout.print_area.action.clear"}})
	}
	actions = append(actions, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})

	a.modal.ShowMessage("spreadsheet.page_layout.print_area", i18n.Message{ID: "spreadsheet.page_layout.print_area.dialog.title"}, message, func(action string) {
		a.modal.Hide()
		switch action {
		case "set":
			if selRef == "" {
				return
			}
			a.runSpreadsheetPageLayoutCommand("设置打印区域", spreadsheetpageSheetOperationPrintAreaArguments{Sheet: sheet, Operation: "print_area", PrintArea: selRef}, func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPrintArea(sheet, selRef)
			})
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.print_area_set", Args: i18n.Args{"Range": selRef}}, feedbackSuccess, notificationToast)
		case "clear":
			a.runSpreadsheetPageLayoutCommand("清除打印区域", spreadsheetpageSheetOperationPrintAreaArguments{Sheet: sheet, Operation: "print_area", PrintArea: ""}, func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPrintArea(sheet, "")
			})
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.print_area_cleared_047ba9"}, feedbackSuccess, notificationToast)
		}
	}, actions...)
}

type spreadsheetpageSheetOperationPrintTitlesArguments struct {
	Sheet       string `json:"sheet"`
	Operation   string `json:"operation"`
	PrintTitles string `json:"print_titles"`
}

func (a *App) showSpreadsheetPrintTitlesDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPageLayoutEditor)
	if !ok {
		return
	}
	sheet := state.sheet
	layout, _ := editor.GetSheetPageLayout(sheet)
	var selRowsRef string
	if state.selectionSet {
		bounds := state.selection.Bounds()
		selRowsRef = fmt.Sprintf("$%d:$%d", bounds.StartRow, bounds.EndRow)
	}

	message := i18n.Message{ID: "spreadsheet.page_layout.print_titles.dialog.body.empty"}
	if layout.PrintTitles != "" {
		message = i18n.Message{ID: "spreadsheet.page_layout.print_titles.dialog.body.set", Args: i18n.Args{"Rows": layout.PrintTitles}}
	}

	actions := make([]modalDialogActionSpec, 0, 3)
	if selRowsRef != "" {
		actions = append(actions, modalDialogActionSpec{ID: "set", LabelRef: i18n.Message{ID: "spreadsheet.page_layout.print_titles.action.set", Args: i18n.Args{"Rows": selRowsRef}}})
	}
	if layout.PrintTitles != "" {
		actions = append(actions, modalDialogActionSpec{ID: "clear", LabelRef: i18n.Message{ID: "spreadsheet.page_layout.print_titles.action.clear"}})
	}
	actions = append(actions, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})

	a.modal.ShowMessage("spreadsheet.page_layout.print_titles", i18n.Message{ID: "spreadsheet.page_layout.print_titles.dialog.title"}, message, func(action string) {
		a.modal.Hide()
		switch action {
		case "set":
			if selRowsRef == "" {
				return
			}
			a.runSpreadsheetPageLayoutCommand("设置打印标题行", spreadsheetpageSheetOperationPrintTitlesArguments{Sheet: sheet, Operation: "print_titles", PrintTitles: selRowsRef}, func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPrintTitles(sheet, selRowsRef)
			})
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.print_title_rows_set", Args: i18n.Args{"Rows": selRowsRef}}, feedbackSuccess, notificationToast)
		case "clear":
			a.runSpreadsheetPageLayoutCommand("清除打印标题行", spreadsheetpageSheetOperationPrintTitlesArguments{Sheet: sheet, Operation: "print_titles", PrintTitles: ""}, func(ed document.SpreadsheetPageLayoutEditor) error {
				return ed.SetSheetPrintTitles(sheet, "")
			})
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.print_title_rows_cleared_20e250"}, feedbackSuccess, notificationToast)
		}
	}, actions...)
}
