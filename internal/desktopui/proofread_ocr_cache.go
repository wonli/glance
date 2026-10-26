package desktopui

import (
	"context"
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

type proofreadingOCRStepPageCache struct {
	Version      int        `json:"version"`
	PageNumber   int        `json:"page_number"`
	PageCount    int        `json:"page_count"`
	RenderDPI    int        `json:"render_dpi"`
	SourceAHash  string     `json:"source_a_sha256"`
	Model        string     `json:"model"`
	OptionsHash  string     `json:"options_sha256"`
	OriginalText string     `json:"original_text"`
	Layout       ocr.Layout `json:"layout"`
}

const proofreadingOCRStepRenderDPI = 144

func proofreadingOCRCachePaths(sourceAHash, optionsHash string) (root, pagesDir, manifestPath string, err error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", "", "", fmt.Errorf("resolve proofreading cache directory: %w", err)
	}
	if len(sourceAHash) != 64 {
		return "", "", "", errors.New("invalid PDF source hash")
	}
	if _, err = hex.DecodeString(sourceAHash); err != nil {
		return "", "", "", errors.New("invalid PDF source hash")
	}
	root = filepath.Join(configDir, "Glance Office", "proofreading", "ocr", sourceAHash, optionsHash)
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

// loadProofreadingOCRCachePage reuses one page from the complete-book cache or
// a page-at-a-time cache without loading every page into memory.
func loadProofreadingOCRCachePage(sourceAHash, model string, options ocr.Options, pageNumber, expectedPages int) (ocr.Page, bool, error) {
	if sourceAHash == "" || model == "" || pageNumber < 1 || expectedPages < pageNumber || expectedPages > proofreadingOCRCacheMaxPages {
		return ocr.Page{}, false, nil
	}
	optionsHash, err := proofreadingOCRCacheOptionsHash(model, options)
	if err != nil {
		return ocr.Page{}, false, fmt.Errorf("hash OCR cache options: %w", err)
	}
	root, pagesDir, manifestPath, err := proofreadingOCRCachePaths(sourceAHash, optionsHash)
	if err != nil {
		return ocr.Page{}, false, err
	}
	page, found, stepErr := loadProofreadingOCRStepCachePage(root, sourceAHash, model, optionsHash, pageNumber, expectedPages)
	if found {
		return page, true, nil
	}
	var manifest proofreadingOCRCacheManifest
	if err = readProofreadingOCRCacheJSON(manifestPath, &manifest); err == nil &&
		manifest.Version == proofreadingOCRCacheVersion && manifest.Complete &&
		manifest.SourceAHash == sourceAHash && manifest.Model == model &&
		manifest.OptionsHash == optionsHash && manifest.PageCount == expectedPages {
		var record proofreadingOCRCachePage
		path := filepath.Join(pagesDir, fmt.Sprintf("%06d.json", pageNumber))
		if err = readProofreadingOCRCacheJSON(path, &record); err == nil &&
			record.Version == proofreadingOCRCacheVersion && record.PageNumber == pageNumber &&
			record.PageCount == expectedPages && record.SourceAHash == sourceAHash &&
			record.Model == model && record.OptionsHash == optionsHash {
			page, pageErr := proofreadingOCRPageFromCache(record.PageNumber, record.OriginalText, record.Layout)
			if pageErr == nil {
				return page, true, nil
			}
		}
	}

	return ocr.Page{}, false, stepErr
}

func loadProofreadingOCRStepCachePage(root, sourceAHash, model, optionsHash string, pageNumber, expectedPages int) (ocr.Page, bool, error) {
	path := filepath.Join(root, "step-pages", fmt.Sprintf("%06d.json", pageNumber))
	var record proofreadingOCRStepPageCache
	if err := readProofreadingOCRCacheJSON(path, &record); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ocr.Page{}, false, nil
		}
		return ocr.Page{}, false, fmt.Errorf("read OCR page cache %d: %w", pageNumber, err)
	}
	if record.Version != 1 || record.PageNumber != pageNumber || record.PageCount != expectedPages ||
		record.RenderDPI != proofreadingOCRStepRenderDPI || record.SourceAHash != sourceAHash ||
		record.Model != model || record.OptionsHash != optionsHash {
		return ocr.Page{}, false, nil
	}
	page, err := proofreadingOCRPageFromCache(record.PageNumber, record.OriginalText, record.Layout)
	if err != nil {
		return ocr.Page{}, false, fmt.Errorf("validate OCR page cache %d: %w", pageNumber, err)
	}
	return page, true, nil
}

func proofreadingOCRPageFromCache(pageNumber int, markdown string, layout ocr.Layout) (ocr.Page, error) {
	raw, err := json.Marshal(struct {
		PrunedResult ocr.Layout `json:"prunedResult"`
	}{PrunedResult: layout})
	if err != nil {
		return ocr.Page{}, err
	}
	page := ocr.Page{Number: pageNumber, Markdown: markdown, Raw: raw}
	if _, err = page.Layout(); err != nil {
		return ocr.Page{}, err
	}
	return page, nil
}

func saveProofreadingOCRStepPageCache(sourceAHash, model string, options ocr.Options, page ocr.Page, pageCount int) error {
	if sourceAHash == "" || model == "" || page.Number < 1 || pageCount < page.Number || pageCount > proofreadingOCRCacheMaxPages {
		return errors.New("invalid OCR page cache identity")
	}
	optionsHash, err := proofreadingOCRCacheOptionsHash(model, options)
	if err != nil {
		return fmt.Errorf("hash OCR cache options: %w", err)
	}
	root, _, _, err := proofreadingOCRCachePaths(sourceAHash, optionsHash)
	if err != nil {
		return err
	}
	layout, err := page.Layout()
	if err != nil {
		return fmt.Errorf("read OCR layout for page %d: %w", page.Number, err)
	}
	dir := filepath.Join(root, "step-pages")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create page OCR cache: %w", err)
	}
	record := proofreadingOCRStepPageCache{
		Version: 1, PageNumber: page.Number, PageCount: pageCount,
		RenderDPI: proofreadingOCRStepRenderDPI, SourceAHash: sourceAHash,
		Model: model, OptionsHash: optionsHash, OriginalText: page.Markdown, Layout: layout,
	}
	if err = writeProofreadingOCRCacheJSON(filepath.Join(dir, fmt.Sprintf("%06d.json", page.Number)), record); err != nil {
		return fmt.Errorf("save OCR page cache %d: %w", page.Number, err)
	}
	return nil
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

type proofreadingOCRComparisonCacheRecord struct {
	Version      int                           `json:"version"`
	APage        int                           `json:"a_page"`
	BPage        int                           `json:"b_page"`
	SourceHash   string                        `json:"source_hash"`
	DraftHash    string                        `json:"draft_hash"`
	Comparison   proofreadingOCRStepComparison `json:"comparison"`
	UpdatedAtUTC time.Time                     `json:"updated_at_utc"`
}

func proofreadingOCRComparisonCacheDir(sourceAHash string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve cache dir: %w", err)
	}
	if len(sourceAHash) != 64 {
		return "", errors.New("invalid PDF source hash")
	}
	dir := filepath.Join(configDir, "Glance Office", "proofreading", "comparisons", sourceAHash)
	return dir, nil
}

func saveProofreadingOCRComparisonCache(comparison proofreadingOCRStepComparison) error {
	if comparison.SourceHash == "" || comparison.DraftHash == "" {
		return errors.New("cannot cache comparison without source and draft hash")
	}
	dir, err := proofreadingOCRComparisonCacheDir(comparison.SourceHash)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	filename := fmt.Sprintf("a%d_b%d_%s.json", comparison.APage, comparison.BPage, comparison.DraftHash)
	path := filepath.Join(dir, filename)
	record := proofreadingOCRComparisonCacheRecord{
		Version:      proofreadingOCRCacheVersion,
		APage:        comparison.APage,
		BPage:        comparison.BPage,
		SourceHash:   comparison.SourceHash,
		DraftHash:    comparison.DraftHash,
		Comparison:   comparison,
		UpdatedAtUTC: time.Now().UTC(),
	}
	return writeProofreadingOCRCacheJSON(path, record)
}

func loadProofreadingOCRComparisonCache(sourceAHash string, aPage, bPage int, draftHash string) (proofreadingOCRStepComparison, bool, error) {
	if sourceAHash == "" || draftHash == "" {
		return proofreadingOCRStepComparison{}, false, nil
	}
	dir, err := proofreadingOCRComparisonCacheDir(sourceAHash)
	if err != nil {
		return proofreadingOCRStepComparison{}, false, err
	}
	filename := fmt.Sprintf("a%d_b%d_%s.json", aPage, bPage, draftHash)
	path := filepath.Join(dir, filename)
	var record proofreadingOCRComparisonCacheRecord
	if err := readProofreadingOCRCacheJSON(path, &record); err != nil {
		if os.IsNotExist(err) {
			return proofreadingOCRStepComparison{}, false, nil
		}
		return proofreadingOCRStepComparison{}, false, err
	}
	if record.SourceHash != sourceAHash || record.DraftHash != draftHash || record.APage != aPage || record.BPage != bPage {
		return proofreadingOCRStepComparison{}, false, nil
	}
	return record.Comparison, true, nil
}

func loadProofreadingOCRStepHistory(ctx context.Context, sourceAHash string, pageCount int, xPages []proofreadingXPage) []proofreadingOCRStepHistoryEntry {
	if ctx == nil || sourceAHash == "" || pageCount <= 0 || pageCount > proofreadingOCRCacheMaxPages || len(xPages) == 0 {
		return nil
	}

	entries := make([]proofreadingOCRStepHistoryEntry, 0)
	for pageIndex := 0; pageIndex < min(pageCount, len(xPages)); pageIndex++ {
		if ctx.Err() != nil {
			return nil
		}
		xPage := xPages[pageIndex]
		draftHash := proofreadingOCRDraftSnapshotHash(xPage.Content)
		comparison, found, err := loadProofreadingOCRComparisonCache(sourceAHash, pageIndex, pageIndex, draftHash)
		if err != nil || !found || comparison.SourceHash != sourceAHash || comparison.DraftHash != draftHash ||
			comparison.APage != pageIndex || comparison.BPage != pageIndex || comparison.BText != xPage.Content {
			continue
		}
		if comparison.BStart != xPage.ContentStart || comparison.BEnd != xPage.ContentEnd {
			rebaseProofreadingOCRStepComparison(&comparison, xPage.ContentStart)
		}

		cachedPage, found, err := loadProofreadingOCRCachePage(sourceAHash, ocr.DefaultModel, ocr.Options{}, pageIndex+1, pageCount)
		if err != nil || !found || cachedPage.Number != pageIndex+1 {
			continue
		}
		layout, err := cachedPage.Layout()
		if err != nil || layout.Width <= 0 || layout.Height <= 0 {
			continue
		}
		entries = append(entries, proofreadingOCRStepHistoryEntry{page: pageIndex, layout: layout, comparison: comparison})
	}
	if ctx.Err() != nil {
		return nil
	}
	return entries
}
