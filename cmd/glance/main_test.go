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

func TestDesktopArgumentsAcceptWindowsChromePreview(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		path    string
		preview bool
	}{
		{[]string{"--simulate-windows-chrome"}, "", true},
		{[]string{"--simulate-windows-chrome", "/tmp/report.docx"}, "/tmp/report.docx", true},
		{[]string{"/tmp/report.docx", "--simulate-windows-chrome"}, "/tmp/report.docx", true},
		{[]string{"/tmp/report.docx"}, "/tmp/report.docx", false},
	} {
		path, opts := parseDesktopArguments(tc.args)
		if path != tc.path || opts.SimulateWindowsChrome != tc.preview {
			t.Fatalf("%v: path=%q preview=%v", tc.args, path, opts.SimulateWindowsChrome)
		}
	}
}
