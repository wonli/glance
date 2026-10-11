package mcpserver

import (
	"bytes"
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
	if stderr.Len() == 0 {
		t.Fatal("CLI help did not describe the stdio command")
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
