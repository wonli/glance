// Package proofhistory stores only explicitly confirmed pages in a task-local Git repository.
package proofhistory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"glance/internal/proofreading"
)

type Identity struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (i Identity) Validate() error {
	if strings.TrimSpace(i.Name) == "" || !strings.Contains(i.Email, "@") || strings.ContainsAny(i.Name+i.Email, "\r\n\x00<>") {
		return errors.New("校对者姓名或邮箱无效")
	}
	return nil
}

type PageResult struct {
	Next        proofreading.CharacterID `json:"next,omitempty"`
	Version     int                      `json:"version"`
	ProjectID   string                   `json:"project_id"`
	SourceHash  string                   `json:"source_hash"`
	Page        proofreading.Page        `json:"page"`
	Runs        []proofreading.Run       `json:"runs"`
	ConfirmedAt time.Time                `json:"confirmed_at"`
	Identity    Identity                 `json:"identity"`
}

func (p PageResult) Text() string {
	var b strings.Builder
	for _, r := range p.Runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// ContentHash omits timestamps and author so unchanged confirmations are no-ops.
func (p PageResult) ContentHash() string {
	p.Next = 0
	p.ConfirmedAt = time.Time{}
	p.Identity = Identity{}
	data, _ := json.Marshal(p)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

type CommitResult struct {
	Hash, ContentHash string
	Created           bool
}
type Entry struct{ Hash, Author, Date, Message string }
type Repository struct{ Directory string }

func (r Repository) Available(ctx context.Context) error { _, err := exec.LookPath("git"); return err }
func (r Repository) command(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	base := []string{"-c", "core.hooksPath=" + os.DevNull, "-c", "commit.gpgSign=false", "-c", "tag.gpgSign=false", "-c", "core.autocrlf=false", "-C", r.Directory}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		command := gitOperation(args)
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return "", fmt.Errorf("git %s: %w: %s", command, err, detail)
		}
		return "", fmt.Errorf("git %s: %w", command, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func gitOperation(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" || args[i] == "-C" {
			i++
			continue
		}
		if !strings.HasPrefix(args[i], "-") {
			return args[i]
		}
	}
	return "command"
}

func pagePath(id proofreading.PageID) string { return fmt.Sprintf("pages/%06d.json", id) }
func (r Repository) Confirm(ctx context.Context, p PageResult) (CommitResult, error) {
	var result CommitResult
	if err := p.Identity.Validate(); err != nil {
		return result, err
	}
	if p.Page.ID == 0 || p.ProjectID == "" {
		return result, errors.New("invalid page identity")
	}
	if err := r.Available(ctx); err != nil {
		return result, err
	}
	if err := os.MkdirAll(r.Directory, 0700); err != nil {
		return result, err
	}
	gitPath := filepath.Join(r.Directory, ".git")
	if _, err := os.Stat(gitPath); errors.Is(err, os.ErrNotExist) {
		if _, err = r.command(ctx, "init", "-b", "main"); err != nil {
			return result, err
		}
	} else if err != nil {
		return result, err
	}
	if err := r.reserveWatermark(p.Next); err != nil {
		return result, err
	}
	path := pagePath(p.Page.ID)
	result.ContentHash = p.ContentHash()
	old, err := r.ReadPage(ctx, "HEAD", p.Page.ID)
	if err == nil && old.ContentHash() == result.ContentHash {
		result.Hash, _ = r.command(ctx, "rev-parse", "HEAD")
		return result, nil
	}
	if p.ConfirmedAt.IsZero() {
		p.ConfirmedAt = time.Now().UTC()
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return result, err
	}
	if err = os.MkdirAll(filepath.Join(r.Directory, "pages"), 0700); err != nil {
		return result, err
	}
	tmp, err := os.CreateTemp(filepath.Join(r.Directory, "pages"), ".confirm-*")
	if err != nil {
		return result, err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return result, err
	}
	if err = tmp.Close(); err != nil {
		return result, err
	}
	if err = os.Rename(name, filepath.Join(r.Directory, path)); err != nil {
		return result, err
	}
	if _, err = r.command(ctx, "add", "--", path); err != nil {
		return result, err
	}
	_, err = r.command(ctx, "-c", "user.name="+p.Identity.Name, "-c", "user.email="+p.Identity.Email, "commit", "--only", "-m", fmt.Sprintf("第 %d 页：确认校对", p.Page.ID), "--", path)
	if err != nil {
		return result, err
	}
	result.Hash, err = r.command(ctx, "rev-parse", "HEAD")
	result.Created = err == nil
	return result, err
}

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

func (r Repository) ReadPage(ctx context.Context, commit string, id proofreading.PageID) (PageResult, error) {
	var p PageResult
	if commit != "HEAD" && !commitPattern.MatchString(commit) {
		return p, errors.New("invalid commit")
	}
	data, err := r.command(ctx, "show", commit+":"+pagePath(id))
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(data), &p)
	if err == nil && (p.Version != 1 || p.Page.ID != id) {
		err = errors.New("invalid historical page")
	}
	return p, err
}
func (r Repository) List(ctx context.Context, id proofreading.PageID) ([]Entry, error) {
	args := []string{"log", "--format=%H%x1f%an%x1f%aI%x1f%s"}
	if id != 0 {
		args = append(args, "--", pagePath(id))
	}
	data, err := r.command(ctx, args...)
	if err != nil {
		return nil, err
	}
	var result []Entry
	for _, line := range strings.Split(data, "\n") {
		v := strings.SplitN(line, "\x1f", 4)
		if len(v) == 4 {
			result = append(result, Entry{v[0], v[1], v[2], v[3]})
		}
	}
	return result, nil
}

// Diff returns a bounded, page-only patch for a selected historical commit.
func (r Repository) Diff(ctx context.Context, commit string) (string, error) {
	if !commitPattern.MatchString(commit) {
		return "", errors.New("invalid commit")
	}
	return r.command(ctx, "show", "--format=fuller", "--stat", "--patch", commit, "--", "pages")
}

// AllocationWatermark survives undo and interrupted commits, including superseded page versions.
func (r Repository) AllocationWatermark() proofreading.CharacterID {
	data, err := os.ReadFile(filepath.Join(r.Directory, ".git", "glance-next"))
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return proofreading.CharacterID(n)
}
func (r Repository) reserveWatermark(next proofreading.CharacterID) error {
	next = max(next, r.AllocationWatermark())
	if next == 0 {
		return nil
	}
	path := filepath.Join(r.Directory, ".git", "glance-next")
	f, err := os.CreateTemp(filepath.Dir(path), ".next-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.WriteString(strconv.FormatUint(uint64(next), 10)); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func (r Repository) Head(ctx context.Context) (string, error) {
	return r.command(ctx, "rev-parse", "HEAD")
}
