package pptx

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	gopptx "github.com/F31/go-pptx/v2/pptx"
	"glance/internal/document"
)

// Document wraps a PPTX presentation behind a safe format adapter.
// Third-party library types are kept strictly internal.
type shapeBodyProps struct {
	wrap         string
	autoFit      bool
	fillColorHex string
	fillGradient *document.PresentationGradient
}

type Document struct {
	mu          sync.RWMutex
	path        string
	pres        *gopptx.Presentation
	widthPT     float64
	heightPT    float64
	activeSlide int
	dirty       bool
	revision    uint64

	// Cached media images by target path (e.g. "ppt/media/image1.png")
	mediaImages    map[string]image.Image
	mediaBytes     map[string][]byte
	slideBodyProps map[int]map[uint32]shapeBodyProps
}

const (
	defaultSlideWidthPT  = 960.0 // 16:9 widescreen: 13.333 in * 72 pt/in = 960 pt
	defaultSlideHeightPT = 540.0 // 7.5 in * 72 pt/in = 540 pt
	emuPerPoint          = 12700.0
)

// Open opens an existing .pptx file.
func Open(path string) (*Document, error) {
	path = filepath.Clean(path)
	pres, err := gopptx.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开演示文稿失败: %w", err)
	}

	doc := &Document{
		path:           path,
		pres:           pres,
		widthPT:        defaultSlideWidthPT,
		heightPT:       defaultSlideHeightPT,
		mediaImages:    make(map[string]image.Image),
		mediaBytes:     make(map[string][]byte),
		slideBodyProps: make(map[int]map[uint32]shapeBodyProps),
		revision:       1,
	}

	// Read slide dimensions and preload media from the zip package
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		if fi, err := f.Stat(); err == nil {
			if zr, err := zip.NewReader(f, fi.Size()); err == nil {
				doc.inspectZipPackage(zr)
			}
		}
	}

	return doc, nil
}

// NewBlank creates a new presentation with one default title slide.
func NewBlank() (*Document, error) {
	pres, err := gopptx.New()
	if err != nil {
		return nil, fmt.Errorf("创建演示文稿失败: %w", err)
	}

	doc := &Document{
		pres:           pres,
		widthPT:        defaultSlideWidthPT,
		heightPT:       defaultSlideHeightPT,
		mediaImages:    make(map[string]image.Image),
		mediaBytes:     make(map[string][]byte),
		slideBodyProps: make(map[int]map[uint32]shapeBodyProps),
		dirty:          true,
		revision:       1,
	}

	// Add an initial title slide if layouts exist
	layouts, err := pres.Layouts()
	if err == nil && len(layouts) > 0 {
		slide, err := pres.AddSlide(layouts[0])
		if err == nil && slide != nil {
			// Title: 960 pt width, centered horizontally
			// In EMU: 960 pt * 12700 = 12192000 EMU
			// Title at X: 1.5 in (1371600 EMU), Y: 2.0 in (1828800 EMU), Width: 10.33 in, Height: 1.5 in
			_, _ = slide.AddTextBox(gopptx.TextBoxSpec{
				X:      1371600,
				Y:      1828800,
				Width:  9448800,
				Height: 1371600,
				Text:   "点击添加演示文稿标题",
			})
			// Subtitle at Y: 3.8 in
			_, _ = slide.AddTextBox(gopptx.TextBoxSpec{
				X:      1371600,
				Y:      3474720,
				Width:  9448800,
				Height: 914400,
				Text:   "点击添加副标题",
			})
		}
	}

	return doc, nil
}

// Close closes the presentation.
func (d *Document) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres != nil {
		return d.pres.Close()
	}
	return nil
}

// Path returns the canonical file path.
func (d *Document) Path() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.path
}

// IsDirty returns whether the document has unsaved changes.
func (d *Document) IsDirty() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.dirty
}

// Dirty returns whether the document has unsaved changes, conforming to the Document interface.
func (d *Document) Dirty() bool {
	return d.IsDirty()
}

// SetDirty manually sets the dirty flag.
func (d *Document) SetDirty(dirty bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dirty = dirty
}

// Revision returns the current revision count.
func (d *Document) Revision() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.revision
}

// ActiveSlideIndex returns the zero-based index of the active slide.
func (d *Document) ActiveSlideIndex() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.activeSlide
}

// SetActiveSlideIndex changes the active slide index.
func (d *Document) SetActiveSlideIndex(idx int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	slides, _ := d.pres.Slides()
	if idx >= 0 && idx < len(slides) {
		d.activeSlide = idx
	}
}

// SlideCount returns the total number of slides.
func (d *Document) SlideCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.pres == nil {
		return 0
	}
	slides, _ := d.pres.Slides()
	return len(slides)
}

// AddSlide adds a new slide with the default layout.
func (d *Document) AddSlide() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	layouts, err := d.pres.Layouts()
	if err != nil || len(layouts) == 0 {
		return fmt.Errorf("无可用版式")
	}

	slide, err := d.pres.AddSlide(layouts[0])
	if err != nil {
		return fmt.Errorf("新建幻灯片失败: %w", err)
	}

	// Add default title and content placeholders
	_, _ = slide.AddTextBox(gopptx.TextBoxSpec{
		X:      914400,
		Y:      731520,
		Width:  10363200,
		Height: 1097280,
		Text:   "点击添加标题",
	})
	_, _ = slide.AddTextBox(gopptx.TextBoxSpec{
		X:      914400,
		Y:      2011680,
		Width:  10363200,
		Height: 4114800,
		Text:   "• 点击添加正文文本\n• 支持多级要点与编辑",
	})

	slides, _ := d.pres.Slides()
	d.activeSlide = len(slides) - 1
	d.dirty = true
	d.revision++
	return nil
}

// DeleteSlide removes the slide at the given index.
func (d *Document) DeleteSlide(index int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	slides, err := d.pres.Slides()
	if err != nil || index < 0 || index >= len(slides) {
		return fmt.Errorf("幻灯片序号无效: %d", index)
	}
	if len(slides) <= 1 {
		return fmt.Errorf("无法删除演示文稿中的最后一页幻灯片")
	}

	target := slides[index]
	if err := d.pres.RemoveSlide(target.ID()); err != nil {
		return fmt.Errorf("删除幻灯片失败: %w", err)
	}

	if d.activeSlide >= len(slides)-1 {
		d.activeSlide = len(slides) - 2
	}
	d.dirty = true
	d.revision++
	return nil
}

// UpdateSlideText updates the text of a specific shape on a slide.
func (d *Document) UpdateSlideText(slideIndex int, shapeID int64, newText string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	slides, err := d.pres.Slides()
	if err != nil || slideIndex < 0 || slideIndex >= len(slides) {
		return fmt.Errorf("幻灯片序号无效: %d", slideIndex)
	}

	shapes, err := slides[slideIndex].Shapes()
	if err != nil {
		return fmt.Errorf("读取幻灯片形状失败: %w", err)
	}

	for _, sh := range shapes {
		if int64(sh.ID()) == shapeID {
			if autoSh, ok := sh.(*gopptx.AutoShape); ok {
				tf, err := autoSh.TextFrame()
				if err != nil || tf == nil {
					return fmt.Errorf("形状不包含文本框")
				}
				if err := tf.SetPlainText(newText); err != nil {
					return fmt.Errorf("设置文本失败: %w", err)
				}
				d.dirty = true
				d.revision++
				return nil
			}
		}
	}
	return fmt.Errorf("未找到形状 ID %d", shapeID)
}

// UpdateSlideNotes updates the speaker notes for a slide.
func (d *Document) UpdateSlideNotes(slideIndex int, notes string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	slides, err := d.pres.Slides()
	if err != nil || slideIndex < 0 || slideIndex >= len(slides) {
		return fmt.Errorf("幻灯片序号无效: %d", slideIndex)
	}

	if err := slides[slideIndex].SetSpeakerNotes(notes); err != nil {
		return fmt.Errorf("更新备注失败: %w", err)
	}
	d.dirty = true
	d.revision++
	return nil
}

// Save saves the presentation to its original path using an atomic temporary file swap.
func (d *Document) Save() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.path == "" {
		return fmt.Errorf("未指定保存路径，请使用另存为")
	}

	return d.saveAtomicLocked(d.path)
}

// SaveAs saves the presentation to a new path.
func (d *Document) SaveAs(targetPath string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	targetPath = filepath.Clean(targetPath)
	if err := d.saveAtomicLocked(targetPath); err != nil {
		return err
	}

	d.path = targetPath
	return nil
}

func (d *Document) saveAtomicLocked(targetPath string) error {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, "glance-pptx-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时保存文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	// Save to sibling temporary file
	if _, err := d.pres.Save(context.Background(), tmpPath, gopptx.WithSaveOverwrite(true)); err != nil {
		return fmt.Errorf("写入演示文稿失败: %w", err)
	}

	// Verify reopening before replacement
	checkPres, err := gopptx.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("校验保存后的演示文稿失败: %w", err)
	}
	_ = checkPres.Close()

	// Atomic replace
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("替换目标文件失败: %w", err)
	}

	d.dirty = false
	d.revision++
	return nil
}

// SnapshotBytes serializes the presentation to bytes for workspace recovery or draft snapshotting.
func (d *Document) SnapshotBytes() ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.pres == nil {
		return nil, fmt.Errorf("演示文稿已关闭")
	}

	tmpFile, err := os.CreateTemp("", "glance-pptx-snap-*.pptx")
	if err != nil {
		return nil, fmt.Errorf("创建快照临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	if _, err := d.pres.Save(context.Background(), tmpPath, gopptx.WithSaveOverwrite(true)); err != nil {
		return nil, fmt.Errorf("生成演示文稿快照失败: %w", err)
	}
	return os.ReadFile(tmpPath)
}

// OpenBytes loads a PPTX presentation from in-memory byte slice.
func OpenBytes(data []byte) (*Document, error) {
	tmpFile, err := os.CreateTemp("", "glance-pptx-restore-*.pptx")
	if err != nil {
		return nil, fmt.Errorf("创建恢复临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("写入恢复数据失败: %w", err)
	}
	_ = tmpFile.Close()

	doc, err := Open(tmpPath)
	if err != nil {
		return nil, err
	}
	doc.path = "" // restored drafts have no canonical disk path yet
	return doc, nil
}

// Snapshot generates an immutable view data model for the desktop UI.
func (d *Document) Snapshot() document.PresentationSnapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()

	title := "未命名演示文稿"
	if d.path != "" {
		title = filepath.Base(d.path)
	}

	snap := document.PresentationSnapshot{
		Title:       title,
		WidthPT:     d.widthPT,
		HeightPT:    d.heightPT,
		ActiveSlide: d.activeSlide,
		Revision:    d.revision,
		Dirty:       d.dirty,
	}

	if d.pres == nil {
		return snap
	}

	slides, err := d.pres.Slides()
	if err != nil {
		return snap
	}
	snap.SlideCount = len(slides)

	for i, sl := range slides {
		pSlide := document.PresentationSlide{
			Index:      i,
			ID:         uint32(sl.ID()),
			Title:      sl.Name(),
			WidthPT:    d.widthPT,
			HeightPT:   d.heightPT,
			BgColorHex: "#FFFFFF",
		}

		if notes, err := sl.SpeakerNotesText(); err == nil {
			pSlide.Notes = notes
		}

		// Extract shapes
		if shapes, err := sl.Shapes(); err == nil {
			for _, sh := range shapes {
				elem := d.projectShape(sh, i)
				pSlide.Elements = append(pSlide.Elements, elem)
				if pSlide.Title == "" && len(elem.Paragraphs) > 0 {
					pSlide.Title = elem.PlainText()
				}
			}
		}

		if pSlide.Title == "" {
			pSlide.Title = fmt.Sprintf("幻灯片 %d", i+1)
		}

		snap.Slides = append(snap.Slides, pSlide)
	}

	return snap
}

// projectShape maps a go-pptx Shape into neutral PresentationElement.
func (d *Document) projectShape(sh gopptx.Shape, slideIdx int) document.PresentationElement {
	elem := document.PresentationElement{
		ID:   int64(sh.ID()),
		Name: sh.Name(),
		Kind: document.ElementKindShape,
	}

	// Calculate bounds in points
	if bounds, err := sh.Bounds(); err == nil {
		elem.Bounds = document.PresentationRect{
			X: float64(bounds.X) / emuPerPoint,
			Y: float64(bounds.Y) / emuPerPoint,
			W: float64(bounds.W) / emuPerPoint,
			H: float64(bounds.H) / emuPerPoint,
		}
	}

	// Geometry / shape kind
	if geom, _, err := sh.Geometry(); err == nil {
		elem.ShapeType = string(geom.Preset)
		if geom.Kind == gopptx.GeometryCustom {
			elem.Paths = make([]document.PresentationPath, 0, len(geom.Paths))
			for _, path := range geom.Paths {
				projected := document.PresentationPath{
					Width:  float64(path.Width),
					Height: float64(path.Height),
					Fill:   path.Fill,
					Stroke: path.Stroke,
				}
				for _, command := range path.Commands {
					projectedCommand := document.PresentationPathCommand{Kind: command.Kind}
					for _, point := range command.Points {
						projectedCommand.Points = append(projectedCommand.Points, document.PresentationPoint{
							X: float64(point.X),
							Y: float64(point.Y),
						})
					}
					projected.Commands = append(projected.Commands, projectedCommand)
				}
				elem.Paths = append(elem.Paths, projected)
			}
		}
	}
	if elem.ShapeType == "" {
		elem.ShapeType = "rect"
	}

	// Fill color
	if fill, _, err := sh.Fill(); err == nil {
		if fill.Color.RGB != "" {
			elem.FillColorHex = "#" + fill.Color.RGB
		} else if fill.Color.Spec.RGB != "" {
			elem.FillColorHex = "#" + fill.Color.Spec.RGB
		}
		if fill.Gradient != nil {
			gradient := &document.PresentationGradient{
				AngleDeg: float64(fill.Gradient.Angle) / 60000,
				PathType: fill.Gradient.PathType,
				Stops:    make([]document.PresentationGradientStop, 0, len(fill.Gradient.Stops)),
			}
			for _, stop := range fill.Gradient.Stops {
				gradient.Stops = append(gradient.Stops, document.PresentationGradientStop{
					Position: float64(stop.Position),
					ColorHex: stop.Color.RGB,
					Alpha:    stop.Color.Alpha,
				})
			}
			elem.FillGradient = gradient
		}
	}

	// Line / border
	if line, _, err := sh.Line(); err == nil && line.Specified {
		elem.BorderWidth = float64(line.Width) / emuPerPoint
		if line.Color.RGB != "" {
			elem.BorderColor = "#" + line.Color.RGB
		} else if line.Color.Spec.RGB != "" {
			elem.BorderColor = "#" + line.Color.Spec.RGB
		}
	}

	// Text wrapping & auto-fit properties from slide XML
	elem.WrapText = true
	elem.AutoFit = false
	if slideProps, ok := d.slideBodyProps[slideIdx]; ok {
		if sp, ok := slideProps[uint32(sh.ID())]; ok {
			elem.WrapText = (sp.wrap != "none")
			elem.AutoFit = sp.autoFit
			if elem.FillColorHex == "" {
				elem.FillColorHex = sp.fillColorHex
			}
			if elem.FillGradient == nil {
				elem.FillGradient = sp.fillGradient
			}
		}
	}

	// Text content
	switch s := sh.(type) {
	case *gopptx.AutoShape:
		if tf, err := s.TextFrame(); err == nil && tf != nil {
			if paras, err := tf.Paragraphs(); err == nil {
				for _, para := range paras {
					pPara := document.PresentationParagraph{}
					if props, _, err := para.Props(); err == nil && props.Specified {
						switch props.Align {
						case "ctr":
							pPara.Align = "center"
						case "r":
							pPara.Align = "right"
						case "l":
							pPara.Align = "left"
						case "just":
							pPara.Align = "justify"
						default:
							pPara.Align = props.Align
						}
					}
					if runs, err := para.Runs(); err == nil {
						for _, r := range runs {
							pRun := document.PresentationTextRun{}
							if txt, err := r.Text(); err == nil {
								pRun.Text = txt
							}
							if font, err := r.ExplicitFont(); err == nil {
								if font.EastAsian.Set {
									pRun.Font = font.EastAsian.Value
								} else if font.Latin.Set {
									pRun.Font = font.Latin.Value
								}
								if font.Bold.Set {
									pRun.Bold = font.Bold.Value
								}
								if font.Italic.Set {
									pRun.Italic = font.Italic.Value
								}
								if font.Size.Set {
									pRun.SizePT = float64(font.Size.Value)
								}
								if font.Color.Set && font.Color.Value.RGB != "" {
									pRun.ColorHex = "#" + font.Color.Value.RGB
								}
							}
							if advanced, _, err := r.AdvancedProps(); err == nil {
								pRun.Baseline = advanced.Baseline
							}
							if pRun.SizePT <= 0 {
								pRun.SizePT = 18.0 // default reasonable slide font size
							}
							if pRun.ColorHex == "" {
								pRun.ColorHex = "#1F2937"
							}
							if pRun.Text != "" {
								pPara.Runs = append(pPara.Runs, pRun)
							}
						}
					}
					if len(pPara.Runs) > 0 {
						elem.Paragraphs = append(elem.Paragraphs, pPara)
					}
				}
			}
		}
		if len(elem.Paragraphs) > 0 {
			elem.Kind = document.ElementKindText
		}
	case *gopptx.PictureShape:
		elem.Kind = document.ElementKindPicture
		elem.AltText = s.AltText()
		// Try matching preloaded media images
		for _, img := range d.mediaImages {
			elem.Image = img
			break
		}
	}

	return elem
}

// inspectZipPackage extracts presentation dimensions and preloads media.
func (d *Document) inspectZipPackage(zr *zip.Reader) {
	files := make(map[string]*zip.File)
	for _, f := range zr.File {
		files[f.Name] = f
		// Preload images from ppt/media/
		if strings.HasPrefix(f.Name, "ppt/media/") {
			if rc, err := f.Open(); err == nil {
				imgBytes, _ := io.ReadAll(rc)
				rc.Close()
				d.mediaBytes[f.Name] = imgBytes
				if img, _, err := image.Decode(bytes.NewReader(imgBytes)); err == nil {
					d.mediaImages[f.Name] = img
				}
			}
		}
	}

	// Read presentation.xml to detect slide size and ordered slide IDs
	var orderedSlideFiles []string
	if presFile, ok := files["ppt/presentation.xml"]; ok {
		if rc, err := presFile.Open(); err == nil {
			data, _ := io.ReadAll(rc)
			rc.Close()
			d.parsePresentationXML(data)

			// Parse relationships to map rId -> target slide file
			var relsMap map[string]string
			if relsFile, ok := files["ppt/_rels/presentation.xml.rels"]; ok {
				if rrc, err := relsFile.Open(); err == nil {
					rData, _ := io.ReadAll(rrc)
					rrc.Close()
					relsMap = parsePresentationRels(rData)
				}
			}

			orderedSlideFiles = parsePresentationSlideFiles(data, relsMap)
		}
	}

	// Parse slide XMLs for shape text body properties (wrap, autoFit)
	if len(orderedSlideFiles) > 0 {
		for idx, slidePath := range orderedSlideFiles {
			if sf, ok := files[slidePath]; ok {
				if rc, err := sf.Open(); err == nil {
					data, _ := io.ReadAll(rc)
					rc.Close()
					d.slideBodyProps[idx] = parseSlideXML(data)
				}
			}
		}
	} else {
		// Fallback: parse all slide files matching ppt/slides/slide*.xml
		slideIdx := 0
		for _, f := range zr.File {
			if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
				if rc, err := f.Open(); err == nil {
					data, _ := io.ReadAll(rc)
					rc.Close()
					d.slideBodyProps[slideIdx] = parseSlideXML(data)
					slideIdx++
				}
			}
		}
	}
}

// parsePresentationXML parses <p:sldSz cx="..." cy="..."/> from presentation.xml
func (d *Document) parsePresentationXML(data []byte) {
	type sldSzXML struct {
		XMLName xml.Name `xml:"sldSz"`
		CX      int64    `xml:"cx,attr"`
		CY      int64    `xml:"cy,attr"`
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "sldSz" {
			var sz sldSzXML
			if err := decoder.DecodeElement(&sz, &se); err == nil {
				if sz.CX > 0 && sz.CY > 0 {
					d.widthPT = float64(sz.CX) / emuPerPoint
					d.heightPT = float64(sz.CY) / emuPerPoint
				}
			}
			break
		}
	}
}

// parsePresentationRels maps relationship IDs to target paths.
func parsePresentationRels(data []byte) map[string]string {
	rels := make(map[string]string)
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "Relationship" {
			var id, target string
			for _, a := range se.Attr {
				if a.Name.Local == "Id" {
					id = a.Value
				} else if a.Name.Local == "Target" {
					target = a.Value
				}
			}
			if id != "" && target != "" {
				rels[id] = target
			}
		}
	}
	return rels
}

// parsePresentationSlideFiles extracts ordered slide file paths from presentation.xml.
func parsePresentationSlideFiles(data []byte, rels map[string]string) []string {
	var files []string
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "sldId" {
			var rId string
			for _, a := range se.Attr {
				if a.Name.Local == "id" && strings.HasPrefix(a.Value, "rId") {
					rId = a.Value
				} else if a.Name.Local == "rId" {
					rId = a.Value
				}
			}
			if rId != "" {
				if tgt, ok := rels[rId]; ok {
					if !strings.HasPrefix(tgt, "/") {
						tgt = "ppt/" + tgt
					} else {
						tgt = strings.TrimPrefix(tgt, "/")
					}
					files = append(files, tgt)
				}
			}
		}
	}
	return files
}

// parseSlideXML parses text body properties, solid RGB fills, and linear RGB gradients.
func parseSlideXML(data []byte) map[uint32]shapeBodyProps {
	result := make(map[uint32]shapeBodyProps)
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var currentID uint32
	var currentWrap string
	var currentAutoFit bool
	var currentFillColorHex string
	var currentGradient *document.PresentationGradient
	gradientStopIndex := -1
	inShape := false
	inShapeProps := false
	inShapeLine := false
	inShapeSolidFill := false
	inShapeGradFill := false
	parseRGBHex := func(value string) (string, bool) {
		if len(value) != 6 {
			return "", false
		}
		if _, err := strconv.ParseUint(value, 16, 24); err != nil {
			return "", false
		}
		return "#" + strings.ToUpper(value), true
	}

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "sp":
				inShape = true
				currentID = 0
				currentWrap = ""
				currentAutoFit = false
				currentFillColorHex = ""
				currentGradient = nil
				gradientStopIndex = -1
				inShapeProps = false
				inShapeLine = false
				inShapeSolidFill = false
				inShapeGradFill = false
			case "cNvPr":
				if inShape {
					for _, a := range t.Attr {
						if a.Name.Local == "id" {
							var id uint32
							if _, err := fmt.Sscanf(a.Value, "%d", &id); err == nil {
								currentID = id
							}
						}
					}
				}
			case "spPr":
				if inShape {
					inShapeProps = true
				}
			case "ln":
				if inShapeProps {
					inShapeLine = true
				}
			case "solidFill":
				if inShapeProps && !inShapeLine && !inShapeGradFill {
					inShapeSolidFill = true
				}
			case "gradFill":
				if inShapeProps && !inShapeLine {
					currentGradient = &document.PresentationGradient{}
					inShapeGradFill = true
				}
			case "gs":
				if inShapeGradFill && currentGradient != nil {
					stop := document.PresentationGradientStop{Position: -1, Alpha: 1}
					for _, attr := range t.Attr {
						if attr.Name.Local == "pos" {
							if position, err := strconv.ParseFloat(attr.Value, 64); err == nil {
								stop.Position = position
							}
						}
					}
					currentGradient.Stops = append(currentGradient.Stops, stop)
					gradientStopIndex = len(currentGradient.Stops) - 1
				}
			case "lin":
				if inShapeGradFill && currentGradient != nil {
					for _, attr := range t.Attr {
						if attr.Name.Local == "ang" {
							if angle, err := strconv.ParseFloat(attr.Value, 64); err == nil {
								currentGradient.AngleDeg = angle / 60000
							}
						}
					}
				}
			case "path":
				if inShapeGradFill && currentGradient != nil {
					for _, attr := range t.Attr {
						if attr.Name.Local == "path" {
							currentGradient.PathType = attr.Value
						}
					}
				}
			case "srgbClr":
				for _, attr := range t.Attr {
					if attr.Name.Local != "val" {
						continue
					}
					if inShapeGradFill && currentGradient != nil && gradientStopIndex >= 0 {
						if hex, ok := parseRGBHex(attr.Value); ok {
							currentGradient.Stops[gradientStopIndex].ColorHex = hex
						}
					} else if inShapeSolidFill {
						if hex, ok := parseRGBHex(attr.Value); ok {
							currentFillColorHex = hex
						}
					}
				}
			case "alpha":
				if inShapeGradFill && currentGradient != nil && gradientStopIndex >= 0 {
					for _, attr := range t.Attr {
						if attr.Name.Local == "val" {
							if alpha, err := strconv.ParseFloat(attr.Value, 64); err == nil {
								currentGradient.Stops[gradientStopIndex].Alpha = min(1, max(0, alpha/100000))
							}
						}
					}
				}
			case "bodyPr":
				if inShape {
					for _, a := range t.Attr {
						if a.Name.Local == "wrap" {
							currentWrap = a.Value
						}
					}
				}
			case "spAutoFit", "normAutofit":
				if inShape {
					currentAutoFit = true
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "solidFill":
				inShapeSolidFill = false
			case "gs":
				gradientStopIndex = -1
			case "gradFill":
				inShapeGradFill = false
			case "ln":
				inShapeLine = false
			case "spPr":
				inShapeProps = false
			case "sp":
				if currentID > 0 {
					result[currentID] = shapeBodyProps{
						wrap:         currentWrap,
						autoFit:      currentAutoFit,
						fillColorHex: currentFillColorHex,
						fillGradient: currentGradient,
					}
				}
				inShape = false
			}
		}
	}
	return result
}
