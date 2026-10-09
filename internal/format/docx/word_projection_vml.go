package docx

import (
	"math"
	"strconv"
	"strings"

	"glance/internal/document"
)

func (p *wordProjector) vmlImageInline(pict *wordXMLNode, id string) document.WordInline {
	shape := findDescendant(pict, "shape")
	imageData := findDescendant(pict, "imagedata")
	if imageData == nil {
		return unsupportedInline(id, "VML shape has no image reference")
	}
	width := wordVMLDimensionEMU(shape, "width")
	height := wordVMLDimensionEMU(shape, "height")
	preview := &document.WordImage{ID: id, WidthEMU: width, HeightEMU: height}
	relationshipID := attrValue(imageData, "id")
	if relationshipID == "" {
		fallback := unsupportedInline(id, "VML image has no relationship reference")
		fallback.Unsupported.ImagePreview = preview
		return fallback
	}
	relationship, ok := p.relationships[relationshipID]
	if !ok || relationship.external || !relationship.image {
		fallback := unsupportedInline(id, "VML image relationship "+relationshipID+" is missing, external, or not an image")
		fallback.Unsupported.ImagePreview = preview
		return fallback
	}
	partIndex, exists := p.partIndexes[relationship.target]
	if !exists {
		fallback := unsupportedInline(id, "VML image media part is missing: "+relationship.target)
		fallback.Unsupported.ImagePreview = preview
		return fallback
	}
	mediaType := p.contentTypes.mediaType(relationship.target)
	if !supportedWordImageMedia(mediaType) {
		fallback := unsupportedInline(id, "Unsupported VML image media type: "+mediaType)
		fallback.Unsupported.ImagePreview = preview
		return fallback
	}
	p.imageParts[id] = partIndex
	floating, supportsFloating := wordVMLFloatingGeometry(shape, pict)
	image := &document.WordImage{
		ID: id, MediaType: mediaType, WidthEMU: width, HeightEMU: height,
		Resolved: true, InlineFallback: !supportsFloating,
	}
	if supportsFloating {
		image.Floating = floating
	}
	return document.WordInline{Kind: document.WordImageInline, Image: image}
}

func wordVMLDimensionEMU(shape *wordXMLNode, property string) int64 {
	value, ok := wordVMLStyleLengthEMU(shape, property, false)
	if !ok || value <= 0 {
		return 0
	}
	return value
}

func wordVMLFloatingGeometry(shape, pict *wordXMLNode) (*document.WordImageFloating, bool) {
	if shape == nil || pict == nil || wordVMLDimensionEMU(shape, "width") <= 0 || wordVMLDimensionEMU(shape, "height") <= 0 {
		return nil, false
	}
	style := wordVMLStyleProperties(shape)
	if !strings.EqualFold(style["position"], "absolute") {
		return nil, false
	}
	wrap := findDescendant(pict, "wrap")
	wrapType := strings.ToLower(strings.TrimSpace(attrValue(wrap, "type")))
	if wrapType == "" {
		wrapType = "none"
	}
	if wrapType != "tight" {
		return nil, false
	}
	wrapSide := strings.ToLower(strings.TrimSpace(attrValue(wrap, "side")))
	if wrapSide == "" {
		wrapSide = "both"
	}
	if wrapSide != "both" {
		return nil, false
	}
	anchorX := strings.ToLower(strings.TrimSpace(attrValue(wrap, "anchorx")))
	anchorY := strings.ToLower(strings.TrimSpace(attrValue(wrap, "anchory")))
	if anchorX == "" {
		anchorX = "text"
	}
	if anchorY == "" {
		anchorY = "text"
	}
	if anchorX != "text" || anchorY != "text" {
		return nil, false
	}
	xOffset, xOK := wordVMLStyleLengthEMU(shape, "margin-left", true)
	yOffset, yOK := wordVMLStyleLengthEMU(shape, "margin-top", true)
	if !xOK || !yOK {
		return nil, false
	}
	shapeType := findDescendant(pict, "shapetype")
	coordSize := attrValue(shape, "coordsize")
	if coordSize == "" {
		coordSize = attrValue(shapeType, "coordsize")
	}
	coordWidth, coordHeight, ok := parseVMLCoordinatePair(coordSize)
	if !ok {
		return nil, false
	}
	polygon, ok := parseVMLWrapPolygon(attrValue(shape, "wrapcoords"), coordWidth, coordHeight)
	if !ok {
		return nil, false
	}
	leftDistance, leftOK := wordVMLWrapDistanceEMU(style, "mso-wrap-distance-left")
	rightDistance, rightOK := wordVMLWrapDistanceEMU(style, "mso-wrap-distance-right")
	if !leftOK || !rightOK {
		return nil, false
	}
	return &document.WordImageFloating{
		XOffsetEMU: xOffset, YOffsetEMU: yOffset, WrapType: wrapType,
		WrapDistanceLeftEMU: leftDistance, WrapDistanceRightEMU: rightDistance,
		WrapPolygon: polygon,
	}, true
}

func wordVMLWrapDistanceEMU(style map[string]string, property string) (int64, bool) {
	value, exists := style[property]
	if !exists {
		// MS-OI29500 records Word's 9 pt default for left/right distances.
		return 9 * 12700, true
	}
	return parseVMLLengthEMU(value, false)
}

func wordVMLStyleProperties(shape *wordXMLNode) map[string]string {
	properties := make(map[string]string)
	if shape == nil {
		return properties
	}
	for _, item := range strings.Split(attrValue(shape, "style"), ";") {
		name, value, ok := strings.Cut(item, ":")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			properties[name] = strings.TrimSpace(value)
		}
	}
	return properties
}

func wordVMLStyleLengthEMU(shape *wordXMLNode, property string, allowNegative bool) (int64, bool) {
	value, exists := wordVMLStyleProperties(shape)[property]
	if !exists {
		return 0, false
	}
	return parseVMLLengthEMU(value, allowNegative)
}

func parseVMLLengthEMU(value string, allowNegative bool) (int64, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	units := []struct {
		suffix string
		factor float64
	}{{"pt", 12700}, {"pc", 152400}, {"in", 914400}, {"cm", 360000}, {"mm", 36000}, {"px", 9525}}
	factor := 12700.0
	for _, unit := range units {
		if strings.HasSuffix(value, unit.suffix) {
			value = strings.TrimSpace(strings.TrimSuffix(value, unit.suffix))
			factor = unit.factor
			break
		}
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || (!allowNegative && number < 0) || math.Abs(number) >= 1e12 {
		return 0, false
	}
	emu := number * factor
	if math.IsNaN(emu) || math.IsInf(emu, 0) || math.Abs(emu) >= float64(1<<63-1) {
		return 0, false
	}
	return int64(math.Round(emu)), true
}

func parseVMLCoordinatePair(value string) (float64, float64, bool) {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\r' || r == '\n' })
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, errX := strconv.ParseFloat(parts[0], 64)
	height, errY := strconv.ParseFloat(parts[1], 64)
	if errX != nil || errY != nil || !finitePositiveFloat(width) || !finitePositiveFloat(height) {
		return 0, 0, false
	}
	return width, height, true
}

func parseVMLWrapPolygon(value string, coordWidth, coordHeight float64) ([]document.WordImageWrapPoint, bool) {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\r' || r == '\n' })
	if len(parts) < 6 || len(parts)%2 != 0 || len(parts) > 512 {
		return nil, false
	}
	points := make([]document.WordImageWrapPoint, 0, len(parts)/2)
	for index := 0; index < len(parts); index += 2 {
		x, errX := strconv.ParseFloat(parts[index], 64)
		y, errY := strconv.ParseFloat(parts[index+1], 64)
		if errX != nil || errY != nil || math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) || math.Abs(x) > 1e9 || math.Abs(y) > 1e9 {
			return nil, false
		}
		point := document.WordImageWrapPoint{X: x / coordWidth, Y: y / coordHeight}
		if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) {
			return nil, false
		}
		points = append(points, point)
	}
	area := 0.0
	for index, point := range points {
		next := points[(index+1)%len(points)]
		area += point.X*next.Y - next.X*point.Y
	}
	if math.Abs(area) <= 1e-12 {
		return nil, false
	}
	return points, true
}

func finitePositiveFloat(value float64) bool {
	return value > 0 && value < 1e12
}
