package proofhistory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/proofreading"
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

func TestProofreadingPageCommitUsesFallbackIdentityWhenUnset(t *testing.T) {
	globalConfig := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(globalConfig, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)

	repo := Repository{Directory: t.TempDir()}
	ctx := context.Background()
	if repo.Available(ctx) != nil {
		t.Skip("git unavailable")
	}

	page := PageResult{Version: 1, ProjectID: "book", Page: proofreading.Page{ID: 6, Reviewed: true}}
	if _, err := repo.Confirm(ctx, page); err != nil {
		t.Fatalf("confirming a page should not require a configured identity: %v", err)
	}

	saved, err := repo.ReadPage(ctx, "HEAD", 6)
	if err != nil {
		t.Fatal(err)
	}
	if err := saved.Identity.Validate(); err != nil {
		t.Fatalf("snapshot should record a valid fallback identity: %v", err)
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

func TestProofreadingHistoryPageSummaries(t *testing.T) {
	repo := Repository{Directory: t.TempDir()}
	ctx := context.Background()
	if err := repo.Available(ctx); err != nil {
		t.Skip("git unavailable")
	}
	page := func(id proofreading.PageID, content string) PageResult {
		return PageResult{
			Version: 1, ProjectID: "book", SourceHash: "source",
			Page:     proofreading.Page{ID: id, Reviewed: true},
			Runs:     []proofreading.Run{{First: proofreading.CharacterID(id), Page: id, Text: content}},
			Identity: Identity{Name: "Reader", Email: "reader@example.com"},
		}
	}
	for _, p := range []PageResult{page(6, "旧内容"), page(500, "远页"), page(7, "相邻页"), page(6, "新内容")} {
		if _, err := repo.Confirm(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	got, err := repo.ListPages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d page summaries, want 3: %+v", len(got), got)
	}
	if got[0].Page != 6 || got[0].CommitCount != 2 || got[0].Latest.Message != "第 6 页：确认校对" {
		t.Fatalf("latest page summary = %+v, want page 6 with two commits first", got[0])
	}
	if got[1].Page != 7 || got[1].CommitCount != 1 {
		t.Fatalf("second summary = %+v, want page 7 with one commit", got[1])
	}
	if got[2].Page != 500 || got[2].CommitCount != 1 {
		t.Fatalf("third summary = %+v, want page 500 with one commit", got[2])
	}
}

func TestProofreadingHistoryPageChanges(t *testing.T) {
	repo := Repository{Directory: t.TempDir()}
	ctx := context.Background()
	if err := repo.Available(ctx); err != nil {
		t.Skip("git unavailable")
	}
	page := PageResult{
		Version: 1, ProjectID: "book", SourceHash: "source",
		Page:     proofreading.Page{ID: 6, Reviewed: true},
		Runs:     []proofreading.Run{{First: 6, Page: 6, Text: "旧段落"}},
		Identity: Identity{Name: "Reader", Email: "reader@example.com"},
	}
	first, err := repo.Confirm(ctx, page)
	if err != nil {
		t.Fatal(err)
	}
	page.Runs[0].Text = "全新段落"
	second, err := repo.Confirm(ctx, page)
	if err != nil {
		t.Fatal(err)
	}
	pageFile := filepath.Join(repo.Directory, pagePath(6))
	data, err := os.ReadFile(pageFile)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "全新段落", "第三次修改", 1))
	if err := os.WriteFile(pageFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo.Directory, "notes.txt"), []byte("note\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.command(ctx, "add", "--", pagePath(6), "notes.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.command(ctx, "-c", "user.name=Reader", "-c", "user.email=reader@example.com", "commit", "-m", "change two files"); err != nil {
		t.Fatal(err)
	}
	thirdHash, err := repo.command(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	changes, err := repo.PageChanges(ctx, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 || changes[0].Entry.Hash != thirdHash || changes[1].Entry.Hash != second.Hash || changes[2].Entry.Hash != first.Hash {
		t.Fatalf("page changes = %+v, want all three commits newest-first", changes)
	}
	latest := changes[0]
	changedPaths := strings.Join(latest.Paths, " ")
	if latest.Status != "modified" || latest.Additions != 2 || latest.Deletions != 1 || latest.FilesChanged != 2 || !strings.Contains(changedPaths, pagePath(6)) || !strings.Contains(changedPaths, "notes.txt") {
		t.Fatalf("latest change metadata = %+v, want whole-commit totals and both changed paths", latest)
	}
	if changes[1].FilesChanged != 1 || changes[2].FilesChanged != 1 {
		t.Fatalf("single-file change counts = %d and %d, want 1 each", changes[1].FilesChanged, changes[2].FilesChanged)
	}
	if !strings.Contains(latest.Diff, "全新段落") || !strings.Contains(latest.Diff, "第三次修改") || !strings.Contains(latest.Diff, "note") {
		t.Fatalf("latest bounded diff = %q, want page and notes changes from the whole commit", latest.Diff)
	}
	if changes[2].Status != "added" {
		t.Fatalf("first snapshot status = %q, want added", changes[2].Status)
	}
}

func TestPageChangesIncludesCompletePatch(t *testing.T) {
	repo := Repository{Directory: t.TempDir()}
	ctx := context.Background()
	if err := repo.Available(ctx); err != nil {
		t.Skip("git unavailable")
	}
	page := PageResult{Version: 1, ProjectID: "book", SourceHash: "source", Page: proofreading.Page{ID: 6, Reviewed: true}, Runs: []proofreading.Run{{First: 6, Page: 6, Text: strings.Repeat("完整内容", 7000) + "历史末尾"}}, Identity: Identity{Name: "Reader", Email: "reader@example.com"}}
	entry, err := repo.Confirm(ctx, page)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := repo.PageChanges(ctx, 6)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := repo.command(ctx, "show", "--no-color", "--format=", "--patch", "--unified=0", entry.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Diff != expected {
		t.Fatal("history patch was truncated or lost file/hunk context")
	}
}

func TestPageChangesShowsDeletedCharactersInsteadOfJSON(t *testing.T) {
	repo := Repository{Directory: t.TempDir()}
	ctx := context.Background()
	if err := repo.Available(ctx); err != nil {
		t.Skip("git unavailable")
	}
	page := PageResult{Version: 1, ProjectID: "book", SourceHash: "source", Page: proofreading.Page{ID: 6, Reviewed: true}, Runs: []proofreading.Run{{First: 6, Page: 6, Text: "春眠不觉晓，处处闻啼鸟"}}, Identity: Identity{Name: "Reader", Email: "reader@example.com"}}
	if _, err := repo.Confirm(ctx, page); err != nil {
		t.Fatal(err)
	}
	page.Runs[0].Text = "春眠觉晓，处处闻鸟"
	if _, err := repo.Confirm(ctx, page); err != nil {
		t.Fatal(err)
	}
	changes, err := repo.PageChanges(ctx, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || !changes[0].TextAvailable {
		t.Fatal("missing text history")
	}
	var deleted string
	for _, span := range changes[0].TextChanges {
		if span.Kind == TextDeleted {
			deleted += span.Text
		}
		if span.Kind == TextAdded {
			t.Fatalf("unexpected inserted text: %q", span.Text)
		}
	}
	if deleted != "不啼" {
		t.Fatalf("deleted text=%q", deleted)
	}
	if len(changes[1].TextChanges) != 1 || changes[1].TextChanges[0].Kind != TextAdded {
		t.Fatal("initial confirmation must add body text")
	}
	// Metadata-only change remains a snapshot, without inventing body edits.
	page.Next = 99999
	data, err := json.MarshalIndent(page, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(repo.Directory, pagePath(6)), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.command(ctx, "add", "--", pagePath(6)); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.command(ctx, "-c", "user.name=Reader", "-c", "user.email=reader@example.com", "commit", "-m", "metadata only"); err != nil {
		t.Fatal(err)
	}
	changes, err = repo.PageChanges(ctx, 6)
	if err != nil {
		t.Fatal(err)
	}
	for _, span := range changes[0].TextChanges {
		if span.Kind != TextUnchanged {
			t.Fatalf("metadata produced body edit: %+v", span)
		}
	}
	// Deletion has no current page JSON, but must still expose the removed text.
	if _, err = repo.command(ctx, "rm", "--", pagePath(6)); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.command(ctx, "-c", "user.name=Reader", "-c", "user.email=reader@example.com", "commit", "-m", "delete page"); err != nil {
		t.Fatal(err)
	}
	changes, err = repo.PageChanges(ctx, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes[0].TextChanges) != 1 || changes[0].TextChanges[0].Kind != TextDeleted || changes[0].TextChanges[0].Text != page.Text() {
		t.Fatal("deleted page did not retain removed body")
	}
}
