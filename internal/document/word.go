package document

// WordBlockKind identifies a top-level block in a Word document body.
type WordBlockKind uint8

const (
	WordParagraphBlock WordBlockKind = iota + 1
	WordTableBlock
	WordUnsupportedBlock
)

// WordInlineKind identifies an ordered item inside a Word paragraph.
type WordInlineKind uint8

const (
	WordTextInline WordInlineKind = iota + 1
	WordImageInline
	WordUnsupportedInline
)

// WordAlignment is the neutral alignment subset supported by the desktop UI.
type WordAlignment string

const (
	WordAlignDefault   WordAlignment = "default"
	WordAlignLeft      WordAlignment = "left"
	WordAlignCenter    WordAlignment = "center"
	WordAlignRight     WordAlignment = "right"
	WordAlignJustified WordAlignment = "justified"
)

// WordBlock is one ordered paragraph, table, or read-only body item.
type WordBlock struct {
	ID          string
	Kind        WordBlockKind
	Paragraph   *WordParagraph
	Table       *WordTable
	Unsupported *WordUnsupported
}

// WordParagraph preserves the ordered inline contents of one paragraph.
type WordParagraph struct {
	ID        string
	Style     string
	Alignment WordAlignment
	Inlines   []WordInline
}

// WordTable contains rows and cells in their source order.
type WordTable struct {
	Rows []WordTableRow
}

type WordTableRow struct {
	Cells []WordTableCell
}

type WordTableCell struct {
	ID         string
	Paragraphs []WordParagraph
}

// WordInline is a tagged union. Exactly one payload should match Kind.
type WordInline struct {
	Kind        WordInlineKind
	Text        *WordTextRun
	Image       *WordImage
	Unsupported *WordUnsupported
}

// WordTextRun exposes supported text and the formatting applied by its source run.
type WordTextRun struct {
	ID       string
	Text     string
	Style    WordTextStyle
	Editable bool
}

// WordTextStyle is the format-neutral text style subset rendered by the UI.
type WordTextStyle struct {
	Bold       bool
	Italic     bool
	Underline  bool
	FontFamily string
	FontSize   float64
	FontColor  string
}

// WordImage describes an inline image without exposing package relationships.
type WordImage struct {
	ID        string
	MediaType string
	WidthEMU  int64
	HeightEMU int64
	Resolved  bool
	Editable  bool
}

// WordUnsupported describes retained content that cannot be projected safely.
type WordUnsupported struct {
	ID          string
	Description string
}

// WordContentReader is an optional capability for ordered DOCX content.
type WordContentReader interface {
	WordBlocks() []WordBlock
	WordImageBytes(imageID string) ([]byte, error)
}

// WordTextFormatPatch changes only explicitly supplied text properties.
type WordTextFormatPatch struct {
	Bold      *bool
	Italic    *bool
	Underline *bool
	FontColor *string
}

// WordTextEditor is an optional capability for safe Word text operations.
type WordTextEditor interface {
	ReplaceTextRun(runID, value string) error
	SetTextRunStyle(runID string, patch WordTextFormatPatch) error
	SetParagraphAlignment(paragraphID string, alignment WordAlignment) error
}

// WordImageEditor is an optional capability for inline raster image operations.
type WordImageEditor interface {
	InsertInlineImage(paragraphID, afterInlineID, mediaType string, data []byte) (imageID string, err error)
	ReplaceInlineImage(imageID, mediaType string, data []byte) error
	MoveInlineImage(imageID, targetParagraphID, afterInlineID string) error
	ResizeInlineImage(imageID string, widthEMU int64) error
	DeleteInlineImage(imageID string) error
}

// WordHistory is the undo/redo capability for supported Word edits.
type WordHistory interface {
	Undo() (bool, error)
	Redo() (bool, error)
	CanUndo() bool
	CanRedo() bool
}
