package desktopui

import (
	"context"

	"glance/internal/application"
	"glance/internal/document"
)

var _ application.WordCommentAccess = (*desktopSessionAdapter)(nil)
var _ application.WordTextSelectionCommentAccess = (*desktopSessionAdapter)(nil)

func (a *desktopSessionAdapter) WordComments(ctx context.Context) ([]document.WordComment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := a.session
	if s == nil || s.finalized.Load() {
		return nil, application.ErrDocumentNotFound
	}
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	if s.word == nil {
		return nil, application.ErrCapabilityUnsupported
	}
	return s.word.WordComments()
}

func (a *desktopSessionAdapter) AddWordComment(ctx context.Context, runID, author, text string) (string, bool, error) {
	var commentID string
	var didChange bool
	changed, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordCommentEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		var err error
		commentID, didChange, err = editor.AddWordComment(runID, author, text)
		return didChange, err
	})
	return commentID, changed && didChange, err
}

func (a *desktopSessionAdapter) AddWordCommentForTextSelection(ctx context.Context, startRunID string, startOffset int, endRunID string, endOffset int, author, text string) (string, bool, error) {
	var commentID string
	var didChange bool
	changed, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordTextSelectionCommentEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		var err error
		commentID, didChange, err = editor.AddWordCommentForTextSelection(startRunID, startOffset, endRunID, endOffset, author, text)
		return didChange, err
	})
	return commentID, changed && didChange, err
}

func (a *desktopSessionAdapter) DeleteWordComment(ctx context.Context, commentID string) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordCommentEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		return editor.DeleteWordComment(commentID)
	})
}
