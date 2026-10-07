package desktopui

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
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

type jsonTreeRow struct {
	node    int
	closing bool
}
type jsonTreeResult struct {
	baselineSegments []jsonSourceSegment
	segments         []jsonSourceSegment
	text             string
	tree             *document.JSONTree
	err              error
}

type jsonWorkspaceView struct {
	mode                                      jsonViewMode
	toolPending                               bool
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

func (v *jsonWorkspaceView) isCollapsed(id int) bool {
	if state, ok := v.collapsed[id]; ok {
		return state
	}
	if v.tree == nil || id < 0 || id >= len(v.tree.Nodes) {
		return false
	}
	if v.expandedLevel < 0 {
		return false
	}
	return v.tree.Nodes[id].Depth >= v.expandedLevel
}

func (v *jsonWorkspaceView) accept(result jsonTreeResult) {
	v.pending = false
	if result.text != v.requested {
		return
	}
	pointer := ""
	collapsed := make(map[string]bool)
	if v.tree != nil {
		if v.selected >= 0 && v.selected < len(v.tree.Nodes) {
			pointer = v.tree.NodePointer(v.selected)
		}
		if len(v.collapsed) < 1000 {
			for id, closed := range v.collapsed {
				if id >= 0 && id < len(v.tree.Nodes) {
					collapsed[v.tree.NodePointer(id)] = closed
				}
			}
		}
	}
	v.tree, v.err, v.parsed = result.tree, result.err, result.text
	v.sourceSegments = result.segments
	if result.baselineSegments != nil {
		v.baselineSegments = result.baselineSegments
	}
	v.collapsed = make(map[int]bool)
	v.rowButtons, v.toggleButtons, v.outlineButtons = make(map[int]*widget.Clickable), make(map[int]*widget.Clickable), make(map[int]*widget.Clickable)
	selected := 0
	if v.tree != nil {
		if pointer != "" {
			selected = v.tree.FindNodeByPointer(pointer)
		}
		for p, closed := range collapsed {
			target := v.tree.FindNodeByPointer(p)
			if target >= 0 && target < len(v.tree.Nodes) {
				v.collapsed[target] = closed
			}
		}
		v.selectNode(selected)
	}
	v.rebuildRows()
}

func (v *jsonWorkspaceView) selectNode(id int) {
	if v.tree == nil || id < 0 || id >= len(v.tree.Nodes) {
		return
	}
	v.selected = id
	v.valueError = ""
	n := v.tree.Nodes[id]
	value := n.Raw
	if n.Kind == "string" {
		value = n.Display
	}
	v.value.ReadOnly = n.Container() || len(value) > jsonValueEditLimit
	if n.Container() {
		value = fmt.Sprintf("{ %d keys }", v.tree.ChildCount(id))
		if n.Kind == "array" {
			value = fmt.Sprintf("[ %d items ]", v.tree.ChildCount(id))
		}
	} else if len(value) > jsonValueEditLimit {
		value = jsonTextPreview(value, 4096)
	}
	v.value.SetText(value)
}

func (v *jsonWorkspaceView) rebuildRows() {
	v.rows, v.outline = nil, nil
	if v.tree == nil {
		return
	}
	query := strings.ToLower(strings.TrimSpace(v.search.Text()))
	var matches []bool
	if query != "" {
		matches = make([]bool, len(v.tree.Nodes))
		for id, n := range v.tree.Nodes {
			if strings.Contains(strings.ToLower(n.Key), query) || (!n.Container() && strings.Contains(strings.ToLower(n.Display), query)) {
				for at := id; at >= 0 && !matches[at]; at = v.tree.Nodes[at].Parent {
					matches[at] = true
				}
			}
		}
	}
	var walk func(int)
	walk = func(id int) {
		n := v.tree.Nodes[id]
		if query != "" && !matches[id] {
			return
		}
		v.rows = append(v.rows, jsonTreeRow{node: id})
		if n.Container() {
			if !v.isCollapsed(id) || query != "" {
				for child := n.FirstChild; child >= 0; child = v.tree.Nodes[child].NextSibling {
					walk(child)
				}
				if v.tree.ChildCount(id) > 0 {
					v.rows = append(v.rows, jsonTreeRow{node: id, closing: true})
				}
			}
		}
	}
	walk(0)
	maxOutline := 1000
	for id, n := range v.tree.Nodes {
		if n.Container() && (query == "" || matches[id]) {
			if len(v.tree.Nodes) > 10000 && n.Depth > 2 {
				continue
			}
			v.outline = append(v.outline, id)
			if len(v.outline) >= maxOutline {
				break
			}
		}
	}
}

func (a *App) requestJSONTree() {
	v := a.ensureJSONView()
	text := a.jsonDoc.Text()
	v.requested = text
	if v.pending || (v.parsed == text && (v.tree != nil || v.err != nil)) {
		return
	}
	v.pending = true
	if len(text) > jsonSourceEditLimit {
		a.documentSessionState.jsonLarge.Store(true)
	}
	results, window, baseline, done := v.results, a.window, v.baseline, v.done
	go func() {
		defer releaseClosedJSONWorkerMemory(done, len(text) > jsonSourceEditLimit)
		tree, err := jsondoc.ParseTree(text)
		segments := jsonSourceSegments(text)
		baselineSegments := segments
		if baseline != text {
			baselineSegments = jsonSourceSegments(baseline)
		}
		if len(text) > 512*1024 {
			debug.FreeOSMemory()
		}
		select {
		case <-done:
			return
		default:
		}
		select {
		case results <- jsonTreeResult{text: text, tree: tree, err: err, segments: segments, baselineSegments: baselineSegments}:
		case <-done:
			return
		}
		if window != nil {
			window.Invalidate()
		}
	}()
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
	// A revision from another window takes precedence over an unchanged local draft.
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
		switch id {
		case "expand-all", "level-all":
			level = -1
		case "collapse-all":
			level = 0
		case "level-1":
			level = 1
		case "level-2":
			level = 2
		}
		v.expandedLevel = level
		v.collapsed = make(map[int]bool)
		v.rebuildRows()
	case "search":
		gtx.Execute(key.FocusCmd{Tag: &v.search})
	case "copy":
		writeJSONClipboard(gtx, a.jsonDraftText())
		a.setStatus("已复制 JSON")
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
