package proofhistory

import (
	"context"
	"strings"

	"glance/internal/proofreading"
)

type TextChangeKind uint8

const (
	TextUnchanged TextChangeKind = iota
	TextDeleted
	TextAdded
)

type TextChange struct {
	Kind TextChangeKind
	Text string
}

// CompareText compares Unicode characters, retaining all text and context.
// Myers' edit search is bounded; very dissimilar large middles become a full
// replacement rather than consuming quadratic memory or dropping content.
func CompareText(before, after string) []TextChange {
	a, b := []rune(before), []rune(after)
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	var result []TextChange
	appendSpan := func(kind TextChangeKind, text string) {
		if text != "" {
			if n := len(result); n > 0 && result[n-1].Kind == kind {
				result[n-1].Text += text
			} else {
				result = append(result, TextChange{kind, text})
			}
		}
	}
	appendSpan(TextUnchanged, string(a[:prefix]))
	for _, span := range compareMiddle(a[prefix:len(a)-suffix], b[prefix:len(b)-suffix]) {
		appendSpan(span.Kind, span.Text)
	}
	appendSpan(TextUnchanged, string(a[len(a)-suffix:]))
	return result
}

func compareMiddle(a, b []rune) []TextChange {
	replacement := func() []TextChange {
		var out []TextChange
		if len(a) > 0 {
			out = append(out, TextChange{TextDeleted, string(a)})
		}
		if len(b) > 0 {
			out = append(out, TextChange{TextAdded, string(b)})
		}
		return out
	}
	if len(a) == 0 || len(b) == 0 {
		return replacement()
	}
	limit := min(len(a)+len(b), 512)
	offset := limit + 1
	v := make([]int, 2*limit+3)
	trace := make([][]int, 0, limit+1)
	work := 0
	for d := 0; d <= limit; d++ {
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			x := 0
			if k == -d || k != d && v[offset+k-1] < v[offset+k+1] {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < len(a) && y < len(b) && a[x] == b[y] {
				x++
				y++
				work++
			}
			work++
			if work > 4000000 {
				return replacement()
			}
			v[offset+k] = x
			if x >= len(a) && y >= len(b) {
				return reconstructTextChanges(a, b, trace, d, offset)
			}
		}
	}
	return replacement()
}

func reconstructTextChanges(a, b []rune, trace [][]int, depth, offset int) []TextChange {
	type edit struct {
		kind TextChangeKind
		char rune
	}
	edits := make([]edit, 0, len(a)+len(b))
	x, y := len(a), len(b)
	for d := depth; d >= 0; d-- {
		v := trace[d]
		k := x - y
		previous := k - 1
		if k == -d || k != d && v[offset+k-1] < v[offset+k+1] {
			previous = k + 1
		}
		px := v[offset+previous]
		py := px - previous
		for x > px && y > py {
			x--
			y--
			edits = append(edits, edit{TextUnchanged, a[x]})
		}
		if d == 0 {
			break
		}
		if x == px {
			y--
			edits = append(edits, edit{TextAdded, b[y]})
		} else {
			x--
			edits = append(edits, edit{TextDeleted, a[x]})
		}
	}
	var result []TextChange
	var text strings.Builder
	kind := TextUnchanged
	flush := func() {
		if text.Len() > 0 {
			result = append(result, TextChange{kind, text.String()})
			text.Reset()
		}
	}
	for i := len(edits) - 1; i >= 0; i-- {
		if edits[i].kind != kind {
			flush()
			kind = edits[i].kind
		}
		text.WriteRune(edits[i].char)
	}
	flush()
	return result
}

func (r Repository) pageTextChanges(ctx context.Context, commit, status string, page proofreading.PageID) ([]TextChange, error) {
	var before, after string
	if status != "deleted" {
		p, err := r.ReadPage(ctx, commit, page)
		if err != nil {
			return nil, err
		}
		after = p.Text()
	}
	parents, err := r.command(ctx, "rev-list", "--parents", "-n", "1", commit)
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(parents)
	if status != "added" && len(fields) > 1 {
		p, err := r.ReadPage(ctx, fields[1], page)
		if err != nil {
			return nil, err
		}
		before = p.Text()
	}
	return CompareText(before, after), nil
}
