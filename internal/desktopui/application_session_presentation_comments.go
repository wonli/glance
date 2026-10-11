package desktopui

import (
	"context"

	"glance/internal/application"
	"glance/internal/document"
)

var _ application.PresentationCommentAccess = (*desktopSessionAdapter)(nil)

func (a *desktopSessionAdapter) PresentationComments(ctx context.Context, slideID uint32, start, limit int) ([]document.PresentationComment, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return nil, 0, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.pptxDoc == nil {
		return nil, 0, application.ErrCapabilityUnsupported
	}
	return s.pptxDoc.ListPresentationComments(slideID, start, limit)
}

func (a *desktopSessionAdapter) AddPresentationComment(ctx context.Context, slideID uint32, xPT, yPT float64, author, text string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return "", false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return "", false, application.ErrCapabilityUnsupported
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return s.pptxDoc.AddPresentationComment(slideID, xPT, yPT, author, text)
}

func (a *desktopSessionAdapter) DeletePresentationComment(ctx context.Context, slideID uint32, commentID string) (bool, error) {
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
	return s.pptxDoc.DeletePresentationComment(slideID, commentID)
}
