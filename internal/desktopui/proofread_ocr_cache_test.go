package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gioui.org/layout"
	"glance/internal/format/pdfdoc"
	"glance/internal/ocr"
	"glance/internal/textalign"
)

func TestProofreadingOCRAlignmentProgressIsIndependentOfPageOrigin(t *testing.T) {
	page := proofreadingOCRCacheTestPage(1, "天地玄黄宇宙洪荒日月盈昃辰宿列张")
	pages := []ocr.Page{page}
	firstDraft := "前言\n" + page.Markdown + "\n后记"
	changedDraft := page.Markdown + "\n新的正文"
	run := func(markdown string) ([]proofreadingOCRPage, []textalign.AlignmentProgress) {
		t.Helper()
		var progress []textalign.AlignmentProgress
		aligned, err := alignProofreadingOCRPages(context.Background(), pages, markdown, func(update textalign.AlignmentProgress) {
			progress = append(progress, update)
		})
		if err != nil {
			t.Fatal(err)
		}
		return aligned, progress
	}

	cacheAlignment, cacheProgress := run(firstDraft)
	freshAlignment, freshProgress := run(firstDraft)
	if !reflect.DeepEqual(cacheAlignment, freshAlignment) || !reflect.DeepEqual(cacheProgress, freshProgress) {
		t.Fatalf("cached/fresh OCR page inputs diverged: cached=%+v progress=%+v fresh=%+v progress=%+v", cacheAlignment, cacheProgress, freshAlignment, freshProgress)
	}
	if len(cacheProgress) == 0 || cacheProgress[0].Phase != textalign.AlignmentPhaseCandidates || cacheProgress[len(cacheProgress)-1].Phase != textalign.AlignmentPhasePath {
		t.Fatalf("alignment helper did not report candidate and path phases: %+v", cacheProgress)
	}
	changedAlignment, changedProgress := run(changedDraft)
	if !reflect.DeepEqual(cacheProgress, changedProgress) {
		t.Fatalf("B draft changed the alignment progress phases: before=%+v after=%+v", cacheProgress, changedProgress)
	}
	if len(cacheAlignment) != 1 || cacheAlignment[0].result.Selected == nil || len(changedAlignment) != 1 || changedAlignment[0].result.Selected == nil {
		t.Fatalf("fixture failed to produce evidence: original=%+v changed=%+v", cacheAlignment, changedAlignment)
	}
	if cacheAlignment[0].result.Selected.Range == changedAlignment[0].result.Selected.Range {
		t.Fatalf("alignment was not recalculated for changed B draft: range=%+v", changedAlignment[0].result.Selected.Range)
	}
}

func TestProofreadingOCRCacheRoundTripsCompleteBook(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const sourceHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pages := []ocr.Page{
		proofreadingOCRCacheTestPage(1, "天地玄黄"),
		proofreadingOCRCacheTestPage(2, "宇宙洪荒"),
	}
	options := ocr.Options{}
	if err := saveProofreadingOCRPageCache(sourceHash, ocr.DefaultModel, "job-test", options, pages, len(pages)); err != nil {
		t.Fatal(err)
	}
	got, hit, err := loadProofreadingOCRPageCache(sourceHash, ocr.DefaultModel, options, len(pages))
	if err != nil || !hit {
		t.Fatalf("complete OCR cache hit=%v err=%v", hit, err)
	}
	if len(got) != len(pages) {
		t.Fatalf("cached pages=%d, want %d", len(got), len(pages))
	}
	for i := range pages {
		if got[i].Number != i+1 || got[i].Markdown != pages[i].Markdown {
			t.Fatalf("cached page %d identity/text changed: got=%+v want=%+v", i+1, got[i], pages[i])
		}
		layout, err := got[i].Layout()
		if err != nil || len(layout.Blocks) != 1 || layout.Blocks[0].Text != pages[i].Markdown {
			t.Fatalf("cached page %d layout = %+v, err=%v", i+1, layout, err)
		}
	}
	baseLayout, err := pages[0].Layout()
	if err != nil {
		t.Fatal(err)
	}
	localized := baseLayout
	localized.CharacterVersion = ocr.CharacterGeometryVersion
	localized.Characters = []ocr.CharacterBox{{BlockID: 1, Offset: 0, Text: "天", Bounds: [4]float64{1, 2, 10, 20}}}
	enriched, err := pages[0].WithLayout(localized)
	if err != nil {
		t.Fatal(err)
	}
	if err = saveProofreadingOCRStepPageCache(sourceHash, ocr.DefaultModel, options, enriched, len(pages)); err != nil {
		t.Fatal(err)
	}
	preferred, found, err := loadProofreadingOCRCachePage(sourceHash, ocr.DefaultModel, options, 1, len(pages))
	if err != nil || !found || preferred.Markdown != pages[0].Markdown {
		t.Fatalf("geometry cache changed recognized Markdown or was missed: hit=%v err=%v", found, err)
	}
	geometry, geometryErr, cacheErr := ensureProofreadingOCRCharacterGeometry(context.Background(), nil, 0, len(pages), sourceHash, baseLayout)
	if geometryErr != nil || cacheErr != nil || len(geometry.Characters) != 1 {
		t.Fatalf("enriched cache was not reused before rendering: %+v %v %v", geometry, geometryErr, cacheErr)
	}

}

func TestProofreadingOCRCacheRejectsIncompleteOrMismatchedPageSet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const sourceHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	options := ocr.Options{}
	pages := []ocr.Page{proofreadingOCRCacheTestPage(1, "天地玄黄"), proofreadingOCRCacheTestPage(2, "宇宙洪荒")}
	if err := saveProofreadingOCRPageCache(sourceHash, ocr.DefaultModel, "job-test", options, pages, len(pages)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name        string
		sourceHash  string
		model       string
		options     ocr.Options
		pageCount   int
		wantHit     bool
		wantReadErr bool
	}{
		{name: "different source", sourceHash: strings.Repeat("c", 64), model: ocr.DefaultModel, options: options, pageCount: 2},
		{name: "different model", sourceHash: sourceHash, model: "other-model", options: options, pageCount: 2},
		{name: "different options", sourceHash: sourceHash, model: ocr.DefaultModel, options: ocr.Options{UseDocUnwarping: true}, pageCount: 2},
		{name: "different page count", sourceHash: sourceHash, model: ocr.DefaultModel, options: options, pageCount: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, hit, err := loadProofreadingOCRPageCache(test.sourceHash, test.model, test.options, test.pageCount)
			if err != nil || hit != test.wantHit {
				t.Fatalf("hit=%v err=%v, want hit=%v", hit, err, test.wantHit)
			}
		})
	}

	optionsHash, err := proofreadingOCRCacheOptionsHash(ocr.DefaultModel, options)
	if err != nil {
		t.Fatal(err)
	}
	_, pagesDir, manifestPath, err := proofreadingOCRCachePaths(sourceHash, optionsHash)
	if err != nil {
		t.Fatal(err)
	}
	var manifest proofreadingOCRCacheManifest
	if err := readProofreadingOCRCacheJSON(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	t.Run("incomplete manifest", func(t *testing.T) {
		manifest.Complete = false
		if err := writeProofreadingOCRCacheJSON(manifestPath, manifest); err != nil {
			t.Fatal(err)
		}
		_, hit, err := loadProofreadingOCRPageCache(sourceHash, ocr.DefaultModel, options, len(pages))
		if err != nil || hit {
			t.Fatalf("incomplete cache hit=%v err=%v, want a miss", hit, err)
		}
	})

	manifest.Complete = true
	if err := writeProofreadingOCRCacheJSON(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	t.Run("missing page record", func(t *testing.T) {
		if err := os.Remove(filepath.Join(pagesDir, "000002.json")); err != nil {
			t.Fatal(err)
		}
		_, hit, err := loadProofreadingOCRPageCache(sourceHash, ocr.DefaultModel, options, len(pages))
		if err != nil || hit {
			t.Fatalf("cache with missing page hit=%v err=%v, want a miss", hit, err)
		}
	})

	if err := saveProofreadingOCRPageCache(sourceHash, ocr.DefaultModel, "job-test", options, pages, len(pages)); err != nil {
		t.Fatal(err)
	}
	t.Run("mismatched page identity", func(t *testing.T) {
		pagePath := filepath.Join(pagesDir, "000002.json")
		var record proofreadingOCRCachePage
		if err := readProofreadingOCRCacheJSON(pagePath, &record); err != nil {
			t.Fatal(err)
		}
		record.SourceAHash = strings.Repeat("d", 64)
		if err := writeProofreadingOCRCacheJSON(pagePath, record); err != nil {
			t.Fatal(err)
		}
		_, hit, err := loadProofreadingOCRPageCache(sourceHash, ocr.DefaultModel, options, len(pages))
		if err == nil || hit {
			t.Fatalf("mismatched page identity hit=%v err=%v, want rejected cache", hit, err)
		}
	})
}

func TestProofreadingOCRCacheHitSkipsCredentialLookupAndUsesCurrentDraft(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("HOME", configRoot)
	t.Setenv("PADDLEOCR_ENDPOINT", ocr.DefaultEndpoint)
	const sourceHash = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	phrase := "天地玄黄宇宙洪荒日月盈昃辰宿列张寒来暑往秋收冬藏"
	ocrPages := []ocr.Page{
		proofreadingOCRCacheTestPage(1, phrase),
		proofreadingOCRCacheTestPage(2, "风起云涌龙腾虎跃"),
		proofreadingOCRCacheTestPage(3, "春生夏长秋收冬藏"),
		proofreadingOCRCacheTestPage(4, "山高水长云淡风轻"),
	}
	if err := saveProofreadingOCRPageCache(sourceHash, ocr.DefaultModel, "job-cached", ocr.Options{}, ocrPages, len(ocrPages)); err != nil {
		t.Fatal(err)
	}

	bPath := filepath.Join(t.TempDir(), "book.md")
	firstDraft := "前言\n" + phrase + "\n后记"
	if err := os.WriteFile(bPath, []byte(firstDraft), 0o600); err != nil {
		t.Fatal(err)
	}
	bObject, err := loadProofreadingObject(bPath)
	if err != nil {
		t.Fatal(err)
	}
	defer bObject.Adapter.Close()
	pdf, err := pdfdoc.OpenSnapshot(pdfTestBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pdf.Close()
	credentials := &countingProofreadingOCRCredentials{}
	a := New(nil, "")
	a.documentSessionState = &documentSessionState{sharedDocumentSession: &sharedDocumentSession{
		liveDocumentState: liveDocumentState{pdfDoc: pdf},
	}}
	a.proofreading = &proofreadingWorkspace{
		aDocument:          &proofreadingObject{Name: "book.pdf", Path: "book.pdf"},
		bDocument:          bObject,
		bDraftText:         firstDraft,
		aSourceHash:        sourceHash,
		sourceBHash:        "b-file-hash",
		ocrScan:            newProofreadingOCRScan(),
		ocrCredentialStore: credentials,
	}
	defer a.stopAndWaitManagedBackground()

	runCachedScan := func() textalign.Range {
		t.Helper()
		a.requestProofreadingOCRScan()
		a.managedBackgroundTasks.Wait()
		a.processProofreadingOCRScan(layout.Context{})
		if a.proofreading.ocrScan.stage != proofreadingOCRComplete || !a.proofreading.ocrScan.cacheHit {
			t.Fatalf("cached OCR did not complete locally: stage=%v cacheHit=%v err=%q", a.proofreading.ocrScan.stage, a.proofreading.ocrScan.cacheHit, a.proofreading.ocrScan.err)
		}
		if a.proofreading.ocrScan.alignment.Phase != textalign.AlignmentPhasePath || a.proofreading.ocrScan.current != len(ocrPages)-1 {
			t.Fatalf("cached OCR did not publish real matching progress: phase=%q page=%d", a.proofreading.ocrScan.alignment.Phase, a.proofreading.ocrScan.current+1)
		}
		if len(a.proofreading.ocrScan.pages) == 0 || a.proofreading.ocrScan.pages[0].result.Selected == nil {
			t.Fatalf("cached OCR was not aligned to B: %+v", a.proofreading.ocrScan.pages)
		}
		return a.proofreading.ocrScan.pages[0].result.Selected.Range
	}

	firstRange := runCachedScan()
	if a.modal.Visible || credentials.gets != 0 {
		t.Fatalf("cache hit queried/opened credentials: modal=%v gets=%d", a.modal.Visible, credentials.gets)
	}
	a.proofreading.bDraftText = phrase + "\n新的正文"
	secondRange := runCachedScan()
	if a.modal.Visible || credentials.gets != 0 {
		t.Fatalf("second cache hit queried/opened credentials: modal=%v gets=%d", a.modal.Visible, credentials.gets)
	}
	if firstRange == secondRange || secondRange.Start != 0 {
		t.Fatalf("changed B draft did not recompute its OCR range: first=%+v second=%+v", firstRange, secondRange)
	}
}

func proofreadingOCRCacheTestPage(number int, text string) ocr.Page {
	order := 0
	raw, _ := json.Marshal(struct {
		Result ocr.Layout `json:"prunedResult"`
	}{Result: ocr.Layout{Width: 100, Height: 200, Blocks: []ocr.LayoutBlock{{
		ID: number, Label: "text", Text: text, Bounds: [4]float64{1, 2, 90, 190}, Order: &order,
	}}}})
	return ocr.Page{Number: number, Markdown: text, Raw: raw}
}

type countingProofreadingOCRCredentials struct{ gets int }

func (c *countingProofreadingOCRCredentials) Get(context.Context, string) (string, error) {
	c.gets++
	return "", errors.New("unexpected credential lookup")
}

func (*countingProofreadingOCRCredentials) Put(context.Context, string, string) error { return nil }
func (*countingProofreadingOCRCredentials) Delete(context.Context, string) error      { return nil }
