package application

import (
	"fmt"
	"strings"
	"testing"

	"glance/internal/proofreading"
)

func TestAssembleProofreadingExportUsesCurrentAndOriginalPages(t *testing.T) {
	pages := []ProofreadingExportPage{
		{ID: 3, Reviewed: true, CurrentConfirmed: true, CurrentText: "确认三", OriginalText: "原三"},
		{ID: 1, Reviewed: true, CurrentConfirmed: true, CurrentText: "确认一", OriginalText: "原一"},
		{ID: 2, OriginalText: "原二", CurrentText: "未确认编辑"},
	}

	got, err := AssembleProofreadingExport(pages)
	if err != nil {
		t.Fatalf("AssembleProofreadingExport: %v", err)
	}
	if want := "确认一原二确认三"; got != want {
		t.Fatalf("AssembleProofreadingExport() = %q, want %q", got, want)
	}
}

func TestAssembleProofreadingExportOrdersByPageID(t *testing.T) {
	pages := make([]ProofreadingExportPage, 0, 500)
	for id := 500; id >= 1; id-- {
		page := ProofreadingExportPage{ID: proofreading.PageID(id), OriginalText: "o"}
		if id == 6 || id == 7 || id == 500 {
			page.Reviewed = true
			page.CurrentConfirmed = true
			page.CurrentText = fmt.Sprint(id)
		}
		pages = append(pages, page)
	}

	got, err := AssembleProofreadingExport(pages)
	if err != nil {
		t.Fatalf("AssembleProofreadingExport: %v", err)
	}
	want := strings.Repeat("o", 5) + "6" + "7" + strings.Repeat("o", 492) + "500"
	if got != want {
		t.Fatalf("page 6→500→7 export order incorrect; got length %d, want %d", len(got), len(want))
	}
}

func TestAssembleProofreadingExportRequiresCompleteUniquePages(t *testing.T) {
	tests := []struct {
		name  string
		pages []ProofreadingExportPage
	}{
		{name: "missing page", pages: []ProofreadingExportPage{{ID: 1}, {ID: 3}}},
		{name: "duplicate page", pages: []ProofreadingExportPage{{ID: 1}, {ID: 1}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := AssembleProofreadingExport(test.pages); err == nil {
				t.Fatal("AssembleProofreadingExport() error = nil, want incomplete page sequence error")
			}
		})
	}
}

func TestAssembleProofreadingExportRejectsStaleReviewedPage(t *testing.T) {
	pages := []ProofreadingExportPage{
		{ID: 1, Reviewed: true, CurrentConfirmed: true, CurrentText: "确认页", OriginalText: "原页"},
		{ID: 2, Reviewed: true, CurrentConfirmed: false, CurrentText: "修改后", OriginalText: "原页"},
	}

	_, err := AssembleProofreadingExport(pages)
	if err == nil || !strings.Contains(err.Error(), "2") {
		t.Fatalf("AssembleProofreadingExport() error = %v, want error identifying page 2", err)
	}
}
