package proofhistory

import (
	"context"
	"glance/internal/proofreading"
	"strings"
	"testing"
)

func TestProofreadingPageCommitsAreIsolated(t *testing.T) {
	r := Repository{Directory: t.TempDir()}
	ctx := context.Background()
	if r.Available(ctx) != nil {
		t.Skip("git unavailable")
	}
	p := PageResult{Version: 1, ProjectID: "book", SourceHash: "abc", Identity: Identity{Name: "校对者", Email: "reader@example.com"}}
	for _, id := range []proofreading.PageID{6, 500, 7} {
		p.Page = proofreading.Page{ID: id, Reviewed: true}
		p.Runs = []proofreading.Run{{First: proofreading.CharacterID(id), Page: id, Text: "字"}}
		if _, err := r.Confirm(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	all, err := r.List(ctx, 0)
	for _, entry := range all {
		paths, e := r.command(ctx, "show", "--format=", "--name-only", entry.Hash)
		if e != nil || strings.Count(paths, "pages/") != 1 || strings.Contains(paths, "sources/") {
			t.Fatal(paths, e)
		}
	}
	if err != nil || len(all) != 3 {
		t.Fatal(all, err)
	}
	one, err := r.List(ctx, 6)
	if err != nil || len(one) != 1 {
		t.Fatal(one, err)
	}
	result, err := r.Confirm(ctx, p)
	if err != nil || result.Created {
		t.Fatal(result, err)
	}
	p.Runs[0].Text = "修改"
	if _, err = r.Confirm(ctx, p); err != nil {
		t.Fatal(err)
	}
	one, err = r.List(ctx, 7)
	if err != nil || len(one) != 2 {
		t.Fatal(one, err)
	}
	old, err := r.ReadPage(ctx, one[1].Hash, 7)
	if err != nil || old.Runs[0].Text != "字" {
		t.Fatal(old, err)
	}
}

func TestProofreadingGitWatermarkSurvivesSupersededPages(t *testing.T) {
	repo := Repository{Directory: t.TempDir()}
	ctx := context.Background()
	p := PageResult{Version: 1, ProjectID: "task", Next: 100, Page: proofreading.Page{ID: 6}, Identity: Identity{Name: "Reader", Email: "r@example.com"}}
	if _, err := repo.Confirm(ctx, p); err != nil {
		t.Fatal(err)
	}
	p.Next = 10
	if _, err := repo.Confirm(ctx, p); err != nil {
		t.Fatal(err)
	}
	if got := repo.AllocationWatermark(); got != 100 {
		t.Fatal(got)
	}
}
