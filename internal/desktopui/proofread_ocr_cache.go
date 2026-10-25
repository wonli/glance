package desktopui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"glance/internal/fileutil"
	"glance/internal/ocr"
)

const (
	proofreadingOCRCacheVersion  = 1
	proofreadingOCRCacheMaxPages = 2000
	proofreadingOCRCacheMaxFile  = 64 << 20
)

type proofreadingOCRCacheManifest struct {
	Version      int       `json:"version"`
	SourceAHash  string    `json:"source_a_sha256"`
	Model        string    `json:"model"`
	OptionsHash  string    `json:"options_sha256"`
	JobID        string    `json:"job_id,omitempty"`
	PageCount    int       `json:"page_count"`
	Complete     bool      `json:"complete"`
	UpdatedAtUTC time.Time `json:"updated_at_utc"`
}

type proofreadingOCRCachePage struct {
	Version      int        `json:"version"`
	PageNumber   int        `json:"page_number"`
	PageCount    int        `json:"page_count"`
	SourceAHash  string     `json:"source_a_sha256"`
	Model        string     `json:"model"`
	OptionsHash  string     `json:"options_sha256"`
	JobID        string     `json:"job_id,omitempty"`
	OriginalText string     `json:"original_text"`
	Layout       ocr.Layout `json:"layout"`
}

func proofreadingOCRCachePaths(sourceAHash, optionsHash string) (root, pagesDir, manifestPath string, err error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", "", "", fmt.Errorf("resolve Proofreading cache directory: %w", err)
	}
	if len(sourceAHash) != 64 {
		return "", "", "", errors.New("invalid PDF source hash")
	}
	if _, err = hex.DecodeString(sourceAHash); err != nil {
		return "", "", "", errors.New("invalid PDF source hash")
	}
	root = filepath.Join(configDir, "Glance Office", "Proofreading", "ocr", sourceAHash, optionsHash)
	pagesDir = filepath.Join(root, "pages")
	manifestPath = filepath.Join(root, "manifest.json")
	return root, pagesDir, manifestPath, nil
}

func proofreadingOCRCacheOptionsHash(model string, options ocr.Options) (string, error) {
	encoded, err := json.Marshal(struct {
		Model   string      `json:"model"`
		Options ocr.Options `json:"options"`
	}{Model: model, Options: options})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func saveProofreadingOCRPageCache(sourceAHash, model, jobID string, options ocr.Options, pages []ocr.Page, expectedPages int) error {
	if sourceAHash == "" || model == "" {
		return errors.New("OCR cache requires a PDF hash and model")
	}
	if expectedPages <= 0 || expectedPages > proofreadingOCRCacheMaxPages || len(pages) != expectedPages {
		return fmt.Errorf("OCR cache received %d pages; expected %d", len(pages), expectedPages)
	}
	optionsHash, err := proofreadingOCRCacheOptionsHash(model, options)
	if err != nil {
		return fmt.Errorf("hash OCR cache options: %w", err)
	}
	_, pagesDir, manifestPath, err := proofreadingOCRCachePaths(sourceAHash, optionsHash)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(pagesDir, 0700); err != nil {
		return fmt.Errorf("create OCR cache directory: %w", err)
	}
	manifest := proofreadingOCRCacheManifest{
		Version: proofreadingOCRCacheVersion, SourceAHash: sourceAHash, Model: model,
		OptionsHash: optionsHash, JobID: jobID, PageCount: expectedPages,
		Complete: false, UpdatedAtUTC: time.Now().UTC(),
	}
	if err = writeProofreadingOCRCacheJSON(manifestPath, manifest); err != nil {
		return fmt.Errorf("start OCR cache checkpoint: %w", err)
	}
	for i, page := range pages {
		pageNumber := i + 1
		if page.Number != pageNumber {
			return fmt.Errorf("OCR cache page sequence is invalid at page %d", pageNumber)
		}
		layout, layoutErr := page.Layout()
		if layoutErr != nil {
			return fmt.Errorf("read OCR layout for page %d: %w", pageNumber, layoutErr)
		}
		record := proofreadingOCRCachePage{
			Version: proofreadingOCRCacheVersion, PageNumber: pageNumber, PageCount: expectedPages,
			SourceAHash: sourceAHash, Model: model, OptionsHash: optionsHash, JobID: jobID,
			OriginalText: page.Markdown, Layout: layout,
		}
		if err = writeProofreadingOCRCacheJSON(filepath.Join(pagesDir, fmt.Sprintf("%06d.json", pageNumber)), record); err != nil {
			return fmt.Errorf("save OCR cache page %d: %w", pageNumber, err)
		}
	}
	manifest.Complete = true
	manifest.UpdatedAtUTC = time.Now().UTC()
	if err = writeProofreadingOCRCacheJSON(manifestPath, manifest); err != nil {
		return fmt.Errorf("finish OCR cache checkpoint: %w", err)
	}
	return nil
}

func loadProofreadingOCRPageCache(sourceAHash, model string, options ocr.Options, expectedPages int) ([]ocr.Page, bool, error) {
	if sourceAHash == "" || model == "" || expectedPages <= 0 || expectedPages > proofreadingOCRCacheMaxPages {
		return nil, false, nil
	}
	optionsHash, err := proofreadingOCRCacheOptionsHash(model, options)
	if err != nil {
		return nil, false, fmt.Errorf("hash OCR cache options: %w", err)
	}
	_, pagesDir, manifestPath, err := proofreadingOCRCachePaths(sourceAHash, optionsHash)
	if err != nil {
		return nil, false, err
	}
	var manifest proofreadingOCRCacheManifest
	if err = readProofreadingOCRCacheJSON(manifestPath, &manifest); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read OCR cache manifest: %w", err)
	}
	if manifest.Version != proofreadingOCRCacheVersion || !manifest.Complete || manifest.SourceAHash != sourceAHash || manifest.Model != model || manifest.OptionsHash != optionsHash || manifest.PageCount != expectedPages {
		return nil, false, nil
	}
	pages := make([]ocr.Page, expectedPages)
	for i := 0; i < expectedPages; i++ {
		pageNumber := i + 1
		var record proofreadingOCRCachePage
		path := filepath.Join(pagesDir, fmt.Sprintf("%06d.json", pageNumber))
		if err = readProofreadingOCRCacheJSON(path, &record); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, false, nil
			}
			return nil, false, fmt.Errorf("read OCR cache page %d: %w", pageNumber, err)
		}
		if record.Version != proofreadingOCRCacheVersion || record.PageNumber != pageNumber || record.PageCount != expectedPages || record.SourceAHash != sourceAHash || record.Model != model || record.OptionsHash != optionsHash {
			return nil, false, fmt.Errorf("OCR cache page %d does not match its manifest", pageNumber)
		}
		raw, marshalErr := json.Marshal(struct {
			PrunedResult ocr.Layout `json:"prunedResult"`
		}{PrunedResult: record.Layout})
		if marshalErr != nil {
			return nil, false, fmt.Errorf("rebuild OCR layout for page %d: %w", pageNumber, marshalErr)
		}
		pages[i] = ocr.Page{Number: pageNumber, Markdown: record.OriginalText, Raw: raw}
		if _, err = pages[i].Layout(); err != nil {
			return nil, false, fmt.Errorf("validate OCR cache page %d: %w", pageNumber, err)
		}
	}
	return pages, true, nil
}

func writeProofreadingOCRCacheJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteBytes(path, encoded,
		fileutil.WithPerm(0600), fileutil.WithAutoCreateDir(false), fileutil.WithPreserveTargetPerm(false))
}

func readProofreadingOCRCacheJSON(path string, value any) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > proofreadingOCRCacheMaxFile {
		return errors.New("OCR cache file is not a regular file or exceeds the size limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, value); err != nil {
		return err
	}
	return nil
}
