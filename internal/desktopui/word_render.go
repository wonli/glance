package desktopui

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"math/big"
	"strings"

	"glance/internal/document"
	xdraw "golang.org/x/image/draw"
)

const (
	wordPreviewPixelLimit = 32_000_000
	wordImageCacheLimit   = int64(64 << 20)
)

func fitWordImageSize(sourceWidth, sourceHeight, maxWidth, maxHeight int) image.Point {
	if sourceWidth <= 0 || sourceHeight <= 0 || maxWidth <= 0 || maxHeight <= 0 {
		return image.Point{}
	}
	scale := math.Min(1, math.Min(float64(maxWidth)/float64(sourceWidth), float64(maxHeight)/float64(sourceHeight)))
	return image.Pt(max(1, int(math.Round(float64(sourceWidth)*scale))), max(1, int(math.Round(float64(sourceHeight)*scale))))
}

func validateWordPreviewConfig(config image.Config, format string) error {
	format = strings.ToLower(format)
	if format != "png" && format != "jpeg" && format != "gif" {
		return fmt.Errorf("unsupported Word image preview format %q", format)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > wordPreviewPixelLimit/config.Height {
		return fmt.Errorf("Word image preview exceeds the %d-pixel limit", wordPreviewPixelLimit)
	}
	return nil
}

func decodeWordPreview(data []byte) (image.Image, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read image dimensions: %w", err)
	}
	if err := validateWordPreviewConfig(config, format); err != nil {
		return nil, err
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode Word image: %w", err)
	}
	if decodedFormat != format {
		return nil, errors.New("Word image format changed during decode")
	}
	if format == "gif" {
		if animated, ok := decoded.(*image.Paletted); ok {
			return animated, nil // image.Decode returns the first GIF frame.
		}
	}
	return decoded, nil
}

type wordImageCacheEntry struct {
	id    string
	image image.Image
	bytes int64
}

type wordImageCache struct {
	limit int64
	bytes int64
	items map[string]*list.Element
	lru   *list.List
}

func newWordImageCache(limit int64) *wordImageCache {
	if limit <= 0 {
		limit = wordImageCacheLimit
	}
	return &wordImageCache{limit: limit, items: make(map[string]*list.Element), lru: list.New()}
}

func (cache *wordImageCache) Get(id string) (image.Image, bool) {
	if cache == nil {
		return nil, false
	}
	item := cache.items[id]
	if item == nil {
		return nil, false
	}
	cache.lru.MoveToFront(item)
	return item.Value.(wordImageCacheEntry).image, true
}

func (cache *wordImageCache) Put(id string, value image.Image) bool {
	if cache == nil || id == "" || value == nil {
		return false
	}
	width, height := int64(value.Bounds().Dx()), int64(value.Bounds().Dy())
	if width <= 0 || height <= 0 || width > cache.limit/4/height {
		return false
	}
	size := width * height * 4
	if size > cache.limit {
		return false
	}
	cache.Delete(id)
	for cache.bytes+size > cache.limit {
		oldest := cache.lru.Back()
		if oldest == nil {
			break
		}
		cache.remove(oldest)
	}
	item := cache.lru.PushFront(wordImageCacheEntry{id: id, image: value, bytes: size})
	cache.items[id] = item
	cache.bytes += size
	return true
}

func (cache *wordImageCache) Delete(id string) {
	if cache == nil {
		return
	}
	if item := cache.items[id]; item != nil {
		cache.remove(item)
	}
}

func (cache *wordImageCache) Clear() {
	if cache == nil {
		return
	}
	cache.items = make(map[string]*list.Element)
	cache.lru.Init()
	cache.bytes = 0
}

func (cache *wordImageCache) Bytes() int64 {
	if cache == nil {
		return 0
	}
	return cache.bytes
}

func (cache *wordImageCache) remove(item *list.Element) {
	entry := item.Value.(wordImageCacheEntry)
	delete(cache.items, entry.id)
	cache.bytes -= entry.bytes
	cache.lru.Remove(item)
}

func wordImagePlaceholder(unsupported document.WordUnsupported) string {
	description := strings.TrimSpace(unsupported.Description)
	if description == "" {
		description = "无法预览此图片"
	}
	return "图片无法预览：" + description
}

type wordImageGestureMode uint8

const (
	wordImageGestureNone wordImageGestureMode = iota
	wordImageGestureMove
	wordImageGestureResize
)

type wordImagePreview struct {
	Active            bool
	ImageID           string
	SourceParagraph   string
	TargetParagraph   string
	AfterInlineID     string
	WidthEMU          int64
	HeightEMU         int64
	OriginalWidthEMU  int64
	OriginalHeightEMU int64
}

type wordImageGesture struct {
	mode        wordImageGestureMode
	preview     wordImagePreview
	released    bool
	commitReady bool
}

func (gesture *wordImageGesture) BeginMove(imageID, sourceParagraphID string) bool {
	gesture.reset()
	if imageID == "" || sourceParagraphID == "" {
		return false
	}
	gesture.mode = wordImageGestureMove
	gesture.preview = wordImagePreview{Active: true, ImageID: imageID, SourceParagraph: sourceParagraphID}
	return true
}

func (gesture *wordImageGesture) MovePreview(targetParagraphID, afterInlineID string) {
	if gesture.mode != wordImageGestureMove || !gesture.preview.Active {
		return
	}
	gesture.preview.TargetParagraph = targetParagraphID
	gesture.preview.AfterInlineID = afterInlineID
}

func (gesture *wordImageGesture) ReleaseMove() {
	if gesture.mode == wordImageGestureMove && gesture.preview.Active {
		gesture.released = true
		gesture.commitReady = gesture.preview.TargetParagraph != ""
	}
}

func (gesture *wordImageGesture) CommitMove() bool {
	if gesture.mode != wordImageGestureMove || !gesture.released || !gesture.commitReady {
		return false
	}
	gesture.reset()
	return true
}

func (gesture *wordImageGesture) BeginResize(imageID string, widthEMU, heightEMU int64) bool {
	gesture.reset()
	if imageID == "" || widthEMU <= 0 || heightEMU <= 0 {
		return false
	}
	gesture.mode = wordImageGestureResize
	gesture.preview = wordImagePreview{Active: true, ImageID: imageID, WidthEMU: widthEMU, HeightEMU: heightEMU, OriginalWidthEMU: widthEMU, OriginalHeightEMU: heightEMU}
	return true
}

func (gesture *wordImageGesture) ResizePreview(widthEMU int64) {
	if gesture.mode != wordImageGestureResize || !gesture.preview.Active || widthEMU <= 0 {
		return
	}
	height, err := proportionalWordImageHeightForUI(widthEMU, gesture.preview.OriginalWidthEMU, gesture.preview.OriginalHeightEMU)
	if err != nil || height <= 0 {
		return
	}
	gesture.preview.WidthEMU = widthEMU
	gesture.preview.HeightEMU = height
}

func (gesture *wordImageGesture) ReleaseResize() {
	if gesture.mode == wordImageGestureResize && gesture.preview.Active {
		gesture.released = true
		gesture.commitReady = gesture.preview.WidthEMU != gesture.preview.OriginalWidthEMU
	}
}

func (gesture *wordImageGesture) CommitResize() bool {
	if gesture.mode != wordImageGestureResize || !gesture.released || !gesture.commitReady {
		return false
	}
	gesture.reset()
	return true
}

func (gesture *wordImageGesture) Cancel() { gesture.reset() }

func (gesture *wordImageGesture) Preview() wordImagePreview { return gesture.preview }

func (gesture *wordImageGesture) reset() {
	gesture.mode = wordImageGestureNone
	gesture.preview = wordImagePreview{}
	gesture.released = false
	gesture.commitReady = false
}

func proportionalWordImageHeightForUI(width, originalWidth, originalHeight int64) (int64, error) {
	if width <= 0 || originalWidth <= 0 || originalHeight <= 0 {
		return 0, errors.New("invalid image extent")
	}
	numerator := new(big.Int).Mul(big.NewInt(width), big.NewInt(originalHeight))
	numerator.Add(numerator, big.NewInt(originalWidth/2))
	result := numerator.Quo(numerator, big.NewInt(originalWidth))
	if !result.IsInt64() {
		return 0, errors.New("image height overflows")
	}
	return result.Int64(), nil
}

type wordImageDecodeResult struct {
	state   *documentSessionState
	id      string
	version uint64
	image   image.Image
	err     error
}

func compactWordPreview(source image.Image) image.Image {
	if source == nil {
		return nil
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil
	}
	const maxSide = 4096
	if width <= maxSide && height <= maxSide {
		return source
	}
	scale := math.Min(float64(maxSide)/float64(width), float64(maxSide)/float64(height))
	destWidth := max(1, int(math.Round(float64(width)*scale)))
	destHeight := max(1, int(math.Round(float64(height)*scale)))
	destination := image.NewNRGBA(image.Rect(0, 0, destWidth, destHeight))
	xdraw.CatmullRom.Scale(destination, destination.Bounds(), source, bounds, xdraw.Over, nil)
	return destination
}
