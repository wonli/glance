package document

// PDF DTOs never expose backend handles. Run IDs are valid only at Revision.
type PDFRect struct{ X, Y, W, H float64 }
type PDFText struct {
	Run      int
	Text     string
	Bounds   PDFRect
	Editable bool
}
type PDFAnnotation struct {
	Index              int
	Kind, Text, Author string
	Bounds             PDFRect
}
type PDFPage struct {
	Index         int
	Revision      uint64
	Width, Height float64
	Rotation      int
	Text          []PDFText
	Annotations   []PDFAnnotation
}
type PDFInfo struct {
	Pages, Bytes            int
	Revision                uint64
	Dirty, CanUndo, CanRedo bool
}
type PDFPoint struct{ X, Y float64 }
type PDFCommand struct {
	Kind       string
	Page, Run  int
	Revision   uint64
	Text       string
	X, Y, W, H float64
	Points     []PDFPoint
}
