package desktopui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glance/internal/agentsettings"
	"glance/internal/application"
)

func TestDesktopRuntimeLoadsAgentSettingsAndKeepsCredentialOutOfConfig(t *testing.T) {
	ctx := context.Background()
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	credentials := &desktopMemoryCredentials{items: make(map[string]string)}
	path := filepath.Join(t.TempDir(), "agent", "model-settings.json")
	store, err := agentsettings.NewStore(path, credentials)
	if err != nil {
		t.Fatal(err)
	}
	runtime.agentSettings = store
	runtime.agentSettingsErr = nil

	settings := agentsettings.ModelSettings{ProviderID: "minimax", BaseURL: "https://api.minimaxi.com/v1", Model: "test-model"}
	if err := runtime.saveAgentSettings(ctx, settings, "runtime-secret"); err != nil {
		t.Fatalf("save Agent settings: %v", err)
	}
	if host, saved := runtime.currentAgentHost(); host == nil || saved.ProviderID != settings.ProviderID {
		t.Fatalf("Agent host/settings after save = %p / %+v", host, saved)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || strings.Contains(string(data), "runtime-secret") {
		t.Fatalf("ordinary model settings contain the API key: %s", data)
	}
	if len(credentials.items) != 1 {
		t.Fatalf("credential store items after save = %d; want 1", len(credentials.items))
	}

	runtime.closeAgent()
	if err := runtime.restoreAgentHost(ctx); err != nil {
		t.Fatalf("restore configured Agent: %v", err)
	}
	if host, _ := runtime.currentAgentHost(); host == nil {
		t.Fatal("restoring configured settings did not create an Agent Host")
	}
	if err := runtime.clearAgentAPIKey(ctx); err != nil {
		t.Fatalf("clear API key: %v", err)
	}
	if host, saved := runtime.currentAgentHost(); host != nil || saved.CredentialRef != "" {
		t.Fatalf("Agent state after clearing key = %p / %+v; want unavailable host and no reference", host, saved)
	}
}

func TestAgentSubscriptionWatcherStopsWhenHostIsReplacedOrWindowCloses(t *testing.T) {
	ctx := context.Background()
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)

	settings := agentsettings.ModelSettings{ProviderID: "ollama", BaseURL: "http://127.0.0.1:11434", Model: "test-model"}
	if err := runtime.installAgentHost(ctx, settings, ""); err != nil {
		t.Fatalf("install first Agent Host: %v", err)
	}
	firstHost, _ := runtime.currentAgentHost()
	app := newAppWithRuntime(nil, runtime)
	app.documentSessionState = &documentSessionState{sharedDocumentSession: &sharedDocumentSession{}}
	app.documentSessionState.documentID = "doc-subscription"
	app.agentUI.conversationDocumentID = "doc-subscription"
	app.agentUI.conversationID = "conversation-subscription"
	app.subscribeToAgent(firstHost)
	firstWatcher := app.agentUI.subscriptionDone
	if firstWatcher == nil {
		t.Fatal("first Agent subscription did not retain its watcher completion signal")
	}

	settings.Model = "replacement-model"
	if err := runtime.installAgentHost(ctx, settings, ""); err != nil {
		t.Fatalf("install replacement Agent Host: %v", err)
	}
	secondHost, _ := runtime.currentAgentHost()
	app.subscribeToAgent(secondHost)
	assertChannelClosed(t, firstWatcher, "replaced Agent Host watcher")

	secondWatcher := app.agentUI.subscriptionDone
	if secondWatcher == nil {
		t.Fatal("replacement Agent subscription did not retain its watcher completion signal")
	}
	app.subscribeToAgent(nil)
	assertChannelClosed(t, secondWatcher, "disabled Agent watcher")

	app.subscribeToAgent(secondHost)
	closingWatcher := app.agentUI.subscriptionDone
	if closingWatcher == nil {
		t.Fatal("reopened Agent subscription did not retain its watcher completion signal")
	}
	app.closeAgentSubscription()
	assertChannelClosed(t, closingWatcher, "closed window Agent watcher")
}

func TestDesktopRuntimeShutdownClosesAgentSubscriptionAndLiveSessions(t *testing.T) {
	ctx := context.Background()
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	applicationRuntime := runtime.applicationRuntime
	app := newAppWithRuntime(nil, runtime)

	documentInfo, err := applicationRuntime.CreateDocument(ctx, application.FormatJSON)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if err := runtime.installAgentHost(ctx, agentsettings.ModelSettings{ProviderID: "ollama", Model: "test-model"}, ""); err != nil {
		t.Fatalf("install Agent Host: %v", err)
	}
	app.documentSessionState = &documentSessionState{sharedDocumentSession: &sharedDocumentSession{}}
	app.documentSessionState.documentID = documentInfo.DocumentID
	app.agentUI.conversationDocumentID = documentInfo.DocumentID
	app.agentUI.conversationID = "conversation-runtime-shutdown"
	host, _ := runtime.currentAgentHost()
	app.subscribeToAgent(host)
	watcherDone := app.agentUI.subscriptionDone
	if watcherDone == nil {
		t.Fatal("Agent subscription did not retain its watcher completion signal")
	}
	backgroundDone := make(chan struct{})
	if !app.launchManagedBackground(func() {
		<-app.shutdown
		close(backgroundDone)
	}) {
		t.Fatal("active App rejected a managed shutdown task")
	}

	app.shutdownOnce.Do(func() { close(app.shutdown) })
	app.closeAgentSubscription()
	app.stopAndWaitManagedBackground()
	runtime.shutdown()
	assertChannelClosed(t, watcherDone, "desktop Agent watcher after Runtime shutdown")
	assertChannelClosed(t, backgroundDone, "managed desktop task after Runtime shutdown")
	if _, err := applicationRuntime.GetDocument(ctx, documentInfo.DocumentID); !errors.Is(err, application.ErrRuntimeClosed) {
		t.Fatalf("GetDocument after desktop Runtime shutdown = %v, want ErrRuntimeClosed", err)
	}
}

func assertChannelClosed(t *testing.T, done <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("%s remained active", description)
	}
}

type desktopMemoryCredentials struct {
	items map[string]string
}

func (s *desktopMemoryCredentials) Get(ctx context.Context, reference string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	secret, ok := s.items[reference]
	if !ok {
		return "", errors.New("credential not found")
	}
	return secret, nil
}

func (s *desktopMemoryCredentials) Put(ctx context.Context, reference, secret string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.items[reference] = secret
	return nil
}

func (s *desktopMemoryCredentials) Delete(ctx context.Context, reference string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	delete(s.items, reference)
	return nil
}
