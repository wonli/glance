package proofreading

import (
	"errors"
	"unicode/utf8"
)

// Anchor identifies a boundary by adjacent character identities, independent of
// numerical offsets and other pages' edits. Zero means the edge of a page.
type Anchor struct{ Left, Right CharacterID }

func AnchorAt(s Snapshot, page PageID, offset int) (Anchor, error) {
	if offset < 0 {
		return Anchor{}, errors.New("negative caret")
	}
	position := 0
	var left CharacterID
	for _, r := range s.Runs {
		if r.Page != page {
			continue
		}
		n := utf8.RuneCountInString(r.Text)
		if offset >= position && offset < position+n {
			right := r.First + CharacterID(offset-position)
			if offset > position {
				left = right - 1
			}
			return Anchor{Left: left, Right: right}, nil
		}
		position += n
		left = r.First + CharacterID(n) - 1
	}
	if offset != position {
		return Anchor{}, errors.New("caret outside page")
	}
	return Anchor{Left: left}, nil
}
func OffsetAt(s Snapshot, page PageID, anchor Anchor) (int, error) {
	pos := 0
	left := -1
	for _, r := range s.Runs {
		if r.Page != page {
			continue
		}
		n := utf8.RuneCountInString(r.Text)
		end := r.First + CharacterID(n)
		if anchor.Right >= r.First && anchor.Right < end {
			return pos + int(anchor.Right-r.First), nil
		}
		if anchor.Left >= r.First && anchor.Left < end {
			left = pos + int(anchor.Left-r.First) + 1
		}
		pos += n
	}
	if left >= 0 {
		return left, nil
	}
	if anchor.Left == 0 && anchor.Right == 0 && pos == 0 {
		return 0, nil
	}
	return 0, errors.New("caret characters are no longer in page")
}
func (m *Model) SplitAt(page PageID, anchor Anchor, up bool) error {
	at, err := OffsetAt(m.PageSnapshot(page), page, anchor)
	if err != nil {
		return err
	}
	return m.Split(page, at, up)
}

func (m *Model) ReplaceRange(page PageID, start, end Anchor, text string) error {
	a, err := OffsetAt(m.PageSnapshot(page), page, start)
	if err != nil {
		return err
	}
	b, err := OffsetAt(m.PageSnapshot(page), page, end)
	if err != nil {
		return err
	}
	if b < a {
		return errors.New("reversed edit range")
	}
	old := []rune(m.Text(page))
	return m.ReplacePage(page, string(old[:a])+text+string(old[b:]))
}
