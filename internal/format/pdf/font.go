package pdf

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/SalvioniDigitalSolutions/gopdf"
	"github.com/go-text/typesetting/font"
)

// FontSource returns bytes for the exact font face selected by document layout.
type FontSource interface {
	FontData(fontKey string) ([]byte, error)
}

// extractSFNT returns a standalone SFNT font (TTF or OTF) for the requested face
// index from font data, which may be either a standalone font or a TTC collection.
func extractSFNT(data []byte, faceIndex int) ([]byte, error) {
	if len(data) < 12 {
		return nil, errors.New("font data is too short")
	}
	if string(data[:4]) != "ttcf" {
		if faceIndex != 0 {
			return nil, fmt.Errorf("non-collection font requested face %d", faceIndex)
		}
		return data, nil
	}
	offset, err := sfntFaceOffset(data, faceIndex)
	if err != nil {
		return nil, err
	}
	if offset+12 > len(data) {
		return nil, fmt.Errorf("face offset %d out of bounds", offset)
	}
	numTables := int(binary.BigEndian.Uint16(data[offset+4 : offset+6]))
	dirEnd := offset + 12 + numTables*16
	if dirEnd > len(data) {
		return nil, errors.New("table directory out of bounds")
	}

	headerLen := 12 + numTables*16
	var tableData bytes.Buffer
	newDir := make([]byte, numTables*16)

	for i := 0; i < numTables; i++ {
		rec := offset + 12 + i*16
		tag := data[rec : rec+4]
		chk := binary.BigEndian.Uint32(data[rec+4 : rec+8])
		tabOff := int(binary.BigEndian.Uint32(data[rec+8 : rec+12]))
		tabLen := int(binary.BigEndian.Uint32(data[rec+12 : rec+16]))

		if tabOff < 0 || tabLen < 0 || tabOff+tabLen > len(data) {
			return nil, fmt.Errorf("table %q out of bounds", string(tag))
		}

		currentOffset := headerLen + tableData.Len()
		copy(newDir[i*16:i*16+4], tag)
		binary.BigEndian.PutUint32(newDir[i*16+4:i*16+8], chk)
		binary.BigEndian.PutUint32(newDir[i*16+8:i*16+12], uint32(currentOffset))
		binary.BigEndian.PutUint32(newDir[i*16+12:i*16+16], uint32(tabLen))

		tableData.Write(data[tabOff : tabOff+tabLen])
		if pad := (4 - (tabLen % 4)) % 4; pad > 0 {
			tableData.Write(make([]byte, pad))
		}
	}

	var standalone bytes.Buffer
	standalone.Write(data[offset : offset+12])
	standalone.Write(newDir)
	standalone.Write(tableData.Bytes())
	return standalone.Bytes(), nil
}

func validateFontCoverage(standaloneData []byte, fontKey, text string) error {
	faces, err := font.ParseTTC(bytes.NewReader(standaloneData))
	if err != nil {
		return fmt.Errorf("parse font %q for Unicode mapping: %w", fontKey, err)
	}
	if len(faces) == 0 {
		return fmt.Errorf("font %q has no faces", fontKey)
	}
	face := faces[0]
	for _, value := range text {
		if _, ok := face.NominalGlyph(value); !ok {
			return fmt.Errorf("font %q does not map U+%04X for PDF text extraction", fontKey, value)
		}
	}
	return nil
}

func fontFaceIndex(key string) (int, error) {
	marker := "#face="
	index := strings.Index(key, marker)
	if index < 0 {
		return 0, nil
	}
	value := key[index+len(marker):]
	if end := strings.IndexByte(value, '#'); end >= 0 {
		value = value[:end]
	}
	face, err := strconv.Atoi(value)
	if err != nil || face < 0 {
		return 0, fmt.Errorf("font key %q has an invalid collection face index", key)
	}
	return face, nil
}

func validateFontEmbedding(standaloneData []byte, key string) error {
	if len(standaloneData) < 12 {
		return fmt.Errorf("font %q is too small to inspect embedding permissions", key)
	}
	base, err := sfntFaceOffset(standaloneData, 0)
	if err != nil {
		return fmt.Errorf("inspect font %q embedding permissions: %w", key, err)
	}
	count := int(binary.BigEndian.Uint16(standaloneData[base+4 : base+6]))
	if base+12+count*16 > len(standaloneData) {
		return fmt.Errorf("font %q has a truncated table directory", key)
	}
	var fsType uint16
	var hasOS2, hasCFF bool
	for index := 0; index < count; index++ {
		record := base + 12 + index*16
		tag := string(standaloneData[record : record+4])
		offset := int(binary.BigEndian.Uint32(standaloneData[record+8 : record+12]))
		length := int(binary.BigEndian.Uint32(standaloneData[record+12 : record+16]))
		if offset < 0 || length < 0 || offset > len(standaloneData) || length > len(standaloneData)-offset {
			return fmt.Errorf("font %q has an invalid %q table", key, tag)
		}
		switch tag {
		case "OS/2":
			if length < 10 {
				return fmt.Errorf("font %q has a truncated OS/2 table", key)
			}
			fsType = binary.BigEndian.Uint16(standaloneData[offset+8 : offset+10])
			hasOS2 = true
		case "CFF ", "CFF2":
			hasCFF = true
		}
	}
	if !hasOS2 {
		return fmt.Errorf("font %q has no OS/2 embedding permissions; PDF export is blocked", key)
	}
	if fsType&0x0002 != 0 {
		return fmt.Errorf("font %q forbids embedding; PDF export is blocked", key)
	}
	if fsType&0x0200 != 0 {
		return fmt.Errorf("font %q permits bitmap-only embedding; vector PDF text is blocked", key)
	}
	if fsType&0x0100 != 0 && !hasCFF {
		return fmt.Errorf("font %q forbids subsetting, which the PDF backend requires for TrueType fonts", key)
	}
	return nil
}

func sfntFaceOffset(data []byte, faceIndex int) (int, error) {
	if len(data) >= 12 && string(data[:4]) == "ttcf" {
		count := int(binary.BigEndian.Uint32(data[8:12]))
		if faceIndex < 0 || faceIndex >= count || 12+count*4 > len(data) {
			return 0, errors.New("invalid TrueType collection directory")
		}
		offset := int(binary.BigEndian.Uint32(data[12+faceIndex*4 : 16+faceIndex*4]))
		if offset < 0 || offset+12 > len(data) {
			return 0, errors.New("invalid TrueType collection face offset")
		}
		return offset, nil
	}
	if faceIndex != 0 || len(data) < 12 {
		return 0, errors.New("invalid SFNT face index or header")
	}
	return 0, nil
}

func textColor(value string) gopdf.Color {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) == 6 {
		var red, green, blue uint8
		if _, err := fmt.Sscanf(value, "%02x%02x%02x", &red, &green, &blue); err == nil {
			return gopdf.RGB(red, green, blue)
		}
	}
	return gopdf.Black
}

func highlightColor(value string) (gopdf.Color, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "black":
		return gopdf.Black, true
	case "blue":
		return gopdf.RGB(0, 0, 255), true
	case "cyan":
		return gopdf.RGB(0, 255, 255), true
	case "darkblue":
		return gopdf.RGB(0, 0, 128), true
	case "darkcyan":
		return gopdf.RGB(0, 128, 128), true
	case "darkgray":
		return gopdf.RGB(128, 128, 128), true
	case "darkgreen":
		return gopdf.RGB(0, 128, 0), true
	case "darkmagenta":
		return gopdf.RGB(128, 0, 128), true
	case "darkred":
		return gopdf.RGB(128, 0, 0), true
	case "darkyellow":
		return gopdf.RGB(128, 128, 0), true
	case "green":
		return gopdf.RGB(0, 255, 0), true
	case "lightgray":
		return gopdf.RGB(192, 192, 192), true
	case "magenta":
		return gopdf.RGB(255, 0, 255), true
	case "red":
		return gopdf.RGB(255, 0, 0), true
	case "white":
		return gopdf.White, true
	case "yellow":
		return gopdf.RGB(255, 255, 0), true
	default:
		return gopdf.Color{}, false
	}
}

func effectiveFontSize(size float64) float64 {
	if size <= 0 || !finite(size) {
		return 11
	}
	return size
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

type contextWriter struct {
	ctx context.Context
	dst io.Writer
}

func (writer contextWriter) Write(data []byte) (int, error) {
	if err := writer.ctx.Err(); err != nil {
		return 0, err
	}
	return writer.dst.Write(data)
}
