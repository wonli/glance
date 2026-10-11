package pptx

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	"math"
	"path"
	"strconv"
	"strings"

	gopptx "github.com/F31/go-pptx/v2/pptx"
	"glance/internal/document"
)

const (
	maxPresentationPictureBytes  = 128 << 20
	maxPresentationPictureEdge   = 32768
	maxPresentationPicturePixels = 64 << 20
)

type presentationPictureCropState struct {
	Crop  document.PresentationCrop
	Valid bool
}

type presentationPictureImageOverride struct {
	Data  []byte
	Image image.Image
}

func validatePresentationCrop(crop document.PresentationCrop) error {
	for _, value := range []int{crop.Left, crop.Top, crop.Right, crop.Bottom} {
		if value < 0 || value > 100000 {
			return fmt.Errorf("图片裁剪值必须在 0..100000 范围内")
		}
	}
	if crop.Left+crop.Right >= 100000 || crop.Top+crop.Bottom >= 100000 {
		return fmt.Errorf("图片裁剪后必须保留可见内容")
	}
	return nil
}

func validatePresentationPictureData(data []byte, mediaType string) (image.Image, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("图片数据为空")
	}
	if len(data) > maxPresentationPictureBytes {
		return nil, fmt.Errorf("图片文件超过 %d MiB", maxPresentationPictureBytes>>20)
	}
	wantFormat := ""
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/png":
		wantFormat = "png"
	case "image/jpeg":
		wantFormat = "jpeg"
	default:
		return nil, fmt.Errorf("仅支持 PNG/JPEG 图片")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("图片文件损坏: %w", err)
	}
	if format != wantFormat {
		return nil, fmt.Errorf("图片扩展名与实际 %s 格式不匹配", format)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxPresentationPictureEdge || config.Height > maxPresentationPictureEdge || uint64(config.Width)*uint64(config.Height) > maxPresentationPicturePixels {
		return nil, fmt.Errorf("图片尺寸超过安全上限")
	}
	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("图片文件损坏: %w", err)
	}
	if format != wantFormat || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return nil, fmt.Errorf("图片解码信息不一致")
	}
	return decoded, nil
}

func pictureMediaTypeForPart(part string) string {
	switch strings.ToLower(path.Ext(part)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return ""
	}
}

func parseSlidePictureCropStates(data []byte) map[uint32]presentationPictureCropState {
	var root pptxXMLNode
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil
	}
	tree := shapeTree(root)
	if tree == nil {
		return nil
	}
	result := make(map[uint32]presentationPictureCropState)
	for _, shape := range tree.Children {
		if shape.XMLName.Local != "pic" {
			continue
		}
		id := shapeIDFromXML(shape)
		if id == 0 {
			continue
		}
		state := presentationPictureCropState{Valid: true}
		fill := shape.child("blipFill")
		if fill == nil {
			state.Valid = false
		} else if srcRect := fill.child("srcRect"); srcRect != nil {
			values := []*int{&state.Crop.Left, &state.Crop.Top, &state.Crop.Right, &state.Crop.Bottom}
			for index, name := range []string{"l", "t", "r", "b"} {
				value := srcRect.attr(name)
				if value == "" {
					continue
				}
				parsed, err := strconv.Atoi(value)
				if err != nil {
					state.Valid = false
					break
				}
				*values[index] = parsed
			}
			if state.Valid && validatePresentationCrop(state.Crop) != nil {
				state.Valid = false
			}
		}
		result[id] = state
	}
	return result
}

func (d *Document) pictureCanEditLocked(slideIndex int, shapeID uint32) bool {
	targets, ok := d.slidePictureTargets[slideIndex]
	if !ok {
		return false
	}
	target, ok := targets[shapeID]
	if !ok || pictureMediaTypeForPart(target) == "" {
		return false
	}
	if slideIndex >= len(d.slidePictureCropStates) {
		return false
	}
	cropState, ok := d.slidePictureCropStates[slideIndex][shapeID]
	if !ok || !cropState.Valid {
		return false
	}
	if changes := d.slidePictureCropChanges; slideIndex < len(changes) {
		if crop, changed := changes[slideIndex][shapeID]; changed && validatePresentationCrop(crop) != nil {
			return false
		}
	}
	if overrides := d.slidePictureImageChanges; slideIndex < len(overrides) {
		if override, changed := overrides[slideIndex][shapeID]; changed {
			return len(override.Data) > 0 && override.Image != nil
		}
	}
	return len(d.mediaBytes[target]) > 0 && d.mediaImages[target] != nil
}

func (d *Document) findSlidePictureLocked(slideIndex int, shapeID int64) (*gopptx.PictureShape, error) {
	if shapeID <= 0 || uint64(shapeID) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("形状 ID 无效: %d", shapeID)
	}
	slides, err := d.pres.Slides()
	if err != nil || slideIndex < 0 || slideIndex >= len(slides) {
		return nil, fmt.Errorf("幻灯片序号无效: %d", slideIndex)
	}
	shapes, err := slides[slideIndex].Shapes()
	if err != nil {
		return nil, fmt.Errorf("读取幻灯片形状失败: %w", err)
	}
	var selected *gopptx.PictureShape
	count := 0
	for _, shape := range shapes {
		if int64(shape.ID()) != shapeID {
			continue
		}
		count++
		if picture, ok := shape.(*gopptx.PictureShape); ok {
			selected = picture
		}
	}
	if count != 1 || selected == nil {
		return nil, fmt.Errorf("幻灯片 %d 上的形状 ID %d 不是唯一的可编辑图片", slideIndex, shapeID)
	}
	if !d.pictureCanEditLocked(slideIndex, uint32(shapeID)) {
		return nil, fmt.Errorf("幻灯片 %d 上的图片 ID %d 不支持替换或裁剪", slideIndex, shapeID)
	}
	return selected, nil
}

// ReplaceSlidePicture replaces a supported slide-local picture while preserving its identity and frame.
func (d *Document) ReplaceSlidePicture(slideIndex int, shapeID int64, mediaType string, data []byte) error {
	decoded, err := validatePresentationPictureData(data, mediaType)
	if err != nil {
		return err
	}
	imageCopy := append([]byte(nil), data...)
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err = d.replaceSlidePictureLocked(slideIndex, shapeID, mediaType, imageCopy, decoded)
	return err
}

// ReplaceSlidePictureByID replaces a slide-local picture using stable IDs.
func (d *Document) ReplaceSlidePictureByID(slideID uint32, shapeID int64, mediaType string, data []byte) (bool, error) {
	decoded, err := validatePresentationPictureData(data, mediaType)
	if err != nil {
		return false, err
	}
	imageCopy := append([]byte(nil), data...)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return false, fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return false, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for slideIndex, slide := range slides {
		if uint32(slide.ID()) == slideID {
			return d.replaceSlidePictureLocked(slideIndex, shapeID, mediaType, imageCopy, decoded)
		}
	}
	return false, fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

func (d *Document) replaceSlidePictureLocked(slideIndex int, shapeID int64, mediaType string, imageCopy []byte, decoded image.Image) (bool, error) {
	if d.pres == nil {
		return false, fmt.Errorf("演示文稿已关闭")
	}
	picture, err := d.findSlidePictureLocked(slideIndex, shapeID)
	if err != nil {
		return false, err
	}
	elem := d.projectShape(picture, slideIndex)
	if bytes.Equal(elem.ImageBytes, imageCopy) {
		return false, nil
	}
	if err := picture.ReplaceImage(context.Background(), gopptx.BytesMedia(imageCopy, mediaType)); err != nil {
		return false, fmt.Errorf("替换图片失败: %w", err)
	}
	slides, _ := d.pres.Slides()
	d.ensureSlideProjectionCacheCount(len(slides))
	id := uint32(shapeID)
	d.slidePictureImageChanges[slideIndex][id] = presentationPictureImageOverride{Data: imageCopy, Image: decoded}
	d.slidePictureCropStates[slideIndex][id] = presentationPictureCropState{Valid: true}
	delete(d.slidePictureCropChanges[slideIndex], id)
	d.dirty = true
	d.revision++
	return true, nil
}

// AddSlidePictureByID inserts a PNG/JPEG into an explicitly identified slide.
// Geometry is expressed in points and fit is one of stretch, contain or cover.
func (d *Document) AddSlidePictureByID(slideID uint32, bounds document.PresentationRect, mediaType string, data []byte, fit, altText string, decorative bool) (int64, document.PresentationRect, error) {
	decoded, err := validatePresentationPictureData(data, mediaType)
	if err != nil {
		return 0, document.PresentationRect{}, err
	}
	if err := validatePresentationTransform(bounds, 0); err != nil {
		return 0, document.PresentationRect{}, err
	}
	if len([]rune(altText)) > 1000 || (decorative && altText != "") {
		return 0, document.PresentationRect{}, fmt.Errorf("图片替代文本最多 1000 个字符，装饰性图片不能同时设置替代文本")
	}
	var fitMode gopptx.PictureFitMode
	switch fit {
	case "stretch":
		fitMode = gopptx.FitStretch
	case "contain":
		fitMode = gopptx.FitContain
	case "cover":
		fitMode = gopptx.FitCover
	default:
		return 0, document.PresentationRect{}, fmt.Errorf("不支持的图片适配方式: %q", fit)
	}
	imageCopy := append([]byte(nil), data...)

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return 0, document.PresentationRect{}, fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return 0, document.PresentationRect{}, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for index, slide := range slides {
		if uint32(slide.ID()) != slideID {
			continue
		}
		picture, err := slide.AddPicture(context.Background(), gopptx.BytesMedia(imageCopy, mediaType), gopptx.PictureSpec{
			X: pointToEMU(bounds.X), Y: pointToEMU(bounds.Y),
			Width: pointToEMU(bounds.W), Height: pointToEMU(bounds.H), Fit: fitMode,
			AltText: altText, IsDecorative: decorative,
		})
		if err != nil {
			return 0, document.PresentationRect{}, fmt.Errorf("插入图片失败: %w", err)
		}
		d.ensureSlideProjectionCacheCount(len(slides))
		id := uint32(picture.ID())
		if d.slidePictureTargets[index] == nil {
			d.slidePictureTargets[index] = make(map[uint32]string)
		}
		extension := ".png"
		if mediaType == "image/jpeg" {
			extension = ".jpg"
		}
		// The target path is used only to classify this live, newly inserted
		// picture. Its bytes and decoded image are held in the override until
		// the next package reopen rebuilds the real relationship map.
		d.slidePictureTargets[index][id] = fmt.Sprintf("ppt/media/live-%d%s", id, extension)
		d.slidePictureImageChanges[index][id] = presentationPictureImageOverride{Data: imageCopy, Image: decoded}
		d.slidePictureCropStates[index][id] = presentationPictureCropState{Valid: true}
		d.activeSlide = index
		d.dirty = true
		d.revision++
		return int64(id), d.projectShape(picture, index).Bounds, nil
	}
	return 0, document.PresentationRect{}, fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

// UpdateSlidePictureCrop stores a slide-local crop override for a supported picture.
func (d *Document) UpdateSlidePictureCrop(slideIndex int, shapeID int64, crop document.PresentationCrop) error {
	if err := validatePresentationCrop(crop); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.updateSlidePictureCropLocked(slideIndex, shapeID, crop)
	return err
}

// UpdateSlidePictureCropByID stores a crop using stable slide and element IDs.
func (d *Document) UpdateSlidePictureCropByID(slideID uint32, shapeID int64, crop document.PresentationCrop) (bool, error) {
	if err := validatePresentationCrop(crop); err != nil {
		return false, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return false, fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return false, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for slideIndex, slide := range slides {
		if uint32(slide.ID()) == slideID {
			return d.updateSlidePictureCropLocked(slideIndex, shapeID, crop)
		}
	}
	return false, fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

func (d *Document) updateSlidePictureCropLocked(slideIndex int, shapeID int64, crop document.PresentationCrop) (bool, error) {
	if d.pres == nil {
		return false, fmt.Errorf("演示文稿已关闭")
	}
	if _, err := d.findSlidePictureLocked(slideIndex, shapeID); err != nil {
		return false, err
	}
	var current document.PresentationCrop
	if slideIndex < len(d.slidePictureCropStates) {
		current = d.slidePictureCropStates[slideIndex][uint32(shapeID)].Crop
	}
	if slideIndex < len(d.slidePictureCropChanges) {
		if changed, ok := d.slidePictureCropChanges[slideIndex][uint32(shapeID)]; ok {
			current = changed
		}
	}
	if current == crop {
		return false, nil
	}
	slides, _ := d.pres.Slides()
	d.ensureSlideProjectionCacheCount(len(slides))
	d.slidePictureCropChanges[slideIndex][uint32(shapeID)] = crop
	d.dirty = true
	d.revision++
	return true, nil
}

func (d *Document) presentationSaveExpectationsLocked() (map[int]map[uint32]document.PresentationElement, error) {
	slides, err := d.pres.Slides()
	if err != nil {
		return nil, fmt.Errorf("读取待保存幻灯片失败: %w", err)
	}
	expectations := make(map[int]map[uint32]document.PresentationElement)
	slideCount := max(len(d.slideTransformChanges), len(d.slidePictureCropChanges), len(d.slidePictureImageChanges), len(d.slideStyleChanges))
	for slideIndex := 0; slideIndex < slideCount; slideIndex++ {
		ids := make(map[uint32]bool)
		if slideIndex < len(d.slideTransformChanges) {
			for id := range d.slideTransformChanges[slideIndex] {
				ids[id] = true
			}
		}
		if slideIndex < len(d.slidePictureCropChanges) {
			for id := range d.slidePictureCropChanges[slideIndex] {
				ids[id] = true
			}
		}
		if slideIndex < len(d.slidePictureImageChanges) {
			for id := range d.slidePictureImageChanges[slideIndex] {
				ids[id] = true
			}
		}
		if slideIndex < len(d.slideStyleChanges) {
			for id := range d.slideStyleChanges[slideIndex] {
				ids[id] = true
			}
		}
		if len(ids) == 0 {
			continue
		}
		if slideIndex < 0 || slideIndex >= len(slides) {
			return nil, fmt.Errorf("待保存对象的幻灯片序号无效: %d", slideIndex)
		}
		shapes, err := slides[slideIndex].Shapes()
		if err != nil {
			return nil, fmt.Errorf("读取幻灯片 %d 对象失败: %w", slideIndex, err)
		}
		for id := range ids {
			var selected gopptx.Shape
			count := 0
			for _, shape := range shapes {
				if uint32(shape.ID()) == id {
					selected = shape
					count++
				}
			}
			if count != 1 {
				return nil, fmt.Errorf("待保存的形状 ID %d 不唯一或不存在", id)
			}
			if expectations[slideIndex] == nil {
				expectations[slideIndex] = make(map[uint32]document.PresentationElement)
			}
			expectations[slideIndex][id] = d.projectShape(selected, slideIndex)
		}
	}
	return expectations, nil
}

func validateSavedPresentationEdits(path string, expected map[int]map[uint32]document.PresentationElement, styles []map[uint32]document.PresentationElementStylePatch, backgrounds []presentationBackgroundOverride, slideSize presentationSlideSizeOverride) error {
	if len(expected) == 0 && !hasPresentationBackgroundOverrides(backgrounds) && !slideSize.Set {
		return nil
	}
	reopened, err := Open(path)
	if err != nil {
		return fmt.Errorf("重开已更新演示文稿失败: %w", err)
	}
	defer reopened.Close()
	snapshot := reopened.Snapshot()
	if slideSize.Set {
		wantWidthPT := float64(slideSize.WidthEMU) / emuPerPoint
		wantHeightPT := float64(slideSize.HeightEMU) / emuPerPoint
		if math.Abs(snapshot.WidthPT-wantWidthPT) > 0.5/emuPerPoint+1e-9 || math.Abs(snapshot.HeightPT-wantHeightPT) > 0.5/emuPerPoint+1e-9 {
			return fmt.Errorf("重开后演示文稿尺寸与请求不一致")
		}
	}
	for slideIndex, elements := range expected {
		if slideIndex < 0 || slideIndex >= len(snapshot.Slides) {
			return fmt.Errorf("重开后缺少幻灯片 %d", slideIndex)
		}
		for id, want := range elements {
			var actual *document.PresentationElement
			count := 0
			for elementIndex := range snapshot.Slides[slideIndex].Elements {
				if uint32(snapshot.Slides[slideIndex].Elements[elementIndex].ID) == id {
					actual = &snapshot.Slides[slideIndex].Elements[elementIndex]
					count++
				}
			}
			if count != 1 || actual == nil {
				return fmt.Errorf("重开后形状 ID %d 不唯一或缺失", id)
			}
			rotationDelta := math.Abs(normalizePresentationRotation(actual.RotationDeg) - normalizePresentationRotation(want.RotationDeg))
			rotationDelta = math.Min(rotationDelta, 360-rotationDelta)
			if actual.Kind != want.Kind || !samePresentationRect(actual.Bounds, want.Bounds) || rotationDelta > 0.5/60000+1e-9 {
				return fmt.Errorf("重开后形状 ID %d 的类型或几何与请求不一致", id)
			}
			if want.Kind == document.ElementKindPicture && (actual.Crop != want.Crop || actual.AltText != want.AltText || !bytes.Equal(actual.ImageBytes, want.ImageBytes)) {
				return fmt.Errorf("重开后图片 ID %d 的裁剪、媒体字节或替代文本与请求不一致", id)
			}
			if slideIndex < len(styles) {
				if patch, ok := styles[slideIndex][id]; ok {
					if err := validateProjectedPresentationStyle(*actual, patch); err != nil {
						return fmt.Errorf("重开后形状 ID %d 的样式与请求不一致: %w", id, err)
					}
				}
			}
		}
	}
	for slideIndex, background := range backgrounds {
		if !background.Set {
			continue
		}
		if slideIndex < 0 || slideIndex >= len(snapshot.Slides) {
			return fmt.Errorf("重开后缺少设置背景的幻灯片 %d", slideIndex)
		}
		if normalizedRGB(snapshot.Slides[slideIndex].BgColorHex) != normalizedRGB(background.ColorHex) || snapshot.Slides[slideIndex].BackgroundImage != nil {
			return fmt.Errorf("重开后幻灯片 %d 的背景颜色与请求不一致", slideIndex)
		}
	}
	return nil
}

func validateProjectedPresentationStyle(actual document.PresentationElement, patch document.PresentationElementStylePatch) error {
	if patch.FillColorHex != nil {
		if actual.FillColorHex != *patch.FillColorHex || actual.FillGradient != nil {
			return fmt.Errorf("填充颜色或类型不同")
		}
	}
	if patch.BorderColor != nil && actual.BorderColor != *patch.BorderColor {
		return fmt.Errorf("轮廓颜色不同")
	}
	if patch.BorderWidthPT != nil && math.Abs(actual.BorderWidth-*patch.BorderWidthPT) > 0.5/emuPerPoint+1e-9 {
		return fmt.Errorf("轮廓宽度不同")
	}
	if patch.ParagraphAlignment != nil || patch.Bullet != nil || patch.LineSpacing != nil || patch.SpaceBefore != nil || patch.SpaceAfter != nil {
		if len(actual.Paragraphs) == 0 {
			return fmt.Errorf("缺少文字段落")
		}
		for _, paragraph := range actual.Paragraphs {
			if patch.ParagraphAlignment != nil && paragraph.Align != *patch.ParagraphAlignment {
				return fmt.Errorf("段落对齐方式不同")
			}
			if patch.Bullet != nil && paragraph.Bullet != *patch.Bullet {
				return fmt.Errorf("项目符号状态不同")
			}
			if patch.LineSpacing != nil && !samePresentationSpacing(paragraph.LineSpacing, patch.LineSpacing) {
				return fmt.Errorf("行距不同")
			}
			if patch.SpaceBefore != nil && !samePresentationSpacing(paragraph.SpaceBefore, patch.SpaceBefore) {
				return fmt.Errorf("段前距不同")
			}
			if patch.SpaceAfter != nil && !samePresentationSpacing(paragraph.SpaceAfter, patch.SpaceAfter) {
				return fmt.Errorf("段后距不同")
			}
		}
	}
	if patch.FontFamily != nil || patch.FontSizePT != nil || patch.Bold != nil || patch.Italic != nil || patch.Underline != nil || patch.TextColorHex != nil {
		if len(actual.Paragraphs) == 0 {
			return fmt.Errorf("缺少文字段落")
		}
		for _, paragraph := range actual.Paragraphs {
			for _, run := range paragraph.Runs {
				if patch.FontFamily != nil && run.Font != *patch.FontFamily {
					return fmt.Errorf("字体不同")
				}
				if patch.FontSizePT != nil && math.Abs(run.SizePT-*patch.FontSizePT) > 0.005 {
					return fmt.Errorf("字号不同")
				}
				if patch.Bold != nil && run.Bold != *patch.Bold {
					return fmt.Errorf("粗体不同")
				}
				if patch.Italic != nil && run.Italic != *patch.Italic {
					return fmt.Errorf("斜体不同")
				}
				if patch.Underline != nil && run.Underline != *patch.Underline {
					return fmt.Errorf("下划线不同")
				}
				if patch.TextColorHex != nil && run.ColorHex != *patch.TextColorHex {
					return fmt.Errorf("文字颜色不同")
				}
			}
		}
	}
	return nil
}

func samePresentationSpacing(actual, expected *document.PresentationSpacing) bool {
	if actual == nil || expected == nil {
		return actual == nil && expected == nil
	}
	return actual.Kind == expected.Kind && math.Abs(actual.Value-expected.Value) <= 0.00001
}
