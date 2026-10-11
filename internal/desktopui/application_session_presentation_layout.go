package desktopui

import (
	"context"
	"fmt"

	"glance/internal/application"
	"glance/internal/document"
)

func (a *desktopSessionAdapter) PresentationLayouts(ctx context.Context) (document.PresentationLayoutSummary, error) {
	if err := ctx.Err(); err != nil {
		return document.PresentationLayoutSummary{}, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.PresentationLayoutSummary{}, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.pptxDoc == nil {
		return document.PresentationLayoutSummary{}, application.ErrCapabilityUnsupported
	}
	return s.pptxDoc.PresentationLayouts(ctx)
}

func (a *desktopSessionAdapter) SetPresentationSlideLayout(ctx context.Context, slideID uint32, layoutIndex int, expectedRevision uint64) (document.PresentationLayoutOption, bool, error) {
	if err := ctx.Err(); err != nil {
		return document.PresentationLayoutOption{}, false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return document.PresentationLayoutOption{}, false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return document.PresentationLayoutOption{}, false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return document.PresentationLayoutOption{}, false, err
	}
	option, changed, err := s.pptxDoc.SetSlideLayoutByID(ctx, slideID, layoutIndex, expectedRevision)
	if err != nil {
		return document.PresentationLayoutOption{}, false, fmt.Errorf("设置幻灯片版式失败: %w", err)
	}
	return option, changed, nil
}
