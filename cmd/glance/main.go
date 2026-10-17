package main

import (
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"

	"glance/internal/desktoplaunch"
)

func main() {
	if handled, exitCode := dispatchBeforeDesktop(os.Args[1:], os.Stdout, os.Stderr); handled {
		os.Exit(exitCode)
	}

	pprofAddr := os.Getenv("GLANCE_PPROF_ADDR")
	if pprofAddr == "" {
		pprofAddr = "localhost:6060"
	}
	go func() {
		listener, err := net.Listen("tcp", pprofAddr)
		if err != nil {
			log.Printf("pprof listener failed on %s: %v", pprofAddr, err)
			return
		}
		log.Printf("pprof profiling available at http://%s/debug/pprof/", listener.Addr().String())
		if err := http.Serve(listener, nil); err != nil {
			log.Printf("pprof HTTP server: %v", err)
		}
	}()

	path := ""
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	desktoplaunch.RunDesktop(path)
}

func dispatchBeforeDesktop(args []string, stdout, stderr io.Writer) (bool, int) {
	return desktoplaunch.DispatchCLI(args, stdout, stderr)
}
