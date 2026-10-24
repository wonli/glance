package proofhistory

import (
	"context"
	"glance/internal/proofreading"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProofreadingRemoteRejectsCredentialURL(t *testing.T) {
	if err := ValidateRemoteURL("https://user:secret@example.com/repo"); err == nil {
		t.Fatal("embedded credentials accepted")
	}
}
func TestProofreadingRemotePushToLocalBare(t *testing.T) {
	ctx := context.Background()
	remote := filepath.Join(t.TempDir(), "remote.git")
	if err := exec.Command("git", "init", "--bare", remote).Run(); err != nil {
		t.Skip(err)
	}
	repo := Repository{Directory: t.TempDir()}
	_, err := repo.Confirm(ctx, PageResult{Version: 1, ProjectID: "p", Page: proofreading.Page{ID: 6}, Identity: Identity{Name: "Reader", Email: "r@example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := repo.push(ctx, remote, TransportAuth{})
	if err != nil || hash == "" {
		t.Fatal(hash, err)
	}
}

func TestProofreadingRemoteDivergenceDoesNotOverwriteRemote(t *testing.T) {
	ctx := context.Background()
	remote := filepath.Join(t.TempDir(), "remote.git")
	if err := exec.Command("git", "init", "--bare", remote).Run(); err != nil {
		t.Skip(err)
	}
	first := Repository{Directory: t.TempDir()}
	page := PageResult{Version: 1, ProjectID: "a", Page: proofreading.Page{ID: 6}, Identity: Identity{Name: "Reader", Email: "r@example.com"}}
	if _, err := first.Confirm(ctx, page); err != nil {
		t.Fatal(err)
	}
	head, err := first.push(ctx, remote, TransportAuth{})
	if err != nil {
		t.Fatal(err)
	}
	second := Repository{Directory: t.TempDir()}
	page.ProjectID = "b"
	page.Page.ID = 500
	if _, err = second.Confirm(ctx, page); err != nil {
		t.Fatal(err)
	}
	if _, err = second.push(ctx, remote, TransportAuth{}); err == nil {
		t.Fatal("divergent history overwrote remote")
	}
	out, err := exec.Command("git", "--git-dir", remote, "rev-parse", "refs/heads/main").Output()
	if err != nil || strings.TrimSpace(string(out)) != head {
		t.Fatal(string(out), err)
	}
}
