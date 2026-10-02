// Package pdf contains format-specific writers for Word page scenes.
package pdf

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/SalvioniDigitalSolutions/gopdf"
	"github.com/go-text/typesetting/font"

	"glance/internal/fileutil"
	"glance/internal/wordlayout"
)

// FontSource returns bytes for the exact font face selected by Word layout.
type FontSource interface {
	FontData(fontKey string) ([]byte, error)
}

// ImageSource returns the original image data referenced by a page scene.
type ImageSource interface {
	ImageBytes(imageID string) ([]byte, error)
}

// WriteWordPDF replays the page scene without measuring, wrapping, or
// repositioning content. Visible text is emitted as vector outlines; a
// transparent Unicode layer supplies search and copy text.
func WriteWordPDF(ctx context.Context, dst io.Writer, result wordlayout.Result, fonts FontSource, images ImageSource) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if dst == nil {
		return errors.New("PDF destination is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !result.CanPreview {
		return blockingLayoutError(result)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == wordlayout.DiagnosticError {
			return fmt.Errorf("Word layout blocks PDF export: %s", diagnostic.Detail)
		}
	}
	if len(result.Pages) == 0 {
		return errors.New("Word layout has no pages")
	}
	if fonts == nil && wordResultHasText(result) {
		return errors.New("PDF text extraction requires a font source")
	}
	if images == nil && wordResultHasImages(result) {
		return errors.New("PDF image placement requires an image source")
	}

	documentPDF := gopdf.New()
	documentPDF.CompressObjects = true
	fontCache := make(map[string]*gopdf.Font)
	fontBytes := make(map[string][]byte)
	imageCache := make(map[string]*gopdf.Image)
	for pageIndex, scene := range result.Pages {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validatePage(scene); err != nil {
			return fmt.Errorf("Word page %d: %w", pageIndex+1, err)
		}
		page := documentPDF.AddPageSize(gopdf.PageSize{W: scene.Clip.WidthPT, H: scene.Clip.HeightPT})
		page.ClipRect(0, 0, scene.Clip.WidthPT, scene.Clip.HeightPT)

		for _, cell := range scene.TableCells {
			if err := validateRect(cell.Bounds); err != nil {
				return fmt.Errorf("Word page %d table cell %q: %w", pageIndex+1, cell.CellID, err)
			}
			if cell.Bounds.WidthPT <= 0 || cell.Bounds.HeightPT <= 0 {
				return fmt.Errorf("Word page %d table cell %q has empty bounds", pageIndex+1, cell.CellID)
			}
			drawCellBorder(page, cell.Bounds)
		}
		for _, fragment := range scene.Text {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := validateFragment(fragment); err != nil {
				return fmt.Errorf("Word page %d text run %q: %w", pageIndex+1, fragment.RunID, err)
			}
			drawTextFragment(page, fragment)
			chunks, err := fragmentTextChunks(fragment)
			if err != nil {
				return fmt.Errorf("Word page %d text run %q: %w", pageIndex+1, fragment.RunID, err)
			}
			for _, chunk := range chunks {
				pdfFont := fontCache[chunk.fontKey]
				if pdfFont == nil {
					data, err := fonts.FontData(chunk.fontKey)
					if err != nil {
						return fmt.Errorf("load font %q for PDF: %w", chunk.fontKey, err)
					}
					if err := validateFontEmbedding(data, chunk.fontKey); err != nil {
						return err
					}
					if err := validateFontCoverage(data, chunk.fontKey, chunk.text); err != nil {
						return err
					}
					pdfFont, err = gopdf.ParseFont(data)
					if err != nil {
						return fmt.Errorf("parse font %q for PDF: %w", chunk.fontKey, err)
					}
					fontCache[chunk.fontKey] = pdfFont
					fontBytes[chunk.fontKey] = data
				} else if err := validateFontCoverage(fontBytes[chunk.fontKey], chunk.fontKey, chunk.text); err != nil {
					return err
				}
				page.Push()
				page.SetAlpha(0, 0)
				page.SetFillColor(gopdf.Black)
				page.SetFont(pdfFont, effectiveFontSize(fragment.Style.FontSize))
				page.Text(fragment.XPT, fragment.BaselineYPT, chunk.text)
				page.Pop()
			}
		}
		for _, placement := range scene.Images {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := validateRect(placement.Bounds); err != nil {
				return fmt.Errorf("Word page %d image %q: %w", pageIndex+1, placement.ImageID, err)
			}
			if placement.Bounds.WidthPT <= 0 || placement.Bounds.HeightPT <= 0 {
				return fmt.Errorf("Word page %d image %q has empty bounds", pageIndex+1, placement.ImageID)
			}
			if !finite(placement.RotationDegrees) {
				return fmt.Errorf("Word page %d image %q has invalid rotation", pageIndex+1, placement.ImageID)
			}
			pdfImage := imageCache[placement.ImageID]
			if pdfImage == nil {
				data, err := images.ImageBytes(placement.ImageID)
				if err != nil {
					return fmt.Errorf("load Word image %q: %w", placement.ImageID, err)
				}
				pdfImage, err = documentPDF.AddImageReader(bytes.NewReader(data))
				if err != nil {
					return fmt.Errorf("add Word image %q to PDF: %w", placement.ImageID, err)
				}
				imageCache[placement.ImageID] = pdfImage
			}
			bounds := placement.Bounds
			page.Push()
			if placement.RotationDegrees != 0 {
				page.RotateAt(-placement.RotationDegrees, bounds.XPT+bounds.WidthPT/2, bounds.YPT+bounds.HeightPT/2)
			}
			page.DrawImage(pdfImage, bounds.XPT, bounds.YPT, bounds.WidthPT, bounds.HeightPT)
			page.Pop()
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := documentPDF.WriteTo(contextWriter{ctx: ctx, dst: dst}); err != nil {
		return fmt.Errorf("serialize Word PDF: %w", err)
	}
	return nil
}

// ExportWordPDFFile writes a complete PDF beside path, validates it, then
// atomically replaces path. Existing output survives cancellation and errors.
func ExportWordPDFFile(ctx context.Context, path string, result wordlayout.Result, fonts FontSource, images ImageSource) error {
	return ExportWordPDFFileWithCommitCheck(ctx, path, result, fonts, images, nil)
}

// ExportWordPDFFileWithCommitCheck runs check immediately before replacing the
// destination. It lets asynchronous callers reject a stale scene without
// touching an existing PDF.
func ExportWordPDFFileWithCommitCheck(ctx context.Context, path string, result wordlayout.Result, fonts FontSource, images ImageSource, check func() error) error {
	return ExportWordPDFFileWithCommitGuard(ctx, path, result, fonts, images, func() (func(), error) {
		if check != nil {
			if err := check(); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
}

// ExportWordPDFFileWithCommitGuard holds the returned release function around
// the final atomic replacement, allowing callers to serialize a revision check
// with edits that could make the page scene stale.
func ExportWordPDFFileWithCommitGuard(ctx context.Context, path string, result wordlayout.Result, fonts FontSource, images ImageSource, guard func() (release func(), err error)) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("PDF destination path is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".word-pdf-*.pdf")
	if err != nil {
		return fmt.Errorf("create temporary PDF beside destination: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := WriteWordPDF(ctx, temporary, result, fonts, images); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary PDF: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary PDF: %w", err)
	}
	reader, err := gopdf.Open(temporaryPath)
	if err != nil {
		return fmt.Errorf("validate generated PDF: %w", err)
	}
	if reader.Repaired() {
		return errors.New("validate generated PDF: reader had to repair the file")
	}
	if reader.NumPages() != len(result.Pages) {
		return fmt.Errorf("validate generated PDF: got %d pages, want %d", reader.NumPages(), len(result.Pages))
	}
	for index, scene := range result.Pages {
		size, err := reader.PageSize(index)
		if err != nil {
			return fmt.Errorf("validate generated PDF page %d: %w", index+1, err)
		}
		if math.Abs(size.W-scene.Clip.WidthPT) > 0.01 || math.Abs(size.H-scene.Clip.HeightPT) > 0.01 {
			return fmt.Errorf("validate generated PDF page %d size %.2fx%.2f, want %.2fx%.2f points", index+1, size.W, size.H, scene.Clip.WidthPT, scene.Clip.HeightPT)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var release func()
	if guard != nil {
		var err error
		release, err = guard()
		if err != nil {
			return err
		}
		if release != nil {
			defer release()
		}
	}
	if err := fileutil.ReplaceFile(temporaryPath, path); err != nil {
		return fmt.Errorf("replace PDF destination: %w", err)
	}
	return nil
}

type textChunk struct {
	fontKey string
	text    string
}

func blockingLayoutError(result wordlayout.Result) error {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == wordlayout.DiagnosticError {
			return fmt.Errorf("Word layout blocks PDF export: %s", diagnostic.Detail)
		}
	}
	return errors.New("Word layout is not safe to export")
}

func wordResultHasText(result wordlayout.Result) bool {
	for _, page := range result.Pages {
		for _, fragment := range page.Text {
			if fragment.Text != "" {
				return true
			}
		}
	}
	return false
}

func wordResultHasImages(result wordlayout.Result) bool {
	for _, page := range result.Pages {
		if len(page.Images) > 0 {
			return true
		}
	}
	return false
}

func validatePage(scene wordlayout.PageScene) error {
	if !finite(scene.Clip.WidthPT) || !finite(scene.Clip.HeightPT) || scene.Clip.WidthPT <= 0 || scene.Clip.HeightPT <= 0 {
		return errors.New("invalid page dimensions")
	}
	if !finite(scene.Clip.XPT) || !finite(scene.Clip.YPT) || scene.Clip.XPT != 0 || scene.Clip.YPT != 0 {
		return errors.New("page clip must start at the page origin")
	}
	return nil
}

func validateRect(rect wordlayout.Rect) error {
	if !finite(rect.XPT) || !finite(rect.YPT) || !finite(rect.WidthPT) || !finite(rect.HeightPT) || rect.WidthPT < 0 || rect.HeightPT < 0 {
		return errors.New("invalid point geometry")
	}
	return nil
}

func validateFragment(fragment wordlayout.TextFragment) error {
	if !finite(fragment.XPT) || !finite(fragment.BaselineYPT) || !finite(fragment.Style.FontSize) || fragment.Style.FontSize < 0 {
		return errors.New("invalid text geometry")
	}
	if fragment.Text != "" && len(fragment.Glyphs) == 0 {
		return errors.New("text has no positioned glyphs")
	}
	if fragment.Text != "" && (fragment.RuneStart < 0 || fragment.RuneEnd < fragment.RuneStart || fragment.RuneEnd-fragment.RuneStart != utf8.RuneCountInString(fragment.Text)) {
		return errors.New("text rune range does not match fragment text")
	}
	for _, glyph := range fragment.Glyphs {
		if glyph.FontKey == "" || !finite(glyph.XPT) || !finite(glyph.YPT) || !finite(glyph.AdvanceXPT) || glyph.AdvanceXPT < 0 {
			return errors.New("invalid positioned glyph")
		}
		hasPath := false
		for _, command := range glyph.Outline {
			if command.Op < wordlayout.GlyphPathMoveTo || command.Op > wordlayout.GlyphPathCurveTo {
				return errors.New("unknown glyph path command")
			}
			if command.Op == wordlayout.GlyphPathMoveTo {
				hasPath = true
			} else if !hasPath {
				return errors.New("glyph path segment has no initial move")
			}
			count := 1
			if command.Op == wordlayout.GlyphPathCurveTo {
				count = 3
			}
			for index := 0; index < count; index++ {
				if !finite(command.Points[index].XPT) || !finite(command.Points[index].YPT) {
					return errors.New("invalid glyph outline point")
				}
			}
		}
	}
	return nil
}

func drawTextFragment(page *gopdf.Page, fragment wordlayout.TextFragment) {
	pen := 0.0
	for _, glyph := range fragment.Glyphs {
		pen += glyph.AdvanceXPT
	}
	if highlight, ok := highlightColor(fragment.Style.Highlight); ok && pen > 0 {
		fontSize := effectiveFontSize(fragment.Style.FontSize)
		page.SetFillColor(highlight)
		page.Rect(fragment.XPT, fragment.BaselineYPT-fontSize*0.88, pen, fontSize*1.18, gopdf.Fill)
	}
	for _, glyph := range fragment.Glyphs {
		if len(glyph.Outline) == 0 {
			continue
		}
		page.SetFillColor(textColor(fragment.Style.FontColor))
		var hasPath bool
		for _, command := range glyph.Outline {
			switch command.Op {
			case wordlayout.GlyphPathMoveTo:
				if hasPath {
					page.ClosePath()
				}
				point := command.Points[0]
				page.MoveTo(glyph.XPT+point.XPT, glyph.YPT+point.YPT)
				hasPath = true
			case wordlayout.GlyphPathLineTo:
				point := command.Points[0]
				page.LineTo(glyph.XPT+point.XPT, glyph.YPT+point.YPT)
			case wordlayout.GlyphPathCurveTo:
				points := command.Points
				page.CurveTo(
					glyph.XPT+points[0].XPT, glyph.YPT+points[0].YPT,
					glyph.XPT+points[1].XPT, glyph.YPT+points[1].YPT,
					glyph.XPT+points[2].XPT, glyph.YPT+points[2].YPT,
				)
			}
		}
		if hasPath {
			page.ClosePath()
			page.DrawPath(gopdf.Fill)
		}
	}
	if fragment.Style.Underline && pen > 0 {
		fontSize := effectiveFontSize(fragment.Style.FontSize)
		page.SetFillColor(textColor(fragment.Style.FontColor))
		page.Rect(fragment.XPT, fragment.BaselineYPT+math.Max(0.5, fontSize*0.08), pen, 0.6, gopdf.Fill)
	}
}

func drawCellBorder(page *gopdf.Page, bounds wordlayout.Rect) {
	const thickness = 0.5
	ink := gopdf.RGB(136, 143, 151)
	page.SetFillColor(ink)
	page.Rect(bounds.XPT, bounds.YPT, bounds.WidthPT, thickness, gopdf.Fill)
	page.Rect(bounds.XPT, bounds.YPT+bounds.HeightPT-thickness, bounds.WidthPT, thickness, gopdf.Fill)
	page.Rect(bounds.XPT, bounds.YPT, thickness, bounds.HeightPT, gopdf.Fill)
	page.Rect(bounds.XPT+bounds.WidthPT-thickness, bounds.YPT, thickness, bounds.HeightPT, gopdf.Fill)
}

func fragmentTextChunks(fragment wordlayout.TextFragment) ([]textChunk, error) {
	if fragment.Text == "" {
		return nil, nil
	}
	runes := []rune(fragment.Text)
	type glyphCluster struct {
		start int
		end   int
		key   string
	}
	byRange := make(map[[2]int]string)
	for _, glyph := range fragment.Glyphs {
		start := glyph.RuneStart - fragment.RuneStart
		end := glyph.RuneEnd - fragment.RuneStart
		if start < 0 || end <= start || end > len(runes) {
			return nil, fmt.Errorf("glyph rune range [%d,%d) does not match fragment [%d,%d)", glyph.RuneStart, glyph.RuneEnd, fragment.RuneStart, fragment.RuneEnd)
		}
		rangeKey := [2]int{start, end}
		if existing := byRange[rangeKey]; existing != "" && existing != glyph.FontKey {
			return nil, errors.New("one Unicode cluster uses multiple PDF text fonts")
		}
		byRange[rangeKey] = glyph.FontKey
	}
	clusters := make([]glyphCluster, 0, len(byRange))
	for key, fontKey := range byRange {
		clusters = append(clusters, glyphCluster{start: key[0], end: key[1], key: fontKey})
	}
	for i := 1; i < len(clusters); i++ {
		for j := i; j > 0 && clusters[j].start < clusters[j-1].start; j-- {
			clusters[j], clusters[j-1] = clusters[j-1], clusters[j]
		}
	}
	chunks := make([]textChunk, 0, len(clusters))
	covered := 0
	for _, cluster := range clusters {
		if cluster.start != covered {
			return nil, errors.New("glyph clusters do not cover fragment text exactly once")
		}
		text := string(runes[cluster.start:cluster.end])
		if len(chunks) > 0 && chunks[len(chunks)-1].fontKey == cluster.key {
			chunks[len(chunks)-1].text += text
		} else {
			chunks = append(chunks, textChunk{fontKey: cluster.key, text: text})
		}
		covered = cluster.end
	}
	if covered != len(runes) {
		return nil, errors.New("glyph clusters do not cover fragment text")
	}
	return chunks, nil
}

func validateFontCoverage(data []byte, fontKey, text string) error {
	faceIndex, err := fontFaceIndex(fontKey)
	if err != nil {
		return err
	}
	faces, err := font.ParseTTC(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse font %q for Unicode mapping: %w", fontKey, err)
	}
	if faceIndex >= len(faces) {
		return fmt.Errorf("font %q requests face %d, but the file has %d faces", fontKey, faceIndex, len(faces))
	}
	if faceIndex != 0 {
		return fmt.Errorf("font %q selects face %d; PDF backend can only embed collection face 0 safely", fontKey, faceIndex)
	}
	face := faces[faceIndex]
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

func validateFontEmbedding(data []byte, key string) error {
	faceIndex, err := fontFaceIndex(key)
	if err != nil {
		return err
	}
	if faceIndex != 0 {
		return fmt.Errorf("font %q selects collection face %d; PDF embedding supports face 0 only", key, faceIndex)
	}
	if len(data) < 12 {
		return fmt.Errorf("font %q is too small to inspect embedding permissions", key)
	}
	base, err := sfntFaceOffset(data, faceIndex)
	if err != nil {
		return fmt.Errorf("inspect font %q embedding permissions: %w", key, err)
	}
	count := int(binary.BigEndian.Uint16(data[base+4 : base+6]))
	if base+12+count*16 > len(data) {
		return fmt.Errorf("font %q has a truncated table directory", key)
	}
	var fsType uint16
	var hasOS2, hasCFF bool
	for index := 0; index < count; index++ {
		record := base + 12 + index*16
		tag := string(data[record : record+4])
		offset := int(binary.BigEndian.Uint32(data[record+8 : record+12]))
		length := int(binary.BigEndian.Uint32(data[record+12 : record+16]))
		if offset < 0 || length < 0 || offset > len(data) || length > len(data)-offset {
			return fmt.Errorf("font %q has an invalid %q table", key, tag)
		}
		switch tag {
		case "OS/2":
			if length < 10 {
				return fmt.Errorf("font %q has a truncated OS/2 table", key)
			}
			fsType = binary.BigEndian.Uint16(data[offset+8 : offset+10])
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
