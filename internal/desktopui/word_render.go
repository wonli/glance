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

	"gioui.org/f32"
	"glance/internal/document"
	xdraw "golang.org/x/image/draw"
)

const (
	wordPreviewPixelLimit      = 32_000_000
	wordImageCacheLimit        = int64(64 << 20)
	wordImageDecodeConcurrency = 2
)

func fitWordImageSize(sourceWidth, sourceHeight, maxWidth, maxHeight int) image.Point {
	if sourceWidth <= 0 || sourceHeight <= 0 || maxWidth <= 0 || maxHeight <= 0 {
		return image.Point{}
	}
	scale := math.Min(1, math.Min(float64(maxWidth)/float64(sourceWidth), float64(maxHeight)/float64(sourceHeight)))
	return image.Pt(max(1, int(math.Round(float64(sourceWidth)*scale))), max(1, int(math.Round(float64(sourceHeight)*scale))))
}

func rotatedWordImageBounds(size image.Point, degrees float64) image.Point {
	if size.X <= 0 || size.Y <= 0 {
		return image.Point{}
	}
	angle := math.Mod(degrees, 360)
	if angle < 0 {
		angle += 360
	}
	if math.Abs(angle-90) < 1e-9 || math.Abs(angle-270) < 1e-9 {
		return image.Pt(size.Y, size.X)
	}
	if math.Abs(angle) < 1e-9 || math.Abs(angle-180) < 1e-9 {
		return size
	}
	radians := angle * math.Pi / 180
	width := math.Abs(math.Cos(radians))*float64(size.X) + math.Abs(math.Sin(radians))*float64(size.Y)
	height := math.Abs(math.Sin(radians))*float64(size.X) + math.Abs(math.Cos(radians))*float64(size.Y)
	return image.Pt(max(1, int(math.Ceil(width-1e-9))), max(1, int(math.Ceil(height-1e-9))))
}

// fitRotatedWordImageSize returns the unrotated image size whose rotated bounds fit the viewport.
func fitRotatedWordImageSize(sourceWidth, sourceHeight, maxWidth, maxHeight int, degrees float64) image.Point {
	if sourceWidth <= 0 || sourceHeight <= 0 || maxWidth <= 0 || maxHeight <= 0 {
		return image.Point{}
	}
	angle := math.Mod(degrees, 360) * math.Pi / 180
	cosine, sine := math.Abs(math.Cos(angle)), math.Abs(math.Sin(angle))
	boundWidth := cosine*float64(sourceWidth) + sine*float64(sourceHeight)
	boundHeight := sine*float64(sourceWidth) + cosine*float64(sourceHeight)
	scale := math.Min(1, math.Min(float64(maxWidth)/boundWidth, float64(maxHeight)/boundHeight))
	for {
		size := image.Pt(max(1, int(math.Floor(float64(sourceWidth)*scale))), max(1, int(math.Floor(float64(sourceHeight)*scale))))
		bounds := rotatedWordImageBounds(size, degrees)
		if bounds.X <= maxWidth && bounds.Y <= maxHeight {
			return size
		}
		scale *= 0.99
	}
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
	wordImageGestureRotate
)

type wordImagePreview struct {
	Active                  bool
	ImageID                 string
	SourceParagraph         string
	TargetParagraph         string
	AfterInlineID           string
	WidthEMU                int64
	HeightEMU               int64
	OriginalWidthEMU        int64
	OriginalHeightEMU       int64
	RotationDegrees         float64
	OriginalRotationDegrees float64
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

func (gesture *wordImageGesture) BeginRotate(imageID string, degrees float64) bool {
	gesture.reset()
	if imageID == "" || math.IsNaN(degrees) || math.IsInf(degrees, 0) {
		return false
	}
	gesture.mode = wordImageGestureRotate
	gesture.preview = wordImagePreview{Active: true, ImageID: imageID, RotationDegrees: degrees, OriginalRotationDegrees: degrees}
	return true
}

func (gesture *wordImageGesture) RotatePreview(degrees float64) {
	if gesture.mode != wordImageGestureRotate || !gesture.preview.Active || math.IsNaN(degrees) || math.IsInf(degrees, 0) {
		return
	}
	gesture.preview.RotationDegrees = normalizeWordRotationForUI(degrees)
}

func (gesture *wordImageGesture) ReleaseRotate() {
	if gesture.mode == wordImageGestureRotate && gesture.preview.Active {
		gesture.released = true
		gesture.commitReady = math.Abs(shortestWordRotationDelta(gesture.preview.RotationDegrees, gesture.preview.OriginalRotationDegrees)) > 1e-6
	}
}

func (gesture *wordImageGesture) CommitRotate() bool {
	if gesture.mode != wordImageGestureRotate || !gesture.released || !gesture.commitReady {
		return false
	}
	gesture.reset()
	return true
}

type wordImageResizeHandle struct {
	name string
	x    int
	y    int
}

var wordImageResizeHandles = [8]wordImageResizeHandle{
	{name: "左上", x: -1, y: -1},
	{name: "上", y: -1},
	{name: "右上", x: 1, y: -1},
	{name: "右", x: 1},
	{name: "右下", x: 1, y: 1},
	{name: "下", y: 1},
	{name: "左下", x: -1, y: 1},
	{name: "左", x: -1},
}

func wordImageResizeWidthForHandle(widthEMU int64, frame image.Point, handleIndex int, drag f32.Point) int64 {
	if widthEMU <= 0 || frame.X <= 0 || frame.Y <= 0 || handleIndex < 0 || handleIndex >= len(wordImageResizeHandles) {
		return widthEMU
	}
	handle := wordImageResizeHandles[handleIndex]
	var scaleX, scaleY float64
	if handle.x != 0 {
		scaleX = (float64(frame.X) + float64(handle.x)*float64(drag.X)) / float64(frame.X)
	}
	if handle.y != 0 {
		scaleY = (float64(frame.Y) + float64(handle.y)*float64(drag.Y)) / float64(frame.Y)
	}
	var scale float64
	switch {
	case handle.x == 0:
		scale = scaleY
	case handle.y == 0:
		scale = scaleX
	default:
		// Follow the stronger axis. This keeps a purely horizontal or vertical
		// corner drag responsive while diagonal drags still combine naturally.
		if math.Abs(scaleX-1) >= math.Abs(scaleY-1) {
			scale = scaleX
		} else {
			scale = scaleY
		}
	}
	scale = math.Max(0.05, scale)
	result := math.Round(float64(widthEMU) * scale)
	if result >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return max(int64(1), int64(result))
}

func normalizeWordRotationForUI(degrees float64) float64 {
	degrees = math.Mod(degrees, 360)
	if degrees < 0 {
		degrees += 360
	}
	if math.Abs(degrees) < 1e-9 || math.Abs(degrees-360) < 1e-9 {
		return 0
	}
	return degrees
}

func shortestWordRotationDelta(from, to float64) float64 {
	delta := math.Mod(from-to+180, 360)
	if delta < 0 {
		delta += 360
	}
	return delta - 180
}

func wordImageRotationForPointer(startDegrees float64, center, start, current f32.Point) float64 {
	startAngle := math.Atan2(float64(start.Y-center.Y), float64(start.X-center.X))
	currentAngle := math.Atan2(float64(current.Y-center.Y), float64(current.X-center.X))
	delta := (currentAngle - startAngle) * 180 / math.Pi
	return normalizeWordRotationForUI(startDegrees + delta)
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
	state      *documentSessionState
	id         string
	version    uint64
	image      image.Image
	err        error
	decodeSlot bool
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
