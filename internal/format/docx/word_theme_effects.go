package docx

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"glance/internal/document"
)

const wordThemeRelationshipSuffix = "/theme"
const drawingMLThemeNamespace = "http://schemas.openxmlformats.org/drawingml/2006/main"

var _ document.WordThemeEffectsEditor = (*Document)(nil)

func (d *Document) CanSetWordThemeEffects() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.checkOpen() != nil {
		return false
	}
	_, _, err := locateWordThemeEffects(d.parts)
	return err == nil
}

// SetWordThemeEffects replaces the three standard effects in the DOCX theme.
// Direct object effects are preserved; only objects using theme effect
// references are affected when the document is opened by an Office editor.
func (d *Document) SetWordThemeEffects(profile string) (string, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", false, err
	}
	if !validWordThemeEffectProfile(profile) {
		return "", false, fmt.Errorf("Word 主题效果预设无效")
	}
	partName, list, err := locateWordThemeEffects(d.parts)
	if err != nil {
		return "", false, err
	}
	partValue, exists := findPackagePart(d.parts, partName)
	if !exists {
		return "", false, fmt.Errorf("DOCX 主题部件缺失: %s", partName)
	}
	prefix := elementPrefix(list.rawName)
	wanted := wordThemeEffectStyles(prefix, profile)
	previous := "custom"
	for _, candidate := range []string{"subtle", "moderate", "intense"} {
		styles := wordThemeEffectStyles(prefix, candidate)
		if wordThemeStylesMatch(partValue.data, list, styles) {
			previous = candidate
			break
		}
	}
	if wordThemeStylesMatch(partValue.data, list, wanted) {
		return previous, false, nil
	}
	replacements := make([]xmlReplacement, 0, len(list.children))
	for index, child := range list.children {
		replacements = append(replacements, xmlReplacement{
			start: child.start,
			end:   child.end,
			value: []byte(wanted[index]),
		})
	}
	updated, err := applyXMLReplacements(partValue.data, replacements)
	if err != nil {
		return previous, false, fmt.Errorf("更新 Word 主题效果失败: %w", err)
	}
	if err := d.commitWordPackageChange(map[string]*part{
		partName: d.partWithData(partName, updated),
	}); err != nil {
		return previous, false, err
	}
	return previous, true, nil
}

func locateWordThemeEffects(parts []part) (string, *wordXMLNode, error) {
	relationshipIndex := packagePartIndex(parts, "word/_rels/document.xml.rels")
	if relationshipIndex < 0 {
		return "", nil, errors.New("DOCX 没有主题关系，无法设置主题效果")
	}
	relationships, err := parseWordXML(parts[relationshipIndex].data)
	if err != nil {
		return "", nil, fmt.Errorf("解析 DOCX 主题关系失败: %w", err)
	}
	themePart := ""
	themeRelationships := 0
	for _, relationship := range relationships.children {
		if relationship.name.Local != "Relationship" || !strings.HasSuffix(attrValue(relationship, "Type"), wordThemeRelationshipSuffix) {
			continue
		}
		themeRelationships++
		if strings.EqualFold(attrValue(relationship, "TargetMode"), "External") {
			return "", nil, errors.New("DOCX 外部主题关系不支持修改")
		}
		themePart = resolveDocumentTarget(attrValue(relationship, "Target"))
	}
	if themeRelationships != 1 || themePart == "" {
		return "", nil, errors.New("DOCX 主题关系缺失或不唯一，拒绝修改")
	}
	themeIndex := packagePartIndex(parts, themePart)
	if themeIndex < 0 {
		return "", nil, fmt.Errorf("DOCX 主题关系目标不存在: %s", themePart)
	}
	themeRoot, err := parseWordXML(parts[themeIndex].data)
	if err != nil {
		return "", nil, fmt.Errorf("解析 DOCX 主题失败: %w", err)
	}
	themeElements := firstChild(themeRoot, "themeElements")
	formatScheme := firstChild(themeElements, "fmtScheme")
	effectList := firstChild(formatScheme, "effectStyleLst")
	if themeRoot.name.Local != "theme" || themeRoot.name.Space != drawingMLThemeNamespace || themeElements == nil || formatScheme == nil || themeElements.name.Space != drawingMLThemeNamespace || formatScheme.name.Space != drawingMLThemeNamespace ||
		effectList == nil || effectList.name.Space != drawingMLThemeNamespace || len(effectList.children) != 3 {
		return "", nil, errors.New("DOCX 主题效果不是可安全编辑的三项标准列表")
	}
	for _, child := range effectList.children {
		if child.name.Space != effectList.name.Space || child.name.Local != "effectStyle" {
			return "", nil, errors.New("DOCX 主题效果列表包含未知结构，拒绝修改")
		}
		for _, effectChild := range child.children {
			if effectChild.name.Space != drawingMLThemeNamespace || (effectChild.name.Local != "effectLst" && effectChild.name.Local != "scene3d" && effectChild.name.Local != "sp3d") {
				return "", nil, errors.New("DOCX 主题效果项包含未知扩展，拒绝修改")
			}
		}
	}
	return themePart, effectList, nil
}

func validWordThemeEffectProfile(profile string) bool {
	switch profile {
	case "subtle", "moderate", "intense":
		return true
	default:
		return false
	}
}

func wordThemeEffectStyles(prefix, profile string) []string {
	q := func(local string) string {
		if prefix == "" {
			return local
		}
		return prefix + ":" + local
	}
	intensity := map[string][3]int{"subtle": {7000, 11000, 15000}, "moderate": {12000, 20000, 28000}, "intense": {22000, 36000, 52000}}[profile]
	styles := make([]string, 0, len(intensity))
	for index, alpha := range intensity {
		blur := []int{50800, 101600, 152400}[index]
		distance := []int{12700, 25400, 38100}[index]
		style := fmt.Sprintf(`<%s><%s><%s blurRad="%d" dist="%d" dir="2700000" algn="ctr" rotWithShape="0"><%s val="000000"><%s val="%d"/></%s></%s></%s></%s>`,
			q("effectStyle"), q("effectLst"), q("outerShdw"), blur, distance, q("srgbClr"), q("alpha"), alpha, q("srgbClr"), q("outerShdw"), q("effectLst"), q("effectStyle"))
		styles = append(styles, style)
	}
	return styles
}

func wordThemeStylesMatch(data []byte, list *wordXMLNode, expected []string) bool {
	if list == nil || len(list.children) != len(expected) {
		return false
	}
	for index, child := range list.children {
		if !bytes.Equal(data[child.start:child.end], []byte(expected[index])) {
			return false
		}
	}
	return true
}
