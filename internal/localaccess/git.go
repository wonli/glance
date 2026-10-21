package localaccess

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrNotGitRepository  = errors.New("authorized directory is not a Git repository root")
	ErrInvalidDateRange  = errors.New("Git date range is invalid")
	ErrInvalidGitLimit   = errors.New("Git result limit must be between 1 and 200")
	ErrGitTimedOut       = errors.New("Git query timed out")
	ErrGitOutputTooLarge = errors.New("Git query output exceeded its size limit")
	ErrGitQueryFailed    = errors.New("Git query failed")
)

const (
	maxGitCommits        = 200
	maxGitCommandOutput  = 1 << 20
	maxGitPathsPerCommit = 200
	gitCommandTimeout    = 8 * time.Second
)

func (b *Broker) ListGitCommits(ctx context.Context, turnID, scopeID string, since, until time.Time, limit int) (CommitPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	scope, state, err := b.lookupScope(turnID, scopeID)
	if err != nil {
		return CommitPage{}, err
	}
	if err := scope.validateRoot(); err != nil {
		return CommitPage{}, err
	}
	if limit < 1 || limit > maxGitCommits {
		return CommitPage{}, ErrInvalidGitLimit
	}
	if !since.IsZero() && !until.IsZero() && since.After(until) {
		return CommitPage{}, ErrInvalidDateRange
	}
	if err := ctx.Err(); err != nil {
		return CommitPage{}, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	defer cancel()
	rootOutput, err := runGit(queryCtx, scope.root, "--no-pager", "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, ErrGitQueryFailed) {
			return CommitPage{}, ErrNotGitRepository
		}
		return CommitPage{}, err
	}
	if err := scope.validateRoot(); err != nil {
		return CommitPage{}, err
	}
	repoRoot := strings.TrimSuffix(string(rootOutput), "\n")
	canonicalRoot, err := filepath.EvalSymlinks(scope.root)
	if err != nil {
		return CommitPage{}, ErrNotGitRepository
	}
	canonicalRepoRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil || filepath.Clean(canonicalRepoRoot) != filepath.Clean(canonicalRoot) {
		return CommitPage{}, ErrNotGitRepository
	}
	repoInfo, err := os.Stat(canonicalRepoRoot)
	if err != nil || !os.SameFile(scope.rootInfo, repoInfo) {
		return CommitPage{}, ErrNotGitRepository
	}

	args := []string{"--no-pager", "log", "--no-show-signature", "-z", "--name-only", "--format=%H%x00%aI%x00%an%x00%s%x00", "--max-count=" + strconv.Itoa(limit+1)}
	if !since.IsZero() {
		args = append(args, "--after="+since.Format(time.RFC3339Nano))
	}
	if !until.IsZero() {
		args = append(args, "--before="+until.Format(time.RFC3339Nano))
	}
	if err := scope.validateRoot(); err != nil {
		return CommitPage{}, err
	}
	output, err := runGit(queryCtx, scope.root, args...)
	if err != nil {
		return CommitPage{}, err
	}
	if err := scope.validateRoot(); err != nil {
		return CommitPage{}, err
	}
	commits, err := parseGitLog(output)
	if err != nil {
		return CommitPage{}, err
	}
	page := CommitPage{Commits: commits}
	if len(page.Commits) > limit {
		page.HasMore = true
		page.Commits = page.Commits[:limit]
	}
	if !b.isActiveTurn(turnID, state) {
		return CommitPage{}, ErrTurnExpired
	}
	if err := ctx.Err(); err != nil {
		return CommitPage{}, err
	}
	return page, nil
}

func runGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmdCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "git", append([]string{"-C", root}, args...)...)
	cmd.Env = gitEnvironment(os.Environ())
	stdout := &boundedBuffer{limit: maxGitCommandOutput}
	stderr := &boundedBuffer{limit: 16 << 10}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(stdout.err, ErrGitOutputTooLarge) || errors.Is(stderr.err, ErrGitOutputTooLarge) {
			return nil, ErrGitOutputTooLarge
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if cmdCtx.Err() != nil {
			return nil, ErrGitTimedOut
		}
		return nil, ErrGitQueryFailed
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return stdout.Bytes(), nil
}

func gitEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+5)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(strings.ToUpper(name), "GIT_") {
			continue
		}
		result = append(result, entry)
	}
	return append(result,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
	)
}

type boundedBuffer struct {
	buffer bytes.Buffer
	limit  int
	err    error
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 || len(value) > remaining {
		b.err = ErrGitOutputTooLarge
		return 0, b.err
	}
	return b.buffer.Write(value)
}

func (b *boundedBuffer) Bytes() []byte {
	return b.buffer.Bytes()
}

func parseGitLog(output []byte) ([]Commit, error) {
	tokens := bytes.Split(output, []byte{0})
	commits := make([]Commit, 0, min(maxGitCommits+1, len(tokens)/6))
	for index := 0; index < len(tokens); {
		if len(tokens[index]) == 0 {
			index++
			continue
		}
		if !validGitHeader(tokens, index) {
			return nil, fmt.Errorf("%w: malformed record at token %d", ErrGitQueryFailed, index)
		}
		date, err := time.Parse(time.RFC3339, string(tokens[index+1]))
		if err != nil {
			return nil, fmt.Errorf("%w: invalid timestamp at token %d", ErrGitQueryFailed, index+1)
		}
		hash := string(tokens[index])
		if len(hash) > 12 {
			hash = hash[:12]
		}
		commit := Commit{ID: hash, Date: date, Author: string(tokens[index+2]), Subject: string(tokens[index+3]), Paths: make([]string, 0)}
		index += 5
		for index < len(tokens) && !validGitHeader(tokens, index) {
			path := string(tokens[index])
			index++
			if path == "" {
				continue
			}
			if strings.HasPrefix(path, "\n") {
				path = strings.TrimPrefix(path, "\n")
			}
			if !utf8.ValidString(path) || path == "" || isSensitivePath(path) {
				continue
			}
			clean, err := cleanRelativePath(filepath.ToSlash(path))
			if err != nil || clean == "." {
				continue
			}
			if len(commit.Paths) < maxGitPathsPerCommit {
				commit.Paths = append(commit.Paths, filepath.ToSlash(clean))
			}
		}
		commits = append(commits, commit)
		if len(commits) > maxGitCommits+1 {
			return nil, ErrGitQueryFailed
		}
	}
	return commits, nil
}

func validGitHeader(tokens [][]byte, index int) bool {
	if index+4 >= len(tokens) || !isGitObjectID(tokens[index]) || len(tokens[index+4]) != 0 {
		return false
	}
	if _, err := time.Parse(time.RFC3339, string(tokens[index+1])); err != nil {
		return false
	}
	return true
}

func isGitObjectID(value []byte) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}
