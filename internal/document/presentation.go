package document

import "image"

// PresentationRect represents an axis-aligned bounding box in points (1 pt = 1/72 inch).
type PresentationRect struct {
	X, Y, W, H float64
}

// PresentationTableData carries plain cell text and built-in row style flags.
type PresentationTableData struct {
	Rows     [][]string
	FirstRow bool
	BandRows bool
}

// PresentationChartSeries is one named series aligned to chart categories.
type PresentationChartSeries struct {
	Name   string
	Values []float64
}

// PresentationChartData is the format-neutral subset supported for chart creation.
type PresentationChartData struct {
	Type       string
	Title      string
	Categories []string
	Series     []PresentationChartSeries
}

// PresentationAudioData is the neutral input for embedding audio in a slide.
type PresentationAudioData struct {
	TrackKey  string
	Role      string
	MediaType string
	Bytes     []byte
}

// PresentationVideoData is the neutral input for embedding video in a slide.
type PresentationVideoData struct {
	TrackKey   string
	MediaType  string
	AltText    string
	Decorative bool
	Bytes      []byte
}

// PresentationCrop stores the visible-source margins in DrawingML's 0..100000 units.
type PresentationCrop struct {
	Left, Top, Right, Bottom int
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
	ElementKindAudio   PresentationElementKind = "audio"
	ElementKindVideo   PresentationElementKind = "video"
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
	Runs        []PresentationTextRun
	Align       string // "left", "center", "right", "justify"
	LineSpacing *PresentationSpacing
	SpaceBefore *PresentationSpacing
	SpaceAfter  *PresentationSpacing
	Bullet      bool
}

// PresentationSpacing preserves explicit paragraph spacing without exposing
// the PPTX adapter's library types. Percentage values are ratios (1.5 = 150%);
// point values are points.
type PresentationSpacing struct {
	Kind  string  `json:"kind"` // "pct" or "pts"
	Value float64 `json:"value"`
}

// PresentationTransition is the neutral, format-independent subset of a
// slide transition. Direction and split settings are meaningful only for the
// corresponding transition types.
type PresentationTransition struct {
	Type           string `json:"type"`
	Speed          string `json:"speed"`
	Direction      string `json:"direction,omitempty"`
	SplitDirection string `json:"split_direction,omitempty"`
	SplitAxis      string `json:"split_axis,omitempty"`
	ThroughBlack   bool   `json:"through_black"`
	AdvanceOnClick bool   `json:"advance_on_click"`
}

// PresentationTransitionPatch preserves omitted fields while editing a
// transition already stored on a slide.
type PresentationTransitionPatch struct {
	Type           *string `json:"type,omitempty"`
	Speed          *string `json:"speed,omitempty"`
	Direction      *string `json:"direction,omitempty"`
	SplitDirection *string `json:"split_direction,omitempty"`
	SplitAxis      *string `json:"split_axis,omitempty"`
	ThroughBlack   *bool   `json:"through_black,omitempty"`
	AdvanceOnClick *bool   `json:"advance_on_click,omitempty"`
}

// PresentationComment is a neutral projection of a classic PPTX slide comment.
// Position is measured in points from the slide's upper-left corner.
type PresentationComment struct {
	ID          string  `json:"id"`
	SlideID     uint32  `json:"slide_id"`
	SlideNumber int     `json:"slide_number"`
	Author      string  `json:"author"`
	Text        string  `json:"text"`
	XPT         float64 `json:"x_pt"`
	YPT         float64 `json:"y_pt"`
}

// PresentationElementStylePatch contains only explicitly requested style
// changes. Pointer fields preserve the distinction between an omitted field
// and an explicit false, zero, or "none" value.
type PresentationElementStylePatch struct {
	FontFamily         *string
	FontSizePT         *float64
	Bold               *bool
	Italic             *bool
	Underline          *bool
	TextColorHex       *string
	ParagraphAlignment *string
	Bullet             *bool
	LineSpacing        *PresentationSpacing
	SpaceBefore        *PresentationSpacing
	SpaceAfter         *PresentationSpacing
	FillColorHex       *string
	BorderColor        *string
	BorderWidthPT      *float64
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
	ID   int64
	Name string
	Kind PresentationElementKind
	// GroupID is the stable top-level DrawingML group ID when this element is
	// nested inside a group. Group containers stay out of the flat canvas model.
	GroupID     uint32 `json:"group_id,omitempty"`
	Bounds      PresentationRect
	RotationDeg float64
	// AspectRatioLocked reflects an explicit DrawingML noChangeAspect lock.
	AspectRatioLocked bool
	// PlaceholderType preserves the slide-local DrawingML placeholder role so
	// the UI can show an empty-field prompt while keeping its actual text empty.
	PlaceholderType string

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
	// Adapter-projected capabilities keep unsupported source structures out
	// of the style controls without exposing format-library types to the UI.
	CanEditTextStyle  bool
	CanEditShapeStyle bool

	// Image content (used by ElementKindPicture)
	ImageBytes     []byte
	Image          image.Image
	AltText        string
	Decorative     bool
	Crop           PresentationCrop
	CanEditPicture bool
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

// PresentationAnimation is a neutral, read-only projection of one PPTX
// timing effect. It deliberately preserves timing metadata as strings because
// OOXML permits values that are not understood by every editor.
type PresentationAnimation struct {
	Index           int    `json:"index"`
	Kind            string `json:"kind"`
	TargetElementID uint32 `json:"target_element_id,omitempty"`
	PresetID        string `json:"preset_id,omitempty"`
	PresetClass     string `json:"preset_class,omitempty"`
	Transition      string `json:"transition,omitempty"`
	Effect          string `json:"effect,omitempty"`
	Duration        string `json:"duration,omitempty"`
}

// PresentationAnimationSummary contains a bounded view of one slide's
// animation timing tree. A truncated result is still useful for inspection,
// but callers must not treat it as a complete inventory.
type PresentationAnimationSummary struct {
	SlideID     uint32                  `json:"slide_id"`
	Revision    uint64                  `json:"revision"`
	HasTiming   bool                    `json:"has_timing"`
	Truncated   bool                    `json:"truncated"`
	Animations  []PresentationAnimation `json:"animations"`
	Diagnostics []string                `json:"diagnostics"`
}

// PresentationLayoutOption describes one layout in the document-local order
// returned by the format adapter. Index is valid only for the accompanying
// revision and is intended for follow-up operations that require selection.
type PresentationLayoutOption struct {
	Index         int    `json:"index"`
	Name          string `json:"name"`
	NameTruncated bool   `json:"name_truncated,omitempty"`
}

// PresentationLayoutSummary is a revision-bound catalog of layouts available
// to the current presentation.
type PresentationLayoutSummary struct {
	Revision  uint64                     `json:"revision"`
	Total     int                        `json:"total"`
	Truncated bool                       `json:"truncated"`
	Layouts   []PresentationLayoutOption `json:"layouts"`
}
