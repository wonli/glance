package desktopui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"glance/internal/application"
	"glance/internal/fileutil"
	"glance/internal/fontmanager/fontcatalog"
	"glance/internal/format/pdfdoc"
	"glance/internal/i18n"
	"glance/internal/proofhistory"
	"glance/internal/proofreading"
	"glance/internal/workspace"
)

var (
	errProofreadingExportExtensionMismatch = errors.New("export destination extension must match the original B format")
	errProofreadingExportProtectedPath     = errors.New("export destination is a protected proofreading source or project file")
	errProofreadingLegacyPageMapping       = errors.New("original page mapping cannot be verified")
)

func proofreadingExportExtension(path string) (string, error) {
	extension := filepath.Ext(path)
	switch strings.ToLower(extension) {
	case ".md", ".markdown", ".txt":
		return extension, nil
	case ".doc", ".docx":
		return "", fmt.Errorf("proofreading export does not support %s", extension)
	default:
		return "", fmt.Errorf("unsupported proofreading source format %s", extension)
	}
}

func resolvedProofreadingPath(path string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	for current := absolute; ; current = filepath.Dir(current) {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			relative, relErr := filepath.Rel(current, absolute)
			if relErr != nil {
				return "", relErr
			}
			return filepath.Clean(filepath.Join(resolved, relative)), nil
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return "", resolveErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return absolute, nil
		}
	}
}

func sameProofreadingPath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	resolvedLeft, leftErr := resolvedProofreadingPath(left)
	resolvedRight, rightErr := resolvedProofreadingPath(right)
	if leftErr == nil && rightErr == nil {
		if resolvedLeft == resolvedRight || ((runtime.GOOS == "windows" || runtime.GOOS == "darwin") && strings.EqualFold(resolvedLeft, resolvedRight)) {
			return true
		}
	}
	leftInfo, leftStatErr := os.Stat(left)
	rightInfo, rightStatErr := os.Stat(right)
	return leftStatErr == nil && rightStatErr == nil && os.SameFile(leftInfo, rightInfo)
}

func proofreadingPathWithin(path, directory string) bool {
	if path == "" || directory == "" {
		return false
	}
	resolvedPath, pathErr := resolvedProofreadingPath(path)
	resolvedDirectory, directoryErr := resolvedProofreadingPath(directory)
	if pathErr != nil || directoryErr != nil {
		return false
	}
	relative, err := filepath.Rel(resolvedDirectory, resolvedPath)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return true
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		prefix := strings.TrimRight(resolvedDirectory, string(filepath.Separator)) + string(filepath.Separator)
		return strings.EqualFold(resolvedPath, resolvedDirectory) || strings.HasPrefix(strings.ToLower(resolvedPath), strings.ToLower(prefix))
	}
	return false
}

func validateProofreadingExportTarget(path, extension string, project workspace.ProofreadingProject, store *workspace.Store) error {
	if path == "" {
		return errors.New("export path is empty")
	}
	protected := []string{project.SourceA, project.SourceB, project.SourceAOriginalPath, project.SourceBOriginalPath}
	draftPath, err := proofreadingDraftFilePath(project.SourceB, project.ID)
	if err != nil {
		return fmt.Errorf("resolve proofreading recovery copy path: %w", err)
	}
	protected = append(protected, draftPath, draftPath+".previous")
	if storePath := store.Path(); storePath != "" {
		protected = append(protected, storePath, storePath+"-wal", storePath+"-shm")
	}
	for _, candidate := range protected {
		if sameProofreadingPath(path, candidate) {
			return errProofreadingExportProtectedPath
		}
	}
	if proofreadingPathWithin(path, project.Directory) {
		return errProofreadingExportProtectedPath
	}
	if !strings.EqualFold(filepath.Ext(path), extension) {
		return errProofreadingExportExtensionMismatch
	}
	return nil
}

func hashProofreadingFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func loadProofreadingExportSourceB(path string) (string, error) {
	object, err := loadProofreadingObject(path)
	if err != nil {
		return "", err
	}
	defer object.Adapter.Close()
	text, _ := proofreadingProjectionContent(object.contentUnits())
	return text, nil
}

func decodeProofreadingExportPayload(project workspace.ProofreadingProject) (proofreadingProjectPayload, error) {
	var payload proofreadingProjectPayload
	if len(project.Payload) == 0 {
		return payload, errors.New("proofreading project payload is unavailable")
	}
	if err := json.Unmarshal(project.Payload, &payload); err != nil {
		return payload, fmt.Errorf("read proofreading project: %w", err)
	}
	if payload.Draft.Model == nil {
		return payload, errors.New("proofreading project model is unavailable")
	}
	return payload, nil
}

func verifyProofreadingExportSources(project workspace.ProofreadingProject, payload proofreadingProjectPayload, originalText string) error {
	if project.SourceAHash == "" || payload.Draft.SourceAHash == "" || project.SourceAHash != payload.Draft.SourceAHash {
		return errors.New("reference PDF hash is missing or inconsistent")
	}
	aHash, err := hashProofreadingFile(project.SourceA)
	if err != nil {
		return fmt.Errorf("read retained reference PDF: %w", err)
	}
	if aHash != project.SourceAHash {
		return errors.New("retained reference PDF no longer matches this proofreading task")
	}
	if payload.Draft.SourceBHash == "" {
		return errors.New("original B text hash is unavailable")
	}
	bDigest := sha256.Sum256([]byte(originalText))
	if hex.EncodeToString(bDigest[:]) != payload.Draft.SourceBHash {
		return errors.New("retained original B text no longer matches this proofreading task")
	}
	return nil
}

func reconstructProofreadingOriginalPageEnds(ctx context.Context, project workspace.ProofreadingProject, themeFace string, fonts fontcatalog.Snapshot) ([]int, error) {
	_, err := proofreadingExportExtension(project.SourceB)
	if err != nil {
		return nil, err
	}
	payload, err := decodeProofreadingExportPayload(project)
	if err != nil {
		return nil, err
	}
	originalText, err := loadProofreadingExportSourceB(project.SourceB)
	if err != nil {
		return nil, fmt.Errorf("read retained original B text: %w", err)
	}
	if err = verifyProofreadingExportSources(project, payload, originalText); err != nil {
		return nil, err
	}
	doc, err := pdfdoc.Open(project.SourceA)
	if err != nil {
		return nil, fmt.Errorf("reconstruct original page mapping: open reference PDF: %w", err)
	}
	defer doc.Close()
	pageCount := doc.Info().Pages
	if pageCount <= 0 || len(payload.Draft.Model.Pages) != pageCount {
		return nil, fmt.Errorf("reconstruct original page mapping: page count differs from saved proofreading model")
	}
	ids := make([]proofreading.PageID, pageCount)
	geometry := make([]application.ProofreadingPageGeometry, pageCount)
	for i := 0; i < pageCount; i++ {
		ids[i] = proofreading.PageID(i + 1)
		width, height, frameErr := doc.PageDimensions(i)
		if frameErr != nil {
			return nil, fmt.Errorf("reconstruct original page mapping: read PDF page %d: %w", i+1, frameErr)
		}
		marginX, marginY := proofreadingTextMargins(width, height)
		geometry[i] = application.ProofreadingPageGeometry{
			Page: ids[i], Width: max(1, width-2*marginX), Height: max(1, height-2*marginY),
		}
	}
	for i, page := range payload.Draft.Model.Pages {
		if page.ID != ids[i] {
			return nil, fmt.Errorf("reconstruct original page mapping: saved PDF page identities are inconsistent")
		}
	}
	model, err := proofreading.New(originalText, ids)
	if err != nil {
		return nil, fmt.Errorf("reconstruct original page mapping: create source model: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	layout, err := reflowProofreadingForEditor(ctx, model.Snapshot(), geometry, application.ProofreadingTextStyle{Size: 12, Family: themeFace}, true, fonts)
	if err != nil {
		return nil, fmt.Errorf("reconstruct original page mapping: layout original B text: %w", err)
	}
	ends := make([]int, pageCount)
	for i, id := range ids {
		end, ok := layout.Ends[id]
		if !ok {
			return nil, fmt.Errorf("reconstruct original page mapping: page %d boundary is unavailable", id)
		}
		ends[i] = end
	}
	if _, err = proofreadingOriginalPagesFromEnds(originalText, ends); err != nil {
		return nil, fmt.Errorf("reconstruct original page mapping: %w", err)
	}
	return ends, nil
}

func inferLegacyProofreadingPageEnds(snapshot proofreading.Snapshot, originalText string, reconstructedEnds []int) ([]int, error) {
	if len(snapshot.Pages) == 0 || len(reconstructedEnds) != len(snapshot.Pages) {
		return nil, errors.New("reconstructed page boundaries do not match the saved model")
	}
	if _, err := proofreadingOriginalPagesFromEnds(originalText, reconstructedEnds); err != nil {
		return nil, err
	}
	originalRunes := uint64(utf8.RuneCountInString(originalText))
	if originalRunes == 0 {
		return append([]int(nil), reconstructedEnds...), nil
	}
	pageIndexes := make(map[proofreading.PageID]int, len(snapshot.Pages))
	for i, page := range snapshot.Pages {
		if _, exists := pageIndexes[page.ID]; exists {
			return nil, errors.New("saved proofreading model contains duplicate page identities")
		}
		pageIndexes[page.ID] = i
	}
	type sourceRange struct {
		first uint64
		end   uint64
		page  int
	}
	ranges := make([]sourceRange, 0, len(snapshot.Runs))
	minByPage := make([]uint64, len(snapshot.Pages))
	maxByPage := make([]uint64, len(snapshot.Pages))
	for i := range minByPage {
		minByPage[i] = originalRunes + 1
	}
	for _, run := range snapshot.Runs {
		runLength := utf8.RuneCountInString(run.Text)
		first := uint64(run.First)
		if first > originalRunes || runLength == 0 {
			continue
		}
		lastExclusive := first + uint64(runLength)
		if lastExclusive <= first {
			return nil, errors.New("saved source character run has an invalid identity range")
		}
		lastExclusive = min(lastExclusive, originalRunes+1)
		owner := run.Page
		if run.Origin != 0 {
			owner = run.Origin
		}
		page, ok := pageIndexes[owner]
		if !ok {
			return nil, fmt.Errorf("saved source character belongs to unknown page %d", owner)
		}
		ranges = append(ranges, sourceRange{first: first, end: lastExclusive, page: page})
		minByPage[page] = min(minByPage[page], first)
		maxByPage[page] = max(maxByPage[page], lastExclusive-1)
	}
	if len(ranges) == 0 {
		return nil, errors.New("saved proofreading model no longer contains original source character identities")
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].first < ranges[j].first })
	for i := 1; i < len(ranges); i++ {
		if ranges[i].first < ranges[i-1].end {
			return nil, errors.New("saved source character identities overlap")
		}
		if ranges[i].page < ranges[i-1].page {
			return nil, errors.New("saved source character ownership is not in page order")
		}
	}
	prefixMax := make([]uint64, len(snapshot.Pages))
	for i := range prefixMax {
		prefixMax[i] = maxByPage[i]
		if i > 0 {
			prefixMax[i] = max(prefixMax[i], prefixMax[i-1])
		}
	}
	suffixMin := make([]uint64, len(snapshot.Pages)+1)
	suffixMin[len(snapshot.Pages)] = originalRunes + 1
	for i := len(snapshot.Pages) - 1; i >= 0; i-- {
		suffixMin[i] = min(minByPage[i], suffixMin[i+1])
	}
	ends := make([]int, len(snapshot.Pages))
	var previous uint64
	for i := 0; i < len(snapshot.Pages)-1; i++ {
		lower := prefixMax[i]
		upper := originalRunes
		if suffixMin[i+1] <= originalRunes {
			upper = suffixMin[i+1] - 1
		}
		if lower > upper {
			return nil, fmt.Errorf("saved source characters cannot be assigned to page boundary %d", i+1)
		}
		boundary := uint64(reconstructedEnds[i])
		boundary = max(lower, min(boundary, upper))
		boundary = max(boundary, previous)
		if boundary > upper {
			return nil, fmt.Errorf("saved source characters conflict with page boundary %d", i+1)
		}
		ends[i] = int(boundary)
		previous = boundary
	}
	ends[len(ends)-1] = int(originalRunes)
	if _, err := proofreadingOriginalPagesFromEnds(originalText, ends); err != nil {
		return nil, err
	}
	return ends, nil
}

type proofreadingExportLiveSnapshot struct {
	view         proofreading.View
	sourceAHash  string
	sourceBHash  string
	originalEnds []int
	confirmed    map[int]string
}

func (a *App) proofreadingExportSnapshot(projectID string) (*proofreadingExportLiveSnapshot, bool, bool) {
	if a == nil {
		return nil, false, false
	}
	runtime := a.desktopRuntime
	var host *App
	var id string
	var closing bool
	ownerFound := false
	if runtime != nil {
		runtime.mu.Lock()
		owner := runtime.proofTabOwners[projectID]
		if owner != nil {
			ownerFound = true
			host, id, closing = owner.host, owner.id, owner.closing
		}
		runtime.mu.Unlock()
	}
	if ownerFound {
		if host == nil || host != a || closing {
			return nil, true, false
		}
		tab := a.proofreadingTab(id)
		if tab == nil || tab.closing || tab.editor == nil || tab.editor.proofreading == nil || tab.editor.proofreading.projectDocumentID != projectID {
			return nil, true, false
		}
		snapshot, ready := captureProofreadingExportState(tab.editor.proofreading)
		return snapshot, true, ready
	}
	if state := a.proofreading; state != nil && state.projectDocumentID == projectID {
		snapshot, ready := captureProofreadingExportState(state)
		return snapshot, true, ready
	}
	return nil, false, true
}

func captureProofreadingExportState(state *proofreadingWorkspace) (*proofreadingExportLiveSnapshot, bool) {
	if state == nil || state.controller == nil || state.sourceLoading || state.draftLoading || state.draftRestoring || state.draftRestoreFailed ||
		state.modelLayoutPending || state.history.pending || state.history.err != "" || state.draftSavePending || !state.draftAutoSaved || state.draftPersistError != "" {
		return nil, false
	}
	return &proofreadingExportLiveSnapshot{
		view: state.controller.Model.Capture(), sourceAHash: state.aSourceHash, sourceBHash: state.sourceBHash,
		originalEnds: append([]int(nil), state.originalPageEnds...), confirmed: cloneProofreadingHashes(state.history.confirmed),
	}, true
}

func buildProofreadingProjectExport(ctx context.Context, store *workspace.Store, project workspace.ProofreadingProject, themeFace string, fonts fontcatalog.Snapshot) (string, string, error) {
	return buildProofreadingProjectExportWithSnapshot(ctx, store, project, themeFace, fonts, nil)
}

func buildProofreadingProjectExportWithSnapshot(ctx context.Context, store *workspace.Store, project workspace.ProofreadingProject, themeFace string, fonts fontcatalog.Snapshot, live *proofreadingExportLiveSnapshot) (string, string, error) {
	extension, err := proofreadingExportExtension(project.SourceB)
	if err != nil {
		return "", "", err
	}
	if store == nil {
		return "", "", errors.New("proofreading workspace storage is unavailable")
	}
	stored, found, err := store.LoadProofreadingProject(project.ID)
	if err != nil {
		return "", "", err
	}
	if !found {
		return "", "", errors.New("proofreading project no longer exists")
	}
	extension, err = proofreadingExportExtension(stored.SourceB)
	if err != nil {
		return "", "", err
	}
	payload, err := decodeProofreadingExportPayload(stored)
	if err != nil {
		return "", "", err
	}
	if live != nil {
		if live.sourceAHash == "" || live.sourceAHash != payload.Draft.SourceAHash || live.sourceBHash == "" || live.sourceBHash != payload.Draft.SourceBHash {
			return "", "", errors.New("live proofreading source no longer matches the saved task")
		}
		snapshot := live.view.Snapshot()
		payload.Draft.Model = &snapshot
		var liveText strings.Builder
		for _, run := range snapshot.Runs {
			liveText.WriteString(run.Text)
		}
		payload.Draft.Text = liveText.String()
		payload.Draft.OriginalPageEnds = append([]int(nil), live.originalEnds...)
		payload.Confirmed = cloneProofreadingHashes(live.confirmed)
		payload.ConfirmedCurrent = nil
	}
	originalText, err := loadProofreadingExportSourceB(stored.SourceB)
	if err != nil {
		return "", "", fmt.Errorf("read retained original B text: %w", err)
	}
	if err = verifyProofreadingExportSources(stored, payload, originalText); err != nil {
		return "", "", err
	}
	model, err := proofreading.Restore(*payload.Draft.Model)
	if err != nil {
		return "", "", fmt.Errorf("restore proofreading model: %w", err)
	}
	doc, err := pdfdoc.Open(stored.SourceA)
	if err != nil {
		return "", "", fmt.Errorf("open retained reference PDF: %w", err)
	}
	pageCount := doc.Info().Pages
	closeErr := doc.Close()
	if closeErr != nil {
		return "", "", closeErr
	}
	snapshot := model.Snapshot()
	if pageCount <= 0 || len(snapshot.Pages) != pageCount || (stored.PageCount > 0 && stored.PageCount != pageCount) {
		return "", "", errors.New("proofreading page count differs from retained reference PDF")
	}
	for i, page := range snapshot.Pages {
		if page.ID != proofreading.PageID(i+1) {
			return "", "", errors.New("proofreading page identities differ from retained reference PDF")
		}
	}
	currentTexts := make(map[proofreading.PageID]*strings.Builder, pageCount)
	for _, page := range snapshot.Pages {
		currentTexts[page.ID] = &strings.Builder{}
	}
	for _, run := range snapshot.Runs {
		if text := currentTexts[run.Page]; text != nil {
			text.WriteString(run.Text)
		}
	}
	confirmedCurrent := proofreadingConfirmedCurrent(payload, stored.ID)
	for index, page := range snapshot.Pages {
		if page.Reviewed && !confirmedCurrent[index] {
			return "", "", application.ProofreadingExportUnconfirmedPageError{Page: page.ID}
		}
	}
	allPagesConfirmedCurrent := len(confirmedCurrent) == pageCount
	if allPagesConfirmedCurrent {
		for page := 0; page < pageCount; page++ {
			if !confirmedCurrent[page] {
				allPagesConfirmedCurrent = false
				break
			}
		}
	}
	var originalPages []string
	if len(payload.Draft.OriginalPageEnds) == 0 && allPagesConfirmedCurrent {
		originalPages = make([]string, pageCount)
	} else {
		ends := append([]int(nil), payload.Draft.OriginalPageEnds...)
		if len(ends) == 0 {
			ends, err = reconstructProofreadingOriginalPageEnds(ctx, stored, themeFace, fonts)
			if err != nil {
				return "", "", fmt.Errorf("%w: %w", errProofreadingLegacyPageMapping, err)
			}
			ends, err = inferLegacyProofreadingPageEnds(snapshot, originalText, ends)
			if err != nil {
				return "", "", fmt.Errorf("%w: %w", errProofreadingLegacyPageMapping, err)
			}
		}
		originalPages, err = proofreadingOriginalPagesFromEnds(originalText, ends)
		if err != nil {
			return "", "", err
		}
		if len(originalPages) != pageCount {
			return "", "", errors.New("original page mapping differs from retained reference PDF")
		}
	}
	gitSnapshots := loadProofreadingExportGitSnapshots(ctx, stored, payload)
	exportPages := make([]application.ProofreadingExportPage, 0, pageCount)
	for i, page := range snapshot.Pages {
		index := i
		confirmedHash := payload.Confirmed[index]
		currentText := currentTexts[page.ID].String()
		if snapshotPage, ok := gitSnapshots[index]; ok && confirmedCurrent[index] && confirmedHash != "" {
			currentText = snapshotPage.Text()
		}
		exportPages = append(exportPages, application.ProofreadingExportPage{
			ID: page.ID, Reviewed: page.Reviewed || confirmedHash != "", CurrentConfirmed: confirmedCurrent[index],
			CurrentText: currentText, OriginalText: originalPages[i],
		})
	}
	text, err := application.AssembleProofreadingExport(exportPages)
	if err != nil {
		return "", "", err
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text, extension, nil
}

func loadProofreadingExportGitSnapshots(ctx context.Context, project workspace.ProofreadingProject, payload proofreadingProjectPayload) map[int]proofhistory.PageResult {
	result := map[int]proofhistory.PageResult{}
	if len(payload.Confirmed) == 0 {
		return result
	}
	repository := proofhistory.Repository{Directory: project.Directory}
	if repository.Available(ctx) != nil {
		return result
	}
	if _, err := os.Stat(filepath.Join(project.Directory, ".git")); err != nil {
		return result
	}
	for page, contentHash := range payload.Confirmed {
		if contentHash == "" {
			continue
		}
		snapshot, err := repository.ReadPage(ctx, "HEAD", proofreadingPageID(page))
		if err != nil || snapshot.ProjectID != project.ID || snapshot.SourceHash != payload.Draft.SourceAHash || snapshot.ContentHash() != contentHash {
			continue
		}
		result[page] = snapshot
	}
	return result
}

func writeProofreadingProjectExport(path, text string) error {
	if path == "" {
		return errors.New("export path is empty")
	}
	return fileutil.AtomicWriteString(path, text, fileutil.WithTempDir(filepath.Dir(path)))
}

func (a *App) startProofreadingProjectExport(project workspace.ProofreadingProject) {
	w := a.ensureProofreadingWorkbench()
	if w.busy {
		return
	}
	extension, err := proofreadingExportExtension(project.SourceB)
	if err != nil {
		a.showToastMessage(i18n.Message{ID: "proofread.export.unsupported", Args: i18n.Args{"Extension": filepath.Ext(project.SourceB)}}, feedbackWarning, "")
		return
	}
	live, active, ready := a.proofreadingExportSnapshot(project.ID)
	if active && !ready {
		a.showToastMessage(i18n.Message{ID: "proofread.export.wait"}, feedbackWarning, "")
		return
	}
	labels := a.filePickerLabels()
	labels.SaveAs = a.textID("proofread.export.save_as")
	labels.SaveFilter = a.textID("proofread.export.file_filter")
	defaultSourceName := project.SourceBName
	if defaultSourceName == "" {
		defaultSourceName = filepath.Base(project.SourceB)
	}
	labels.UntitledFile = strings.TrimSuffix(filepath.Base(defaultSourceName), filepath.Ext(defaultSourceName))
	if labels.UntitledFile == "" || labels.UntitledFile == "." {
		labels.UntitledFile = a.textID("filepicker.untitled_file")
	}
	picker := w.exportPathPicker
	store := a.proofreadingStore()
	themeFace := ""
	if a.theme != nil {
		themeFace = string(a.theme.Face)
	}
	fonts := a.currentFontSnapshot()
	a.showProgressToastMessage(i18n.Message{ID: "proofread.export.progress"})
	a.proofreadingWorkbenchJob(func() func() {
		var path string
		var pickErr error
		if picker != nil {
			path, pickErr = picker(strings.TrimPrefix(extension, "."), labels)
		} else {
			path, pickErr = chooseSavePath(strings.TrimPrefix(extension, "."), labels)
		}
		if pickErr != nil {
			return func() {
				a.showToastMessage(i18n.Message{ID: "proofread.export.failed_short"}, feedbackError, pickErr.Error())
			}
		}
		if path == "" {
			return func() {
				if a.feedback.messageRef.ID == "proofread.export.progress" {
					a.feedback.dismiss()
				}
			}
		}
		text, outputExtension, buildErr := buildProofreadingProjectExportWithSnapshot(context.Background(), store, project, themeFace, fonts, live)
		if buildErr != nil {
			var pageErr application.ProofreadingExportUnconfirmedPageError
			if errors.As(buildErr, &pageErr) {
				return func() {
					a.showToastMessage(i18n.Message{ID: "proofread.export.confirm_page", Args: i18n.Args{"Page": int(pageErr.Page)}}, feedbackWarning, buildErr.Error())
				}
			}
			if errors.Is(buildErr, errProofreadingLegacyPageMapping) {
				return func() {
					a.showToastMessage(i18n.Message{ID: "proofread.export.mapping_unverified"}, feedbackError, buildErr.Error())
				}
			}
			return func() {
				a.showToastMessage(i18n.Message{ID: "proofread.export.failed_short"}, feedbackError, buildErr.Error())
			}
		}
		if err = validateProofreadingExportTarget(path, outputExtension, project, store); err != nil {
			messageID := "proofread.export.failed_short"
			if errors.Is(err, errProofreadingExportExtensionMismatch) {
				messageID = "proofread.export.extension_mismatch"
			} else if errors.Is(err, errProofreadingExportProtectedPath) {
				messageID = "proofread.export.protected_path"
			}
			return func() { a.showToastMessage(i18n.Message{ID: messageID}, feedbackError, err.Error()) }
		}
		if writeErr := writeProofreadingProjectExport(path, text); writeErr != nil {
			return func() {
				a.showToastMessage(i18n.Message{ID: "proofread.export.failed_short"}, feedbackError, writeErr.Error())
			}
		}
		return func() {
			a.showToastMessage(i18n.Message{ID: "proofread.export.success", Args: i18n.Args{"Path": path}}, feedbackSuccess, "")
		}
	})
	if !w.busy && a.feedback.messageRef.ID == "proofread.export.progress" {
		a.feedback.dismiss()
	}
}
