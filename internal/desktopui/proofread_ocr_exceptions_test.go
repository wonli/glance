package desktopui

import (
	"image"
	"strings"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/ocr"
)

func TestProofreadingDiffExceptionStateMatchingAndPersistence(t *testing.T) {
	state := newProofreadingDiffExceptionState()

	// 1. Add character exceptions
	if !state.AddCharException('（') {
		t.Fatal("expected '（' to be added")
	}
	if state.AddCharException('（') {
		t.Fatal("expected duplicate '（' to return false")
	}
	if !state.AddCharException('）') {
		t.Fatal("expected '）' to be added")
	}

	// 2. Test matches
	frame1 := proofreadingOCRDiffFrame{
		Kind:  proofreadingOCRDiffReplace,
		AChar: '（',
		BChar: '(',
	}
	frame2 := proofreadingOCRDiffFrame{
		Kind:  proofreadingOCRDiffAOnly,
		AChar: '）',
	}
	frame3 := proofreadingOCRDiffFrame{
		Kind:  proofreadingOCRDiffReplace,
		AChar: '正',
		BChar: '误',
	}

	if !state.Matches(frame1) {
		t.Fatal("expected frame1 containing '（' to match exception")
	}
	if !state.Matches(frame2) {
		t.Fatal("expected frame2 containing '）' to match exception")
	}
	if state.Matches(frame3) {
		t.Fatal("expected frame3 with '正'/'误' not to match exception")
	}

	// 3. Test persistence roundtrip
	rules := state.ToRuleStrings()
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}

	restored := newProofreadingDiffExceptionState()
	restored.LoadFromRuleStrings(rules)
	if !restored.Matches(frame1) || !restored.Matches(frame2) {
		t.Fatal("expected restored rules to match frames 1 and 2")
	}
	if restored.Matches(frame3) {
		t.Fatal("expected restored rules not to match frame 3")
	}
}

func TestProofreadingDiffInspectorApplyExceptionBatchSkips(t *testing.T) {
	a, _, comparison := proofreadingConnectorFixture(1)
	page := a.proofreading.xPages[3]

	// Create 4 diffs:
	// Diff 0: '（' vs '(' (bracket)
	// Diff 1: '错' vs '对' (real diff)
	// Diff 2: '）' vs ')' (bracket)
	// Diff 3: '（' (AOnly bracket)
	comparison.Frames = []proofreadingOCRDiffFrame{
		{Kind: proofreadingOCRDiffReplace, BPosition: page.ContentStart + 1, AChar: '（', BChar: '(', ABlockID: 7, AOffset: 0},
		{Kind: proofreadingOCRDiffReplace, BPosition: page.ContentStart + 5, AChar: '错', BChar: '对', ABlockID: 7, AOffset: 1},
		{Kind: proofreadingOCRDiffReplace, BPosition: page.ContentStart + 10, AChar: '）', BChar: ')', ABlockID: 7, AOffset: 2},
		{Kind: proofreadingOCRDiffAOnly, BPosition: page.ContentStart + 15, AChar: '（', BChar: 0, ABlockID: 7, AOffset: 3},
	}
	pair := proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}
	a.proofreading.ocrStep.comparisons[pair] = comparison
	a.proofreading.ocrStep.diffCursor = len(comparison.Frames)
	a.proofreading.ocrStep.diffRunning = false

	step := a.proofreading.ocrStep
	step.currentConnections = []proofreadingOCRConnection{
		{from: image.Pt(100, 50), to: image.Pt(400, 50), kind: proofreadingOCRDiffReplace, frame: comparison.Frames[0]},
		{from: image.Pt(100, 80), to: image.Pt(400, 80), kind: proofreadingOCRDiffReplace, frame: comparison.Frames[1]},
		{from: image.Pt(100, 110), to: image.Pt(400, 110), kind: proofreadingOCRDiffReplace, frame: comparison.Frames[2]},
		{from: image.Pt(100, 140), to: image.Pt(400, 140), kind: proofreadingOCRDiffAOnly, frame: comparison.Frames[3]},
	}

	// 1. Open inspector on Diff 0 (the '（' bracket diff)
	a.openProofreadingDiffInspector(0)
	if !step.inspector.open {
		t.Fatal("expected inspector to be open")
	}
	if step.inspector.diffIndex != 0 {
		t.Fatalf("expected diffIndex=0, got %d", step.inspector.diffIndex)
	}

	// 2. Click "Set as Exception" on Diff 0
	a.applyCurrentDiffAsException()

	// Verification 1: Diff 0 should be marked resolved with reason exception
	if !step.inspector.resolved[0] || step.inspector.resolvedReasons[0] != proofreadingResolvedException {
		t.Fatalf("expected diff 0 resolved as exception, got %v (%s)", step.inspector.resolved[0], step.inspector.resolvedReasons[0])
	}

	// Verification 2: Diff 3 also has '（', so it should have been batch-skipped automatically!
	if !step.inspector.resolved[3] || step.inspector.resolvedReasons[3] != proofreadingResolvedException {
		t.Fatalf("expected diff 3 (also with '（') to be batch-skipped as exception, got %v (%s)", step.inspector.resolved[3], step.inspector.resolvedReasons[3])
	}

	// Verification 3: Inspector should have automatically advanced to Diff 1 ('错' vs '对', which is NOT an exception)
	if step.inspector.diffIndex != 1 {
		t.Fatalf("expected inspector to advance to diff 1 ('错' vs '对'), got %d", step.inspector.diffIndex)
	}

	// Verification 4: Feedback toast should indicate exception applied
	if !strings.Contains(step.inspector.exceptionToast, "例外") {
		t.Fatalf("expected exceptionToast to mention 例外, got %q", step.inspector.exceptionToast)
	}

	// 3. Now on Diff 1, click "Keep"
	a.keepProofreadingDiffInspector()

	// Should advance to Diff 2 (the '）' bracket diff)
	if step.inspector.diffIndex != 2 {
		t.Fatalf("expected inspector to advance to diff 2, got %d", step.inspector.diffIndex)
	}

	// 4. Set '）' as exception as well
	a.applyCurrentDiffAsException()

	// All 4 diffs should now be resolved!
	// 0: exception, 1: kept, 2: exception, 3: exception
	if !step.inspector.allResolved {
		t.Fatal("expected allResolved to be true after resolving all diffs with exceptions")
	}
}

func TestProofreadingDiffInspectorRemoveExceptionReactivatesDiffs(t *testing.T) {
	a, _, comparison := proofreadingConnectorFixture(1)
	page := a.proofreading.xPages[3]

	comparison.Frames = []proofreadingOCRDiffFrame{
		{Kind: proofreadingOCRDiffReplace, BPosition: page.ContentStart + 1, AChar: '（', BChar: '(', ABlockID: 7, AOffset: 0},
		{Kind: proofreadingOCRDiffReplace, BPosition: page.ContentStart + 5, AChar: '（', BChar: '(', ABlockID: 7, AOffset: 1},
	}
	step := a.proofreading.ocrStep
	step.currentConnections = []proofreadingOCRConnection{
		{from: image.Pt(100, 50), to: image.Pt(400, 50), kind: proofreadingOCRDiffReplace, frame: comparison.Frames[0]},
		{from: image.Pt(100, 80), to: image.Pt(400, 80), kind: proofreadingOCRDiffReplace, frame: comparison.Frames[1]},
	}

	a.openProofreadingDiffInspector(0)
	a.applyCurrentDiffAsException()

	if !step.inspector.allResolved {
		t.Fatal("expected both '（' diffs to be resolved by exception")
	}

	// Now remove the exception rule at index 0
	a.removeProofreadingDiffException(0)

	// Since the rule was removed, the diffs should be restored to unresolved!
	if step.inspector.allResolved {
		t.Fatal("expected allResolved to be false after removing exception rule")
	}
	if step.inspector.resolved[0] || step.inspector.resolved[1] {
		t.Fatalf("expected diffs to be restored, got resolved[0]=%v, resolved[1]=%v", step.inspector.resolved[0], step.inspector.resolved[1])
	}
}

func TestProofreadingExceptionsModalLayout(t *testing.T) {
	a, _, _ := proofreadingConnectorFixture(1)
	step := a.proofreading.ocrStep
	step.inspector.open = true
	step.inspector.exceptionsOpen = true
	step.inspector.exceptions.AddCharException('（')
	step.inspector.exceptions.AddCharException('）')

	var ops op.Ops
	layoutGtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 600))}
	dims := a.layoutProofreadingExceptionsModal(layoutGtx, step)
	if dims.Size.X == 0 || dims.Size.Y == 0 {
		t.Fatal("expected exceptions modal layout to produce non-zero dimensions")
	}
}

func TestProofreadingOCRCharacterDiffPreExcludesExceptions(t *testing.T) {
	// Layout text contains brackets '（' and '）'
	layout := ocr.Layout{
		Blocks: []ocr.LayoutBlock{
			{ID: 1, Label: "text", Text: "本项目（以下简称甲方）负责。"},
		},
	}
	// Markdown text doesn't have brackets
	markdown := "本项目以下简称甲方负责。"

	// Without exceptions: '（' and '）' will be detected as AOnly diffs!
	diffBefore := proofreadingOCRCharacterDiff(layout, markdown, 0)
	if diffBefore.AOnly != 2 {
		t.Fatalf("expected 2 AOnly diffs before excluding brackets, got %d", diffBefore.AOnly)
	}

	// Now pre-exclude '（' and '）' in exception list before scan
	exceptions := map[rune]bool{
		'（': true,
		'）': true,
	}
	diffAfter := proofreadingOCRCharacterDiff(layout, markdown, 0, exceptions)
	// Both brackets are pre-excluded before diff scan -> perfectly matches with 0 diffs!
	if diffAfter.AOnly != 0 {
		t.Fatalf("expected 0 AOnly diffs after pre-excluding brackets in diff scan, got %d", diffAfter.AOnly)
	}
	if diffAfter.BOnly != 0 {
		t.Fatalf("expected 0 BOnly diffs, got %d", diffAfter.BOnly)
	}
}

func TestProofreadingDiffInspectorToggleException(t *testing.T) {
	a, _, comparison := proofreadingConnectorFixture(1)
	page := a.proofreading.xPages[3]

	comparison.Frames = []proofreadingOCRDiffFrame{
		{Kind: proofreadingOCRDiffReplace, BPosition: page.ContentStart + 1, AChar: '（', BChar: '(', ABlockID: 7, AOffset: 0},
	}
	step := a.proofreading.ocrStep
	step.currentConnections = []proofreadingOCRConnection{
		{from: image.Pt(100, 50), to: image.Pt(400, 50), kind: proofreadingOCRDiffReplace, frame: comparison.Frames[0]},
	}

	a.openProofreadingDiffInspector(0)

	// 1. Not in exception yet -> toggle adds to exception
	if step.inspector.exceptions.Contains('（') {
		t.Fatal("expected '（' not in exception initially")
	}
	a.toggleCurrentDiffException()
	if !step.inspector.exceptions.Contains('（') {
		t.Fatal("expected '（' to be added to exception list")
	}
	if !strings.Contains(step.inspector.exceptionToast, "添加到例外") {
		t.Fatalf("expected toast to say 添加到例外, got %q", step.inspector.exceptionToast)
	}

	// 2. Now '（' is in exception -> toggle again removes from exception
	a.toggleCurrentDiffException()
	if step.inspector.exceptions.Contains('（') {
		t.Fatal("expected '（' to be removed from exception list")
	}
	if !strings.Contains(step.inspector.exceptionToast, "从例外中移除") {
		t.Fatalf("expected toast to say 从例外中移除, got %q", step.inspector.exceptionToast)
	}
}
