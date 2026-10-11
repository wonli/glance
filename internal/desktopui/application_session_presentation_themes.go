package desktopui

import (
	"context"

	"glance/internal/application"
)

func (a *desktopSessionAdapter) SetPresentationTheme(ctx context.Context, themeID string) (bool, error) {
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
	return s.pptxDoc.SetThemePreset(ctx, themeID)
}

func (a *desktopSessionAdapter) SetPresentationThemePalette(ctx context.Context, paletteID string) (bool, error) {
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
	return s.pptxDoc.SetThemePalette(ctx, paletteID)
}
