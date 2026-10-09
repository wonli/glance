package pptx

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"glance/internal/document"
	"io"
	"os"
	"path/filepath"
	"sort"
)

func hasPresentationTransformOverrides(overrides []map[uint32]presentationTransformOverride) bool {
	for _, slide := range overrides {
		if len(slide) > 0 {
			return true
		}
	}
	return false
}

func hasPresentationCropOverrides(overrides []map[uint32]document.PresentationCrop) bool {
	for _, slide := range overrides {
		if len(slide) > 0 {
			return true
		}
	}
	return false
}

func hasPresentationPackageEdits(transforms []map[uint32]presentationTransformOverride, crops []map[uint32]document.PresentationCrop, styles []map[uint32]document.PresentationElementStylePatch) bool {
	if hasPresentationTransformOverrides(transforms) || hasPresentationCropOverrides(crops) {
		return true
	}
	for _, slide := range styles {
		if len(slide) > 0 {
			return true
		}
	}
	return false
}

// patchPresentationEdits applies adapter-owned geometry and picture crops to a
// serialized package. Other package entries are copied without modification.
func patchPresentationEdits(sourcePath, targetPath string, transforms []map[uint32]presentationTransformOverride, crops []map[uint32]document.PresentationCrop, styles []map[uint32]document.PresentationElementStylePatch) error {
	if filepath.Clean(sourcePath) == filepath.Clean(targetPath) {
		return fmt.Errorf("PPTX 几何补丁的源和目标路径不能相同")
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("打开 PPTX 几何补丁源失败: %w", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return fmt.Errorf("读取 PPTX 几何补丁源信息失败: %w", err)
	}
	reader, err := zip.NewReader(source, info.Size())
	if err != nil {
		return fmt.Errorf("读取 PPTX 几何补丁源失败: %w", err)
	}
	pkg := newPPTXPackage(reader)
	patchedSlides := make(map[string][]byte)
	slideCount := max(len(transforms), len(crops), len(styles))
	for slideIndex := 0; slideIndex < slideCount; slideIndex++ {
		var slideTransforms map[uint32]presentationTransformOverride
		if slideIndex < len(transforms) {
			slideTransforms = transforms[slideIndex]
		}
		var slideCrops map[uint32]document.PresentationCrop
		if slideIndex < len(crops) {
			slideCrops = crops[slideIndex]
		}
		var slideStyles map[uint32]document.PresentationElementStylePatch
		if slideIndex < len(styles) {
			slideStyles = styles[slideIndex]
		}
		if len(slideTransforms) == 0 && len(slideCrops) == 0 && len(slideStyles) == 0 {
			continue
		}
		if slideIndex < 0 || slideIndex >= len(pkg.slideParts) {
			return fmt.Errorf("几何补丁的幻灯片序号无效: %d", slideIndex)
		}
		slidePart := pkg.slideParts[slideIndex]
		part := pkg.parts[slidePart]
		if part == nil {
			return fmt.Errorf("PPTX 缺少幻灯片部件 %s", slidePart)
		}
		data, err := readOPCPart(part)
		if err != nil {
			return fmt.Errorf("读取幻灯片部件 %s 失败: %w", slidePart, err)
		}
		patched, err := patchSlidePresentationData(data, slidePart, slideIndex, pkg, slideTransforms, slideCrops, slideStyles)
		if err != nil {
			return fmt.Errorf("更新幻灯片 %d 对象失败: %w", slideIndex, err)
		}
		patchedSlides[slidePart] = patched
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建几何补丁目录失败: %w", err)
	}
	tmpFile, err := os.CreateTemp(dir, "glance-pptx-transform-*.tmp")
	if err != nil {
		return fmt.Errorf("创建几何补丁临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	writer := zip.NewWriter(tmpFile)
	for _, part := range reader.File {
		header := part.FileHeader
		out, err := writer.CreateHeader(&header)
		if err != nil {
			_ = writer.Close()
			_ = tmpFile.Close()
			return fmt.Errorf("创建 PPTX 部件 %s 失败: %w", part.Name, err)
		}
		if data, ok := patchedSlides[part.Name]; ok {
			if _, err := out.Write(data); err != nil {
				_ = writer.Close()
				_ = tmpFile.Close()
				return fmt.Errorf("写入已更新的 PPTX 部件 %s 失败: %w", part.Name, err)
			}
			continue
		}
		in, err := part.Open()
		if err != nil {
			_ = writer.Close()
			_ = tmpFile.Close()
			return fmt.Errorf("打开 PPTX 部件 %s 失败: %w", part.Name, err)
		}
		_, copyErr := io.Copy(out, in)
		closeErr := in.Close()
		if copyErr != nil || closeErr != nil {
			_ = writer.Close()
			_ = tmpFile.Close()
			if copyErr != nil {
				return fmt.Errorf("复制 PPTX 部件 %s 失败: %w", part.Name, copyErr)
			}
			return fmt.Errorf("关闭 PPTX 部件 %s 失败: %w", part.Name, closeErr)
		}
	}
	if err := writer.Close(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("完成 PPTX 几何补丁失败: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("同步 PPTX 几何补丁失败: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("关闭 PPTX 几何补丁失败: %w", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("安装 PPTX 几何补丁失败: %w", err)
	}
	return nil
}

func patchSlidePresentationData(data []byte, slidePart string, slideIndex int, pkg *pptxPackage, transforms map[uint32]presentationTransformOverride, crops map[uint32]document.PresentationCrop, styles map[uint32]document.PresentationElementStylePatch) ([]byte, error) {
	shapes, err := slideShapeSpans(data)
	if err != nil {
		return nil, err
	}
	ids := make([]uint32, 0, len(transforms)+len(crops)+len(styles))
	seen := make(map[uint32]bool, len(transforms)+len(crops)+len(styles))
	for id := range transforms {
		ids = append(ids, id)
		seen[id] = true
	}
	for id := range crops {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	for id := range styles {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var replacements []shapeXMLReplacement
	for _, id := range ids {
		var match *slideShapeSpan
		count := 0
		for index := range shapes {
			if shapes[index].ID == id {
				match = &shapes[index]
				count++
			}
		}
		if count != 1 {
			if count == 0 {
				return nil, fmt.Errorf("形状 ID %d 不存在", id)
			}
			return nil, fmt.Errorf("形状 ID %d 不唯一", id)
		}
		span := *match
		if span.Kind != "sp" && span.Kind != "pic" {
			return nil, fmt.Errorf("形状 ID %d 的类型 %q 不支持几何编辑", id, span.Kind)
		}
		fragment := data[span.Start:span.End]
		if override, ok := transforms[id]; ok {
			if err := validatePresentationTransform(override.Bounds, override.RotationDeg); err != nil {
				return nil, fmt.Errorf("形状 ID %d: %w", id, err)
			}
			fragment, err = patchShapeTransformFragment(fragment, slidePart, slideIndex, id, pkg, override)
			if err != nil {
				return nil, fmt.Errorf("形状 ID %d: %w", id, err)
			}
		}
		if crop, ok := crops[id]; ok {
			if span.Kind != "pic" {
				return nil, fmt.Errorf("形状 ID %d 不是可裁剪图片", id)
			}
			if err := validatePresentationCrop(crop); err != nil {
				return nil, fmt.Errorf("形状 ID %d: %w", id, err)
			}
			relID, err := pictureEmbedRelationship(fragment)
			if err != nil {
				return nil, fmt.Errorf("形状 ID %d: %w", id, err)
			}
			if _, ok := pkg.imagePartForRelationship(slidePart, relID); !ok {
				return nil, fmt.Errorf("形状 ID %d 没有有效的内部图片关系", id)
			}
			fragment, err = patchShapePictureCropFragment(fragment, crop)
			if err != nil {
				return nil, fmt.Errorf("形状 ID %d: %w", id, err)
			}
		}
		if style, ok := styles[id]; ok {
			if err := validatePresentationStylePatch(style); err != nil {
				return nil, fmt.Errorf("形状 ID %d 样式无效: %w", id, err)
			}
			if span.Kind == "pic" && presentationStylePatchHasTextOrFill(style) {
				return nil, fmt.Errorf("图片 ID %d 仅支持修改轮廓样式", id)
			}
			if span.Kind != "sp" && span.Kind != "pic" {
				return nil, fmt.Errorf("形状 ID %d 的类型 %q 不支持样式编辑", id, span.Kind)
			}
			fragment, err = patchShapePresentationStyle(fragment, style)
			if err != nil {
				return nil, fmt.Errorf("形状 ID %d 样式: %w", id, err)
			}
		}
		replacements = append(replacements, shapeXMLReplacement{start: span.Start, end: span.End, data: fragment})
	}
	sort.Slice(replacements, func(i, j int) bool { return replacements[i].start > replacements[j].start })
	result := append([]byte(nil), data...)
	for _, replacement := range replacements {
		result = append(result[:replacement.start], append(replacement.data, result[replacement.end:]...)...)
	}
	return result, nil
}

func pictureEmbedRelationship(fragment []byte) (string, error) {
	var picture pptxXMLNode
	if err := xml.Unmarshal(fragment, &picture); err != nil {
		return "", fmt.Errorf("解析图片对象 XML 失败: %w", err)
	}
	fill := picture.child("blipFill")
	if fill == nil {
		return "", fmt.Errorf("图片对象缺少 p:blipFill")
	}
	blip := fill.child("blip")
	if blip == nil {
		return "", fmt.Errorf("图片对象缺少 a:blip")
	}
	for _, attr := range blip.Attrs {
		if attr.Name.Local == "embed" && attr.Value != "" {
			return attr.Value, nil
		}
	}
	return "", fmt.Errorf("图片对象缺少内部图片关系")
}
