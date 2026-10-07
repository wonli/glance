// Package pdfdoc owns the single authoritative PDF package for viewing/editing.
// A mutation is published only after serialization and reopening succeed.
package pdfdoc

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/SalvioniDigitalSolutions/gopdf"
	"glance/internal/document"
	"math"
	"os"
	"sync"
	"sync/atomic"
)

type Document struct {
	published           atomic.Value
	modified            bool
	mu                  sync.Mutex
	data, saved         []byte
	reader              *gopdf.Reader
	path                string
	revision            uint64
	undo, redo          [][]byte
	closed, forcedDirty bool
}

// NewBlank creates an unsaved blank single-page PDF document.
func NewBlank() (*Document, error) {
	doc := gopdf.New()
	_ = doc.AddPage()
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		return nil, err
	}
	d, err := OpenSnapshot(buf.Bytes())
	if err != nil {
		return nil, err
	}
	d.SetDirty(true)
	return d, nil
}

func Open(path string) (*Document, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	d, e := OpenSnapshot(b)
	if e == nil {
		d.path = path
	}
	return d, e
}
func OpenSnapshot(b []byte) (*Document, error) {
	b = bytes.Clone(b)
	r, e := gopdf.NewReader(b)
	if e != nil {
		return nil, e
	}
	if r.IsEncrypted() {
		return nil, errors.New("暂不支持加密 PDF，请先提供未加密副本")
	}
	if r.Repaired() {
		return nil, errors.New("PDF 需要修复，暂不支持安全编辑")
	}
	if r.NumPages() == 0 {
		return nil, errors.New("PDF 没有页面")
	}
	d := &Document{data: b, saved: bytes.Clone(b), reader: r, revision: 1}
	d.publishInfo()
	return d, nil
}
func (d *Document) Snapshot() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, errors.New("PDF 已关闭")
	}
	return bytes.Clone(d.data), nil
}
func (d *Document) Info() document.PDFInfo { return d.published.Load().(document.PDFInfo) }
func (d *Document) publishInfo() {
	d.published.Store(document.PDFInfo{Pages: d.reader.NumPages(), Bytes: len(d.data), Revision: d.revision, Dirty: d.dirty(), CanUndo: len(d.undo) > 0, CanRedo: len(d.redo) > 0})
}
func (d *Document) dirty() bool { return d.forcedDirty || d.modified }
func (d *Document) Dirty() bool { return d.Info().Dirty }
func (d *Document) SetDirty(v bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	defer d.publishInfo()
	d.forcedDirty = v
	if !v {
		d.saved = bytes.Clone(d.data)
		d.modified = false
	}
}
func (d *Document) SetSourcePath(p string) { d.mu.Lock(); defer d.mu.Unlock(); d.path = p }
func (d *Document) Close() error           { d.mu.Lock(); defer d.mu.Unlock(); d.closed = true; return nil }
func (d *Document) Page(index int) (document.PDFPage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.page(index)
}
func (d *Document) page(index int) (document.PDFPage, error) {
	size, e := d.reader.PageSize(index)
	if e != nil {
		return document.PDFPage{}, e
	}
	p := document.PDFPage{Index: index, Revision: d.revision, Width: size.W, Height: size.H, Rotation: rotation(d.reader, index)}
	u := gopdf.Update(d.reader)
	page, e := u.Page(index)
	if e != nil {
		return p, e
	}
	frags, e := d.reader.PageTextFragments(index)
	if e != nil {
		return p, e
	}
	for i, r := range page.Runs() {
		editable := !d.reader.HasSignatures() && safeTextResources(d.reader, index) && i < len(frags) && !frags[i].Invisible() && r.Text == frags[i].Text
		// Outlined/invisible and transformed text remains selectable, never guessed editable.
		box := document.PDFRect{X: r.X, Y: r.Y - r.FontSize, W: r.Width, H: r.FontSize * 1.25}
		p.Text = append(p.Text, document.PDFText{Run: i, Text: r.Text, Bounds: displayRect(box, p), Editable: editable && p.Rotation == 0})
	}
	anns, e := d.reader.Annotations(index)
	if e != nil {
		return p, e
	}
	for i, a := range anns {
		p.Annotations = append(p.Annotations, document.PDFAnnotation{Index: i, Kind: string(a.Type), Text: a.Contents, Author: a.Author, Bounds: displayRect(document.PDFRect{X: a.Rect[0], Y: a.Rect[1], W: a.Rect[2] - a.Rect[0], H: a.Rect[3] - a.Rect[1]}, p)})
	}
	return p, nil
}
func rotation(r *gopdf.Reader, index int) int {
	v := r.InheritedPageValue(index, gopdf.Name("Rotate"))
	switch n := v.(type) {
	case int64:
		return (int(n)%360 + 360) % 360
	case int:
		return (n%360 + 360) % 360
	}
	return 0
}
func displayRect(b document.PDFRect, p document.PDFPage) document.PDFRect {
	switch p.Rotation {
	case 90:
		return document.PDFRect{X: p.Width - b.Y - b.H, Y: b.X, W: b.H, H: b.W}
	case 180:
		return document.PDFRect{X: p.Width - b.X - b.W, Y: p.Height - b.Y - b.H, W: b.W, H: b.H}
	case 270:
		return document.PDFRect{X: b.Y, Y: p.Height - b.X - b.W, W: b.H, H: b.W}
	}
	return b
}
func (d *Document) Apply(c document.PDFCommand) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("PDF 已关闭")
	}
	if d.reader.HasSignatures() {
		return errors.New("已签署的 PDF 暂只支持阅读，不能修改签名文档")
	}
	if c.Revision != d.revision {
		return errors.New("页面已更新，请重新选择内容")
	}
	p, e := d.page(c.Page)
	if e != nil {
		return e
	}
	u := gopdf.Update(d.reader)
	target, e := u.Page(c.Page)
	if e != nil {
		return e
	}
	switch c.Kind {
	case "replace":
		if c.Run < 0 || c.Run >= len(p.Text) {
			return errors.New("文字对象不存在")
		}
		if !p.Text[c.Run].Editable {
			return errors.New("当前文字暂不支持原位编辑")
		}
		if p.Text[c.Run].Text == c.Text {
			return nil
		}
		i := -1
		_, e = target.ReplaceFunc(func(r *gopdf.TextRun) (string, bool) { i++; return c.Text, i == c.Run })
	case "rotate":
		e = u.SetPageRotation(c.Page, (p.Rotation+90)%360)
	case "note", "highlight", "ink":
		if p.Rotation != 0 {
			return errors.New("请先将页面旋转回原方向后批注")
		}
		if !validPoint(c.X, c.Y, p) {
			return errors.New("批注位置超出页面")
		}
		switch c.Kind {
		case "note":
			if c.Text == "" {
				return nil
			}
			target.AddNote(c.X, c.Y, c.Text, gopdf.NoteOptions{Author: "我"})
		case "highlight":
			if c.W <= 0 || c.H <= 0 || !validPoint(c.X+c.W, c.Y+c.H, p) {
				return errors.New("请先选择文字")
			}
			target.AddHighlight(c.X, c.Y, c.W, c.H, c.Text, gopdf.NoteOptions{Author: "我"})
		case "ink":
			if len(c.Points) < 2 {
				return nil
			}
			target.SetStrokeColor(gopdf.RGB(43, 108, 176))
			target.SetLineWidth(1.5)
			for i := 1; i < len(c.Points); i++ {
				a, b := c.Points[i-1], c.Points[i]
				if !validPoint(a.X, a.Y, p) || !validPoint(b.X, b.Y, p) {
					return errors.New("笔迹超出页面")
				}
				target.Line(a.X, a.Y, b.X, b.Y)
			}
		}
	default:
		return fmt.Errorf("不支持的 PDF 操作 %q", c.Kind)
	}
	if e != nil {
		return e
	}
	var buf bytes.Buffer
	if _, e = u.WriteTo(&buf); e != nil {
		return e
	}
	r, e := gopdf.NewReader(buf.Bytes())
	if e != nil {
		return e
	}
	if r.Repaired() || r.NumPages() != d.reader.NumPages() {
		return errors.New("修改后的 PDF 验证失败")
	}
	d.undo = append(d.undo, d.data)
	if len(d.undo) > 20 {
		d.undo = d.undo[1:]
	}
	d.redo = nil
	d.modified = true
	d.data = buf.Bytes()
	d.reader = r
	d.revision++
	d.publishInfo()
	return nil
}
func (d *Document) Undo() bool { d.mu.Lock(); defer d.mu.Unlock(); return d.history(&d.undo, &d.redo) }
func (d *Document) Redo() bool { d.mu.Lock(); defer d.mu.Unlock(); return d.history(&d.redo, &d.undo) }
func (d *Document) history(from, to *[][]byte) bool {
	if len(*from) == 0 || d.closed {
		return false
	}
	b := (*from)[len(*from)-1]
	r, e := gopdf.NewReader(b)
	if e != nil {
		return false
	}
	*from = (*from)[:len(*from)-1]
	*to = append(*to, d.data)
	d.modified = !bytes.Equal(b, d.saved)
	d.data = b
	d.reader = r
	d.revision++
	d.publishInfo()
	return true
}

func validPoint(x, y float64, p document.PDFPage) bool {
	return !math.IsNaN(x) && !math.IsNaN(y) && x >= 0 && y >= 0 && x <= p.Width && y <= p.Height
}

// Until per-run graphics state is exposed, pages with external graphics state
// or form objects remain selectable but not editable (including alpha-zero OCR).
func safeTextResources(r *gopdf.Reader, index int) bool {
	resources, ok := r.Resolve(r.InheritedPageValue(index, gopdf.Name("Resources"))).(gopdf.Dict)
	if !ok {
		return false
	}
	for _, key := range []gopdf.Name{"ExtGState", "XObject"} {
		if entries, ok := r.Resolve(resources[key]).(gopdf.Dict); ok && len(entries) > 0 {
			return false
		}
	}
	return true
}
