package xls

import (
	"fmt"

	legacy "github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
)

func (r *importReader) ExcelizeStyle(xfIndex uint16) (excelize.Style, error) {
	raw, err := r.book.XF(xfIndex)
	if err != nil {
		return excelize.Style{}, err
	}
	switch xf := raw.(type) {
	case *legacy.Xf8:
		return r.excelizeStyleFromXF8(xf)
	case *legacy.Xf5:
		return excelize.Style{}, fmt.Errorf("XF %d uses BIFF5 styles, which are not supported by the XLSX importer", xfIndex)
	default:
		return excelize.Style{}, fmt.Errorf("XF %d has unsupported BIFF style type %T", xfIndex, raw)
	}
}

func (r *importReader) excelizeStyleFromXF8(xf *legacy.Xf8) (excelize.Style, error) {
	style := excelize.Style{}
	fontIndex := int(xf.Font)
	// BIFF omits font record index 4; records after that reserved slot are
	// shifted by one in the parser's compact Fonts slice.
	if fontIndex >= 4 {
		fontIndex--
	}
	if fontIndex < 0 || fontIndex >= len(r.book.Fonts) {
		return excelize.Style{}, fmt.Errorf("BIFF XF font index %d is outside the font table", xf.Font)
	}
	font := r.book.Fonts[fontIndex]
	if font.Info == nil {
		return excelize.Style{}, fmt.Errorf("BIFF font index %d has no font record", xf.Font)
	}
	fontColor, err := r.colorHex(font.Info.Color)
	if err != nil {
		return excelize.Style{}, fmt.Errorf("BIFF font color: %w", err)
	}
	style.Font = &excelize.Font{
		Bold:      font.Info.Bold >= 700,
		Italic:    font.Info.Flag&0x0002 != 0,
		Strike:    font.Info.Flag&0x0008 != 0,
		Underline: biffUnderline(font.Info.Underline),
		Family:    font.Name,
		Size:      float64(font.Info.Height) / 20,
		Color:     fontColor,
	}

	formatCode := r.book.FormatString(xf.Format)
	if formatCode != "" {
		style.CustomNumFmt = &formatCode
	} else if xf.Format < 164 {
		style.NumFmt = int(xf.Format)
	} else {
		return excelize.Style{}, fmt.Errorf("BIFF custom number format %d is missing", xf.Format)
	}

	horizontal, err := biffHorizontalAlignment(xf.Align & 0x07)
	if err != nil {
		return excelize.Style{}, err
	}
	vertical, err := biffVerticalAlignment((xf.Align >> 4) & 0x07)
	if err != nil {
		return excelize.Style{}, err
	}
	style.Alignment = &excelize.Alignment{
		Horizontal:      horizontal,
		Vertical:        vertical,
		WrapText:        xf.Align&0x08 != 0,
		Indent:          int(xf.Ident & 0x0F),
		ShrinkToFit:     xf.Ident&0x10 != 0,
		ReadingOrder:    uint64((xf.Ident >> 5) & 0x03),
		JustifyLastLine: xf.Align&0x80 != 0,
	}

	// BIFF8 stores fill colors in Groundcolor, but stores the 6-bit fill
	// pattern in the high bits of Linecolor after the border color fields.
	// Reading the high bits of Groundcolor as a pattern turns ordinary
	// automatic foreground/background colors into a black patterned fill.
	pattern := int((xf.Linecolor >> 26) & 0x3F)
	if pattern > 18 {
		return excelize.Style{}, fmt.Errorf("BIFF fill pattern %d cannot be represented", pattern)
	}
	if pattern != 0 {
		foreground, err := r.colorHex(xf.Groundcolor & 0x7F)
		if err != nil {
			return excelize.Style{}, fmt.Errorf("BIFF fill foreground: %w", err)
		}
		style.Fill = excelize.Fill{Type: "pattern", Pattern: pattern, Color: []string{foreground}}
	}

	style.Border, err = r.bordersFromXF8(xf)
	if err != nil {
		return excelize.Style{}, err
	}
	return style, nil
}

func (r *importReader) bordersFromXF8(xf *legacy.Xf8) ([]excelize.Border, error) {
	var borders []excelize.Border
	sides := []struct {
		name       string
		styleShift uint
		colorShift uint
	}{
		{"left", 0, 0}, {"right", 4, 7}, {"top", 8, 14}, {"bottom", 12, 21},
	}
	for _, side := range sides {
		borderStyle := int((xf.Linestyle >> side.styleShift) & 0x0F)
		if borderStyle == 0 {
			continue
		}
		if borderStyle > 13 {
			return nil, fmt.Errorf("BIFF %s border style %d cannot be represented", side.name, borderStyle)
		}
		colorIndex := uint16((xf.Linecolor >> side.colorShift) & 0x7F)
		color, err := r.colorHex(colorIndex)
		if err != nil {
			return nil, fmt.Errorf("BIFF %s border color: %w", side.name, err)
		}
		borders = append(borders, excelize.Border{Type: side.name, Style: borderStyle, Color: color})
	}
	diagonalStyle := int((xf.Linestyle >> 16) & 0x0F)
	if diagonalStyle != 0 {
		colorIndex := uint16((xf.Linecolor >> 28) & 0x0F)
		color, err := r.colorHex(colorIndex)
		if err != nil {
			return nil, fmt.Errorf("BIFF diagonal border color: %w", err)
		}
		if xf.Linestyle&(1<<20) != 0 {
			borders = append(borders, excelize.Border{Type: "diagonalDown", Style: diagonalStyle, Color: color})
		}
		if xf.Linestyle&(1<<21) != 0 {
			borders = append(borders, excelize.Border{Type: "diagonalUp", Style: diagonalStyle, Color: color})
		}
		if xf.Linestyle&(3<<20) == 0 {
			return nil, fmt.Errorf("BIFF diagonal border has no direction flag")
		}
	}
	return borders, nil
}

func (r *importReader) colorHex(index uint16) (string, error) {
	color, err := r.book.ColorRGB(index)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%02X%02X%02X", color.Red, color.Green, color.Blue), nil
}

func biffUnderline(value byte) string {
	switch value {
	case 1:
		return "single"
	case 2:
		return "double"
	case 0x21:
		return "singleAccounting"
	case 0x22:
		return "doubleAccounting"
	default:
		return ""
	}
}

func biffHorizontalAlignment(value byte) (string, error) {
	alignments := [...]string{"general", "left", "center", "right", "fill", "justify", "centerContinuous", "distributed"}
	if int(value) >= len(alignments) {
		return "", fmt.Errorf("BIFF horizontal alignment %d is invalid", value)
	}
	return alignments[value], nil
}

func biffVerticalAlignment(value byte) (string, error) {
	alignments := [...]string{"top", "center", "bottom", "justify", "distributed"}
	if int(value) >= len(alignments) {
		return "", fmt.Errorf("BIFF vertical alignment %d is unsupported", value)
	}
	return alignments[value], nil
}
