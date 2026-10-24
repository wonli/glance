package textalign

func placed(r Result) bool {
	return !r.hasUnread && r.Selected != nil && r.Status != "limited" && r.pathChecked && !r.canSkip && !r.Selected.Reordered && r.Context != "limited" && r.Context != "unsupported"
}
func edge(c Candidate, q pageQuery, tail bool) (int, bool) {
	if len(q.parts) == 0 {
		return 0, false
	}
	part := q.parts[0]
	if tail {
		part = q.parts[len(q.parts)-1]
	}
	for _, e := range c.Evidence {
		if e.queryOrder != part.order {
			continue
		}
		if tail && e.QueryRange.End == len(part.text) {
			return e.Range.End, true
		}
		if !tail && e.QueryRange.Start == 0 {
			return e.Range.Start, true
		}
	}
	return 0, false
}
func inferBoundaries(queries []pageQuery, results []Result) {
	for i := 0; i+1 < len(results); i++ {
		a, b := &results[i], &results[i+1]
		if b.Page != a.Page+1 {
			continue
		}
		cut := &Boundary{NextPage: b.Page, Status: "unresolved", Method: "adjacent-evidence", Reason: "page identity or edge evidence is incomplete"}
		a.BoundaryAfter = cut
		if !placed(*a) || !placed(*b) {
			continue
		}
		minimum, maximum := int(^uint(0)>>1), -1
		valid := true
		for _, li := range a.supported {
			for _, ri := range b.supported {
				left, right := a.Candidates[li], b.Candidates[ri]
				if left.end > right.start {
					continue
				}
				end, ok := edge(left, queries[i], true)
				if !ok {
					valid = false
					continue
				}
				start, ok := edge(right, queries[i+1], false)
				if !ok {
					valid = false
					continue
				}
				minimum = min(minimum, end)
				maximum = max(maximum, start)
			}
		}
		if !valid || maximum < minimum {
			continue
		}
		cut.Min, cut.Max = minimum, maximum
		cut.Status = "range"
		cut.Reason = "envelope of supported page-edge cuts; intervening markup remains unassigned"
		if minimum == maximum {
			cut.Status = "point"
		}
	}
}
