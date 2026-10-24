package application

import (
	"glance/internal/proofhistory"
	"glance/internal/proofreading"
	"testing"
)

func TestProofreadingHistoryRestoreRejectsOtherPageCharacters(t *testing.T) {
	m, _ := proofreading.New("abcd", []proofreading.PageID{1, 2})
	m.AssignInitial([]int{2, 4})
	before := m.Snapshot()
	p := proofhistory.PageResult{Version: 1, Page: proofreading.Page{ID: 1}, Runs: []proofreading.Run{{Page: 1, First: 3, Text: "c"}}}
	if _, err := RestoreProofreadingPage(before, p); err == nil {
		t.Fatal("accepted duplicate ownership")
	}
}
