// Package diagnostics provides opt-in local desktop profiling.
package diagnostics

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"time"
)

// StartProfiling starts a pprof server at the explicitly configured address. An empty address disables it.
// Close the returned server to stop profiling. Addr contains the bound address.
func StartProfiling(addr string) (*http.Server, error) {
	if addr == "" {
		return nil, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("pprof listen: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	server := &http.Server{Addr: listener.Addr().String(), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("pprof HTTP server: %v", err)
		}
	}()
	return server, nil
}
