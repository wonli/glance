package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/markdowndoc"
	"glance/internal/format/pdfdoc"
	"glance/internal/format/pptx"
	legacyxls "glance/internal/format/xls"
	"glance/internal/format/xlsx"
)

type adapterFuncs struct {
	mu                               sync.RWMutex
	format                           Format
	path                             string
	dirty                            func() bool
	readOnly                         bool
	snapshot                         func() ([]byte, error)
	presentationSnapshot             func(context.Context) (document.PresentationSnapshot, error)
	save                             func() error
	saveAs                           func(string) error
	saveWithSnapshot                 func() ([]byte, error)
	saveAsWithSnapshot               func(string) ([]byte, error)
	close                            func() error
	inspect                          func(context.Context, InspectRequest) (json.RawMessage, []Diagnostic, error)
	applyPDFCommand                  func(context.Context, document.PDFCommand) (bool, error)
	addPresentationSlide             func(context.Context) (int, error)
	deletePresentationSlide          func(context.Context, uint32) (int, error)
	duplicatePresentationSlide       func(context.Context, uint32) (uint32, int, error)
	addPresentationTextBox           func(context.Context, uint32, document.PresentationRect, string) (int64, error)
	addPresentationShape             func(context.Context, uint32, document.PresentationRect, string, string) (int64, error)
	addPresentationTable             func(context.Context, uint32, document.PresentationRect, document.PresentationTableData) (int64, error)
	addPresentationChart             func(context.Context, uint32, document.PresentationRect, document.PresentationChartData) (int64, error)
	addPresentationAudio             func(context.Context, uint32, document.PresentationRect, document.PresentationAudioData) (int64, error)
	addPresentationVideo             func(context.Context, uint32, document.PresentationRect, document.PresentationVideoData) (int64, error)
	addPresentationPicture           func(context.Context, uint32, document.PresentationRect, string, []byte, string, string, bool) (int64, document.PresentationRect, error)
	replacePresentationPicture       func(context.Context, uint32, int64, string, []byte) (bool, error)
	setPresentationPictureCrop       func(context.Context, uint32, int64, document.PresentationCrop) (bool, error)
	readPresentationElement          func(context.Context, uint32, int64) (document.PresentationElement, error)
	setPresentationElementText       func(context.Context, uint32, int64, string) (bool, error)
	setPresentationElementStyle      func(context.Context, uint32, int64, document.PresentationElementStylePatch) (bool, error)
	setPresentationElementTransform  func(context.Context, uint32, int64, document.PresentationRect, float64) (bool, error)
	setPresentationSlideNotes        func(context.Context, uint32, string, string) (bool, error)
	listPresentationComments         func(context.Context, uint32, int, int) ([]document.PresentationComment, int, error)
	addPresentationComment           func(context.Context, uint32, float64, float64, string, string) (string, bool, error)
	deletePresentationComment        func(context.Context, uint32, string) (bool, error)
	getPresentationTransition        func(context.Context, uint32) (document.PresentationTransition, error)
	presentationAnimations           func(context.Context, uint32) (document.PresentationAnimationSummary, error)
	setPresentationElementAnimation  func(context.Context, uint32, int64, string, int, uint64) (bool, error)
	setPresentationAnimationDuration func(context.Context, uint32, int64, int, int, uint64) (bool, error)
	deletePresentationAnimation      func(context.Context, uint32, int64, int, uint64) (int, error)
	presentationLayouts              func(context.Context) (document.PresentationLayoutSummary, error)
	setPresentationSlideLayout       func(context.Context, uint32, int, uint64) (document.PresentationLayoutOption, bool, error)
	setPresentationTransition        func(context.Context, uint32, document.PresentationTransitionPatch, bool) (document.PresentationTransition, []uint32, bool, error)
	setPresentationSlideBackground   func(context.Context, uint32, string) (bool, error)
	setPresentationSlideSize         func(context.Context, float64, float64) (bool, error)
	setPresentationTheme             func(context.Context, string) (bool, error)
	setPresentationThemePalette      func(context.Context, string) (bool, error)
	groupPresentationElements        func(context.Context, uint32, []uint32) (uint32, bool, error)
	setCellText                      func(context.Context, string, string, string) (bool, error)
	setCellValue                     func(context.Context, string, string, document.CellValue) (bool, error)
	setCellFormula                   func(context.Context, string, string, string) (bool, error)
	recalculateSpreadsheet           func(context.Context) error
	setCells                         func(context.Context, string, []document.CellEdit) (bool, error)
	setCellFormats                   func(context.Context, string, document.CellRange, document.CellFormatPatch) (bool, error)
	sortRange                        func(context.Context, document.SortRequest) (bool, error)
	applyHistory                     func(context.Context, bool) (bool, error)
	searchCells                      func(context.Context, string, string, int) ([]document.CellMatch, bool, error)
	applyStructure                   func(context.Context, document.StructureEdit) (bool, error)
	readSpreadsheetRange             func(context.Context, string, document.CellRange) ([][]document.Cell, error)
	readSpreadsheetGridWindow        func(context.Context, string, document.CellRange) (document.SheetWindow, error)
	readSpreadsheetPDFWindow         func(context.Context, string, document.CellRange) (document.SheetWindow, error)
	readSpreadsheetImageBytes        func(context.Context, string) ([]byte, error)
	getSpreadsheetPageLayout         func(context.Context, string) (document.SpreadsheetPageLayout, error)
	setSpreadsheetPageLayout         func(context.Context, string, SpreadsheetPageLayoutPatch) (document.SpreadsheetPageLayout, bool, error)
	getSpreadsheetGridlines          func(context.Context, string) (bool, error)
	setSpreadsheetGridlines          func(context.Context, string, bool) (bool, error)
	getSpreadsheetProtection         func(context.Context, string, string) (bool, error)
	setSpreadsheetProtection         func(context.Context, string, string, bool, string) (bool, error)
	setSpreadsheetDataValidation     func(context.Context, document.SpreadsheetDataValidationSpec, bool) (bool, error)
	setSpreadsheetConditionalFormat  func(context.Context, document.SpreadsheetConditionalFormatSpec, bool) (bool, error)
	createSpreadsheetTable           func(context.Context, document.SpreadsheetTableSpec) error
	createSpreadsheetPivotTable      func(context.Context, document.SpreadsheetPivotTableSpec) error
	createSpreadsheetChart           func(context.Context, document.SpreadsheetChartSpec) error
	createSpreadsheetShape           func(context.Context, document.SpreadsheetShapeSpec) error
	getSpreadsheetDefinedNames       func(context.Context) ([]document.SpreadsheetDefinedName, error)
	setSpreadsheetDefinedName        func(context.Context, document.SpreadsheetDefinedName) (bool, error)
	deleteSpreadsheetDefinedName     func(context.Context, string, string) (bool, error)
	splitSpreadsheetTextToColumns    func(context.Context, string, document.CellRange, int, string) (document.SpreadsheetTextToColumnsResult, error)
	removeSpreadsheetDuplicates      func(context.Context, string, document.CellRange, int) (document.SpreadsheetDuplicatesResult, error)
	getSpreadsheetPanes              func(context.Context, string) (document.SpreadsheetPaneFreeze, error)
	setSpreadsheetPanes              func(context.Context, string, document.SpreadsheetPaneFreeze) (bool, error)
	getSpreadsheetView               func(context.Context, string) (document.SpreadsheetSheetView, error)
	setSpreadsheetViewMode           func(context.Context, string, string) (bool, error)
	setSpreadsheetViewRuler          func(context.Context, string, bool) (bool, error)
	getSpreadsheetComments           func(context.Context, string) ([]document.SpreadsheetComment, error)
	addSpreadsheetComment            func(context.Context, string, document.SpreadsheetComment) (bool, error)
	deleteSpreadsheetComment         func(context.Context, string, string) (bool, error)
	flashFillSpreadsheet             func(context.Context, string, document.CellRange) (document.SpreadsheetFlashFillResult, error)
	getSpreadsheetHyperlink          func(context.Context, string, string) (document.CellHyperlink, bool, error)
	setSpreadsheetHyperlink          func(context.Context, string, string, string, string, bool) (bool, error)
	listSpreadsheetImages            func(context.Context, string) ([]document.SpreadsheetImage, error)
	insertSpreadsheetImage           func(context.Context, document.SpreadsheetImagePlacement, []byte) (string, error)
	replaceSpreadsheetImage          func(context.Context, string, string, string, []byte) (bool, error)
	moveSpreadsheetImage             func(context.Context, string, string, int, int, int, int) (bool, error)
	resizeSpreadsheetImage           func(context.Context, string, string, int) (bool, error)
	deleteSpreadsheetImage           func(context.Context, string, string) (bool, error)
	appendWordRun                    func(context.Context, string, string) (bool, error)
	replaceWordRun                   func(context.Context, string, string) (bool, error)
	replaceWordRange                 func(context.Context, string, int, int, string) (bool, error)
	applyWordFormatting              func(context.Context, string, string) (bool, error)
	readWordRange                    func(context.Context, string, int, int) (string, error)
	cutWordRange                     func(context.Context, string, int, int) (string, bool, error)
	setWordRunStyle                  func(context.Context, string, document.WordTextFormatPatch) (bool, error)
	setWordAlignment                 func(context.Context, string, document.WordAlignment) (bool, error)
	setWordIndents                   func(context.Context, string, int64, int64) (bool, error)
	setWordFontFamily                func(context.Context, string, string) (bool, error)
	setWordFontSize                  func(context.Context, string, float64) (bool, error)
	setWordList                      func(context.Context, string, document.WordListKind) (bool, error)
	setWordSpacing                   func(context.Context, string, *int64, *int64) (bool, error)
	setWordAllSpacing                func(context.Context, int64, int64) (int, bool, error)
	insertWordTable                  func(context.Context, string, int, int) (string, error)
	insertWordCover                  func(context.Context, string, string, string) error
	insertWordBreak                  func(context.Context, string, int, int, document.WordBreakType) (string, string, error)
	insertWordBlankPage              func(context.Context, string, int, int) (string, string, error)
	insertWordBookmark               func(context.Context, string, string) error
	insertWordHyperlink              func(context.Context, string, string, string) error
	insertWordTextBox                func(context.Context, string, string) error
	setWordHeader                    func(context.Context, string) (bool, error)
	setWordFooter                    func(context.Context, string) (bool, error)
	setWordPageNumber                func(context.Context, string) (bool, error)
	setWordOrientation               func(context.Context, string) (bool, error)
	setWordMargins                   func(context.Context, float64, float64, float64, float64) (bool, error)
	setWordPageSize                  func(context.Context, float64, float64) (bool, error)
	setWordColumns                   func(context.Context, int) (bool, error)
	setWordLineNumbers               func(context.Context, string) (bool, error)
	setWordPageBorders               func(context.Context, string) (bool, error)
	readWordBlocks                   func(context.Context, int, int) ([]document.WordBlock, int, error)
	readWordLayout                   func(context.Context) (document.WordLayoutData, error)
	setWordLanguage                  func(context.Context, string, string) (bool, error)
	getWordTrackState                func(context.Context) (bool, error)
	setWordTrackState                func(context.Context, bool) (bool, error)
	applyWordRevisions               func(context.Context, string) (int, error)
	getWordProtection                func(context.Context) (bool, error)
	setWordProtection                func(context.Context, bool) (bool, error)
	getWordProperties                func(context.Context) (document.WordDocumentProperties, error)
	patchWordProperties              func(context.Context, WordDocumentPropertiesPatch) (document.WordDocumentProperties, bool, error)
	getWordWatermark                 func(context.Context) (string, error)
	setWordWatermark                 func(context.Context, string) (bool, error)
	getPageBackground                func(context.Context) (string, error)
	setPageBackground                func(context.Context, string) (bool, error)
	insertWordImage                  func(context.Context, string, string, string, []byte) (string, error)
	moveWordImage                    func(context.Context, string, string, string) (bool, error)
	replaceWordImage                 func(context.Context, string, string, []byte) (bool, error)
	deleteWordImage                  func(context.Context, string) (bool, error)
	resizeWordImage                  func(context.Context, string, int64) (bool, error)
	rotateWordImage                  func(context.Context, string, float64) (float64, bool, error)
	setWordImageWrap                 func(context.Context, string, string) (string, bool, error)
	setWordThemeEffects              func(context.Context, string) (string, bool, error)
	appendWordText                   func(context.Context, string) (string, string, error)
	readWordComments                 func(context.Context) ([]document.WordComment, error)
	addWordComment                   func(context.Context, string, string, string) (string, bool, error)
	deleteWordComment                func(context.Context, string) (bool, error)
}

func applyWordPackageSnapshot(ctx context.Context, doc *docx.Document, update func() error) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	before, err := doc.Snapshot()
	if err != nil {
		return false, err
	}
	if err := update(); err != nil {
		return false, err
	}
	after, err := doc.Snapshot()
	if err != nil {
		return false, err
	}
	return !bytes.Equal(before, after), nil
}

func applyWordPageSetupSnapshot(ctx context.Context, doc *docx.Document, update func(document.WordPageSetupEditor) error) (bool, error) {
	editor, ok := any(doc).(document.WordPageSetupEditor)
	if !ok {
		return false, ErrCapabilityUnsupported
	}
	return applyWordPackageSnapshot(ctx, doc, func() error { return update(editor) })
}

var _ WordCommentAccess = (*adapterFuncs)(nil)
var _ PresentationCommentAccess = (*adapterFuncs)(nil)

func (a *adapterFuncs) Format() Format { return a.format }
func (a *adapterFuncs) Path() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.path
}
func (a *adapterFuncs) Dirty() bool    { return a.dirty != nil && a.dirty() }
func (a *adapterFuncs) ReadOnly() bool { return a.readOnly }
func (a *adapterFuncs) Snapshot(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a.snapshot == nil {
		return nil, ErrCapabilityUnsupported
	}
	return a.snapshot()
}
func (a *adapterFuncs) PresentationSnapshot(ctx context.Context) (document.PresentationSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return document.PresentationSnapshot{}, err
	}
	if a.presentationSnapshot == nil {
		return document.PresentationSnapshot{}, ErrCapabilityUnsupported
	}
	return a.presentationSnapshot(ctx)
}
func (a *adapterFuncs) Save(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.readOnly {
		return ErrReadOnlyDocument
	}
	if a.save == nil {
		return errors.New("save is not supported by this document format")
	}
	return a.save()
}
func (a *adapterFuncs) SaveWithSnapshot(ctx context.Context) ([]byte, error) {
	if a.saveWithSnapshot != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if a.readOnly {
			return nil, ErrReadOnlyDocument
		}
		return a.saveWithSnapshot()
	}
	return nil, a.Save(ctx)
}
func (a *adapterFuncs) SaveAs(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.readOnly {
		return ErrReadOnlyDocument
	}
	if a.saveAs == nil {
		return errors.New("save as is not supported by this document format")
	}
	if err := a.saveAs(path); err != nil {
		return err
	}
	a.mu.Lock()
	a.path = path
	a.mu.Unlock()
	return nil
}
func (a *adapterFuncs) SaveAsWithSnapshot(ctx context.Context, path string) ([]byte, error) {
	if a.saveAsWithSnapshot != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if a.readOnly {
			return nil, ErrReadOnlyDocument
		}
		data, err := a.saveAsWithSnapshot(path)
		if err != nil {
			return nil, err
		}
		a.mu.Lock()
		a.path = path
		a.mu.Unlock()
		return data, nil
	}
	return nil, a.SaveAs(ctx, path)
}
func (a *adapterFuncs) Close(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.close == nil {
		return nil
	}
	return a.close()
}

func (a *adapterFuncs) Inspect(ctx context.Context, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if a.inspect == nil {
		return nil, nil, ErrQueryNotSupported
	}
	return a.inspect(ctx, request)
}

func (a *adapterFuncs) ApplyPDFCommand(ctx context.Context, command document.PDFCommand) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.applyPDFCommand == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.applyPDFCommand(ctx, command)
}

func (a *adapterFuncs) AddPresentationSlide(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a.readOnly {
		return 0, ErrReadOnlyDocument
	}
	if a.addPresentationSlide == nil {
		return 0, ErrCapabilityUnsupported
	}
	return a.addPresentationSlide(ctx)
}

func (a *adapterFuncs) DeletePresentationSlide(ctx context.Context, slideID uint32) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a.readOnly {
		return 0, ErrReadOnlyDocument
	}
	if a.deletePresentationSlide == nil {
		return 0, ErrCapabilityUnsupported
	}
	return a.deletePresentationSlide(ctx, slideID)
}

func (a *adapterFuncs) DuplicatePresentationSlide(ctx context.Context, slideID uint32) (uint32, int, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	if a.readOnly {
		return 0, 0, ErrReadOnlyDocument
	}
	if a.duplicatePresentationSlide == nil {
		return 0, 0, ErrCapabilityUnsupported
	}
	return a.duplicatePresentationSlide(ctx, slideID)
}

func (a *adapterFuncs) GetPresentationTransition(ctx context.Context, slideID uint32) (document.PresentationTransition, error) {
	if err := ctx.Err(); err != nil {
		return document.PresentationTransition{}, err
	}
	if a.getPresentationTransition == nil {
		return document.PresentationTransition{}, ErrCapabilityUnsupported
	}
	return a.getPresentationTransition(ctx, slideID)
}

func (a *adapterFuncs) PresentationAnimations(ctx context.Context, slideID uint32) (document.PresentationAnimationSummary, error) {
	if err := ctx.Err(); err != nil {
		return document.PresentationAnimationSummary{}, err
	}
	if a.presentationAnimations == nil {
		return document.PresentationAnimationSummary{}, ErrCapabilityUnsupported
	}
	return a.presentationAnimations(ctx, slideID)
}

func (a *adapterFuncs) SetPresentationElementAnimation(ctx context.Context, slideID uint32, elementID int64, effect string, durationMS int, expectedRevision uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	a.mu.RLock()
	fn := a.setPresentationElementAnimation
	a.mu.RUnlock()
	if fn == nil {
		return false, ErrCapabilityUnsupported
	}
	return fn(ctx, slideID, elementID, effect, durationMS, expectedRevision)
}

func (a *adapterFuncs) SetPresentationAnimationDuration(ctx context.Context, slideID uint32, elementID int64, animationIndex, durationMS int, expectedRevision uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	a.mu.RLock()
	fn := a.setPresentationAnimationDuration
	a.mu.RUnlock()
	if fn == nil {
		return false, ErrCapabilityUnsupported
	}
	return fn(ctx, slideID, elementID, animationIndex, durationMS, expectedRevision)
}

func (a *adapterFuncs) DeletePresentationAnimation(ctx context.Context, slideID uint32, elementID int64, animationIndex int, expectedRevision uint64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	a.mu.RLock()
	fn := a.deletePresentationAnimation
	a.mu.RUnlock()
	if fn == nil {
		return 0, ErrCapabilityUnsupported
	}
	return fn(ctx, slideID, elementID, animationIndex, expectedRevision)
}

func (a *adapterFuncs) PresentationLayouts(ctx context.Context) (document.PresentationLayoutSummary, error) {
	if err := ctx.Err(); err != nil {
		return document.PresentationLayoutSummary{}, err
	}
	if a.presentationLayouts == nil {
		return document.PresentationLayoutSummary{}, ErrCapabilityUnsupported
	}
	return a.presentationLayouts(ctx)
}

func (a *adapterFuncs) SetPresentationSlideLayout(ctx context.Context, slideID uint32, layoutIndex int, expectedRevision uint64) (document.PresentationLayoutOption, bool, error) {
	if err := ctx.Err(); err != nil {
		return document.PresentationLayoutOption{}, false, err
	}
	if a.readOnly {
		return document.PresentationLayoutOption{}, false, ErrReadOnlyDocument
	}
	if a.setPresentationSlideLayout == nil {
		return document.PresentationLayoutOption{}, false, ErrCapabilityUnsupported
	}
	return a.setPresentationSlideLayout(ctx, slideID, layoutIndex, expectedRevision)
}

func (a *adapterFuncs) SetPresentationTransition(ctx context.Context, slideID uint32, patch document.PresentationTransitionPatch, applyToAll bool) (document.PresentationTransition, []uint32, bool, error) {
	if err := ctx.Err(); err != nil {
		return document.PresentationTransition{}, nil, false, err
	}
	if a.readOnly {
		return document.PresentationTransition{}, nil, false, ErrReadOnlyDocument
	}
	if a.setPresentationTransition == nil {
		return document.PresentationTransition{}, nil, false, ErrCapabilityUnsupported
	}
	return a.setPresentationTransition(ctx, slideID, patch, applyToAll)
}

func (a *adapterFuncs) SetPresentationSlideBackground(ctx context.Context, slideID uint32, colorHex string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setPresentationSlideBackground == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setPresentationSlideBackground(ctx, slideID, colorHex)
}

func (a *adapterFuncs) SetPresentationSlideSize(ctx context.Context, widthPT, heightPT float64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setPresentationSlideSize == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setPresentationSlideSize(ctx, widthPT, heightPT)
}

func (a *adapterFuncs) SetPresentationTheme(ctx context.Context, themeID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setPresentationTheme == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setPresentationTheme(ctx, themeID)
}

func (a *adapterFuncs) SetPresentationThemePalette(ctx context.Context, paletteID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setPresentationThemePalette == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setPresentationThemePalette(ctx, paletteID)
}

func (a *adapterFuncs) GroupPresentationElements(ctx context.Context, slideID uint32, elementIDs []uint32) (uint32, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	if a.readOnly {
		return 0, false, ErrReadOnlyDocument
	}
	if a.groupPresentationElements == nil {
		return 0, false, ErrCapabilityUnsupported
	}
	return a.groupPresentationElements(ctx, slideID, append([]uint32(nil), elementIDs...))
}

func (a *adapterFuncs) AddPresentationTable(ctx context.Context, slideID uint32, bounds document.PresentationRect, table document.PresentationTableData) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a.readOnly {
		return 0, ErrReadOnlyDocument
	}
	if a.addPresentationTable == nil {
		return 0, ErrCapabilityUnsupported
	}
	return a.addPresentationTable(ctx, slideID, bounds, table)
}

func (a *adapterFuncs) AddPresentationChart(ctx context.Context, slideID uint32, bounds document.PresentationRect, chart document.PresentationChartData) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a.readOnly {
		return 0, ErrReadOnlyDocument
	}
	if a.addPresentationChart == nil {
		return 0, ErrCapabilityUnsupported
	}
	return a.addPresentationChart(ctx, slideID, bounds, chart)
}

func (a *adapterFuncs) AddPresentationAudio(ctx context.Context, slideID uint32, bounds document.PresentationRect, audio document.PresentationAudioData) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a.readOnly {
		return 0, ErrReadOnlyDocument
	}
	if a.addPresentationAudio == nil {
		return 0, ErrCapabilityUnsupported
	}
	return a.addPresentationAudio(ctx, slideID, bounds, audio)
}

func (a *adapterFuncs) AddPresentationVideo(ctx context.Context, slideID uint32, bounds document.PresentationRect, video document.PresentationVideoData) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a.readOnly {
		return 0, ErrReadOnlyDocument
	}
	if a.addPresentationVideo == nil {
		return 0, ErrCapabilityUnsupported
	}
	return a.addPresentationVideo(ctx, slideID, bounds, video)
}

func (a *adapterFuncs) AddPresentationTextBox(ctx context.Context, slideID uint32, bounds document.PresentationRect, text string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a.readOnly {
		return 0, ErrReadOnlyDocument
	}
	if a.addPresentationTextBox == nil {
		return 0, ErrCapabilityUnsupported
	}
	return a.addPresentationTextBox(ctx, slideID, bounds, text)
}

func (a *adapterFuncs) AddPresentationShape(ctx context.Context, slideID uint32, bounds document.PresentationRect, geometry, text string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a.readOnly {
		return 0, ErrReadOnlyDocument
	}
	if a.addPresentationShape == nil {
		return 0, ErrCapabilityUnsupported
	}
	return a.addPresentationShape(ctx, slideID, bounds, geometry, text)
}

func (a *adapterFuncs) AddPresentationPicture(ctx context.Context, slideID uint32, bounds document.PresentationRect, mediaType string, data []byte, fit, altText string, decorative bool) (int64, document.PresentationRect, error) {
	if err := ctx.Err(); err != nil {
		return 0, document.PresentationRect{}, err
	}
	if a.readOnly {
		return 0, document.PresentationRect{}, ErrReadOnlyDocument
	}
	if a.addPresentationPicture == nil {
		return 0, document.PresentationRect{}, ErrCapabilityUnsupported
	}
	return a.addPresentationPicture(ctx, slideID, bounds, mediaType, data, fit, altText, decorative)
}

func (a *adapterFuncs) ReplacePresentationPicture(ctx context.Context, slideID uint32, elementID int64, mediaType string, data []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.replacePresentationPicture == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.replacePresentationPicture(ctx, slideID, elementID, mediaType, data)
}

func (a *adapterFuncs) SetPresentationPictureCrop(ctx context.Context, slideID uint32, elementID int64, crop document.PresentationCrop) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setPresentationPictureCrop == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setPresentationPictureCrop(ctx, slideID, elementID, crop)
}

func (a *adapterFuncs) SetPresentationElementText(ctx context.Context, slideID uint32, elementID int64, text string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setPresentationElementText == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setPresentationElementText(ctx, slideID, elementID, text)
}

func (a *adapterFuncs) ReadPresentationElement(ctx context.Context, slideID uint32, elementID int64) (document.PresentationElement, error) {
	if err := ctx.Err(); err != nil {
		return document.PresentationElement{}, err
	}
	if a.readPresentationElement == nil {
		return document.PresentationElement{}, ErrCapabilityUnsupported
	}
	return a.readPresentationElement(ctx, slideID, elementID)
}

func (a *adapterFuncs) SetPresentationElementStyle(ctx context.Context, slideID uint32, elementID int64, patch document.PresentationElementStylePatch) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setPresentationElementStyle == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setPresentationElementStyle(ctx, slideID, elementID, patch)
}

func (a *adapterFuncs) SetPresentationElementTransform(ctx context.Context, slideID uint32, elementID int64, bounds document.PresentationRect, rotationDeg float64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setPresentationElementTransform == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setPresentationElementTransform(ctx, slideID, elementID, bounds, rotationDeg)
}

func (a *adapterFuncs) SetPresentationSlideNotes(ctx context.Context, slideID uint32, expectedNotes, notes string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setPresentationSlideNotes == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setPresentationSlideNotes(ctx, slideID, expectedNotes, notes)
}

func (a *adapterFuncs) PresentationComments(ctx context.Context, slideID uint32, start, limit int) ([]document.PresentationComment, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if a.listPresentationComments == nil {
		return nil, 0, ErrCapabilityUnsupported
	}
	return a.listPresentationComments(ctx, slideID, start, limit)
}

func (a *adapterFuncs) AddPresentationComment(ctx context.Context, slideID uint32, xPT, yPT float64, author, text string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if a.readOnly {
		return "", false, ErrReadOnlyDocument
	}
	if a.addPresentationComment == nil {
		return "", false, ErrCapabilityUnsupported
	}
	return a.addPresentationComment(ctx, slideID, xPT, yPT, author, text)
}

func (a *adapterFuncs) DeletePresentationComment(ctx context.Context, slideID uint32, commentID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.deletePresentationComment == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.deletePresentationComment(ctx, slideID, commentID)
}

func (a *adapterFuncs) SetCellText(ctx context.Context, sheet, cell, value string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setCellText == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setCellText(ctx, sheet, cell, value)
}

func (a *adapterFuncs) SetSpreadsheetCellValue(ctx context.Context, sheet, cell string, value document.CellValue) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setCellValue == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setCellValue(ctx, sheet, cell, value)
}

func (a *adapterFuncs) SetSpreadsheetCellFormula(ctx context.Context, sheet, cell, formula string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setCellFormula == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setCellFormula(ctx, sheet, cell, formula)
}

func (a *adapterFuncs) RecalculateSpreadsheet(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.readOnly {
		return ErrReadOnlyDocument
	}
	if a.recalculateSpreadsheet == nil {
		return ErrCapabilityUnsupported
	}
	return a.recalculateSpreadsheet(ctx)
}

func (a *adapterFuncs) SetSpreadsheetCells(ctx context.Context, sheet string, edits []document.CellEdit) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setCells == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setCells(ctx, sheet, edits)
}

func (a *adapterFuncs) SetSpreadsheetCellFormats(ctx context.Context, sheet string, bounds document.CellRange, patch document.CellFormatPatch) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setCellFormats == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setCellFormats(ctx, sheet, bounds, patch)
}

func (a *adapterFuncs) SortSpreadsheetRange(ctx context.Context, request document.SortRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.sortRange == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.sortRange(ctx, request)
}

func (a *adapterFuncs) ApplySpreadsheetHistory(ctx context.Context, undo bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.applyHistory == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.applyHistory(ctx, undo)
}

func (a *adapterFuncs) FindSpreadsheetCells(ctx context.Context, sheet, query string, limit int) ([]document.CellMatch, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if a.searchCells == nil {
		return nil, false, ErrCapabilityUnsupported
	}
	return a.searchCells(ctx, sheet, query, limit)
}

func (a *adapterFuncs) ApplySpreadsheetStructure(ctx context.Context, edit document.StructureEdit) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.applyStructure == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.applyStructure(ctx, edit)
}

func (a *adapterFuncs) ReadSpreadsheetRange(ctx context.Context, sheet string, bounds document.CellRange) ([][]document.Cell, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a.readSpreadsheetRange == nil {
		return nil, ErrCapabilityUnsupported
	}
	return a.readSpreadsheetRange(ctx, sheet, bounds)
}

func (a *adapterFuncs) ReadSpreadsheetGridWindow(ctx context.Context, sheet string, bounds document.CellRange) (document.SheetWindow, error) {
	if err := ctx.Err(); err != nil {
		return document.SheetWindow{}, err
	}
	if a.readSpreadsheetGridWindow == nil {
		return document.SheetWindow{}, ErrCapabilityUnsupported
	}
	return a.readSpreadsheetGridWindow(ctx, sheet, bounds)
}

func (a *adapterFuncs) ReadSpreadsheetPDFWindow(ctx context.Context, sheet string, bounds document.CellRange) (document.SheetWindow, error) {
	if err := ctx.Err(); err != nil {
		return document.SheetWindow{}, err
	}
	if a.readSpreadsheetPDFWindow == nil {
		return document.SheetWindow{}, ErrCapabilityUnsupported
	}
	return a.readSpreadsheetPDFWindow(ctx, sheet, bounds)
}

func (a *adapterFuncs) ReadSpreadsheetImageBytes(ctx context.Context, imageID string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a.readSpreadsheetImageBytes == nil {
		return nil, ErrCapabilityUnsupported
	}
	return a.readSpreadsheetImageBytes(ctx, imageID)
}

func (a *adapterFuncs) GetSpreadsheetPageLayout(ctx context.Context, sheet string) (document.SpreadsheetPageLayout, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetPageLayout{}, err
	}
	if a.getSpreadsheetPageLayout == nil {
		return document.SpreadsheetPageLayout{}, ErrCapabilityUnsupported
	}
	return a.getSpreadsheetPageLayout(ctx, sheet)
}

func (a *adapterFuncs) SetSpreadsheetPageLayout(ctx context.Context, sheet string, patch SpreadsheetPageLayoutPatch) (document.SpreadsheetPageLayout, bool, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetPageLayout{}, false, err
	}
	if a.readOnly {
		return document.SpreadsheetPageLayout{}, false, ErrReadOnlyDocument
	}
	if a.setSpreadsheetPageLayout == nil {
		return document.SpreadsheetPageLayout{}, false, ErrCapabilityUnsupported
	}
	return a.setSpreadsheetPageLayout(ctx, sheet, patch)
}

func (a *adapterFuncs) GetSpreadsheetGridlines(ctx context.Context, sheet string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.getSpreadsheetGridlines == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.getSpreadsheetGridlines(ctx, sheet)
}

func (a *adapterFuncs) SetSpreadsheetGridlines(ctx context.Context, sheet string, show bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setSpreadsheetGridlines == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setSpreadsheetGridlines(ctx, sheet, show)
}

func (a *adapterFuncs) GetSpreadsheetProtection(ctx context.Context, scope, sheet string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.getSpreadsheetProtection == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.getSpreadsheetProtection(ctx, scope, sheet)
}

func (a *adapterFuncs) SetSpreadsheetProtection(ctx context.Context, scope, sheet string, enabled bool, password string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setSpreadsheetProtection == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setSpreadsheetProtection(ctx, scope, sheet, enabled, password)
}

func (a *adapterFuncs) SetSpreadsheetDataValidation(ctx context.Context, spec document.SpreadsheetDataValidationSpec, clear bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setSpreadsheetDataValidation == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setSpreadsheetDataValidation(ctx, spec, clear)
}

func (a *adapterFuncs) SetSpreadsheetConditionalFormat(ctx context.Context, spec document.SpreadsheetConditionalFormatSpec, clear bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setSpreadsheetConditionalFormat == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setSpreadsheetConditionalFormat(ctx, spec, clear)
}

func (a *adapterFuncs) CreateSpreadsheetTable(ctx context.Context, spec document.SpreadsheetTableSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.readOnly {
		return ErrReadOnlyDocument
	}
	if a.createSpreadsheetTable == nil {
		return ErrCapabilityUnsupported
	}
	return a.createSpreadsheetTable(ctx, spec)
}

func (a *adapterFuncs) CreateSpreadsheetPivotTable(ctx context.Context, spec document.SpreadsheetPivotTableSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.readOnly {
		return ErrReadOnlyDocument
	}
	if a.createSpreadsheetPivotTable == nil {
		return ErrCapabilityUnsupported
	}
	return a.createSpreadsheetPivotTable(ctx, spec)
}

func (a *adapterFuncs) CreateSpreadsheetChart(ctx context.Context, spec document.SpreadsheetChartSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.readOnly {
		return ErrReadOnlyDocument
	}
	if a.createSpreadsheetChart == nil {
		return ErrCapabilityUnsupported
	}
	return a.createSpreadsheetChart(ctx, spec)
}

func (a *adapterFuncs) CreateSpreadsheetShape(ctx context.Context, spec document.SpreadsheetShapeSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.readOnly {
		return ErrReadOnlyDocument
	}
	if a.createSpreadsheetShape == nil {
		return ErrCapabilityUnsupported
	}
	return a.createSpreadsheetShape(ctx, spec)
}

func (a *adapterFuncs) GetSpreadsheetDefinedNames(ctx context.Context) ([]document.SpreadsheetDefinedName, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a.getSpreadsheetDefinedNames == nil {
		return nil, ErrCapabilityUnsupported
	}
	return a.getSpreadsheetDefinedNames(ctx)
}

func (a *adapterFuncs) SetSpreadsheetDefinedName(ctx context.Context, name document.SpreadsheetDefinedName) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setSpreadsheetDefinedName == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setSpreadsheetDefinedName(ctx, name)
}

func (a *adapterFuncs) DeleteSpreadsheetDefinedName(ctx context.Context, name, scope string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.deleteSpreadsheetDefinedName == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.deleteSpreadsheetDefinedName(ctx, name, scope)
}

func (a *adapterFuncs) SplitSpreadsheetTextToColumns(ctx context.Context, sheet string, bounds document.CellRange, keyColumn int, delimiter string) (document.SpreadsheetTextToColumnsResult, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetTextToColumnsResult{}, err
	}
	if a.readOnly {
		return document.SpreadsheetTextToColumnsResult{}, ErrReadOnlyDocument
	}
	if a.splitSpreadsheetTextToColumns == nil {
		return document.SpreadsheetTextToColumnsResult{}, ErrCapabilityUnsupported
	}
	return a.splitSpreadsheetTextToColumns(ctx, sheet, bounds, keyColumn, delimiter)
}

func (a *adapterFuncs) RemoveSpreadsheetDuplicates(ctx context.Context, sheet string, bounds document.CellRange, keyColumn int) (document.SpreadsheetDuplicatesResult, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetDuplicatesResult{}, err
	}
	if a.readOnly {
		return document.SpreadsheetDuplicatesResult{}, ErrReadOnlyDocument
	}
	if a.removeSpreadsheetDuplicates == nil {
		return document.SpreadsheetDuplicatesResult{}, ErrCapabilityUnsupported
	}
	return a.removeSpreadsheetDuplicates(ctx, sheet, bounds, keyColumn)
}

func (a *adapterFuncs) GetSpreadsheetPanes(ctx context.Context, sheet string) (document.SpreadsheetPaneFreeze, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetPaneFreeze{}, err
	}
	if a.getSpreadsheetPanes == nil {
		return document.SpreadsheetPaneFreeze{}, ErrCapabilityUnsupported
	}
	return a.getSpreadsheetPanes(ctx, sheet)
}

func (a *adapterFuncs) SetSpreadsheetPanes(ctx context.Context, sheet string, panes document.SpreadsheetPaneFreeze) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setSpreadsheetPanes == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setSpreadsheetPanes(ctx, sheet, panes)
}

func (a *adapterFuncs) GetSpreadsheetView(ctx context.Context, sheet string) (document.SpreadsheetSheetView, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetSheetView{}, err
	}
	if a.getSpreadsheetView == nil {
		return document.SpreadsheetSheetView{}, ErrCapabilityUnsupported
	}
	return a.getSpreadsheetView(ctx, sheet)
}

func (a *adapterFuncs) SetSpreadsheetViewMode(ctx context.Context, sheet, mode string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setSpreadsheetViewMode == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setSpreadsheetViewMode(ctx, sheet, mode)
}

func (a *adapterFuncs) SetSpreadsheetViewRuler(ctx context.Context, sheet string, show bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setSpreadsheetViewRuler == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setSpreadsheetViewRuler(ctx, sheet, show)
}

func (a *adapterFuncs) GetSpreadsheetComments(ctx context.Context, sheet string) ([]document.SpreadsheetComment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a.getSpreadsheetComments == nil {
		return nil, ErrCapabilityUnsupported
	}
	return a.getSpreadsheetComments(ctx, sheet)
}

func (a *adapterFuncs) AddSpreadsheetComment(ctx context.Context, sheet string, comment document.SpreadsheetComment) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.addSpreadsheetComment == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.addSpreadsheetComment(ctx, sheet, comment)
}

func (a *adapterFuncs) DeleteSpreadsheetComment(ctx context.Context, sheet, cell string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.deleteSpreadsheetComment == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.deleteSpreadsheetComment(ctx, sheet, cell)
}

func (a *adapterFuncs) FlashFillSpreadsheet(ctx context.Context, sheet string, bounds document.CellRange) (document.SpreadsheetFlashFillResult, error) {
	if err := ctx.Err(); err != nil {
		return document.SpreadsheetFlashFillResult{}, err
	}
	if a.readOnly {
		return document.SpreadsheetFlashFillResult{}, ErrReadOnlyDocument
	}
	if a.flashFillSpreadsheet == nil {
		return document.SpreadsheetFlashFillResult{}, ErrCapabilityUnsupported
	}
	return a.flashFillSpreadsheet(ctx, sheet, bounds)
}

func (a *adapterFuncs) GetSpreadsheetHyperlink(ctx context.Context, sheet, cell string) (document.CellHyperlink, bool, error) {
	if err := ctx.Err(); err != nil {
		return document.CellHyperlink{}, false, err
	}
	if a.getSpreadsheetHyperlink == nil {
		return document.CellHyperlink{}, false, ErrCapabilityUnsupported
	}
	return a.getSpreadsheetHyperlink(ctx, sheet, cell)
}

func (a *adapterFuncs) SetSpreadsheetHyperlink(ctx context.Context, sheet, cell, target, tooltip string, remove bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setSpreadsheetHyperlink == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setSpreadsheetHyperlink(ctx, sheet, cell, target, tooltip, remove)
}

func (a *adapterFuncs) ListSpreadsheetImages(ctx context.Context, sheet string) ([]document.SpreadsheetImage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a.listSpreadsheetImages == nil {
		return nil, ErrCapabilityUnsupported
	}
	return a.listSpreadsheetImages(ctx, sheet)
}

func (a *adapterFuncs) InsertSpreadsheetImage(ctx context.Context, placement document.SpreadsheetImagePlacement, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if a.readOnly {
		return "", ErrReadOnlyDocument
	}
	if a.insertSpreadsheetImage == nil {
		return "", ErrCapabilityUnsupported
	}
	return a.insertSpreadsheetImage(ctx, placement, data)
}

func (a *adapterFuncs) ReplaceSpreadsheetImage(ctx context.Context, sheet, imageID, mediaType string, data []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.replaceSpreadsheetImage == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.replaceSpreadsheetImage(ctx, sheet, imageID, mediaType, data)
}

func (a *adapterFuncs) MoveSpreadsheetImage(ctx context.Context, sheet, imageID string, row, column, offsetX, offsetY int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.moveSpreadsheetImage == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.moveSpreadsheetImage(ctx, sheet, imageID, row, column, offsetX, offsetY)
}

func (a *adapterFuncs) ResizeSpreadsheetImage(ctx context.Context, sheet, imageID string, widthPx int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.resizeSpreadsheetImage == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.resizeSpreadsheetImage(ctx, sheet, imageID, widthPx)
}

func (a *adapterFuncs) DeleteSpreadsheetImage(ctx context.Context, sheet, imageID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.deleteSpreadsheetImage == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.deleteSpreadsheetImage(ctx, sheet, imageID)
}

func (a *adapterFuncs) ReplaceWordTextRun(ctx context.Context, runID, value string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.replaceWordRun == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.replaceWordRun(ctx, runID, value)
}

func (a *adapterFuncs) AppendWordTextRun(ctx context.Context, runID, value string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.appendWordRun == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.appendWordRun(ctx, runID, value)
}

func (a *adapterFuncs) ReplaceWordTextRange(ctx context.Context, runID string, start, end int, replacement string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.replaceWordRange == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.replaceWordRange(ctx, runID, start, end, replacement)
}

func (a *adapterFuncs) ApplyWordFormatting(ctx context.Context, sourceRunID, targetRunID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.applyWordFormatting == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.applyWordFormatting(ctx, sourceRunID, targetRunID)
}

func (a *adapterFuncs) ReadWordTextRange(ctx context.Context, runID string, start, end int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if a.readWordRange == nil {
		return "", ErrCapabilityUnsupported
	}
	return a.readWordRange(ctx, runID, start, end)
}

func (a *adapterFuncs) CutWordTextRange(ctx context.Context, runID string, start, end int) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if a.readOnly {
		return "", false, ErrReadOnlyDocument
	}
	if a.cutWordRange == nil {
		return "", false, ErrCapabilityUnsupported
	}
	return a.cutWordRange(ctx, runID, start, end)
}

func (a *adapterFuncs) AppendBodyTextParagraph(ctx context.Context, text string) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	if a.readOnly {
		return "", "", ErrReadOnlyDocument
	}
	if a.appendWordText == nil {
		return "", "", ErrCapabilityUnsupported
	}
	return a.appendWordText(ctx, text)
}

func (a *adapterFuncs) SetWordTextRunStyle(ctx context.Context, runID string, patch document.WordTextFormatPatch) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordRunStyle == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordRunStyle(ctx, runID, patch)
}

func (a *adapterFuncs) SetWordParagraphAlignment(ctx context.Context, paragraphID string, alignment document.WordAlignment) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordAlignment == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordAlignment(ctx, paragraphID, alignment)
}

func (a *adapterFuncs) SetWordParagraphIndents(ctx context.Context, paragraphID string, leftEMU, rightEMU int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordIndents == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordIndents(ctx, paragraphID, leftEMU, rightEMU)
}

func (a *adapterFuncs) SetWordTextRunFontFamily(ctx context.Context, runID, family string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordFontFamily == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordFontFamily(ctx, runID, family)
}

func (a *adapterFuncs) SetWordTextRunFontSize(ctx context.Context, runID string, points float64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordFontSize == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordFontSize(ctx, runID, points)
}

func (a *adapterFuncs) SetWordParagraphList(ctx context.Context, paragraphID string, kind document.WordListKind) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordList == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordList(ctx, paragraphID, kind)
}

func (a *adapterFuncs) SetWordParagraphSpacing(ctx context.Context, paragraphID string, beforeTwips, afterTwips *int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordSpacing == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordSpacing(ctx, paragraphID, beforeTwips, afterTwips)
}

func (a *adapterFuncs) SetWordAllParagraphSpacing(ctx context.Context, beforeTwips, afterTwips int64) (int, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	if a.readOnly {
		return 0, false, ErrReadOnlyDocument
	}
	if a.setWordAllSpacing == nil {
		return 0, false, ErrCapabilityUnsupported
	}
	return a.setWordAllSpacing(ctx, beforeTwips, afterTwips)
}

func (a *adapterFuncs) InsertWordTable(ctx context.Context, afterParagraphID string, rows, columns int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if a.readOnly {
		return "", ErrReadOnlyDocument
	}
	if a.insertWordTable == nil {
		return "", ErrCapabilityUnsupported
	}
	return a.insertWordTable(ctx, afterParagraphID, rows, columns)
}

func (a *adapterFuncs) InsertWordCoverPage(ctx context.Context, title, subtitle, author string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.readOnly {
		return ErrReadOnlyDocument
	}
	if a.insertWordCover == nil {
		return ErrCapabilityUnsupported
	}
	return a.insertWordCover(ctx, title, subtitle, author)
}

func (a *adapterFuncs) InsertWordPageBreak(ctx context.Context, runID string, start, end int) (string, string, error) {
	return a.InsertWordBreak(ctx, runID, start, end, document.WordBreakPage)
}

func (a *adapterFuncs) InsertWordBreak(ctx context.Context, runID string, start, end int, breakType document.WordBreakType) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	if a.readOnly {
		return "", "", ErrReadOnlyDocument
	}
	if a.insertWordBreak == nil {
		return "", "", ErrCapabilityUnsupported
	}
	return a.insertWordBreak(ctx, runID, start, end, breakType)
}

func (a *adapterFuncs) InsertWordBlankPage(ctx context.Context, runID string, start, end int) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	if a.readOnly {
		return "", "", ErrReadOnlyDocument
	}
	if a.insertWordBlankPage == nil {
		return "", "", ErrCapabilityUnsupported
	}
	return a.insertWordBlankPage(ctx, runID, start, end)
}

func (a *adapterFuncs) InsertWordBookmark(ctx context.Context, runID, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.readOnly {
		return ErrReadOnlyDocument
	}
	if a.insertWordBookmark == nil {
		return ErrCapabilityUnsupported
	}
	return a.insertWordBookmark(ctx, runID, name)
}

func (a *adapterFuncs) InsertWordHyperlink(ctx context.Context, runID, text, targetURL string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.readOnly {
		return ErrReadOnlyDocument
	}
	if a.insertWordHyperlink == nil {
		return ErrCapabilityUnsupported
	}
	return a.insertWordHyperlink(ctx, runID, text, targetURL)
}

func (a *adapterFuncs) InsertWordTextBox(ctx context.Context, afterParagraphID, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.readOnly {
		return ErrReadOnlyDocument
	}
	if a.insertWordTextBox == nil {
		return ErrCapabilityUnsupported
	}
	return a.insertWordTextBox(ctx, afterParagraphID, text)
}

func (a *adapterFuncs) SetWordHeader(ctx context.Context, text string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordHeader == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordHeader(ctx, text)
}

func (a *adapterFuncs) SetWordFooter(ctx context.Context, text string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordFooter == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordFooter(ctx, text)
}

func (a *adapterFuncs) SetWordPageNumber(ctx context.Context, alignment string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordPageNumber == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordPageNumber(ctx, alignment)
}

func (a *adapterFuncs) SetWordPageOrientation(ctx context.Context, orientation string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordOrientation == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordOrientation(ctx, orientation)
}

func (a *adapterFuncs) SetWordPageMargins(ctx context.Context, top, right, bottom, left float64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordMargins == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordMargins(ctx, top, right, bottom, left)
}

func (a *adapterFuncs) SetWordPageSize(ctx context.Context, width, height float64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordPageSize == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordPageSize(ctx, width, height)
}

func (a *adapterFuncs) SetWordPageColumns(ctx context.Context, columns int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordColumns == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordColumns(ctx, columns)
}

func (a *adapterFuncs) SetWordPageLineNumbers(ctx context.Context, mode string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordLineNumbers == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordLineNumbers(ctx, mode)
}

func (a *adapterFuncs) SetWordPageBorders(ctx context.Context, borderType string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordPageBorders == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordPageBorders(ctx, borderType)
}

func (a *adapterFuncs) ReadWordBlockWindow(ctx context.Context, start, limit int) ([]document.WordBlock, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if a.readWordBlocks == nil {
		return nil, 0, ErrCapabilityUnsupported
	}
	return a.readWordBlocks(ctx, start, limit)
}

func (a *adapterFuncs) ReadWordLayoutData(ctx context.Context) (document.WordLayoutData, error) {
	if err := ctx.Err(); err != nil {
		return document.WordLayoutData{}, err
	}
	if a.readWordLayout == nil {
		return document.WordLayoutData{}, ErrCapabilityUnsupported
	}
	return a.readWordLayout(ctx)
}

func (a *adapterFuncs) SetWordLanguage(ctx context.Context, runID, languageCode string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordLanguage == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordLanguage(ctx, runID, languageCode)
}

func (a *adapterFuncs) GetWordTrackRevisions(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.getWordTrackState == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.getWordTrackState(ctx)
}

func (a *adapterFuncs) SetWordTrackRevisions(ctx context.Context, enabled bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordTrackState == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordTrackState(ctx, enabled)
}

func (a *adapterFuncs) ApplyWordRevisionChanges(ctx context.Context, action string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a.readOnly {
		return 0, ErrReadOnlyDocument
	}
	if a.applyWordRevisions == nil {
		return 0, ErrCapabilityUnsupported
	}
	return a.applyWordRevisions(ctx, action)
}

func (a *adapterFuncs) GetWordDocumentProtection(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.getWordProtection == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.getWordProtection(ctx)
}

func (a *adapterFuncs) SetWordDocumentProtection(ctx context.Context, enabled bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordProtection == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordProtection(ctx, enabled)
}

func (a *adapterFuncs) GetWordDocumentProperties(ctx context.Context) (document.WordDocumentProperties, error) {
	if err := ctx.Err(); err != nil {
		return document.WordDocumentProperties{}, err
	}
	if a.getWordProperties == nil {
		return document.WordDocumentProperties{}, ErrCapabilityUnsupported
	}
	return a.getWordProperties(ctx)
}

func (a *adapterFuncs) PatchWordDocumentProperties(ctx context.Context, patch WordDocumentPropertiesPatch) (document.WordDocumentProperties, bool, error) {
	if err := ctx.Err(); err != nil {
		return document.WordDocumentProperties{}, false, err
	}
	if a.readOnly {
		return document.WordDocumentProperties{}, false, ErrReadOnlyDocument
	}
	if a.patchWordProperties == nil {
		return document.WordDocumentProperties{}, false, ErrCapabilityUnsupported
	}
	return a.patchWordProperties(ctx, patch)
}

func (a *adapterFuncs) GetWordWatermark(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if a.getWordWatermark == nil {
		return "", ErrCapabilityUnsupported
	}
	return a.getWordWatermark(ctx)
}

func (a *adapterFuncs) SetWordWatermark(ctx context.Context, text string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setWordWatermark == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setWordWatermark(ctx, text)
}

func (a *adapterFuncs) GetWordPageBackground(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if a.getPageBackground == nil {
		return "", ErrCapabilityUnsupported
	}
	return a.getPageBackground(ctx)
}

func (a *adapterFuncs) SetWordPageBackground(ctx context.Context, color string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.setPageBackground == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.setPageBackground(ctx, color)
}

func (a *adapterFuncs) InsertWordImage(ctx context.Context, paragraphID, afterInlineID, mediaType string, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if a.readOnly {
		return "", ErrReadOnlyDocument
	}
	if a.insertWordImage == nil {
		return "", ErrCapabilityUnsupported
	}
	return a.insertWordImage(ctx, paragraphID, afterInlineID, mediaType, data)
}

func (a *adapterFuncs) MoveWordImage(ctx context.Context, imageID, paragraphID, afterInlineID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.moveWordImage == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.moveWordImage(ctx, imageID, paragraphID, afterInlineID)
}

func (a *adapterFuncs) SetWordImageWrap(ctx context.Context, imageID, wrapMode string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if a.readOnly {
		return "", false, ErrReadOnlyDocument
	}
	if a.setWordImageWrap == nil {
		return "", false, ErrCapabilityUnsupported
	}
	return a.setWordImageWrap(ctx, imageID, wrapMode)
}

func (a *adapterFuncs) SetWordThemeEffects(ctx context.Context, profile string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if a.readOnly {
		return "", false, ErrReadOnlyDocument
	}
	if a.setWordThemeEffects == nil {
		return "", false, ErrCapabilityUnsupported
	}
	return a.setWordThemeEffects(ctx, profile)
}

func (a *adapterFuncs) ReplaceWordImage(ctx context.Context, imageID, mediaType string, data []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.replaceWordImage == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.replaceWordImage(ctx, imageID, mediaType, data)
}

func (a *adapterFuncs) DeleteWordImage(ctx context.Context, imageID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.deleteWordImage == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.deleteWordImage(ctx, imageID)
}

func (a *adapterFuncs) ResizeWordImage(ctx context.Context, imageID string, widthEMU int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.resizeWordImage == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.resizeWordImage(ctx, imageID, widthEMU)
}

func (a *adapterFuncs) RotateWordImage(ctx context.Context, imageID string, deltaDegrees float64) (float64, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	if a.readOnly {
		return 0, false, ErrReadOnlyDocument
	}
	if a.rotateWordImage == nil {
		return 0, false, ErrCapabilityUnsupported
	}
	return a.rotateWordImage(ctx, imageID, deltaDegrees)
}

func (a *adapterFuncs) WordComments(ctx context.Context) ([]document.WordComment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a.readWordComments == nil {
		return nil, ErrCapabilityUnsupported
	}
	return a.readWordComments(ctx)
}

func (a *adapterFuncs) AddWordComment(ctx context.Context, runID, author, text string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if a.readOnly {
		return "", false, ErrReadOnlyDocument
	}
	if a.addWordComment == nil {
		return "", false, ErrCapabilityUnsupported
	}
	return a.addWordComment(ctx, runID, author, text)
}

func (a *adapterFuncs) DeleteWordComment(ctx context.Context, commentID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a.readOnly {
		return false, ErrReadOnlyDocument
	}
	if a.deleteWordComment == nil {
		return false, ErrCapabilityUnsupported
	}
	return a.deleteWordComment(ctx, commentID)
}

type builtInFormatFactory struct{ format Format }

func (f builtInFormatFactory) Format() Format { return f.format }

func defaultFormatFactories() []FormatFactory {
	return []FormatFactory{
		builtInFormatFactory{format: FormatDocx},
		builtInFormatFactory{format: FormatXLSX},
		builtInFormatFactory{format: FormatXLS},
		builtInFormatFactory{format: FormatPPTX},
		builtInFormatFactory{format: FormatPDF},
		builtInFormatFactory{format: FormatJSON},
		builtInFormatFactory{format: FormatMarkdown},
	}
}

func (f builtInFormatFactory) Open(ctx context.Context, path string) (DocumentAdapter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch f.format {
	case FormatDocx:
		d, err := docx.Open(path)
		if err != nil {
			return nil, err
		}
		return wordAdapter(path, d), nil
	case FormatXLSX:
		d, err := xlsx.Open(path)
		if err != nil {
			return nil, err
		}
		return spreadsheetAdapter(FormatXLSX, path, d, !d.Editable(), d.Save, d.SaveAsDocument, d.Close), nil
	case FormatXLS:
		d, err := legacyxls.Open(path)
		if err != nil {
			return nil, err
		}
		return spreadsheetAdapter(FormatXLS, path, d, true, d.Save, nil, d.Close), nil
	case FormatPPTX:
		d, err := pptx.Open(path)
		if err != nil {
			return nil, err
		}
		return presentationAdapter(path, d), nil
	case FormatPDF:
		d, err := pdfdoc.Open(path)
		if err != nil {
			return nil, err
		}
		return pdfAdapter(path, d), nil
	case FormatJSON:
		d, err := jsondoc.Open(path)
		if err != nil {
			return nil, err
		}
		return jsonAdapter(path, d), nil
	case FormatMarkdown:
		d, err := markdowndoc.Open(path)
		if err != nil {
			return nil, err
		}
		return markdownAdapter(path, d), nil
	default:
		return nil, ErrUnsupportedFormat
	}
}

func (f builtInFormatFactory) Create(ctx context.Context) (DocumentAdapter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch f.format {
	case FormatDocx:
		d, err := docx.NewBlank()
		if err != nil {
			return nil, err
		}
		return wordAdapter("", d), nil
	case FormatXLSX:
		d := xlsx.NewBlank()
		return spreadsheetAdapter(FormatXLSX, "", d, !d.Editable(), d.Save, d.SaveAsDocument, d.Close), nil
	case FormatPPTX:
		d, err := pptx.NewBlank()
		if err != nil {
			return nil, err
		}
		return presentationAdapter("", d), nil
	case FormatPDF:
		d, err := pdfdoc.NewBlank()
		if err != nil {
			return nil, err
		}
		return pdfAdapter("", d), nil
	case FormatJSON:
		d := jsondoc.NewBlank()
		return jsonAdapter("", d), nil
	case FormatMarkdown:
		d := markdowndoc.NewBlank()
		return markdownAdapter("", d), nil
	case FormatXLS:
		return nil, errors.New("legacy XLS is read-only and cannot be created")
	default:
		return nil, ErrUnsupportedFormat
	}
}

func wordAdapter(path string, d *docx.Document) DocumentAdapter {
	return &adapterFuncs{
		format: FormatDocx, path: path, dirty: d.Dirty, snapshot: d.Snapshot, save: d.Save, saveAs: d.SaveAs,
		saveWithSnapshot: d.SaveWithSnapshot, saveAsWithSnapshot: d.SaveAsDocumentWithSnapshot, close: d.Close,
		replaceWordRun: func(ctx context.Context, runID, value string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if current, ok := editableWordRun(d.WordBlocks(), runID); ok && current == value {
				return false, nil
			}
			if err := d.ReplaceTextRun(runID, value); err != nil {
				return false, err
			}
			return true, nil
		},
		appendWordRun: func(ctx context.Context, runID, value string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			run, ok := editableWordTextRun(d.WordBlocks(), runID)
			if !ok {
				return false, ErrWordTextRunUnavailable
			}
			appended := value
			if run.Text != "" {
				appended = run.Text + " " + value
			}
			if err := d.ReplaceTextRun(runID, appended); err != nil {
				return false, err
			}
			return appended != run.Text, nil
		},
		replaceWordRange: func(ctx context.Context, runID string, start, end int, replacement string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			run, ok := editableWordTextRun(d.WordBlocks(), runID)
			if !ok {
				return false, ErrWordTextRunUnavailable
			}
			current := []rune(run.Text)
			if start < 0 || end < start || end > len(current) {
				return false, ErrInvalidCommandArguments
			}
			updated := make([]rune, 0, len(current)-(end-start)+len([]rune(replacement)))
			updated = append(updated, current[:start]...)
			updated = append(updated, []rune(replacement)...)
			updated = append(updated, current[end:]...)
			if string(updated) == run.Text {
				return false, nil
			}
			editor, ok := any(d).(document.WordTextRangeEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if err := editor.ReplaceTextRange(runID, start, end, replacement); err != nil {
				return false, err
			}
			return true, nil
		},
		applyWordFormatting: func(ctx context.Context, sourceRunID, targetRunID string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			blocks := d.WordBlocks()
			if _, ok := editableWordTextRun(blocks, sourceRunID); !ok {
				return false, ErrWordTextRunUnavailable
			}
			if _, ok := editableWordTextRun(blocks, targetRunID); !ok {
				return false, ErrWordTextRunUnavailable
			}
			painter, ok := any(d).(document.WordFormatPainter)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			return painter.ApplyWordFormatting(sourceRunID, targetRunID)
		},
		readWordRange: func(ctx context.Context, runID string, start, end int) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			run, ok := editableWordTextRun(d.WordBlocks(), runID)
			if !ok {
				return "", ErrWordTextRunUnavailable
			}
			current := []rune(run.Text)
			if start < 0 || end < start || end > len(current) {
				return "", ErrInvalidCommandArguments
			}
			return string(current[start:end]), nil
		},
		cutWordRange: func(ctx context.Context, runID string, start, end int) (string, bool, error) {
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
			run, ok := editableWordTextRun(d.WordBlocks(), runID)
			if !ok {
				return "", false, ErrWordTextRunUnavailable
			}
			current := []rune(run.Text)
			if start < 0 || end < start || end > len(current) {
				return "", false, ErrInvalidCommandArguments
			}
			selected := string(current[start:end])
			if len(selected) > MaxQueryResultBytes {
				return "", false, ErrQueryResultTooLarge
			}
			if start == end {
				return selected, false, nil
			}
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
			editor, ok := any(d).(document.WordTextRangeEditor)
			if !ok {
				return "", false, ErrCapabilityUnsupported
			}
			if err := editor.ReplaceTextRange(runID, start, end, ""); err != nil {
				return "", false, err
			}
			return selected, true, nil
		},
		appendWordText: func(ctx context.Context, text string) (string, string, error) {
			if err := ctx.Err(); err != nil {
				return "", "", err
			}
			return d.AppendBodyTextParagraph(text)
		},
		readWordComments: func(ctx context.Context) ([]document.WordComment, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			reader, ok := any(d).(document.WordCommentReader)
			if !ok {
				return nil, ErrCapabilityUnsupported
			}
			return reader.WordComments()
		},
		addWordComment: func(ctx context.Context, runID, author, text string) (string, bool, error) {
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
			editor, ok := any(d).(document.WordCommentEditor)
			if !ok {
				return "", false, ErrCapabilityUnsupported
			}
			return editor.AddWordComment(runID, author, text)
		},
		deleteWordComment: func(ctx context.Context, commentID string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := any(d).(document.WordCommentEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			return editor.DeleteWordComment(commentID)
		},
		setWordRunStyle: func(ctx context.Context, runID string, patch document.WordTextFormatPatch) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			run, ok := editableWordTextRun(d.WordBlocks(), runID)
			if !ok {
				return false, ErrWordTextRunUnavailable
			}
			if wordTextStylePatchUnchanged(run.Style, patch) {
				return false, nil
			}
			if err := d.SetTextRunStyle(runID, patch); err != nil {
				return false, err
			}
			return true, nil
		},
		setWordAlignment: func(ctx context.Context, paragraphID string, alignment document.WordAlignment) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			paragraph, ok := wordParagraphByID(d.WordBlocks(), paragraphID)
			if !ok {
				return false, ErrWordParagraphUnavailable
			}
			if paragraph.Alignment == alignment {
				return false, nil
			}
			if err := d.SetParagraphAlignment(paragraphID, alignment); err != nil {
				return false, err
			}
			return true, nil
		},
		setWordIndents: func(ctx context.Context, paragraphID string, leftEMU, rightEMU int64) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			paragraph, ok := wordParagraphByID(d.WordBlocks(), paragraphID)
			if !ok {
				return false, ErrWordParagraphUnavailable
			}
			if paragraph.LeftIndentEMU == leftEMU && paragraph.RightIndentEMU == rightEMU {
				return false, nil
			}
			if err := d.SetParagraphIndents(paragraphID, leftEMU, rightEMU); err != nil {
				return false, err
			}
			return true, nil
		},
		setWordFontFamily: func(ctx context.Context, runID, family string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			run, ok := editableWordTextRun(d.WordBlocks(), runID)
			if !ok {
				return false, ErrWordTextRunUnavailable
			}
			if run.Style.FontFamily == family {
				return false, nil
			}
			editor, ok := any(d).(document.WordTextFontFamilyEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if err := editor.SetTextRunFontFamily(runID, family); err != nil {
				return false, err
			}
			return true, nil
		},
		setWordFontSize: func(ctx context.Context, runID string, points float64) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			run, ok := editableWordTextRun(d.WordBlocks(), runID)
			if !ok {
				return false, ErrWordTextRunUnavailable
			}
			if run.Style.FontSize == points {
				return false, nil
			}
			editor, ok := any(d).(document.WordTextFontSizeEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if err := editor.SetTextRunFontSize(runID, points); err != nil {
				return false, err
			}
			return true, nil
		},
		setWordList: func(ctx context.Context, paragraphID string, kind document.WordListKind) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			paragraph, ok := wordParagraphByID(d.WordBlocks(), paragraphID)
			if !ok {
				return false, ErrWordParagraphUnavailable
			}
			if paragraph.ListKind == kind {
				return false, nil
			}
			editor, ok := any(d).(document.WordParagraphListEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if err := editor.SetParagraphList(paragraphID, kind); err != nil {
				return false, err
			}
			return true, nil
		},
		setWordSpacing: func(ctx context.Context, paragraphID string, beforeTwips, afterTwips *int64) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			paragraph, ok := wordParagraphByID(d.WordBlocks(), paragraphID)
			if !ok {
				return false, ErrWordParagraphUnavailable
			}
			if wordParagraphSpacingUnchanged(paragraph, beforeTwips, afterTwips) {
				return false, nil
			}
			editor, ok := any(d).(document.WordParagraphSpacingEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if err := editor.SetParagraphSpacing(paragraphID, beforeTwips, afterTwips); err != nil {
				return false, err
			}
			return true, nil
		},
		setWordAllSpacing: func(ctx context.Context, beforeTwips, afterTwips int64) (int, bool, error) {
			if err := ctx.Err(); err != nil {
				return 0, false, err
			}
			editor, ok := any(d).(document.WordParagraphSpacingEditor)
			if !ok {
				return 0, false, ErrCapabilityUnsupported
			}
			before, err := d.Snapshot()
			if err != nil {
				return 0, false, err
			}
			paragraphCount := countWordParagraphs(d.WordBlocks())
			if err := editor.SetAllParagraphsSpacing(beforeTwips, afterTwips); err != nil {
				return paragraphCount, false, err
			}
			after, err := d.Snapshot()
			if err != nil {
				return paragraphCount, false, err
			}
			return paragraphCount, !bytes.Equal(before, after), nil
		},
		insertWordTable: func(ctx context.Context, afterParagraphID string, rows, columns int) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if afterParagraphID != "" {
				if _, ok := wordBodyParagraphByID(d.WordBlocks(), afterParagraphID); !ok {
					return "", ErrWordParagraphUnavailable
				}
			}
			editor, ok := any(d).(document.WordTableInsertEditor)
			if !ok {
				return "", ErrCapabilityUnsupported
			}
			return editor.InsertTable(afterParagraphID, rows, columns)
		},
		insertWordCover: func(ctx context.Context, title, subtitle, author string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			editor, ok := any(d).(document.WordCoverPageEditor)
			if !ok {
				return ErrCapabilityUnsupported
			}
			return editor.InsertCoverPage(title, subtitle, author)
		},
		insertWordBreak: func(ctx context.Context, runID string, start, end int, breakType document.WordBreakType) (string, string, error) {
			if err := ctx.Err(); err != nil {
				return "", "", err
			}
			run, ok := editableWordTextRun(d.WordBlocks(), runID)
			if !ok {
				return "", "", ErrWordTextRunUnavailable
			}
			editor, ok := any(d).(document.WordPageBreakEditor)
			if !ok {
				return "", "", ErrCapabilityUnsupported
			}
			return editor.InsertBreakAtRange(runID, run.Text, start, end, breakType)
		},
		insertWordBlankPage: func(ctx context.Context, runID string, start, end int) (string, string, error) {
			if err := ctx.Err(); err != nil {
				return "", "", err
			}
			run, ok := editableWordTextRun(d.WordBlocks(), runID)
			if !ok {
				return "", "", ErrWordTextRunUnavailable
			}
			editor, ok := any(d).(document.WordPageBreakEditor)
			if !ok {
				return "", "", ErrCapabilityUnsupported
			}
			return editor.InsertBlankPageAtRange(runID, run.Text, start, end)
		},
		insertWordBookmark: func(ctx context.Context, runID, name string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, ok := editableWordTextRun(d.WordBlocks(), runID); !ok {
				return ErrWordTextRunUnavailable
			}
			editor, ok := any(d).(document.WordBookmarkEditor)
			if !ok {
				return ErrCapabilityUnsupported
			}
			return editor.InsertBookmark(runID, name)
		},
		insertWordHyperlink: func(ctx context.Context, runID, text, targetURL string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, ok := editableWordTextRun(d.WordBlocks(), runID); !ok {
				return ErrWordTextRunUnavailable
			}
			editor, ok := any(d).(document.WordHyperlinkEditor)
			if !ok {
				return ErrCapabilityUnsupported
			}
			return editor.InsertHyperlink(runID, text, targetURL)
		},
		insertWordTextBox: func(ctx context.Context, afterParagraphID, text string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if afterParagraphID != "" {
				if _, ok := wordBodyParagraphByID(d.WordBlocks(), afterParagraphID); !ok {
					return ErrWordParagraphUnavailable
				}
			}
			editor, ok := any(d).(document.WordTextBoxEditor)
			if !ok {
				return ErrCapabilityUnsupported
			}
			return editor.InsertTextBox(afterParagraphID, text)
		},
		setWordHeader: func(ctx context.Context, text string) (bool, error) {
			return applyWordPackageSnapshot(ctx, d, func() error {
				editor, ok := any(d).(document.WordHeaderFooterEditor)
				if !ok {
					return ErrCapabilityUnsupported
				}
				return editor.SetHeader(text)
			})
		},
		setWordFooter: func(ctx context.Context, text string) (bool, error) {
			return applyWordPackageSnapshot(ctx, d, func() error {
				editor, ok := any(d).(document.WordHeaderFooterEditor)
				if !ok {
					return ErrCapabilityUnsupported
				}
				return editor.SetFooter(text)
			})
		},
		setWordPageNumber: func(ctx context.Context, alignment string) (bool, error) {
			return applyWordPackageSnapshot(ctx, d, func() error {
				editor, ok := any(d).(document.WordHeaderFooterEditor)
				if !ok {
					return ErrCapabilityUnsupported
				}
				return editor.SetPageNumber(alignment)
			})
		},
		setWordOrientation: func(ctx context.Context, orientation string) (bool, error) {
			return applyWordPageSetupSnapshot(ctx, d, func(editor document.WordPageSetupEditor) error {
				return editor.SetPageOrientation(orientation)
			})
		},
		setWordMargins: func(ctx context.Context, top, right, bottom, left float64) (bool, error) {
			return applyWordPageSetupSnapshot(ctx, d, func(editor document.WordPageSetupEditor) error {
				return editor.SetPageMargins(top, right, bottom, left)
			})
		},
		setWordPageSize: func(ctx context.Context, width, height float64) (bool, error) {
			return applyWordPageSetupSnapshot(ctx, d, func(editor document.WordPageSetupEditor) error {
				return editor.SetPageSize(width, height)
			})
		},
		setWordColumns: func(ctx context.Context, columns int) (bool, error) {
			return applyWordPageSetupSnapshot(ctx, d, func(editor document.WordPageSetupEditor) error {
				return editor.SetPageColumns(columns)
			})
		},
		setWordLineNumbers: func(ctx context.Context, mode string) (bool, error) {
			return applyWordPageSetupSnapshot(ctx, d, func(editor document.WordPageSetupEditor) error {
				return editor.SetPageLineNumbers(mode)
			})
		},
		setWordPageBorders: func(ctx context.Context, borderType string) (bool, error) {
			return applyWordPageSetupSnapshot(ctx, d, func(editor document.WordPageSetupEditor) error {
				return editor.SetPageBorders(borderType)
			})
		},
		readWordBlocks: func(ctx context.Context, start, limit int) ([]document.WordBlock, int, error) {
			if err := ctx.Err(); err != nil {
				return nil, 0, err
			}
			return d.WordBlockWindow(start, limit)
		},
		readWordLayout: func(ctx context.Context) (document.WordLayoutData, error) {
			if err := ctx.Err(); err != nil {
				return document.WordLayoutData{}, err
			}
			reader, ok := any(d).(document.WordLayoutReader)
			if !ok {
				return document.WordLayoutData{}, ErrCapabilityUnsupported
			}
			return reader.WordLayoutData(), nil
		},
		setWordLanguage: func(ctx context.Context, runID, languageCode string) (bool, error) {
			return applyWordPackageSnapshot(ctx, d, func() error {
				blocks := d.WordBlocks()
				if runID != "" {
					if _, ok := editableWordTextRun(blocks, runID); !ok {
						return ErrWordTextRunUnavailable
					}
				} else if !wordBlocksContainEditableText(blocks) {
					return nil
				}
				editor, ok := any(d).(document.WordLanguageEditor)
				if !ok {
					return ErrCapabilityUnsupported
				}
				return editor.SetLanguage(runID, languageCode)
			})
		},
		getWordTrackState: func(ctx context.Context) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := any(d).(document.WordTrackRevisionsEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			return editor.GetTrackRevisions()
		},
		setWordTrackState: func(ctx context.Context, enabled bool) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := any(d).(document.WordTrackRevisionsEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			current, err := editor.GetTrackRevisions()
			if err != nil {
				return false, err
			}
			if current == enabled {
				return false, nil
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := editor.SetTrackRevisions(enabled); err != nil {
				return false, err
			}
			return true, nil
		},
		applyWordRevisions: func(ctx context.Context, action string) (int, error) {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			editor, ok := any(d).(document.WordTrackRevisionsEditor)
			if !ok {
				return 0, ErrCapabilityUnsupported
			}
			switch action {
			case "accept_all":
				return editor.AcceptAllRevisions()
			case "reject_all":
				return editor.RejectAllRevisions()
			default:
				return 0, ErrInvalidCommandArguments
			}
		},
		getWordProtection: func(ctx context.Context) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := any(d).(document.WordDocumentProtectionEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			return editor.GetWordDocumentProtection()
		},
		setWordProtection: func(ctx context.Context, enabled bool) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := any(d).(document.WordDocumentProtectionEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			current, err := editor.GetWordDocumentProtection()
			if err != nil {
				return false, err
			}
			if current == enabled {
				return false, nil
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := editor.SetWordDocumentProtection(enabled); err != nil {
				return false, err
			}
			return true, nil
		},
		getWordProperties: func(ctx context.Context) (document.WordDocumentProperties, error) {
			if err := ctx.Err(); err != nil {
				return document.WordDocumentProperties{}, err
			}
			editor, ok := any(d).(document.WordDocumentPropertiesEditor)
			if !ok {
				return document.WordDocumentProperties{}, ErrCapabilityUnsupported
			}
			return editor.GetWordDocumentProperties()
		},
		getWordWatermark: func(ctx context.Context) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			editor, ok := any(d).(document.WordWatermarkEditor)
			if !ok {
				return "", ErrCapabilityUnsupported
			}
			return editor.GetWatermark()
		},
		setWordWatermark: func(ctx context.Context, text string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := any(d).(document.WordWatermarkEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			current, err := editor.GetWatermark()
			if err != nil {
				return false, err
			}
			if current == text {
				return false, nil
			}
			if text == "" {
				err = editor.ClearWatermark()
			} else {
				err = editor.SetWatermark(text)
			}
			return err == nil, err
		},
		patchWordProperties: func(ctx context.Context, patch WordDocumentPropertiesPatch) (document.WordDocumentProperties, bool, error) {
			if err := ctx.Err(); err != nil {
				return document.WordDocumentProperties{}, false, err
			}
			editor, ok := any(d).(document.WordDocumentPropertiesEditor)
			if !ok {
				return document.WordDocumentProperties{}, false, ErrCapabilityUnsupported
			}
			current, err := editor.GetWordDocumentProperties()
			if err != nil {
				return document.WordDocumentProperties{}, false, err
			}
			updated := ApplyWordDocumentPropertiesPatch(current, patch)
			if updated == current {
				return current, false, nil
			}
			if err := ctx.Err(); err != nil {
				return document.WordDocumentProperties{}, false, err
			}
			if err := editor.SetWordDocumentProperties(updated); err != nil {
				return document.WordDocumentProperties{}, false, err
			}
			return updated, true, nil
		},
		getPageBackground: func(ctx context.Context) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			editor, ok := any(d).(document.WordPageBackgroundEditor)
			if !ok {
				return "", ErrCapabilityUnsupported
			}
			return editor.GetPageBackgroundColor()
		},
		insertWordImage: func(ctx context.Context, paragraphID, afterInlineID, mediaType string, data []byte) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			editor, ok := any(d).(document.WordImageEditor)
			if !ok {
				return "", ErrCapabilityUnsupported
			}
			return editor.InsertInlineImage(paragraphID, afterInlineID, mediaType, data)
		},
		moveWordImage: func(ctx context.Context, imageID, paragraphID, afterInlineID string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			blocks := d.WordBlocks()
			current := findWordImage(blocks, imageID)
			if current == nil {
				return false, ErrInvalidCommandArguments
			}
			if !current.Editable {
				return false, ErrCapabilityUnsupported
			}
			if imageID == afterInlineID {
				return false, ErrInvalidCommandArguments
			}
			if wordImageMoveIsNoop(blocks, imageID, paragraphID, afterInlineID) {
				return false, nil
			}
			editor, ok := any(d).(document.WordImageEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := editor.MoveInlineImage(imageID, paragraphID, afterInlineID); err != nil {
				return false, err
			}
			return true, nil
		},
		replaceWordImage: func(ctx context.Context, imageID, mediaType string, data []byte) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			current := findWordImage(d.WordBlocks(), imageID)
			if current == nil {
				return false, ErrInvalidCommandArguments
			}
			if !current.Editable {
				return false, ErrCapabilityUnsupported
			}
			if current.MediaType == mediaType {
				if reader, ok := any(d).(interface{ WordImageBytes(string) ([]byte, error) }); ok {
					previous, err := reader.WordImageBytes(imageID)
					if err != nil {
						return false, err
					}
					if bytes.Equal(previous, data) {
						return false, nil
					}
				}
			}
			editor, ok := any(d).(document.WordImageEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if err := editor.ReplaceInlineImage(imageID, mediaType, data); err != nil {
				return false, err
			}
			return true, nil
		},
		deleteWordImage: func(ctx context.Context, imageID string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			current := findWordImage(d.WordBlocks(), imageID)
			if current == nil {
				return false, ErrInvalidCommandArguments
			}
			if !current.Editable {
				return false, ErrCapabilityUnsupported
			}
			editor, ok := any(d).(document.WordImageEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if err := editor.DeleteInlineImage(imageID); err != nil {
				return false, err
			}
			return true, nil
		},
		resizeWordImage: func(ctx context.Context, imageID string, widthEMU int64) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			current := findWordImage(d.WordBlocks(), imageID)
			if current == nil {
				return false, ErrInvalidCommandArguments
			}
			if !current.Editable {
				return false, ErrCapabilityUnsupported
			}
			if current.WidthEMU == widthEMU {
				return false, nil
			}
			editor, ok := any(d).(document.WordImageEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if err := editor.ResizeInlineImage(imageID, widthEMU); err != nil {
				return false, err
			}
			return true, nil
		},
		rotateWordImage: func(ctx context.Context, imageID string, deltaDegrees float64) (float64, bool, error) {
			if err := ctx.Err(); err != nil {
				return 0, false, err
			}
			current := findWordImage(d.WordBlocks(), imageID)
			if current == nil {
				return 0, false, ErrInvalidCommandArguments
			}
			if !current.Editable {
				return 0, false, ErrCapabilityUnsupported
			}
			rotation := normalizeWordImageRotation(current.RotationDegrees + deltaDegrees)
			if math.Abs(rotation-current.RotationDegrees) < 1e-9 {
				return current.RotationDegrees, false, nil
			}
			rotator, ok := any(d).(document.WordImageRotator)
			if !ok {
				return 0, false, ErrCapabilityUnsupported
			}
			if err := rotator.SetInlineImageRotation(imageID, rotation); err != nil {
				return 0, false, err
			}
			return rotation, true, nil
		},
		setWordImageWrap: func(ctx context.Context, imageID, wrapMode string) (string, bool, error) {
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
			current := findWordImage(d.WordBlocks(), imageID)
			if current == nil {
				return "", false, ErrInvalidCommandArguments
			}
			if !current.WrapEditable {
				return current.WrapMode, false, ErrCapabilityUnsupported
			}
			previous := current.WrapMode
			if previous == wrapMode {
				return previous, false, nil
			}
			editor, ok := any(d).(document.WordImageWrapEditor)
			if !ok {
				return previous, false, ErrCapabilityUnsupported
			}
			if err := ctx.Err(); err != nil {
				return previous, false, err
			}
			if err := editor.SetWordImageWrap(imageID, wrapMode); err != nil {
				return previous, false, err
			}
			return previous, true, nil
		},
		setWordThemeEffects: func(ctx context.Context, profile string) (string, bool, error) {
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
			editor, ok := any(d).(document.WordThemeEffectsEditor)
			if !ok || !editor.CanSetWordThemeEffects() {
				return "", false, ErrCapabilityUnsupported
			}
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
			return editor.SetWordThemeEffects(profile)
		},
		setPageBackground: func(ctx context.Context, color string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := any(d).(document.WordPageBackgroundEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			current, err := editor.GetPageBackgroundColor()
			if err != nil {
				return false, err
			}
			if strings.EqualFold(current, color) {
				return false, nil
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := editor.SetPageBackgroundColor(color); err != nil {
				return false, err
			}
			return true, nil
		},
		inspect: func(ctx context.Context, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
			return InspectWord(ctx, d, request)
		},
	}
}

func sameSpreadsheetDefinedNameScope(left, right string) bool {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left == "" {
		left = "Workbook"
	}
	if right == "" {
		right = "Workbook"
	}
	return strings.EqualFold(left, right)
}

func spreadsheetAdapter(format Format, path string, d document.Spreadsheet, readOnly bool, save func() error, saveAs func(string) error, close func() error) DocumentAdapter {
	return &adapterFuncs{
		format: format, path: path, dirty: d.Dirty, readOnly: readOnly, save: save, saveAs: saveAs, close: close,
		snapshot: func() ([]byte, error) {
			snapshooter, ok := d.(interface{ Snapshot() ([]byte, error) })
			if !ok {
				return nil, ErrCapabilityUnsupported
			}
			return snapshooter.Snapshot()
		},
		recalculateSpreadsheet: func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			recalculator, ok := d.(document.SpreadsheetRecalculateEditor)
			if !ok {
				return ErrCapabilityUnsupported
			}
			return recalculator.RecalculateFormulas()
		},
		setCellFormula: func(ctx context.Context, sheet, cell, formula string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetFormulaEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			cleanFormula := strings.TrimPrefix(strings.TrimSpace(formula), "=")
			current, err := editor.GetCellFormula(sheet, cell)
			if err != nil {
				return false, err
			}
			if current == cleanFormula {
				return false, nil
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := editor.SetCellFormula(sheet, cell, formula); err != nil {
				return false, err
			}
			return true, nil
		},
		setCells: func(ctx context.Context, sheet string, edits []document.CellEdit) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetBatchChangeEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			return editor.SetCellsChanged(sheet, edits)
		},
		setCellFormats: func(ctx context.Context, sheet string, bounds document.CellRange, patch document.CellFormatPatch) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetFormatChangeEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			return editor.SetCellFormatsChanged(sheet, bounds, patch)
		},
		sortRange: func(ctx context.Context, request document.SortRequest) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetSortChangeEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			return editor.SortRangeChanged(request)
		},
		applyHistory: func(ctx context.Context, undo bool) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			history, ok := d.(document.SpreadsheetHistory)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if undo {
				return history.Undo()
			}
			return history.Redo()
		},
		searchCells: func(ctx context.Context, sheet, query string, limit int) ([]document.CellMatch, bool, error) {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
			searcher, ok := d.(document.SpreadsheetSearcher)
			if !ok {
				return nil, false, ErrCapabilityUnsupported
			}
			matches, err := searcher.SearchSheet(sheet, query)
			if err != nil {
				return nil, false, err
			}
			truncated := len(matches) > limit
			if truncated {
				matches = matches[:limit]
			}
			if matches == nil {
				matches = []document.CellMatch{}
			}
			return matches, truncated, nil
		},
		applyStructure: func(ctx context.Context, edit document.StructureEdit) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetStructureEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if edit.Kind == document.StructureRenameSheet && edit.NewName == edit.Sheet {
				return false, nil
			}
			if err := editor.EditStructure(edit); err != nil {
				return false, err
			}
			return true, nil
		},
		readSpreadsheetRange: func(ctx context.Context, sheet string, bounds document.CellRange) ([][]document.Cell, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			reader, ok := d.(document.SpreadsheetRangeReader)
			if !ok {
				return nil, ErrCapabilityUnsupported
			}
			return reader.ReadRange(sheet, bounds)
		},
		readSpreadsheetGridWindow: func(ctx context.Context, sheet string, bounds document.CellRange) (document.SheetWindow, error) {
			if err := ctx.Err(); err != nil {
				return document.SheetWindow{}, err
			}
			if !validCellRange(bounds, 10_000) {
				return document.SheetWindow{}, ErrInvalidCommandArguments
			}
			reader, ok := d.(document.GridReader)
			if !ok {
				return document.SheetWindow{}, ErrCapabilityUnsupported
			}
			rows := bounds.EndRow - bounds.StartRow + 1
			columns := bounds.EndColumn - bounds.StartColumn + 1
			return reader.Window(sheet, bounds.StartRow, bounds.StartColumn, rows, columns)
		},
		readSpreadsheetPDFWindow: func(ctx context.Context, sheet string, bounds document.CellRange) (document.SheetWindow, error) {
			if err := ctx.Err(); err != nil {
				return document.SheetWindow{}, err
			}
			if !validCellRange(bounds, 500_000) || bounds.EndRow-bounds.StartRow+1 > 5000 || bounds.EndColumn-bounds.StartColumn+1 > 100 {
				return document.SheetWindow{}, ErrInvalidCommandArguments
			}
			reader, ok := d.(document.GridReader)
			if !ok {
				return document.SheetWindow{}, ErrCapabilityUnsupported
			}
			rows := bounds.EndRow - bounds.StartRow + 1
			columns := bounds.EndColumn - bounds.StartColumn + 1
			return reader.Window(sheet, bounds.StartRow, bounds.StartColumn, rows, columns)
		},
		getSpreadsheetPageLayout: func(ctx context.Context, sheet string) (document.SpreadsheetPageLayout, error) {
			if err := ctx.Err(); err != nil {
				return document.SpreadsheetPageLayout{}, err
			}
			reader, ok := d.(document.SpreadsheetPageLayoutEditor)
			if !ok {
				return document.SpreadsheetPageLayout{}, ErrCapabilityUnsupported
			}
			return reader.GetSheetPageLayout(sheet)
		},
		setSpreadsheetPageLayout: func(ctx context.Context, sheet string, patch SpreadsheetPageLayoutPatch) (document.SpreadsheetPageLayout, bool, error) {
			if err := ctx.Err(); err != nil {
				return document.SpreadsheetPageLayout{}, false, err
			}
			editor, ok := d.(document.SpreadsheetPageLayoutEditor)
			if !ok {
				return document.SpreadsheetPageLayout{}, false, ErrCapabilityUnsupported
			}
			return ApplySpreadsheetPageLayoutPatch(editor, sheet, patch)
		},
		getSpreadsheetGridlines: func(ctx context.Context, sheet string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			reader, ok := d.(document.SpreadsheetGridlinesEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			return reader.GetSheetGridlines(sheet)
		},
		setSpreadsheetGridlines: func(ctx context.Context, sheet string, show bool) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetGridlinesEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			return ApplySpreadsheetGridlines(ctx, editor, sheet, show)
		},
		getSpreadsheetProtection: func(ctx context.Context, scope, sheet string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			switch scope {
			case "sheet":
				if sheet == "" {
					return false, ErrInvalidCommandArguments
				}
				reader, ok := d.(document.SpreadsheetProtectionEditor)
				if !ok {
					return false, ErrCapabilityUnsupported
				}
				return reader.GetSheetProtection(sheet)
			case "workbook":
				reader, ok := d.(document.SpreadsheetWorkbookProtectionEditor)
				if !ok {
					return false, ErrCapabilityUnsupported
				}
				return reader.GetWorkbookProtection()
			default:
				return false, ErrInvalidCommandArguments
			}
		},
		setSpreadsheetProtection: func(ctx context.Context, scope, sheet string, enabled bool, password string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			switch scope {
			case "sheet":
				if sheet == "" {
					return false, ErrInvalidCommandArguments
				}
				editor, ok := d.(document.SpreadsheetProtectionEditor)
				if !ok {
					return false, ErrCapabilityUnsupported
				}
				current, err := editor.GetSheetProtection(sheet)
				if err != nil || current == enabled {
					return false, err
				}
				if err := ctx.Err(); err != nil {
					return false, err
				}
				if enabled {
					err = editor.ProtectSheet(sheet, password)
				} else {
					err = editor.UnprotectSheet(sheet, password)
				}
				return err == nil, err
			case "workbook":
				editor, ok := d.(document.SpreadsheetWorkbookProtectionEditor)
				if !ok {
					return false, ErrCapabilityUnsupported
				}
				current, err := editor.GetWorkbookProtection()
				if err != nil || current == enabled {
					return false, err
				}
				if err := ctx.Err(); err != nil {
					return false, err
				}
				if enabled {
					err = editor.ProtectWorkbook(password)
				} else {
					err = editor.UnprotectWorkbook(password)
				}
				return err == nil, err
			default:
				return false, ErrInvalidCommandArguments
			}
		},
		setSpreadsheetDataValidation: func(ctx context.Context, spec document.SpreadsheetDataValidationSpec, clear bool) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetDataValidationEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if clear {
				return editor.DeleteDataValidation(spec.Sheet, spec.Range)
			}
			return editor.AddDataValidation(spec)
		},
		setSpreadsheetConditionalFormat: func(ctx context.Context, spec document.SpreadsheetConditionalFormatSpec, clear bool) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetConditionalFormatEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			if clear {
				return editor.ClearConditionalFormat(spec.Sheet, spec.Range)
			}
			return editor.SetConditionalFormat(spec)
		},
		createSpreadsheetTable: func(ctx context.Context, spec document.SpreadsheetTableSpec) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			editor, ok := d.(document.SpreadsheetTableEditor)
			if !ok {
				return ErrCapabilityUnsupported
			}
			return editor.AddTable(spec)
		},
		createSpreadsheetPivotTable: func(ctx context.Context, spec document.SpreadsheetPivotTableSpec) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			editor, ok := d.(document.SpreadsheetPivotTableEditor)
			if !ok {
				return ErrCapabilityUnsupported
			}
			return editor.AddPivotTable(spec)
		},
		createSpreadsheetChart: func(ctx context.Context, spec document.SpreadsheetChartSpec) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			editor, ok := d.(document.SpreadsheetChartEditor)
			if !ok {
				return ErrCapabilityUnsupported
			}
			return editor.AddChart(spec)
		},
		createSpreadsheetShape: func(ctx context.Context, spec document.SpreadsheetShapeSpec) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			editor, ok := d.(document.SpreadsheetShapeEditor)
			if !ok {
				return ErrCapabilityUnsupported
			}
			return editor.AddShape(spec)
		},
		getSpreadsheetDefinedNames: func(ctx context.Context) ([]document.SpreadsheetDefinedName, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			reader, ok := d.(document.SpreadsheetDefinedNameEditor)
			if !ok {
				return nil, ErrCapabilityUnsupported
			}
			return reader.GetDefinedNames()
		},
		setSpreadsheetDefinedName: func(ctx context.Context, name document.SpreadsheetDefinedName) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetDefinedNameEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			current, err := editor.GetDefinedNames()
			if err != nil {
				return false, err
			}
			for _, existing := range current {
				if strings.EqualFold(existing.Name, name.Name) && sameSpreadsheetDefinedNameScope(existing.Scope, name.Scope) && existing.Ref == name.Ref && existing.Comment == name.Comment {
					return false, nil
				}
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := editor.SetDefinedName(name); err != nil {
				return false, err
			}
			return true, nil
		},
		deleteSpreadsheetDefinedName: func(ctx context.Context, name, scope string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetDefinedNameEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			current, err := editor.GetDefinedNames()
			if err != nil {
				return false, err
			}
			found := false
			for _, existing := range current {
				if strings.EqualFold(existing.Name, name) && sameSpreadsheetDefinedNameScope(existing.Scope, scope) {
					found = true
					break
				}
			}
			if !found {
				return false, nil
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := editor.DeleteDefinedName(name, scope); err != nil {
				return false, err
			}
			return true, nil
		},
		splitSpreadsheetTextToColumns: func(ctx context.Context, sheet string, bounds document.CellRange, keyColumn int, delimiter string) (document.SpreadsheetTextToColumnsResult, error) {
			if err := ctx.Err(); err != nil {
				return document.SpreadsheetTextToColumnsResult{}, err
			}
			editor, ok := d.(document.SpreadsheetTextToColumnsEditor)
			if !ok {
				return document.SpreadsheetTextToColumnsResult{}, ErrCapabilityUnsupported
			}
			return editor.SplitTextToColumns(sheet, bounds, keyColumn, delimiter)
		},
		removeSpreadsheetDuplicates: func(ctx context.Context, sheet string, bounds document.CellRange, keyColumn int) (document.SpreadsheetDuplicatesResult, error) {
			if err := ctx.Err(); err != nil {
				return document.SpreadsheetDuplicatesResult{}, err
			}
			editor, ok := d.(document.SpreadsheetDuplicatesEditor)
			if !ok {
				return document.SpreadsheetDuplicatesResult{}, ErrCapabilityUnsupported
			}
			return editor.RemoveDuplicates(sheet, bounds, keyColumn)
		},
		getSpreadsheetPanes: func(ctx context.Context, sheet string) (document.SpreadsheetPaneFreeze, error) {
			if err := ctx.Err(); err != nil {
				return document.SpreadsheetPaneFreeze{}, err
			}
			reader, ok := d.(document.SpreadsheetPanesEditor)
			if !ok {
				return document.SpreadsheetPaneFreeze{}, ErrCapabilityUnsupported
			}
			return reader.GetSheetPanes(sheet)
		},
		setSpreadsheetPanes: func(ctx context.Context, sheet string, panes document.SpreadsheetPaneFreeze) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetPanesEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			current, err := editor.GetSheetPanes(sheet)
			if err != nil {
				return false, err
			}
			if current == panes {
				return false, nil
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := editor.SetSheetPanes(sheet, panes); err != nil {
				return false, err
			}
			return true, nil
		},
		getSpreadsheetView: func(ctx context.Context, sheet string) (document.SpreadsheetSheetView, error) {
			if err := ctx.Err(); err != nil {
				return document.SpreadsheetSheetView{}, err
			}
			reader, ok := d.(document.SpreadsheetViewsEditor)
			if !ok {
				return document.SpreadsheetSheetView{}, ErrCapabilityUnsupported
			}
			return reader.GetSheetView(sheet)
		},
		setSpreadsheetViewMode: func(ctx context.Context, sheet, mode string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetViewsEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			current, err := editor.GetSheetView(sheet)
			if err != nil {
				return false, err
			}
			if current.ViewMode == mode || (current.ViewMode == "" && mode == "normal") {
				return false, nil
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := editor.SetSheetViewMode(sheet, mode); err != nil {
				return false, err
			}
			return true, nil
		},
		setSpreadsheetViewRuler: func(ctx context.Context, sheet string, show bool) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetViewsEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			current, err := editor.GetSheetView(sheet)
			if err != nil {
				return false, err
			}
			if current.ShowRuler == show {
				return false, nil
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := editor.SetSheetRuler(sheet, show); err != nil {
				return false, err
			}
			return true, nil
		},
		getSpreadsheetComments: func(ctx context.Context, sheet string) ([]document.SpreadsheetComment, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			reader, ok := d.(document.SpreadsheetCommentEditor)
			if !ok {
				return nil, ErrCapabilityUnsupported
			}
			return reader.GetComments(sheet)
		},
		addSpreadsheetComment: func(ctx context.Context, sheet string, comment document.SpreadsheetComment) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetCommentEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			comments, err := editor.GetComments(sheet)
			if err != nil {
				return false, err
			}
			for _, existing := range comments {
				if strings.EqualFold(existing.Cell, comment.Cell) && existing.Author == comment.Author && existing.Text == comment.Text {
					return false, nil
				}
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := editor.AddComment(sheet, comment); err != nil {
				return false, err
			}
			return true, nil
		},
		deleteSpreadsheetComment: func(ctx context.Context, sheet, cell string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetCommentEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			comments, err := editor.GetComments(sheet)
			if err != nil {
				return false, err
			}
			found := false
			for _, existing := range comments {
				if strings.EqualFold(existing.Cell, cell) {
					found = true
					break
				}
			}
			if !found {
				return false, nil
			}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := editor.DeleteComment(sheet, cell); err != nil {
				return false, err
			}
			return true, nil
		},
		flashFillSpreadsheet: func(ctx context.Context, sheet string, bounds document.CellRange) (document.SpreadsheetFlashFillResult, error) {
			if err := ctx.Err(); err != nil {
				return document.SpreadsheetFlashFillResult{}, err
			}
			editor, ok := d.(document.SpreadsheetFlashFillEditor)
			if !ok {
				return document.SpreadsheetFlashFillResult{}, ErrCapabilityUnsupported
			}
			return editor.FlashFill(sheet, bounds)
		},
		getSpreadsheetHyperlink: func(ctx context.Context, sheet, cell string) (document.CellHyperlink, bool, error) {
			if err := ctx.Err(); err != nil {
				return document.CellHyperlink{}, false, err
			}
			reader, ok := d.(document.SpreadsheetHyperlinkEditor)
			if !ok {
				return document.CellHyperlink{}, false, ErrCapabilityUnsupported
			}
			return reader.GetCellHyperlink(sheet, cell)
		},
		setSpreadsheetHyperlink: func(ctx context.Context, sheet, cell, target, tooltip string, remove bool) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetHyperlinkEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			current, exists, err := editor.GetCellHyperlink(sheet, cell)
			if err != nil {
				return false, err
			}
			if remove {
				if !exists {
					return false, nil
				}
				if err := editor.RemoveCellHyperlink(sheet, cell); err != nil {
					return false, err
				}
				return true, nil
			}
			if exists && current.Target == target && current.Tooltip == tooltip {
				return false, nil
			}
			if err := editor.SetCellHyperlink(sheet, cell, target, tooltip); err != nil {
				return false, err
			}
			return true, nil
		},
		setCellText: func(ctx context.Context, sheet, cell, value string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if reader, ok := d.(interface {
				Value(string, string) (string, error)
			}); ok {
				current, err := reader.Value(sheet, cell)
				if err != nil {
					return false, err
				}
				if current == value {
					return false, nil
				}
			}
			if err := d.SetCellText(sheet, cell, value); err != nil {
				return false, err
			}
			return true, nil
		},
		setCellValue: func(ctx context.Context, sheet, cell string, value document.CellValue) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			editor, ok := d.(document.SpreadsheetCellValueEditor)
			if !ok {
				return false, ErrCapabilityUnsupported
			}
			return editor.SetCellValueChanged(sheet, cell, value)
		},
		listSpreadsheetImages: func(ctx context.Context, sheet string) ([]document.SpreadsheetImage, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			reader, ok := d.(document.SpreadsheetImageReader)
			if !ok {
				return nil, ErrCapabilityUnsupported
			}
			return ListSpreadsheetImageMetadata(ctx, reader, sheet)
		},
		readSpreadsheetImageBytes: func(ctx context.Context, imageID string) ([]byte, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			reader, ok := d.(document.SpreadsheetImageReader)
			if !ok {
				return nil, ErrCapabilityUnsupported
			}
			return reader.SpreadsheetImageBytes(imageID)
		},
		insertSpreadsheetImage: func(ctx context.Context, placement document.SpreadsheetImagePlacement, data []byte) (string, error) {
			editor, ok := d.(document.SpreadsheetImageEditor)
			if !ok {
				return "", ErrCapabilityUnsupported
			}
			return InsertSpreadsheetImage(ctx, editor, placement, data)
		},
		replaceSpreadsheetImage: func(ctx context.Context, sheet, imageID, mediaType string, data []byte) (bool, error) {
			reader, readable := d.(document.SpreadsheetImageReader)
			editor, editable := d.(document.SpreadsheetImageEditor)
			if !readable || !editable {
				return false, ErrCapabilityUnsupported
			}
			return ReplaceSpreadsheetImage(ctx, reader, editor, sheet, imageID, mediaType, data)
		},
		moveSpreadsheetImage: func(ctx context.Context, sheet, imageID string, row, column, offsetX, offsetY int) (bool, error) {
			reader, readable := d.(document.SpreadsheetImageReader)
			editor, editable := d.(document.SpreadsheetImageEditor)
			if !readable || !editable {
				return false, ErrCapabilityUnsupported
			}
			return MoveSpreadsheetImage(ctx, reader, editor, sheet, imageID, row, column, offsetX, offsetY)
		},
		resizeSpreadsheetImage: func(ctx context.Context, sheet, imageID string, widthPx int) (bool, error) {
			reader, readable := d.(document.SpreadsheetImageReader)
			editor, editable := d.(document.SpreadsheetImageEditor)
			if !readable || !editable {
				return false, ErrCapabilityUnsupported
			}
			return ResizeSpreadsheetImage(ctx, reader, editor, sheet, imageID, widthPx)
		},
		deleteSpreadsheetImage: func(ctx context.Context, sheet, imageID string) (bool, error) {
			reader, readable := d.(document.SpreadsheetImageReader)
			editor, editable := d.(document.SpreadsheetImageEditor)
			if !readable || !editable {
				return false, ErrCapabilityUnsupported
			}
			return DeleteSpreadsheetImage(ctx, reader, editor, sheet, imageID)
		},
		inspect: func(ctx context.Context, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
			return InspectSpreadsheet(ctx, d, request)
		},
	}
}

func presentationAdapter(path string, d *pptx.Document) DocumentAdapter {
	return &adapterFuncs{
		format: FormatPPTX, path: path, dirty: d.Dirty, save: d.Save, saveAs: d.SaveAs, close: d.Close,
		presentationSnapshot: func(ctx context.Context) (document.PresentationSnapshot, error) {
			if err := ctx.Err(); err != nil {
				return document.PresentationSnapshot{}, err
			}
			snapshot := d.Snapshot()
			if snapshot.SlideCount == 0 || len(snapshot.Slides) != snapshot.SlideCount {
				return document.PresentationSnapshot{}, errors.New("演示文稿快照不完整")
			}
			return snapshot, nil
		},
		addPresentationSlide: func(ctx context.Context) (int, error) {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			if err := d.AddSlide(); err != nil {
				return 0, err
			}
			return d.Snapshot().SlideCount, nil
		},
		deletePresentationSlide: func(ctx context.Context, slideID uint32) (int, error) {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			if err := d.DeleteSlideByID(slideID); err != nil {
				return 0, err
			}
			return d.Snapshot().SlideCount, nil
		},
		duplicatePresentationSlide: func(ctx context.Context, slideID uint32) (uint32, int, error) {
			if err := ctx.Err(); err != nil {
				return 0, 0, err
			}
			return d.DuplicateSlideByID(slideID)
		},
		addPresentationTextBox: func(ctx context.Context, slideID uint32, bounds document.PresentationRect, text string) (int64, error) {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			return d.AddTextBoxByID(slideID, bounds, text)
		},
		addPresentationShape: func(ctx context.Context, slideID uint32, bounds document.PresentationRect, geometry, text string) (int64, error) {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			return d.AddAutoShapeByID(slideID, bounds, geometry, text)
		},
		addPresentationTable: func(ctx context.Context, slideID uint32, bounds document.PresentationRect, table document.PresentationTableData) (int64, error) {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			return d.AddTableByID(ctx, slideID, bounds, table)
		},
		addPresentationChart: func(ctx context.Context, slideID uint32, bounds document.PresentationRect, chart document.PresentationChartData) (int64, error) {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			return d.AddChartByID(ctx, slideID, bounds, chart)
		},
		addPresentationAudio: func(ctx context.Context, slideID uint32, bounds document.PresentationRect, audio document.PresentationAudioData) (int64, error) {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			return d.AddAudioByID(ctx, slideID, bounds, audio)
		},
		addPresentationVideo: func(ctx context.Context, slideID uint32, bounds document.PresentationRect, video document.PresentationVideoData) (int64, error) {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			return d.AddVideoByID(ctx, slideID, bounds, video)
		},
		addPresentationPicture: func(ctx context.Context, slideID uint32, bounds document.PresentationRect, mediaType string, data []byte, fit, altText string, decorative bool) (int64, document.PresentationRect, error) {
			if err := ctx.Err(); err != nil {
				return 0, document.PresentationRect{}, err
			}
			return d.AddSlidePictureByID(slideID, bounds, mediaType, data, fit, altText, decorative)
		},
		replacePresentationPicture: func(ctx context.Context, slideID uint32, elementID int64, mediaType string, data []byte) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			return d.ReplaceSlidePictureByID(slideID, elementID, mediaType, data)
		},
		setPresentationPictureCrop: func(ctx context.Context, slideID uint32, elementID int64, crop document.PresentationCrop) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			return d.UpdateSlidePictureCropByID(slideID, elementID, crop)
		},
		readPresentationElement: func(ctx context.Context, slideID uint32, elementID int64) (document.PresentationElement, error) {
			if err := ctx.Err(); err != nil {
				return document.PresentationElement{}, err
			}
			return d.PresentationElementByID(slideID, elementID)
		},
		setPresentationElementText: func(ctx context.Context, slideID uint32, elementID int64, value string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			before := d.Snapshot().Revision
			if err := d.UpdateSlideTextByID(slideID, elementID, value); err != nil {
				return false, err
			}
			return d.Snapshot().Revision != before, nil
		},
		setPresentationElementStyle: func(ctx context.Context, slideID uint32, elementID int64, patch document.PresentationElementStylePatch) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			before := d.Snapshot().Revision
			if err := d.UpdateSlideElementStyleByID(slideID, elementID, patch); err != nil {
				return false, err
			}
			return d.Snapshot().Revision != before, nil
		},
		setPresentationElementTransform: func(ctx context.Context, slideID uint32, elementID int64, bounds document.PresentationRect, rotationDeg float64) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			before := d.Snapshot().Revision
			if err := d.UpdateSlideElementTransformByID(slideID, elementID, bounds, rotationDeg); err != nil {
				return false, err
			}
			return d.Snapshot().Revision != before, nil
		},
		setPresentationSlideNotes: func(ctx context.Context, slideID uint32, expectedNotes, notes string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			changed, err := d.UpdateSlideNotesByIDIfCurrent(slideID, expectedNotes, notes)
			if errors.Is(err, pptx.ErrSpeakerNotesConflict) {
				return false, ErrPresentationNotesConflict
			}
			return changed, err
		},
		getPresentationTransition: func(ctx context.Context, slideID uint32) (document.PresentationTransition, error) {
			if err := ctx.Err(); err != nil {
				return document.PresentationTransition{}, err
			}
			return d.PresentationTransitionByID(slideID)
		},
		presentationAnimations: func(ctx context.Context, slideID uint32) (document.PresentationAnimationSummary, error) {
			if err := ctx.Err(); err != nil {
				return document.PresentationAnimationSummary{}, err
			}
			return d.PresentationAnimationsByID(ctx, slideID)
		},
		setPresentationElementAnimation: func(ctx context.Context, slideID uint32, elementID int64, effect string, durationMS int, expectedRevision uint64) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			return d.SetPresentationElementAnimationByID(ctx, slideID, elementID, effect, durationMS, expectedRevision)
		},
		setPresentationAnimationDuration: func(ctx context.Context, slideID uint32, elementID int64, animationIndex, durationMS int, expectedRevision uint64) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			return d.SetPresentationAnimationDurationByID(ctx, slideID, elementID, animationIndex, durationMS, expectedRevision)
		},
		deletePresentationAnimation: func(ctx context.Context, slideID uint32, elementID int64, animationIndex int, expectedRevision uint64) (int, error) {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			return d.DeletePresentationAnimationByID(ctx, slideID, elementID, animationIndex, expectedRevision)
		},
		presentationLayouts: func(ctx context.Context) (document.PresentationLayoutSummary, error) {
			if err := ctx.Err(); err != nil {
				return document.PresentationLayoutSummary{}, err
			}
			return d.PresentationLayouts(ctx)
		},
		setPresentationSlideLayout: func(ctx context.Context, slideID uint32, layoutIndex int, expectedRevision uint64) (document.PresentationLayoutOption, bool, error) {
			if err := ctx.Err(); err != nil {
				return document.PresentationLayoutOption{}, false, err
			}
			return d.SetSlideLayoutByID(ctx, slideID, layoutIndex, expectedRevision)
		},
		setPresentationTransition: func(ctx context.Context, slideID uint32, patch document.PresentationTransitionPatch, applyToAll bool) (document.PresentationTransition, []uint32, bool, error) {
			if err := ctx.Err(); err != nil {
				return document.PresentationTransition{}, nil, false, err
			}
			return d.SetPresentationTransitionByID(slideID, patch, applyToAll)
		},
		setPresentationSlideBackground: func(ctx context.Context, slideID uint32, colorHex string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			return d.SetSlideBackgroundByID(slideID, colorHex)
		},
		setPresentationSlideSize: func(ctx context.Context, widthPT, heightPT float64) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			return d.SetSlideSize(widthPT, heightPT)
		},
		setPresentationTheme: func(ctx context.Context, themeID string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			return d.SetThemePreset(ctx, themeID)
		},
		setPresentationThemePalette: func(ctx context.Context, paletteID string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			return d.SetThemePalette(ctx, paletteID)
		},
		groupPresentationElements: func(ctx context.Context, slideID uint32, elementIDs []uint32) (uint32, bool, error) {
			if err := ctx.Err(); err != nil {
				return 0, false, err
			}
			return d.GroupElementsByID(ctx, slideID, elementIDs)
		},
		listPresentationComments: func(ctx context.Context, slideID uint32, start, limit int) ([]document.PresentationComment, int, error) {
			if err := ctx.Err(); err != nil {
				return nil, 0, err
			}
			return d.ListPresentationComments(slideID, start, limit)
		},
		addPresentationComment: func(ctx context.Context, slideID uint32, xPT, yPT float64, author, text string) (string, bool, error) {
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
			return d.AddPresentationComment(slideID, xPT, yPT, author, text)
		},
		deletePresentationComment: func(ctx context.Context, slideID uint32, commentID string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			return d.DeletePresentationComment(slideID, commentID)
		},
		inspect: func(ctx context.Context, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
			return InspectPresentation(ctx, d, request)
		},
	}
}

func pdfAdapter(path string, d *pdfdoc.Document) DocumentAdapter {
	return &adapterFuncs{
		format: FormatPDF, path: path, dirty: d.Dirty, snapshot: d.Snapshot, save: d.Save, saveAs: d.SaveAs, close: d.Close,
		inspect: func(ctx context.Context, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
			return InspectPDF(ctx, d, request)
		},
		applyPDFCommand: func(ctx context.Context, command document.PDFCommand) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			before := d.Info().Revision
			var err error
			switch command.Kind {
			case "undo":
				d.Undo()
			case "redo":
				d.Redo()
			default:
				err = d.Apply(command)
			}
			if err != nil {
				return false, err
			}
			return d.Info().Revision != before, nil
		},
	}
}

func jsonAdapter(path string, d *jsondoc.Document) DocumentAdapter {
	return &jsonTextAdapter{adapterFuncs: &adapterFuncs{
		format: FormatJSON, path: path, dirty: d.Dirty, save: d.Save, saveAs: d.SaveAs, close: d.Close,
		inspect: func(ctx context.Context, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
			return InspectJSON(ctx, d, request)
		},
	}, document: d}
}

// jsonTextAdapter exposes the JSON document's source text through a narrow
// application capability. Format-specific implementation stays behind the
// application adapter boundary.
type jsonTextAdapter struct {
	*adapterFuncs
	document *jsondoc.Document
}

func (a *jsonTextAdapter) ReadJSONText(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if a == nil || a.document == nil {
		return "", ErrCapabilityUnsupported
	}
	return a.document.Text(), nil
}

func (a *jsonTextAdapter) SetJSONText(ctx context.Context, value string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a == nil || a.document == nil {
		return false, ErrCapabilityUnsupported
	}
	if a.document.Text() == value {
		return false, nil
	}
	if err := a.document.SetText(value); err != nil {
		return false, err
	}
	return true, nil
}

func markdownAdapter(path string, d *markdowndoc.Document) DocumentAdapter {
	return &markdownTextAdapter{adapterFuncs: &adapterFuncs{
		format: FormatMarkdown, path: path, dirty: d.Dirty, save: d.Save, saveAs: d.SaveAs, close: d.Close,
		inspect: func(ctx context.Context, request InspectRequest) (json.RawMessage, []Diagnostic, error) {
			return InspectMarkdown(ctx, d, request)
		},
	}, document: d}
}

type markdownTextAdapter struct {
	*adapterFuncs
	document *markdowndoc.Document
}

func (a *markdownTextAdapter) Snapshot(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a == nil || a.document == nil {
		return nil, ErrCapabilityUnsupported
	}
	return a.document.Snapshot()
}

func (a *markdownTextAdapter) ReadMarkdownText(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if a == nil || a.document == nil {
		return "", ErrCapabilityUnsupported
	}
	return a.document.Text(), nil
}

func (a *markdownTextAdapter) SetMarkdownText(ctx context.Context, value string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if a == nil || a.document == nil {
		return false, ErrCapabilityUnsupported
	}
	if a.document.Text() == value {
		return false, nil
	}
	if err := a.document.SetText(value); err != nil {
		return false, err
	}
	return true, nil
}
