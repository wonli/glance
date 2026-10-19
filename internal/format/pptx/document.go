package pptx

import (
	"archive/zip"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sync"

	"glance/internal/document"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

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
	mediaImages              map[string]image.Image
	mediaBytes               map[string][]byte
	slideBodyProps           map[int]map[uint32]shapeBodyProps
	slidePictureTargets      map[int]map[uint32]string
	slideBackgrounds         []pptxBackground
	slideInherited           [][]document.PresentationElement
	slidePlaceholderBounds   []map[uint32]document.PresentationRect
	slidePlaceholderTypes    []map[uint32]string
	slidePlaceholderChanges  []map[uint32]string
	slideShapeTransforms     []map[uint32]presentationTransformMetadata
	slidePlaceholderXfrms    []map[uint32]presentationTransformMetadata
	slideTransformChanges    []map[uint32]presentationTransformOverride
	slideStyleChanges        []map[uint32]document.PresentationElementStylePatch
	slidePictureCropStates   []map[uint32]presentationPictureCropState
	slidePictureCropChanges  []map[uint32]document.PresentationCrop
	slidePictureImageChanges []map[uint32]presentationPictureImageOverride
	slideBackgroundChanges   []presentationBackgroundOverride
	slideSizeChange          presentationSlideSizeOverride
	presentationComments     *pptxCommentState
	slideLayoutChanges       map[uint32]string
	slideAnimationChanges    map[uint32][]byte
	defaultSlideBackground   pptxBackground
	defaultSlideInherited    []document.PresentationElement
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
		path:                     path,
		pres:                     pres,
		widthPT:                  defaultSlideWidthPT,
		heightPT:                 defaultSlideHeightPT,
		mediaImages:              make(map[string]image.Image),
		mediaBytes:               make(map[string][]byte),
		presentationComments:     newPPTXCommentState(),
		slideLayoutChanges:       make(map[uint32]string),
		slideAnimationChanges:    make(map[uint32][]byte),
		slideBodyProps:           make(map[int]map[uint32]shapeBodyProps),
		slidePictureTargets:      make(map[int]map[uint32]string),
		slideTransformChanges:    make([]map[uint32]presentationTransformOverride, 0),
		slideStyleChanges:        make([]map[uint32]document.PresentationElementStylePatch, 0),
		slidePictureCropChanges:  make([]map[uint32]document.PresentationCrop, 0),
		slidePictureImageChanges: make([]map[uint32]presentationPictureImageOverride, 0),
		slidePlaceholderChanges:  make([]map[uint32]string, 0),
		revision:                 1,
	}

	// Read slide dimensions and preload media from the zip package
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		if fi, err := f.Stat(); err == nil {
			if zr, err := zip.NewReader(f, fi.Size()); err == nil {
				doc.inspectZipPackage(zr)
				doc.loadPPTXCommentState(zr)
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
		pres:                     pres,
		widthPT:                  defaultSlideWidthPT,
		heightPT:                 defaultSlideHeightPT,
		mediaImages:              make(map[string]image.Image),
		mediaBytes:               make(map[string][]byte),
		presentationComments:     newPPTXCommentState(),
		slideLayoutChanges:       make(map[uint32]string),
		slideAnimationChanges:    make(map[uint32][]byte),
		slideBodyProps:           make(map[int]map[uint32]shapeBodyProps),
		slidePictureTargets:      make(map[int]map[uint32]string),
		slideTransformChanges:    make([]map[uint32]presentationTransformOverride, 0),
		slideStyleChanges:        make([]map[uint32]document.PresentationElementStylePatch, 0),
		slidePictureCropChanges:  make([]map[uint32]document.PresentationCrop, 0),
		slidePictureImageChanges: make([]map[uint32]presentationPictureImageOverride, 0),
		dirty:                    true,
		revision:                 1,
	}

	// Add an initial title slide if layouts exist
	layouts, err := pres.Layouts()
	if err == nil && len(layouts) > 0 {
		slide, err := pres.AddSlide(layouts[0])
		if err == nil && slide != nil {
			// Title: 960 pt width, centered horizontally
			// In EMU: 960 pt * 12700 = 12192000 EMU
			// Title at X: 1.5 in (1371600 EMU), Y: 2.0 in (1828800 EMU), Width: 10.33 in, Height: 1.5 in
			title, err := slide.AddTextBox(gopptx.TextBoxSpec{
				X:      1371600,
				Y:      1828800,
				Width:  9448800,
				Height: 1371600,
				Text:   "",
			})
			if err != nil {
				_ = pres.Close()
				return nil, fmt.Errorf("创建标题占位符失败: %w", err)
			}
			// Subtitle at Y: 3.8 in
			subtitle, err := slide.AddTextBox(gopptx.TextBoxSpec{
				X:      1371600,
				Y:      3474720,
				Width:  9448800,
				Height: 914400,
				Text:   "",
			})
			if err != nil {
				_ = pres.Close()
				return nil, fmt.Errorf("创建副标题占位符失败: %w", err)
			}
			titleID, subtitleID := uint32(title.ID()), uint32(subtitle.ID())
			doc.slidePlaceholderTypes = []map[uint32]string{{titleID: "ctrTitle", subtitleID: "subTitle"}}
			doc.slidePlaceholderChanges = []map[uint32]string{{titleID: "ctrTitle", subtitleID: "subTitle"}}
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

// SetSourcePath restores the canonical save destination without changing content.
func (d *Document) SetSourcePath(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.path = path
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
