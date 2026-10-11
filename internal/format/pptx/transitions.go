package pptx

import (
	"errors"
	"fmt"

	"glance/internal/document"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

type slideTransitionSnapshot struct {
	value  document.PresentationTransition
	exists bool
}

func (d *Document) PresentationTransitionByID(slideID uint32) (document.PresentationTransition, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.pres == nil {
		return document.PresentationTransition{}, fmt.Errorf("演示文稿已关闭")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return document.PresentationTransition{}, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	for _, slide := range slides {
		if uint32(slide.ID()) == slideID {
			transition, _, err := readPresentationTransition(slide)
			return transition, err
		}
	}
	return document.PresentationTransition{}, fmt.Errorf("未找到幻灯片 ID %d", slideID)
}

func (d *Document) SetPresentationTransitionByID(slideID uint32, patch document.PresentationTransitionPatch, applyToAll bool) (document.PresentationTransition, []uint32, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return document.PresentationTransition{}, nil, false, fmt.Errorf("演示文稿已关闭")
	}
	if transitionPatchEmpty(patch) && !applyToAll {
		return document.PresentationTransition{}, nil, false, fmt.Errorf("切换效果设置不能为空")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return document.PresentationTransition{}, nil, false, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	sourceIndex := -1
	for index, slide := range slides {
		if uint32(slide.ID()) == slideID {
			sourceIndex = index
			break
		}
	}
	if sourceIndex < 0 {
		return document.PresentationTransition{}, nil, false, fmt.Errorf("未找到幻灯片 ID %d", slideID)
	}
	source, _, err := readPresentationTransition(slides[sourceIndex])
	if err != nil {
		return document.PresentationTransition{}, nil, false, err
	}
	target, err := mergePresentationTransition(source, patch)
	if err != nil {
		return document.PresentationTransition{}, nil, false, err
	}

	indices := []int{sourceIndex}
	if applyToAll {
		indices = make([]int, len(slides))
		for index := range slides {
			indices[index] = index
		}
	}
	previous := make(map[int]slideTransitionSnapshot, len(indices))
	for _, index := range indices {
		value, exists, err := readPresentationTransition(slides[index])
		if err != nil {
			return document.PresentationTransition{}, nil, false, err
		}
		previous[index] = slideTransitionSnapshot{value: value, exists: exists}
	}

	changedSlides := make([]uint32, 0, len(indices))
	for _, index := range indices {
		before := previous[index]
		if before.value == target {
			continue
		}
		if err := slides[index].SetTransition(presentationTransitionToLibrary(target)); err != nil {
			rollbackErr := rollbackPresentationTransitions(slides, previous, changedSlides)
			if rollbackErr != nil {
				return document.PresentationTransition{}, nil, false, fmt.Errorf("set transition: %w; rollback failed: %v", err, rollbackErr)
			}
			return document.PresentationTransition{}, nil, false, fmt.Errorf("set transition: %w", err)
		}
		changedSlides = append(changedSlides, uint32(slides[index].ID()))
	}
	if len(changedSlides) > 0 {
		d.dirty = true
		d.revision++
	}
	return target, changedSlides, len(changedSlides) > 0, nil
}

func readPresentationTransition(slide *gopptx.Slide) (document.PresentationTransition, bool, error) {
	spec, err := slide.Transition()
	if errors.Is(err, gopptx.ErrNotFound) {
		return document.PresentationTransition{Type: "none", Speed: "fast", AdvanceOnClick: true}, false, nil
	}
	if err != nil {
		return document.PresentationTransition{}, false, fmt.Errorf("读取幻灯片切换效果失败: %w", err)
	}
	return presentationTransitionFromLibrary(spec), true, nil
}

func presentationTransitionFromLibrary(spec gopptx.TransitionSpec) document.PresentationTransition {
	result := document.PresentationTransition{
		Type: string(spec.Type), Speed: string(spec.Speed), Direction: string(spec.Dir),
		SplitDirection: string(spec.SplitDir), SplitAxis: string(spec.SplitAxis), AdvanceOnClick: true,
	}
	if spec.AdvanceClick != nil {
		result.AdvanceOnClick = *spec.AdvanceClick
	}
	switch spec.Type {
	case gopptx.TransitionFade:
		result.ThroughBlack = spec.Fade.ThroughBlack
	case gopptx.TransitionCut:
		result.ThroughBlack = spec.Cut.ThroughBlack
	}
	if result.Speed == "" {
		result.Speed = string(gopptx.SpeedFast)
	}
	return result
}

func presentationTransitionToLibrary(value document.PresentationTransition) gopptx.TransitionSpec {
	advanceOnClick := value.AdvanceOnClick
	spec := gopptx.TransitionSpec{
		Type: gopptx.TransitionType(value.Type), Speed: gopptx.TransitionSpeed(value.Speed),
		AdvanceClick: &advanceOnClick, Dir: gopptx.TransitionDir(value.Direction),
		SplitDir: gopptx.SplitDir(value.SplitDirection), SplitAxis: gopptx.SplitAxis(value.SplitAxis),
	}
	switch spec.Type {
	case gopptx.TransitionFade:
		spec.Fade.ThroughBlack = value.ThroughBlack
	case gopptx.TransitionCut:
		spec.Cut.ThroughBlack = value.ThroughBlack
	}
	return spec
}

func transitionPatchEmpty(patch document.PresentationTransitionPatch) bool {
	return patch.Type == nil && patch.Speed == nil && patch.Direction == nil && patch.SplitDirection == nil &&
		patch.SplitAxis == nil && patch.ThroughBlack == nil && patch.AdvanceOnClick == nil
}

func mergePresentationTransition(current document.PresentationTransition, patch document.PresentationTransitionPatch) (document.PresentationTransition, error) {
	result := current
	if patch.Type != nil {
		result.Type = *patch.Type
		result.Direction = ""
		result.SplitDirection = ""
		result.SplitAxis = ""
		result.ThroughBlack = false
	}
	if patch.Speed != nil {
		result.Speed = *patch.Speed
	}
	if patch.Direction != nil {
		result.Direction = *patch.Direction
	}
	if patch.SplitDirection != nil {
		result.SplitDirection = *patch.SplitDirection
	}
	if patch.SplitAxis != nil {
		result.SplitAxis = *patch.SplitAxis
	}
	if patch.ThroughBlack != nil {
		result.ThroughBlack = *patch.ThroughBlack
	}
	if patch.AdvanceOnClick != nil {
		result.AdvanceOnClick = *patch.AdvanceOnClick
	}
	if patch.Type == nil && current.Type == "none" {
		return document.PresentationTransition{}, fmt.Errorf("该幻灯片尚未设置切换效果")
	}
	if result.Speed != "slow" && result.Speed != "med" && result.Speed != "fast" {
		return document.PresentationTransition{}, fmt.Errorf("不支持的切换速度 %q", result.Speed)
	}
	switch result.Type {
	case "none", "fade", "push", "wipe", "split", "cover", "cut", "dissolve":
	default:
		return document.PresentationTransition{}, fmt.Errorf("不支持的切换类型 %q", result.Type)
	}
	if (result.Type == "push" || result.Type == "wipe" || result.Type == "cover") && result.Direction != "" && result.Direction != "l" && result.Direction != "r" && result.Direction != "u" && result.Direction != "d" {
		return document.PresentationTransition{}, fmt.Errorf("不支持的切换方向 %q", result.Direction)
	}
	if result.Type == "split" {
		if result.SplitDirection != "" && result.SplitDirection != "in" && result.SplitDirection != "out" {
			return document.PresentationTransition{}, fmt.Errorf("不支持的分割方向 %q", result.SplitDirection)
		}
		if result.SplitAxis != "" && result.SplitAxis != "hor" && result.SplitAxis != "vert" {
			return document.PresentationTransition{}, fmt.Errorf("不支持的分割轴 %q", result.SplitAxis)
		}
	}
	if result.Type != "fade" && result.Type != "cut" && result.ThroughBlack {
		return document.PresentationTransition{}, fmt.Errorf("through_black 仅适用于 fade 或 cut")
	}
	if result.Type != "split" && (result.SplitDirection != "" || result.SplitAxis != "") {
		return document.PresentationTransition{}, fmt.Errorf("split 参数仅适用于 split 切换")
	}
	if result.Type != "push" && result.Type != "wipe" && result.Type != "cover" && result.Direction != "" {
		return document.PresentationTransition{}, fmt.Errorf("direction 不适用于 %s 切换", result.Type)
	}
	return result, nil
}

func rollbackPresentationTransitions(slides []*gopptx.Slide, previous map[int]slideTransitionSnapshot, changed []uint32) error {
	byID := make(map[uint32]int, len(changed))
	for _, id := range changed {
		for index, slide := range slides {
			if uint32(slide.ID()) == id {
				byID[id] = index
				break
			}
		}
	}
	for index := len(changed) - 1; index >= 0; index-- {
		id := changed[index]
		slideIndex, ok := byID[id]
		if !ok {
			continue
		}
		before := previous[slideIndex]
		if !before.exists {
			if err := slides[slideIndex].RemoveTransition(); err != nil {
				return err
			}
			continue
		}
		if err := slides[slideIndex].SetTransition(presentationTransitionToLibrary(before.value)); err != nil {
			return err
		}
	}
	return nil
}
