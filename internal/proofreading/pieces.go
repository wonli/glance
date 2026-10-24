package proofreading

import (
	"strings"
	"unicode/utf8"
)

// pieceTree is an immutable AVL rope. Leaves reference UTF-8 substrings rather
// than copying the source. Cached rune lengths avoid rescanning other boxes.
type pieceSource struct{ text string }

type pieceTree struct {
	source         *pieceSource
	start, end     int
	left, right    *pieceTree
	run            Run
	length, height int
	returnable     int
	originSet      bool
	origin         PageID
	protection     uint8
}

const (
	protectionKeep uint8 = iota
	protectionFalse
	protectionTrue
	protectionTyped
)

func pieceLen(n *pieceTree) int {
	if n == nil {
		return 0
	}
	return n.length
}
func pieceHeight(n *pieceTree) int {
	if n == nil {
		return 0
	}
	return n.height
}
func pieceSpan(r Run, source *pieceSource, start, end int) *pieceTree {
	if start == end {
		return nil
	}
	r.Text = source.text[start:end]
	r.Page = 0
	n := utf8.RuneCountInString(r.Text)
	available := n
	if r.Typed {
		available = 0
	}
	return &pieceTree{run: r, source: source, start: start, end: end, length: n, height: 1, returnable: available}
}
func pieceFromRun(r Run) *pieceTree {
	// Bound the work needed to split a leaf at a rune boundary.
	const chunk = 1024
	var root *pieceTree
	source := &pieceSource{text: r.Text}
	start, count := 0, 0
	for at := range r.Text {
		if count == chunk {
			part := r
			part.Text = r.Text[start:at]
			root = pieceConcat(root, pieceSpan(part, source, start, at))
			r.First += CharacterID(count)
			start = at
			count = 0
		}
		count++
	}
	return pieceConcat(root, pieceSpan(r, source, start, len(r.Text)))
}
func pieceBranch(a, b *pieceTree) *pieceTree {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	available := b.returnable
	if available == b.length {
		available += a.returnable
	}
	return &pieceTree{left: a, right: b, length: a.length + b.length, height: 1 + max(a.height, b.height), returnable: available}
}
func pieceFlags(n *pieceTree, origin PageID, protection uint8) *pieceTree {
	if n == nil {
		return nil
	}
	c := *n
	if c.left == nil {
		c.run.Origin = origin
		switch protection {
		case protectionFalse:
			c.run.Protected = false
		case protectionTrue:
			c.run.Protected = true
		case protectionTyped:
			c.run.Protected = c.run.Typed
		}
	} else {
		c.originSet = true
		c.origin = origin
		if protection != protectionKeep {
			c.protection = protection
		}
	}
	return &c
}
func piecePush(n *pieceTree) *pieceTree {
	if n == nil || !n.originSet {
		return n
	}
	c := *n
	c.left = pieceFlags(n.left, n.origin, n.protection)
	c.right = pieceFlags(n.right, n.origin, n.protection)
	c.originSet = false
	c.protection = protectionKeep
	return &c
}
func pieceBalance(a, b *pieceTree) *pieceTree {
	if pieceHeight(a) > pieceHeight(b)+1 {
		a = piecePush(a)
		if pieceHeight(a.left) >= pieceHeight(a.right) {
			return pieceBranch(a.left, pieceBranch(a.right, b))
		}
		r := piecePush(a.right)
		return pieceBranch(pieceBranch(a.left, r.left), pieceBranch(r.right, b))
	}
	if pieceHeight(b) > pieceHeight(a)+1 {
		b = piecePush(b)
		if pieceHeight(b.right) >= pieceHeight(b.left) {
			return pieceBranch(pieceBranch(a, b.left), b.right)
		}
		l := piecePush(b.left)
		return pieceBranch(pieceBranch(a, l.left), pieceBranch(l.right, b.right))
	}
	return pieceBranch(a, b)
}
func pieceConcat(a, b *pieceTree) *pieceTree {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if a.left == nil && b.left == nil && a.source == b.source && a.end == b.start && a.length+b.length <= 1024 && a.run.First+CharacterID(a.length) == b.run.First && a.run.Origin == b.run.Origin && a.run.Typed == b.run.Typed && a.run.Protected == b.run.Protected {
		return pieceSpan(a.run, a.source, a.start, b.end)
	}

	if a.height > b.height+1 {
		a = piecePush(a)
		return pieceBalance(a.left, pieceConcat(a.right, b))
	}
	if b.height > a.height+1 {
		b = piecePush(b)
		return pieceBalance(pieceConcat(a, b.left), b.right)
	}
	return pieceBranch(a, b)
}
func pieceSplit(n *pieceTree, at int) (*pieceTree, *pieceTree) {
	if at <= 0 {
		return nil, n
	}
	if at >= pieceLen(n) {
		return n, nil
	}
	n = piecePush(n)
	if n.left == nil {
		byteAt, seen := len(n.run.Text), 0
		for pos := range n.run.Text {
			if seen == at {
				byteAt = pos
				break
			}
			seen++
		}
		a, b := n.run, n.run
		a.Text = n.run.Text[:byteAt]
		b.Text = n.run.Text[byteAt:]
		b.First += CharacterID(at)
		return pieceSpan(a, n.source, n.start, n.start+byteAt), pieceSpan(b, n.source, n.start+byteAt, n.end)
	}
	if at < n.left.length {
		a, b := pieceSplit(n.left, at)
		return a, pieceConcat(b, n.right)
	}
	a, b := pieceSplit(n.right, at-n.left.length)
	return pieceConcat(n.left, a), b
}
func pieceVisit(n *pieceTree, visit func(Run)) {
	pieceVisitFlags(n, false, 0, protectionKeep, visit)
}
func pieceVisitFlags(n *pieceTree, originSet bool, origin PageID, protection uint8, visit func(Run)) {
	if n == nil {
		return
	}
	if !originSet && n.originSet {
		originSet = true
		origin = n.origin
	}
	if protection == protectionKeep {
		protection = n.protection
	}
	if n.left == nil {
		r := n.run
		if originSet {
			r.Origin = origin
		}
		switch protection {
		case protectionFalse:
			r.Protected = false
		case protectionTrue:
			r.Protected = true
		case protectionTyped:
			r.Protected = r.Typed
		}
		visit(r)
		return
	}
	pieceVisitFlags(n.left, originSet, origin, protection, visit)
	pieceVisitFlags(n.right, originSet, origin, protection, visit)
}
func pieceString(n *pieceTree) string {
	if n == nil {
		return ""
	}
	if n.left == nil {
		return n.run.Text
	}
	var b strings.Builder
	pieceVisit(n, func(r Run) { b.WriteString(r.Text) })
	return b.String()
}

// Flag-only compatibility actions must remain semantic no-ops. This bounded
// page walk is used for explicit lock/split, not continuous boundary movement.
func pieceFlagsIfChanged(n *pieceTree, origin PageID, protection uint8) *pieceTree {
	changed := false
	pieceVisit(n, func(r Run) {
		if r.Origin != origin {
			changed = true
		}
		switch protection {
		case protectionFalse:
			changed = changed || r.Protected
		case protectionTrue:
			changed = changed || !r.Protected
		case protectionTyped:
			changed = changed || r.Protected != r.Typed
		}
	})
	if !changed {
		return n
	}
	return pieceFlags(n, origin, protection)
}
