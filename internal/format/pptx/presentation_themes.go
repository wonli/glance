package pptx

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"strings"
)

const presentationThemeNamespace = "http://schemas.openxmlformats.org/drawingml/2006/main"

type presentationThemeProfile struct {
	Name      string
	Palette   string
	MajorFont string
	MinorFont string
}

type presentationThemePalette struct {
	Name   string
	Colors map[string]string
}

var presentationThemeColorOrder = []string{
	"dk1", "lt1", "dk2", "lt2", "accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "hlink", "folHlink",
}

func presentationThemePalettes() map[string]presentationThemePalette {
	return map[string]presentationThemePalette{
		"office": {Name: "Office", Colors: map[string]string{
			"dk1": "000000", "lt1": "FFFFFF", "dk2": "44546A", "lt2": "E7E6E6",
			"accent1": "4472C4", "accent2": "ED7D31", "accent3": "A5A5A5", "accent4": "FFC000", "accent5": "5B9BD5", "accent6": "70AD47",
			"hlink": "0563C1", "folHlink": "954F72",
		}},
		"ocean": {Name: "海湾蓝", Colors: map[string]string{
			"dk1": "102A43", "lt1": "FFFFFF", "dk2": "243B53", "lt2": "D9EAF5",
			"accent1": "147D92", "accent2": "2F80ED", "accent3": "00A896", "accent4": "F2C14E", "accent5": "7B61A8", "accent6": "E76F51",
			"hlink": "1261A0", "folHlink": "7A5195",
		}},
		"forest": {Name: "林地绿", Colors: map[string]string{
			"dk1": "173B2F", "lt1": "FFFFFF", "dk2": "31594B", "lt2": "E5EFE8",
			"accent1": "287D5A", "accent2": "78A55A", "accent3": "D9A441", "accent4": "4E8098", "accent5": "9A6FB0", "accent6": "D26A5C",
			"hlink": "1E6C58", "folHlink": "76508E",
		}},
		"warm": {Name: "暖阳橙", Colors: map[string]string{
			"dk1": "3C2F2F", "lt1": "FFFFFF", "dk2": "6B4F4F", "lt2": "F5E9DF",
			"accent1": "C65D3B", "accent2": "E09F3E", "accent3": "9C6644", "accent4": "6C8E7F", "accent5": "7D6B91", "accent6": "D17A8A",
			"hlink": "A3472F", "folHlink": "74506A",
		}},
	}
}

func presentationThemeProfiles() map[string]presentationThemeProfile {
	return map[string]presentationThemeProfile{
		"glance-corporate": {Name: "Glance 企业蓝", Palette: "ocean", MajorFont: "Aptos Display", MinorFont: "Aptos"},
		"glance-forest":    {Name: "Glance 林地", Palette: "forest", MajorFont: "Georgia", MinorFont: "Arial"},
		"glance-warm":      {Name: "Glance 暖阳", Palette: "warm", MajorFont: "Georgia", MinorFont: "Arial"},
	}
}

// SetThemePreset changes only the theme color and font schemes. Effects,
// backgrounds, theme assets, master layouts, and direct object formatting are
// retained. The edited package is reopened so the live adapter uses the new
// theme in subsequent inspection and rendering.
func (d *Document) SetThemePreset(ctx context.Context, themeID string) (bool, error) {
	profile, ok := presentationThemeProfiles()[themeID]
	if !ok {
		return false, fmt.Errorf("未知演示文稿主题 %q", themeID)
	}
	palette, ok := presentationThemePalettes()[profile.Palette]
	if !ok {
		return false, fmt.Errorf("主题 %q 的配色定义缺失", themeID)
	}
	return d.setPresentationTheme(ctx, palette, profile.MajorFont, profile.MinorFont, profile.Name)
}

// SetThemePalette changes theme colors and retains the current font scheme.
func (d *Document) SetThemePalette(ctx context.Context, paletteID string) (bool, error) {
	palette, ok := presentationThemePalettes()[paletteID]
	if !ok {
		return false, fmt.Errorf("未知演示文稿配色 %q", paletteID)
	}
	return d.setPresentationTheme(ctx, palette, "", "", palette.Name)
}

func (d *Document) setPresentationTheme(ctx context.Context, palette presentationThemePalette, majorFont, minorFont, themeName string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return false, fmt.Errorf("演示文稿已关闭")
	}
	if d.presentationComments != nil && d.presentationComments.LoadErr != nil {
		return false, fmt.Errorf("演示文稿包含未能读取的批注状态，拒绝重载文档")
	}
	base, err := d.snapshotBytesLocked()
	if err != nil {
		return false, err
	}
	reader, err := zip.NewReader(bytes.NewReader(base), int64(len(base)))
	if err != nil {
		return false, fmt.Errorf("读取 PPTX 暂存包失败: %w", err)
	}
	pkg := newPPTXPackage(reader)
	themeParts := presentationThemePartNames(pkg)
	if len(themeParts) == 0 {
		return false, fmt.Errorf("演示文稿没有可编辑的主题部件")
	}
	edits := make(map[string][]byte, len(themeParts))
	changed := false
	for _, name := range themeParts {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		part := pkg.parts[name]
		if part == nil {
			return false, fmt.Errorf("主题部件缺失: %s", name)
		}
		data, err := readOPCPart(part)
		if err != nil {
			return false, fmt.Errorf("读取主题部件 %s 失败: %w", name, err)
		}
		updated, err := patchPresentationTheme(data, palette, majorFont, minorFont, themeName)
		if err != nil {
			return false, fmt.Errorf("修改主题部件 %s 失败: %w", name, err)
		}
		if !bytes.Equal(data, updated) {
			changed = true
			edits[name] = updated
		}
	}
	if !changed {
		return false, nil
	}
	updatedPackage, err := replacePPTXPackageParts(ctx, base, edits)
	if err != nil {
		return false, err
	}
	if err := d.adoptPPTXSnapshotLocked(updatedPackage); err != nil {
		return false, err
	}
	return true, nil
}

func presentationThemePartNames(pkg *pptxPackage) []string {
	seenMasters := make(map[string]bool)
	seenThemes := make(map[string]bool)
	var themes []string
	for _, master := range pkg.layoutMasterParts {
		if master == "" || seenMasters[master] {
			continue
		}
		seenMasters[master] = true
		if theme, ok := pkg.relatedPart(master, pptxThemeRelationshipType); ok && !seenThemes[theme] {
			seenThemes[theme] = true
			themes = append(themes, theme)
		}
	}
	return themes
}

func patchPresentationTheme(data []byte, palette presentationThemePalette, majorFont, minorFont, themeName string) ([]byte, error) {
	nodes, err := scanStyleXML(data)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 || nodes[0].local != "theme" {
		return nil, fmt.Errorf("主题 XML 根节点不是 a:theme")
	}
	var root pptxXMLNode
	if err := xml.Unmarshal(data, &root); err != nil || root.XMLName.Space != presentationThemeNamespace {
		return nil, fmt.Errorf("主题 XML 命名空间无效")
	}
	themeElements := directStyleNode(nodes, 0, "themeElements")
	if themeElements < 0 {
		return nil, fmt.Errorf("主题缺少 a:themeElements")
	}
	colorScheme := directStyleNode(nodes, themeElements, "clrScheme")
	if colorScheme < 0 {
		return nil, fmt.Errorf("主题缺少 a:clrScheme")
	}
	patched, err := patchThemeColorScheme(data, nodes, colorScheme, palette, themeName)
	if err != nil {
		return nil, err
	}
	if majorFont == "" && minorFont == "" {
		return patched, nil
	}
	fontNodes, err := scanStyleXML(patched)
	if err != nil {
		return nil, err
	}
	fontScheme := directStyleNode(fontNodes, themeElements, "fontScheme")
	if fontScheme < 0 {
		return nil, fmt.Errorf("主题缺少 a:fontScheme")
	}
	return patchThemeFontScheme(patched, fontNodes, fontScheme, majorFont, minorFont, themeName)
}

func directStyleNode(nodes []styleXMLNode, parent int, local string) int {
	for index := range nodes {
		if nodes[index].parent == parent && nodes[index].local == local {
			return index
		}
	}
	return -1
}

func patchThemeColorScheme(data []byte, nodes []styleXMLNode, schemeIndex int, palette presentationThemePalette, name string) ([]byte, error) {
	counts := make(map[string]int, len(presentationThemeColorOrder))
	replacements := make([]shapeXMLReplacement, 0, len(presentationThemeColorOrder)+1)
	for _, childIndex := range styleNodeChildren(nodes, schemeIndex) {
		child := nodes[childIndex]
		if child.local == "extLst" {
			continue
		}
		if _, ok := palette.Colors[child.local]; !ok {
			return nil, fmt.Errorf("配色方案包含未知结构 %s", child.local)
		}
		counts[child.local]++
		prefix := styleXMLPrefix(child.qName)
		color := strings.ToUpper(palette.Colors[child.local])
		if len(color) != 6 {
			return nil, fmt.Errorf("配色 %s 不是六位 RGB", child.local)
		}
		replacement := fmt.Sprintf("<%s%s><%ssrgbClr val=\"%s\"/></%s%s>", prefix, child.local, prefix, color, prefix, child.local)
		replacements = append(replacements, shapeXMLReplacement{start: child.start, end: child.end, data: []byte(replacement)})
	}
	for _, local := range presentationThemeColorOrder {
		if counts[local] != 1 {
			return nil, fmt.Errorf("主题配色项 %s 缺失或重复", local)
		}
	}
	scheme := nodes[schemeIndex]
	startTag, err := setXMLStartTagAttributes(data[scheme.start:scheme.startTagEnd], map[string]string{"name": name})
	if err != nil {
		return nil, err
	}
	replacements = append(replacements, shapeXMLReplacement{start: scheme.start, end: scheme.startTagEnd, data: startTag})
	return applyXMLReplacements(data, replacements), nil
}

func patchThemeFontScheme(data []byte, nodes []styleXMLNode, schemeIndex int, major, minor, name string) ([]byte, error) {
	scheme := nodes[schemeIndex]
	startTag, err := setXMLStartTagAttributes(data[scheme.start:scheme.startTagEnd], map[string]string{"name": name})
	if err != nil {
		return nil, err
	}
	replacements := []shapeXMLReplacement{{start: scheme.start, end: scheme.startTagEnd, data: startTag}}
	for _, family := range []struct{ local, value string }{{"majorFont", major}, {"minorFont", minor}} {
		if family.value == "" {
			continue
		}
		familyIndex := directStyleNode(nodes, schemeIndex, family.local)
		if familyIndex < 0 {
			return nil, fmt.Errorf("主题字体方案缺少 a:%s", family.local)
		}
		counts := map[string]int{"latin": 0, "ea": 0, "cs": 0}
		for _, childIndex := range styleNodeChildren(nodes, familyIndex) {
			child := nodes[childIndex]
			if child.local == "font" {
				fontTag, err := setXMLStartTagAttributes(data[child.start:child.startTagEnd], map[string]string{"typeface": family.value})
				if err != nil {
					return nil, err
				}
				replacements = append(replacements, shapeXMLReplacement{start: child.start, end: child.startTagEnd, data: fontTag})
				continue
			}
			if child.local == "extLst" {
				continue
			}
			if _, ok := counts[child.local]; !ok {
				return nil, fmt.Errorf("主题字体方案包含未知结构 %s", child.local)
			}
			counts[child.local]++
			fontTag, err := setXMLStartTagAttributes(data[child.start:child.startTagEnd], map[string]string{"typeface": family.value})
			if err != nil {
				return nil, err
			}
			replacements = append(replacements, shapeXMLReplacement{start: child.start, end: child.startTagEnd, data: fontTag})
		}
		for local, count := range counts {
			if count != 1 {
				return nil, fmt.Errorf("主题字体项 %s 缺失或重复", local)
			}
		}
	}
	return applyXMLReplacements(data, replacements), nil
}
