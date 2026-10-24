package proofreading

import (
	"reflect"
	"strings"
	"testing"
)

func TestPieceSplitSharesBackingAndKeepsIdentity(t *testing.T) {
	root := pieceFromRun(Run{First: 10, Text: "甲乙e\u0301👩‍💻尾", Typed: true})
	left, right := pieceSplit(root, 3)
	if pieceString(left) != "甲乙e" || pieceString(right) != "\u0301👩‍💻尾" {
		t.Fatal("bad rune split")
	}
	joined := pieceConcat(left, right)
	if pieceString(root) != pieceString(joined) {
		t.Fatal("original tree mutated")
	}
	var ids []CharacterID
	pieceVisit(joined, func(r Run) {
		for i := 0; i < len([]rune(r.Text)); i++ {
			ids = append(ids, r.First+CharacterID(i))
		}
	})
	if !reflect.DeepEqual(ids, []CharacterID{10, 11, 12, 13, 14, 15, 16, 17}) {
		t.Fatal(ids)
	}
}

func TestPieceLazyFlagsDoNotMutateSharedHistory(t *testing.T) {
	root := pieceFromRun(Run{First: 1, Text: "甲乙丙丁", Origin: 8, Protected: true})
	moved := pieceFlags(root, 0, protectionTyped)
	a, b := pieceSplit(moved, 2)
	joined := pieceConcat(a, b)
	pieceVisit(joined, func(r Run) {
		if r.Origin != 0 || r.Protected {
			t.Fatal(r)
		}
	})
	pieceVisit(root, func(r Run) {
		if r.Origin != 8 || !r.Protected {
			t.Fatal("history changed", r)
		}
	})
}

func TestPieceTreeRemainsBalancedAcrossRepeatedTransfers(t *testing.T) {
	root := pieceFromRun(Run{First: 1, Text: strings.Repeat("甲乙e\u0301👩‍💻尾", 256)})
	original := pieceString(root)
	saved := root
	for i := 1; i <= 600; i++ {
		at := (i * 7919) % pieceLen(root)
		left, right := pieceSplit(root, at)
		root = pieceConcat(left, right)
	}
	var check func(*pieceTree) (int, int)
	check = func(n *pieceTree) (int, int) {
		if n == nil {
			return 0, 0
		}
		if n.left == nil {
			return 1, len([]rune(n.run.Text))
		}
		ah, an := check(n.left)
		bh, bn := check(n.right)
		if ah-bh > 1 || bh-ah > 1 || n.length != an+bn || n.height != 1+max(ah, bh) {
			t.Fatal("unbalanced or incorrect cached length", ah, bh, n.height, n.length)
		}
		return n.height, n.length
	}
	check(root)
	if pieceString(root) != original || pieceString(saved) != original {
		t.Fatal("transfer changed shared text")
	}
}

func TestCapturedBoxesStayImmutableAfterRefillAndBranchEdit(t *testing.T) {
	m := model(t)
	view := m.Capture()
	before := view.Snapshot()
	must(t, m.AdjustBoundary(1, 2))
	must(t, m.ReplacePage(2, "改过的豆子"))
	if !m.Undo() || !m.Undo() {
		t.Fatal("undo")
	}
	must(t, m.ReplacePage(1, "新的分支"))
	if m.Redo() {
		t.Fatal("redo branch retained")
	}
	after := view.Snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("worker capture mutated after UI edit")
	}
	must(t, m.Validate())
}

func TestDerivedReflowJoinsChangedBoxHistory(t *testing.T) {
	m, _ := New("abcd", []PageID{1, 2})
	must(t, m.AssignInitial([]int{2, 4}))
	must(t, m.ReplacePage(1, "aXb"))
	must(t, m.ReflowDerived(map[PageID]int{1: 1, 2: 5}))
	if m.Text(1) != "a" || m.Text(2) != "Xbcd" {
		t.Fatal(m.Snapshot())
	}
	if !m.Undo() || m.Text(1) != "ab" || m.Text(2) != "cd" {
		t.Fatal("derived page left behind by delta undo", m.Snapshot())
	}
	if !m.Redo() || m.Text(1) != "a" || m.Text(2) != "Xbcd" {
		t.Fatal("derived redo incomplete", m.Snapshot())
	}
}

func TestNoopFlagsAndEndpointSplitPreserveRedo(t *testing.T) {
	m, _ := New("abcd", []PageID{1, 2})
	must(t, m.AssignInitial([]int{2, 4}))
	must(t, m.ReplacePage(1, "aXb"))
	if !m.Undo() {
		t.Fatal("undo")
	}
	revision := m.Revision()
	must(t, m.Lock(1, false))
	must(t, m.Lock(1, false))
	must(t, m.Split(1, 2, false))
	if m.Revision() != revision || !m.Redo() || m.Text(1) != "aXb" {
		t.Fatal("no-op changed history", m.Snapshot())
	}
}

func TestSharedSourceCoalescesTransferredPieces(t *testing.T) {
	root := pieceFromRun(Run{First: 1, Text: strings.Repeat("甲", 900)})
	var rebuilt *pieceTree
	for pieceLen(root) > 0 {
		first, rest := pieceSplit(root, 1)
		rebuilt = pieceConcat(rebuilt, first)
		root = rest
	}
	if rebuilt.height != 1 || pieceLen(rebuilt) != 900 || rebuilt.start != 0 || rebuilt.end != 2700 {
		t.Fatal("contiguous source was fragmented", rebuilt.height, pieceLen(rebuilt))
	}
}
