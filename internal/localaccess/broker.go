// Package localaccess provides turn-scoped, read-only access to user-selected
// local directories for the built-in desktop Agent.
package localaccess

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	ErrInteractiveAuthorizationUnavailable = errors.New("interactive directory authorization is unavailable")
	ErrDirectorySelectionCancelled         = errors.New("directory selection was cancelled")
	ErrAccessDenied                        = errors.New("directory access was denied")
	ErrInvalidScope                        = errors.New("directory access scope is invalid")
	ErrTurnExpired                         = errors.New("Agent request has ended")
	ErrBrokerClosed                        = errors.New("local access broker is closed")
	ErrInvalidDirectory                    = errors.New("selected directory is unavailable")
	ErrInvalidPurpose                      = errors.New("directory access purpose is invalid")
	ErrScopeIDGeneration                   = errors.New("could not create a directory scope")
)

const maxRequestExcerptRunes = 240

// Prompt is the desktop-owned boundary for choosing a local directory and
// presenting its authorization details to the user.
type Prompt interface {
	SelectDirectory(context.Context, SelectionRequest) (string, error)
	ConfirmDirectory(context.Context, ConsentRequest) (bool, error)
}

type SelectionRequest struct {
	TurnID             string
	WindowID           uint64
	Purpose            string
	UserRequestExcerpt string
}

type ConsentRequest struct {
	TurnID             string
	WindowID           uint64
	Purpose            string
	UserRequestExcerpt string
	FolderName         string
	FolderPath         string
}

// ScopeSummary deliberately excludes the selected directory's absolute path.
type ScopeSummary struct {
	ID         string   `json:"scope_id"`
	FolderName string   `json:"folder_name"`
	Operations []string `json:"operations"`
}

type Entry struct {
	Name         string `json:"name"`
	RelativePath string `json:"relative_path"`
	IsDirectory  bool   `json:"is_directory"`
	Size         int64  `json:"size"`
}

type TextFile struct {
	RelativePath string `json:"relative_path"`
	Content      string `json:"content"`
	Size         int64  `json:"size"`
}

type Commit struct {
	ID      string    `json:"id"`
	Date    time.Time `json:"date"`
	Author  string    `json:"author"`
	Subject string    `json:"subject"`
	Paths   []string  `json:"paths"`
}

type CommitPage struct {
	Commits []Commit `json:"commits"`
	HasMore bool     `json:"has_more"`
}

type Broker struct {
	mu      sync.Mutex
	prompt  Prompt
	closed  bool
	turns   map[string]*turnState
	retired map[string]struct{}
}

type turnState struct {
	requestMu           sync.Mutex
	mu                  sync.RWMutex
	ctx                 context.Context
	cancel              context.CancelFunc
	scopes              map[string]*folderScope
	byPath              map[string]*folderScope
	denied              map[string]struct{}
	authorizationDenied bool
	pickerCancelled     bool
}

type folderScope struct {
	id         string
	root       string
	rootInfo   os.FileInfo
	folderName string
}

func NewBroker(prompt Prompt) *Broker {
	return &Broker{
		prompt:  prompt,
		turns:   make(map[string]*turnState),
		retired: make(map[string]struct{}),
	}
}

func (b *Broker) RequestDirectory(ctx context.Context, turnID string, windowID uint64, purpose, userRequestExcerpt string) (ScopeSummary, error) {
	if b == nil || b.prompt == nil || strings.TrimSpace(turnID) == "" || windowID == 0 {
		return ScopeSummary{}, ErrInteractiveAuthorizationUnavailable
	}
	if strings.TrimSpace(purpose) == "" || utf8.RuneCountInString(purpose) > maxRequestExcerptRunes {
		return ScopeSummary{}, ErrInvalidPurpose
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state, err := b.getOrCreateTurn(turnID)
	if err != nil {
		return ScopeSummary{}, err
	}
	state.requestMu.Lock()
	defer state.requestMu.Unlock()
	if !b.isActiveTurn(turnID, state) {
		return ScopeSummary{}, ErrTurnExpired
	}

	state.mu.RLock()
	cancelled, denied := state.pickerCancelled, state.authorizationDenied
	state.mu.RUnlock()
	if cancelled {
		return ScopeSummary{}, ErrDirectorySelectionCancelled
	}
	if denied {
		return ScopeSummary{}, ErrAccessDenied
	}

	requestCtx, cancel := context.WithCancel(ctx)
	stopTurnCancel := context.AfterFunc(state.ctx, cancel)
	defer func() {
		stopTurnCancel()
		cancel()
	}()

	request := SelectionRequest{
		TurnID:             turnID,
		WindowID:           windowID,
		Purpose:            purpose,
		UserRequestExcerpt: truncateRunes(userRequestExcerpt, maxRequestExcerptRunes),
	}
	selectedPath, err := b.prompt.SelectDirectory(requestCtx, request)
	if err != nil {
		if !b.isActiveTurn(turnID, state) {
			return ScopeSummary{}, ErrTurnExpired
		}
		return ScopeSummary{}, sanitizePromptError(err)
	}
	if !b.isActiveTurn(turnID, state) {
		return ScopeSummary{}, ErrTurnExpired
	}
	if err := requestCtx.Err(); err != nil {
		return ScopeSummary{}, err
	}
	if selectedPath == "" {
		if !b.markPickerCancelled(turnID, state) {
			return ScopeSummary{}, ErrTurnExpired
		}
		return ScopeSummary{}, ErrDirectorySelectionCancelled
	}

	root, folderName, rootInfo, err := canonicalDirectory(selectedPath)
	if err != nil {
		return ScopeSummary{}, err
	}
	if !b.isActiveTurn(turnID, state) {
		return ScopeSummary{}, ErrTurnExpired
	}
	if err := requestCtx.Err(); err != nil {
		return ScopeSummary{}, err
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ScopeSummary{}, ErrBrokerClosed
	}
	if b.turns[turnID] != state {
		b.mu.Unlock()
		return ScopeSummary{}, ErrTurnExpired
	}
	state.mu.RLock()
	existingScope := state.byPath[root]
	_, wasDenied := state.denied[root]
	state.mu.RUnlock()
	b.mu.Unlock()
	if existingScope != nil {
		if err := existingScope.validateRoot(); err != nil {
			return ScopeSummary{}, err
		}
		return existingScope.summary(), nil
	}
	if wasDenied {
		return ScopeSummary{}, ErrAccessDenied
	}

	consent := ConsentRequest{
		TurnID:             turnID,
		WindowID:           windowID,
		Purpose:            purpose,
		UserRequestExcerpt: truncateRunes(userRequestExcerpt, maxRequestExcerptRunes),
		FolderName:         folderName,
		FolderPath:         root,
	}
	allowed, err := b.prompt.ConfirmDirectory(requestCtx, consent)
	if err != nil {
		if !b.isActiveTurn(turnID, state) {
			return ScopeSummary{}, ErrTurnExpired
		}
		return ScopeSummary{}, sanitizePromptError(err)
	}
	if !b.isActiveTurn(turnID, state) {
		return ScopeSummary{}, ErrTurnExpired
	}
	if err := requestCtx.Err(); err != nil {
		return ScopeSummary{}, err
	}
	if !allowed {
		if !b.markDeniedDirectory(turnID, state, root) {
			return ScopeSummary{}, ErrTurnExpired
		}
		return ScopeSummary{}, ErrAccessDenied
	}
	if err := verifyDirectoryIdentity(root, rootInfo); err != nil {
		return ScopeSummary{}, err
	}

	id, err := newScopeID()
	if err != nil {
		return ScopeSummary{}, ErrScopeIDGeneration
	}
	scope := &folderScope{id: id, root: root, rootInfo: rootInfo, folderName: folderName}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ScopeSummary{}, ErrBrokerClosed
	}
	if b.turns[turnID] != state {
		return ScopeSummary{}, ErrTurnExpired
	}
	if err := requestCtx.Err(); err != nil {
		return ScopeSummary{}, err
	}
	state.mu.Lock()
	state.scopes[id] = scope
	state.byPath[root] = scope
	state.mu.Unlock()
	return scope.summary(), nil
}

func (b *Broker) ReleaseTurn(turnID string) {
	if b == nil || turnID == "" {
		return
	}
	b.mu.Lock()
	state := b.turns[turnID]
	if state != nil {
		delete(b.turns, turnID)
		b.retired[turnID] = struct{}{}
	}
	b.mu.Unlock()
	if state != nil {
		state.cancel()
	}
}

func (b *Broker) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	states := make([]*turnState, 0, len(b.turns))
	for _, state := range b.turns {
		states = append(states, state)
	}
	b.turns = make(map[string]*turnState)
	b.retired = make(map[string]struct{})
	b.mu.Unlock()
	for _, state := range states {
		state.cancel()
	}
}

func (b *Broker) getOrCreateTurn(turnID string) (*turnState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrBrokerClosed
	}
	if _, expired := b.retired[turnID]; expired {
		return nil, ErrTurnExpired
	}
	if state := b.turns[turnID]; state != nil {
		return state, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &turnState{
		ctx:    ctx,
		cancel: cancel,
		scopes: make(map[string]*folderScope),
		byPath: make(map[string]*folderScope),
		denied: make(map[string]struct{}),
	}
	b.turns[turnID] = state
	return state, nil
}

func (b *Broker) isActiveTurn(turnID string, state *turnState) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.closed && b.turns[turnID] == state
}

func (b *Broker) markPickerCancelled(turnID string, state *turnState) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.turns[turnID] != state {
		return false
	}
	state.mu.Lock()
	state.pickerCancelled = true
	state.mu.Unlock()
	return true
}

func (b *Broker) markDeniedDirectory(turnID string, state *turnState, root string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.turns[turnID] != state {
		return false
	}
	state.mu.Lock()
	state.denied[root] = struct{}{}
	state.authorizationDenied = true
	state.mu.Unlock()
	return true
}

func canonicalDirectory(path string) (string, string, os.FileInfo, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", "", nil, ErrInvalidDirectory
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", "", nil, ErrInvalidDirectory
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", "", nil, ErrInvalidDirectory
	}
	resolved = filepath.Clean(resolved)
	name := filepath.Base(resolved)
	if name == "." || name == string(filepath.Separator) {
		name = resolved
	}
	return resolved, name, info, nil
}

func verifyDirectoryIdentity(root string, expected os.FileInfo) error {
	if expected == nil {
		return ErrInvalidDirectory
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() || !os.SameFile(expected, info) {
		return ErrPathOutsideScope
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(root) {
		return ErrPathOutsideScope
	}
	return nil
}

func (s *folderScope) validateRoot() error {
	if s == nil {
		return ErrInvalidDirectory
	}
	return verifyDirectoryIdentity(s.root, s.rootInfo)
}

func newScopeID() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func sanitizePromptError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, ErrInteractiveAuthorizationUnavailable) {
		return ErrInteractiveAuthorizationUnavailable
	}
	return fmt.Errorf("%w", ErrInteractiveAuthorizationUnavailable)
}

func (s *folderScope) summary() ScopeSummary {
	return ScopeSummary{
		ID:         s.id,
		FolderName: s.folderName,
		Operations: []string{"list_directory", "read_text_file", "list_git_commits"},
	}
}
