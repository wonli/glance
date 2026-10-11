package desktopui

import (
	"context"

	"glance/internal/application"
	"glance/internal/document"
)

func (a *desktopSessionAdapter) ListSpreadsheetImages(ctx context.Context, sheet string) ([]document.SpreadsheetImage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return nil, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return nil, application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.book).(document.SpreadsheetImageReader)
	if !ok {
		return nil, application.ErrCapabilityUnsupported
	}
	return application.ListSpreadsheetImageMetadata(ctx, reader, sheet)
}

func (a *desktopSessionAdapter) ReadSpreadsheetImageBytes(ctx context.Context, imageID string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return nil, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.book == nil {
		return nil, application.ErrCapabilityUnsupported
	}
	reader, ok := any(s.book).(document.SpreadsheetImageReader)
	if !ok {
		return nil, application.ErrCapabilityUnsupported
	}
	return reader.SpreadsheetImageBytes(imageID)
}

func (a *desktopSessionAdapter) InsertSpreadsheetImage(ctx context.Context, placement document.SpreadsheetImagePlacement, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return "", application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil {
		return "", application.ErrCapabilityUnsupported
	}
	if !s.book.Editable() {
		return "", application.ErrReadOnlyDocument
	}
	editor, ok := any(s.book).(document.SpreadsheetImageEditor)
	if !ok {
		return "", application.ErrCapabilityUnsupported
	}
	return application.InsertSpreadsheetImage(ctx, editor, placement, data)
}

func (a *desktopSessionAdapter) ReplaceSpreadsheetImage(ctx context.Context, sheet, imageID, mediaType string, data []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil {
		return false, application.ErrCapabilityUnsupported
	}
	if !s.book.Editable() {
		return false, application.ErrReadOnlyDocument
	}
	reader, readable := any(s.book).(document.SpreadsheetImageReader)
	editor, editable := any(s.book).(document.SpreadsheetImageEditor)
	if !readable || !editable {
		return false, application.ErrCapabilityUnsupported
	}
	return application.ReplaceSpreadsheetImage(ctx, reader, editor, sheet, imageID, mediaType, data)
}

func (a *desktopSessionAdapter) MoveSpreadsheetImage(ctx context.Context, sheet, imageID string, row, column, offsetX, offsetY int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil {
		return false, application.ErrCapabilityUnsupported
	}
	if !s.book.Editable() {
		return false, application.ErrReadOnlyDocument
	}
	reader, readable := any(s.book).(document.SpreadsheetImageReader)
	editor, editable := any(s.book).(document.SpreadsheetImageEditor)
	if !readable || !editable {
		return false, application.ErrCapabilityUnsupported
	}
	return application.MoveSpreadsheetImage(ctx, reader, editor, sheet, imageID, row, column, offsetX, offsetY)
}

func (a *desktopSessionAdapter) ResizeSpreadsheetImage(ctx context.Context, sheet, imageID string, widthPx int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil {
		return false, application.ErrCapabilityUnsupported
	}
	if !s.book.Editable() {
		return false, application.ErrReadOnlyDocument
	}
	reader, readable := any(s.book).(document.SpreadsheetImageReader)
	editor, editable := any(s.book).(document.SpreadsheetImageEditor)
	if !readable || !editable {
		return false, application.ErrCapabilityUnsupported
	}
	return application.ResizeSpreadsheetImage(ctx, reader, editor, sheet, imageID, widthPx)
}

func (a *desktopSessionAdapter) DeleteSpreadsheetImage(ctx context.Context, sheet, imageID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return false, application.ErrDocumentNotFound
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.book == nil {
		return false, application.ErrCapabilityUnsupported
	}
	if !s.book.Editable() {
		return false, application.ErrReadOnlyDocument
	}
	reader, readable := any(s.book).(document.SpreadsheetImageReader)
	editor, editable := any(s.book).(document.SpreadsheetImageEditor)
	if !readable || !editable {
		return false, application.ErrCapabilityUnsupported
	}
	return application.DeleteSpreadsheetImage(ctx, reader, editor, sheet, imageID)
}
