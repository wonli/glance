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

// SupportImpact describes how a known support gap affects the document.
type SupportImpact string

const (
	// SupportImpactApproximation means content remains available through a
	// documented fallback. Preview is allowed; print/PDF asks for confirmation.
	SupportImpactApproximation SupportImpact = "approximation"
	// SupportImpactBlocksPreview means page geometry cannot be represented
	// reliably, so preview and page-based output are blocked.
	SupportImpactBlocksPreview SupportImpact = "blocks-preview"
	// SupportImpactBlocksExport means preview is useful, but page-based output
	// would silently discard or misrepresent required content.
	SupportImpactBlocksExport SupportImpact = "blocks-export"
)

// SupportFinding is one actionable result from the DOCX support preflight or
// the layout pass. SourceID points back to the neutral source model.
type SupportFinding struct {
	SourceID string
	Location string
	Code     string
	Detail   string
	Impact   SupportImpact
	Fallback string
}

// SupportReport summarizes known gaps without treating every one as a fatal
// document error.
type SupportReport struct {
	Findings []SupportFinding
}

func (r SupportReport) BlocksPreview() bool {
	for _, finding := range r.Findings {
		if finding.Impact == SupportImpactBlocksPreview {
			return true
		}
	}
	return false
}

func (r SupportReport) BlocksExport() bool {
	for _, finding := range r.Findings {
		if finding.Impact == SupportImpactBlocksPreview || finding.Impact == SupportImpactBlocksExport {
			return true
		}
	}
	return false
}

func (r SupportReport) RequiresConfirmation() bool {
	for _, finding := range r.Findings {
		if finding.Impact == SupportImpactApproximation {
			return true
		}
	}
	return false
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
	SupportReport     SupportReport
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
