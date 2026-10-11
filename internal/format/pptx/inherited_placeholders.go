package pptx

import (
	"strconv"

	"glance/internal/document"
)

// placeholderBounds returns geometry for slide placeholders that do not have
// their own transform. Matches use normalized type/index pairs and require a
// unique candidate at each inherited level.
func (p *pptxPackage) placeholderBounds(slidePart, layoutPart, masterPart string) map[uint32]document.PresentationRect {
	slideRoot, ok := p.xmlPart(slidePart)
	if !ok {
		return nil
	}
	slideTree := shapeTree(slideRoot)
	if slideTree == nil {
		return nil
	}
	layoutRoot, _ := p.xmlPart(layoutPart)
	masterRoot, _ := p.xmlPart(masterPart)
	layoutTree, masterTree := shapeTree(layoutRoot), shapeTree(masterRoot)
	result := make(map[uint32]document.PresentationRect)
	for _, shape := range slideTree.Children {
		if shape.XMLName.Local != "sp" {
			continue
		}
		record, ok := placeholderRecordForShape(shape)
		if !ok || record.id == 0 || record.hasXfrm {
			continue
		}
		bounds, matched, ambiguous := uniquePlaceholderBounds(layoutTree, record.key)
		if ambiguous {
			continue
		}
		if matched && validPresentationRect(bounds) {
			result[record.id] = bounds
			continue
		}
		bounds, matched, ambiguous = uniquePlaceholderBounds(masterTree, record.key)
		if !ambiguous && matched && validPresentationRect(bounds) {
			result[record.id] = bounds
		}
	}
	return result
}

// placeholderTransforms resolves the effective inherited transform for each
// slide-local placeholder that has no transform of its own. These values are
// used for editing previews and to materialize a slide-local transform on
// save without changing its layout or master.
func (p *pptxPackage) placeholderTransforms(slidePart, layoutPart, masterPart string) map[uint32]presentationTransformMetadata {
	slideRoot, ok := p.xmlPart(slidePart)
	if !ok {
		return nil
	}
	slideTree := shapeTree(slideRoot)
	if slideTree == nil {
		return nil
	}
	layoutRoot, _ := p.xmlPart(layoutPart)
	masterRoot, _ := p.xmlPart(masterPart)
	layoutTree, masterTree := shapeTree(layoutRoot), shapeTree(masterRoot)
	result := make(map[uint32]presentationTransformMetadata)
	for _, shape := range slideTree.Children {
		if shape.XMLName.Local != "sp" {
			continue
		}
		slideRecord, found := placeholderRecordForShape(shape)
		if !found || slideRecord.id == 0 || slideRecord.hasXfrm {
			continue
		}
		inherited, matched, ambiguous := uniquePlaceholderRecord(layoutTree, slideRecord.key)
		if ambiguous {
			continue
		}
		if !matched || !inherited.hasXfrm || !validPresentationRect(inherited.bounds) {
			inherited, matched, ambiguous = uniquePlaceholderRecord(masterTree, slideRecord.key)
		}
		if ambiguous || !matched || !inherited.hasXfrm || !validPresentationRect(inherited.bounds) {
			continue
		}
		metadata := inherited.transform
		metadata.Bounds = inherited.bounds
		metadata.HasXfrm = true
		result[slideRecord.id] = metadata
	}
	return result
}

// placeholderTypes returns the role of each slide-local placeholder keyed by
// its non-visual shape ID. The layout/master versions supply appearance and
// geometry, but only slide-local placeholders are editable.
func (p *pptxPackage) placeholderTypes(slidePart string) map[uint32]string {
	root, ok := p.xmlPart(slidePart)
	if !ok {
		return nil
	}
	tree := shapeTree(root)
	if tree == nil {
		return nil
	}
	result := make(map[uint32]string)
	for _, shape := range tree.Children {
		if shape.XMLName.Local != "sp" {
			continue
		}
		record, ok := placeholderRecordForShape(shape)
		if !ok || record.id == 0 {
			continue
		}
		result[record.id] = record.key.typeName
	}
	return result
}

func uniquePlaceholderBounds(tree *pptxXMLNode, key placeholderKey) (document.PresentationRect, bool, bool) {
	record, found, ambiguous := uniquePlaceholderRecord(tree, key)
	if !found || ambiguous || !record.hasXfrm {
		return document.PresentationRect{}, false, ambiguous
	}
	return record.bounds, true, false
}

func uniquePlaceholderRecord(tree *pptxXMLNode, key placeholderKey) (placeholderRecord, bool, bool) {
	if tree == nil {
		return placeholderRecord{}, false, false
	}
	var match *placeholderRecord
	count := 0
	for _, shape := range tree.Children {
		if shape.XMLName.Local != "sp" {
			continue
		}
		record, ok := placeholderRecordForShape(shape)
		if !ok || record.key != key {
			continue
		}
		count++
		if count == 1 {
			recordCopy := record
			match = &recordCopy
		}
	}
	if count > 1 {
		return placeholderRecord{}, false, true
	}
	if match == nil {
		return placeholderRecord{}, false, false
	}
	return *match, true, false
}

func placeholderRecordForShape(shape pptxXMLNode) (placeholderRecord, bool) {
	nv := shape.child("nvSpPr")
	if nv == nil {
		return placeholderRecord{}, false
	}
	nvPr := nv.child("nvPr")
	if nvPr == nil {
		return placeholderRecord{}, false
	}
	ph := nvPr.child("ph")
	if ph == nil {
		return placeholderRecord{}, false
	}
	cNvPr := nv.child("cNvPr")
	if cNvPr == nil {
		return placeholderRecord{}, false
	}
	id, err := strconv.ParseUint(cNvPr.attr("id"), 10, 32)
	if err != nil {
		return placeholderRecord{}, false
	}
	key := placeholderKey{typeName: ph.attr("type"), index: 0}
	if key.typeName == "" {
		key.typeName = "obj"
	}
	if idx := ph.attr("idx"); idx != "" {
		index, parseErr := strconv.ParseUint(idx, 10, 32)
		if parseErr != nil {
			return placeholderRecord{}, false
		}
		key.index = uint32(index)
	}
	bounds, hasXfrm := shapeBounds(shape)
	transform := shapeTransformMetadataFromXML(shape)
	transform.Bounds = bounds
	transform.HasXfrm = hasXfrm
	return placeholderRecord{id: uint32(id), key: key, bounds: bounds, hasXfrm: hasXfrm, transform: transform}, true
}

func shapeIDFromXML(shape pptxXMLNode) uint32 {
	nv := shape.child("nvSpPr")
	if nv == nil {
		nv = shape.child("nvPicPr")
	}
	if nv == nil {
		nv = shape.child("nvGraphicFramePr")
	}
	if nv == nil {
		nv = shape.child("nvCxnSpPr")
	}
	if nv == nil {
		nv = shape.child("nvGrpSpPr")
	}
	if nv == nil {
		return 0
	}
	cNvPr := nv.child("cNvPr")
	if cNvPr == nil {
		return 0
	}
	id, err := strconv.ParseUint(cNvPr.attr("id"), 10, 32)
	if err != nil {
		return 0
	}
	return uint32(id)
}
