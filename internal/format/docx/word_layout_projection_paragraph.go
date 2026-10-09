package docx

import (
	"strconv"
	"strings"

	"glance/internal/document"
)

func parseWordTextStyleSet(properties *wordXMLNode) document.WordTextStyleMask {
	if properties == nil {
		return 0
	}
	var mask document.WordTextStyleMask
	if firstChild(properties, "b") != nil {
		mask |= document.WordTextBoldSet
	}
	if firstChild(properties, "i") != nil {
		mask |= document.WordTextItalicSet
	}
	if firstChild(properties, "u") != nil {
		mask |= document.WordTextUnderlineSet
	}
	if fonts := firstChild(properties, "rFonts"); fonts != nil {
		for _, name := range []string{"eastAsia", "ascii", "hAnsi", "cs"} {
			if attrValue(fonts, name) != "" {
				mask |= document.WordTextFontFamilySet
				break
			}
		}
	}
	if firstChild(properties, "sz") != nil {
		mask |= document.WordTextFontSizeSet
	}
	if firstChild(properties, "color") != nil {
		mask |= document.WordTextFontColorSet
	}
	if firstChild(properties, "highlight") != nil {
		mask |= document.WordTextHighlightSet
	}
	return mask
}

func parseWordParagraphLayout(properties *wordXMLNode) document.WordParagraphLayout {
	var layout document.WordParagraphLayout
	if properties == nil {
		return layout
	}
	if spacing := firstChild(properties, "spacing"); spacing != nil {
		if value := attrValue(spacing, "before"); value != "" {
			layout.BeforePT = wordTwipsToPoints(value)
			layout.Set |= document.WordParagraphBeforeSet
		}
		if value := attrValue(spacing, "after"); value != "" {
			layout.AfterPT = wordTwipsToPoints(value)
			layout.Set |= document.WordParagraphAfterSet
		}
		if value := attrValue(spacing, "line"); value != "" {
			layout.LineSpacingRule = strings.ToLower(attrValue(spacing, "lineRule"))
			if layout.LineSpacingRule == "" || layout.LineSpacingRule == "auto" {
				layout.LineSpacingRule = "auto"
				if amount, err := strconv.ParseFloat(value, 64); err == nil && amount >= 0 {
					layout.LineSpacingMultiple = amount / 240
					layout.Set |= document.WordParagraphLineSpacingSet
				}
			} else if layout.LineSpacingRule == "exact" || layout.LineSpacingRule == "atleast" {
				layout.LineSpacingPT = wordTwipsToPoints(value)
				layout.Set |= document.WordParagraphLineSpacingSet
			}
		}
	}
	if indentation := firstChild(properties, "ind"); indentation != nil {
		if value, present := attrValuePresent(indentation, "firstLineChars"); present {
			if amount, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && amount == 0 {
				layout.FirstLineIndentEMU = 0
				layout.Set |= document.WordParagraphFirstLineIndentSet
			}
		} else if value, present := attrValuePresent(indentation, "hangingChars"); present {
			if amount, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && amount == 0 {
				layout.HangingIndentEMU = 0
				layout.Set |= document.WordParagraphHangingIndentSet
			}
		} else if value := attrValue(indentation, "firstLine"); value != "" {
			layout.FirstLineIndentEMU = parseWordIndentEMU(value)
			layout.Set |= document.WordParagraphFirstLineIndentSet
		} else if value := attrValue(indentation, "hanging"); value != "" {
			layout.HangingIndentEMU = parseWordIndentEMU(value)
			layout.Set |= document.WordParagraphHangingIndentSet
		}
		left := attrValue(indentation, "left")
		if left == "" {
			left = attrValue(indentation, "start")
		}
		if left != "" {
			layout.LeftIndentEMU = parseWordIndentEMU(left)
			layout.Set |= document.WordParagraphLeftIndentSet
		}
		right := attrValue(indentation, "right")
		if right == "" {
			right = attrValue(indentation, "end")
		}
		if right != "" {
			layout.RightIndentEMU = parseWordIndentEMU(right)
			layout.Set |= document.WordParagraphRightIndentSet
		}
	}
	if alignment := firstChild(properties, "jc"); alignment != nil {
		layout.Alignment = parseWordAlignment(attrValue(alignment, "val"))
		layout.Set |= document.WordParagraphAlignmentSet
	}
	if node := firstChild(properties, "keepNext"); node != nil {
		layout.KeepWithNext = toggleProperty(node)
		layout.Set |= document.WordParagraphKeepWithNextSet
	}
	if node := firstChild(properties, "keepLines"); node != nil {
		layout.KeepTogether = toggleProperty(node)
		layout.Set |= document.WordParagraphKeepTogetherSet
	}
	if node := firstChild(properties, "pageBreakBefore"); node != nil {
		layout.PageBreakBefore = toggleProperty(node)
		layout.Set |= document.WordParagraphPageBreakBeforeSet
	}
	if borders := firstChild(properties, "pBdr"); borders != nil {
		if bottom := firstChild(borders, "bottom"); bottom != nil {
			layout.BottomBorder = parseWordParagraphBorder(bottom)
			layout.Set |= document.WordParagraphBottomBorderSet
		}
	}
	if tabs := firstChild(properties, "tabs"); tabs != nil {
		layout.Set |= document.WordParagraphTabStopsSet
		for _, tab := range tabs.children {
			if tab.name.Local != "tab" {
				continue
			}
			position, err := strconv.ParseFloat(attrValue(tab, "pos"), 64)
			if err != nil || position < 0 {
				continue
			}
			alignment := strings.ToLower(attrValue(tab, "val"))
			if alignment == "" {
				alignment = "left"
			}
			layout.TabStops = append(layout.TabStops, document.WordTabStop{
				PositionPT: position / 20,
				Alignment:  alignment,
				Leader:     strings.ToLower(attrValue(tab, "leader")),
			})
		}
	}
	return layout
}

func parseWordParagraphBorder(node *wordXMLNode) document.WordParagraphBorder {
	border := document.WordParagraphBorder{Style: strings.ToLower(attrValue(node, "val")), SizePT: 0.5}
	if border.Style == "" {
		border.Style = "single"
	}
	if value := attrValue(node, "sz"); value != "" {
		if eighths, err := strconv.ParseFloat(value, 64); err == nil && eighths >= 0 {
			border.SizePT = eighths / 8
		}
	}
	if value := attrValue(node, "space"); value != "" {
		if space, err := strconv.ParseFloat(value, 64); err == nil && space >= 0 {
			border.SpacePT = space
		}
	}
	border.Color = strings.ToUpper(attrValue(node, "color"))
	return border
}
