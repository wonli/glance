// Package textalign locates OCR page evidence in exact Markdown source without
// editing either document. Scores describe evidence, never correctness probabilities.
package textalign

type Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}
type Block struct {
	ID     int        `json:"id"`
	Text   string     `json:"text"`
	Label  string     `json:"label,omitempty"`
	Bounds [4]float64 `json:"bounds"`
}
type Page struct {
	Number int     `json:"number"`
	Blocks []Block `json:"blocks"`
}
type Evidence struct {
	BlockID           int        `json:"blockID"`
	Range             Range      `json:"range"`
	Bounds            [4]float64 `json:"bounds"`
	QueryCharacters   int        `json:"queryCharacters"`
	Matches           int        `json:"matches"`
	Edits             int        `json:"edits"`
	QueryRange        Range      `json:"queryRange"`
	start, end        int
	score, queryOrder int
	title             bool
}
type Candidate struct {
	Range          Range      `json:"range"`
	Score          float64    `json:"score"`
	Coverage       float64    `json:"coverage"`
	Reordered      bool       `json:"reordered"`
	SourceCoverage float64    `json:"sourceCoverage"`
	Evidence       []Evidence `json:"evidence"`
	Joint          bool       `json:"joint,omitempty"`
	start, end     int
}
type Result struct {
	Page                                int         `json:"page"`
	Status                              string      `json:"status"`
	Selected                            *Candidate  `json:"selected,omitempty"`
	Candidates                          []Candidate `json:"candidates"`
	PathChoices                         []Candidate `json:"pathChoices,omitempty"`
	Reasons                             []string    `json:"reasons,omitempty"`
	Search                              SearchStats `json:"search"`
	Anchors                             []Anchor    `json:"anchors,omitempty"`
	BoundaryAfter                       *Boundary   `json:"boundaryAfter,omitempty"`
	Context                             string      `json:"context,omitempty"`
	searchLimited, pathChecked, canSkip bool
	hasUnread                           bool
	supported                           []int
	selectedChoice                      int
	// ExactBoundary remains false: text evidence endpoints do not assign surrounding
	// Markdown markup, whitespace, or unrecognized images to pages.
	ExactBoundary bool `json:"exactBoundary"`
}

// SearchStats counts alignment work, not network time or total process memory.
type SearchStats struct {
	Windows      int `json:"windows"`
	TitleWork    int `json:"titleWork"`
	WorkUnits    int `json:"workUnits"`
	LocalCalls   int `json:"localCalls"`
	CacheHits    int `json:"cacheHits"`
	AnchorWork   int `json:"anchorWork"`
	InternalPins int `json:"internalPins"`
	JointCells   int `json:"jointCells"`
}
type Options struct {
	// MaxCells bounds dynamic-programming work per page. Zero uses 4 million.
	MaxCells int
	// MaxCandidates and BeamWidth limit search; pruning is explicitly reported.
	MaxCandidates int
	BeamWidth     int
	// MaxPathWork bounds candidate visits when checking exact cross-page paths.
	// Zero uses one million; BeamWidth is retained for compatibility and ignored.
	MaxPathWork int
	// MaxJointCells bounds forward+reverse cells over all anchored spans.
	// Zero uses 2 million; the hard ceiling is 8 million.
	MaxJointCells int
	// OnProgress reports candidate-search and global-path phases. Align calls it
	// synchronously; the callback should return quickly and must not block on UI
	// work. A callback may cancel the supplied context to stop the alignment.
	OnProgress func(AlignmentProgress)
}

// AlignmentProgress reports page-level work performed by Align. Page is the
// source PDF page number during candidate search, and zero during path solving.
// CompletedPages counts candidate searches that have finished; it does not
// imply that those pages have final matches until Align returns.
type AlignmentProgress struct {
	Phase          string
	Page           int
	CompletedPages int
	TotalPages     int
}

const (
	AlignmentPhaseCandidates = "candidates"
	AlignmentPhasePath       = "path"
)

type Anchor struct {
	BlockID int   `json:"blockID"`
	Range   Range `json:"range"`
	// QueryRange is a normalized character range within the OCR block.
	QueryRange                       Range `json:"queryRange"`
	start, end, queryStart, queryEnd int
	page, queryOrder                 int
}

// Boundary encloses plausible original-source cut positions, inclusive at both
// ends. A point is conditional on the text model, not verified page ownership.
type Boundary struct {
	NextPage int    `json:"nextPage"`
	Status   string `json:"status"`
	Min      int    `json:"min"`
	Max      int    `json:"max"`
	Method   string `json:"method"`
	Reason   string `json:"reason,omitempty"`
}
