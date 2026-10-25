package desktopui

import (
	"context"
	"fmt"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"image"
	"image/color"
	"reflect"
	"unicode"

	"glance/internal/format/pdfdoc"
	"glance/internal/ocr"
)

func proofreadingOCRStepSourceCharacterBounds(step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison, frame proofreadingOCRDiffFrame, size image.Point) (image.Rectangle, bool) {
	if step == nil || frame.AOffset < 0 {
		return image.Rectangle{}, false
	}
	page, exists := step.pages[comparison.APage]
	if !exists {
		return image.Rectangle{}, false
	}
	for _, character := range page.layout.Characters {
		if character.BlockID == frame.ABlockID && character.Offset == frame.AOffset {
			bounds := proofreadingOCRBlockPixelBounds(size, page.layout.Width, page.layout.Height, character.Bounds)
			return bounds, !bounds.Empty()
		}
	}
	return image.Rectangle{}, false
}

func drawProofreadingOCRCharacterBoxes(gtx layout.Context, size image.Point, step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison) {
	frames := comparison.Frames
	running := step != nil && (step.diffRunning || step.diffPaused) &&
		step.diffPair == (proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}) &&
		step.diffDraftHash == comparison.DraftHash
	if running {
		frames = proofreadingOCRScannedDiffFrames(step, comparison)
	}
	if len(frames) == 0 {
		return
	}
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	defer area.Pop()
	seen := map[[2]int]bool{}
	resolvedA, _ := proofreadingOCRResolvedDiffRuns(nil, nil, step, comparison)
	redInk := color.NRGBA{R: 217, G: 48, B: 37, A: 255}
	greenStroke := color.NRGBA{R: 46, G: 160, B: 67, A: 255}
	greenFill := color.NRGBA{R: 46, G: 160, B: 67, A: 45}

	for index, frame := range frames {
		active := running && index == len(frames)-1
		if frame.Kind == proofreadingOCRDiffEqual && !active {
			continue
		}
		if frame.Kind == proofreadingOCRDiffBOnly || frame.ABlockID < 0 || frame.AOffset < 0 {
			continue
		}
		key := [2]int{frame.ABlockID, frame.AOffset}
		if seen[key] && !active {
			continue
		}
		seen[key] = true
		bounds, ok := proofreadingOCRStepSourceCharacterBounds(step, comparison, frame, size)
		if !ok {
			continue
		}
		if active {
			paint.FillShape(gtx.Ops, proofreadScanFill, clip.Rect(bounds).Op())
			paint.FillShape(gtx.Ops, proofreadScanInk, clip.Stroke{Path: clip.Rect(bounds).Path(), Width: float32(max(1, gtx.Dp(2)))}.Op())
		} else if resolvedA[key] {
			paint.FillShape(gtx.Ops, greenFill, clip.Rect(bounds).Op())
			paint.FillShape(gtx.Ops, greenStroke, clip.Stroke{Path: clip.Rect(bounds).Path(), Width: float32(max(1, gtx.Dp(1.5)))}.Op())
		} else {
			paint.FillShape(gtx.Ops, redInk, clip.Stroke{Path: clip.Rect(bounds).Path(), Width: float32(max(1, gtx.Dp(1.5)))}.Op())
		}
	}
}

func proofreadingOCRCharacterCoverage(layout ocr.Layout) (int, int) {
	eligible := map[int]bool{}
	total := 0
	for _, block := range layout.Blocks {
		if !proofreadingOCRDiffFurniture(block.Label) {
			eligible[block.ID] = true
			for _, r := range []rune(block.Text) {
				if !unicode.IsSpace(r) {
					total++
				}
			}
		}
	}
	seen := map[[2]int]bool{}
	for _, character := range layout.Characters {
		text := []rune(character.Text)
		if eligible[character.BlockID] && len(text) == 1 && !unicode.IsSpace(text[0]) {
			seen[[2]int{character.BlockID, character.Offset}] = true
		}
	}
	return len(seen), total
}

// Character localization runs on the existing managed comparison worker. A
// cache hit avoids both the cloud service and another native recognition pass.
func ensureProofreadingOCRCharacterGeometry(ctx context.Context, document *pdfdoc.Document, pageIndex, pageCount int, sourceHash string, layout ocr.Layout) (ocr.Layout, error, error) {
	cached, hit, cacheErr := loadProofreadingOCRCachePage(sourceHash, ocr.DefaultModel, ocr.Options{}, pageIndex+1, pageCount)
	if hit {
		prior, err := cached.Layout()
		if err == nil && prior.CharacterVersion == ocr.CharacterGeometryVersion && prior.Width == layout.Width && prior.Height == layout.Height && reflect.DeepEqual(prior.Blocks, layout.Blocks) {
			return prior, nil, cacheErr
		}
	}
	if err := ctx.Err(); err != nil {
		return layout, err, cacheErr
	}
	if document == nil {
		return layout, fmt.Errorf("原文字框定位失败：PDF不可用"), cacheErr
	}
	pixels, _, err := document.RenderVisualPage(pageIndex, proofreadingOCRStepRenderDPI)
	if err != nil {
		return layout, fmt.Errorf("原文字框定位失败：%w", err), cacheErr
	}
	localized, err := ocr.LocalizeCharacters(ctx, pixels, layout)
	if err != nil {
		return localized, fmt.Errorf("原文字框定位失败：%w", err), cacheErr
	}
	if hit {
		page, err := cached.WithLayout(localized)
		if err == nil {
			err = saveProofreadingOCRStepPageCache(sourceHash, ocr.DefaultModel, ocr.Options{}, page, pageCount)
		}
		return localized, nil, err
	}
	return localized, nil, cacheErr
}
