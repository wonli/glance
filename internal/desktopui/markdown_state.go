package desktopui

import (
	"context"
	"gioui.org/layout"
	"gioui.org/widget"
	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	"glance/internal/i18n"
	"image"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
)

type markdownParseResult struct {
	source, path     string
	generation       uint64
	resourcesPending bool
	snapshot         document.MarkdownSnapshot
	images           map[string]image.Image
	errors           map[string]string
	err              error
}
type markdownImagePickResult struct {
	path  string
	err   error
	epoch uint64
}
type markdownEditResult struct {
	state      *documentSessionState
	view       *markdownViewState
	source     string
	action     string
	data       []byte
	err        error
	generation uint64
}
type markdownViewState struct {
	tasks                                  map[string]int
	formulas                               map[string]document.MarkdownSpan
	parseCancel                            context.CancelFunc
	parseGeneration                        uint64
	imagePicks                             chan markdownImagePickResult
	dialogEpoch                            uint64
	imagePickPending                       bool
	editPending                            bool
	cachedText                             string
	sourceLines                            []int
	cachedVersion                          uint64
	cacheValid                             bool
	observedPath                           string
	draftEpoch                             atomic.Uint64
	observedDraft, parsedPath              string
	editor, search                         widget.Editor
	pageClick                              widget.Clickable
	toolbar, outline, preview              widget.List
	buttons                                map[string]*widget.Clickable
	links                                  map[string]string
	mode                                   int
	collapsed, pending                     bool
	sourceSynced, requested, requestedPath string
	snapshot                               document.MarkdownSnapshot
	images                                 map[string]image.Image
	errors                                 map[string]string
	err                                    error
	results                                chan markdownParseResult
	exports                                chan markdownFileResult
	editResults                            chan markdownEditResult
	exportPending                          bool
	exportMessage                          i18n.Message
	lastPDFPath                            string
	focus                                  bool
	draftPending                           atomic.Bool
	compositionPending                     atomic.Bool
}

func newMarkdownView(source string) *markdownViewState {
	v := &markdownViewState{sourceSynced: source, observedDraft: source, buttons: map[string]*widget.Clickable{}, links: map[string]string{}, results: make(chan markdownParseResult, 1), exports: make(chan markdownFileResult, 1), editResults: make(chan markdownEditResult, 1), toolbar: widget.List{List: layout.List{Axis: layout.Horizontal}}, outline: widget.List{List: layout.List{Axis: layout.Vertical}}, preview: widget.List{List: layout.List{Axis: layout.Vertical}}}
	v.tasks = map[string]int{}
	v.formulas = map[string]document.MarkdownSpan{}
	v.imagePicks = make(chan markdownImagePickResult, 1)
	v.editor.SetText(source)
	v.focus = true
	v.search.SingleLine = true
	return v
}
func (a *App) ensureMarkdownView() *markdownViewState {
	if a.markdownView == nil {
		a.markdownView = newMarkdownView(a.markdownDoc.Text())
	}
	return a.markdownView
}
func (v *markdownViewState) button(id string) *widget.Clickable {
	if v.buttons[id] == nil {
		v.buttons[id] = new(widget.Clickable)
	}
	return v.buttons[id]
}
func (v *markdownViewState) accept(r markdownParseResult) bool {
	if r.generation != v.parseGeneration || r.source != v.requested || r.path != v.requestedPath {
		return false
	}
	v.pending = r.resourcesPending
	if v.snapshot.Source != r.source || v.parsedPath != r.path {
		for id := range v.buttons {
			if strings.HasPrefix(id, "heading-") || strings.HasPrefix(id, "link-") || strings.HasPrefix(id, "task-") || strings.HasPrefix(id, "math-") {
				delete(v.buttons, id)
			}
		}
		v.tasks = map[string]int{}
		v.formulas = map[string]document.MarkdownSpan{}
		v.links = map[string]string{}
	}
	v.parsedPath = r.path
	v.snapshot, v.images, v.errors, v.err = r.snapshot, r.images, r.errors, r.err
	return true
}
func (v *markdownViewState) headings() []document.MarkdownHeading {
	query := strings.ToLower(v.search.Text())
	out := []document.MarkdownHeading{}
	for _, h := range v.snapshot.Headings {
		if strings.Contains(strings.ToLower(h.Title), query) {
			out = append(out, h)
		}
	}
	return out
}

// Gio 0.10.2 does not expose composition state publicly. Read its range without
// unsafe access; this compatibility shim is confined to the input boundary.
func markdownEditorComposing(e *widget.Editor) bool {
	return editorHasActiveComposition(e)
}
func markdownAssetKey(span document.MarkdownSpan) string {
	return markdowndoc.ResourceKey(span)
}

// Text extraction is O(document size); unchanged frames only inspect Gio's
// text revision. Confine the version compatibility shim to this view boundary.
func (v *markdownViewState) text() string {
	revision := reflect.ValueOf(&v.editor).Elem().FieldByName("text").FieldByName("version")
	if !revision.IsValid() || revision.Kind() != reflect.Uint64 {
		return v.editor.Text()
	}
	version := revision.Uint()
	if !v.cacheValid || version != v.cachedVersion {
		v.cachedText = v.editor.Text()
		v.cachedVersion = version
		v.cacheValid = true
		v.sourceLines = []int{0}
		runeIndex := 0
		for _, r := range v.cachedText {
			runeIndex++
			if r == '\n' {
				v.sourceLines = append(v.sourceLines, runeIndex)
			}
		}
	}
	return v.cachedText
}

func (v *markdownViewState) sourceCaret() (int, int) {
	v.text()
	caret, _ := v.editor.Selection()
	line := sort.Search(len(v.sourceLines), func(i int) bool { return v.sourceLines[i] > caret }) - 1
	if line < 0 {
		return 1, caret + 1
	}
	return line + 1, caret - v.sourceLines[line] + 1
}
