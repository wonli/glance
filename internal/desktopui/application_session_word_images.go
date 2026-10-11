package desktopui

import (
	"bytes"
	"context"
	"math"

	"glance/internal/application"
	"glance/internal/document"
)

func (a *desktopSessionAdapter) InsertWordImage(ctx context.Context, paragraphID, afterInlineID, mediaType string, data []byte) (string, error) {
	var imageID string
	_, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordImageEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		var err error
		imageID, err = editor.InsertInlineImage(paragraphID, afterInlineID, mediaType, data)
		if err != nil {
			return false, err
		}
		return true, nil
	})
	return imageID, err
}

func (a *desktopSessionAdapter) MoveWordImage(ctx context.Context, imageID, paragraphID, afterInlineID string) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		blocks := content.WordBlocks()
		current := findWordImage(blocks, imageID)
		if current == nil {
			return false, application.ErrInvalidCommandArguments
		}
		if !current.Editable {
			return false, application.ErrCapabilityUnsupported
		}
		if imageID == afterInlineID {
			return false, application.ErrInvalidCommandArguments
		}
		if wordImageMoveIsNoop(blocks, imageID, paragraphID, afterInlineID) {
			return false, nil
		}
		editor, ok := content.(document.WordImageEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.MoveInlineImage(imageID, paragraphID, afterInlineID); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) SetWordImageWrap(ctx context.Context, imageID, wrapMode string) (string, bool, error) {
	previous := ""
	changed, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		current := findWordImage(content.WordBlocks(), imageID)
		if current == nil {
			return false, application.ErrInvalidCommandArguments
		}
		previous = current.WrapMode
		if !current.WrapEditable {
			return false, application.ErrCapabilityUnsupported
		}
		if previous == wrapMode {
			return false, nil
		}
		editor, ok := content.(document.WordImageWrapEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.SetWordImageWrap(imageID, wrapMode); err != nil {
			return false, err
		}
		return true, nil
	})
	return previous, changed, err
}

func (a *desktopSessionAdapter) SetWordThemeEffects(ctx context.Context, profile string) (string, bool, error) {
	previous := ""
	changed, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordThemeEffectsEditor)
		if !ok || !editor.CanSetWordThemeEffects() {
			return false, application.ErrCapabilityUnsupported
		}
		previousProfile, didChange, setErr := editor.SetWordThemeEffects(profile)
		previous = previousProfile
		return didChange, setErr
	})
	return previous, changed, err
}

func wordImageMoveIsNoop(blocks []document.WordBlock, imageID, paragraphID, afterInlineID string) bool {
	for _, paragraph := range allWordParagraphs(blocks) {
		imageIndex := -1
		anchorIndex := -1
		for index, inline := range paragraph.Inlines {
			id := wordInlineID(inline)
			if paragraph.ID == paragraphID && id == afterInlineID && (inline.Text != nil || inline.Image != nil) {
				anchorIndex = index
			}
			if id == imageID {
				imageIndex = index
				if paragraph.ID != paragraphID {
					return false
				}
			}
		}
		if imageIndex >= 0 {
			if afterInlineID == "" {
				return imageIndex == 0
			}
			return anchorIndex >= 0 && imageIndex == anchorIndex+1
		}
	}
	return false
}

func wordInlineID(inline document.WordInline) string {
	switch {
	case inline.Text != nil:
		return inline.Text.ID
	case inline.Image != nil:
		return inline.Image.ID
	case inline.Unsupported != nil:
		return inline.Unsupported.ID
	case inline.PageBreak != nil:
		return inline.PageBreak.ID
	case inline.Tab != nil:
		return inline.Tab.ID
	case inline.FieldCode != nil:
		return inline.FieldCode.ID
	default:
		return ""
	}
}

func (a *desktopSessionAdapter) ReplaceWordImage(ctx context.Context, imageID, mediaType string, data []byte) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		current := findWordImage(content.WordBlocks(), imageID)
		if current == nil {
			return false, application.ErrInvalidCommandArguments
		}
		if !current.Editable {
			return false, application.ErrCapabilityUnsupported
		}
		if current.MediaType == mediaType {
			if reader, ok := any(content).(interface{ WordImageBytes(string) ([]byte, error) }); ok {
				previous, err := reader.WordImageBytes(imageID)
				if err != nil {
					return false, err
				}
				if bytes.Equal(previous, data) {
					return false, nil
				}
			}
		}
		editor, ok := content.(document.WordImageEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.ReplaceInlineImage(imageID, mediaType, data); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) DeleteWordImage(ctx context.Context, imageID string) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		current := findWordImage(content.WordBlocks(), imageID)
		if current == nil {
			return false, application.ErrInvalidCommandArguments
		}
		if !current.Editable {
			return false, application.ErrCapabilityUnsupported
		}
		editor, ok := content.(document.WordImageEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.DeleteInlineImage(imageID); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) ResizeWordImage(ctx context.Context, imageID string, widthEMU int64) (bool, error) {
	return a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		current := findWordImage(content.WordBlocks(), imageID)
		if current == nil {
			return false, application.ErrInvalidCommandArguments
		}
		if !current.Editable {
			return false, application.ErrCapabilityUnsupported
		}
		if current.WidthEMU == widthEMU {
			return false, nil
		}
		editor, ok := content.(document.WordImageEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := editor.ResizeInlineImage(imageID, widthEMU); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (a *desktopSessionAdapter) RotateWordImage(ctx context.Context, imageID string, deltaDegrees float64) (float64, bool, error) {
	var rotation float64
	changed, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		current := findWordImage(content.WordBlocks(), imageID)
		if current == nil {
			return false, application.ErrInvalidCommandArguments
		}
		if !current.Editable {
			return false, application.ErrCapabilityUnsupported
		}
		rotation = normalizeApplicationWordImageRotation(current.RotationDegrees + deltaDegrees)
		if math.Abs(rotation-current.RotationDegrees) < 1e-9 {
			return false, nil
		}
		rotator, ok := content.(document.WordImageRotator)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		if err := rotator.SetInlineImageRotation(imageID, rotation); err != nil {
			return false, err
		}
		return true, nil
	})
	return rotation, changed, err
}

func (a *desktopSessionAdapter) SetWordAllParagraphSpacing(ctx context.Context, beforeTwips, afterTwips int64) (int, bool, error) {
	paragraphCount := 0
	changed, err := a.mutateWordContent(ctx, func(content document.WordContentReader) (bool, error) {
		editor, ok := content.(document.WordParagraphSpacingEditor)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		snapshotter, ok := content.(document.WordPackageSnapshotter)
		if !ok {
			return false, application.ErrCapabilityUnsupported
		}
		before, err := snapshotter.Snapshot()
		if err != nil {
			return false, err
		}
		paragraphCount = desktopWordParagraphCount(content.WordBlocks())
		if err := editor.SetAllParagraphsSpacing(beforeTwips, afterTwips); err != nil {
			return false, err
		}
		after, err := snapshotter.Snapshot()
		if err != nil {
			return false, err
		}
		return !bytes.Equal(before, after), nil
	})
	return paragraphCount, changed, err
}

func desktopWordParagraphCount(blocks []document.WordBlock) int {
	count := 0
	for _, block := range blocks {
		if block.Paragraph != nil {
			count++
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					count += len(cell.Paragraphs)
				}
			}
		}
	}
	return count
}

func normalizeApplicationWordImageRotation(degrees float64) float64 {
	degrees = math.Mod(degrees, 360)
	if degrees < 0 {
		degrees += 360
	}
	if math.Abs(degrees) < 1e-9 || math.Abs(degrees-360) < 1e-9 {
		return 0
	}
	return degrees
}
