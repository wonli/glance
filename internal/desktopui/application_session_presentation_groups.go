package desktopui

import (
	"context"

	"glance/internal/application"
)

func (a *desktopSessionAdapter) GroupPresentationElements(ctx context.Context, slideID uint32, elementIDs []uint32) (uint32, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return 0, false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.pptxDoc == nil {
		return 0, false, application.ErrCapabilityUnsupported
	}
	return s.pptxDoc.GroupElementsByID(ctx, slideID, elementIDs)
}
