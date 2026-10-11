package desktopui

import (
	"context"
	"fmt"

	"glance/internal/application"
)

func (a *desktopSessionAdapter) SetPresentationElementAnimation(ctx context.Context, slideID uint32, elementID int64, effect string, durationMS int, expectedRevision uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	changed, err := s.pptxDoc.SetPresentationElementAnimationByID(ctx, slideID, elementID, effect, durationMS, expectedRevision)
	if err != nil {
		return false, fmt.Errorf("设置幻灯片动画失败: %w", err)
	}
	return changed, nil
}

func (a *desktopSessionAdapter) SetPresentationAnimationDuration(ctx context.Context, slideID uint32, elementID int64, animationIndex, durationMS int, expectedRevision uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	changed, err := s.pptxDoc.SetPresentationAnimationDurationByID(ctx, slideID, elementID, animationIndex, durationMS, expectedRevision)
	if err != nil {
		return false, fmt.Errorf("设置幻灯片动画时长失败: %w", err)
	}
	return changed, nil
}

func (a *desktopSessionAdapter) DeletePresentationAnimation(ctx context.Context, slideID uint32, elementID int64, animationIndex int, expectedRevision uint64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return 0, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return 0, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	removed, err := s.pptxDoc.DeletePresentationAnimationByID(ctx, slideID, elementID, animationIndex, expectedRevision)
	if err != nil {
		return 0, fmt.Errorf("删除幻灯片动画失败: %w", err)
	}
	return removed, nil
}
