package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/SalvioniDigitalSolutions/gopdf"

	"glance/internal/document"
	"glance/internal/wordlayout"
)

const (
	maxPresentationPDFSlides   = 2000
	maxPresentationPDFElements = 100000
	maxPresentationPDFImage    = 32 << 20
	maxPresentationPDFImages   = 128 << 20
)

// PresentationTextSource shapes neutral presentation text and returns vector
// outlines for the selected system font faces.
type PresentationTextSource interface {
	wordlayout.TextShaper
	wordlayout.GlyphOutlineSource
}

// WritePresentationPDF writes one A4 page per slide. Slide content is drawn
// from the neutral presentation snapshot and scaled uniformly to fit the page.
func WritePresentationPDF(ctx context.Context, dst io.Writer, snapshot document.PresentationSnapshot, orientation string, fonts PresentationTextSource) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if dst == nil {
		return errors.New("PDF destination is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(snapshot.Slides) == 0 || len(snapshot.Slides) != snapshot.SlideCount {
		return fmt.Errorf("presentation snapshot has %d projected slides for %d total slides", len(snapshot.Slides), snapshot.SlideCount)
	}
	if len(snapshot.Slides) > maxPresentationPDFSlides {
		return fmt.Errorf("presentation has %d slides; PDF export limit is %d", len(snapshot.Slides), maxPresentationPDFSlides)
	}
	orientation = strings.ToLower(strings.TrimSpace(orientation))
	pageWidth, pageHeight := 595.28, 841.89
	switch orientation {
	case "", "portrait":
	case "landscape":
		pageWidth, pageHeight = pageHeight, pageWidth
	default:
		return fmt.Errorf("unsupported presentation PDF orientation %q", orientation)
	}
	elementCount, imageBytes, hasText := presentationPDFResources(snapshot)
	if elementCount > maxPresentationPDFElements {
		return fmt.Errorf("presentation has %d projected elements; PDF export limit is %d", elementCount, maxPresentationPDFElements)
	}
	if imageBytes > maxPresentationPDFImages {
		return fmt.Errorf("presentation images use %d bytes; PDF export limit is %d", imageBytes, maxPresentationPDFImages)
	}
	if hasText && fonts == nil {
		return errors.New("presentation PDF text requires a system font source")
	}

	pdf := gopdf.New()
	pdf.CompressObjects = true
	fontCache := make(map[string]*gopdf.Font)
	fontBytes := make(map[string][]byte)
	imageCache := make(map[string]*gopdf.Image)
	for slideIndex, slide := range snapshot.Slides {
		if err := ctx.Err(); err != nil {
			return err
		}
		width, height := slide.WidthPT, slide.HeightPT
		if width <= 0 {
			width = snapshot.WidthPT
		}
		if height <= 0 {
			height = snapshot.HeightPT
		}
		if !finite(width) || !finite(height) || width <= 0 || height <= 0 || width > 20000 || height > 20000 {
			return fmt.Errorf("presentation slide %d has invalid dimensions %.2fx%.2f points", slideIndex+1, width, height)
		}
		page := pdf.AddPageSize(gopdf.PageSize{W: pageWidth, H: pageHeight})
		page.ClipRect(0, 0, pageWidth, pageHeight)
		page.SetFillColor(gopdf.White)
		page.Rect(0, 0, pageWidth, pageHeight, gopdf.Fill)
		background, err := presentationPDFColor(slide.BgColorHex, gopdf.White)
		if err != nil {
			return fmt.Errorf("presentation slide %d background: %w", slideIndex+1, err)
		}
		page.SetFillColor(background)
		page.Rect(0, 0, pageWidth, pageHeight, gopdf.Fill)
		scale := math.Min(pageWidth/width, pageHeight/height)
		offsetX := (pageWidth - width*scale) / 2
		offsetY := (pageHeight - height*scale) / 2
		page.Push()
		page.Translate(offsetX, offsetY)
		page.Scale(scale, scale, 0, 0)
		if slide.BackgroundImage != nil {
			cacheKey := fmt.Sprintf("slide-background:%d", slideIndex)
			if err := drawPresentationSnapshotImage(pdf, page, cacheKey, slide.BackgroundImage, imageCache, image.Rect(0, 0, int(width), int(height)), width, height); err != nil {
				page.Pop()
				return fmt.Errorf("presentation slide %d background image: %w", slideIndex+1, err)
			}
		}
		for _, element := range slide.InheritedElements {
			if err := drawPresentationPDFElement(ctx, pdf, page, slideIndex, element, fonts, fontCache, fontBytes, imageCache); err != nil {
				page.Pop()
				return err
			}
		}
		for _, element := range slide.Elements {
			if err := drawPresentationPDFElement(ctx, pdf, page, slideIndex, element, fonts, fontCache, fontBytes, imageCache); err != nil {
				page.Pop()
				return err
			}
		}
		page.Pop()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := pdf.WriteTo(contextWriter{ctx: ctx, dst: dst}); err != nil {
		return fmt.Errorf("serialize presentation PDF: %w", err)
	}
	return nil
}

// RenderPresentationSlidePreview renders a single slide through the same PDF
// scene used by export so the preview and saved output share the same content.
func RenderPresentationSlidePreview(ctx context.Context, snapshot document.PresentationSnapshot, slideIndex int, orientation string, fonts PresentationTextSource) (image.Image, error) {
	if slideIndex < 0 || slideIndex >= len(snapshot.Slides) {
		return nil, fmt.Errorf("presentation slide index %d is out of range", slideIndex)
	}
	single := snapshot
	single.Slides = []document.PresentationSlide{snapshot.Slides[slideIndex]}
	single.SlideCount = 1
	var data bytes.Buffer
	if err := WritePresentationPDF(ctx, &data, single, orientation, fonts); err != nil {
		return nil, err
	}
	reader, err := gopdf.NewReader(data.Bytes())
	if err != nil {
		return nil, fmt.Errorf("open presentation preview PDF: %w", err)
	}
	preview, err := reader.RenderPage(0, gopdf.RenderOpts{DPI: 48, IncludeVector: true, IncludeRasterImages: true, IncludeText: true})
	if err != nil {
		return nil, fmt.Errorf("render presentation preview: %w", err)
	}
	return preview, nil
}

func presentationPDFResources(snapshot document.PresentationSnapshot) (elements, imageBytes int, hasText bool) {
	addImageBytes := func(amount int) {
		if amount == 0 {
			return
		}
		if amount < 0 || imageBytes > maxPresentationPDFImages-amount {
			imageBytes = maxPresentationPDFImages + 1
			return
		}
		imageBytes += amount
	}
	imageMemory := func(source image.Image) int {
		if source == nil {
			return 0
		}
		bounds := source.Bounds()
		width, height := int64(bounds.Dx()), int64(bounds.Dy())
		if width <= 0 || height <= 0 || width > int64(maxPresentationPDFImages)/4/height {
			return maxPresentationPDFImages + 1
		}
		return int(width * height * 4)
	}
	visit := func(element document.PresentationElement) {
		elements++
		if len(element.Paragraphs) > 0 || element.Kind == document.ElementKindAudio || element.Kind == document.ElementKindVideo {
			hasText = true
		}
		addImageBytes(len(element.ImageBytes))
		addImageBytes(imageMemory(element.Image))
	}
	for _, slide := range snapshot.Slides {
		addImageBytes(imageMemory(slide.BackgroundImage))
		for _, element := range slide.InheritedElements {
			visit(element)
		}
		for _, element := range slide.Elements {
			visit(element)
		}
	}
	return elements, imageBytes, hasText
}

func drawPresentationPDFElement(ctx context.Context, pdf *gopdf.Document, page *gopdf.Page, slideIndex int, element document.PresentationElement, fonts PresentationTextSource, fontCache map[string]*gopdf.Font, fontBytes map[string][]byte, imageCache map[string]*gopdf.Image) error {
	bounds := element.Bounds
	if !finite(bounds.X) || !finite(bounds.Y) || !finite(bounds.W) || !finite(bounds.H) || bounds.W <= 0 || bounds.H <= 0 || bounds.W > 20000 || bounds.H > 20000 {
		return fmt.Errorf("presentation slide %d element %d has invalid geometry", slideIndex+1, element.ID)
	}
	if !finite(element.RotationDeg) {
		return fmt.Errorf("presentation slide %d element %d has invalid rotation", slideIndex+1, element.ID)
	}
	if element.FillGradient != nil {
		return fmt.Errorf("presentation slide %d element %d uses a gradient fill that PDF export cannot render safely", slideIndex+1, element.ID)
	}
	page.Push()
	if element.RotationDeg != 0 {
		page.RotateAt(-element.RotationDeg, bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
	}
	page.ClipRect(bounds.X, bounds.Y, bounds.W, bounds.H)
	if element.Kind == document.ElementKindPicture {
		if err := drawPresentationPDFPicture(ctx, pdf, page, slideIndex, element, imageCache); err != nil {
			page.Pop()
			return err
		}
		if element.BorderColor != "" {
			if err := drawPresentationPDFShape(page, element); err != nil {
				page.Pop()
				return err
			}
		}
		page.Pop()
		return nil
	}
	if element.Kind == document.ElementKindTable {
		page.Pop()
		return fmt.Errorf("presentation slide %d element %d is a table outside the supported PDF projection", slideIndex+1, element.ID)
	}
	if err := drawPresentationPDFShape(page, element); err != nil {
		page.Pop()
		return fmt.Errorf("presentation slide %d element %d: %w", slideIndex+1, element.ID, err)
	}
	if len(element.Paragraphs) > 0 || element.Kind == document.ElementKindAudio || element.Kind == document.ElementKindVideo {
		if err := drawPresentationPDFText(ctx, page, slideIndex, element, fonts, fontCache, fontBytes); err != nil {
			page.Pop()
			return err
		}
	}
	page.Pop()
	return nil
}

func drawPresentationPDFShape(page *gopdf.Page, element document.PresentationElement) error {
	bounds := element.Bounds
	fill, err := presentationPDFColor(element.FillColorHex, gopdf.Color{})
	if err != nil {
		return err
	}
	stroke, err := presentationPDFColor(element.BorderColor, gopdf.Color{})
	if err != nil {
		return err
	}
	lineWidth := element.BorderWidth
	if lineWidth <= 0 {
		lineWidth = 1
	}
	if len(element.Paths) == 0 && element.FillColorHex == "" && element.BorderColor == "" {
		return nil
	}
	page.SetFillColor(fill)
	page.SetStrokeColor(stroke)
	page.SetLineWidth(lineWidth)
	drawMode := func(doFill, doStroke bool) gopdf.DrawMode {
		switch {
		case doFill && doStroke:
			return gopdf.FillStroke
		case doFill:
			return gopdf.Fill
		case !doFill && !doStroke:
			return gopdf.ClipPath
		default:
			return gopdf.Stroke
		}
	}
	if len(element.Paths) > 0 {
		for pathIndex, source := range element.Paths {
			if source.Width <= 0 || source.Height <= 0 {
				return fmt.Errorf("custom path %d has invalid local dimensions", pathIndex+1)
			}
			page.Push()
			page.Translate(bounds.X, bounds.Y)
			page.Scale(bounds.W/source.Width, bounds.H/source.Height, 0, 0)
			var current, subpathStart document.PresentationPoint
			hasCurrent, hasSubpathStart := false, false
			for commandIndex, command := range source.Commands {
				switch command.Kind {
				case "moveTo":
					if len(command.Points) != 1 || !finite(command.Points[0].X) || !finite(command.Points[0].Y) {
						page.Pop()
						return fmt.Errorf("custom path %d command %d has invalid moveTo points", pathIndex+1, commandIndex+1)
					}
					page.MoveTo(command.Points[0].X, command.Points[0].Y)
					current, subpathStart = command.Points[0], command.Points[0]
					hasCurrent, hasSubpathStart = true, true
				case "lnTo":
					if len(command.Points) != 1 || !hasCurrent || !finite(command.Points[0].X) || !finite(command.Points[0].Y) {
						page.Pop()
						return fmt.Errorf("custom path %d command %d has invalid lnTo points", pathIndex+1, commandIndex+1)
					}
					page.LineTo(command.Points[0].X, command.Points[0].Y)
					current = command.Points[0]
				case "cubicBezTo":
					if len(command.Points) != 3 || !hasCurrent || !finitePresentationPoints(command.Points) {
						page.Pop()
						return fmt.Errorf("custom path %d command %d has invalid cubicBezTo points", pathIndex+1, commandIndex+1)
					}
					p := command.Points
					page.CurveTo(p[0].X, p[0].Y, p[1].X, p[1].Y, p[2].X, p[2].Y)
					current = p[2]
				case "quadBezTo":
					if len(command.Points) != 2 || !hasCurrent || !finitePresentationPoints(command.Points) {
						page.Pop()
						return fmt.Errorf("custom path %d command %d has invalid quadBezTo points", pathIndex+1, commandIndex+1)
					}
					// Convert the quadratic curve to an equivalent cubic curve.
					control, end := command.Points[0], command.Points[1]
					control1 := document.PresentationPoint{X: current.X + (control.X-current.X)*2/3, Y: current.Y + (control.Y-current.Y)*2/3}
					control2 := document.PresentationPoint{X: end.X + (control.X-end.X)*2/3, Y: end.Y + (control.Y-end.Y)*2/3}
					page.CurveTo(control1.X, control1.Y, control2.X, control2.Y, end.X, end.Y)
					current = end
				case "close":
					if !hasCurrent || !hasSubpathStart {
						page.Pop()
						return fmt.Errorf("custom path %d command %d closes without a current subpath", pathIndex+1, commandIndex+1)
					}
					page.ClosePath()
					current = subpathStart
				default:
					page.Pop()
					return fmt.Errorf("custom path %d uses unsupported command %q", pathIndex+1, command.Kind)
				}
			}
			mode := drawMode(!strings.EqualFold(source.Fill, "none") && element.FillColorHex != "", !strings.EqualFold(source.Stroke, "none") && element.BorderColor != "")
			page.DrawPath(mode)
			page.Pop()
		}
		return nil
	}
	doFill := element.FillColorHex != ""
	doStroke := element.BorderColor != ""
	switch strings.ToLower(element.ShapeType) {
	case "line", "lineinv":
		x1, y1, x2, y2 := bounds.X, bounds.Y, bounds.X+bounds.W, bounds.Y+bounds.H
		if strings.EqualFold(element.ShapeType, "lineinv") {
			y1, y2 = bounds.Y+bounds.H, bounds.Y
		}
		page.Line(x1, y1, x2, y2)
	case "ellipse", "oval":
		page.Ellipse(bounds.X+bounds.W/2, bounds.Y+bounds.H/2, bounds.W/2, bounds.H/2, drawMode(doFill, doStroke))
	case "roundrect", "roundrectangle":
		page.RoundedRect(bounds.X, bounds.Y, bounds.W, bounds.H, math.Min(bounds.W, bounds.H)/8, drawMode(doFill, doStroke))
	default:
		page.Rect(bounds.X, bounds.Y, bounds.W, bounds.H, drawMode(doFill, doStroke))
	}
	return nil
}

func finitePresentationPoints(points []document.PresentationPoint) bool {
	for _, point := range points {
		if !finite(point.X) || !finite(point.Y) {
			return false
		}
	}
	return true
}

func drawPresentationPDFPicture(ctx context.Context, pdf *gopdf.Document, page *gopdf.Page, slideIndex int, element document.PresentationElement, imageCache map[string]*gopdf.Image) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if element.Crop.Left < 0 || element.Crop.Top < 0 || element.Crop.Right < 0 || element.Crop.Bottom < 0 || element.Crop.Left+element.Crop.Right >= 100000 || element.Crop.Top+element.Crop.Bottom >= 100000 {
		return fmt.Errorf("presentation slide %d picture %d has invalid crop margins", slideIndex+1, element.ID)
	}
	cacheKey := fmt.Sprintf("%d:%d", slideIndex, element.ID)
	imageData := bytes.Clone(element.ImageBytes)
	if len(imageData) == 0 && element.Image != nil {
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, element.Image); err != nil {
			return fmt.Errorf("encode presentation slide %d picture %d: %w", slideIndex+1, element.ID, err)
		}
		imageData = encoded.Bytes()
	}
	if len(imageData) == 0 {
		return fmt.Errorf("presentation slide %d picture %d has no projected image data", slideIndex+1, element.ID)
	}
	if len(imageData) > maxPresentationPDFImage {
		return fmt.Errorf("presentation slide %d picture %d exceeds the PDF image limit of %d bytes", slideIndex+1, element.ID, maxPresentationPDFImage)
	}
	pdfImage := imageCache[cacheKey]
	if pdfImage == nil {
		var err error
		pdfImage, err = pdf.AddImageReader(bytes.NewReader(imageData))
		if err != nil {
			return fmt.Errorf("decode presentation slide %d picture %d: %w", slideIndex+1, element.ID, err)
		}
		imageCache[cacheKey] = pdfImage
	}
	bounds := element.Bounds
	left, top := float64(element.Crop.Left)/100000, float64(element.Crop.Top)/100000
	right, bottom := float64(element.Crop.Right)/100000, float64(element.Crop.Bottom)/100000
	visibleWidth, visibleHeight := 1-left-right, 1-top-bottom
	fullWidth, fullHeight := bounds.W/visibleWidth, bounds.H/visibleHeight
	page.Push()
	page.ClipRect(bounds.X, bounds.Y, bounds.W, bounds.H)
	page.DrawImage(pdfImage, bounds.X-left*fullWidth, bounds.Y-top*fullHeight, fullWidth, fullHeight)
	page.Pop()
	return nil
}

func drawPresentationSnapshotImage(pdf *gopdf.Document, page *gopdf.Page, key string, source image.Image, imageCache map[string]*gopdf.Image, rect image.Rectangle, width, height float64) error {
	if source == nil {
		return nil
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		return err
	}
	if encoded.Len() > maxPresentationPDFImage {
		return fmt.Errorf("image exceeds %d bytes", maxPresentationPDFImage)
	}
	pdfImage := imageCache[key]
	if pdfImage == nil {
		var err error
		pdfImage, err = pdf.AddImageReader(bytes.NewReader(encoded.Bytes()))
		if err != nil {
			return err
		}
		imageCache[key] = pdfImage
	}
	page.DrawImage(pdfImage, float64(rect.Min.X), float64(rect.Min.Y), width, height)
	return nil
}

func drawPresentationPDFText(ctx context.Context, page *gopdf.Page, slideIndex int, element document.PresentationElement, fonts PresentationTextSource, fontCache map[string]*gopdf.Font, fontBytes map[string][]byte) error {
	paragraphs := element.Paragraphs
	if len(paragraphs) == 0 {
		label := "音频"
		if element.Kind == document.ElementKindVideo {
			label = "视频"
		}
		paragraphs = []document.PresentationParagraph{{Runs: []document.PresentationTextRun{{Text: label, SizePT: 14, ColorHex: "#1F2937"}}}}
	}
	blocks := make([]document.WordBlock, 0, len(paragraphs))
	for paragraphIndex, paragraph := range paragraphs {
		wordParagraph := document.WordParagraph{ID: fmt.Sprintf("pptx-%d-%d-%d", slideIndex+1, element.ID, paragraphIndex)}
		switch strings.ToLower(paragraph.Align) {
		case "center":
			wordParagraph.Alignment = document.WordAlignCenter
		case "right":
			wordParagraph.Alignment = document.WordAlignRight
		case "justify":
			wordParagraph.Alignment = document.WordAlignJustified
		default:
			wordParagraph.Alignment = document.WordAlignLeft
		}
		wordParagraph.Layout.Alignment = wordParagraph.Alignment
		wordParagraph.Layout.Set |= document.WordParagraphAlignmentSet
		if paragraph.Bullet {
			wordParagraph.ListKind = document.WordListBullet
		}
		if paragraph.LineSpacing != nil {
			if err := applyPresentationSpacing(paragraph.LineSpacing, &wordParagraph.Layout, "line"); err != nil {
				return fmt.Errorf("presentation slide %d element %d: %w", slideIndex+1, element.ID, err)
			}
		}
		if paragraph.SpaceBefore != nil {
			if err := applyPresentationSpacing(paragraph.SpaceBefore, &wordParagraph.Layout, "before"); err != nil {
				return fmt.Errorf("presentation slide %d element %d: %w", slideIndex+1, element.ID, err)
			}
		}
		if paragraph.SpaceAfter != nil {
			if err := applyPresentationSpacing(paragraph.SpaceAfter, &wordParagraph.Layout, "after"); err != nil {
				return fmt.Errorf("presentation slide %d element %d: %w", slideIndex+1, element.ID, err)
			}
		}
		for runIndex, run := range paragraph.Runs {
			if run.Baseline != 0 {
				return fmt.Errorf("presentation slide %d element %d run %d uses a baseline shift that PDF export cannot reproduce", slideIndex+1, element.ID, runIndex+1)
			}
			size := run.SizePT
			if size <= 0 {
				size = 18
			}
			if !finite(size) || size > 512 {
				return fmt.Errorf("presentation slide %d element %d run %d has invalid font size", slideIndex+1, element.ID, runIndex+1)
			}
			style := document.WordTextStyle{FontFamily: run.Font, FontSize: size, Bold: run.Bold, Italic: run.Italic, Underline: run.Underline, FontColor: run.ColorHex}
			text := &document.WordTextRun{ID: fmt.Sprintf("pptx-%d-%d-%d-%d", slideIndex+1, element.ID, paragraphIndex, runIndex), Text: run.Text, Style: style,
				StyleSet: document.WordTextBoldSet | document.WordTextItalicSet | document.WordTextUnderlineSet | document.WordTextStrikeSet | document.WordTextFontFamilySet | document.WordTextFontSizeSet | document.WordTextFontColorSet}
			text.Style.Strike = false
			wordParagraph.Inlines = append(wordParagraph.Inlines, document.WordInline{Kind: document.WordTextInline, Text: text})
		}
		blocks = append(blocks, document.WordBlock{ID: wordParagraph.ID, Kind: document.WordParagraphBlock, Paragraph: &wordParagraph})
	}
	setup := document.WordPageSetup{
		WidthPT: element.Bounds.W, HeightPT: element.Bounds.H,
		Columns: 1,
		Set: document.WordPageWidthSet | document.WordPageHeightSet |
			document.WordPageMarginTopSet | document.WordPageMarginRightSet |
			document.WordPageMarginBottomSet | document.WordPageMarginLeftSet,
	}
	layoutResult, err := wordlayout.NewEngine(fonts).Layout(ctx, wordlayout.Input{DocumentID: "presentation-pdf", Blocks: blocks, DefaultPageSetup: setup})
	if err != nil {
		return fmt.Errorf("presentation slide %d element %d text layout: %w", slideIndex+1, element.ID, err)
	}
	if !layoutResult.CanPreview || len(layoutResult.Pages) != 1 {
		return fmt.Errorf("presentation slide %d element %d text exceeds its projected bounds", slideIndex+1, element.ID)
	}
	for _, diagnostic := range layoutResult.Diagnostics {
		if diagnostic.Severity == wordlayout.DiagnosticError {
			return fmt.Errorf("presentation slide %d element %d text: %s", slideIndex+1, element.ID, diagnostic.Detail)
		}
	}
	for _, local := range layoutResult.Pages[0].Text {
		fragment := local
		fragment.XPT += element.Bounds.X
		fragment.BaselineYPT += element.Bounds.Y
		for index := range fragment.Glyphs {
			fragment.Glyphs[index].XPT += element.Bounds.X
			fragment.Glyphs[index].YPT += element.Bounds.Y
		}
		drawTextFragment(page, fragment)
		chunks, err := fragmentTextChunks(fragment)
		if err != nil {
			return fmt.Errorf("presentation slide %d element %d text layer: %w", slideIndex+1, element.ID, err)
		}
		for _, chunk := range chunks {
			pdfFont := fontCache[chunk.fontKey]
			if pdfFont == nil {
				raw, err := fonts.FontData(chunk.fontKey)
				if err != nil {
					return fmt.Errorf("presentation slide %d element %d load font %q: %w", slideIndex+1, element.ID, chunk.fontKey, err)
				}
				faceIndex, err := fontFaceIndex(chunk.fontKey)
				if err != nil {
					return err
				}
				data, err := extractSFNT(raw, faceIndex)
				if err != nil {
					return err
				}
				if err := validateFontEmbedding(data, chunk.fontKey); err != nil {
					return err
				}
				if err := validateFontCoverage(data, chunk.fontKey, chunk.text); err != nil {
					return err
				}
				pdfFont, err = gopdf.ParseFont(data)
				if err != nil {
					return fmt.Errorf("parse presentation font %q: %w", chunk.fontKey, err)
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
	return nil
}

func applyPresentationSpacing(source *document.PresentationSpacing, target *document.WordParagraphLayout, kind string) error {
	if source == nil || !finite(source.Value) || source.Value < 0 || source.Value > 10000 {
		return errors.New("invalid paragraph spacing")
	}
	switch kind {
	case "before":
		target.BeforePT = source.Value
		if source.Kind != "pts" {
			return errors.New("paragraph space_before must use point units")
		}
		target.Set |= document.WordParagraphBeforeSet
	case "after":
		target.AfterPT = source.Value
		if source.Kind != "pts" {
			return errors.New("paragraph space_after must use point units")
		}
		target.Set |= document.WordParagraphAfterSet
	case "line":
		switch source.Kind {
		case "pts":
			target.LineSpacingPT = source.Value
			target.LineSpacingRule = "exact"
			target.Set |= document.WordParagraphLineSpacingSet
		case "pct":
			target.LineSpacingMultiple = source.Value
			target.LineSpacingRule = "multiple"
			target.Set |= document.WordParagraphLineSpacingSet
		default:
			return fmt.Errorf("unsupported line spacing kind %q", source.Kind)
		}
	}
	return nil
}

func presentationPDFColor(value string, fallback gopdf.Color) (gopdf.Color, error) {
	value = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(value), "#"))
	if value == "" || value == "none" {
		return fallback, nil
	}
	if len(value) != 6 {
		return gopdf.Color{}, fmt.Errorf("invalid RGB color %q", value)
	}
	component := func(start int) (uint8, error) {
		parsed, err := strconv.ParseUint(value[start:start+2], 16, 8)
		return uint8(parsed), err
	}
	r, err := component(0)
	if err != nil {
		return gopdf.Color{}, fmt.Errorf("invalid RGB color %q", value)
	}
	g, err := component(2)
	if err != nil {
		return gopdf.Color{}, fmt.Errorf("invalid RGB color %q", value)
	}
	b, err := component(4)
	if err != nil {
		return gopdf.Color{}, fmt.Errorf("invalid RGB color %q", value)
	}
	return gopdf.RGB(r, g, b), nil
}
