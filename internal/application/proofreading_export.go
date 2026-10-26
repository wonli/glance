package application

import (
	"fmt"
	"sort"
	"strings"

	"glance/internal/proofreading"
)

// ProofreadingExportPage supplies both possible text sources for one PDF page.
type ProofreadingExportPage struct {
	ID               proofreading.PageID
	Reviewed         bool
	CurrentConfirmed bool
	CurrentText      string
	OriginalText     string
}

// ProofreadingExportUnconfirmedPageError identifies a page whose current text
// no longer matches the saved confirmation hash.
type ProofreadingExportUnconfirmedPageError struct {
	Page proofreading.PageID
}

func (e ProofreadingExportUnconfirmedPageError) Error() string {
	return fmt.Sprintf("proofreading page %d has unconfirmed changes", e.Page)
}

// AssembleProofreadingExport returns a complete manuscript in PDF page order.
// A reviewed page must still match its confirmation; all other pages use the
// text that was imported from the original B document.
func AssembleProofreadingExport(pages []ProofreadingExportPage) (string, error) {
	ordered := append([]ProofreadingExportPage(nil), pages...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })

	var result strings.Builder
	for i, page := range ordered {
		wantID := proofreading.PageID(i + 1)
		if page.ID != wantID {
			return "", fmt.Errorf("proofreading export requires page %d, got page %d", wantID, page.ID)
		}
		if page.Reviewed {
			if !page.CurrentConfirmed {
				return "", ProofreadingExportUnconfirmedPageError{Page: page.ID}
			}
			result.WriteString(page.CurrentText)
			continue
		}
		result.WriteString(page.OriginalText)
	}
	return result.String(), nil
}
