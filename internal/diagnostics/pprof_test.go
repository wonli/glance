package diagnostics

import (
	"io"
	"net"
	"net/http"
	"testing"
)

func TestProfilingDisabled(t *testing.T) {
	server, err := StartProfiling("")
	if err != nil || server != nil {
		t.Fatalf("disabled: server=%v err=%v", server, err)
	}
}

func TestProfilingEndpoints(t *testing.T) {
	server, err := StartProfiling("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	for _, path := range []string{"/debug/pprof/", "/debug/pprof/heap", "/debug/pprof/window-latency", "/debug/pprof/word-selection", "/debug/word-selection", "/debug/pprof/allocs", "/debug/pprof/goroutine?debug=1"} {
		response, err := http.Get("http://" + server.Addr + path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || len(body) == 0 {
			t.Fatalf("%s: status=%d bytes=%d err=%v", path, response.StatusCode, len(body), err)
		}
	}
}

func TestProfilingLANListener(t *testing.T) {
	server, err := StartProfiling("0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	host, port, err := net.SplitHostPort(server.Addr)
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsUnspecified() {
		t.Fatalf("listener is not wildcard: %s", server.Addr)
	}
	response, err := http.Get("http://127.0.0.1:" + port + "/debug/pprof/heap")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", response.StatusCode)
	}
}

func TestProfilingInvalidAddress(t *testing.T) {
	server, err := StartProfiling("bad-address")
	if server != nil {
		server.Close()
	}
	if err == nil {
		t.Fatal("accepted invalid address")
	}
}
