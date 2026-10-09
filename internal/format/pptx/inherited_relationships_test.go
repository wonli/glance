package pptx

import (
	"fmt"

	"image/color"

	"testing"
)

func TestResolveOPCTargetNormalizesOwnerRelativeAndRootRelativePaths(t *testing.T) {
	rels := parsePresentationRels([]byte(`<Relationships xmlns="` + testPackageRelNS + `">` +
		`<Relationship Id="rId1" Target="../ppt/slides/slide17.xml"/>` +
		`<Relationship Id="rId2" Target="/ppt/slides/slide2.xml"/>` +
		`<Relationship Id="rId3" Target="../../outside/slide3.xml"/>` +
		`</Relationships>`))
	presentation := []byte(`<p:presentation xmlns:r="` + testOfficeRelNS + `"><p:sldIdLst>` +
		`<p:sldId id="1" r:id="rId1"/><p:sldId id="2" r:id="rId2"/><p:sldId id="3" r:id="rId3"/>` +
		`</p:sldIdLst></p:presentation>`)

	got := parsePresentationSlideFiles(presentation, rels)
	want := []string{"ppt/slides/slide17.xml", "ppt/slides/slide2.xml"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("normalized slide targets = %v, want %v", got, want)
	}
}

func TestOPCPackageRejectsExternalImageRelationships(t *testing.T) {
	parts := pictureFixtureParts(
		[]testPictureRef{{id: 2, rid: "rId2"}},
		[]testImageRel{{id: "rId2", target: "../media/unrelated.png", external: true}},
		map[string][]byte{"ppt/media/unrelated.png": encodeTestRaster(t, "png", color.NRGBA{B: 255, A: 255})},
	)
	if img := firstPictureImage(t, parts); img != nil {
		t.Fatal("external relationship unexpectedly borrowed a local media image")
	}
}

func TestPPTXPackagePreservesPresentationSlideRelationshipOrder(t *testing.T) {
	rels := parsePresentationRels([]byte(`<Relationships xmlns="` + testPackageRelNS + `">` +
		`<Relationship Id="rIdZ" Target="slides/slide17.xml"/>` +
		`<Relationship Id="rIdA" Target="slides/slide2.xml"/>` +
		`</Relationships>`))
	presentation := []byte(`<p:presentation xmlns:r="` + testOfficeRelNS + `"><p:sldIdLst>` +
		`<p:sldId id="1" r:id="rIdZ"/><p:sldId id="2" r:id="rIdA"/>` +
		`</p:sldIdLst></p:presentation>`)

	got := parsePresentationSlideFiles(presentation, rels)
	want := []string{"ppt/slides/slide17.xml", "ppt/slides/slide2.xml"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("slide parts = %v, want presentation order %v", got, want)
	}
}

func TestPPTXMissingOrMalformedRelationshipsAreSafe(t *testing.T) {
	for name, data := range map[string][]byte{
		"missing":   nil,
		"malformed": []byte(`<Relationships><Relationship Id="rId1" Target="../media/image.png"`),
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("relationship parsing panicked: %v", recovered)
				}
			}()
			rels := parsePresentationRels(data)
			if got := parsePresentationSlideFiles([]byte(`<p:presentation><p:sldIdLst><p:sldId id="1" r:id="rId1"/></p:sldIdLst></p:presentation>`), rels); len(got) != 0 {
				t.Fatalf("malformed relationship resolved slide parts %v", got)
			}
		})
	}
}
