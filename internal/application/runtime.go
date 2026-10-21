package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"glance/internal/localaccess"
	"glance/internal/workspace"
)

type Format string

const (
	FormatDocx     Format = "docx"
	FormatDOC      Format = "doc"
	FormatXLSX     Format = "xlsx"
	FormatXLS      Format = "xls"
	FormatCSV      Format = "csv"
	FormatPPTX     Format = "pptx"
	FormatPDF      Format = "pdf"
	FormatJSON     Format = "json"
	FormatMarkdown Format = "markdown"
)

var (
	ErrRuntimeClosed           = errors.New("application runtime is closed")
	ErrUnsupportedFormat       = errors.New("unsupported document format")
	ErrDocumentNotFound        = errors.New("document session not found")
	ErrDirtyDocument           = errors.New("document has unsaved changes")
	ErrReadOnlyDocument        = errors.New("document is read-only")
	ErrNoSavePath              = errors.New("document has no save path; use SaveDocumentAs")
	ErrTargetExists            = errors.New("save target already exists")
	ErrDocumentPathInUse       = errors.New("document path is already open")
	ErrDuplicateFactory        = errors.New("duplicate document format factory")
	ErrInvalidDocumentID       = errors.New("document ID is empty")
	ErrInvalidDocumentPath     = errors.New("document path is empty")
	ErrInvalidQuery            = errors.New("invalid document query")
	ErrQueryNotSupported       = errors.New("document query is not supported")
	ErrQueryFilterNotSupported = errors.New("document query filter is not supported")
	ErrQueryResultTooLarge     = errors.New("document query result is too large")
)

// DocumentAdapter is the format-neutral lifecycle contract owned by Runtime.
// Format-specific editing and snapshots are added through neutral capabilities.
type DocumentAdapter interface {
	Format() Format
	Path() string
	Dirty() bool
	ReadOnly() bool
	Save(context.Context) error
	SaveAs(context.Context, string) error
	Close(context.Context) error
}

// SourceFormatAdapter declares the on-disk format when an adapter imports a
// document into another editable format. Runtime accepts the format alias only
// when it exactly matches the source extension being opened.
type SourceFormatAdapter interface {
	SourceFormat() Format
}

// DocumentSnapshotReader exposes a format adapter's current serialized bytes
// to read-only workflows such as inspection, export, and printing.
type DocumentSnapshotReader interface {
	Snapshot(context.Context) ([]byte, error)
}

type FormatFactory interface {
	Format() Format
	Open(context.Context, string) (DocumentAdapter, error)
	Create(context.Context) (DocumentAdapter, error)
}

type RuntimeOptions struct {
	WorkspaceDBPath   string
	Factories         []FormatFactory
	PrintService      PrintService
	LocalAccessBroker *localaccess.Broker
}

type DocumentInfo struct {
	DocumentID          string   `json:"document_id"`
	Format              Format   `json:"format"`
	SourcePath          string   `json:"source_path,omitempty"`
	Revision            uint64   `json:"revision"`
	Dirty               bool     `json:"dirty"`
	ReadOnly            bool     `json:"read_only"`
	SupportedOperations []string `json:"supported_operations"`
}

type runtimeSession struct {
	mu       sync.Mutex
	id       string
	format   Format
	path     string
	revision uint64
	adapter  DocumentAdapter
	closed   bool
}

type openingDocument struct {
	done chan struct{}
}

// Runtime owns live document adapters and their stable session identities.
// Gio window state remains outside this package.
type Runtime struct {
	mu                sync.Mutex
	closed            bool
	catalog           *Catalog
	events            *EventBus
	commands          *CommandBus
	confirmations     *confirmationService
	documentPlans     *documentPlanService
	workspaceDBPath   string
	printService      PrintService
	localAccessBroker *localaccess.Broker
	workspaceMu       sync.Mutex
	workspaceStore    *workspace.Store
	factories         map[Format]FormatFactory
	sessions          map[string]*runtimeSession
	paths             map[string]string
	pathReservations  map[string]string
	opening           map[string]*openingDocument
	viewHandlers      map[uint64]viewIntentRegistration
	nextViewHandler   uint64
}

// Queries returns the read-only query facade for live Runtime sessions.
func (r *Runtime) Queries() *QueryService { return &QueryService{runtime: r} }

func NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	factories := options.Factories
	if len(factories) == 0 {
		factories = defaultFormatFactories()
	}
	runtime := &Runtime{
		workspaceDBPath:   options.WorkspaceDBPath,
		printService:      options.PrintService,
		localAccessBroker: options.LocalAccessBroker,
		factories:         make(map[Format]FormatFactory, len(factories)),
		sessions:          make(map[string]*runtimeSession),
		paths:             make(map[string]string),
		pathReservations:  make(map[string]string),
		opening:           make(map[string]*openingDocument),
		viewHandlers:      make(map[uint64]viewIntentRegistration),
	}
	runtime.catalog = NewCatalog()
	runtime.events = newEventBus()
	runtime.commands = newCommandBus(runtime, runtime.catalog, runtime.events)
	runtime.confirmations = newConfirmationService(runtime)
	runtime.documentPlans = newDocumentPlanService()
	for _, factory := range factories {
		if factory == nil || factory.Format() == "" {
			return nil, fmt.Errorf("%w: factory and format are required", ErrUnsupportedFormat)
		}
		format := factory.Format()
		if _, exists := runtime.factories[format]; exists {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateFactory, format)
		}
		runtime.factories[format] = factory
	}
	if err := registerCoreCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetRangeClearCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetFillDownCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetFillRightCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetMergeCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetCellDateCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetCellTimeCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetCellTransferCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetProofreadCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetRecalculateCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetProtectionCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetImageCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetPivotTableCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetChartCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetShapeCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerJSONCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerMarkdownCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerMarkdownPDFExportCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerViewCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPDFViewCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPDFSearchCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPDFEditCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationViewCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationEditCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationInsertCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationDataObjectCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationMediaCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationPictureCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationTransitionCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationAnimationCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationAnimationEditCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationLayoutCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationBackgroundCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationThemeCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationGroupCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationSlideSizeCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationPDFExportCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationReviewCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerPresentationCommentCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerConfirmationCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordFormatCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordFormatPainterCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordHeaderFooterCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordPageSetupCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordLayoutQueryCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordReviewCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordCommentCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordMacroInspectionCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordTrackRevisionCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordProtectionCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordDocumentCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordTextInsertCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordWatermarkCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordImageCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordImageWrapCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordThemeEffectsCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordVersionCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordParagraphBulkCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordClipboardCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerLifecycleCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerDocumentExportCopyCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordPDFExportCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWordPrintCapability(runtime.catalog, runtime.printService); err != nil {
		return nil, err
	}
	if err := registerWordCapabilityCatalog(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetPDFExportCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerSpreadsheetPrintCapability(runtime.catalog, runtime.printService); err != nil {
		return nil, err
	}
	if err := registerPDFPrintCapability(runtime.catalog, runtime.printService); err != nil {
		return nil, err
	}
	if err := registerPresentationPrintCapability(runtime.catalog, runtime.printService); err != nil {
		return nil, err
	}
	if err := registerWorkspaceCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerWorkbookAuditCapability(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerAgentWorkflowCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if err := registerLocalAccessCapabilities(runtime.catalog); err != nil {
		return nil, err
	}
	if options.WorkspaceDBPath != "" {
		if _, err := runtime.OpenWorkspace(options.WorkspaceDBPath); err != nil {
			return nil, err
		}
	}
	return runtime, nil
}

// OpenWorkspace opens the Runtime-owned workspace store once. A desktop may
// keep a pointer for its asynchronous writer, but the Runtime controls lifetime.
func (r *Runtime) OpenWorkspace(path string) (*workspace.Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("workspace database path is empty")
	}
	canonical := canonicalPath(path)
	r.workspaceMu.Lock()
	defer r.workspaceMu.Unlock()
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return nil, ErrRuntimeClosed
	}
	if r.workspaceStore != nil {
		if canonicalPath(r.workspaceDBPath) != canonical {
			return nil, errors.New("runtime already owns a different workspace database")
		}
		return r.workspaceStore, nil
	}
	store, err := workspace.Open(canonical)
	if err != nil {
		return nil, err
	}
	r.workspaceDBPath = canonical
	r.workspaceStore = store
	return store, nil
}

func (r *Runtime) WorkspaceStore() *workspace.Store {
	r.workspaceMu.Lock()
	defer r.workspaceMu.Unlock()
	return r.workspaceStore
}

func (r *Runtime) Catalog() *Catalog {
	if r == nil {
		return nil
	}
	return r.catalog
}

func (r *Runtime) Commands() *CommandBus {
	if r == nil {
		return nil
	}
	return r.commands
}

func (r *Runtime) OpenDocument(ctx context.Context, path string) (DocumentInfo, error) {
	if err := ctx.Err(); err != nil {
		return DocumentInfo{}, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return DocumentInfo{}, ErrInvalidDocumentPath
	}
	format, err := formatForPath(path)
	if err != nil {
		return DocumentInfo{}, err
	}
	canonical := canonicalPath(path)
	r.mu.Lock()
	factory := r.factories[format]
	r.mu.Unlock()
	if factory == nil {
		return DocumentInfo{}, fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
	}
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return DocumentInfo{}, ErrRuntimeClosed
		}
		if id := r.paths[canonical]; id != "" {
			if session := r.sessions[id]; session != nil {
				r.mu.Unlock()
				return session.info(), nil
			}
			delete(r.paths, canonical)
		}
		if opening := r.opening[canonical]; opening != nil {
			r.mu.Unlock()
			select {
			case <-ctx.Done():
				return DocumentInfo{}, ctx.Err()
			case <-opening.done:
				continue
			}
		}
		opening := &openingDocument{done: make(chan struct{})}
		r.opening[canonical] = opening
		r.mu.Unlock()

		adapter, openErr := factory.Open(ctx, path)
		if openErr == nil && adapter == nil {
			openErr = errors.New("format factory returned a nil document adapter")
		}
		if openErr == nil && adapter.Format() != format {
			declared, ok := adapter.(SourceFormatAdapter)
			if !ok || declared.SourceFormat() != format {
				openErr = fmt.Errorf("format factory for %s returned adapter for %s", format, adapter.Format())
			}
		}
		if openErr == nil {
			openErr = ctx.Err()
		}

		r.mu.Lock()
		delete(r.opening, canonical)
		close(opening.done)
		if r.closed && openErr == nil {
			openErr = ErrRuntimeClosed
		}
		if openErr != nil {
			r.mu.Unlock()
			if adapter != nil {
				_ = adapter.Close(context.Background())
			}
			return DocumentInfo{}, openErr
		}
		if id := r.paths[canonical]; id != "" && r.sessions[id] != nil {
			existing := r.sessions[id]
			r.mu.Unlock()
			_ = adapter.Close(context.Background())
			return existing.info(), nil
		}
		session := newRuntimeSession(adapter, canonical, stableDocumentID(canonical))
		r.sessions[session.id] = session
		r.paths[canonical] = session.id
		info := session.infoLocked()
		r.mu.Unlock()
		r.publishDocumentEvent(EventDocumentOpened, info)
		return info, nil
	}
}

func (r *Runtime) CreateDocument(ctx context.Context, format Format) (DocumentInfo, error) {
	if err := ctx.Err(); err != nil {
		return DocumentInfo{}, err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return DocumentInfo{}, ErrRuntimeClosed
	}
	factory := r.factories[format]
	r.mu.Unlock()
	if factory == nil {
		return DocumentInfo{}, fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
	}
	adapter, err := factory.Create(ctx)
	if err != nil {
		return DocumentInfo{}, err
	}
	if adapter == nil {
		return DocumentInfo{}, errors.New("format factory returned a nil document adapter")
	}
	if adapter.Format() != format {
		_ = adapter.Close(context.Background())
		return DocumentInfo{}, fmt.Errorf("format factory for %s returned adapter for %s", format, adapter.Format())
	}
	if err := ctx.Err(); err != nil {
		_ = adapter.Close(context.Background())
		return DocumentInfo{}, err
	}
	id := draftDocumentID(format)
	if identity, ok := adapter.(interface{ DocumentID() string }); ok && identity.DocumentID() != "" {
		id = identity.DocumentID()
	}
	session := newRuntimeSession(adapter, "", id)
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		_ = adapter.Close(context.Background())
		return DocumentInfo{}, ErrRuntimeClosed
	}
	if existing := r.sessions[id]; existing != nil {
		r.mu.Unlock()
		_ = adapter.Close(context.Background())
		return existing.info(), nil
	}
	r.sessions[id] = session
	info := session.infoLocked()
	r.mu.Unlock()
	r.publishDocumentEvent(EventDocumentOpened, info)
	return info, nil
}

// RegisterLiveSession attaches a document session created by another in-process
// owner, such as the desktop's existing Gio session. It lets the application
// runtime index the same live adapter without copying a workspace snapshot.
func (r *Runtime) RegisterLiveSession(ctx context.Context, documentID string, revision uint64, adapter DocumentAdapter) (DocumentInfo, error) {
	if err := ctx.Err(); err != nil {
		return DocumentInfo{}, err
	}
	if documentID == "" {
		return DocumentInfo{}, ErrInvalidDocumentID
	}
	if adapter == nil || adapter.Format() == "" {
		return DocumentInfo{}, fmt.Errorf("%w: live adapter and format are required", ErrUnsupportedFormat)
	}
	path := adapter.Path()
	canonical := ""
	if path != "" {
		canonical = canonicalPath(path)
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return DocumentInfo{}, ErrRuntimeClosed
	}
	if canonical != "" {
		if opening := r.opening[canonical]; opening != nil {
			r.mu.Unlock()
			return DocumentInfo{}, ErrDocumentPathInUse
		}
		if id := r.paths[canonical]; id != "" {
			if existing := r.sessions[id]; existing != nil && id == documentID {
				r.mu.Unlock()
				return existing.info(), nil
			}
			r.mu.Unlock()
			return DocumentInfo{}, ErrDocumentPathInUse
		}
		if reservation := r.pathReservations[canonical]; reservation != "" && reservation != documentID {
			r.mu.Unlock()
			return DocumentInfo{}, ErrDocumentPathInUse
		}
	}
	if existing := r.sessions[documentID]; existing != nil {
		r.mu.Unlock()
		return existing.info(), nil
	}
	if revision == 0 {
		revision = 1
	}
	session := newRuntimeSession(adapter, canonical, documentID)
	session.revision = revision
	r.sessions[documentID] = session
	if canonical != "" {
		r.paths[canonical] = documentID
	}
	r.mu.Unlock()
	info := session.info()
	r.publishDocumentEvent(EventDocumentOpened, info)
	return info, nil
}

func (r *Runtime) ListDocuments(ctx context.Context) ([]DocumentInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, ErrRuntimeClosed
	}
	sessions := make([]*runtimeSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.mu.Unlock()
	items := make([]DocumentInfo, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, session.info())
	}
	sort.Slice(items, func(i, j int) bool { return items[i].DocumentID < items[j].DocumentID })
	return items, nil
}

func (r *Runtime) GetDocument(ctx context.Context, documentID string) (DocumentInfo, error) {
	if err := ctx.Err(); err != nil {
		return DocumentInfo{}, err
	}
	if documentID == "" {
		return DocumentInfo{}, ErrInvalidDocumentID
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return DocumentInfo{}, ErrRuntimeClosed
	}
	session := r.sessions[documentID]
	r.mu.Unlock()
	if session == nil {
		return DocumentInfo{}, ErrDocumentNotFound
	}
	return session.info(), nil
}

func (r *Runtime) SaveDocument(ctx context.Context, documentID string) (DocumentInfo, error) {
	info, _, err := r.SaveDocumentWithSnapshot(ctx, documentID)
	return info, err
}

// SaveDocumentWithSnapshot saves the live adapter and returns an optional
// format package snapshot for history/version recording.
func (r *Runtime) SaveDocumentWithSnapshot(ctx context.Context, documentID string) (DocumentInfo, []byte, error) {
	session, err := r.session(documentID)
	if err != nil {
		return DocumentInfo{}, nil, err
	}
	session.mu.Lock()
	var savedEvent *Event
	defer func() {
		session.mu.Unlock()
		if savedEvent != nil {
			r.publishDomainEvent(*savedEvent)
		}
	}()
	if err := ctx.Err(); err != nil {
		return DocumentInfo{}, nil, err
	}
	if session.closed {
		return DocumentInfo{}, nil, ErrDocumentNotFound
	}
	if session.adapter.ReadOnly() {
		return session.infoLocked(), nil, ErrReadOnlyDocument
	}
	if session.path == "" {
		return session.infoLocked(), nil, ErrNoSavePath
	}
	var snapshot []byte
	if saver, ok := session.adapter.(interface {
		SaveWithSnapshot(context.Context) ([]byte, error)
	}); ok {
		snapshot, err = saver.SaveWithSnapshot(ctx)
	} else {
		err = session.adapter.Save(ctx)
	}
	if err != nil {
		return session.infoLocked(), nil, err
	}
	info := session.infoLocked()
	savedEvent = documentEvent(EventDocumentSaved, info)
	return info, snapshot, nil
}

func (r *Runtime) SaveDocumentAs(ctx context.Context, documentID, targetPath string) (DocumentInfo, error) {
	info, _, err := r.SaveDocumentAsWithSnapshot(ctx, documentID, targetPath)
	return info, err
}

func (r *Runtime) SaveDocumentAsWithSnapshot(ctx context.Context, documentID, targetPath string) (DocumentInfo, []byte, error) {
	session, err := r.session(documentID)
	if err != nil {
		return DocumentInfo{}, nil, err
	}
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return DocumentInfo{}, nil, ErrInvalidDocumentPath
	}
	canonical := canonicalPath(targetPath)
	session.mu.Lock()
	var savedEvent *Event
	defer func() {
		session.mu.Unlock()
		if savedEvent != nil {
			r.publishDomainEvent(*savedEvent)
		}
	}()
	if err := ctx.Err(); err != nil {
		return DocumentInfo{}, nil, err
	}
	if session.closed {
		return DocumentInfo{}, nil, ErrDocumentNotFound
	}
	if session.adapter.ReadOnly() {
		return session.infoLocked(), nil, ErrReadOnlyDocument
	}
	if !extensionMatchesFormat(filepath.Ext(canonical), session.format) {
		return session.infoLocked(), nil, fmt.Errorf("target extension does not match document format %q", session.format)
	}
	if canonical == session.path {
		var snapshot []byte
		if saver, ok := session.adapter.(interface {
			SaveWithSnapshot(context.Context) ([]byte, error)
		}); ok {
			snapshot, err = saver.SaveWithSnapshot(ctx)
		} else {
			err = session.adapter.Save(ctx)
		}
		if err != nil {
			return session.infoLocked(), nil, err
		}
		info := session.infoLocked()
		savedEvent = documentEvent(EventDocumentSaved, info)
		return info, snapshot, nil
	}
	if _, err := os.Lstat(canonical); err == nil {
		return session.infoLocked(), nil, ErrTargetExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return session.infoLocked(), nil, err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return session.infoLocked(), nil, ErrRuntimeClosed
	}
	if existingID := r.paths[canonical]; existingID != "" && existingID != documentID {
		r.mu.Unlock()
		return session.infoLocked(), nil, ErrDocumentPathInUse
	}
	if reservation := r.pathReservations[canonical]; reservation != "" && reservation != documentID {
		r.mu.Unlock()
		return session.infoLocked(), nil, ErrDocumentPathInUse
	}
	r.pathReservations[canonical] = documentID
	r.mu.Unlock()
	var snapshot []byte
	if saver, ok := session.adapter.(interface {
		SaveAsWithSnapshot(context.Context, string) ([]byte, error)
	}); ok {
		snapshot, err = saver.SaveAsWithSnapshot(ctx, canonical)
	} else {
		err = session.adapter.SaveAs(ctx, canonical)
	}
	if err != nil {
		r.mu.Lock()
		delete(r.pathReservations, canonical)
		r.mu.Unlock()
		return session.infoLocked(), nil, err
	}
	r.mu.Lock()
	delete(r.pathReservations, canonical)
	if oldPath := session.path; oldPath != "" && r.paths[oldPath] == documentID {
		delete(r.paths, oldPath)
	}
	session.path = canonical
	r.paths[canonical] = documentID
	r.mu.Unlock()
	info := session.infoLocked()
	savedEvent = documentEvent(EventDocumentSaved, info)
	return info, snapshot, nil
}

func (r *Runtime) CloseDocument(ctx context.Context, documentID string) error {
	session, err := r.session(documentID)
	if err != nil {
		return err
	}
	session.mu.Lock()
	var closedEvent *Event
	defer func() {
		session.mu.Unlock()
		if closedEvent != nil {
			r.publishDomainEvent(*closedEvent)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	if session.closed {
		return ErrDocumentNotFound
	}
	if session.adapter.Dirty() {
		return ErrDirtyDocument
	}
	closedInfo := session.infoLocked()
	if err := session.adapter.Close(ctx); err != nil {
		return err
	}
	session.closed = true
	r.removeSession(session)
	closedEvent = documentEvent(EventDocumentClosed, closedInfo)
	return nil
}

// MarkDocumentChanged advances the live revision after a successful edit.
func (r *Runtime) MarkDocumentChanged(ctx context.Context, documentID string) (DocumentInfo, error) {
	session, err := r.session(documentID)
	if err != nil {
		return DocumentInfo{}, err
	}
	session.mu.Lock()
	var changedEvent *Event
	defer func() {
		session.mu.Unlock()
		if changedEvent != nil {
			r.publishDomainEvent(*changedEvent)
		}
	}()
	if err := ctx.Err(); err != nil {
		return DocumentInfo{}, err
	}
	if session.closed {
		return DocumentInfo{}, ErrDocumentNotFound
	}
	if session.adapter.ReadOnly() {
		return session.infoLocked(), ErrReadOnlyDocument
	}
	session.revision++
	info := session.infoLocked()
	changedEvent = documentEvent(EventDocumentChanged, info)
	return info, nil
}

// SyncDocumentRevision records a revision already advanced by a shared session.
// It never moves a live session backwards.
func (r *Runtime) SyncDocumentRevision(ctx context.Context, documentID string, revision uint64) error {
	session, err := r.session(documentID)
	if err != nil {
		return err
	}
	session.mu.Lock()
	var changedEvent *Event
	defer func() {
		session.mu.Unlock()
		if changedEvent != nil {
			r.publishDomainEvent(*changedEvent)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	if session.closed {
		return ErrDocumentNotFound
	}
	if revision > session.revision {
		session.revision = revision
		changedEvent = documentEvent(EventDocumentChanged, session.infoLocked())
	}
	return nil
}

// SyncDocumentPath updates a path changed by a desktop save operation.
func (r *Runtime) SyncDocumentPath(ctx context.Context, documentID, path string) error {
	session, err := r.session(documentID)
	if err != nil {
		return err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return ErrInvalidDocumentPath
	}
	canonical := canonicalPath(path)
	session.mu.Lock()
	defer session.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if session.closed {
		return ErrDocumentNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrRuntimeClosed
	}
	if existing := r.paths[canonical]; existing != "" && existing != documentID {
		return ErrDocumentPathInUse
	}
	if reservation := r.pathReservations[canonical]; reservation != "" && reservation != documentID {
		return ErrDocumentPathInUse
	}
	if session.path != "" && r.paths[session.path] == documentID {
		delete(r.paths, session.path)
	}
	session.path = canonical
	r.paths[canonical] = documentID
	return nil
}

// ForgetLiveSession removes an in-process session that was closed by its owner.
// The caller remains responsible for closing the adapter.
func (r *Runtime) ForgetLiveSession(ctx context.Context, documentID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions[documentID]
	if session == nil {
		return nil
	}
	if session.path != "" && r.paths[session.path] == documentID {
		delete(r.paths, session.path)
	}
	delete(r.sessions, documentID)
	return nil
}

func (r *Runtime) Close(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	sessions := make([]*runtimeSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.mu.Unlock()
	if r.localAccessBroker != nil {
		r.localAccessBroker.Close()
	}
	if r.confirmations != nil {
		r.confirmations.close()
	}
	var closeErr error
	for _, session := range sessions {
		session.mu.Lock()
		if !session.closed {
			if err := session.adapter.Close(ctx); err != nil && closeErr == nil {
				closeErr = err
			}
			session.closed = true
		}
		session.mu.Unlock()
	}
	r.mu.Lock()
	r.sessions = make(map[string]*runtimeSession)
	r.paths = make(map[string]string)
	r.viewHandlers = make(map[uint64]viewIntentRegistration)
	r.mu.Unlock()
	r.workspaceMu.Lock()
	store := r.workspaceStore
	r.workspaceStore = nil
	r.workspaceMu.Unlock()
	if store != nil {
		if err := store.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	if r.events != nil {
		r.events.close()
	}
	return closeErr
}

// ReleaseAgentTurn revokes the in-memory local directory scopes for one
// built-in Agent request.
func (r *Runtime) ReleaseAgentTurn(turnID string) {
	if r == nil || r.localAccessBroker == nil {
		return
	}
	r.localAccessBroker.ReleaseTurn(turnID)
}

func (r *Runtime) session(documentID string) (*runtimeSession, error) {
	if documentID == "" {
		return nil, ErrInvalidDocumentID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrRuntimeClosed
	}
	session := r.sessions[documentID]
	if session == nil {
		return nil, ErrDocumentNotFound
	}
	return session, nil
}

func (r *Runtime) removeSession(session *runtimeSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions[session.id] == session {
		delete(r.sessions, session.id)
	}
	if session.path != "" && r.paths[session.path] == session.id {
		delete(r.paths, session.path)
	}
}

func newRuntimeSession(adapter DocumentAdapter, path, id string) *runtimeSession {
	return &runtimeSession{id: id, format: adapter.Format(), path: path, revision: 1, adapter: adapter}
}

func (s *runtimeSession) info() DocumentInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.infoLocked()
}

func (s *runtimeSession) infoLocked() DocumentInfo {
	operations := []string{"inspect", "close"}
	if !s.adapter.ReadOnly() {
		operations = []string{"inspect", "modify", "save", "save_as", "close"}
	}
	return DocumentInfo{
		DocumentID:          s.id,
		Format:              s.format,
		SourcePath:          s.path,
		Revision:            s.revision,
		Dirty:               s.adapter.Dirty(),
		ReadOnly:            s.adapter.ReadOnly(),
		SupportedOperations: operations,
	}
}

func formatForPath(path string) (Format, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".docx":
		return FormatDocx, nil
	case ".doc":
		return FormatDOC, nil
	case ".xlsx":
		return FormatXLSX, nil
	case ".xls":
		return FormatXLS, nil
	case ".csv":
		return FormatCSV, nil
	case ".pptx":
		return FormatPPTX, nil
	case ".pdf":
		return FormatPDF, nil
	case ".json":
		return FormatJSON, nil
	case ".md", ".markdown":
		return FormatMarkdown, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedFormat, filepath.Ext(path))
	}
}

func extensionMatchesFormat(extension string, format Format) bool {
	extension = strings.ToLower(extension)
	switch format {
	case FormatDocx:
		return extension == ".docx"
	case FormatDOC:
		return extension == ".doc"
	case FormatXLSX:
		return extension == ".xlsx"
	case FormatXLS:
		return extension == ".xls"
	case FormatCSV:
		return extension == ".csv"
	case FormatPPTX:
		return extension == ".pptx"
	case FormatPDF:
		return extension == ".pdf"
	case FormatJSON:
		return extension == ".json"
	case FormatMarkdown:
		return extension == ".md" || extension == ".markdown"
	default:
		return false
	}
}

func canonicalPath(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	} else if parent, parentErr := filepath.EvalSymlinks(filepath.Dir(path)); parentErr == nil {
		path = filepath.Join(parent, filepath.Base(path))
	}
	return filepath.Clean(path)
}

func stableDocumentID(path string) string {
	sum := sha256.Sum256([]byte(path))
	return fmt.Sprintf("doc-%x", sum[:])[:20]
}

func draftDocumentID(format Format) string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err == nil {
		return fmt.Sprintf("draft-%s-%x", format, id[:])
	}
	return fmt.Sprintf("draft-%s-%x", format, sha256.Sum256([]byte(fmt.Sprintf("%p", &id))))
}
