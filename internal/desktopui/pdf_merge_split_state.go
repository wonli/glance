package desktopui

import (
	"context"
	"image/color"

	"gioui.org/layout"
	"gioui.org/widget"
)

type pdfMergeSplitMode uint8

const (
	pdfMergeSplitModeMerge pdfMergeSplitMode = iota
	pdfMergeSplitModeSplit
)

type pdfMergeSplitSplitMode uint8

const (
	pdfMergeSplitSplitModeRanges pdfMergeSplitSplitMode = iota
	pdfMergeSplitSplitModeByPage
)

type pdfMergeSplitFile struct {
	path      string
	name      string
	pageCount int
	sizeBytes int64
	identity  uint64
}

type pdfMergeSplitMergeDraft struct {
	files          []pdfMergeSplitFile
	outputName     widget.Editor
	outputFolder   string
	fileList       widget.List
	addButton      widget.Clickable
	clearButton    widget.Clickable
	outputButton   widget.Clickable
	mergeButton    widget.Clickable
	fileButtons    map[uint64]*pdfMergeSplitFileButtons
	nextIdentity   uint64
	totalPageCount int
	totalSizeBytes int64
}

type pdfMergeSplitFileButtons struct {
	remove widget.Clickable
	grab   widget.Clickable
}

type pdfMergeSplitSplitDraft struct {
	source         *pdfMergeSplitFile
	outputFolder   string
	prefix         widget.Editor
	rangeInput     widget.Editor
	rangeMode      pdfMergeSplitSplitMode
	selectButton   widget.Clickable
	outputButton   widget.Clickable
	rangesButton   widget.Clickable
	byPageButton   widget.Clickable
	splitButton    widget.Clickable
	rangeCount     int
	rangeError     string
	pageCountLabel int
}

type pdfMergeSplitToolState struct {
	mode            pdfMergeSplitMode
	mergeDraft      pdfMergeSplitMergeDraft
	splitDraft      pdfMergeSplitSplitDraft
	backButton      widget.Clickable
	mergeModeButton widget.Clickable
	splitModeButton widget.Clickable
	pageList        widget.List
	busy            bool
	generation      uint64
	cancel          context.CancelFunc
	lastError       string
	lastOutputs     []string
}

var pdfMergeSplitAccent = color.NRGBA{R: 222, G: 111, B: 48, A: 255}

func newPDFMergeSplitToolState() pdfMergeSplitToolState {
	state := pdfMergeSplitToolState{
		mode: pdfMergeSplitModeMerge,
		mergeDraft: pdfMergeSplitMergeDraft{
			fileList:    widget.List{List: layout.List{Axis: layout.Vertical}},
			fileButtons: make(map[uint64]*pdfMergeSplitFileButtons),
		},
		splitDraft: pdfMergeSplitSplitDraft{rangeMode: pdfMergeSplitSplitModeRanges},
		pageList:   widget.List{List: layout.List{Axis: layout.Vertical}},
	}
	state.mergeDraft.outputName.SingleLine = true
	state.mergeDraft.outputName.SetText("merged.pdf")
	state.splitDraft.prefix.SingleLine = true
	state.splitDraft.prefix.SetText("pages")
	state.splitDraft.rangeInput.SingleLine = true
	return state
}

func (s *pdfMergeSplitToolState) selectMode(mode pdfMergeSplitMode) {
	if s == nil || s.busy {
		return
	}
	s.mode = mode
}

func (s *pdfMergeSplitToolState) clearOperationResult() {
	s.lastError = ""
	s.lastOutputs = nil
}
