package pptx

import (
	"encoding/xml"
	"fmt"
	"glance/internal/document"
	"math"
	"strconv"
	"strings"
)

const drawingMLNamespace = "http://schemas.openxmlformats.org/drawingml/2006/main"

// presentationTransformOverride is an adapter-owned edit layered over the
// parsed presentation. Keeping it separate lets snapshots expose committed
// geometry without mutating the third-party object model.
type presentationTransformOverride struct {
	Bounds      document.PresentationRect
	RotationDeg float64
}

// presentationTransformMetadata retains attributes that matter when a
// placeholder has to materialize a local a:xfrm.
type presentationTransformMetadata struct {
	Bounds             document.PresentationRect
	RotationDeg        float64
	FlipH              bool
	FlipV              bool
	HasXfrm            bool
	AspectRatioLocked  bool
	HasAspectRatioLock bool
}

func validatePresentationTransform(bounds document.PresentationRect, rotationDeg float64) error {
	values := []float64{bounds.X, bounds.Y, bounds.W, bounds.H, rotationDeg}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("对象变换包含非有限数值")
		}
	}
	if bounds.W < 8 || bounds.H < 8 {
		return fmt.Errorf("对象宽高不得小于 8 pt")
	}
	const maxEMU = float64(math.MaxInt64) / 2
	for _, value := range []float64{bounds.X, bounds.Y, bounds.W, bounds.H} {
		if math.Abs(value*emuPerPoint) > maxEMU-1 {
			return fmt.Errorf("对象几何超出 PPTX 范围")
		}
	}
	return nil
}

func normalizePresentationRotation(degrees float64) float64 {
	degrees = math.Mod(degrees, 360)
	if degrees < 0 {
		degrees += 360
	}
	if degrees == 360 || degrees == 0 {
		return 0
	}
	return degrees
}

func pointToEMU(points float64) int64 {
	return int64(math.Round(points * emuPerPoint))
}

func degreesToDrawingML(degrees float64) int64 {
	return int64(math.Round(normalizePresentationRotation(degrees) * 60000))
}

func shapeTransformMetadataFromXML(shape pptxXMLNode) presentationTransformMetadata {
	metadata := presentationTransformMetadata{}
	properties := shape.child("spPr")
	if properties != nil {
		if xfrm := properties.child("xfrm"); xfrm != nil {
			metadata.HasXfrm = true
			metadata.RotationDeg = rotationDegreesFromXfrm(*xfrm)
			metadata.FlipH = xmlBoolDefaultFalse(xfrm.attr("flipH"))
			metadata.FlipV = xmlBoolDefaultFalse(xfrm.attr("flipV"))
		}
	}
	var nonVisual *pptxXMLNode
	if nv := shape.child("nvSpPr"); nv != nil {
		nonVisual = nv.child("cNvSpPr")
	} else if nv := shape.child("nvPicPr"); nv != nil {
		nonVisual = nv.child("cNvPicPr")
	}
	if nonVisual != nil {
		locks := nonVisual.child("spLocks")
		if locks == nil {
			locks = nonVisual.child("picLocks")
		}
		if locks != nil {
			if value, exists := xmlOnOff(locks.attr("noChangeAspect")); exists {
				metadata.HasAspectRatioLock = true
				metadata.AspectRatioLocked = value
			}
		}
	}
	return metadata
}

func rotationDegreesFromXfrm(xfrm pptxXMLNode) float64 {
	value := xfrm.attr("rot")
	if value == "" {
		return 0
	}
	units, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return normalizePresentationRotation(float64(units) / 60000)
}

func xmlOnOff(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "on":
		return true, true
	case "0", "false", "off":
		return false, true
	default:
		return false, false
	}
}

func xmlBoolDefaultFalse(value string) bool {
	result, _ := xmlOnOff(value)
	return result
}

// parseSlideTransformMetadata projects slide-local rotation and explicit
// noChangeAspect metadata by non-visual shape ID.
func parseSlideTransformMetadata(data []byte) map[uint32]presentationTransformMetadata {
	var root pptxXMLNode
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil
	}
	tree := shapeTree(root)
	if tree == nil {
		return nil
	}
	result := make(map[uint32]presentationTransformMetadata)
	for _, shape := range tree.Children {
		if shape.XMLName.Local != "sp" && shape.XMLName.Local != "pic" {
			continue
		}
		id := shapeIDFromXML(shape)
		if id == 0 {
			continue
		}
		result[id] = shapeTransformMetadataFromXML(shape)
	}
	return result
}

func samePresentationRect(actual, expected document.PresentationRect) bool {
	const tolerance = 0.5 / emuPerPoint
	return math.Abs(actual.X-expected.X) <= tolerance && math.Abs(actual.Y-expected.Y) <= tolerance &&
		math.Abs(actual.W-expected.W) <= tolerance && math.Abs(actual.H-expected.H) <= tolerance
}
