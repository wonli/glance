package pptx

import (
	"fmt"
	"glance/internal/document"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

// UpdateSlideElementTransform records a slide-local geometry change. The
// change is projected immediately and serialized by the PPTX adapter during
// file saves and recovery snapshots.
func (d *Document) UpdateSlideElementTransform(slideIndex int, shapeID int64, bounds document.PresentationRect, rotationDeg float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := validatePresentationTransform(bounds, rotationDeg); err != nil {
		return err
	}
	if shapeID <= 0 || uint64(shapeID) > uint64(^uint32(0)) {
		return fmt.Errorf("形状 ID 无效: %d", shapeID)
	}
	slides, err := d.pres.Slides()
	if err != nil || slideIndex < 0 || slideIndex >= len(slides) {
		return fmt.Errorf("幻灯片序号无效: %d", slideIndex)
	}
	shapes, err := slides[slideIndex].Shapes()
	if err != nil {
		return fmt.Errorf("读取幻灯片形状失败: %w", err)
	}
	var selected gopptx.Shape
	count := 0
	for _, shape := range shapes {
		if int64(shape.ID()) == shapeID {
			selected = shape
			count++
		}
	}
	if count != 1 {
		if count == 0 {
			return fmt.Errorf("未找到幻灯片 %d 上的形状 ID %d", slideIndex, shapeID)
		}
		return fmt.Errorf("幻灯片 %d 上的形状 ID %d 不唯一", slideIndex, shapeID)
	}
	switch selected.(type) {
	case *gopptx.AutoShape, *gopptx.PictureShape:
	default:
		return fmt.Errorf("不支持变换幻灯片 %d 上的形状 ID %d", slideIndex, shapeID)
	}
	current := d.projectShape(selected, slideIndex)
	rotationDeg = normalizePresentationRotation(rotationDeg)
	if current.Bounds == bounds && normalizePresentationRotation(current.RotationDeg) == rotationDeg {
		return nil
	}
	d.ensureSlideProjectionCacheCount(len(slides))
	id := uint32(shapeID)
	if d.slideTransformChanges[slideIndex] == nil {
		d.slideTransformChanges[slideIndex] = make(map[uint32]presentationTransformOverride)
	}
	d.slideTransformChanges[slideIndex][id] = presentationTransformOverride{Bounds: bounds, RotationDeg: rotationDeg}
	d.dirty = true
	d.revision++
	return nil
}
