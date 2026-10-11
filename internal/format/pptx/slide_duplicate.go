package pptx

import (
	"fmt"

	"glance/internal/document"

	"github.com/F31/go-pptx/v2/pptx"
)

// DuplicateSlideByID appends a restricted copy of the selected slide.
// The go-pptx clone operation preserves supported slide relationships and
// rejects unknown internal relationships before committing any package edits.
func (d *Document) DuplicateSlideByID(slideID uint32) (uint32, int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return 0, 0, fmt.Errorf("演示文稿已关闭")
	}

	slides, err := d.pres.Slides()
	if err != nil {
		return 0, 0, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	sourceIndex := -1
	for index, slide := range slides {
		if uint32(slide.ID()) == slideID {
			sourceIndex = index
			break
		}
	}
	if sourceIndex < 0 {
		return 0, 0, fmt.Errorf("未找到幻灯片 ID %d", slideID)
	}

	d.ensureSlideProjectionCacheCount(len(slides))
	duplicate, err := slides[sourceIndex].Clone(&pptx.ClonePolicy{})
	if err != nil {
		return 0, 0, fmt.Errorf("复制幻灯片失败: %w", err)
	}
	d.appendClonedSlideProjectionCaches(sourceIndex)
	if layoutPart := d.slideLayoutChanges[slideID]; layoutPart != "" {
		if d.slideLayoutChanges == nil {
			d.slideLayoutChanges = make(map[uint32]string)
		}
		d.slideLayoutChanges[uint32(duplicate.ID())] = layoutPart
	}
	if timing := d.slideAnimationChanges[slideID]; len(timing) > 0 {
		if d.slideAnimationChanges == nil {
			d.slideAnimationChanges = make(map[uint32][]byte)
		}
		d.slideAnimationChanges[uint32(duplicate.ID())] = append([]byte(nil), timing...)
	}

	newIndex := len(slides)
	d.activeSlide = newIndex
	d.dirty = true
	d.revision++
	return uint32(duplicate.ID()), newIndex + 1, nil
}

func (d *Document) appendClonedSlideProjectionCaches(sourceIndex int) {
	d.slideBackgrounds = append(d.slideBackgrounds, d.slideBackgrounds[sourceIndex])
	d.slideBackgroundChanges = append(d.slideBackgroundChanges, d.slideBackgroundChanges[sourceIndex])
	d.slideInherited = append(d.slideInherited, append([]document.PresentationElement(nil), d.slideInherited[sourceIndex]...))
	d.slidePlaceholderBounds = append(d.slidePlaceholderBounds, cloneSlideMap(d.slidePlaceholderBounds[sourceIndex]))
	d.slidePlaceholderTypes = append(d.slidePlaceholderTypes, cloneSlideMap(d.slidePlaceholderTypes[sourceIndex]))
	d.slidePlaceholderChanges = append(d.slidePlaceholderChanges, cloneSlideMap(d.slidePlaceholderChanges[sourceIndex]))
	d.slideShapeTransforms = append(d.slideShapeTransforms, cloneSlideMap(d.slideShapeTransforms[sourceIndex]))
	d.slidePlaceholderXfrms = append(d.slidePlaceholderXfrms, cloneSlideMap(d.slidePlaceholderXfrms[sourceIndex]))
	d.slideTransformChanges = append(d.slideTransformChanges, cloneSlideMap(d.slideTransformChanges[sourceIndex]))
	d.slidePictureCropStates = append(d.slidePictureCropStates, cloneSlideMap(d.slidePictureCropStates[sourceIndex]))
	d.slidePictureCropChanges = append(d.slidePictureCropChanges, cloneSlideMap(d.slidePictureCropChanges[sourceIndex]))

	styleChanges := make(map[uint32]document.PresentationElementStylePatch, len(d.slideStyleChanges[sourceIndex]))
	for shapeID, patch := range d.slideStyleChanges[sourceIndex] {
		styleChanges[shapeID] = clonePresentationStylePatch(patch)
	}
	d.slideStyleChanges = append(d.slideStyleChanges, styleChanges)

	imageChanges := make(map[uint32]presentationPictureImageOverride, len(d.slidePictureImageChanges[sourceIndex]))
	for shapeID, override := range d.slidePictureImageChanges[sourceIndex] {
		override.Data = append([]byte(nil), override.Data...)
		imageChanges[shapeID] = override
	}
	d.slidePictureImageChanges = append(d.slidePictureImageChanges, imageChanges)

	bodyProps := make(map[uint32]shapeBodyProps, len(d.slideBodyProps[sourceIndex]))
	for shapeID, props := range d.slideBodyProps[sourceIndex] {
		if props.fillGradient != nil {
			gradient := *props.fillGradient
			gradient.Stops = append([]document.PresentationGradientStop(nil), props.fillGradient.Stops...)
			props.fillGradient = &gradient
		}
		bodyProps[shapeID] = props
	}
	d.slideBodyProps[len(d.slideBackgrounds)-1] = bodyProps
	d.slidePictureTargets[len(d.slideBackgrounds)-1] = cloneSlideMap(d.slidePictureTargets[sourceIndex])
}

func cloneSlideMap[K comparable, V any](source map[K]V) map[K]V {
	if source == nil {
		return nil
	}
	copy := make(map[K]V, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}
