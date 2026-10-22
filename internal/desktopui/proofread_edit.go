package desktopui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"glance/internal/proofreading"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const proofreadingDraftFileHeader = "<!-- glance-proofreading-draft-v1\n"

type proofreadingPersistedDraft struct {
	Model                 *proofreading.Snapshot      `json:"model,omitempty"`
	Version               int                         `json:"version"`
	SourceAHash           string                      `json:"source_a_hash"`
	SourceBHash           string                      `json:"source_b_hash"`
	Text                  string                      `json:"text"`
	Pages                 []proofreadingPageOwnership `json:"pages,omitempty"`
	ManualBreaks          []proofreadingManualBreak   `json:"manual_breaks"`
	AlignmentOffset       int                         `json:"alignment_offset"`
	AlignmentScrollOffset float64                     `json:"alignment_scroll_offset,omitempty"`
	TextSizeStep          int                         `json:"text_size_step"`
}

type proofreadingDraftFileMetadata struct {
	Model                 *proofreading.Snapshot      `json:"model,omitempty"`
	Version               int                         `json:"version"`
	SourceAHash           string                      `json:"source_a_hash"`
	SourceBHash           string                      `json:"source_b_hash"`
	Pages                 []proofreadingPageOwnership `json:"pages,omitempty"`
	ManualBreaks          []proofreadingManualBreak   `json:"manual_breaks"`
	AlignmentOffset       int                         `json:"alignment_offset"`
	AlignmentScrollOffset float64                     `json:"alignment_scroll_offset,omitempty"`
	TextSizeStep          int                         `json:"text_size_step"`
}

func proofreadingProjectDocumentID(aPath, sourceAHash, bPath, sourceText string) (string, string) {
	canonicalPath := func(path string) string {
		if path == "" {
			return ""
		}
		absolute, err := filepath.Abs(path)
		if err == nil {
			return filepath.Clean(absolute)
		}
		return filepath.Clean(path)
	}
	sourceHash := sha256.Sum256([]byte(sourceText))
	sourceDigest := hex.EncodeToString(sourceHash[:])
	projectHash := sha256.Sum256([]byte(canonicalPath(aPath) + "\x00" + sourceAHash + "\x00" + canonicalPath(bPath) + "\x00" + sourceDigest))
	return "proofread-" + hex.EncodeToString(projectHash[:]), sourceDigest
}

func proofreadingLegacyWorkspaceDraftID(aPath, bPath, sourceBHash string) string {
	canonicalPath := func(path string) string {
		if path == "" {
			return ""
		}
		absolute, err := filepath.Abs(path)
		if err == nil {
			return filepath.Clean(absolute)
		}
		return filepath.Clean(path)
	}
	projectHash := sha256.Sum256([]byte(canonicalPath(aPath) + "\x00" + canonicalPath(bPath) + "\x00" + sourceBHash))
	return "proofread-" + hex.EncodeToString(projectHash[:])
}

func proofreadingDraftFilePath(bPath, projectID string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve proofreading directory: %w", err)
	}
	name := strings.TrimSuffix(filepath.Base(bPath), filepath.Ext(bPath))
	if name == "" || name == "." {
		name = "proofreading-copy"
	}
	var safeName strings.Builder
	lastDash := false
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			safeName.WriteRune(r)
			lastDash = false
		} else if safeName.Len() > 0 && !lastDash {
			safeName.WriteByte('-')
			lastDash = true
		}
	}
	segment := strings.Trim(safeName.String(), "-_.")
	if segment == "" {
		segment = "proofreading-copy"
	}
	if runes := []rune(segment); len(runes) > 48 {
		segment = string(runes[:48])
	}
	suffix := strings.TrimPrefix(projectID, "proofread-")
	if len(suffix) > 12 {
		suffix = suffix[:12]
	}
	if suffix == "" {
		fallback := sha256.Sum256([]byte(projectID))
		suffix = hex.EncodeToString(fallback[:6])
	}
	return filepath.Join(configDir, "Glance Office", "Proofreading", segment+"-"+suffix, "proofreading-copy.md"), nil
}

func encodeProofreadingDraftFile(draft proofreadingPersistedDraft) ([]byte, error) {
	metadata := proofreadingDraftFileMetadata{
		Model: draft.Model, Version: draft.Version, SourceAHash: draft.SourceAHash, SourceBHash: draft.SourceBHash,
		Pages: draft.Pages, ManualBreaks: draft.ManualBreaks, AlignmentOffset: draft.AlignmentOffset,
		AlignmentScrollOffset: draft.AlignmentScrollOffset, TextSizeStep: draft.TextSizeStep,
	}
	if draft.Model != nil {
		if _, err := proofreading.Restore(*draft.Model); err != nil {
			return nil, err
		}
		var body strings.Builder
		for _, run := range draft.Model.Runs {
			body.WriteString(run.Text)
		}
		if body.String() != draft.Text {
			return nil, fmt.Errorf("proofreading text and ownership differ")
		}
	}
	header, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, err
	}
	data := make([]byte, 0, len(proofreadingDraftFileHeader)+len(header)+len("\n-->\n")+len(draft.Text))
	data = append(data, proofreadingDraftFileHeader...)
	data = append(data, header...)
	data = append(data, '\n', '-', '-', '>', '\n')
	data = append(data, draft.Text...)
	return data, nil
}

func decodeProofreadingDraftFile(data []byte) (proofreadingPersistedDraft, error) {
	if len(data) > 128<<20 {
		return proofreadingPersistedDraft{}, fmt.Errorf("saved proofreading copy exceeds 128 MB")
	}
	if !bytes.HasPrefix(data, []byte(proofreadingDraftFileHeader)) {
		return proofreadingPersistedDraft{}, fmt.Errorf("proofreading copy has an invalid header")
	}
	headerStart := len(proofreadingDraftFileHeader)
	separator := []byte("\n-->\n")
	separatorOffset := bytes.Index(data[headerStart:], separator)
	if separatorOffset < 0 {
		return proofreadingPersistedDraft{}, fmt.Errorf("proofreading copy metadata is incomplete")
	}
	var metadata proofreadingDraftFileMetadata
	if err := json.Unmarshal(data[headerStart:headerStart+separatorOffset], &metadata); err != nil {
		return proofreadingPersistedDraft{}, fmt.Errorf("read proofreading copy metadata: %w", err)
	}
	if metadata.Model != nil {
		if _, err := proofreading.Restore(*metadata.Model); err != nil {
			return proofreadingPersistedDraft{}, err
		}
	}
	if metadata.Version != 1 && metadata.Version != 2 {
		return proofreadingPersistedDraft{}, fmt.Errorf("unsupported proofreading copy version %d", metadata.Version)
	}
	if metadata.Version == 2 && metadata.Model == nil {
		return proofreadingPersistedDraft{}, fmt.Errorf("proofreading v2 model missing")
	}
	textStart := headerStart + separatorOffset + len(separator)
	text := string(data[textStart:])
	if metadata.Model != nil {
		var canonical strings.Builder
		for _, run := range metadata.Model.Runs {
			canonical.WriteString(run.Text)
		}
		if canonical.String() != text {
			return proofreadingPersistedDraft{}, fmt.Errorf("proofreading text and ownership snapshot differ; original copy retained")
		}
	}
	if len(text) > 50<<20 {
		return proofreadingPersistedDraft{}, fmt.Errorf("saved proofreading text exceeds 50 MB")
	}
	return proofreadingPersistedDraft{
		Model: metadata.Model, Version: metadata.Version, SourceAHash: metadata.SourceAHash, SourceBHash: metadata.SourceBHash,
		Pages: metadata.Pages, Text: text, ManualBreaks: metadata.ManualBreaks,
		AlignmentOffset: metadata.AlignmentOffset, AlignmentScrollOffset: metadata.AlignmentScrollOffset,
		TextSizeStep: metadata.TextSizeStep,
	}, nil
}

func validateProofreadingDraftForRestore(draft proofreadingPersistedDraft, sourceAHash, sourceBHash string) error {
	if draft.Version != 1 && draft.Version != 2 {
		return fmt.Errorf("unsupported proofreading copy version %d", draft.Version)
	}
	if draft.Version == 2 && draft.Model == nil {
		return fmt.Errorf("proofreading v2 model missing")
	}
	if draft.SourceAHash != sourceAHash || draft.SourceBHash != sourceBHash {
		return fmt.Errorf("saved proofreading copy does not match the selected A/B sources")
	}
	return nil
}

func writeProofreadingDraftFile(path string, draft proofreadingPersistedDraft) error {
	data, err := encodeProofreadingDraftFile(draft)
	if err != nil {
		return fmt.Errorf("encode proofreading copy: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create proofreading project directory: %w", err)
	}
	if draft.Model != nil {
		if old, readErr := os.ReadFile(path); readErr == nil {
			legacy, decodeErr := decodeProofreadingDraftFile(old)
			if decodeErr != nil {
				return fmt.Errorf("preserve unreadable proofreading copy: %w", decodeErr)
			}
			if legacy.Model == nil {
				backup, e := os.OpenFile(path+".v1-backup", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
				if e == nil {
					_, e = backup.Write(old)
					if e == nil {
						e = backup.Sync()
					}
					closeErr := backup.Close()
					if e == nil {
						e = closeErr
					}
				}
				if os.IsExist(e) {
					saved, readErr := os.ReadFile(path + ".v1-backup")
					if readErr != nil {
						return readErr
					}
					if !bytes.Equal(saved, old) {
						return fmt.Errorf("legacy backup already exists with different content; original copy retained")
					}
					e = nil
				}
				if e != nil {
					return fmt.Errorf("preserve legacy proofreading copy: %w", e)
				}
			}
		} else if !os.IsNotExist(readErr) {
			return readErr
		}
	}
	temporary, err := os.CreateTemp(dir, ".proofreading-copy-*.tmp")
	if err != nil {
		return fmt.Errorf("create proofreading copy temp file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	defer temporary.Close()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure proofreading copy temp file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write proofreading copy: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync proofreading copy: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close proofreading copy: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if _, statErr := os.Stat(path); statErr != nil {
			return fmt.Errorf("replace proofreading copy: %w", err)
		}
		backupPath := path + ".previous"
		_ = os.Remove(backupPath)
		if backupErr := os.Rename(path, backupPath); backupErr != nil {
			return fmt.Errorf("preserve previous proofreading copy: %w", backupErr)
		}
		if replaceErr := os.Rename(temporaryPath, path); replaceErr != nil {
			_ = os.Rename(backupPath, path)
			return fmt.Errorf("replace proofreading copy: %w", replaceErr)
		}
		_ = os.Remove(backupPath)
	}
	if directory, err := os.Open(dir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func loadLegacyProofreadingDraft(runtime *desktopRuntime, projectID, sourceAHash, sourceBHash string) (*proofreadingPersistedDraft, bool, error) {
	if runtime == nil {
		return nil, false, nil
	}
	runtime.workspaceDBMu.Lock()
	store := runtime.workspaceStore
	if store == nil {
		runtime.workspaceDBMu.Unlock()
		return nil, false, nil
	}
	document, found, err := store.LoadDocument(projectID)
	runtime.workspaceDBMu.Unlock()
	if err != nil || !found {
		return nil, found, err
	}
	if document.Format != "proofreading-draft-v1" {
		return nil, true, fmt.Errorf("unsupported legacy proofreading draft format %q", document.Format)
	}
	if len(document.Payload) > (64 << 20) {
		return nil, true, fmt.Errorf("legacy proofreading draft exceeds 128 MB")
	}
	var draft proofreadingPersistedDraft
	if err := json.Unmarshal(document.Payload, &draft); err != nil {
		return nil, true, fmt.Errorf("read legacy proofreading draft: %w", err)
	}
	if draft.Version != 1 || draft.SourceBHash != sourceBHash || (draft.SourceAHash != "" && draft.SourceAHash != sourceAHash) {
		return nil, true, fmt.Errorf("legacy proofreading draft does not match the selected A/B sources")
	}
	if len(draft.Text) > (50 << 20) {
		return nil, true, fmt.Errorf("legacy proofreading text exceeds 50 MB")
	}
	draft.SourceAHash = sourceAHash
	return &draft, true, nil
}

func (a *App) startProofreadingDraftRestore() {
	state := a.proofreading
	if state == nil || state.projectDocumentID == "" {
		return
	}
	if state.draftFilePath == "" {
		path, err := proofreadingDraftFilePath(state.bDocument.Path, state.projectDocumentID)
		if err != nil {
			state.draftPersistError = err.Error()
			a.rebuildProofreadingXPages()
			return
		}
		state.draftFilePath = path
	}
	state.draftRestoreGen++
	generation := state.draftRestoreGen
	state.draftRestoring = true
	state.draftPersistError = ""
	projectID, sourceAHash, sourceBHash := state.projectDocumentID, state.aSourceHash, state.sourceBHash
	aPath, bPath := state.aDocument.Path, state.bDocument.Path
	runtime := a.desktopRuntime
	path := state.draftFilePath
	results, window, shutdown := state.draftRestoreResults, a.window, a.shutdown
	if !a.launchManagedBackground(func() {
		select {
		case <-shutdown:
			return
		default:
		}
		info, statErr := os.Stat(path)
		found := statErr == nil
		if errors.Is(statErr, os.ErrNotExist) {
			backupPath := path + ".previous"
			if backupInfo, backupErr := os.Stat(backupPath); backupErr == nil {
				path, info, found = backupPath, backupInfo, true
				statErr = nil
			}
		}
		var data []byte
		var err error
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			err = fmt.Errorf("inspect proofreading copy: %w", statErr)
		} else if found && info.Size() > (128<<20) {
			err = fmt.Errorf("saved proofreading copy exceeds 128 MB")
		} else if found {
			data, err = os.ReadFile(path)
		}
		var draft *proofreadingPersistedDraft
		if err == nil && found {
			decoded, decodeErr := decodeProofreadingDraftFile(data)
			if decodeErr != nil {
				err = decodeErr
			} else {
				if validateErr := validateProofreadingDraftForRestore(decoded, sourceAHash, sourceBHash); validateErr != nil {
					err = validateErr
				} else {
					draft = &decoded
				}
			}
		}
		if err == nil && !found {
			legacyDraft, legacyFound, legacyErr := loadLegacyProofreadingDraft(runtime, projectID, sourceAHash, sourceBHash)
			if legacyErr == nil && !legacyFound {
				olderProjectID := proofreadingLegacyWorkspaceDraftID(aPath, bPath, sourceBHash)
				if olderProjectID != projectID {
					legacyDraft, legacyFound, legacyErr = loadLegacyProofreadingDraft(runtime, olderProjectID, sourceAHash, sourceBHash)
				}
			}
			if legacyErr != nil {
				err = legacyErr
				found = legacyFound
			} else if legacyFound {
				draft = legacyDraft
				found = true
				err = writeProofreadingDraftFile(path, *legacyDraft)
			}
		}
		result := proofreadingDraftRestoreResult{generation: generation, draft: draft, found: found, err: err}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}) {
		state.draftRestoring = false
		state.draftPersistError = "could not start proofreading draft recovery"
		a.rebuildProofreadingXPages()
	}
}

func (a *App) queueProofreadingDraftPersistence() {
	state := a.proofreading
	if state == nil || state.controller == nil || !state.modelInitialAssigned || !state.bDraftInitialized || state.projectDocumentID == "" || state.draftRestoreFailed {
		return
	}
	if state.draftFilePath == "" {
		path, err := proofreadingDraftFilePath(state.bDocument.Path, state.projectDocumentID)
		if err != nil {
			state.draftPersistError = err.Error()
			return
		}
		state.draftFilePath = path
	}
	generation := state.draftPersistGen.Add(1)
	state.draftSavePending = true
	state.draftAutoSaved = false
	state.draftPersistError = ""
	if state.controller == nil {
		return
	}
	modelSnapshot := state.controller.Model.Snapshot()
	draft := proofreadingPersistedDraft{
		Model:   &modelSnapshot,
		Version: 2, SourceAHash: state.aSourceHash, SourceBHash: state.sourceBHash, Text: state.bDraftText,

		TextSizeStep: state.xTextSizeStep,
	}
	results, path, window := state.draftSaveResults, state.draftFilePath, a.window
	if !a.launchManagedBackground(func() {
		state.draftFileMu.Lock()
		if state.draftPersistGen.Load() != generation {
			state.draftFileMu.Unlock()
			return
		}
		err := writeProofreadingDraftFile(path, draft)
		state.draftFileMu.Unlock()
		if state.draftPersistGen.Load() != generation {
			return
		}
		result := proofreadingDraftSaveResult{generation: generation, err: err}
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		default:
		}
	}) {
		state.draftSavePending = false
		state.draftPersistError = "could not start proofreading draft save"
	}
}

func (a *App) applyProofreadingPageEdit(page proofreadingXPage, editedText string) {
	s := a.proofreading
	if s == nil || s.controller == nil {
		return
	}
	if err := s.controller.Model.ReplacePage(proofreading.PageID(page.PageIndex+1), editedText); err != nil {
		s.draftPersistError = err.Error()
		return
	}
	a.proofreadingModelChanged(false)
}
