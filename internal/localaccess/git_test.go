package localaccess

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrokerGitCommitsBoundsAndFailures(t *testing.T) {
	root := t.TempDir()
	runGitTest(t, root, "init", "-q")
	runGitTest(t, root, "config", "user.name", "Local Access Tester")
	runGitTest(t, root, "config", "user.email", "secret@example.test")
	if err := os.WriteFile(filepath.Join(root, "changed.txt"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "changed.txt")
	initialEnv := gitIdentityAt(time.Date(2024, 12, 31, 23, 59, 0, 0, time.UTC))
	runGitTestEnv(t, root, initialEnv, "commit", "-q", "-m", "Initial summary", "-m", "body-secret-only")
	base := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD"))
	branch := strings.TrimSpace(runGitTest(t, root, "symbolic-ref", "--short", "HEAD"))

	// Add 205 empty commits in one fast-import process so the bound is tested
	// without making the test spend time launching hundreds of Git processes.
	var history strings.Builder
	previous := base
	for i := 0; i < 205; i++ {
		mark := i + 1
		date := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute)
		message := fmt.Sprintf("commit %03d\n\nprivate body text", i)
		fmt.Fprintf(&history, "commit refs/heads/%s\nmark :%d\ncommitter Local Access Tester <secret@example.test> %d +0000\ndata %d\n%s\nfrom %s\n\n", branch, mark, date.Unix(), len(message), message, previous)
		previous = fmt.Sprintf(":%d", mark)
	}
	runGitTestInput(t, root, []byte(history.String()), "fast-import", "--quiet")

	broker, scope := newAuthorizedBroker(t, root, "turn-git")
	page, err := broker.ListGitCommits(context.Background(), "turn-git", scope.ID, time.Time{}, time.Time{}, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Commits) != 200 || !page.HasMore {
		t.Fatalf("default bounded page = (%d commits, hasMore=%v), want (200, true)", len(page.Commits), page.HasMore)
	}
	for _, commit := range page.Commits {
		if len(commit.ID) != 12 || strings.Contains(commit.Author, "@") || strings.Contains(commit.Subject, "private body") {
			t.Fatalf("commit exposed an unexpected field: %#v", commit)
		}
	}

	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	until := since.Add(4 * time.Minute)
	filtered, err := broker.ListGitCommits(context.Background(), "turn-git", scope.ID, since, until, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Commits) == 0 || len(filtered.Commits) > 5 {
		t.Fatalf("date-filtered commit count = %d, want 1–5 commits", len(filtered.Commits))
	}
	for _, commit := range filtered.Commits {
		if commit.Date.Before(since) || commit.Date.After(until) {
			t.Fatalf("commit date %s fell outside requested range", commit.Date)
		}
		for _, path := range commit.Paths {
			if strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
				t.Fatalf("commit path is not relative: %q", path)
			}
		}
	}

	initialOnly, err := broker.ListGitCommits(context.Background(), "turn-git", scope.ID, time.Date(2024, 12, 31, 23, 58, 0, 0, time.UTC), time.Date(2024, 12, 31, 23, 59, 30, 0, time.UTC), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(initialOnly.Commits) != 1 || initialOnly.Commits[0].Subject != "Initial summary" || len(initialOnly.Commits[0].Paths) != 1 || initialOnly.Commits[0].Paths[0] != "changed.txt" {
		t.Fatalf("initial commit metadata = %#v, want summary and relative path only", initialOnly.Commits)
	}

	if _, err := broker.ListGitCommits(context.Background(), "turn-git", scope.ID, until, since, 10); !errors.Is(err, ErrInvalidDateRange) {
		t.Fatalf("inverted date range error = %v, want ErrInvalidDateRange", err)
	}
	if _, err := broker.ListGitCommits(context.Background(), "turn-git", scope.ID, time.Time{}, time.Time{}, 201); !errors.Is(err, ErrInvalidGitLimit) {
		t.Fatalf("over-limit query error = %v, want ErrInvalidGitLimit", err)
	}
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := broker.ListGitCommits(expired, "turn-git", scope.ID, time.Time{}, time.Time{}, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired query context error = %v, want deadline exceeded", err)
	}

	nonRepo := t.TempDir()
	otherBroker, otherScope := newAuthorizedBroker(t, nonRepo, "turn-not-repo")
	if _, err := otherBroker.ListGitCommits(context.Background(), "turn-not-repo", otherScope.ID, time.Time{}, time.Time{}, 10); !errors.Is(err, ErrNotGitRepository) {
		t.Fatalf("non-repository query error = %v, want ErrNotGitRepository", err)
	}

	largeRoot := t.TempDir()
	runGitTest(t, largeRoot, "init", "-q")
	runGitTest(t, largeRoot, "config", "user.name", "Tester")
	runGitTest(t, largeRoot, "config", "user.email", "secret@example.test")
	largeMessage := strings.Repeat("x", maxGitCommandOutput+100)
	largeCommit := fmt.Sprintf("tree %s\nauthor Tester <secret@example.test> 1735689600 +0000\ncommitter Tester <secret@example.test> 1735689600 +0000\n\n%s\n", strings.TrimSpace(runGitTest(t, largeRoot, "mktree")), largeMessage)
	oversizedHash := strings.TrimSpace(runGitTestInput(t, largeRoot, []byte(largeCommit), "hash-object", "-t", "commit", "-w", "--stdin"))
	runGitTest(t, largeRoot, "update-ref", "refs/heads/main", oversizedHash)
	runGitTest(t, largeRoot, "symbolic-ref", "HEAD", "refs/heads/main")
	largeBroker, largeScope := newAuthorizedBroker(t, largeRoot, "turn-large-git")
	if _, err := largeBroker.ListGitCommits(context.Background(), "turn-large-git", largeScope.ID, time.Time{}, time.Time{}, 10); !errors.Is(err, ErrGitOutputTooLarge) {
		t.Fatalf("oversized Git output error = %v, want ErrGitOutputTooLarge", err)
	}
}

func gitIdentityAt(value time.Time) []string {
	date := value.Format("2006-01-02T15:04:05Z07:00")
	return []string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date}
}

func runGitTest(t *testing.T, root string, args ...string) string {
	t.Helper()
	return runGitTestInput(t, root, nil, args...)
}

func runGitTestEnv(t *testing.T, root string, extraEnv []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), extraEnv...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output.String())
	}
	return output.String()
}

func runGitTestInput(t *testing.T, root string, input []byte, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	cmd.Stdin = bytes.NewReader(input)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output.String())
	}
	return output.String()
}
