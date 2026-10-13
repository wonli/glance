package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DesktopEndpointInfo contains the short-lived connection details for the
// local desktop MCP endpoint. The token is generated for every app launch.
type DesktopEndpointInfo struct {
	URL           string
	Token         string
	Configuration string
}

// DesktopEndpoint serves the same MCP catalog as stdio against a desktop's
// live application runtime.
type DesktopEndpoint struct {
	info     DesktopEndpointInfo
	listener net.Listener
	server   *http.Server
	mcp      *Server
	close    sync.Once
	closeErr error
}

type desktopEndpointConfiguration struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// StartDesktopEndpoint binds only to IPv4 loopback and starts a bearer-protected
// Streamable HTTP endpoint.
func StartDesktopEndpoint(runtime *application.Runtime) (*DesktopEndpoint, error) {
	if runtime == nil {
		return nil, errors.New("application runtime is required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen on loopback: %w", err)
	}
	closeListener := true
	defer func() {
		if closeListener {
			_ = listener.Close()
		}
	}()

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate MCP bearer token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	address := listener.Addr().(*net.TCPAddr)
	endpointURL := "http://127.0.0.1:" + strconv.Itoa(address.Port) + "/mcp"
	configuration, err := json.MarshalIndent(desktopEndpointConfiguration{
		URL: endpointURL, Headers: map[string]string{"Authorization": "Bearer " + token},
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode MCP connection configuration: %w", err)
	}

	instance, err := New(runtime)
	if err != nil {
		return nil, err
	}
	endpoint := &DesktopEndpoint{
		info:     DesktopEndpointInfo{URL: endpointURL, Token: token, Configuration: string(configuration)},
		listener: listener,
		mcp:      instance,
	}
	endpoint.server = &http.Server{
		Handler:           endpoint.authorize(instance.HTTPHandler(&mcp.StreamableHTTPOptions{SessionTimeout: 30 * time.Minute, MaxRequestBodyBytes: 4 << 20, PropagateRequestCancellation: true})),
		ReadHeaderTimeout: 5 * time.Second,
	}
	closeListener = false
	go func() {
		_ = endpoint.server.Serve(listener)
	}()
	return endpoint, nil
}

func (e *DesktopEndpoint) Info() DesktopEndpointInfo {
	if e == nil {
		return DesktopEndpointInfo{}
	}
	return e.info
}

// Close stops accepting requests and waits briefly for active MCP requests to
// finish before forcing remaining connections closed.
func (e *DesktopEndpoint) Close() error {
	if e == nil || e.server == nil {
		return nil
	}
	e.close.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		shutdownErr := e.server.Shutdown(ctx)
		if shutdownErr != nil {
			shutdownErr = e.server.Close()
		}
		e.closeErr = errors.Join(shutdownErr, e.mcp.CloseSessions())
	})
	return e.closeErr
}

func (e *DesktopEndpoint) authorize(next http.Handler) http.Handler {
	port := e.listener.Addr().(*net.TCPAddr).Port
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRemote(r.RemoteAddr) || !isExpectedLoopbackHost(r.Host, port) {
			http.Error(w, "MCP endpoint accepts loopback requests only", http.StatusForbidden)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && !isExpectedLoopbackOrigin(origin, port) {
			http.Error(w, "MCP endpoint rejects this Origin", http.StatusForbidden)
			return
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Mcp-Session-Id, Mcp-Protocol-Version, Last-Event-Id")
		}
		if r.Method == http.MethodOptions {
			if origin == "" {
				http.Error(w, "Origin is required for preflight", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(provided), []byte(e.info.Token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "valid MCP bearer token required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	host, _, _ = strings.Cut(host, "%")
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isExpectedLoopbackHost(hostport string, expectedPort int) bool {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil || port != strconv.Itoa(expectedPort) {
		return false
	}
	return isLoopbackHost(host)
}

func isExpectedLoopbackOrigin(origin string, expectedPort int) bool {
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	port := parsed.Port()
	if port == "" || port != strconv.Itoa(expectedPort) {
		return false
	}
	return isLoopbackHost(parsed.Hostname())
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	host, _, _ = strings.Cut(host, "%")
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
