package desktopui

import (
	"encoding/json"
	"fmt"
	"io"

	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/widget"

	"glance/internal/document"
	"glance/internal/format/jsondoc"
)

type jsonViewMode uint8

const (
	jsonModeTree jsonViewMode = iota
	jsonModeSource
	jsonModeCompare
)

type jsonWorkspaceView struct {
	mode                                      jsonViewMode
	toolPending                               bool
	clipboardWritePending                     bool
	clipboardWriteText                        string
	toolResults                               chan jsonToolResult
	toolMessage                               string
	tree                                      *document.JSONTree
	err                                       error
	valueError                                string
	requested, parsed, sourceSynced, baseline string
	pending                                   bool
	results                                   chan jsonTreeResult
	done                                      chan struct{}
	selected                                  int
	collapsed                                 map[int]bool
	rows                                      []jsonTreeRow
	outline                                   []int
	search                                    widget.Editor
	value                                     widget.Editor
	original                                  widget.Editor
	treeList, outlineList, inspectorList      widget.List
	rowButtons, toggleButtons, outlineButtons map[int]*widget.Clickable
	clipboardTarget                           struct{}
	expandedLevel                             int
	sourceLoaded                              bool
	sourceSegments                            []jsonSourceSegment
	baselineSegments                          []jsonSourceSegment
	sourceList, baselineList                  widget.List
}

func (a *App) ensureJSONView() *jsonWorkspaceView {
	if a.jsonView == nil {
		text := a.jsonDoc.Text()
		v := &jsonWorkspaceView{selected: 0, collapsed: make(map[int]bool), results: make(chan jsonTreeResult, 1), baseline: text, sourceSynced: text, requested: text, expandedLevel: 3, rowButtons: make(map[int]*widget.Clickable), toggleButtons: make(map[int]*widget.Clickable), outlineButtons: make(map[int]*widget.Clickable)}
		v.toolResults = make(chan jsonToolResult, 1)
		v.done = a.documentSessionState.jsonWorkerSignal()
		v.search.SingleLine = true
		v.original.ReadOnly = true
		if len(text) <= jsonSourceEditLimit {
			v.original.SetText(text)
		}
		v.treeList.List.Axis, v.outlineList.List.Axis, v.inspectorList.List.Axis = layout.Vertical, layout.Vertical, layout.Vertical
		if len(text) <= jsonSourceEditLimit {
			a.jsonEditor.SetText(text)
			v.sourceLoaded = true
		}
		v.sourceList.List.Axis, v.baselineList.List.Axis = layout.Vertical, layout.Vertical
		if len(text) > jsonSourceEditLimit {
			v.expandedLevel = 1
			a.documentSessionState.jsonLarge.Store(true)
		}
		a.jsonView = v
	}
	return a.jsonView
}

func (a *App) handleJSONWorkspaceEvents(gtx layout.Context) {
	v := a.ensureJSONView()
	if a.modal.Visible {
		return
	}
	for {
		if _, ok := a.jsonEditor.Update(gtx); !ok {
			break
		}
	}
	a.commitJSONSourceDraft()
	a.pollJSONToolResults()
	if v.clipboardWritePending {
		writeJSONClipboard(gtx, v.clipboardWriteText)
		v.clipboardWriteText = ""
		v.clipboardWritePending = false
	}
	text := a.jsonDoc.Text()
	if v.sourceSynced != text {
		if len(text) <= jsonSourceEditLimit {
			a.jsonEditor.SetText(text)
			v.sourceLoaded = true
		} else if v.sourceLoaded {
			a.jsonEditor.SetText("")
			v.sourceLoaded = false
		}
		v.sourceSynced = text
	}
	v.requested = text
	select {
	case result := <-v.results:
		v.accept(result)
	default:
	}
	a.requestJSONTree()
	changed := false
	for {
		if _, ok := v.search.Update(gtx); !ok {
			break
		}
		changed = true
	}
	for {
		if _, ok := v.value.Update(gtx); !ok {
			break
		}
	}
	if changed {
		v.rebuildRows()
	}
	for id, b := range v.rowButtons {
		if b.Clicked(gtx) {
			v.selectNode(id)
		}
	}
	for id, b := range v.outlineButtons {
		if b.Clicked(gtx) {
			v.selectNode(id)
			for parent := id; parent >= 0; parent = v.tree.Nodes[parent].Parent {
				v.collapsed[parent] = false
			}
			v.rebuildRows()
			for at, row := range v.rows {
				if row.node == id && !row.closing {
					v.treeList.List.ScrollTo(at)
					break
				}
			}
		}
	}
	for id, b := range v.toggleButtons {
		if b.Clicked(gtx) {
			v.collapsed[id] = !v.collapsed[id]
			v.rebuildRows()
		}
	}
	for {
		ev, ok := gtx.Event(transfer.TargetFilter{Target: &v.clipboardTarget, Type: "application/text"})
		if !ok {
			break
		}
		if data, ok := ev.(transfer.DataEvent); ok {
			reader := data.Open()
			raw, err := io.ReadAll(io.LimitReader(reader, 8*1024*1024+1))
			_ = reader.Close()
			if err != nil {
				a.setStatus(err.Error())
			} else if len(raw) > 8*1024*1024 {
				a.setStatus("JSON 粘贴内容超过 8 MB")
			} else {
				a.applyJSONTextMutation(string(raw), "已粘贴 JSON")
				v.mode = jsonModeSource
			}
		}
	}
}

func (a *App) commitJSONSourceDraft() {
	v := a.ensureJSONView()
	if !v.sourceLoaded {
		return
	}
	updated := a.jsonEditor.Text()

	if updated == v.sourceSynced || updated == a.jsonDoc.Text() {
		return
	}
	token, ok := a.documentSessionState.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	unlock := lockSessionAdapterWrite(a.documentSessionState)
	err := a.jsonDoc.SetText(updated)
	unlock()
	a.completeSessionOperation(a.documentSessionState, token, err, err == nil)
	if err != nil {
		a.setStatus(err.Error())
		return
	}
	v.sourceSynced = updated
	if len(updated) > jsonSourceEditLimit {
		a.jsonEditor.SetText("")
		v.sourceLoaded = false
	}
	a.persistActiveSession()
}

func (a *App) commitJSONNodeValue() error {
	v := a.ensureJSONView()
	if v.tree == nil || v.tree.Source != a.jsonDoc.Text() {
		return fmt.Errorf("JSON 结构正在更新，请稍候")
	}
	n := v.tree.Nodes[v.selected]
	if v.value.ReadOnly {
		return fmt.Errorf("请展开并选择要编辑的值")
	}
	value := v.value.Text()
	original := n.Raw
	if n.Kind == "string" {
		original = n.Display
	}
	if value == original {
		return nil
	}
	if n.Kind == "string" {
		encoded, _ := json.Marshal(value)
		value = string(encoded)
	}
	text, err := jsondoc.ReplaceValue(v.tree, v.selected, value)
	if err != nil {
		return err
	}
	if text != v.tree.Source {
		if !a.applyJSONTextMutationIfSource(text, "已修改 JSON 值", v.tree.Source) {
			return fmt.Errorf("%s", a.status)
		}
	}
	return nil
}

func (a *App) handleJSONWorkspaceAction(gtx layout.Context, id string) {
	v := a.ensureJSONView()
	switch id {
	case "mode-tree":
		v.mode = jsonModeTree
	case "mode-source":
		v.mode = jsonModeSource
	case "mode-compare":
		v.mode = jsonModeCompare
	case "expand-all", "level-all", "collapse-all", "level-1", "level-2", "level-3", "expand-level":
		level := 3
		operation := "expand_level"
		switch id {
		case "expand-all", "level-all":
			level = -1
			operation = "expand_all"
		case "collapse-all":
			level = 0
			operation = "collapse_all"
		case "level-1":
			level = 1
		case "level-2":
			level = 2
		}
		if a.startJSONViewApplicationTool(v, operation, level) {
			return
		}
		v.expandedLevel = level
		v.collapsed = make(map[int]bool)
		v.rebuildRows()
	case "search":
		if state := a.documentSessionState; state != nil && strings.TrimSpace(v.search.Text()) != "" && a.applicationSessionRegistered(state.documentID) {
			a.startJSONTool("search")
		} else {
			gtx.Execute(key.FocusCmd{Tag: &v.search})
		}
	case "copy":
		if state := a.documentSessionState; state != nil && a.applicationSessionRegistered(state.documentID) {
			a.startJSONTool("copy")
		} else {
			writeJSONClipboard(gtx, a.jsonDraftText())
			a.setStatus("已复制 JSON")
		}
	case "paste":
		gtx.Execute(clipboard.ReadCmd{Tag: &v.clipboardTarget})
	case "import":
		a.startOpenFilePicker()
	case "apply-value":
		if err := a.commitJSONNodeValue(); err != nil {
			a.setStatus(err.Error())
			v.valueError = err.Error()
		} else {
			v.valueError = ""
		}
	case "cancel-value":
		v.selectNode(v.selected)
	default:
		if v.tree == nil || v.tree.Source != a.jsonDoc.Text() {
			return
		}
		n := v.tree.Nodes[v.selected]
		value := ""
		switch id {
		case "copy-value", "row-copy-value":
			value = n.Display
			if n.Container() {
				value = n.Raw
			}
		case "copy-key":
			value = n.Key
		case "copy-object":
			value = n.Raw
			if !n.Container() && n.Parent >= 0 {
				value = v.tree.Nodes[n.Parent].Raw
			}
		case "copy-dot":
			value = v.tree.NodeDotPath(v.selected)
		case "copy-path", "row-copy-path":
			value = v.tree.NodeJSONPath(v.selected)
		case "copy-pointer":
			value = v.tree.NodePointer(v.selected)
		default:
			return
		}
		writeJSONClipboard(gtx, value)
		a.setStatus("已复制到剪贴板")
	}
}

func writeJSONClipboard(gtx layout.Context, text string) {
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
}

func jsonViewButton(buttons map[int]*widget.Clickable, id int) *widget.Clickable {
	if buttons[id] == nil {
		buttons[id] = new(widget.Clickable)
	}
	return buttons[id]
}
