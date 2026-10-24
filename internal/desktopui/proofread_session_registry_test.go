package desktopui

import (
	"gioui.org/app"
	"testing"
)

func TestProofreadingProjectHasSingleWritableWindow(t *testing.T) {
	r := &desktopRuntime{}
	first, second := new(app.Window), new(app.Window)
	if got := r.claimProofreadingWindow("book", first); got != nil {
		t.Fatal("first claim failed")
	}
	if got := r.claimProofreadingWindow("book", second); got != first {
		t.Fatal("second window could overwrite the first")
	}
	if got := r.claimProofreadingWindow("other", second); got != nil {
		t.Fatal("independent task blocked")
	}
}
