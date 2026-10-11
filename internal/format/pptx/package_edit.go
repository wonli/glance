package pptx

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	"io"

	"glance/internal/document"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

// applyPPTXPackageEditsLocked materializes adapter overlays, applies narrowly
// scoped OPC part changes, then reopens the package so the live projection and
// the bytes that will be saved describe the same document.
func (d *Document) applyPPTXPackageEditsLocked(ctx context.Context, edits map[string][]byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	base, err := d.snapshotBytesLocked()
	if err != nil {
		return err
	}
	updated, err := replacePPTXPackageParts(ctx, base, edits)
	if err != nil {
		return err
	}
	return d.adoptPPTXSnapshotLocked(updated)
}

func replacePPTXPackageParts(ctx context.Context, source []byte, edits map[string][]byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(source), int64(len(source)))
	if err != nil {
		return nil, fmt.Errorf("解析 PPTX 暂存包失败: %w", err)
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	written := make(map[string]bool, len(edits))
	for _, part := range reader.File {
		if err := ctx.Err(); err != nil {
			_ = writer.Close()
			return nil, err
		}
		header := part.FileHeader
		out, err := writer.CreateHeader(&header)
		if err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("创建 PPTX 部件 %s 失败: %w", part.Name, err)
		}
		name := part.Name
		if edit, ok := edits[name]; ok {
			if _, err := out.Write(edit); err != nil {
				_ = writer.Close()
				return nil, fmt.Errorf("写入 PPTX 部件 %s 失败: %w", name, err)
			}
			written[name] = true
			continue
		}
		input, err := part.Open()
		if err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("打开 PPTX 部件 %s 失败: %w", name, err)
		}
		_, copyErr := io.Copy(out, input)
		closeErr := input.Close()
		if copyErr != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("复制 PPTX 部件 %s 失败: %w", name, copyErr)
		}
		if closeErr != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("关闭 PPTX 部件 %s 失败: %w", name, closeErr)
		}
	}
	for name := range edits {
		if !written[name] {
			_ = writer.Close()
			return nil, fmt.Errorf("PPTX 编辑目标部件不存在: %s", name)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("完成 PPTX 暂存包失败: %w", err)
	}
	return output.Bytes(), nil
}

func (d *Document) adoptPPTXSnapshotLocked(snapshot []byte) error {
	reader, err := zip.NewReader(bytes.NewReader(snapshot), int64(len(snapshot)))
	if err != nil {
		return fmt.Errorf("校验 PPTX 更新包失败: %w", err)
	}
	presentation, err := gopptx.OpenReader(bytes.NewReader(snapshot), int64(len(snapshot)))
	if err != nil {
		return fmt.Errorf("重新载入 PPTX 更新包失败: %w", err)
	}
	refreshed := &Document{
		path:                     d.path,
		pres:                     presentation,
		widthPT:                  defaultSlideWidthPT,
		heightPT:                 defaultSlideHeightPT,
		mediaImages:              make(map[string]image.Image),
		mediaBytes:               make(map[string][]byte),
		presentationComments:     newPPTXCommentState(),
		slideLayoutChanges:       make(map[uint32]string),
		slideAnimationChanges:    make(map[uint32][]byte),
		slideBodyProps:           make(map[int]map[uint32]shapeBodyProps),
		slidePictureTargets:      make(map[int]map[uint32]string),
		slideTransformChanges:    make([]map[uint32]presentationTransformOverride, 0),
		slideStyleChanges:        make([]map[uint32]document.PresentationElementStylePatch, 0),
		slidePictureCropChanges:  make([]map[uint32]document.PresentationCrop, 0),
		slidePictureImageChanges: make([]map[uint32]presentationPictureImageOverride, 0),
		revision:                 1,
	}
	refreshed.inspectZipPackage(reader)
	refreshed.loadPPTXCommentState(reader)

	oldPresentation := d.pres
	path := d.path
	activeSlide := d.activeSlide
	revision := d.revision
	d.pres = refreshed.pres
	refreshed.pres = nil
	d.path = path
	d.widthPT = refreshed.widthPT
	d.heightPT = refreshed.heightPT
	d.activeSlide = activeSlide
	d.dirty = true
	d.revision = revision + 1
	d.mediaImages = refreshed.mediaImages
	d.mediaBytes = refreshed.mediaBytes
	d.slideBodyProps = refreshed.slideBodyProps
	d.slidePictureTargets = refreshed.slidePictureTargets
	d.slideBackgrounds = refreshed.slideBackgrounds
	d.slideInherited = refreshed.slideInherited
	d.slidePlaceholderBounds = refreshed.slidePlaceholderBounds
	d.slidePlaceholderTypes = refreshed.slidePlaceholderTypes
	d.slidePlaceholderChanges = refreshed.slidePlaceholderChanges
	d.slideShapeTransforms = refreshed.slideShapeTransforms
	d.slidePlaceholderXfrms = refreshed.slidePlaceholderXfrms
	d.slideTransformChanges = refreshed.slideTransformChanges
	d.slideStyleChanges = refreshed.slideStyleChanges
	d.slidePictureCropStates = refreshed.slidePictureCropStates
	d.slidePictureCropChanges = refreshed.slidePictureCropChanges
	d.slidePictureImageChanges = refreshed.slidePictureImageChanges
	d.slideBackgroundChanges = refreshed.slideBackgroundChanges
	d.slideSizeChange = refreshed.slideSizeChange
	d.presentationComments = refreshed.presentationComments
	d.slideLayoutChanges = refreshed.slideLayoutChanges
	d.slideAnimationChanges = refreshed.slideAnimationChanges
	d.defaultSlideBackground = refreshed.defaultSlideBackground
	d.defaultSlideInherited = refreshed.defaultSlideInherited
	if oldPresentation != nil {
		_ = oldPresentation.Close()
	}
	return nil
}
