package pptx

import (
	"fmt"
	"glance/internal/document"
	"path/filepath"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

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
		if i < len(d.slideBackgrounds) {
			background := d.slideBackgrounds[i]
			if background.ColorHex != "" {
				pSlide.BgColorHex = background.ColorHex
			}
			pSlide.BackgroundImage = background.Image
		}
		if i < len(d.slideInherited) {
			pSlide.InheritedElements = d.slideInherited[i]
		}

		if notes, err := sl.SpeakerNotesText(); err == nil {
			pSlide.Notes = notes
		}

		// Extract shapes
		if shapes, err := sl.Shapes(); err == nil {
			idCounts := make(map[uint32]int, len(shapes))
			for _, sh := range shapes {
				idCounts[uint32(sh.ID())]++
			}
			for _, sh := range shapes {
				elem := d.projectShape(sh, i)
				if idCounts[uint32(sh.ID())] != 1 {
					elem.CanEditPicture = false
					elem.CanEditTextStyle = false
					elem.CanEditShapeStyle = false
				}
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
	shapeID := uint32(sh.ID())
	if slideIdx < len(d.slideShapeTransforms) {
		if metadata, ok := d.slideShapeTransforms[slideIdx][shapeID]; ok {
			elem.RotationDeg = metadata.RotationDeg
			elem.AspectRatioLocked = metadata.AspectRatioLocked
		}
	}
	if slideIdx < len(d.slidePlaceholderXfrms) {
		if metadata, ok := d.slidePlaceholderXfrms[slideIdx][shapeID]; ok {
			elem.Bounds = metadata.Bounds
			if slideIdx >= len(d.slideShapeTransforms) || !d.slideShapeTransforms[slideIdx][shapeID].HasXfrm {
				elem.RotationDeg = metadata.RotationDeg
				if metadata.HasAspectRatioLock {
					elem.AspectRatioLocked = metadata.AspectRatioLocked
				}
			}
		}
	}
	if slideIdx < len(d.slidePlaceholderBounds) {
		if bounds, ok := d.slidePlaceholderBounds[slideIdx][shapeID]; ok {
			elem.Bounds = bounds
		}
	}
	if slideIdx < len(d.slidePlaceholderTypes) {
		elem.PlaceholderType = d.slidePlaceholderTypes[slideIdx][shapeID]
	}
	if slideIdx < len(d.slideTransformChanges) {
		if override, ok := d.slideTransformChanges[slideIdx][shapeID]; ok {
			elem.Bounds = override.Bounds
			elem.RotationDeg = override.RotationDeg
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
		elem.CanEditShapeStyle = true
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
						pPara.LineSpacing = projectPresentationSpacing(props.LineSpacing)
						pPara.SpaceBefore = projectPresentationSpacing(props.SpaceBefore)
						pPara.SpaceAfter = projectPresentationSpacing(props.SpaceAfter)
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
								pRun.Underline = advanced.Underline != "" && advanced.Underline != "none"
							}
							if pRun.SizePT <= 0 {
								pRun.SizePT = placeholderDefaultFontSize(elem.PlaceholderType)
							}
							if pRun.ColorHex == "" {
								pRun.ColorHex = "#1F2937"
							}
							if pRun.Text != "" {
								pPara.Runs = append(pPara.Runs, pRun)
							}
						}
					}
					if len(pPara.Runs) > 0 || pPara.LineSpacing != nil || pPara.SpaceBefore != nil || pPara.SpaceAfter != nil {
						elem.Paragraphs = append(elem.Paragraphs, pPara)
					}
				}
			}
		}
		if len(elem.Paragraphs) > 0 {
			elem.Kind = document.ElementKindText
		}
		if isEditableTextPlaceholderType(elem.PlaceholderType) {
			elem.Kind = document.ElementKindText
		}
		elem.CanEditTextStyle = len(elem.Paragraphs) > 0 && validateStyleTextTarget(s) == nil
	case *gopptx.PictureShape:
		elem.Kind = document.ElementKindPicture
		elem.AltText = s.AltText()
		if targets := d.slidePictureTargets[slideIdx]; targets != nil {
			if target, ok := targets[uint32(s.ID())]; ok {
				elem.Image = d.mediaImages[target]
				elem.ImageBytes = append([]byte(nil), d.mediaBytes[target]...)
			}
		}
		id := uint32(s.ID())
		if slideIdx < len(d.slidePictureCropStates) {
			if state, ok := d.slidePictureCropStates[slideIdx][id]; ok {
				elem.Crop = state.Crop
			}
		}
		if slideIdx < len(d.slidePictureCropChanges) {
			if crop, ok := d.slidePictureCropChanges[slideIdx][id]; ok {
				elem.Crop = crop
			}
		}
		if slideIdx < len(d.slidePictureImageChanges) {
			if override, ok := d.slidePictureImageChanges[slideIdx][id]; ok {
				elem.ImageBytes = append([]byte(nil), override.Data...)
				elem.Image = override.Image
			}
		}
		elem.CanEditPicture = d.pictureCanEditLocked(slideIdx, id)
	}
	if slideIdx < len(d.slideStyleChanges) {
		if patch, ok := d.slideStyleChanges[slideIdx][shapeID]; ok {
			applyPresentationElementStyle(&elem, patch)
		}
	}

	return elem
}

func projectPresentationSpacing(spacing *gopptx.Spacing) *document.PresentationSpacing {
	if spacing == nil {
		return nil
	}
	value := float64(spacing.Value)
	switch spacing.Kind {
	case "pct":
		value /= 100000
	case "pts":
		value /= 100
	default:
		return nil
	}
	return &document.PresentationSpacing{Kind: spacing.Kind, Value: value}
}

func placeholderDefaultFontSize(placeholderType string) float64 {
	switch placeholderType {
	case "title", "ctrTitle":
		return 44
	case "subTitle":
		return 32
	case "body", "obj", "content":
		return 24
	default:
		return 18
	}
}

func isEditableTextPlaceholderType(placeholderType string) bool {
	switch placeholderType {
	case "title", "ctrTitle", "subTitle", "body", "obj", "content", "vert", "vert270", "vertTitle", "vertBody":
		return true
	default:
		return false
	}
}
