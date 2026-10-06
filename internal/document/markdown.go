package document

// Markdown snapshots contain only neutral view data; source remains authoritative.
type MarkdownSpan struct {
	Text, Destination, Kind string
	LinkDestination         string
	CodeColor               string
	Bold, Italic            bool
	Strike, DisplayMath     bool
	Checked                 bool
	SourceStart, SourceEnd  int
}
type MarkdownBlock struct {
	Kind, Text, Prefix, Language string
	Anchor                       string
	Level, Offset                int
	SourceStart, SourceEnd       int
	Spans                        []MarkdownSpan
	CodeSpans                    []MarkdownSpan
	TableSpans                   [][][]MarkdownSpan
	TableAlignments              []string
	Rows                         [][]string
}
type MarkdownHeading struct {
	Title                string
	Level, Offset, Block int
}
type MarkdownSnapshot struct {
	Source                            string
	Blocks                            []MarkdownBlock
	Headings                          []MarkdownHeading
	Characters, Words, ReadingMinutes int
	Diagnostics                       []string
}
