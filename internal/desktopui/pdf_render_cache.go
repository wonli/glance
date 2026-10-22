package desktopui

import (
	"gioui.org/op/paint"
	"image"
	"math"
)

const (
	pdfPageCacheRadius           = 3
	pdfPageViewportOverscan      = 1
	pdfThumbnailVisiblePrefetch  = 5
	pdfThumbnailCacheOverscan    = 1
	pdfThumbnailDPI              = 24.0
	pdfPageCacheBudgetBytes      = int64(64 << 20)
	pdfThumbnailCacheBudgetBytes = int64(16 << 20)
	pdfPageMaxRasterBytes        = int64(8_000_000 * 4)
	pdfPageCacheMaxEntries       = pdfPageCacheRadius*2 + 1
	pdfThumbnailCacheMaxEntries  = pdfThumbnailVisiblePrefetch + pdfThumbnailCacheOverscan*2
	pdfThumbnailErrorMaxEntries  = pdfThumbnailCacheMaxEntries
)

func (v *pdfViewState) pageCacheWindow() (first, last int) {
	if v == nil {
		return 0, 0
	}
	// At low zoom, the viewport can show more pages than the fixed radius.
	// Keep those pages and one neighbor eligible so an edge-crossing render
	// result is not discarded before the page becomes fully visible.
	first = v.page - pdfPageCacheRadius
	last = v.page + pdfPageCacheRadius
	visibleFirst := max(0, v.canvas.Position.First)
	visibleCount := max(1, v.canvas.Position.Count)
	visibleLast := visibleFirst + visibleCount - 1
	first = min(first, visibleFirst-pdfPageViewportOverscan)
	last = max(last, visibleLast+pdfPageViewportOverscan)
	return first, last
}

func (v *pdfViewState) pageInCacheWindow(index int) bool {
	first, last := v.pageCacheWindow()
	return index >= first && index <= last
}

func (v *pdfViewState) pageVisible(index int) bool {
	if v == nil {
		return false
	}
	position := v.canvas.Position
	count := position.Count
	if count <= 0 {
		return index == v.page
	}
	first := max(0, position.First)
	return index >= first && index < first+count
}

func (v *pdfViewState) canPrefetchPage(index int, dpi float64) bool {
	if v == nil || v.pageVisible(index) || index == v.page {
		return v != nil
	}
	position := v.canvas.Position
	first, count := max(0, position.First), position.Count
	if count <= 0 {
		first, count = v.page, 1
	}
	var pinnedBytes int64
	for page, result := range v.cache {
		if page == v.page || (page >= first && page < first+count) {
			pinnedBytes += pdfPageResultBytes(result)
		}
	}
	width, height := pdfCachedPageSize(v, index)
	pixels := width * height * dpi * dpi / (72 * 72)
	if pixels > 8_000_000 {
		pixels = 8_000_000
	}
	// Reserve the largest allowed raster plus a small allowance for page text
	// and metadata. Skip speculative work if visible pages leave no room for it.
	estimatedBytes := min(pdfPageMaxRasterBytes+(1<<20), int64(math.Ceil(pixels))*4+(1<<20))
	return pinnedBytes+estimatedBytes <= pdfPageCacheBudgetBytes
}

func pdfThumbnailInCacheWindow(index, firstVisible int) bool {
	start := max(0, firstVisible-pdfThumbnailCacheOverscan)
	end := firstVisible + pdfThumbnailVisiblePrefetch + pdfThumbnailCacheOverscan
	return index >= start && index < end
}

func (v *pdfViewState) thumbnailPrefetchCount() int {
	if v == nil {
		return pdfThumbnailVisiblePrefetch
	}
	return max(pdfThumbnailVisiblePrefetch, v.thumbList.Position.Count)
}

func (v *pdfViewState) thumbnailInCacheWindow(index, firstVisible int) bool {
	if v == nil {
		return pdfThumbnailInCacheWindow(index, firstVisible)
	}
	first := max(0, firstVisible)
	start := max(0, first-pdfThumbnailCacheOverscan)
	end := first + v.thumbnailPrefetchCount() + pdfThumbnailCacheOverscan
	return index >= start && index < end
}

func (v *pdfViewState) trimCacheWindow() {
	if v == nil {
		return
	}
	for index := range v.cache {
		if !v.pageInCacheWindow(index) {
			delete(v.cache, index)
		}
	}
}

func (v *pdfViewState) trimThumbnailCacheWindow() {
	if v == nil {
		return
	}
	for index := range v.thumbCache {
		if !v.thumbnailInCacheWindow(index, v.thumbList.Position.First) {
			delete(v.thumbCache, index)
		}
	}
	for index := range v.thumbErrors {
		if !v.thumbnailInCacheWindow(index, v.thumbList.Position.First) {
			delete(v.thumbErrors, index)
		}
	}
}

func pdfCanvasDPI(scale float64) float64 {
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return 72
	}
	// scale is already expressed in physical output pixels per PDF point.
	dpi := math.Max(24, scale*72)
	return math.Ceil(dpi/12) * 12
}

func pdfImageBytes(img image.Image) int64 {
	if img == nil {
		return 0
	}
	bounds := img.Bounds()
	return int64(bounds.Dx()) * int64(bounds.Dy()) * 4
}

func (result pdfPageResult) hasImage() bool {
	return result.imageOp.Size().X > 0 || result.pixels != nil
}

func (result *pdfPageResult) setImage(img image.Image) {
	if img == nil {
		return
	}
	result.imageOp = paint.NewImageOp(img)
	result.pixels = nil
}

func (result *pdfPageResult) prepareImageOp() {
	if result != nil && result.imageOp.Size().X == 0 && result.pixels != nil {
		result.setImage(result.pixels)
	}
}

func pdfImageOpBytes(op paint.ImageOp) int64 {
	size := op.Size()
	return int64(size.X) * int64(size.Y) * 4
}

func pdfPageResultBytes(result pdfPageResult) int64 {
	// Keep a small allowance for the page metadata and cached error text too.
	imageBytes := pdfImageOpBytes(result.imageOp)
	if imageBytes == 0 {
		imageBytes = pdfImageBytes(result.pixels)
	}
	bytes := imageBytes + 512
	for _, text := range result.page.Text {
		bytes += int64(len(text.Text)) + 64
	}
	return bytes
}

func (v *pdfViewState) cachePage(result pdfPageResult) {
	if !v.pageInCacheWindow(result.page.Index) {
		return
	}
	result.prepareImageOp()
	if v.cache == nil {
		v.cache = make(map[int]pdfPageResult)
	}
	v.cache[result.page.Index] = result
	first, last := v.pageCacheWindow()
	maxEntries := min(max(1, v.info.Pages), last-first+1)
	for len(v.cache) > maxEntries || pdfPageCacheBytes(v.cache) > pdfPageCacheBudgetBytes {
		visibleFirst := max(0, v.canvas.Position.First)
		visibleEnd := visibleFirst + max(1, v.canvas.Position.Count)
		victim := farthestPDFPage(v.cache, v.page, visibleFirst, visibleEnd)
		if victim < 0 {
			break
		}
		delete(v.cache, victim)
	}
}

func pdfPageCacheBytes(cache map[int]pdfPageResult) int64 {
	var total int64
	for _, result := range cache {
		total += pdfPageResultBytes(result)
	}
	return total
}

func farthestPDFPage(cache map[int]pdfPageResult, current, visibleFirst, visibleEnd int) int {
	victim, maxDistance := -1, -1
	for index := range cache {
		if index == current || (index >= visibleFirst && index < visibleEnd) {
			continue
		}
		distance := index - current
		if distance < 0 {
			distance = -distance
		}
		// Resolve equal-distance ties toward older pages so a forward
		// prefetch isn't nondeterministically evicted and requested again.
		if distance > maxDistance || (distance == maxDistance && victim >= 0 && index < victim) {
			victim, maxDistance = index, distance
		}
	}
	return victim
}

func (v *pdfViewState) cacheThumbnail(index int, imageOp paint.ImageOp) {
	if imageOp.Size().X == 0 || !v.thumbnailInCacheWindow(index, v.thumbList.Position.First) {
		return
	}
	if v.thumbCache == nil {
		v.thumbCache = make(map[int]paint.ImageOp)
	}
	v.thumbCache[index] = imageOp
	delete(v.thumbErrors, index)
	maxEntries := max(pdfThumbnailCacheMaxEntries, v.thumbnailPrefetchCount()+pdfThumbnailCacheOverscan*2)
	for len(v.thumbCache) > maxEntries || pdfThumbnailCacheBytes(v.thumbCache) > pdfThumbnailCacheBudgetBytes {
		victim := farthestPDFThumbnailInWindow(v.thumbCache, v.thumbList.Position.First, v.thumbnailPrefetchCount())
		if victim < 0 {
			break
		}
		delete(v.thumbCache, victim)
	}
}

func pdfThumbnailCacheBytes(cache map[int]paint.ImageOp) int64 {
	var total int64
	for _, imageOp := range cache {
		total += pdfImageOpBytes(imageOp)
	}
	return total
}

func farthestPDFThumbnail(cache map[int]paint.ImageOp, firstVisible int) int {
	return farthestPDFThumbnailInWindow(cache, firstVisible, pdfThumbnailVisiblePrefetch)
}

func farthestPDFThumbnailInWindow(cache map[int]paint.ImageOp, firstVisible, visibleCount int) int {
	start, end := max(0, firstVisible), max(0, firstVisible)+max(1, visibleCount)
	victim, maxDistance := -1, -1
	for index := range cache {
		distance := 0
		if index < start {
			distance = start - index
		} else if index >= end {
			distance = index - end + 1
		}
		if distance > maxDistance {
			victim, maxDistance = index, distance
		}
	}
	return victim
}

func (v *pdfViewState) rememberThumbnailError(index int, message string) {
	if !v.thumbnailInCacheWindow(index, v.thumbList.Position.First) {
		return
	}
	if v.thumbErrors == nil {
		v.thumbErrors = make(map[int]string)
	}
	v.thumbErrors[index] = message
	maxEntries := max(pdfThumbnailErrorMaxEntries, v.thumbnailPrefetchCount()+pdfThumbnailCacheOverscan*2)
	for len(v.thumbErrors) > maxEntries {
		for old := range v.thumbErrors {
			delete(v.thumbErrors, old)
			break
		}
	}
}
