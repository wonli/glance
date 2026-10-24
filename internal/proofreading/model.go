// Package proofreading owns editable copy content independently of file formats and UI.
package proofreading

import (
	"errors"
	"slices"
	"unicode/utf8"
)

type CharacterID uint64
type PageID uint64
type Run struct {
	First     CharacterID `json:"first"`
	Text      string      `json:"text"`
	Page      PageID      `json:"page"`
	Protected bool        `json:"protected,omitempty"`
	Typed     bool        `json:"typed,omitempty"`
	Origin    PageID      `json:"origin,omitempty"`
}
type Page struct {
	ID     PageID `json:"id"`
	Locked bool   `json:"locked,omitempty"`
	// Manual preserves an explicitly adjusted boundary without a user lock mode.
	Manual   bool `json:"manual,omitempty"`
	Reviewed bool `json:"reviewed,omitempty"`
}

func (p Page) Independent() bool { return p.Locked || p.Manual || p.Reviewed }

type Snapshot struct {
	Version  int         `json:"version"`
	Revision uint64      `json:"revision"`
	Next     CharacterID `json:"next"`
	Runs     []Run       `json:"runs"`
	Pages    []Page      `json:"pages"`
}

// box stores one fixed page's state and its immutable text root.
type box struct {
	page Page
	root *pieceTree
}
type boxChange struct {
	index         int
	before, after box
}
type modelEdit struct {
	changes               []boxChange
	beforeNext, afterNext CharacterID
}
type Model struct {
	boxes            []box
	pageIndex        map[PageID]int
	next             CharacterID
	revision         uint64
	undo, redo       []modelEdit
	boundaryAffected []PageID
}

func New(text string, pages []PageID) (*Model, error) {
	s := Snapshot{Version: 2, Next: CharacterID(utf8.RuneCountInString(text)) + 1}
	for _, id := range pages {
		s.Pages = append(s.Pages, Page{ID: id})
	}
	if text != "" && len(pages) > 0 {
		s.Runs = []Run{{First: 1, Text: text, Page: pages[0]}}
	}
	return Restore(s)
}

// Capture shares immutable pieces and copies only the fixed box reference table.
// Workers can expand it after later UI edits without reading mutable Model state.
type View struct {
	boxes    []box
	revision uint64
	next     CharacterID
}

func (m *Model) Capture() View {
	return View{boxes: append([]box(nil), m.boxes...), revision: m.revision, next: m.next}
}
func (v View) Pages() []Page {
	pages := make([]Page, len(v.boxes))
	for i, b := range v.boxes {
		pages[i] = b.page
	}
	return pages
}
func (v View) Snapshot() Snapshot   { return snapshotBoxes(v.boxes, v.revision, v.next) }
func (m *Model) Snapshot() Snapshot { return snapshotBoxes(m.boxes, m.revision, m.next) }
func snapshotBoxes(boxes []box, revision uint64, next CharacterID) Snapshot {
	s := Snapshot{Version: 2, Revision: revision, Next: next}
	for _, b := range boxes {
		s.Pages = append(s.Pages, b.page)
		pieceVisit(b.root, func(r Run) { r.Page = b.page.ID; s.Runs = append(s.Runs, r) })
	}
	return s
}
func (m *Model) Revision() uint64 { return m.revision }
func (m *Model) index(id PageID) int {
	if i, ok := m.pageIndex[id]; ok {
		return i
	}
	return -1
}
func (m *Model) Page(id PageID) (Page, bool) {
	i := m.index(id)
	if i < 0 {
		return Page{}, false
	}
	return m.boxes[i].page, true
}
func (m *Model) NextPage(id PageID) (Page, bool) {
	i := m.index(id)
	if i < 0 || i+1 >= len(m.boxes) {
		return Page{}, false
	}
	return m.boxes[i+1].page, true
}
func (m *Model) Text(id PageID) string {
	i := m.index(id)
	if i < 0 {
		return ""
	}
	return pieceString(m.boxes[i].root)
}
func (m *Model) RuneCount(id PageID) int {
	i := m.index(id)
	if i < 0 {
		return 0
	}
	return pieceLen(m.boxes[i].root)
}
func (m *Model) IDs(id PageID) []CharacterID {
	i := m.index(id)
	if i < 0 {
		return nil
	}
	var ids []CharacterID
	pieceVisit(m.boxes[i].root, func(r Run) {
		n := utf8.RuneCountInString(r.Text)
		for j := 0; j < n; j++ {
			ids = append(ids, r.First+CharacterID(j))
		}
	})
	return ids
}
func (m *Model) commit(changes []boxChange, next CharacterID) {
	filtered := changes[:0]
	for _, c := range changes {
		if c.before != c.after {
			filtered = append(filtered, c)
		}
	}
	changes = filtered
	if len(changes) == 0 {
		return
	}
	m.undo = append(m.undo, modelEdit{changes: changes, beforeNext: m.next, afterNext: next})
	m.redo = nil
	for _, c := range changes {
		m.boxes[c.index] = c.after
	}
	m.next = next
	m.revision++
}
func (m *Model) Undo() bool {
	if len(m.undo) == 0 {
		return false
	}
	e := m.undo[len(m.undo)-1]
	m.undo[len(m.undo)-1] = modelEdit{}
	m.undo = m.undo[:len(m.undo)-1]
	for _, c := range e.changes {
		m.boxes[c.index] = c.before
	}
	m.next = e.beforeNext
	m.revision++
	m.redo = append(m.redo, e)
	return true
}
func (m *Model) Redo() bool {
	if len(m.redo) == 0 {
		return false
	}
	e := m.redo[len(m.redo)-1]
	m.redo[len(m.redo)-1] = modelEdit{}
	m.redo = m.redo[:len(m.redo)-1]
	for _, c := range e.changes {
		m.boxes[c.index] = c.after
	}
	m.next = e.afterNext
	m.revision++
	m.undo = append(m.undo, e)
	return true
}
func (m *Model) CanUndo() bool   { return len(m.undo) > 0 }
func (m *Model) CanRedo() bool   { return len(m.redo) > 0 }
func (m *Model) Validate() error { l := &legacyModel{s: m.Snapshot()}; return l.Validate() }

// legacy converts only at the obsolete API boundary, never on the fine-scroll path.
func (m *Model) legacy(f func(*legacyModel) error, history bool) error {
	before := m.Snapshot()
	l := &legacyModel{s: clone(before)}
	if err := f(l); err != nil {
		return err
	}
	if slices.Equal(before.Runs, l.s.Runs) && slices.Equal(before.Pages, l.s.Pages) {
		return nil
	}
	next, err := Restore(l.s)
	if err != nil {
		return err
	}
	var changes []boxChange
	for i, b := range next.boxes {
		changes = append(changes, boxChange{i, m.boxes[i], b})
	}
	if history {
		m.commit(changes, next.next)
	} else {
		m.boxes = next.boxes
		m.next = next.next
	}
	return nil
}
func (m *Model) AssignInitial(ends []int) error {
	if len(m.undo) > 0 {
		return errors.New("invalid initial assignment")
	}
	return m.legacy(func(l *legacyModel) error { return l.AssignInitial(ends) }, false)
}

// PageSnapshot exposes only one box for text anchors and visible editors.
func (m *Model) PageSnapshot(id PageID) Snapshot {
	i := m.index(id)
	if i < 0 {
		return Snapshot{Version: 2, Revision: m.revision, Next: m.next}
	}
	return snapshotBoxes(m.boxes[i:i+1], m.revision, m.next)
}

// ContentToken is an opaque comparable identity for one immutable box root.
// It lets derived caches detect edits without flattening text or the book.
type ContentToken struct{ root *pieceTree }

func (m *Model) ContentToken(id PageID) ContentToken {
	i := m.index(id)
	if i < 0 {
		return ContentToken{}
	}
	return ContentToken{root: m.boxes[i].root}
}

func (m *Model) PreviousPage(id PageID) (Page, bool) {
	i := m.index(id)
	if i <= 0 {
		return Page{}, false
	}
	return m.boxes[i-1].page, true
}
