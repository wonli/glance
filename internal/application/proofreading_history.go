package application

import (
	"errors"
	"glance/internal/proofhistory"
	"glance/internal/proofreading"
	"unicode/utf8"
)

// RestoreProofreadingPage builds and validates a candidate without mutating the live model.
func RestoreProofreadingPage(current proofreading.Snapshot, result proofhistory.PageResult) (proofreading.Snapshot, error) {
	candidate := current
	candidate.Pages = append([]proofreading.Page(nil), current.Pages...)
	candidate.Runs = nil
	found := false
	for i, p := range candidate.Pages {
		if p.ID == result.Page.ID {
			found = true
			p = result.Page
			p.Reviewed = current.Pages[i].Reviewed
			p.Manual = true
			candidate.Pages[i] = p
		}
		if p.ID == result.Page.ID {
			for _, r := range result.Runs {
				if r.Page != p.ID {
					return current, errors.New("historical run belongs to another page")
				}
				candidate.Runs = append(candidate.Runs, r)
				candidate.Next = max(candidate.Next, r.First+proofreading.CharacterID(utf8.RuneCountInString(r.Text)))
			}
		} else {
			for _, r := range current.Runs {
				if r.Page == p.ID {
					candidate.Runs = append(candidate.Runs, r)
				}
			}
		}
	}
	if !found {
		return current, errors.New("historical page does not exist")
	}
	candidate.Revision++
	if _, err := proofreading.Restore(candidate); err != nil {
		return current, err
	}
	return candidate, nil
}
