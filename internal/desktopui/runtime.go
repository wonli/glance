package desktopui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	"glance/internal/agent"
	"glance/internal/agent/provider"
	"glance/internal/agentsettings"
	"glance/internal/application"
	"glance/internal/mcpserver"
	"glance/internal/printservice"
	"glance/internal/workspace"
)

// desktopRuntime is the process-level owner of workspace persistence, shared
// document sessions, and the set of live windows.
type desktopRuntime struct {
	applicationRuntime      *application.Runtime
	applicationRuntimeErr   error
	nativePrinter           printservice.Printer
	agentSettings           *agentsettings.Store
	agentSettingsErr        error
	agentSettingsValue      agentsettings.ModelSettings
	agentClient             *agent.InProcessMCPClient
	agentHost               *agent.AgentHost
	agentMu                 sync.RWMutex
	agentConfigMu           sync.Mutex
	applicationEventCancel  func()
	mcpEndpoint             *mcpserver.DesktopEndpoint
	mcpEndpointErr          error
	shuttingDown            bool
	mu                      sync.Mutex
	workspaceQueueMu        sync.Mutex
	workspaceInitMu         sync.Mutex
	workspaceInitialized    bool
	workspaceDBMu           sync.Mutex
	workspaceStore          *workspace.Store
	collection              *workspace.Collection
	workspaceWriteJobs      chan workspaceWriteJob
	workspaceWriteResults   chan error
	workspaceWriterDone     chan struct{}
	workspaceWriterStopOnce sync.Once
	workspaceWriterStopped  bool
	shutdownOnce            sync.Once
	completion              chan struct{}
	completionOnce          sync.Once
	windowLauncher          windowLauncher
	sessions                map[string]*sharedDocumentSession
	sessionsByCanonicalPath map[string]string
	openingPaths            map[string]chan struct{}
	windows                 map[uint64]*App
	windowAttachmentOrder   map[uint64]uint64
	primaryWindowID         uint64
	nextWindowID            uint64
	nextWindowAttachment    uint64
}

func (r *desktopRuntime) startDesktopMCPEndpoint() error {
	if r == nil {
		return errors.New("desktop runtime is unavailable")
	}
	r.mu.Lock()
	if r.shuttingDown {
		r.mu.Unlock()
		return errors.New("desktop runtime is shutting down")
	}
	if r.mcpEndpoint != nil {
		r.mu.Unlock()
		return nil
	}
	if r.mcpEndpointErr != nil {
		err := r.mcpEndpointErr
		r.mu.Unlock()
		return err
	}
	applicationRuntime := r.applicationRuntime
	r.mu.Unlock()
	if applicationRuntime == nil {
		return errors.New("application runtime is unavailable")
	}
	endpoint, err := mcpserver.StartDesktopEndpoint(applicationRuntime)
	r.mu.Lock()
	if r.shuttingDown {
		r.mu.Unlock()
		if endpoint != nil {
			_ = endpoint.Close()
		}
		return errors.New("desktop runtime is shutting down")
	}
	if err != nil {
		r.mcpEndpointErr = err
		r.mu.Unlock()
		return err
	}
	if r.mcpEndpoint != nil {
		r.mu.Unlock()
		_ = endpoint.Close()
		return nil
	}
	r.mcpEndpoint = endpoint
	r.mu.Unlock()
	return nil
}

func (r *desktopRuntime) desktopMCPConnectionInfo() (mcpserver.DesktopEndpointInfo, bool, error) {
	if r == nil {
		return mcpserver.DesktopEndpointInfo{}, false, errors.New("desktop runtime is unavailable")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.mcpEndpoint != nil {
		return r.mcpEndpoint.Info(), true, nil
	}
	return mcpserver.DesktopEndpointInfo{}, false, r.mcpEndpointErr
}

type desktopPrintService struct {
	printer printservice.Printer
}

func (service desktopPrintService) Available() bool {
	return service.printer != nil && service.printer.Available()
}

func (service desktopPrintService) PrintPDF(ctx context.Context, data []byte, options application.PrintOptions) error {
	if service.printer == nil {
		return application.ErrCapabilityUnsupported
	}
	return service.printer.Print(ctx, data, printservice.Options{
		Copies: options.Copies, FirstPage: options.FirstPage, LastPage: options.LastPage,
	})
}

func newDesktopRuntime() *desktopRuntime {
	return newDesktopRuntimeWithPrinter(printservice.New())
}

func newDesktopRuntimeWithPrinter(printer printservice.Printer) *desktopRuntime {
	if printer == nil {
		printer = printservice.New()
	}
	runtime := &desktopRuntime{
		sessions:                make(map[string]*sharedDocumentSession),
		sessionsByCanonicalPath: make(map[string]string),
		openingPaths:            make(map[string]chan struct{}),
		windows:                 make(map[uint64]*App),
		windowAttachmentOrder:   make(map[uint64]uint64),
		nativePrinter:           printer,
	}
	runtime.agentSettings, runtime.agentSettingsErr = agentsettings.NewDefaultStore(agentsettings.SystemCredentialStore{})
	applicationRuntime, err := application.NewRuntime(context.Background(), application.RuntimeOptions{
		Factories: desktopApplicationFactories(runtime), PrintService: desktopPrintService{printer: printer},
	})
	runtime.applicationRuntime = applicationRuntime
	runtime.applicationRuntimeErr = err
	if err == nil && applicationRuntime != nil {
		runtime.applicationEventCancel = applicationRuntime.On(context.Background(), application.EventFilter{
			Types: []string{
				application.EventDocumentOpened, application.EventDocumentSaved, application.EventDocumentChanged,
				application.EventConfirmationRequested, application.EventConfirmationUpdated,
			},
		}, runtime.routeApplicationEvent)
	}
	return runtime
}

func (r *desktopRuntime) routeApplicationEvent(event application.Event) {
	if r == nil || !isDesktopApplicationEvent(event.Type) {
		return
	}
	r.mu.Lock()
	var app *App
	if (event.Type == application.EventConfirmationRequested || event.Type == application.EventConfirmationUpdated) && event.View.WindowID != 0 {
		app = r.windows[event.View.WindowID]
	}
	if app == nil {
		app = r.windows[r.primaryWindowID]
	}
	if app == nil {
		for _, candidate := range r.windows {
			if candidate != nil {
				app = candidate
				break
			}
		}
	}
	r.mu.Unlock()
	if app == nil || app.agentUI.applicationEvents == nil {
		return
	}
	select {
	case app.agentUI.applicationEvents <- event:
	default:
		if event.Type == application.EventConfirmationRequested || event.Type == application.EventConfirmationUpdated || event.Type == application.EventDocumentChanged {
			app.launchManagedBackground(func() {
				select {
				case app.agentUI.applicationEvents <- event:
					if app.window != nil {
						app.window.Invalidate()
					}
				case <-app.shutdown:
				}
			})
		} else {
			log.Printf("drop application event %s for document %s: desktop event queue is full", event.Type, event.DocumentID)
		}
	}
	if app.window != nil {
		app.window.Invalidate()
	}
}

func isDesktopApplicationEvent(eventType string) bool {
	switch eventType {
	case application.EventDocumentOpened, application.EventDocumentSaved,
		application.EventDocumentChanged, application.EventConfirmationRequested, application.EventConfirmationUpdated:
		return true
	default:
		return false
	}
}

func (r *desktopRuntime) loadAgentSettings(ctx context.Context) (agentsettings.ModelSettings, error) {
	if r == nil {
		return agentsettings.ModelSettings{}, errors.New("桌面运行时不可用")
	}
	r.agentMu.RLock()
	store, storeErr := r.agentSettings, r.agentSettingsErr
	r.agentMu.RUnlock()
	if storeErr != nil {
		return agentsettings.ModelSettings{}, storeErr
	}
	if store == nil {
		return agentsettings.ModelSettings{}, errors.New("Agent 设置存储不可用")
	}
	settings, err := store.Load(ctx)
	if err != nil {
		return agentsettings.ModelSettings{}, err
	}
	r.agentMu.Lock()
	r.agentSettingsValue = settings
	r.agentMu.Unlock()
	return settings, nil
}

func (r *desktopRuntime) saveAgentSettings(ctx context.Context, settings agentsettings.ModelSettings, apiKey string) error {
	if r == nil {
		return errors.New("桌面运行时不可用")
	}
	r.agentConfigMu.Lock()
	defer r.agentConfigMu.Unlock()
	r.agentMu.RLock()
	store, storeErr := r.agentSettings, r.agentSettingsErr
	r.agentMu.RUnlock()
	if storeErr != nil {
		return storeErr
	}
	if store == nil {
		return errors.New("Agent 设置存储不可用")
	}
	if err := store.Save(ctx, settings, apiKey); err != nil {
		return err
	}
	saved, err := store.Load(ctx)
	if err != nil {
		return err
	}
	var secret string
	if saved.ProviderID != "ollama" && saved.CredentialRef != "" {
		secret, err = store.LoadAPIKey(ctx)
		if err != nil {
			return err
		}
	}
	return r.installAgentHost(ctx, saved, secret)
}

func (r *desktopRuntime) testAgentSettings(ctx context.Context, settings agentsettings.ModelSettings, apiKey string) error {
	if r == nil {
		return errors.New("桌面运行时不可用")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	settings.ProviderID = strings.ToLower(strings.TrimSpace(settings.ProviderID))
	settings.BaseURL = strings.TrimSpace(settings.BaseURL)
	settings.Model = strings.TrimSpace(settings.Model)
	if settings.Model == "" {
		return errors.New("请填写模型名")
	}
	secret := strings.TrimSpace(apiKey)
	if settings.ProviderID != "ollama" && secret == "" {
		r.agentMu.RLock()
		store, storeErr := r.agentSettings, r.agentSettingsErr
		r.agentMu.RUnlock()
		if storeErr != nil {
			return storeErr
		}
		if store == nil {
			return errors.New("Agent 设置存储不可用")
		}
		stored, err := store.Load(ctx)
		if err != nil {
			return err
		}
		if stored.ProviderID != settings.ProviderID || stored.CredentialRef == "" {
			return errors.New("请先输入 API Key，或为当前服务商保存一个 API Key")
		}
		secret, err = store.LoadAPIKey(ctx)
		if err != nil {
			return err
		}
	}

	var modelProvider provider.ModelProvider
	switch settings.ProviderID {
	case "ollama":
		baseURL := settings.BaseURL
		if baseURL == "" {
			baseURL = "http://127.0.0.1:11434"
		}
		modelProvider = provider.NewOllama(provider.OllamaConfig{BaseURL: baseURL, Model: settings.Model})
	case "minimax", "kimi", "deepseek", "openai-compatible":
		baseURL := settings.BaseURL
		if baseURL == "" {
			baseURL = provider.DefaultChatCompletionsBaseURL(settings.ProviderID)
		}
		if baseURL == "" {
			return errors.New("请填写兼容 Chat Completions 的服务地址")
		}
		if secret == "" {
			return errors.New("请填写 API Key")
		}
		modelProvider = provider.NewChatCompletions(provider.ChatCompletionsConfig{BaseURL: baseURL, APIKey: secret, Model: settings.Model})
	default:
		return errors.New("不支持的模型服务商")
	}

	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var response strings.Builder
	err := modelProvider.Stream(probeCtx, provider.ModelRequest{
		Model:    settings.Model,
		Messages: []provider.ModelMessage{{Role: "user", Content: "请只回复：连接成功"}},
	}, func(event provider.ModelEvent) error {
		if event.ToolCallDelta != nil {
			return errors.New("连通性探测意外收到工具调用响应")
		}
		if response.Len()+len(event.TextDelta) > 4096 {
			return errors.New("模型探测回复超过限制")
		}
		response.WriteString(event.TextDelta)
		return nil
	})
	if err != nil {
		return fmt.Errorf("模型连接失败：%w", err)
	}
	if strings.TrimSpace(response.String()) == "" {
		return errors.New("模型连接成功，但没有返回文本")
	}
	return nil
}

func (r *desktopRuntime) restoreAgentHost(ctx context.Context) error {
	if r == nil {
		return errors.New("桌面运行时不可用")
	}
	r.agentConfigMu.Lock()
	defer r.agentConfigMu.Unlock()
	settings, err := r.loadAgentSettings(ctx)
	if err != nil {
		return err
	}
	var secret string
	if settings.ProviderID != "" && settings.ProviderID != "ollama" && settings.CredentialRef != "" {
		r.agentMu.RLock()
		store := r.agentSettings
		r.agentMu.RUnlock()
		secret, err = store.LoadAPIKey(ctx)
		if err != nil {
			return err
		}
	}
	return r.installAgentHost(ctx, settings, secret)
}

func (r *desktopRuntime) installAgentHost(ctx context.Context, saved agentsettings.ModelSettings, secret string) error {
	r.mu.Lock()
	if r.shuttingDown {
		r.mu.Unlock()
		return errors.New("桌面运行时正在关闭")
	}
	applicationRuntime, applicationRuntimeErr := r.applicationRuntime, r.applicationRuntimeErr
	r.mu.Unlock()
	var modelProvider provider.ModelProvider
	var err error
	if saved.ProviderID == "ollama" {
		baseURL := saved.BaseURL
		if baseURL == "" {
			baseURL = "http://127.0.0.1:11434"
		}
		modelProvider = provider.NewOllama(provider.OllamaConfig{BaseURL: baseURL, Model: saved.Model})
	} else if secret != "" {
		baseURL := saved.BaseURL
		if baseURL == "" {
			baseURL = provider.DefaultChatCompletionsBaseURL(saved.ProviderID)
		}
		if baseURL == "" {
			return errors.New("该模型服务需要填写兼容 Chat Completions 的服务地址")
		}
		modelProvider = provider.NewChatCompletions(provider.ChatCompletionsConfig{BaseURL: baseURL, APIKey: secret, Model: saved.Model})
	}

	var newClient *agent.InProcessMCPClient
	var newHost *agent.AgentHost
	if modelProvider != nil {
		if applicationRuntimeErr != nil {
			return applicationRuntimeErr
		}
		newClient, err = agent.ConnectMCP(ctx, applicationRuntime)
		if err != nil {
			return fmt.Errorf("连接 Glance 能力目录: %w", err)
		}
		newHost, err = agent.NewHost(ctx, modelProvider, newClient.Session, agent.HostOptions{Model: saved.Model})
		if err != nil {
			_ = newClient.Close()
			return fmt.Errorf("创建内置 Agent Host: %w", err)
		}
	}

	r.mu.Lock()
	if r.shuttingDown {
		r.mu.Unlock()
		if newHost != nil {
			newHost.Close()
		}
		if newClient != nil {
			_ = newClient.Close()
		}
		return errors.New("桌面运行时正在关闭")
	}
	r.agentMu.Lock()
	oldHost, oldClient := r.agentHost, r.agentClient
	r.agentHost, r.agentClient, r.agentSettingsValue = newHost, newClient, saved
	r.agentMu.Unlock()
	r.mu.Unlock()
	if oldHost != nil {
		oldHost.Close()
	}
	if oldClient != nil {
		_ = oldClient.Close()
	}
	r.invalidateWindows()
	return nil
}

func (r *desktopRuntime) currentAgentHost() (*agent.AgentHost, agentsettings.ModelSettings) {
	if r == nil {
		return nil, agentsettings.ModelSettings{}
	}
	r.agentMu.RLock()
	host, settings := r.agentHost, r.agentSettingsValue
	r.agentMu.RUnlock()
	return host, settings
}

func (r *desktopRuntime) clearAgentAPIKey(ctx context.Context) error {
	if r == nil {
		return errors.New("桌面运行时不可用")
	}
	r.agentConfigMu.Lock()
	defer r.agentConfigMu.Unlock()
	r.agentMu.RLock()
	store, storeErr := r.agentSettings, r.agentSettingsErr
	r.agentMu.RUnlock()
	if storeErr != nil {
		return storeErr
	}
	if store == nil {
		return errors.New("Agent 设置存储不可用")
	}
	previous, err := store.Load(ctx)
	if err != nil {
		return err
	}
	if err := store.ClearAPIKey(ctx); err != nil {
		return err
	}
	settings, err := store.Load(ctx)
	if err != nil {
		return err
	}
	r.agentMu.Lock()
	r.agentSettingsValue = settings
	r.agentMu.Unlock()
	if previous.CredentialRef != "" {
		r.closeAgent()
	}
	r.invalidateWindows()
	return nil
}

func (r *desktopRuntime) closeAgent() {
	if r == nil {
		return
	}
	r.agentMu.Lock()
	host, client := r.agentHost, r.agentClient
	r.agentHost, r.agentClient = nil, nil
	r.agentMu.Unlock()
	if host != nil {
		host.Close()
	}
	if client != nil {
		_ = client.Close()
	}
}

func (r *desktopRuntime) allocateWindowID() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextWindowID++
	return r.nextWindowID
}

func (a *App) ensureDesktopRuntime() *desktopRuntime {
	if a.desktopRuntime == nil {
		a.desktopRuntime = newDesktopRuntimeWithPrinter(a.nativePrinter)
	}
	return a.desktopRuntime
}

func (r *desktopRuntime) session(documentID string) (*sharedDocumentSession, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	session, ok := r.sessions[documentID]
	r.mu.Unlock()
	return session, ok
}

func (r *desktopRuntime) registerSession(session *sharedDocumentSession) *sharedDocumentSession {
	existing := r.registerSessionWithoutApplication(session)
	r.registerApplicationSession(existing)
	return existing
}

func (r *desktopRuntime) registerSessionWithoutApplication(session *sharedDocumentSession) *sharedDocumentSession {
	if r == nil || session == nil {
		return session
	}
	r.mu.Lock()
	if r.sessions == nil {
		r.sessions = make(map[string]*sharedDocumentSession)
	}
	if r.sessionsByCanonicalPath == nil {
		r.sessionsByCanonicalPath = make(map[string]string)
	}
	if session.documentID == "" {
		if session.path != "" {
			session.documentID = workspaceSessionID(session.path)
		} else {
			session.documentID = newDraftSessionID("document")
		}
	}
	existing := r.findSessionByPathLocked(session.path)
	if existing == nil {
		existing = r.sessions[session.documentID]
	}
	if existing == nil {
		r.sessions[session.documentID] = session
		session.ownerRuntime = r
		if key := canonicalSessionPath(session.path); key != "" {
			r.sessionsByCanonicalPath[key] = session.documentID
		}
		existing = session
	}
	r.mu.Unlock()
	if existing != session {
		closeUnregisteredSessionAdapters(session, existing)
	}
	return existing
}

func (r *desktopRuntime) registerApplicationSession(session *sharedDocumentSession) {
	if r == nil || session == nil || r.applicationRuntime == nil {
		return
	}
	adapter, err := newDesktopSessionAdapter(r, session)
	if err != nil {
		return
	}
	_, err = r.applicationRuntime.RegisterLiveSession(context.Background(), session.documentID, session.wordRevisionAtomic.Load(), adapter)
	if err != nil && !errors.Is(err, application.ErrDocumentPathInUse) {
		log.Printf("register live application session %s: %v", session.documentID, err)
	}
}

func (r *desktopRuntime) findSessionByPathLocked(path string) *sharedDocumentSession {
	if path == "" {
		return nil
	}
	key := canonicalSessionPath(path)
	if documentID := r.sessionsByCanonicalPath[key]; documentID != "" {
		if session := r.sessions[documentID]; session != nil {
			return session
		}
		delete(r.sessionsByCanonicalPath, key)
	}
	for _, session := range r.sessions {
		if session != nil && sameDocumentPath(session.path, path) {
			return session
		}
	}
	return nil
}

func canonicalSessionPath(path string) string {
	if path == "" {
		return ""
	}
	return canonicalDocumentPath(path)
}

func closeUnregisteredSessionAdapters(candidate, existing *sharedDocumentSession) {
	if candidate == nil || existing == nil {
		return
	}
	if candidate.book != nil && !sameAdapterInstance(candidate.book, existing.book) {
		candidate.adapterMu.Lock()
		_ = candidate.book.Close()
		candidate.adapterMu.Unlock()
	}
	if candidate.word != nil && candidate.word != existing.word {
		candidate.adapterMu.Lock()
		_ = candidate.word.Close()
		candidate.adapterMu.Unlock()
	}
	if candidate.pdfDoc != nil && candidate.pdfDoc != existing.pdfDoc {
		_ = candidate.pdfDoc.Close()
	}
	if candidate.markdownDoc != nil && candidate.markdownDoc != existing.markdownDoc {
		_ = candidate.markdownDoc.Close()
	}
	if candidate.jsonDoc != nil && candidate.jsonDoc != existing.jsonDoc {
		candidate.adapterMu.Lock()
		_ = candidate.jsonDoc.Close()
		candidate.adapterMu.Unlock()
	}
	candidate.finalized.Store(true)
}

func sameAdapterInstance(left, right any) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftValue, rightValue := reflect.ValueOf(left), reflect.ValueOf(right)
	if leftValue.Type() != rightValue.Type() || leftValue.Kind() != reflect.Pointer {
		return false
	}
	return leftValue.Pointer() == rightValue.Pointer()
}

func (r *desktopRuntime) openDocumentSession(path string) (*sharedDocumentSession, error) {
	session, _, err := r.openDocumentSessionWithStatus(path)
	return session, err
}

func (r *desktopRuntime) openDocumentSessionWithStatus(path string) (*sharedDocumentSession, bool, error) {
	if r == nil {
		return nil, false, fmt.Errorf("文档运行时不可用")
	}
	path = stringsTrimSpace(path)
	key := canonicalSessionPath(path)
	for {
		r.mu.Lock()
		if r.sessions == nil {
			r.sessions = make(map[string]*sharedDocumentSession)
		}
		if r.sessionsByCanonicalPath == nil {
			r.sessionsByCanonicalPath = make(map[string]string)
		}
		if r.openingPaths == nil {
			r.openingPaths = make(map[string]chan struct{})
		}
		if existing := r.findSessionByPathLocked(path); existing != nil {
			r.mu.Unlock()
			return existing, false, nil
		}
		if wait := r.openingPaths[key]; wait != nil {
			r.mu.Unlock()
			<-wait
			continue
		}
		if r.openingPaths == nil {
			r.openingPaths = make(map[string]chan struct{})
		}
		opened := make(chan struct{})
		r.openingPaths[key] = opened
		r.mu.Unlock()

		loaded, err := loadDocument(path)
		var candidate *sharedDocumentSession
		if err == nil {
			candidate = sharedSessionFromLoadedDocument(loaded)
		}

		r.mu.Lock()
		existing := r.findSessionByPathLocked(path)
		var result *sharedDocumentSession
		openedFromDisk := false
		if err != nil {
			result = nil
		} else if existing != nil {
			result = existing
		} else {
			result = r.registerSessionLocked(candidate)
			openedFromDisk = result == candidate
		}
		delete(r.openingPaths, key)
		close(opened)
		r.mu.Unlock()
		if candidate != nil && result != candidate {
			closeUnregisteredSessionAdapters(candidate, result)
		}
		if result != nil {
			r.registerApplicationSession(result)
		}
		return result, openedFromDisk, err
	}
}

func sharedSessionFromLoadedDocument(loaded loadedDocument) *sharedDocumentSession {
	if loaded.shared != nil {
		return loaded.shared
	}
	session := newSharedDocumentSession(loaded.path)
	if loaded.path == "" {
		format := "document"
		switch {
		case loaded.book != nil:
			format = "xlsx"
		case loaded.word != nil:
			format = "docx"
		case loaded.pptxDoc != nil:
			format = "pptx"
		case loaded.markdownDoc != nil:
			format = "markdown"
		case loaded.jsonDoc != nil:
			format = "json"
		}
		session.documentID = newDraftSessionID(format)
	}
	session.book = loaded.book
	session.word = loaded.word
	session.wordBlocks = cloneWordBlockSnapshot(loaded.wordBlocks)
	if loaded.word != nil && loaded.wordBlocks == nil {
		session.wordBlocks = cloneWordBlockSnapshot(loaded.word.WordBlocks())
	}
	session.pdfDoc = loaded.pdfDoc
	session.pptxDoc = loaded.pptxDoc
	session.markdownDoc = loaded.markdownDoc
	session.jsonDoc = loaded.jsonDoc
	session.sheets = append([]string(nil), loaded.sheets...)
	return session
}

func loadedDocumentFromSharedSession(session *sharedDocumentSession) loadedDocument {
	if session == nil {
		return loadedDocument{}
	}
	return loadedDocument{
		shared:      session,
		path:        session.path,
		book:        session.book,
		word:        session.word,
		wordBlocks:  cloneWordBlockSnapshot(session.wordBlocks),
		jsonDoc:     session.jsonDoc,
		markdownDoc: session.markdownDoc,
		pdfDoc:      session.pdfDoc,
		pptxDoc:     session.pptxDoc,
		sheets:      append([]string(nil), session.sheets...),
	}
}

func (r *desktopRuntime) registerSessionLocked(session *sharedDocumentSession) *sharedDocumentSession {
	if session.documentID == "" {
		if session.path != "" {
			session.documentID = workspaceSessionID(session.path)
		} else {
			session.documentID = newDraftSessionID("document")
		}
	}
	if existing := r.findSessionByPathLocked(session.path); existing != nil {
		return existing
	}
	if existing := r.sessions[session.documentID]; existing != nil {
		return existing
	}
	r.sessions[session.documentID] = session
	session.ownerRuntime = r
	if key := canonicalSessionPath(session.path); key != "" {
		r.sessionsByCanonicalPath[key] = session.documentID
	}
	return session
}

func (r *desktopRuntime) invalidateSessionViews(documentID string) {
	if r == nil || documentID == "" {
		return
	}
	r.mu.Lock()
	apps := make([]*App, 0, len(r.windows))
	for _, app := range r.windows {
		if app != nil && app.activeSessionID() == documentID {
			apps = append(apps, app)
		}
	}
	r.mu.Unlock()
	for _, app := range apps {
		if app.window != nil {
			app.window.Invalidate()
		}
	}
}

func (r *desktopRuntime) updateSessionPath(session *sharedDocumentSession, path string) {
	if r == nil || session == nil {
		return
	}
	r.mu.Lock()
	if r.sessions[session.documentID] == session {
		session.adapterMu.Lock()
		if oldKey := canonicalSessionPath(session.path); oldKey != "" && r.sessionsByCanonicalPath[oldKey] == session.documentID {
			delete(r.sessionsByCanonicalPath, oldKey)
		}
		session.path = path
		if key := canonicalSessionPath(path); key != "" {
			r.sessionsByCanonicalPath[key] = session.documentID
		}
		session.adapterMu.Unlock()
	}
	r.mu.Unlock()
	if r.applicationRuntime != nil {
		if err := r.applicationRuntime.SyncDocumentPath(context.Background(), session.documentID, path); err != nil && !errors.Is(err, application.ErrDocumentNotFound) {
			log.Printf("sync application session path %s: %v", session.documentID, err)
		}
	}
}

func (r *desktopRuntime) updateSessionPathFromApplication(session *sharedDocumentSession, path string) {
	if r == nil || session == nil {
		return
	}
	r.mu.Lock()
	if r.sessions[session.documentID] == session {
		session.adapterMu.Lock()
		if oldKey := canonicalSessionPath(session.path); oldKey != "" && r.sessionsByCanonicalPath[oldKey] == session.documentID {
			delete(r.sessionsByCanonicalPath, oldKey)
		}
		session.path = path
		if key := canonicalSessionPath(path); key != "" {
			r.sessionsByCanonicalPath[key] = session.documentID
		}
		session.adapterMu.Unlock()
	}
	r.mu.Unlock()
}

func (r *desktopRuntime) removeSession(documentID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	session := r.sessions[documentID]
	delete(r.sessions, documentID)
	if session != nil {
		if key := canonicalSessionPath(session.path); key != "" && r.sessionsByCanonicalPath[key] == documentID {
			delete(r.sessionsByCanonicalPath, key)
		}
	}
	r.mu.Unlock()
	if session != nil {
		session.viewsMu.RLock()
		views := make([]*documentSessionState, 0, len(session.views))
		for _, view := range session.views {
			views = append(views, view)
		}
		session.viewsMu.RUnlock()
		for _, view := range views {
			if view != nil {
				view.releaseWordViewResources()
				view.closed.Store(true)
			}
		}
		r.mu.Lock()
		apps := make([]*App, 0, len(r.windows))
		for _, app := range r.windows {
			apps = append(apps, app)
		}
		r.mu.Unlock()
		for _, app := range apps {
			if app != nil && app.activeSessionID() == documentID {
				app.setActiveSessionID("")
			}
		}
	}
	closeSharedDocumentSession(session)
	if r.applicationRuntime != nil {
		_ = r.applicationRuntime.ForgetLiveSession(context.Background(), documentID)
	}
	r.invalidateWindows()
}

func closeSharedDocumentSession(session *sharedDocumentSession) {
	if session == nil {
		return
	}
	for {
		session.operationMu.Lock()
		session.finalized.Store(true)
		operationActive := session.operationActive
		changed := session.operationChanged
		session.operationMu.Unlock()
		session.cancelWordLayout()
		if !operationActive {
			break
		}
		<-changed
	}
	session.releaseWordLayout()
	if session.adaptersClosed.CompareAndSwap(false, true) {
		session.adapterMu.Lock()
		if session.book != nil {
			_ = session.book.Close()
		}
		if session.word != nil {
			_ = session.word.Close()
		}
		if session.pdfDoc != nil {
			_ = session.pdfDoc.Close()
			session.pdfDoc = nil
		}
		if session.markdownDoc != nil {
			_ = session.markdownDoc.Close()
			session.markdownDoc = nil
		}
		if session.jsonDoc != nil {
			_ = session.jsonDoc.Close()
		}
		if session.pptxDoc != nil {
			_ = session.pptxDoc.Close()
		}
		session.book, session.word, session.jsonDoc, session.pptxDoc = nil, nil, nil, nil
		session.adapterMu.Unlock()
	}
	session.viewsMu.Lock()
	views := make([]*documentSessionState, 0, len(session.views))
	for _, view := range session.views {
		views = append(views, view)
	}
	session.views = nil
	session.viewsMu.Unlock()
	for _, view := range views {
		if view != nil {
			view.releaseWordViewResources()
			view.closed.Store(true)
		}
	}
	session.wordBlocksMu.Lock()
	session.wordDraftMu.Lock()
	session.wordBlocks = nil
	session.wordRunCommitted = nil
	session.wordRunDrafts = nil
	session.wordDraftMu.Unlock()
	session.wordBlocksMu.Unlock()
}

func (r *desktopRuntime) invalidateWindows() {
	if r == nil {
		return
	}
	r.mu.Lock()
	apps := make([]*App, 0, len(r.windows))
	for _, app := range r.windows {
		apps = append(apps, app)
	}
	r.mu.Unlock()
	for _, app := range apps {
		if app != nil && app.window != nil {
			app.window.Invalidate()
		}
	}
}

// retain the package's existing whitespace normalization without duplicating
// path-specific policy in the runtime's session key logic.
func stringsTrimSpace(value string) string {
	return strings.TrimSpace(value)
}
