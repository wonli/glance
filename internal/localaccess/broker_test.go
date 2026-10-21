package localaccess

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type scriptedPrompt struct {
	mu sync.Mutex

	folders   []string
	decisions []bool
	selects   int
	confirms  int

	confirmStarted chan struct{}
	confirmReply   chan bool
}

func (p *scriptedPrompt) SelectDirectory(_ context.Context, _ SelectionRequest) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.selects++
	if len(p.folders) == 0 {
		return "", errors.New("unexpected directory selection")
	}
	folder := p.folders[0]
	p.folders = p.folders[1:]
	return folder, nil
}

func (p *scriptedPrompt) ConfirmDirectory(ctx context.Context, _ ConsentRequest) (bool, error) {
	p.mu.Lock()
	p.confirms++
	started, reply := p.confirmStarted, p.confirmReply
	if reply == nil {
		if len(p.decisions) == 0 {
			p.mu.Unlock()
			return false, errors.New("unexpected directory confirmation")
		}
		decision := p.decisions[0]
		p.decisions = p.decisions[1:]
		p.mu.Unlock()
		return decision, nil
	}
	p.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	select {
	case decision := <-reply:
		return decision, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (p *scriptedPrompt) counts() (selects, confirms int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.selects, p.confirms
}

func newAuthorizedBroker(t *testing.T, root, turnID string) (*Broker, ScopeSummary) {
	t.Helper()
	broker := NewBroker(&scriptedPrompt{folders: []string{root}, decisions: []bool{true}})
	t.Cleanup(broker.Close)
	scope, err := broker.RequestDirectory(context.Background(), turnID, 1, "read selected folder", "summarize this month")
	if err != nil {
		t.Fatal(err)
	}
	return broker, scope
}

func TestBrokerScopesAreTurnAndFolderBound(t *testing.T) {
	rootA := t.TempDir()
	rootB := filepath.Join(t.TempDir(), "other-project")
	rootDenied := filepath.Join(t.TempDir(), "denied-project")
	if err := os.Mkdir(rootB, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(rootDenied, 0o755); err != nil {
		t.Fatal(err)
	}

	prompt := &scriptedPrompt{
		folders:   []string{rootA, rootA, rootB, rootA, rootDenied, ""},
		decisions: []bool{true, true, true, false},
	}
	broker := NewBroker(prompt)
	t.Cleanup(broker.Close)

	first, err := broker.RequestDirectory(context.Background(), "turn-1", 11, "summarize work", "summarize this month")
	if err != nil {
		t.Fatal(err)
	}
	reused, err := broker.RequestDirectory(context.Background(), "turn-1", 11, "read project", "summarize this month")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || reused.ID != first.ID {
		t.Fatalf("same turn and folder scope IDs = %q, %q; want the same non-empty ID", first.ID, reused.ID)
	}
	if first.FolderName != filepath.Base(rootA) {
		t.Fatalf("folder name = %q, want %q", first.FolderName, filepath.Base(rootA))
	}

	otherFolder, err := broker.RequestDirectory(context.Background(), "turn-1", 11, "read another project", "summarize this month")
	if err != nil {
		t.Fatal(err)
	}
	if otherFolder.ID == first.ID {
		t.Fatal("a different selected folder reused the first folder's scope ID")
	}
	newTurn, err := broker.RequestDirectory(context.Background(), "turn-2", 11, "read project", "summarize this month")
	if err != nil {
		t.Fatal(err)
	}
	if newTurn.ID == first.ID {
		t.Fatal("a new turn reused a previous turn's scope ID")
	}
	if _, err := broker.RequestDirectory(context.Background(), "turn-1", 11, "read denied project", "summarize this month"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("denying a folder = %v, want ErrAccessDenied", err)
	}
	selectsBeforeRetry, confirmsBeforeRetry := prompt.counts()
	if _, err := broker.RequestDirectory(context.Background(), "turn-1", 11, "retry denied project", "summarize this month"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("retrying a denied folder = %v, want ErrAccessDenied", err)
	}
	_, confirmsAfterRetry := prompt.counts()
	if confirmsAfterRetry != confirmsBeforeRetry {
		t.Fatalf("repeating a denied folder opened %d extra consent dialogs", confirmsAfterRetry-confirmsBeforeRetry)
	}
	if _, err := broker.RequestDirectory(context.Background(), "turn-1", 11, "read another project", "summarize this month"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("requesting another folder after denial = %v, want ErrAccessDenied", err)
	}
	selectsAfterRetry, confirmsAfterRetry := prompt.counts()
	if selectsAfterRetry != selectsBeforeRetry || confirmsAfterRetry != confirmsBeforeRetry {
		t.Fatalf("request after denial reopened picker/consent: selections %d -> %d, consents %d -> %d", selectsBeforeRetry, selectsAfterRetry, confirmsBeforeRetry, confirmsAfterRetry)
	}

	selectsBeforeCancel, _ := prompt.counts()
	if _, err := broker.RequestDirectory(context.Background(), "turn-cancel", 11, "read project", "summarize this month"); !errors.Is(err, ErrDirectorySelectionCancelled) {
		t.Fatalf("canceling directory selection = %v, want ErrDirectorySelectionCancelled", err)
	}
	selectsAfterCancel, _ := prompt.counts()
	if _, err := broker.RequestDirectory(context.Background(), "turn-cancel", 11, "retry picker", "summarize this month"); !errors.Is(err, ErrDirectorySelectionCancelled) {
		t.Fatalf("retrying a canceled picker = %v, want ErrDirectorySelectionCancelled", err)
	}
	selectsAfterPickerRetry, _ := prompt.counts()
	if selectsAfterPickerRetry != selectsAfterCancel || selectsAfterCancel != selectsBeforeCancel+1 {
		t.Fatalf("picker calls around cancellation = %d, %d, %d; want one initial call and no retry", selectsBeforeCancel, selectsAfterCancel, selectsAfterPickerRetry)
	}
}

func TestBrokerDecisionAndTurnCleanup(t *testing.T) {
	folder := t.TempDir()
	started := make(chan struct{}, 1)
	reply := make(chan bool)
	prompt := &scriptedPrompt{
		folders:        []string{folder},
		confirmStarted: started,
		confirmReply:   reply,
	}
	broker := NewBroker(prompt)
	t.Cleanup(broker.Close)

	result := make(chan error, 1)
	go func() {
		_, err := broker.RequestDirectory(context.Background(), "turn-late", 22, "read project", "summarize this month")
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("directory confirmation did not start")
	}

	broker.ReleaseTurn("turn-late")
	reply <- true
	select {
	case err := <-result:
		if !errors.Is(err, ErrTurnExpired) {
			t.Fatalf("late allow after ReleaseTurn = %v, want ErrTurnExpired", err)
		}
	case <-time.After(time.Second):
		t.Fatal("directory request did not finish after the late decision")
	}
}
