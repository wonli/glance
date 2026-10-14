package mcpserver

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPCLIRecognizesServeAndDocumentsStdoutContract(t *testing.T) {
	var stdout, stderr bytes.Buffer
	handled, exitCode := DispatchCLI([]string{"mcp", "serve", "--help"}, &stdout, &stderr)
	if !handled || exitCode != 0 {
		t.Fatalf("mcp serve --help = handled %v, code %d", handled, exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("CLI help wrote to stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "--instance-id") {
		t.Fatalf("CLI help does not describe --instance-id: %q", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	handled, exitCode = DispatchCLI([]string{"mcp", "serve", "--stdio", "unexpected"}, &stdout, &stderr)
	if !handled || exitCode != 2 || stderr.Len() == 0 {
		t.Fatalf("invalid MCP arguments = handled %v, code %d, stderr %q", handled, exitCode, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("CLI validation wrote to stdout: %q", stdout.String())
	}
}

func TestMCPCLIRejectsDesktopTargetWithoutMatchingInstance(t *testing.T) {
	options := newDesktopIPCOptions(t)
	var stdout, stderr bytes.Buffer
	handled, exitCode := dispatchCLIWithOptions(
		[]string{"mcp", "serve", "--stdio", "--instance-id", "missing-instance"},
		&stdout, &stderr, options, desktopIPCExecutablePath(t), io.NopCloser(strings.NewReader("")), &stdout,
	)
	if !handled || exitCode != 1 {
		t.Fatalf("missing desktop target = handled %v code %d; want handled with exit 1", handled, exitCode)
	}
	if !strings.Contains(stderr.String(), "no running Glance desktop instance") {
		t.Fatalf("missing desktop target diagnostic = %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("missing desktop target wrote protocol data to stdout: %q", stdout.String())
	}
}

func TestMCPCLIRejectsAmbiguousAndMismatchedDesktopTarget(t *testing.T) {
	appRuntime := newDesktopIPCTestRuntime(t)
	options := newDesktopIPCOptions(t)
	executable := desktopIPCExecutablePath(t)
	first, err := StartDesktopIPCServer(appRuntime, executable, options)
	if err != nil {
		t.Fatalf("start first desktop IPC server: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := StartDesktopIPCServer(appRuntime, executable, options)
	if err != nil {
		t.Fatalf("start second desktop IPC server: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	var stdout, stderr bytes.Buffer
	handled, exitCode := dispatchCLIWithOptions(
		[]string{"mcp", "serve", "--stdio"}, &stdout, &stderr, options, executable,
		io.NopCloser(strings.NewReader("")), &stdout,
	)
	if !handled || exitCode != 1 || !strings.Contains(strings.ToLower(stderr.String()), "ambiguous") {
		t.Fatalf("ambiguous desktop target = handled %v code %d stderr %q", handled, exitCode, stderr.String())
	}

	fakeExecutable := filepath.Join(t.TempDir(), "different-glance-binary")
	if err := os.WriteFile(fakeExecutable, []byte("not an executable"), 0o600); err != nil {
		t.Fatalf("create mismatched executable path: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	handled, exitCode = dispatchCLIWithOptions(
		[]string{"mcp", "serve", "--stdio", "--instance-id", first.Instance().ID},
		&stdout, &stderr, options, fakeExecutable, io.NopCloser(strings.NewReader("")), &stdout,
	)
	if !handled || exitCode != 1 || !strings.Contains(stderr.String(), "different executable") {
		t.Fatalf("mismatched executable target = handled %v code %d stderr %q", handled, exitCode, stderr.String())
	}
}

func TestMCPCLIDesktopAndHeadlessModesAreExclusive(t *testing.T) {
	options := newDesktopIPCOptions(t)
	for _, instanceArg := range []string{"--instance-id=instance", "--instance-id="} {
		var stdout, stderr bytes.Buffer
		handled, exitCode := dispatchCLIWithOptions(
			[]string{"mcp", "serve", "--stdio", "--open", filepath.Join(t.TempDir(), "input.json"), instanceArg},
			&stdout, &stderr, options, desktopIPCExecutablePath(t), io.NopCloser(strings.NewReader("")), &stdout,
		)
		if !handled || exitCode != 2 || !strings.Contains(stderr.String(), "cannot be combined") {
			t.Fatalf("mixed MCP modes with %s = handled %v code %d stderr %q", instanceArg, handled, exitCode, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("invalid MCP mode wrote to stdout: %q", stdout.String())
		}
	}
}
