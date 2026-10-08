package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
)

const (
	pptxPackageRelationshipNS = "http://schemas.openxmlformats.org/package/2006/relationships"
	pptxOfficeRelationshipNS  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	pptxSlideRelationshipType = pptxOfficeRelationshipNS + "/slide"
	pptxImageRelationshipType = pptxOfficeRelationshipNS + "/image"
)

type opcRelationship struct {
	ID, Type, Target string
	External         bool
}

type pptxPackage struct {
	parts             map[string]*zip.File
	rels              map[string]map[string]opcRelationship
	relationshipOrder map[string][]string
	slideParts        []string
	layoutParts       []string
	layoutMasterParts map[string]string
	imageCache        map[string]image.Image
}

func newPPTXPackage(zr *zip.Reader) *pptxPackage {
	pkg := &pptxPackage{
		parts:             make(map[string]*zip.File, len(zr.File)),
		rels:              make(map[string]map[string]opcRelationship),
		relationshipOrder: make(map[string][]string),
		layoutMasterParts: make(map[string]string),
	}
	for _, part := range zr.File {
		name := strings.TrimPrefix(part.Name, "/")
		pkg.parts[name] = part
	}
	for name, part := range pkg.parts {
		ownerPart := ownerPartForRelationships(name)
		if ownerPart == "" {
			continue
		}
		data, err := readOPCPart(part)
		if err != nil {
			continue
		}
		pkg.rels[ownerPart], pkg.relationshipOrder[ownerPart] = parseOPCRelationshipRecords(data)
	}
	if presentationFile, ok := pkg.parts["ppt/presentation.xml"]; ok {
		if data, err := readOPCPart(presentationFile); err == nil {
			for _, relID := range presentationSlideRelationshipIDs(data) {
				rel, ok := pkg.relationship("ppt/presentation.xml", relID)
				if !ok || rel.External || rel.Type != pptxSlideRelationshipType {
					continue
				}
				partPath, ok := resolveOPCTarget("ppt/presentation.xml", rel.Target)
				if !ok {
					continue
				}
				if _, exists := pkg.parts[partPath]; exists {
					pkg.slideParts = append(pkg.slideParts, partPath)
				}
			}
		}
	}
	pkg.collectLayoutParts()
	return pkg
}

// collectLayoutParts follows the same master relationship and layout
// relationship order used by go-pptx's Presentation.Layouts API.
func (p *pptxPackage) collectLayoutParts() {
	const presentationPart = "ppt/presentation.xml"
	seenMasters := make(map[string]bool)
	seenLayouts := make(map[string]bool)
	for _, masterRelID := range p.orderedRelationshipIDs(presentationPart) {
		rel, ok := p.relationship(presentationPart, masterRelID)
		if !ok || rel.External || rel.Type != pptxSlideMasterRelationshipType {
			continue
		}
		masterPart, ok := resolveOPCTarget(presentationPart, rel.Target)
		if !ok || p.parts[masterPart] == nil || seenMasters[masterPart] {
			continue
		}
		seenMasters[masterPart] = true
		for _, layoutRelID := range p.orderedRelationshipIDs(masterPart) {
			layoutRel, ok := p.relationship(masterPart, layoutRelID)
			if !ok || layoutRel.External || layoutRel.Type != pptxSlideLayoutRelationshipType {
				continue
			}
			layoutPart, ok := resolveOPCTarget(masterPart, layoutRel.Target)
			if !ok || p.parts[layoutPart] == nil || seenLayouts[layoutPart] {
				continue
			}
			seenLayouts[layoutPart] = true
			p.layoutParts = append(p.layoutParts, layoutPart)
			p.layoutMasterParts[layoutPart] = masterPart
		}
	}
}

func parseOPCRelationships(data []byte) map[string]opcRelationship {
	rels, _ := parseOPCRelationshipRecords(data)
	return rels
}

func parseOPCRelationshipRecords(data []byte) (map[string]opcRelationship, []string) {
	rels := make(map[string]opcRelationship)
	var order []string
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			continue
		}
		rel := opcRelationship{}
		var targetMode string
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "Id":
				rel.ID = attr.Value
			case "Type":
				rel.Type = attr.Value
			case "Target":
				rel.Target = attr.Value
			case "TargetMode":
				targetMode = attr.Value
			}
		}
		if rel.ID == "" || rel.Target == "" {
			continue
		}
		rel.External = strings.EqualFold(targetMode, "External")
		rels[rel.ID] = rel
		order = append(order, rel.ID)
	}
	return rels, order
}

func relationshipPartPath(ownerPart string) string {
	ownerPart = strings.TrimPrefix(ownerPart, "/")
	dir, base := path.Split(ownerPart)
	return path.Join(dir, "_rels", base+".rels")
}

func ownerPartForRelationships(relsPart string) string {
	relsPart = strings.TrimPrefix(relsPart, "/")
	marker := "/_rels/"
	index := strings.LastIndex(relsPart, marker)
	if index < 0 || !strings.HasSuffix(relsPart, ".rels") {
		return ""
	}
	dir := relsPart[:index]
	base := strings.TrimSuffix(relsPart[index+len(marker):], ".rels")
	if dir == "" || base == "" {
		return ""
	}
	return path.Join(dir, base)
}

func resolveOPCTarget(ownerPart, target string) (string, bool) {
	if ownerPart == "" || target == "" || strings.ContainsAny(target, "\\\x00") {
		return "", false
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	partTarget := parsed.Path
	if partTarget == "" {
		return "", false
	}
	if strings.HasPrefix(partTarget, "/") {
		partTarget = strings.TrimPrefix(partTarget, "/")
	} else {
		partTarget = path.Join(path.Dir(strings.TrimPrefix(ownerPart, "/")), partTarget)
	}
	partTarget = path.Clean(partTarget)
	if partTarget == "." || partTarget == ".." || strings.HasPrefix(partTarget, "../") {
		return "", false
	}
	return partTarget, true
}

func (p *pptxPackage) relationship(ownerPart, relID string) (opcRelationship, bool) {
	rel, ok := p.rels[strings.TrimPrefix(ownerPart, "/")][relID]
	return rel, ok
}

func (p *pptxPackage) relatedPart(ownerPart, relationshipType string) (string, bool) {
	ownerPart = strings.TrimPrefix(ownerPart, "/")
	for _, id := range p.orderedRelationshipIDs(ownerPart) {
		rel, ok := p.relationship(ownerPart, id)
		if !ok || rel.External || rel.Type != relationshipType {
			continue
		}
		target, ok := resolveOPCTarget(ownerPart, rel.Target)
		if !ok {
			continue
		}
		if _, exists := p.parts[target]; exists {
			return target, true
		}
	}
	return "", false
}

func (p *pptxPackage) orderedRelationshipIDs(ownerPart string) []string {
	ownerPart = strings.TrimPrefix(ownerPart, "/")
	ids := p.relationshipOrder[ownerPart]
	if len(ids) == 0 {
		for id := range p.rels[ownerPart] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}
	return ids
}

func (p *pptxPackage) imagePartForRelationship(ownerPart, relID string) (string, bool) {
	rel, ok := p.relationship(ownerPart, relID)
	if !ok || rel.External || rel.Type != pptxImageRelationshipType {
		return "", false
	}
	target, ok := resolveOPCTarget(ownerPart, rel.Target)
	if !ok {
		return "", false
	}
	if _, exists := p.parts[target]; !exists {
		return "", false
	}
	return target, true
}

func presentationSlideRelationshipIDs(data []byte) []string {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var ids []string
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "sldId" {
			continue
		}
		for _, attr := range start.Attr {
			if attr.Name.Local == "id" && (attr.Name.Space == pptxOfficeRelationshipNS || strings.HasPrefix(attr.Value, "rId")) {
				ids = append(ids, attr.Value)
				break
			}
		}
	}
	return ids
}

func parseSlidePictureRelIDs(data []byte) map[uint32]string {
	result := make(map[uint32]string)
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var currentID uint32
	var currentRelID string
	inPicture := false
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch token := tok.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "pic":
				inPicture = true
				currentID = 0
				currentRelID = ""
			case "cNvPr":
				if inPicture {
					for _, attr := range token.Attr {
						if attr.Name.Local == "id" {
							_, _ = fmt.Sscanf(attr.Value, "%d", &currentID)
							break
						}
					}
				}
			case "blip":
				if inPicture {
					for _, attr := range token.Attr {
						if attr.Name.Local == "embed" {
							currentRelID = attr.Value
							break
						}
					}
				}
			}
		case xml.EndElement:
			if token.Name.Local == "pic" && inPicture {
				if currentID != 0 && currentRelID != "" {
					result[currentID] = currentRelID
				}
				inPicture = false
			}
		}
	}
	return result
}

func readOPCPart(part *zip.File) ([]byte, error) {
	rc, err := part.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func readOPCPartLimited(part *zip.File, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 || part.UncompressedSize64 > uint64(maxBytes) {
		return nil, fmt.Errorf("package part exceeds %d bytes", maxBytes)
	}
	rc, err := part.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("package part exceeds %d bytes", maxBytes)
	}
	return data, nil
}
