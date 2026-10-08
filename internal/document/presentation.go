package document

import "image"

// PresentationRect represents an axis-aligned bounding box in points (1 pt = 1/72 inch).
type PresentationRect struct {
	X, Y, W, H float64
}

// PresentationPath stores one custom DrawingML path in its local coordinate space.
type PresentationPath struct {
	Width, Height float64
	Fill          string
	Stroke        string
	Commands      []PresentationPathCommand
}

// PresentationPathCommand stores a path operation and its points in path-local coordinates.
type PresentationPathCommand struct {
	Kind   string
	Points []PresentationPoint
}

// PresentationPoint is a point in a custom path's local coordinate space.
type PresentationPoint struct {
	X, Y float64
}

// PresentationGradient stores a multi-stop linear fill when one is supported by the adapter.
type PresentationGradient struct {
	AngleDeg float64
	PathType string
	Stops    []PresentationGradientStop
}

type PresentationGradientStop struct {
	Position float64 // 0..100000
	ColorHex string
	Alpha    float64
}

// PresentationElementKind represents the kind of element on a slide.
type PresentationElementKind string

const (
	ElementKindText    PresentationElementKind = "text"
	ElementKindPicture PresentationElementKind = "picture"
	ElementKindShape   PresentationElementKind = "shape"
	ElementKindTable   PresentationElementKind = "table"
)

// PresentationTextRun represents a formatted run of text within a paragraph.
type PresentationTextRun struct {
	Text      string
	Font      string
	SizePT    float64
	Baseline  int32 // DrawingML baseline shift in thousandths; negative is subscript.
	Bold      bool
	Italic    bool
	Underline bool
	ColorHex  string
}

// PresentationParagraph represents a paragraph of text runs.
type PresentationParagraph struct {
	Runs   []PresentationTextRun
	Align  string // "left", "center", "right", "justify"
	Bullet bool
}

// PlainText returns the full text across all runs in the paragraph.
func (p PresentationParagraph) PlainText() string {
	var total string
	for _, r := range p.Runs {
		total += r.Text
	}
	return total
}

// PresentationElement represents an individual item on a slide.
type PresentationElement struct {
	ID          int64
	Name        string
	Kind        PresentationElementKind
	Bounds      PresentationRect
	RotationDeg float64

	// Text content (used by ElementKindText or shapes with text)
	Paragraphs []PresentationParagraph
	WrapText   bool // false when wrap="none", true when text wraps within bounds
	AutoFit    bool // true when spAutoFit is present

	// Shape styling
	FillColorHex string
	FillGradient *PresentationGradient
	BorderColor  string
	BorderWidth  float64
	ShapeType    string // "rect", "roundRect", "ellipse", etc.
	Paths        []PresentationPath

	// Image content (used by ElementKindPicture)
	ImageBytes []byte
	Image      image.Image
	AltText    string
}

// PlainText returns the combined plain text of all paragraphs in the element.
func (e PresentationElement) PlainText() string {
	var res string
	for i, p := range e.Paragraphs {
		if i > 0 {
			res += "\n"
		}
		res += p.PlainText()
	}
	return res
}

// PresentationSlide represents a single slide in the presentation.
type PresentationSlide struct {
	Index    int
	ID       uint32
	Title    string
	Notes    string
	Elements []PresentationElement
	// BackgroundImage is the resolved slide background image, if supported.
	BackgroundImage image.Image
	// InheritedElements are read-only visuals projected from the layout/master.
	InheritedElements []PresentationElement
	WidthPT           float64
	HeightPT          float64
	BgColorHex        string
}

// PresentationSnapshot represents an immutable view data snapshot for UI rendering.
type PresentationSnapshot struct {
	Title       string
	WidthPT     float64
	HeightPT    float64
	SlideCount  int
	ActiveSlide int
	Slides      []PresentationSlide
	Revision    uint64
	Dirty       bool
	CanUndo     bool
	CanRedo     bool
}
