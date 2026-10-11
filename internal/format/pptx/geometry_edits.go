package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"glance/internal/document"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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

func hasPresentationBackgroundOverrides(overrides []presentationBackgroundOverride) bool {
	for _, override := range overrides {
		if override.Set {
			return true
		}
	}
	return false
}

func hasPresentationPackageEdits(transforms []map[uint32]presentationTransformOverride, crops []map[uint32]document.PresentationCrop, styles []map[uint32]document.PresentationElementStylePatch, placeholders []map[uint32]string, backgrounds []presentationBackgroundOverride, slideSize presentationSlideSizeOverride, slideLayouts map[int]string, slideTimings map[int][]byte, packageEdits map[string][]byte) bool {
	if len(packageEdits) > 0 {
		return true
	}
	if len(slideLayouts) > 0 {
		return true
	}
	if len(slideTimings) > 0 {
		return true
	}
	if hasPresentationTransformOverrides(transforms) || hasPresentationCropOverrides(crops) {
		return true
	}
	if hasPresentationBackgroundOverrides(backgrounds) || slideSize.Set {
		return true
	}
	for _, slide := range styles {
		if len(slide) > 0 {
			return true
		}
	}
	for _, slide := range placeholders {
		if len(slide) > 0 {
			return true
		}
	}
	return false
}

// patchPresentationEdits applies adapter-owned geometry and picture crops to a
// serialized package. Other package entries are copied without modification.
func patchPresentationEdits(sourcePath, targetPath string, transforms []map[uint32]presentationTransformOverride, crops []map[uint32]document.PresentationCrop, styles []map[uint32]document.PresentationElementStylePatch, placeholders []map[uint32]string, backgrounds []presentationBackgroundOverride, slideSize presentationSlideSizeOverride, slideLayouts map[int]string, slideTimings map[int][]byte, packageEdits map[string][]byte) error {
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
	patchedParts := make(map[string][]byte)
	if slideSize.Set {
		part := pkg.parts["ppt/presentation.xml"]
		if part == nil {
			return fmt.Errorf("PPTX 缺少演示文稿部件 ppt/presentation.xml")
		}
		data, err := readOPCPart(part)
		if err != nil {
			return fmt.Errorf("读取演示文稿尺寸失败: %w", err)
		}
		patched, err := patchPresentationSlideSize(data, slideSize)
		if err != nil {
			return fmt.Errorf("更新演示文稿尺寸失败: %w", err)
		}
		patchedParts["ppt/presentation.xml"] = patched
	}
	for name, data := range packageEdits {
		patchedParts[strings.TrimPrefix(name, "/")] = data
	}
	for slideIndex, layoutPart := range slideLayouts {
		if slideIndex < 0 || slideIndex >= len(pkg.slideParts) {
			return fmt.Errorf("版式补丁的幻灯片序号无效: %d", slideIndex)
		}
		if pkg.parts[layoutPart] == nil || pkg.layoutMasterParts[layoutPart] == "" {
			return fmt.Errorf("版式补丁目标不在当前文稿中: %s", layoutPart)
		}
		slidePart := pkg.slideParts[slideIndex]
		relsPart := relationshipPartPath(slidePart)
		relsData, exists := packageEditPart(packageEdits, relsPart)
		if !exists {
			relsData, err = readPackagePartForEdit(pkg, relsPart)
			if err != nil {
				return err
			}
		}
		target := relativeOPCTarget(slidePart, layoutPart)
		patched, err := patchOPCRelationshipTarget(relsData, pptxSlideLayoutRelationshipType, target)
		if err != nil {
			return fmt.Errorf("更新幻灯片 %d 的版式关系失败: %w", slideIndex+1, err)
		}
		if err := setPPTXPackageRelationshipTarget(pkg, slidePart, pptxSlideLayoutRelationshipType, target); err != nil {
			return err
		}
		patchedParts[relsPart] = patched
	}
	slideCount := max(len(transforms), len(crops), len(styles), len(placeholders), len(backgrounds))
	for slideIndex := range slideTimings {
		slideCount = max(slideCount, slideIndex+1)
	}
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
		var slidePlaceholders map[uint32]string
		if slideIndex < len(placeholders) {
			slidePlaceholders = placeholders[slideIndex]
		}
		var slideBackground presentationBackgroundOverride
		if slideIndex < len(backgrounds) {
			slideBackground = backgrounds[slideIndex]
		}
		slideTiming := slideTimings[slideIndex]
		if len(slideTransforms) == 0 && len(slideCrops) == 0 && len(slideStyles) == 0 && len(slidePlaceholders) == 0 && !slideBackground.Set && len(slideTiming) == 0 {
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
		patched, err := patchSlidePresentationData(data, slidePart, slideIndex, pkg, slideTransforms, slideCrops, slideStyles, slidePlaceholders)
		if err != nil {
			return fmt.Errorf("更新幻灯片 %d 对象失败: %w", slideIndex, err)
		}
		if slideBackground.Set {
			patched, err = patchSlideBackground(patched, slideBackground.ColorHex)
			if err != nil {
				return fmt.Errorf("更新幻灯片 %d 背景失败: %w", slideIndex, err)
			}
		}
		if len(slideTiming) > 0 {
			patched, err = patchSlideTiming(patched, slideTiming)
			if err != nil {
				return fmt.Errorf("更新幻灯片 %d 动画时序失败: %w", slideIndex+1, err)
			}
		}
		patchedParts[slidePart] = patched
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
	written := make(map[string]bool, len(patchedParts))
	for _, part := range reader.File {
		header := part.FileHeader
		out, err := writer.CreateHeader(&header)
		if err != nil {
			_ = writer.Close()
			_ = tmpFile.Close()
			return fmt.Errorf("创建 PPTX 部件 %s 失败: %w", part.Name, err)
		}
		partName := strings.TrimPrefix(part.Name, "/")
		if data, ok := patchedParts[partName]; ok {
			if _, err := out.Write(data); err != nil {
				_ = writer.Close()
				_ = tmpFile.Close()
				return fmt.Errorf("写入已更新的 PPTX 部件 %s 失败: %w", part.Name, err)
			}
			written[partName] = true
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
	partNames := make([]string, 0, len(patchedParts))
	for name := range patchedParts {
		if !written[name] {
			partNames = append(partNames, name)
		}
	}
	sort.Strings(partNames)
	for _, name := range partNames {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0644)
		header.Modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
		out, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			_ = tmpFile.Close()
			return fmt.Errorf("创建新增 PPTX 部件 %s 失败: %w", name, err)
		}
		if _, err := out.Write(patchedParts[name]); err != nil {
			_ = writer.Close()
			_ = tmpFile.Close()
			return fmt.Errorf("写入新增 PPTX 部件 %s 失败: %w", name, err)
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

func packageEditPart(edits map[string][]byte, partName string) ([]byte, bool) {
	for name, data := range edits {
		if strings.TrimPrefix(name, "/") == partName {
			return data, true
		}
	}
	return nil, false
}

func patchSlidePresentationData(data []byte, slidePart string, slideIndex int, pkg *pptxPackage, transforms map[uint32]presentationTransformOverride, crops map[uint32]document.PresentationCrop, styles map[uint32]document.PresentationElementStylePatch, placeholders map[uint32]string) ([]byte, error) {
	shapes, err := slideShapeSpans(data)
	if err != nil {
		return nil, err
	}
	ids := make([]uint32, 0, len(transforms)+len(crops)+len(styles)+len(placeholders))
	seen := make(map[uint32]bool, len(transforms)+len(crops)+len(styles)+len(placeholders))
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
			seen[id] = true
		}
	}
	for id := range placeholders {
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
		if placeholderType, ok := placeholders[id]; ok {
			if span.Kind != "sp" {
				return nil, fmt.Errorf("占位符 ID %d 不是文本形状", id)
			}
			fragment, err = patchShapePlaceholderTypeFragment(fragment, placeholderType)
			if err != nil {
				return nil, fmt.Errorf("形状 ID %d 占位符: %w", id, err)
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

func patchShapePlaceholderTypeFragment(fragment []byte, placeholderType string) ([]byte, error) {
	switch placeholderType {
	case "title", "ctrTitle", "subTitle", "body":
	default:
		return nil, fmt.Errorf("不支持的占位符类型 %q", placeholderType)
	}
	var shape pptxXMLNode
	if err := xml.Unmarshal(fragment, &shape); err != nil {
		return nil, fmt.Errorf("解析文本形状 XML 失败: %w", err)
	}
	if shape.XMLName.Local != "sp" {
		return nil, fmt.Errorf("对象不是文本形状")
	}
	nvSpPr := shape.child("nvSpPr")
	if nvSpPr == nil {
		return nil, fmt.Errorf("文本形状缺少 p:nvSpPr")
	}
	nvPr := nvSpPr.child("nvPr")
	if nvPr == nil {
		return nil, fmt.Errorf("文本形状缺少 p:nvPr")
	}
	if existing := nvPr.child("ph"); existing != nil {
		existingType := existing.attr("type")
		if existingType == "" {
			existingType = "obj"
		}
		if existingType != placeholderType {
			return nil, fmt.Errorf("已有占位符类型 %q 与目标类型 %q 不符", existingType, placeholderType)
		}
		return fragment, nil
	}

	const nvPrStart = "<p:nvPr"
	start := bytes.Index(fragment, []byte(nvPrStart))
	if start < 0 {
		return nil, fmt.Errorf("文本形状 XML 缺少 p:nvPr 起始标签")
	}
	end, err := xmlStartTagEnd(fragment, start)
	if err != nil {
		return nil, fmt.Errorf("读取 p:nvPr 起始标签失败: %w", err)
	}
	opening := fragment[start:end]
	trimmedOpening := bytes.TrimRight(opening, " \t\r\n")
	if bytes.HasSuffix(trimmedOpening, []byte("/>")) {
		prefix := bytes.TrimSuffix(trimmedOpening, []byte("/>"))
		updated := make([]byte, 0, len(fragment)+len(placeholderType)+24)
		updated = append(updated, fragment[:start]...)
		updated = append(updated, prefix...)
		updated = append(updated, '>')
		updated = append(updated, `<p:ph type="`...)
		updated = append(updated, placeholderType...)
		updated = append(updated, `"/></p:nvPr>`...)
		updated = append(updated, fragment[end:]...)
		return updated, nil
	}
	const nvPrEnd = "</p:nvPr>"
	close := bytes.Index(fragment[end:], []byte(nvPrEnd))
	if close < 0 {
		return nil, fmt.Errorf("文本形状 XML 缺少 p:nvPr 结束标签")
	}
	insertAt := end + close
	updated := make([]byte, 0, len(fragment)+len(placeholderType)+24)
	updated = append(updated, fragment[:insertAt]...)
	updated = append(updated, `<p:ph type="`...)
	updated = append(updated, placeholderType...)
	updated = append(updated, `"/>`...)
	updated = append(updated, fragment[insertAt:]...)
	return updated, nil
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
