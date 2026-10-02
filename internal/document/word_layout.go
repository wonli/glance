package document

// WordPageSetup stores physical page geometry in points. Set distinguishes an
// omitted value from an explicit zero margin. Synthetic callers may also set
// nonzero values without the mask.
type WordPageSetupMask uint16

const (
	WordPageWidthSet WordPageSetupMask = 1 << iota
	WordPageHeightSet
	WordPageOrientationSet
	WordPageMarginTopSet
	WordPageMarginRightSet
	WordPageMarginBottomSet
	WordPageMarginLeftSet
)

type WordPageSetup struct {
	WidthPT        float64
	HeightPT       float64
	Orientation    string
	MarginTopPT    float64
	MarginRightPT  float64
	MarginBottomPT float64
	MarginLeftPT   float64
	Set            WordPageSetupMask
}

// WordSection maps one section's page settings to the first body block using
// those settings. The MVP layout service diagnoses documents with more than
// one section rather than guessing section transitions.
type WordSection struct {
	StartBlock int
	PageSetup  WordPageSetup
}

// WordParagraphLayoutMask records which values were explicitly supplied, so
// style inheritance can distinguish an omitted value from an explicit zero.
type WordParagraphLayoutMask uint16

const (
	WordParagraphBeforeSet WordParagraphLayoutMask = 1 << iota
	WordParagraphAfterSet
	WordParagraphLineSpacingSet
	WordParagraphFirstLineIndentSet
	WordParagraphHangingIndentSet
	WordParagraphAlignmentSet
	WordParagraphLeftIndentSet
	WordParagraphRightIndentSet
	WordParagraphKeepWithNextSet
	WordParagraphKeepTogetherSet
	WordParagraphPageBreakBeforeSet
)

// WordParagraphLayout contains the neutral paragraph metrics used by page
// layout. Lengths are points except indentation values preserved in EMU.
type WordParagraphLayout struct {
	BeforePT            float64
	AfterPT             float64
	LineSpacingPT       float64
	LineSpacingMultiple float64
	LineSpacingRule     string
	FirstLineIndentEMU  int64
	HangingIndentEMU    int64
	Alignment           WordAlignment
	LeftIndentEMU       int64
	RightIndentEMU      int64
	KeepWithNext        bool
	KeepTogether        bool
	PageBreakBefore     bool
	Set                 WordParagraphLayoutMask
}

// WordTableLayout contains source grid widths in EMU.
type WordTableLayout struct {
	GridColumnsEMU []int64
}

// WordTextStyleMask records which Word text style values were explicitly set.
type WordTextStyleMask uint16

const (
	WordTextBoldSet WordTextStyleMask = 1 << iota
	WordTextItalicSet
	WordTextUnderlineSet
	WordTextFontFamilySet
	WordTextFontSizeSet
	WordTextFontColorSet
	WordTextHighlightSet
)

// WordStyle describes one paragraph or character style and its inheritance
// parent. Masks preserve Word's distinction between absent and explicitly
// disabled formatting.
type WordStyle struct {
	ID        string
	BasedOn   string
	Text      WordTextStyle
	TextSet   WordTextStyleMask
	Paragraph WordParagraphLayout
}

// WordStyleDefaults holds the package-level w:docDefaults values.
type WordStyleDefaults struct {
	Text      WordTextStyle
	TextSet   WordTextStyleMask
	Paragraph WordParagraphLayout
}

// WordLayoutUnsupported locates a retained DOCX construct that cannot be
// paginated faithfully by the currently supported layout subset.
type WordLayoutUnsupported struct {
	SourceID    string
	Code        string
	Description string
}

// WordLayoutData contains page settings and styles projected from the DOCX
// package. It is a read-only neutral view and exposes no OOXML or ZIP types.
type WordLayoutData struct {
	Sections    []WordSection
	Defaults    WordStyleDefaults
	Styles      []WordStyle
	Unsupported []WordLayoutUnsupported
}

// WordLayoutReader is an optional DOCX capability for neutral layout metadata.
type WordLayoutReader interface {
	WordLayoutData() WordLayoutData
}
