package pptx

import (
	"fmt"
	"glance/internal/document"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

// AddSlide adds a new slide with the default layout.
func (d *Document) AddSlide() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	existingSlides, err := d.pres.Slides()
	if err != nil {
		return fmt.Errorf("读取幻灯片失败: %w", err)
	}

	layouts, err := d.pres.Layouts()
	if err != nil || len(layouts) == 0 {
		return fmt.Errorf("无可用版式")
	}

	slide, err := d.pres.AddSlide(layouts[0])
	if err != nil {
		return fmt.Errorf("新建幻灯片失败: %w", err)
	}

	// Add default title and content placeholders
	title, err := slide.AddTextBox(gopptx.TextBoxSpec{
		X:      914400,
		Y:      731520,
		Width:  10363200,
		Height: 1097280,
		Text:   "",
	})
	if err != nil {
		return fmt.Errorf("创建标题占位符失败: %w", err)
	}
	body, err := slide.AddTextBox(gopptx.TextBoxSpec{
		X:      914400,
		Y:      2011680,
		Width:  10363200,
		Height: 4114800,
		Text:   "",
	})
	if err != nil {
		return fmt.Errorf("创建正文占位符失败: %w", err)
	}

	d.ensureSlideProjectionCacheCount(len(existingSlides))
	d.slideBackgrounds = append(d.slideBackgrounds, d.defaultSlideBackground)
	d.slideBackgroundChanges = append(d.slideBackgroundChanges, presentationBackgroundOverride{})
	d.slideInherited = append(d.slideInherited, append([]document.PresentationElement(nil), d.defaultSlideInherited...))
	d.slidePlaceholderBounds = append(d.slidePlaceholderBounds, make(map[uint32]document.PresentationRect))
	d.slidePlaceholderTypes = append(d.slidePlaceholderTypes, map[uint32]string{
		uint32(title.ID()): "title",
		uint32(body.ID()):  "body",
	})
	d.slidePlaceholderChanges = append(d.slidePlaceholderChanges, map[uint32]string{
		uint32(title.ID()): "title",
		uint32(body.ID()):  "body",
	})
	d.slideShapeTransforms = append(d.slideShapeTransforms, make(map[uint32]presentationTransformMetadata))
	d.slidePlaceholderXfrms = append(d.slidePlaceholderXfrms, make(map[uint32]presentationTransformMetadata))
	d.slideTransformChanges = append(d.slideTransformChanges, make(map[uint32]presentationTransformOverride))
	d.slideStyleChanges = append(d.slideStyleChanges, make(map[uint32]document.PresentationElementStylePatch))
	d.slidePictureCropStates = append(d.slidePictureCropStates, make(map[uint32]presentationPictureCropState))
	d.slidePictureCropChanges = append(d.slidePictureCropChanges, make(map[uint32]document.PresentationCrop))
	d.slidePictureImageChanges = append(d.slidePictureImageChanges, make(map[uint32]presentationPictureImageOverride))
	slides, _ := d.pres.Slides()
	d.activeSlide = len(slides) - 1
	d.dirty = true
	d.revision++
	return nil
}

// DeleteSlide removes the slide at the given index.
func (d *Document) DeleteSlide(index int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.deleteSlideLocked(index)
}

// DeleteSlideByID resolves a stable slide ID before deleting the slide.
func (d *Document) DeleteSlideByID(slideID uint32) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for index, slide := range slides {
		if uint32(slide.ID()) == slideID {
			return d.deleteSlideLocked(index)
		}
	}
	return fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

func (d *Document) deleteSlideLocked(index int) error {
	if d.pres == nil {
		return fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil || index < 0 || index >= len(slides) {
		return fmt.Errorf("幻灯片序号无效: %d", index)
	}
	if len(slides) <= 1 {
		return fmt.Errorf("无法删除演示文稿中的最后一页幻灯片")
	}

	target := slides[index]
	if err := d.pres.RemoveSlide(target.ID()); err != nil {
		return fmt.Errorf("删除幻灯片失败: %w", err)
	}
	delete(d.slideLayoutChanges, uint32(target.ID()))
	delete(d.slideAnimationChanges, uint32(target.ID()))
	d.removeSlideProjectionCaches(index, len(slides))
	newCount := len(slides) - 1
	if d.activeSlide > index {
		d.activeSlide--
	} else if d.activeSlide == index && index >= newCount {
		d.activeSlide = newCount - 1
	}
	if d.activeSlide < 0 || d.activeSlide >= newCount {
		d.activeSlide = 0
	}
	d.dirty = true
	d.revision++
	return nil
}

func (d *Document) ensureSlideProjectionCacheCount(count int) {
	for len(d.slideBackgrounds) < count {
		d.slideBackgrounds = append(d.slideBackgrounds, d.defaultSlideBackground)
	}
	for len(d.slideBackgroundChanges) < count {
		d.slideBackgroundChanges = append(d.slideBackgroundChanges, presentationBackgroundOverride{})
	}
	for len(d.slideInherited) < count {
		d.slideInherited = append(d.slideInherited, append([]document.PresentationElement(nil), d.defaultSlideInherited...))
	}
	for len(d.slidePlaceholderBounds) < count {
		d.slidePlaceholderBounds = append(d.slidePlaceholderBounds, make(map[uint32]document.PresentationRect))
	}
	for len(d.slidePlaceholderTypes) < count {
		d.slidePlaceholderTypes = append(d.slidePlaceholderTypes, make(map[uint32]string))
	}
	for len(d.slidePlaceholderChanges) < count {
		d.slidePlaceholderChanges = append(d.slidePlaceholderChanges, make(map[uint32]string))
	}
	for len(d.slideShapeTransforms) < count {
		d.slideShapeTransforms = append(d.slideShapeTransforms, make(map[uint32]presentationTransformMetadata))
	}
	for len(d.slidePlaceholderXfrms) < count {
		d.slidePlaceholderXfrms = append(d.slidePlaceholderXfrms, make(map[uint32]presentationTransformMetadata))
	}
	for len(d.slideTransformChanges) < count {
		d.slideTransformChanges = append(d.slideTransformChanges, make(map[uint32]presentationTransformOverride))
	}
	for len(d.slideStyleChanges) < count {
		d.slideStyleChanges = append(d.slideStyleChanges, make(map[uint32]document.PresentationElementStylePatch))
	}
	for len(d.slidePictureCropStates) < count {
		d.slidePictureCropStates = append(d.slidePictureCropStates, make(map[uint32]presentationPictureCropState))
	}
	for len(d.slidePictureCropChanges) < count {
		d.slidePictureCropChanges = append(d.slidePictureCropChanges, make(map[uint32]document.PresentationCrop))
	}
	for len(d.slidePictureImageChanges) < count {
		d.slidePictureImageChanges = append(d.slidePictureImageChanges, make(map[uint32]presentationPictureImageOverride))
	}
	for index := 0; index < count; index++ {
		if d.slidePictureCropStates[index] == nil {
			d.slidePictureCropStates[index] = make(map[uint32]presentationPictureCropState)
		}
		if d.slidePictureCropChanges[index] == nil {
			d.slidePictureCropChanges[index] = make(map[uint32]document.PresentationCrop)
		}
		if d.slidePictureImageChanges[index] == nil {
			d.slidePictureImageChanges[index] = make(map[uint32]presentationPictureImageOverride)
		}
		if d.slideStyleChanges[index] == nil {
			d.slideStyleChanges[index] = make(map[uint32]document.PresentationElementStylePatch)
		}
	}
}

func (d *Document) removeSlideProjectionCaches(index, oldCount int) {
	if index < len(d.slideBackgrounds) {
		copy(d.slideBackgrounds[index:], d.slideBackgrounds[index+1:])
		d.slideBackgrounds[len(d.slideBackgrounds)-1] = pptxBackground{}
		d.slideBackgrounds = d.slideBackgrounds[:len(d.slideBackgrounds)-1]
	}
	if index < len(d.slideBackgroundChanges) {
		copy(d.slideBackgroundChanges[index:], d.slideBackgroundChanges[index+1:])
		d.slideBackgroundChanges[len(d.slideBackgroundChanges)-1] = presentationBackgroundOverride{}
		d.slideBackgroundChanges = d.slideBackgroundChanges[:len(d.slideBackgroundChanges)-1]
	}
	if index < len(d.slideInherited) {
		copy(d.slideInherited[index:], d.slideInherited[index+1:])
		d.slideInherited[len(d.slideInherited)-1] = nil
		d.slideInherited = d.slideInherited[:len(d.slideInherited)-1]
	}
	if index < len(d.slidePlaceholderBounds) {
		copy(d.slidePlaceholderBounds[index:], d.slidePlaceholderBounds[index+1:])
		d.slidePlaceholderBounds[len(d.slidePlaceholderBounds)-1] = nil
		d.slidePlaceholderBounds = d.slidePlaceholderBounds[:len(d.slidePlaceholderBounds)-1]
	}
	if index < len(d.slidePlaceholderTypes) {
		copy(d.slidePlaceholderTypes[index:], d.slidePlaceholderTypes[index+1:])
		d.slidePlaceholderTypes[len(d.slidePlaceholderTypes)-1] = nil
		d.slidePlaceholderTypes = d.slidePlaceholderTypes[:len(d.slidePlaceholderTypes)-1]
	}
	if index < len(d.slidePlaceholderChanges) {
		copy(d.slidePlaceholderChanges[index:], d.slidePlaceholderChanges[index+1:])
		d.slidePlaceholderChanges[len(d.slidePlaceholderChanges)-1] = nil
		d.slidePlaceholderChanges = d.slidePlaceholderChanges[:len(d.slidePlaceholderChanges)-1]
	}
	if index < len(d.slideShapeTransforms) {
		copy(d.slideShapeTransforms[index:], d.slideShapeTransforms[index+1:])
		d.slideShapeTransforms[len(d.slideShapeTransforms)-1] = nil
		d.slideShapeTransforms = d.slideShapeTransforms[:len(d.slideShapeTransforms)-1]
	}
	if index < len(d.slidePlaceholderXfrms) {
		copy(d.slidePlaceholderXfrms[index:], d.slidePlaceholderXfrms[index+1:])
		d.slidePlaceholderXfrms[len(d.slidePlaceholderXfrms)-1] = nil
		d.slidePlaceholderXfrms = d.slidePlaceholderXfrms[:len(d.slidePlaceholderXfrms)-1]
	}
	if index < len(d.slideTransformChanges) {
		copy(d.slideTransformChanges[index:], d.slideTransformChanges[index+1:])
		d.slideTransformChanges[len(d.slideTransformChanges)-1] = nil
		d.slideTransformChanges = d.slideTransformChanges[:len(d.slideTransformChanges)-1]
	}
	if index < len(d.slideStyleChanges) {
		copy(d.slideStyleChanges[index:], d.slideStyleChanges[index+1:])
		d.slideStyleChanges[len(d.slideStyleChanges)-1] = nil
		d.slideStyleChanges = d.slideStyleChanges[:len(d.slideStyleChanges)-1]
	}
	if index < len(d.slidePictureCropStates) {
		copy(d.slidePictureCropStates[index:], d.slidePictureCropStates[index+1:])
		d.slidePictureCropStates[len(d.slidePictureCropStates)-1] = nil
		d.slidePictureCropStates = d.slidePictureCropStates[:len(d.slidePictureCropStates)-1]
	}
	if index < len(d.slidePictureCropChanges) {
		copy(d.slidePictureCropChanges[index:], d.slidePictureCropChanges[index+1:])
		d.slidePictureCropChanges[len(d.slidePictureCropChanges)-1] = nil
		d.slidePictureCropChanges = d.slidePictureCropChanges[:len(d.slidePictureCropChanges)-1]
	}
	if index < len(d.slidePictureImageChanges) {
		copy(d.slidePictureImageChanges[index:], d.slidePictureImageChanges[index+1:])
		d.slidePictureImageChanges[len(d.slidePictureImageChanges)-1] = nil
		d.slidePictureImageChanges = d.slidePictureImageChanges[:len(d.slidePictureImageChanges)-1]
	}
	delete(d.slideBodyProps, index)
	delete(d.slidePictureTargets, index)
	for slideIndex := index; slideIndex < oldCount-1; slideIndex++ {
		if cached, ok := d.slideBodyProps[slideIndex+1]; ok {
			d.slideBodyProps[slideIndex] = cached
			delete(d.slideBodyProps, slideIndex+1)
		}
		if cached, ok := d.slidePictureTargets[slideIndex+1]; ok {
			d.slidePictureTargets[slideIndex] = cached
			delete(d.slidePictureTargets, slideIndex+1)
		}
	}
	delete(d.slideBodyProps, oldCount-1)
	delete(d.slidePictureTargets, oldCount-1)
}
