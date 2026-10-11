package wordlayout

import "glance/internal/document"

type Input struct {
	DocumentID       string
	Revision         uint64
	Blocks           []document.WordBlock
	Layout           document.WordLayoutData
	DefaultPageSetup document.WordPageSetup
	Previous         *Result
	// EditorParagraphInsets makes page scenes reserve the same small paragraph
	// padding used by the editable Gio canvas, so screen and print share geometry.
	EditorParagraphInsets bool
}

// DefaultPageSetup returns Glance's product default for documents without
// complete source page geometry.
func DefaultPageSetup() document.WordPageSetup {
	return document.WordPageSetup{
		WidthPT: 595.28, HeightPT: 841.89, Orientation: "portrait",
		MarginTopPT: 72, MarginRightPT: 72, MarginBottomPT: 72, MarginLeftPT: 72,
	}
}

type SourcePosition struct {
	RunID      string
	RuneOffset int
}

type Position struct {
	PageIndex int
	XPT       float64
	YPT       float64
}

type DiagnosticSeverity string

type Diagnostic struct {
	Severity DiagnosticSeverity
	SourceID string
	Code     string
	Detail   string
}

type Rect struct {
	XPT      float64
	YPT      float64
	WidthPT  float64
	HeightPT float64
}

type PageScene struct {
	Index            int
	SectionIndex     int
	PageSetup        document.WordPageSetup
	Clip             Rect
	Content          Rect
	Text             []TextFragment
	Images           []ImagePlacement
	TableCells       []TableCellPlacement
	ParagraphBorders []ParagraphBorderPlacement
}

// ParagraphBorderPlacement is a paragraph decoration shared by screen and
// print renderers. Bounds.HeightPT stores the border thickness.
type ParagraphBorderPlacement struct {
	SourceID string
	Style    string
	Color    string
	Bounds   Rect
}

type TextFragment struct {
	RunID       string
	Text        string
	RuneStart   int
	RuneEnd     int
	Editable    bool
	Style       document.WordTextStyle
	XPT         float64
	BaselineYPT float64
	Glyphs      []PositionedGlyph
	Synthetic   bool
}

type PositionedGlyph struct {
	FontKey    string
	GlyphID    uint32
	RuneStart  int
	RuneEnd    int
	XPT        float64
	YPT        float64
	AdvanceXPT float64
	Outline    []GlyphPathCommand
}

type ImagePlacement struct {
	ImageID         string
	Bounds          Rect
	RotationDegrees float64
}

type TableCellPlacement struct {
	TableID          string
	RowIndex         int
	CellID           string
	FillColor        string
	Bounds           Rect
	DrawTopBorder    bool
	DrawBottomBorder bool
	DrawLeftBorder   bool
	DrawRightBorder  bool
}

type Result struct {
	DocumentID        string
	Revision          uint64
	Pages             []PageScene
	Diagnostics       []Diagnostic
	Locations         map[SourcePosition]Position
	CanPreview        bool
	blockFingerprints [][32]byte
	blockKinds        []document.WordBlockKind
	blockHazards      []bool
	layoutFingerprint string
	blockCheckpoints  []layoutCheckpoint
	runBlocks         map[string]int
	diagnosticBlocks  []int
}

type layoutCheckpoint struct {
	PageIndex                  int
	SectionIndex               int
	Col                        int
	YPT                        float64
	Used                       bool
	PendingBreak               bool
	AutomaticPageStart         bool
	HasPreviousParagraph       bool
	PreviousParagraphAfterPT   float64
	PreviousParagraphLineGapPT float64
	TextCount                  int
	ImageCount                 int
	TableCellCount             int
	ParagraphBorderCount       int
}
