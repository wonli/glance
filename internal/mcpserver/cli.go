package mcpserver

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"glance/internal/application"
)

func DispatchCLI(args []string, stdout, stderr io.Writer) (bool, int) {
	return dispatchCLIWithOptions(args, stdout, stderr, DesktopIPCOptions{}, "", os.Stdin, stdout)
}

// dispatchCLIWithOptions isolates endpoint and stdio dependencies for tests;
// production callers go through DispatchCLI and use the process defaults.
func dispatchCLIWithOptions(args []string, stdout, stderr io.Writer, ipcOptions DesktopIPCOptions, executablePath string, stdin io.ReadCloser, protocolStdout io.Writer) (bool, int) {
	_ = executablePath // Kept in the helper signature for existing callers; endpoint selection is per-user.
	if len(args) == 0 || args[0] != "mcp" {
		return false, 0
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if protocolStdout == nil {
		protocolStdout = stdout
	}
	if stdin == nil {
		stdin = os.Stdin
	}
	if len(args) < 2 || args[1] != "serve" {
		writeMCPUsage(stderr)
		return true, 2
	}
	flags := flag.NewFlagSet("glance mcp serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stdio := flags.Bool("stdio", false, "serve MCP over standard input and output")
	flags.String("instance-id", "", "deprecated compatibility flag; ignored")
	var openPaths stringListFlag
	flags.Var(&openPaths, "open", "open a document path before serving; may be repeated for headless work")
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
	if len(openPaths) == 0 {
		address, err := desktopMCPAddressForOptions(ipcOptions)
		if err != nil {
			fmt.Fprintf(stderr, "resolve current Glance desktop MCP endpoint: %v\n", err)
			return true, 1
		}
		conn, err := dialDesktopIPC(address)
		if err != nil {
			fmt.Fprintf(stderr, "connect to the current Glance desktop MCP endpoint: %v\n", err)
			return true, 1
		}
		if err := bridgeDesktopMCPStdio(stdin, protocolStdout, conn); err != nil {
			fmt.Fprintf(stderr, "desktop MCP stdio bridge stopped: %v\n", err)
			return true, 1
		}
		return true, 0
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
	serveErr := server.RunStdioWithIO(context.Background(), stdin, writerAsWriteCloser(protocolStdout))
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

func bridgeDesktopMCPStdio(stdin io.ReadCloser, stdout io.Writer, conn net.Conn) error {
	if stdin == nil || conn == nil {
		return errors.New("desktop MCP stdio and IPC connection are required")
	}
	inputDone := make(chan error, 1)
	outputDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(conn, stdin)
		if err != nil {
			_ = conn.Close()
		} else if halfCloser, ok := conn.(interface{ CloseWrite() error }); ok {
			_ = halfCloser.CloseWrite()
		} else {
			_ = conn.Close()
		}
		inputDone <- err
	}()
	go func() {
		_, err := io.Copy(stdout, conn)
		outputDone <- err
	}()

	select {
	case inputErr := <-inputDone:
		outputErr := <-outputDone
		return errors.Join(normalizeBridgeError(inputErr), normalizeBridgeError(outputErr))
	case outputErr := <-outputDone:
		_ = conn.Close()
		_ = stdin.Close()
		inputErr := <-inputDone
		return errors.Join(normalizeBridgeError(outputErr), normalizeBridgeError(inputErr))
	}
}

func normalizeBridgeError(err error) error {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed) {
		return nil
	}
	return err
}

type writerOnlyCloser struct{ io.Writer }

func (writerOnlyCloser) Close() error { return nil }

func writerAsWriteCloser(writer io.Writer) io.WriteCloser {
	if closer, ok := writer.(io.WriteCloser); ok {
		return closer
	}
	return writerOnlyCloser{Writer: writer}
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
