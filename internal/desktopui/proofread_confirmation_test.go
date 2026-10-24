package desktopui

import (
	"glance/internal/proofhistory"
	"testing"
)

func TestProofreadingPageStatusSeparatesConfirmationAndSnapshot(t *testing.T) {
	p := proofhistory.PageResult{Version: 1, ProjectID: "test"}
	h := proofreadingHistoryState{confirmed: map[int]string{6: p.ContentHash()}}
	if got := proofreadingStatusKey(true, p.ContentHash(), 6, &h); got != "proofread.history.confirmed_no_snapshot" {
		t.Fatal(got)
	}
	h.snapshots = map[int]string{6: p.ContentHash()}
	if got := proofreadingStatusKey(true, p.ContentHash(), 6, &h); got != "proofread.history.saved" {
		t.Fatal(got)
	}
	if got := proofreadingStatusKey(true, "changed", 6, &h); got != "proofread.history.modified" {
		t.Fatal(got)
	}
}

func TestProofreadingPageStatusDoesNotConfirmEditsDuringPendingSnapshot(t *testing.T) {
	h := proofreadingHistoryState{confirmed: map[int]string{6: "old"}, pending: true, pendingPage: 6}
	if got := proofreadingStatusKey(true, "new", 6, &h); got != "proofread.history.modified" {
		t.Fatal(got)
	}
	if got := proofreadingStatusKey(false, "old", 6, &h); got != "proofread.history.unreviewed" {
		t.Fatal(got)
	}
}

func TestProofreadingLegacyReviewedPageDoesNotInventSnapshot(t *testing.T) {
	if got := proofreadingStatusKey(true, "", 0, &proofreadingHistoryState{}); got != "proofread.history.confirmed_no_snapshot" {
		t.Fatal(got)
	}
}
