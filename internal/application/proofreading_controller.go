package application

import (
	"errors"
	"glance/internal/proofreading"
	"math"
	"unicode/utf8"
)

var ErrProofreadingLayoutPending = errors.New("proofreading layout is pending")

type ProofreadingLine struct {
	Start, End  int
	Top, Bottom float64
}
type ProofreadingPageLayout struct {
	Text   string
	Lines  []ProofreadingLine
	Height float64
}
type ProofreadingLayoutSnapshot struct {
	Revision      uint64
	StyleRevision int
	Pages         map[proofreading.PageID]ProofreadingPageLayout
	Ends          map[proofreading.PageID]int
	Initial       bool
}
type ProofreadingPageGeometry struct {
	Page          proofreading.PageID
	Width, Height float64
}
type ProofreadingTextStyle struct {
	Size     float64
	Family   string
	Revision int
}
type ProofreadingController struct {
	fine   proofreadingFineCache
	Model  *proofreading.Model
	Layout ProofreadingLayoutSnapshot
}

func NewProofreadingController(m *proofreading.Model) *ProofreadingController {
	return &ProofreadingController{Model: m}
}
func (c *ProofreadingController) Replace(p proofreading.PageID, text string, revision uint64) error {
	return c.Model.ReplaceAtRevision(p, text, revision)
}
func (c *ProofreadingController) AcceptLayout(l ProofreadingLayoutSnapshot) bool {
	if l.Revision != c.Model.Revision() {
		return false
	}
	c.Layout = l
	return true
}

// AdjustLockedPage uses the shaped content's line boundaries. It never derives
// character counts from wheel distance. Geometry and delta use document units.
func (c *ProofreadingController) AdjustLockedPage(page proofreading.PageID, top, height, delta float64) (float64, error) {
	if math.IsNaN(delta) || math.IsInf(delta, 0) || height <= 0 {
		return top, errors.New("invalid scroll geometry")
	}
	l, ok := c.Layout.Pages[page]
	if !ok {
		return top, ErrProofreadingLayoutPending
	}
	owned := c.Model.Text(page)
	if len(l.Text) < len(owned) || l.Text[:len(owned)] != owned {
		return top, ErrProofreadingLayoutPending
	}
	count := utf8.RuneCountInString(owned)
	desired := math.Max(0, top+delta)
	bottom := desired + height
	end := 0
	for _, line := range l.Lines {
		if line.Top < bottom {
			end = line.End
		}
	}
	// Exterior scroll direction controls ownership direction. In particular,
	// scrolling up from the top of a short locked page must never pull text in
	// just because following-page lines also fit in the current viewport.
	if delta < 0 {
		end = min(end, count)
		if desired == 0 {
			// A short locked base can fit together with acquired text in one
			// viewport. At the adjustment start, return every eligible acquired
			// character; the model itself protects the base and later typing.
			end = 0
		}
	} else if delta > 0 {
		end = max(end, count)
	}
	if end != count {
		if err := c.Model.Adjust(page, end-count); err != nil {
			return top, err
		}
	}
	actualCount := utf8.RuneCountInString(c.Model.Text(page))
	actualHeight := 0.0
	for _, line := range l.Lines {
		if line.Start < actualCount || actualCount == 0 {
			actualHeight = line.Bottom
		}
	}
	return math.Min(desired, math.Max(0, actualHeight-height)), nil
}
