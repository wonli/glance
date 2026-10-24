package desktopui

import "testing"

func TestProofreadingProjectHasSingleWritableTab(t *testing.T) {
	r := newDesktopRuntime()
	first, second := newAppWithRuntime(nil, r), newAppWithRuntime(nil, r)
	a, b := &proofreadingTab{id: "first"}, &proofreadingTab{id: "second"}
	owner := first.registerProofreadingTab(a, "book")
	if owner.host != first || owner.id != a.id {
		t.Fatal("first registration failed")
	}
	if got := second.registerProofreadingTab(b, "book"); got != owner {
		t.Fatal("another tab could overwrite the first task")
	}
	if got := second.registerProofreadingTab(b, "other"); got.host != second {
		t.Fatal("independent task blocked")
	}
}
