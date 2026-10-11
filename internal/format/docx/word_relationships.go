package docx

import (
	"glance/internal/document"
	"path"
	"strconv"
	"strings"
)

type wordRelationship struct {
	target    string
	image     bool
	numbering bool
	styles    bool
	settings  bool
	header    bool
	comments  bool
	footnotes bool
	endnotes  bool
	external  bool
}

type wordNumberingLevel struct {
	kind   document.WordListKind
	format string
	text   string
	start  int
}

type wordNumberingDefinition struct {
	levels map[int]wordNumberingLevel
}

func readDocumentRelationships(parts []part, partIndexes map[string]int) map[string]wordRelationship {
	result := make(map[string]wordRelationship)
	index, ok := partIndexes["word/_rels/document.xml.rels"]
	if !ok {
		return result
	}
	root, err := parseWordXML(parts[index].data)
	if err != nil {
		return result
	}
	for _, node := range root.children {
		if node.name.Local != "Relationship" {
			continue
		}
		id := attrValue(node, "Id")
		target := attrValue(node, "Target")
		relationType := strings.ToLower(attrValue(node, "Type"))
		if id == "" {
			continue
		}
		external := strings.EqualFold(attrValue(node, "TargetMode"), "External")
		result[id] = wordRelationship{
			target:    resolveDocumentTarget(target),
			image:     strings.HasSuffix(relationType, "/image"),
			numbering: strings.HasSuffix(relationType, "/numbering"),
			styles:    strings.HasSuffix(relationType, "/styles"),
			settings:  strings.HasSuffix(relationType, "/settings"),
			header:    strings.HasSuffix(relationType, "/header"),
			comments:  strings.HasSuffix(relationType, "/comments"),
			footnotes: strings.HasSuffix(relationType, "/footnotes"),
			endnotes:  strings.HasSuffix(relationType, "/endnotes"),
			external:  external,
		}
	}
	return result
}

func readWordNumbering(parts []part, partIndexes map[string]int, relationships map[string]wordRelationship) map[string]wordNumberingDefinition {
	result := make(map[string]wordNumberingDefinition)
	partName := ""
	for _, relationship := range relationships {
		if relationship.numbering && !relationship.external {
			partName = relationship.target
			break
		}
	}
	if partName == "" {
		return result
	}
	index, ok := partIndexes[partName]
	if !ok {
		return result
	}
	root, err := parseWordXML(parts[index].data)
	if err != nil {
		return result
	}
	abstracts := make(map[string]map[int]wordNumberingLevel)
	for _, node := range root.children {
		if node.name.Local != "abstractNum" {
			continue
		}
		abstractID := attrValue(node, "abstractNumId")
		levels := make(map[int]wordNumberingLevel)
		for _, levelNode := range node.children {
			if levelNode.name.Local != "lvl" {
				continue
			}
			level, parseErr := strconv.Atoi(attrValue(levelNode, "ilvl"))
			if parseErr != nil || level < 0 || level > 8 {
				continue
			}
			numFmt := strings.ToLower(attrValue(firstChild(levelNode, "numFmt"), "val"))
			lvlText := attrValue(firstChild(levelNode, "lvlText"), "val")
			start := 1
			if value := attrValue(firstChild(levelNode, "start"), "val"); value != "" {
				if parsed, parseErr := strconv.Atoi(value); parseErr == nil && parsed > 0 {
					start = parsed
				}
			}
			kind := document.WordListUnsupported
			switch numFmt {
			case "bullet":
				kind = document.WordListBullet
			case "decimal", "decimalzero", "lowerletter", "upperletter", "lowerroman", "upperroman":
				kind = document.WordListNumbered
			}
			for _, property := range levelNode.children {
				if property.name.Local == "lvlRestart" || property.name.Local == "lvlPicBulletId" {
					kind = document.WordListUnsupported
					break
				}
			}
			levels[level] = wordNumberingLevel{kind: kind, format: numFmt, text: lvlText, start: start}
		}
		abstracts[abstractID] = levels
	}
	for _, node := range root.children {
		if node.name.Local != "num" {
			continue
		}
		numID := attrValue(node, "numId")
		abstractID := attrValue(firstChild(node, "abstractNumId"), "val")
		if numID == "" || abstractID == "" {
			continue
		}
		levels, ok := abstracts[abstractID]
		if !ok {
			continue
		}
		hasOverrides := false
		for _, child := range node.children {
			if child.name.Local == "lvlOverride" || child.name.Local == "startOverride" {
				hasOverrides = true
				break
			}
		}
		if hasOverrides {
			levels = make(map[int]wordNumberingLevel, len(levels))
			for level, definition := range abstracts[abstractID] {
				definition.kind = document.WordListUnsupported
				levels[level] = definition
			}
		}
		result[numID] = wordNumberingDefinition{levels: levels}
	}
	return result
}

func resolveDocumentTarget(target string) string {
	if target == "" {
		return ""
	}
	clean := strings.TrimPrefix(target, "/")
	if !strings.HasPrefix(target, "/") {
		clean = path.Join("word", target)
	}
	clean = path.Clean(clean)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}

type wordContentTypes struct {
	defaults  map[string]string
	overrides map[string]string
}

func readContentTypes(parts []part, partIndexes map[string]int) wordContentTypes {
	result := wordContentTypes{defaults: make(map[string]string), overrides: make(map[string]string)}
	index, ok := partIndexes["[Content_Types].xml"]
	if !ok {
		return result
	}
	root, err := parseWordXML(parts[index].data)
	if err != nil {
		return result
	}
	for _, node := range root.children {
		switch node.name.Local {
		case "Default":
			extension := strings.ToLower(strings.TrimPrefix(attrValue(node, "Extension"), "."))
			if extension != "" {
				result.defaults[extension] = attrValue(node, "ContentType")
			}
		case "Override":
			partName := strings.TrimPrefix(attrValue(node, "PartName"), "/")
			if partName != "" {
				result.overrides[path.Clean(partName)] = attrValue(node, "ContentType")
			}
		}
	}
	return result
}

func (types wordContentTypes) mediaType(partName string) string {
	if mediaType := types.overrides[path.Clean(partName)]; mediaType != "" {
		return mediaType
	}
	return types.defaults[strings.TrimPrefix(strings.ToLower(path.Ext(partName)), ".")]
}
