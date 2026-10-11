package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

// patchOPCRelationshipTarget changes one relationship target while preserving
// every other byte in the relationship part, including unknown extensions.
func patchOPCRelationshipTarget(data []byte, relationshipType, target string) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	matchCount := 0
	valueStart, valueEnd := -1, -1
	external := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解析 relationships XML 失败: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			continue
		}
		var kind, targetMode string
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "Type":
				kind = attr.Value
			case "TargetMode":
				targetMode = attr.Value
			}
		}
		if kind != relationshipType {
			continue
		}
		matchCount++
		if matchCount > 1 {
			return nil, fmt.Errorf("找到多个目标类型为 %q 的关系", relationshipType)
		}
		external = strings.EqualFold(targetMode, "External")
		offset := int(decoder.InputOffset())
		if offset <= 0 || offset > len(data) {
			return nil, fmt.Errorf("无法定位关系起始标签")
		}
		startOffset := bytes.LastIndex(data[:offset], []byte("<"))
		if startOffset < 0 {
			return nil, fmt.Errorf("无法定位关系起始标签")
		}
		startTag := data[startOffset:offset]
		startInTag, endInTag, found, err := xmlAttributeValueSpan(startTag, "Target")
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("目标关系缺少 Target 属性")
		}
		valueStart, valueEnd = startOffset+startInTag, startOffset+endInTag
	}
	if matchCount == 0 {
		return nil, fmt.Errorf("未找到关系类型 %q", relationshipType)
	}
	if external {
		return nil, fmt.Errorf("不能重定向外部关系")
	}
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(target)); err != nil {
		return nil, fmt.Errorf("转义关系目标失败: %w", err)
	}
	patched := make([]byte, 0, len(data)-max(valueEnd-valueStart, 0)+escaped.Len())
	patched = append(patched, data[:valueStart]...)
	patched = append(patched, escaped.Bytes()...)
	patched = append(patched, data[valueEnd:]...)
	return patched, nil
}

func xmlAttributeValueSpan(startTag []byte, wanted string) (int, int, bool, error) {
	if len(startTag) < 3 || startTag[0] != '<' {
		return 0, 0, false, fmt.Errorf("关系 XML 起始标签无效")
	}
	index := 1
	for index < len(startTag) && !isXMLSpace(startTag[index]) && startTag[index] != '>' && startTag[index] != '/' {
		index++
	}
	for index < len(startTag) {
		for index < len(startTag) && isXMLSpace(startTag[index]) {
			index++
		}
		if index >= len(startTag) || startTag[index] == '>' || startTag[index] == '/' {
			return 0, 0, false, nil
		}
		nameStart := index
		for index < len(startTag) && !isXMLSpace(startTag[index]) && startTag[index] != '=' && startTag[index] != '>' && startTag[index] != '/' {
			index++
		}
		name := string(startTag[nameStart:index])
		for index < len(startTag) && isXMLSpace(startTag[index]) {
			index++
		}
		if index >= len(startTag) || startTag[index] != '=' {
			return 0, 0, false, fmt.Errorf("关系 XML 属性 %q 缺少等号", name)
		}
		index++
		for index < len(startTag) && isXMLSpace(startTag[index]) {
			index++
		}
		if index >= len(startTag) || (startTag[index] != '\'' && startTag[index] != '"') {
			return 0, 0, false, fmt.Errorf("关系 XML 属性 %q 未使用引号", name)
		}
		quote := startTag[index]
		index++
		valueStart := index
		for index < len(startTag) && startTag[index] != quote {
			index++
		}
		if index >= len(startTag) {
			return 0, 0, false, fmt.Errorf("关系 XML 属性 %q 未闭合", name)
		}
		if name == wanted {
			return valueStart, index, true, nil
		}
		index++
	}
	return 0, 0, false, nil
}

func setPPTXPackageRelationshipTarget(pkg *pptxPackage, ownerPart, relationshipType, target string) error {
	ownerPart = strings.TrimPrefix(ownerPart, "/")
	count := 0
	for id, relationship := range pkg.rels[ownerPart] {
		if relationship.Type != relationshipType {
			continue
		}
		count++
		if count > 1 || relationship.External {
			return fmt.Errorf("幻灯片 %s 的版式关系不唯一或无效", ownerPart)
		}
		relationship.Target = target
		pkg.rels[ownerPart][id] = relationship
	}
	if count == 0 {
		return fmt.Errorf("幻灯片 %s 缺少版式关系", ownerPart)
	}
	return nil
}

func (d *Document) presentationLayoutOverridesByIndexLocked() (map[int]string, error) {
	result := make(map[int]string)
	if d.pres == nil || len(d.slideLayoutChanges) == 0 {
		return result, nil
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return nil, fmt.Errorf("读取幻灯片版式覆盖失败: %w", err)
	}
	for index, slide := range slides {
		if layoutPart := d.slideLayoutChanges[uint32(slide.ID())]; layoutPart != "" {
			result[index] = layoutPart
		}
	}
	return result, nil
}

func validatePPTXSlideLayoutEdits(pathName string, layouts map[int]string) error {
	if len(layouts) == 0 {
		return nil
	}
	file, err := os.Open(pathName)
	if err != nil {
		return fmt.Errorf("打开版式校验文件失败: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("读取版式校验文件信息失败: %w", err)
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return fmt.Errorf("解析版式校验文件失败: %w", err)
	}
	pkg := newPPTXPackage(reader)
	for index, expected := range layouts {
		if index < 0 || index >= len(pkg.slideParts) {
			return fmt.Errorf("保存后的版式校验幻灯片序号无效: %d", index)
		}
		if pkg.parts[expected] == nil || pkg.layoutMasterParts[expected] == "" {
			return fmt.Errorf("保存后的版式校验目标不存在: %s", expected)
		}
		actual, ok := pkg.relatedPart(pkg.slideParts[index], pptxSlideLayoutRelationshipType)
		if !ok || actual != expected {
			return fmt.Errorf("保存后的第 %d 张幻灯片未指向请求的版式", index+1)
		}
	}
	return nil
}

func readPackagePartForEdit(pkg *pptxPackage, name string) ([]byte, error) {
	part := pkg.parts[name]
	if part == nil {
		return nil, fmt.Errorf("PPTX 缺少关系部件 %q", name)
	}
	data, err := readOPCPart(part)
	if err != nil {
		return nil, fmt.Errorf("读取关系部件 %q 失败: %w", name, err)
	}
	return data, nil
}
