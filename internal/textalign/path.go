package textalign

import (
	"context"
	"errors"
	"math"
)

// Align processes every page in supplied order, evaluating forward-only candidate
// paths and an explicit unmatched-page state. It never assigns gaps or modifies B.
func Align(ctx context.Context, pages []Page, markdown string, opt Options) ([]Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(pages) > 2000 {
		return nil, errors.New("textalign: at most 2000 pages per run")
	}
	for i, p := range pages {
		if p.Number <= 0 || (i > 0 && p.Number <= pages[i-1].Number) {
			return nil, errors.New("textalign: page numbers must be positive and increasing")
		}
	}
	source, err := NewSource(markdown)
	if err != nil {
		return nil, err
	}
	idx := newIndex(source)
	results := make([]Result, len(pages))
	for i, page := range pages {
		var stats SearchStats
		cs, limited, err := idx.candidates(ctx, page, opt, &stats)
		if err != nil {
			return nil, err
		}
		status := "unmatched"
		if limited {
			status = "limited"
		}
		results[i] = Result{Page: page.Number, Status: status, Candidates: cs, Search: stats, searchLimited: limited}
		if limited {
			results[i].Reasons = append(results[i].Reasons, "search budget or candidate limit reached")
		}
	}
	if err := resolvePaths(ctx, pages, results, opt); err != nil {
		return nil, err
	}
	if err := refine(ctx, idx, pages, results, opt); err != nil {
		return nil, err
	}
	return results, nil
}

func resolvePaths(ctx context.Context, pages []Page, results []Result, opt Options) error {
	for i := range results {
		r := &results[i]
		r.Selected = nil
		r.supported = nil
		r.canSkip = false
		r.pathChecked = false
		r.Reasons = nil
		r.Status = "unmatched"
		if r.searchLimited {
			r.Status = "limited"
			r.Reasons = append(r.Reasons, "search budget or candidate limit reached")
		}
	}
	pathBudget := opt.MaxPathWork
	if pathBudget <= 0 {
		pathBudget = 1_000_000
	}
	pathBudget = min(pathBudget, 4_000_000)
	solver := newPathSolver(results, pathBudget)
	best, forward, previous, complete, err := solver.forward(ctx, -1)
	if err != nil {
		return err
	}
	if !complete {
		for i := range results {
			results[i].Status = "limited"
			results[i].Reasons = append(results[i].Reasons, "page path budget reached")
		}
		return nil
	}
	choices := make([]int, len(results))
	for i := range choices {
		choices[i] = -1
	}
	for id := best.node; id >= 0; id = previous[id] {
		n := solver.nodes[id]
		choices[n.page] = n.choice
	}
	suffix, checked, err := solver.suffix(ctx)
	if err != nil {
		return err
	}
	for i, choice := range choices {
		r := &results[i]
		if choice < 0 {
			if len(r.Candidates) > 0 {
				r.Reasons = append(r.Reasons, "no non-overlapping placement selected by best page path")
				if !checked {
					r.Status = "limited"
					r.Reasons = append(r.Reasons, "page path confidence budget reached")
				} else if r.Status != "limited" {
					r.pathChecked = true
					r.canSkip = true
					for _, id := range solver.groups[i] {
						margin := math.Max(3, solver.nodes[id].candidate.Score*0.08)
						if best.score-forward[id]-suffix[id] <= margin {
							r.Status = "ambiguous"
							r.Reasons = append(r.Reasons, "competing path assigns this page")
							r.supported = append(r.supported, solver.nodes[id].choice)
						}
					}
				}
			}
			continue
		}
		c := r.Candidates[choice]
		r.supported = append(r.supported, choice)
		r.Selected = &c
		unrecognized := false
		for _, b := range pages[i].Blocks {
			if b.Text == "" && (b.Label == "image" || b.Label == "header_image" || b.Label == "seal") {
				unrecognized = true
				break
			}
		}
		ambiguous := false
		margin := math.Max(3, c.Score*0.08)
		confidenceChecked := checked
		if checked {
			for _, id := range solver.groups[i] {
				n := solver.nodes[id]
				if n.choice != choice && best.score-(forward[id]+suffix[id]) <= margin {
					ambiguous = true
					r.supported = append(r.supported, n.choice)
				}
			}
			// An alternative can skip this page entirely and choose a competing
			// neighboring page. Check that route as well, within an explicit budget.
			alternative, _, _, ok, err := solver.forward(ctx, i)
			if err != nil {
				return err
			}
			confidenceChecked = ok
			r.pathChecked = ok
			r.canSkip = ok && best.score-alternative.score <= margin
			if !ok {
				checked = false
			}
			if ok && best.score-alternative.score <= margin {
				ambiguous = true
			}
		}
		if !confidenceChecked {
			r.Status = "limited"
			r.Reasons = append(r.Reasons, "page path confidence budget reached")
		}
		if r.Status == "limited" {
			continue
		}
		r.Status = "matched"
		if ambiguous || c.Joint || r.Search.InternalPins > 0 || c.Coverage < 0.85 || c.Reordered || c.SourceCoverage < 0.85 || unrecognized {
			r.Status = "ambiguous"
			if r.Search.InternalPins > 0 {
				r.Reasons = append(r.Reasons, "candidate retrieval scoped by unique internal text fragments")
			}
			if c.Joint {
				r.Reasons = append(r.Reasons, "joint alignment within anchors requires review")
			}
			if ambiguous {
				r.Reasons = append(r.Reasons, "competing page placements")
			}
			if c.Coverage < 0.85 {
				r.Reasons = append(r.Reasons, "incomplete OCR text coverage")
			}
			if c.Reordered {
				r.Reasons = append(r.Reasons, "text blocks occur in a different order")
			}
			if c.SourceCoverage < 0.85 {
				r.Reasons = append(r.Reasons, "unexplained source text between matched blocks")
			}
			if unrecognized {
				r.Reasons = append(r.Reasons, "image or seal regions have no recognized text")
			}
		}
		r.Reasons = append(r.Reasons, "source punctuation, markup and unrecognized regions are not assigned page ownership")
	}
	return nil
}
