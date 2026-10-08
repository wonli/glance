package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"glance/internal/document"
)

const drawingMLNamespace = "http://schemas.openxmlformats.org/drawingml/2006/main"

// presentationTransformOverride is an adapter-owned edit layered over the
// parsed presentation. Keeping it separate lets snapshots expose committed
// geometry without mutating the third-party object model.
type presentationTransformOverride struct {
	Bounds      document.PresentationRect
	RotationDeg float64
}

// presentationTransformMetadata retains attributes that matter when a
// placeholder has to materialize a local a:xfrm.
type presentationTransformMetadata struct {
	Bounds             document.PresentationRect
	RotationDeg        float64
	FlipH              bool
	FlipV              bool
	HasXfrm            bool
	AspectRatioLocked  bool
	HasAspectRatioLock bool
}

func validatePresentationTransform(bounds document.PresentationRect, rotationDeg float64) error {
	values := []float64{bounds.X, bounds.Y, bounds.W, bounds.H, rotationDeg}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("对象变换包含非有限数值")
		}
	}
	if bounds.W < 8 || bounds.H < 8 {
		return fmt.Errorf("对象宽高不得小于 8 pt")
	}
	const maxEMU = float64(math.MaxInt64) / 2
	for _, value := range []float64{bounds.X, bounds.Y, bounds.W, bounds.H} {
		if math.Abs(value*emuPerPoint) > maxEMU-1 {
			return fmt.Errorf("对象几何超出 PPTX 范围")
		}
	}
	return nil
}

func normalizePresentationRotation(degrees float64) float64 {
	degrees = math.Mod(degrees, 360)
	if degrees < 0 {
		degrees += 360
	}
	if degrees == 360 || degrees == 0 {
		return 0
	}
	return degrees
}

func pointToEMU(points float64) int64 {
	return int64(math.Round(points * emuPerPoint))
}

func degreesToDrawingML(degrees float64) int64 {
	return int64(math.Round(normalizePresentationRotation(degrees) * 60000))
}

func shapeTransformMetadataFromXML(shape pptxXMLNode) presentationTransformMetadata {
	metadata := presentationTransformMetadata{}
	properties := shape.child("spPr")
	if properties != nil {
		if xfrm := properties.child("xfrm"); xfrm != nil {
			metadata.HasXfrm = true
			metadata.RotationDeg = rotationDegreesFromXfrm(*xfrm)
			metadata.FlipH = xmlBoolDefaultFalse(xfrm.attr("flipH"))
			metadata.FlipV = xmlBoolDefaultFalse(xfrm.attr("flipV"))
		}
	}
	var nonVisual *pptxXMLNode
	if nv := shape.child("nvSpPr"); nv != nil {
		nonVisual = nv.child("cNvSpPr")
	} else if nv := shape.child("nvPicPr"); nv != nil {
		nonVisual = nv.child("cNvPicPr")
	}
	if nonVisual != nil {
		locks := nonVisual.child("spLocks")
		if locks == nil {
			locks = nonVisual.child("picLocks")
		}
		if locks != nil {
			if value, exists := xmlOnOff(locks.attr("noChangeAspect")); exists {
				metadata.HasAspectRatioLock = true
				metadata.AspectRatioLocked = value
			}
		}
	}
	return metadata
}

func rotationDegreesFromXfrm(xfrm pptxXMLNode) float64 {
	value := xfrm.attr("rot")
	if value == "" {
		return 0
	}
	units, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return normalizePresentationRotation(float64(units) / 60000)
}

func xmlOnOff(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "on":
		return true, true
	case "0", "false", "off":
		return false, true
	default:
		return false, false
	}
}

func xmlBoolDefaultFalse(value string) bool {
	result, _ := xmlOnOff(value)
	return result
}

// parseSlideTransformMetadata projects slide-local rotation and explicit
// noChangeAspect metadata by non-visual shape ID.
func parseSlideTransformMetadata(data []byte) map[uint32]presentationTransformMetadata {
	var root pptxXMLNode
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil
	}
	tree := shapeTree(root)
	if tree == nil {
		return nil
	}
	result := make(map[uint32]presentationTransformMetadata)
	for _, shape := range tree.Children {
		if shape.XMLName.Local != "sp" && shape.XMLName.Local != "pic" {
			continue
		}
		id := shapeIDFromXML(shape)
		if id == 0 {
			continue
		}
		result[id] = shapeTransformMetadataFromXML(shape)
	}
	return result
}

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

type pictureCropElementSpan struct {
	start, startTagEnd, end int
	qName                   string
	selfClose               bool
	parent                  string
}

type pictureCropXMLTags struct {
	srcRect       *pictureCropElementSpan
	blipQName     string
	insertAt      int
	blipFillClose int
}

func inspectPictureCropXMLTags(fragment []byte) (pictureCropXMLTags, error) {
	tags := pictureCropXMLTags{insertAt: -1, blipFillClose: -1}
	type frame struct {
		name        xml.Name
		parent      string
		start       int
		startTagEnd int
		selfClose   bool
	}
	decoder := xml.NewDecoder(bytes.NewReader(fragment))
	var stack []frame
	for {
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return pictureCropXMLTags{}, fmt.Errorf("解析图片裁剪 XML 失败: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			start := findNextXMLStart(fragment, before)
			end, err := xmlStartTagEnd(fragment, start)
			if err != nil {
				return pictureCropXMLTags{}, err
			}
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1].name.Local
			}
			selfClose := bytes.HasSuffix(bytes.TrimSpace(fragment[start:end-1]), []byte("/"))
			current := frame{name: value.Name, parent: parent, start: start, startTagEnd: end, selfClose: selfClose}
			if value.Name.Local == "blip" && parent == "blipFill" && tags.blipQName == "" {
				tags.blipQName, err = xmlElementQName(fragment[start:end])
				if err != nil {
					return pictureCropXMLTags{}, err
				}
			}
			if parent == "blipFill" {
				switch value.Name.Local {
				case "srcRect":
					span := &pictureCropElementSpan{start: start, startTagEnd: end, qName: "", selfClose: selfClose, parent: parent}
					span.qName, err = xmlElementQName(fragment[start:end])
					if err != nil {
						return pictureCropXMLTags{}, err
					}
					// The end offset is filled by the matching EndElement below.
					if tags.srcRect == nil {
						tags.srcRect = span
					}
				case "stretch", "tile":
					if tags.insertAt < 0 {
						tags.insertAt = start
					}
				}
			}
			stack = append(stack, current)
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1].name.Local != value.Name.Local {
				return pictureCropXMLTags{}, fmt.Errorf("图片裁剪 XML 元素层级不匹配")
			}
			current := stack[len(stack)-1]
			if current.name.Local == "srcRect" && current.parent == "blipFill" && tags.srcRect != nil && tags.srcRect.start == current.start {
				tags.srcRect.end = int(decoder.InputOffset())
			}
			if current.name.Local == "blipFill" && tags.blipFillClose < 0 {
				tags.blipFillClose = findNextXMLStart(fragment, before)
			}
			stack = stack[:len(stack)-1]
		}
	}
	if tags.blipQName == "" || tags.blipFillClose < 0 {
		return pictureCropXMLTags{}, fmt.Errorf("图片对象缺少有效的 p:blipFill/a:blip")
	}
	if tags.insertAt < 0 {
		tags.insertAt = tags.blipFillClose
	}
	if tags.srcRect != nil && tags.srcRect.end <= tags.srcRect.start {
		return pictureCropXMLTags{}, fmt.Errorf("a:srcRect 元素不完整")
	}
	return tags, nil
}

func patchShapePictureCropFragment(fragment []byte, crop document.PresentationCrop) ([]byte, error) {
	tags, err := inspectPictureCropXMLTags(fragment)
	if err != nil {
		return nil, err
	}
	allZero := crop == (document.PresentationCrop{})
	if tags.srcRect != nil {
		if allZero {
			result := append([]byte(nil), fragment...)
			return append(result[:tags.srcRect.start], result[tags.srcRect.end:]...), nil
		}
		startTag := fragment[tags.srcRect.start:tags.srcRect.startTagEnd]
		updated, err := setXMLStartTagAttributes(startTag, map[string]string{
			"l": strconv.Itoa(crop.Left), "t": strconv.Itoa(crop.Top),
			"r": strconv.Itoa(crop.Right), "b": strconv.Itoa(crop.Bottom),
		})
		if err != nil {
			return nil, err
		}
		result := append([]byte(nil), fragment...)
		return append(result[:tags.srcRect.start], append(updated, result[tags.srcRect.startTagEnd:]...)...), nil
	}
	if allZero {
		return append([]byte(nil), fragment...), nil
	}
	prefix := ""
	if separator := strings.IndexByte(tags.blipQName, ':'); separator >= 0 {
		prefix = tags.blipQName[:separator+1]
	}
	insert := []byte(fmt.Sprintf(`<%ssrcRect l="%d" t="%d" r="%d" b="%d"/>`, prefix, crop.Left, crop.Top, crop.Right, crop.Bottom))
	result := make([]byte, 0, len(fragment)+len(insert))
	result = append(result, fragment[:tags.insertAt]...)
	result = append(result, insert...)
	result = append(result, fragment[tags.insertAt:]...)
	return result, nil
}

type slideShapeSpan struct {
	ID         uint32
	Kind       string
	Start, End int
}

type shapeXMLReplacement struct {
	start, end int
	data       []byte
}

type xmlStackFrame struct {
	name      xml.Name
	shapeSpan int
}

func slideShapeSpans(data []byte) ([]slideShapeSpan, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var stack []xmlStackFrame
	var shapes []slideShapeSpan
	for {
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解析 slide XML 失败: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			start := findNextXMLStart(data, before)
			if start < 0 {
				return nil, fmt.Errorf("无法定位 slide XML 起始标签")
			}
			parentIsTree := len(stack) > 0 && stack[len(stack)-1].name.Local == "spTree"
			spanIndex := -1
			if parentIsTree && (value.Name.Local == "sp" || value.Name.Local == "pic") {
				spanIndex = len(shapes)
				shapes = append(shapes, slideShapeSpan{Kind: value.Name.Local, Start: start})
			} else if len(stack) > 0 {
				spanIndex = stack[len(stack)-1].shapeSpan
			}
			if spanIndex >= 0 && value.Name.Local == "cNvPr" {
				for _, attr := range value.Attr {
					if attr.Name.Local == "id" {
						parsed, parseErr := strconv.ParseUint(attr.Value, 10, 32)
						if parseErr != nil {
							return nil, fmt.Errorf("对象 cNvPr ID 无效: %q", attr.Value)
						}
						shapes[spanIndex].ID = uint32(parsed)
						break
					}
				}
			}
			stack = append(stack, xmlStackFrame{name: value.Name, shapeSpan: spanIndex})
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("slide XML 元素层级不匹配")
			}
			frame := stack[len(stack)-1]
			if frame.name.Local != value.Name.Local {
				return nil, fmt.Errorf("slide XML 元素层级不匹配: %s / %s", frame.name.Local, value.Name.Local)
			}
			if frame.shapeSpan >= 0 && (value.Name.Local == "sp" || value.Name.Local == "pic") {
				shapes[frame.shapeSpan].End = int(decoder.InputOffset())
			}
			stack = stack[:len(stack)-1]
		}
	}
	for _, shape := range shapes {
		if shape.ID == 0 || shape.End <= shape.Start {
			return nil, fmt.Errorf("slide XML 中存在缺少 ID 或闭合标签的对象")
		}
	}
	return shapes, nil
}

func findNextXMLStart(data []byte, from int) int {
	if from < 0 {
		return -1
	}
	for index := from; index < len(data); index++ {
		if data[index] == '<' {
			return index
		}
	}
	return -1
}

func xmlStartTagEnd(data []byte, start int) (int, error) {
	if start < 0 || start >= len(data) || data[start] != '<' {
		return 0, fmt.Errorf("XML 标签起始位置无效")
	}
	var quote byte
	for index := start + 1; index < len(data); index++ {
		current := data[index]
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			continue
		}
		if current == '>' {
			return index + 1, nil
		}
	}
	return 0, fmt.Errorf("XML 起始标签未闭合")
}

type transformTagSpan struct {
	start, end int
	local      string
	parent     string
	selfClose  bool
}

type shapeTransformTags struct {
	spPr *transformTagSpan
	xfrm *transformTagSpan
	off  *transformTagSpan
	ext  *transformTagSpan
}

func inspectShapeTransformTags(fragment []byte) (shapeTransformTags, error) {
	decoder := xml.NewDecoder(bytes.NewReader(fragment))
	var stack []xml.Name
	var found shapeTransformTags
	for {
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return shapeTransformTags{}, fmt.Errorf("解析对象 XML 失败: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			start := findNextXMLStart(fragment, before)
			end, err := xmlStartTagEnd(fragment, start)
			if err != nil {
				return shapeTransformTags{}, err
			}
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1].Local
			}
			selfClose := bytes.HasSuffix(bytes.TrimSpace(fragment[start:end-1]), []byte("/"))
			span := transformTagSpan{start: start, end: end, local: value.Name.Local, parent: parent, selfClose: selfClose}
			switch {
			case value.Name.Local == "spPr" && found.spPr == nil:
				found.spPr = &span
			case value.Name.Local == "xfrm" && parent == "spPr" && found.xfrm == nil:
				found.xfrm = &span
			case value.Name.Local == "off" && parent == "xfrm" && found.off == nil:
				found.off = &span
			case value.Name.Local == "ext" && parent == "xfrm" && found.ext == nil:
				found.ext = &span
			}
			stack = append(stack, value.Name)
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1].Local != value.Name.Local {
				return shapeTransformTags{}, fmt.Errorf("对象 XML 元素层级不匹配")
			}
			stack = stack[:len(stack)-1]
		}
	}
	if found.spPr == nil {
		return shapeTransformTags{}, fmt.Errorf("对象缺少 p:spPr")
	}
	return found, nil
}

func patchShapeTransformFragment(fragment []byte, slidePart string, slideIndex int, id uint32, pkg *pptxPackage, override presentationTransformOverride) ([]byte, error) {
	tags, err := inspectShapeTransformTags(fragment)
	if err != nil {
		return nil, err
	}
	if tags.xfrm == nil {
		inherited, err := inheritedPlaceholderTransform(pkg, slidePart, id)
		if err != nil {
			return nil, err
		}
		opening := append([]byte(nil), fragment[tags.spPr.start:tags.spPr.end]...)
		spPrName, err := xmlElementQName(opening)
		if err != nil {
			return nil, err
		}
		xfrm := materializedTransformXML(override, inherited)
		if tags.spPr.selfClose {
			opening = bytes.TrimSuffix(opening, []byte("/>"))
			opening = append(opening, '>')
			result := make([]byte, 0, len(fragment)+len(xfrm)+len(spPrName)+4)
			result = append(result, fragment[:tags.spPr.start]...)
			result = append(result, opening...)
			result = append(result, xfrm...)
			result = append(result, []byte("</"+spPrName+">")...)
			result = append(result, fragment[tags.spPr.end:]...)
			return result, nil
		}
		result := make([]byte, 0, len(fragment)+len(xfrm))
		result = append(result, fragment[:tags.spPr.end]...)
		result = append(result, xfrm...)
		result = append(result, fragment[tags.spPr.end:]...)
		return result, nil
	}
	if tags.off == nil || tags.ext == nil || tags.xfrm.selfClose {
		return nil, fmt.Errorf("对象的 a:xfrm 缺少 a:off 或 a:ext")
	}
	updates := []struct {
		span    *transformTagSpan
		changes map[string]string
	}{
		{span: tags.xfrm, changes: map[string]string{"rot": strconv.FormatInt(degreesToDrawingML(override.RotationDeg), 10)}},
		{span: tags.off, changes: map[string]string{
			"x": strconv.FormatInt(pointToEMU(override.Bounds.X), 10),
			"y": strconv.FormatInt(pointToEMU(override.Bounds.Y), 10),
		}},
		{span: tags.ext, changes: map[string]string{
			"cx": strconv.FormatInt(pointToEMU(override.Bounds.W), 10),
			"cy": strconv.FormatInt(pointToEMU(override.Bounds.H), 10),
		}},
	}
	for _, update := range updates {
		if update.span.selfClose && update.span.local == "xfrm" {
			return nil, fmt.Errorf("对象的 a:xfrm 缺少 a:off 或 a:ext")
		}
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].span.start > updates[j].span.start })
	result := append([]byte(nil), fragment...)
	for _, update := range updates {
		startTag := result[update.span.start:update.span.end]
		patchedTag, err := setXMLStartTagAttributes(startTag, update.changes)
		if err != nil {
			return nil, err
		}
		replacement := shapeXMLReplacement{start: update.span.start, end: update.span.end, data: patchedTag}
		result = append(result[:replacement.start], append(replacement.data, result[replacement.end:]...)...)
	}
	return result, nil
}

func inheritedPlaceholderTransform(pkg *pptxPackage, slidePart string, id uint32) (presentationTransformMetadata, error) {
	slideRoot, ok := pkg.xmlPart(slidePart)
	if !ok {
		return presentationTransformMetadata{}, fmt.Errorf("无法读取幻灯片 XML")
	}
	tree := shapeTree(slideRoot)
	if tree == nil {
		return presentationTransformMetadata{}, fmt.Errorf("幻灯片缺少形状树")
	}
	var target *pptxXMLNode
	for index := range tree.Children {
		shape := &tree.Children[index]
		if shape.XMLName.Local == "sp" && shapeIDFromXML(*shape) == id {
			if target != nil {
				return presentationTransformMetadata{}, fmt.Errorf("占位符 ID %d 不唯一", id)
			}
			target = shape
		}
	}
	if target == nil {
		return presentationTransformMetadata{}, fmt.Errorf("未找到幻灯片形状 ID %d", id)
	}
	slideRecord, ok := placeholderRecordForShape(*target)
	if !ok || slideRecord.hasXfrm {
		return presentationTransformMetadata{}, fmt.Errorf("形状 ID %d 没有可继承的占位符几何", id)
	}
	layoutPart, _ := pkg.relatedPart(slidePart, pptxSlideLayoutRelationshipType)
	masterPart, _ := pkg.relatedPart(layoutPart, pptxSlideMasterRelationshipType)
	layoutRoot, _ := pkg.xmlPart(layoutPart)
	if record, found, ambiguous := uniquePlaceholderRecord(shapeTree(layoutRoot), slideRecord.key); ambiguous {
		return presentationTransformMetadata{}, fmt.Errorf("占位符 ID %d 的版式几何不唯一", id)
	} else if found && record.hasXfrm && validPresentationRect(record.bounds) {
		metadata := record.transform
		metadata.Bounds = record.bounds
		return metadata, nil
	}
	masterRoot, _ := pkg.xmlPart(masterPart)
	if record, found, ambiguous := uniquePlaceholderRecord(shapeTree(masterRoot), slideRecord.key); ambiguous {
		return presentationTransformMetadata{}, fmt.Errorf("占位符 ID %d 的母版几何不唯一", id)
	} else if found && record.hasXfrm && validPresentationRect(record.bounds) {
		metadata := record.transform
		metadata.Bounds = record.bounds
		return metadata, nil
	}
	return presentationTransformMetadata{}, fmt.Errorf("占位符 ID %d 没有有效的继承几何", id)
}

func materializedTransformXML(override presentationTransformOverride, inherited presentationTransformMetadata) []byte {
	attrs := []string{
		`xmlns="` + drawingMLNamespace + `"`,
		`rot="` + strconv.FormatInt(degreesToDrawingML(override.RotationDeg), 10) + `"`,
	}
	if inherited.FlipH {
		attrs = append(attrs, `flipH="1"`)
	}
	if inherited.FlipV {
		attrs = append(attrs, `flipV="1"`)
	}
	return []byte(fmt.Sprintf(`<xfrm %s><off x="%d" y="%d"/><ext cx="%d" cy="%d"/></xfrm>`, strings.Join(attrs, " "),
		pointToEMU(override.Bounds.X), pointToEMU(override.Bounds.Y), pointToEMU(override.Bounds.W), pointToEMU(override.Bounds.H)))
}

func xmlElementQName(startTag []byte) (string, error) {
	if len(startTag) < 3 || startTag[0] != '<' {
		return "", fmt.Errorf("XML 元素起始标签无效")
	}
	end := 1
	for end < len(startTag) && !isXMLSpace(startTag[end]) && startTag[end] != '/' && startTag[end] != '>' {
		end++
	}
	if end == 1 {
		return "", fmt.Errorf("XML 元素名称为空")
	}
	return string(startTag[1:end]), nil
}

func isXMLSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func setXMLStartTagAttributes(startTag []byte, changes map[string]string) ([]byte, error) {
	if len(startTag) < 3 || startTag[0] != '<' || startTag[len(startTag)-1] != '>' {
		return nil, fmt.Errorf("XML 属性起始标签无效")
	}
	gt := len(startTag) - 1
	attributeLimit := gt
	for attributeLimit > 0 && isXMLSpace(startTag[attributeLimit-1]) {
		attributeLimit--
	}
	selfClose := attributeLimit > 0 && startTag[attributeLimit-1] == '/'
	if selfClose {
		attributeLimit--
	}
	nameEnd := 1
	for nameEnd < attributeLimit && !isXMLSpace(startTag[nameEnd]) && startTag[nameEnd] != '/' {
		nameEnd++
	}
	result := append([]byte(nil), startTag[:nameEnd]...)
	found := make(map[string]bool, len(changes))
	index := nameEnd
	for index < attributeLimit {
		spaceStart := index
		for index < attributeLimit && isXMLSpace(startTag[index]) {
			index++
		}
		result = append(result, startTag[spaceStart:index]...)
		if index >= attributeLimit {
			break
		}
		attributeStart := index
		for index < attributeLimit && !isXMLSpace(startTag[index]) && startTag[index] != '=' {
			index++
		}
		if attributeStart == index {
			return nil, fmt.Errorf("无法解析 XML 属性")
		}
		qualifiedName := string(startTag[attributeStart:index])
		for index < attributeLimit && isXMLSpace(startTag[index]) {
			index++
		}
		if index >= attributeLimit || startTag[index] != '=' {
			return nil, fmt.Errorf("XML 属性 %s 缺少等号", qualifiedName)
		}
		index++
		for index < attributeLimit && isXMLSpace(startTag[index]) {
			index++
		}
		if index >= attributeLimit || (startTag[index] != '\'' && startTag[index] != '"') {
			return nil, fmt.Errorf("XML 属性 %s 缺少引号", qualifiedName)
		}
		quote := startTag[index]
		valueStart := index + 1
		index = valueStart
		for index < attributeLimit && startTag[index] != quote {
			index++
		}
		if index >= attributeLimit {
			return nil, fmt.Errorf("XML 属性 %s 引号未闭合", qualifiedName)
		}
		localName := qualifiedName
		if colon := strings.LastIndexByte(localName, ':'); colon >= 0 {
			localName = localName[colon+1:]
		}
		if value, ok := changes[localName]; ok {
			result = append(result, startTag[attributeStart:valueStart]...)
			result = append(result, value...)
			result = append(result, quote)
			found[localName] = true
		} else {
			result = append(result, startTag[attributeStart:index+1]...)
		}
		index++
	}
	missing := make([]string, 0, len(changes)-len(found))
	for name := range changes {
		if !found[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		value := changes[name]
		result = append(result, ' ')
		result = append(result, name...)
		result = append(result, `="`...)
		result = append(result, value...)
		result = append(result, '"')
	}
	result = append(result, startTag[attributeLimit:gt]...)
	result = append(result, '>')
	return result, nil
}

func samePresentationRect(actual, expected document.PresentationRect) bool {
	const tolerance = 0.5 / emuPerPoint
	return math.Abs(actual.X-expected.X) <= tolerance && math.Abs(actual.Y-expected.Y) <= tolerance &&
		math.Abs(actual.W-expected.W) <= tolerance && math.Abs(actual.H-expected.H) <= tolerance
}
