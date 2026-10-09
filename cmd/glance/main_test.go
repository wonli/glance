package main

import (
	"bytes"
	"testing"
)

func TestDispatchBeforeDesktopPreservesInteractiveFileArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	handled, code := dispatchBeforeDesktop([]string{"/tmp/report.docx"}, &stdout, &stderr)
	if handled || code != 0 {
		t.Fatalf("interactive path was routed to QA: handled=%v exit=%d", handled, code)
	}
}

func TestDispatchBeforeDesktopHandlesQAWithoutStartingGio(t *testing.T) {
	var stdout, stderr bytes.Buffer
	handled, code := dispatchBeforeDesktop([]string{"qa", "render"}, &stdout, &stderr)
	if !handled || code != 2 {
		t.Fatalf("invalid QA command was not handled before Gio: handled=%v exit=%d", handled, code)
	}
}
