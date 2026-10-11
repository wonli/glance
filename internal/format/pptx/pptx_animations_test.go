package pptx

import (
	"bytes"
	"testing"
)

func TestAppendPresentationEntranceAnimationUsesMainSequenceClicks(t *testing.T) {
	var raw []byte
	for index, effect := range []string{"appear", "fade", "fly_in"} {
		var err error
		raw, err = appendPresentationEntranceAnimation(raw, uint32(17+index), effect, 800)
		if err != nil {
			t.Fatalf("append %s animation: %v", effect, err)
		}
	}

	nodes, err := scanStyleXML(raw)
	if err != nil {
		t.Fatalf("scan generated timing: %v", err)
	}
	sequenceCount, clickCount := 0, 0
	var mainSequenceList int = -1
	for index, node := range nodes {
		if node.local == "seq" {
			sequenceCount++
			children := styleNodeChildren(nodes, index)
			if len(children) != 3 || nodes[children[0]].local != "cTn" || nodes[children[1]].local != "prevCondLst" || nodes[children[2]].local != "nextCondLst" {
				t.Fatalf("main sequence children = %v; want mainSeq cTn, previous, next conditions", presentationNodeNames(nodes, children))
			}
			attrs, attrErr := styleXMLAttributes(raw, nodes[children[0]])
			if attrErr != nil || attrs["nodeType"] != "mainSeq" || attrs["dur"] != "indefinite" {
				t.Fatalf("main sequence cTn attrs=%v error=%v", attrs, attrErr)
			}
			mainSequenceList = styleNodeChildren(nodes, children[0])[0]
		}
		if node.local != "cTn" {
			continue
		}
		attrs, attrErr := styleXMLAttributes(raw, node)
		if attrErr != nil || attrs["nodeType"] != "clickEffect" {
			continue
		}
		clickCount++
		if attrs["presetClass"] != "entr" {
			t.Fatalf("click effect attrs=%v", attrs)
		}
	}
	if sequenceCount != 1 || clickCount != 3 {
		t.Fatalf("generated sequence count=%d click effects=%d; want 1 sequence with 3 click effects", sequenceCount, clickCount)
	}
	if mainSequenceList < 0 {
		t.Fatal("generated timing has no main sequence child list")
	}
	events := styleNodeChildren(nodes, mainSequenceList)
	if len(events) != 3 {
		t.Fatalf("main sequence events=%d; want one independent click event per animation", len(events))
	}
	for index, event := range events {
		if !presentationAnimationChildren(nodes, event, "cTn") {
			t.Fatalf("event %d children=%v; want one outer timing node", index+1, presentationNodeNames(nodes, styleNodeChildren(nodes, event)))
		}
		outer := styleNodeChildren(nodes, event)[0]
		if !presentationAnimationChildren(nodes, outer, "stCondLst", "childTnLst") {
			t.Fatalf("event %d outer timing children=%v", index+1, presentationNodeNames(nodes, styleNodeChildren(nodes, outer)))
		}
		outerChildren := styleNodeChildren(nodes, outer)
		if !presentationAnimationWrapperCtn(raw, nodes[outer]) || !presentationAnimationWrapperCondition(raw, nodes, outerChildren[0], "indefinite") {
			t.Fatalf("event %d has an unsupported outer click wrapper", index+1)
		}
		if !presentationAnimationChildren(nodes, outerChildren[1], "par") {
			t.Fatalf("event %d outer child list does not contain one inner wrapper", index+1)
		}
		innerPar := styleNodeChildren(nodes, outerChildren[1])[0]
		if !presentationAnimationChildren(nodes, innerPar, "cTn") {
			t.Fatalf("event %d inner parallel node children=%v", index+1, presentationNodeNames(nodes, styleNodeChildren(nodes, innerPar)))
		}
		inner := styleNodeChildren(nodes, innerPar)[0]
		if !presentationAnimationChildren(nodes, inner, "stCondLst", "childTnLst") {
			t.Fatalf("event %d inner timing children=%v", index+1, presentationNodeNames(nodes, styleNodeChildren(nodes, inner)))
		}
		innerChildren := styleNodeChildren(nodes, inner)
		if !presentationAnimationWrapperCtn(raw, nodes[inner]) || !presentationAnimationWrapperCondition(raw, nodes, innerChildren[0], "0") || !presentationAnimationChildren(nodes, innerChildren[1], "par") {
			t.Fatalf("event %d has an unsupported inner click wrapper", index+1)
		}
		animationPar := styleNodeChildren(nodes, innerChildren[1])[0]
		if !presentationAnimationChildren(nodes, animationPar, "cTn") {
			t.Fatalf("event %d animation children=%v", index+1, presentationNodeNames(nodes, styleNodeChildren(nodes, animationPar)))
		}
		animationAttrs, attrErr := styleXMLAttributes(raw, nodes[styleNodeChildren(nodes, animationPar)[0]])
		if attrErr != nil || animationAttrs["nodeType"] != "clickEffect" {
			t.Fatalf("event %d animation attrs=%v error=%v", index+1, animationAttrs, attrErr)
		}
	}
}

func presentationNodeNames(nodes []styleXMLNode, indexes []int) []string {
	names := make([]string, len(indexes))
	for i, index := range indexes {
		names[i] = nodes[index].local
	}
	return names
}

func TestRemovePresentationAnimationGroupSupportsGlanceEntranceEffects(t *testing.T) {
	tests := []struct {
		name         string
		effect       string
		animationIdx int
		wantRemoved  int
	}{
		{name: "appear visibility set", effect: "appear", animationIdx: 1, wantRemoved: 1},
		{name: "fade effect and visibility set", effect: "fade", animationIdx: 2, wantRemoved: 2},
		{name: "fly-in effect and visibility set", effect: "fly_in", animationIdx: 1, wantRemoved: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := appendPresentationEntranceAnimation(nil, 17, test.effect, 800)
			if err != nil {
				t.Fatalf("append entrance animation: %v", err)
			}
			updated, removed, err := removePresentationAnimationGroup(raw, 17, test.animationIdx)
			if err != nil || removed != test.wantRemoved {
				t.Fatalf("remove animation group: removed=%d error=%v", removed, err)
			}
			if err := validateEditablePresentationTiming(updated); err != nil {
				t.Fatalf("updated timing is invalid: %v", err)
			}
			nodes, err := scanStyleXML(updated)
			if err != nil {
				t.Fatalf("scan updated timing: %v", err)
			}
			for _, node := range nodes {
				if animationKind(node.local) {
					t.Fatalf("animation node %q remains after deletion", node.local)
				}
			}
		})
	}
}

func TestRemovePresentationAnimationGroupSupportsLegacyRootClickEffects(t *testing.T) {
	raw := []byte(`<p:timing><p:tnLst><p:par><p:cTn id="1" dur="indefinite" restart="never" nodeType="tmRoot"><p:childTnLst>`)
	raw = append(raw, presentationEntranceAnimationFragment("p:", 2, 3, 4, 17, "fade", 800)...)
	raw = append(raw, []byte(`</p:childTnLst></p:cTn></p:par></p:tnLst></p:timing>`)...)

	updated, removed, err := removePresentationAnimationGroup(raw, 17, 2)
	if err != nil || removed != 2 {
		t.Fatalf("remove legacy animation group: removed=%d error=%v", removed, err)
	}
	if bytes.Contains(updated, []byte(`nodeType="clickEffect"`)) {
		t.Fatal("legacy clickEffect group remains after deletion")
	}
	if !bytes.Contains(updated, []byte(`nodeType="tmRoot"`)) {
		t.Fatal("legacy root timing was not preserved")
	}
}

func TestRemovePresentationAnimationGroupRejectsWrongTargetAndComplexGroups(t *testing.T) {
	raw, err := appendPresentationEntranceAnimation(nil, 17, "fade", 800)
	if err != nil {
		t.Fatalf("append entrance animation: %v", err)
	}
	original := append([]byte(nil), raw...)
	if _, _, err := removePresentationAnimationGroup(raw, 18, 2); err == nil {
		t.Fatal("animation deletion accepted a different explicit target")
	}
	if !bytes.Equal(raw, original) {
		t.Fatal("failed wrong-target deletion mutated the source timing tree")
	}

	complex := append([]byte(nil), raw...)
	closing := []byte("</p:childTnLst>")
	position := bytes.Index(complex, closing)
	if position < 0 {
		t.Fatal("generated animation has no child timing list")
	}
	extra := []byte(`<p:animRot><p:cBhvr><p:cTn id="90" dur="800"/><p:tgtEl><p:spTgt spid="17"/></p:tgtEl></p:cBhvr></p:animRot>`)
	complex = append(complex[:position], append(extra, complex[position:]...)...)
	if err := validateEditablePresentationTiming(complex); err != nil {
		t.Fatalf("complex fixture XML invalid: %v", err)
	}
	if _, _, err := removePresentationAnimationGroup(complex, 17, 2); err == nil {
		t.Fatal("animation deletion accepted a complex group containing a third animation node")
	}
	if !bytes.Contains(complex, extra) {
		t.Fatal("failed complex-group deletion mutated the source timing tree")
	}

	withUnknownSibling := bytes.Replace(
		raw,
		[]byte("</p:childTnLst></p:cTn></p:par>"),
		[]byte("</p:childTnLst><p:extLst/></p:cTn></p:par>"),
		1,
	)
	if bytes.Equal(withUnknownSibling, raw) {
		t.Fatal("could not add an unsupported sibling to the animation group")
	}
	if _, _, err := removePresentationAnimationGroup(withUnknownSibling, 17, 2); err == nil {
		t.Fatal("animation deletion accepted an unknown timing sibling in the selected group")
	}
}

func TestRemovePresentationAnimationGroupPreservesOtherElements(t *testing.T) {
	raw, err := appendPresentationEntranceAnimation(nil, 17, "fade", 800)
	if err != nil {
		t.Fatalf("append first entrance animation: %v", err)
	}
	raw, err = appendPresentationEntranceAnimation(raw, 18, "fly_in", 1200)
	if err != nil {
		t.Fatalf("append second entrance animation: %v", err)
	}
	updated, removed, err := removePresentationAnimationGroup(raw, 17, 2)
	if err != nil || removed != 2 {
		t.Fatalf("remove first element's animation group: removed=%d error=%v", removed, err)
	}
	nodes, err := scanStyleXML(updated)
	if err != nil {
		t.Fatalf("scan updated timing: %v", err)
	}
	remaining := 0
	for index, node := range nodes {
		if !animationKind(node.local) {
			continue
		}
		targetID, err := presentationAnimationNodeTarget(updated, nodes, index)
		if err != nil || targetID != 18 {
			t.Fatalf("remaining animation node %q targets %d; error=%v, want element 18", node.local, targetID, err)
		}
		remaining++
	}
	if remaining != 2 {
		t.Fatalf("remaining animation XML nodes=%d; want 2 for untouched fly-in group", remaining)
	}
}
