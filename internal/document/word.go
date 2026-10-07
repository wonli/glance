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
	WordPageBreakInline
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

// WordListKind is the supported direct paragraph list type.
type WordListKind string

const (
	WordListNone        WordListKind = "none"
	WordListBullet      WordListKind = "bullet"
	WordListNumbered    WordListKind = "numbered"
	WordListUnsupported WordListKind = "unsupported"
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
	ID             string
	Style          string
	Alignment      WordAlignment
	LeftIndentEMU  int64
	RightIndentEMU int64
	Layout         WordParagraphLayout
	ListKind       WordListKind
	ListLevel      int
	ListMaxLevel   int
	ListMarker     string
	Inlines        []WordInline
}

// WordTable contains rows and cells in their source order.
type WordTable struct {
	Layout WordTableLayout
	Rows   []WordTableRow
}

type WordTableRow struct {
	Cells []WordTableCell
}

type WordTableCell struct {
	ID         string
	WidthEMU   int64
	FillColor  string
	Paragraphs []WordParagraph
}

// WordInline is a tagged union. Exactly one payload should match Kind.
type WordInline struct {
	Kind        WordInlineKind
	Text        *WordTextRun
	Image       *WordImage
	Unsupported *WordUnsupported
	PageBreak   *WordPageBreak
}

// WordTextRun exposes supported text and the formatting applied by its source run.
type WordTextRun struct {
	ID             string
	Text           string
	Style          WordTextStyle
	StyleSet       WordTextStyleMask
	CharacterStyle string
	Editable       bool
}

// WordTextStyle is the format-neutral text style subset rendered by the UI.
type WordTextStyle struct {
	Bold       bool
	Italic     bool
	Underline  bool
	Strike     bool
	FontFamily string
	FontSize   float64
	FontColor  string
	Highlight  string
}

// WordImage describes an inline image without exposing package relationships.
type WordImage struct {
	ID              string
	MediaType       string
	WidthEMU        int64
	HeightEMU       int64
	RotationDegrees float64
	Resolved        bool
	Editable        bool
}

// WordBreakType identifies the type of break to insert (e.g. "page", "column").
type WordBreakType string

const (
	WordBreakPage   WordBreakType = "page"
	WordBreakColumn WordBreakType = "column"
)

// WordPageBreak represents a native DOCX break in continuous-flow preview.
type WordPageBreak struct {
	ID   string
	Type WordBreakType
}

// WordUnsupported describes retained content that cannot be projected safely.
type WordUnsupported struct {
	ID           string
	Description  string
	ImagePreview *WordImage
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
	Highlight *string
}

// WordTextEditor is an optional capability for safe Word text operations.
type WordTextEditor interface {
	ReplaceTextRun(runID, value string) error
	SetTextRunStyle(runID string, patch WordTextFormatPatch) error
	SetParagraphAlignment(paragraphID string, alignment WordAlignment) error
	SetParagraphIndents(paragraphID string, leftEMU, rightEMU int64) error
}

// WordParagraphSpacingEditor patches direct before/after spacing in DOCX
// twips. A nil value leaves that side and its inherited spacing untouched.
type WordParagraphSpacingEditor interface {
	SetParagraphSpacing(paragraphID string, beforeTwips, afterTwips *int64) error
	SetAllParagraphsSpacing(beforeTwips, afterTwips int64) error
}

// WordTextRangeEditor exposes safe replacement within one editable text run.
// Offsets are Unicode rune indices, matching WordTextRun text selection.
type WordTextRangeEditor interface {
	ReplaceTextRange(runID string, start, end int, replacement string) error
}

// WordFormatPainter copies the supported direct text and paragraph formatting
// from one editable text run to another within the same document.
type WordFormatPainter interface {
	ApplyWordFormatting(sourceRunID, targetRunID string) error
}

// WordParagraphListEditor exposes safe direct paragraph bullet and numbering edits.
type WordParagraphListEditor interface {
	SetParagraphList(paragraphID string, kind WordListKind) error
	SetParagraphListLevel(paragraphID string, level int) error
}

// WordTextFontSizeEditor exposes safe direct font-size edits for a text run.
type WordTextFontSizeEditor interface {
	SetTextRunFontSize(runID string, points float64) error
}

// WordTextFontFamilyEditor exposes safe direct font-family edits for a text run.
type WordTextFontFamilyEditor interface {
	SetTextRunFontFamily(runID, family string) error
}

// WordBodyEditor is an optional capability for adding a body paragraph at the end of a Word document.
type WordBodyEditor interface {
	InsertBodyParagraph() (paragraphID string, err error)
}

// WordPageBreakEditor inserts a page or column break, or a blank page, at a Unicode text range in an
// editable run. The returned run is the caret location after the break.
type WordPageBreakEditor interface {
	InsertPageBreakAtRange(runID, value string, start, end int) (paragraphID, caretRunID string, err error)
	InsertBreakAtRange(runID, value string, start, end int, breakType WordBreakType) (paragraphID, caretRunID string, err error)
	InsertBlankPageAtRange(runID, value string, start, end int) (paragraphID, caretRunID string, err error)
}

// WordParagraphEditor exposes safe keyboard-driven paragraph structure edits.
type WordParagraphEditor interface {
	SplitParagraphAt(runID, value string, selectionStart, selectionEnd int) (paragraphID, newRunID string, err error)
}

// WordParagraphJoinDirection selects the adjacent paragraph to join at a text boundary.
type WordParagraphJoinDirection uint8

const (
	WordJoinPrevious WordParagraphJoinDirection = iota + 1
	WordJoinNext
)

// WordParagraphJoinResult identifies the editable caret position at the paragraph seam.
type WordParagraphJoinResult struct {
	ParagraphID string
	RunID       string
	Caret       int
}

// WordParagraphJoiner exposes safe adjacent paragraph joins for Word documents.
type WordParagraphJoiner interface {
	JoinParagraphAtBoundary(runID string, direction WordParagraphJoinDirection) (WordParagraphJoinResult, error)
}

// WordImageEditor is an optional capability for inline raster image operations.
type WordImageEditor interface {
	InsertInlineImage(paragraphID, afterInlineID, mediaType string, data []byte) (imageID string, err error)
	ReplaceInlineImage(imageID, mediaType string, data []byte) error
	MoveInlineImage(imageID, targetParagraphID, afterInlineID string) error
	ResizeInlineImage(imageID string, widthEMU int64) error
	DeleteInlineImage(imageID string) error
}

// WordImageRotator is an optional capability for rotating inline Word images.
type WordImageRotator interface {
	SetInlineImageRotation(imageID string, degrees float64) error
}

// WordDocumentProperties is the safe, format-neutral core-properties subset
// exposed by Word documents.
type WordDocumentProperties struct {
	Title       string
	Subject     string
	Creator     string
	Keywords    string
	Description string
}

// WordDocumentPropertiesEditor reads and updates supported document metadata.
type WordDocumentPropertiesEditor interface {
	GetWordDocumentProperties() (WordDocumentProperties, error)
	SetWordDocumentProperties(WordDocumentProperties) error
}

// WordHistory is the undo/redo capability for supported Word edits.
type WordHistory interface {
	Undo() (bool, error)
	Redo() (bool, error)
	CanUndo() bool
	CanRedo() bool
}

// WordPackageSnapshotter serializes the complete editable document package.
type WordPackageSnapshotter interface {
	Snapshot() ([]byte, error)
}

// WordPackageSnapshotSaver writes a package and returns the exact bytes committed.
type WordPackageSnapshotSaver interface {
	SaveWithSnapshot() ([]byte, error)
	SaveAsDocumentWithSnapshot(path string) ([]byte, error)
}

// WordPackageSnapshotRestorer applies a complete package as one undoable edit.
type WordPackageSnapshotRestorer interface {
	RestoreSnapshot(snapshot []byte) error
}

// WordPageSetupEditor exposes safe page geometry and section layout edits.
type WordPageSetupEditor interface {
	SetPageOrientation(orientation string) error
	SetPageMargins(topPT, rightPT, bottomPT, leftPT float64) error
	SetPageSize(widthPT, heightPT float64) error
	SetPageColumns(columns int) error
	SetPageLineNumbers(restart string) error
	SetPageBorders(borderType string) error
}

// WordDocumentProtectionEditor is an optional capability for inspecting and updating Word document protection.
type WordDocumentProtectionEditor interface {
	GetWordDocumentProtection() (bool, error)
	SetWordDocumentProtection(readOnly bool) error
}

// WordPageBackgroundEditor is an optional capability for inspecting and setting page background color.
type WordPageBackgroundEditor interface {
	GetPageBackgroundColor() (string, error)
	SetPageBackgroundColor(hexColor string) error
}

// WordTableInsertEditor exposes inserting a basic table into the document body.
type WordTableInsertEditor interface {
	InsertTable(afterParagraphID string, rows, cols int) (tableID string, err error)
}

// WordCoverPageEditor provides inserting a standard cover page into the document.
type WordCoverPageEditor interface {
	InsertCoverPage(title, subtitle, author string) error
}
// WordLanguageEditor provides setting the proofing/document language tag.
type WordLanguageEditor interface {
	SetLanguage(runID, langCode string) error
}

// WordBookmarkEditor provides inserting named bookmarks into the Word document.
type WordBookmarkEditor interface {
	InsertBookmark(runID, name string) error
}

// WordHyperlinkEditor provides adding external or anchor hyperlinks into the Word document.
type WordHyperlinkEditor interface {
	InsertHyperlink(runID, text, targetURL string) error
}

// WordTextBoxEditor provides inserting a stylized callout text box into the Word document.
type WordTextBoxEditor interface {
	InsertTextBox(afterParagraphID, text string) error
}

// WordHeaderFooterEditor provides configuring document header, footer, or page numbers.
type WordHeaderFooterEditor interface {
	SetHeader(text string) error
	SetFooter(text string) error
	SetPageNumber(align string) error
}
