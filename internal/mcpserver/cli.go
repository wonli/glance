package mcpserver

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"glance/internal/application"
)

func DispatchCLI(args []string, stdout, stderr io.Writer) (bool, int) {
	if len(args) == 0 || args[0] != "mcp" {
		return false, 0
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) < 2 || args[1] != "serve" {
		writeMCPUsage(stderr)
		return true, 2
	}
	flags := flag.NewFlagSet("glance mcp serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stdio := flags.Bool("stdio", false, "serve MCP over standard input and output")
	var openPaths stringListFlag
	flags.Var(&openPaths, "open", "open a document path before serving; may be repeated")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: glance mcp serve --stdio [--open PATH ...]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, 0
		}
		return true, 2
	}
	if !*stdio || flags.NArg() != 0 {
		flags.Usage()
		return true, 2
	}
	runtime, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		fmt.Fprintf(stderr, "start application runtime: %v\n", err)
		return true, 1
	}
	for _, path := range openPaths {
		if _, err := runtime.OpenDocument(context.Background(), path); err != nil {
			_ = runtime.Close(context.Background())
			fmt.Fprintf(stderr, "open document %q: %v\n", path, err)
			return true, 1
		}
	}
	server, err := New(runtime)
	if err != nil {
		_ = runtime.Close(context.Background())
		fmt.Fprintf(stderr, "start MCP server: %v\n", err)
		return true, 1
	}
	serveErr := server.RunStdio(context.Background())
	closeErr := runtime.Close(context.Background())
	if serveErr != nil && !errors.Is(serveErr, io.EOF) && !errors.Is(serveErr, context.Canceled) {
		fmt.Fprintf(stderr, "MCP server stopped: %v\n", serveErr)
		return true, 1
	}
	if closeErr != nil {
		fmt.Fprintf(stderr, "close application runtime: %v\n", closeErr)
		return true, 1
	}
	return true, 0
}

func writeMCPUsage(stderr io.Writer) {
	fmt.Fprintln(stderr, "Usage: glance mcp serve --stdio [--open PATH ...]")
	fmt.Fprintln(stderr, "       glance (without a subcommand starts the desktop application)")
}

type stringListFlag []string

func (values *stringListFlag) String() string { return strings.Join(*values, ",") }
func (values *stringListFlag) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("path must not be empty")
	}
	*values = append(*values, value)
	return nil
}
