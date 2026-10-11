package mcpserver

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDesktopEndpointIsLoopbackBearerProtectedAndServesMCP(t *testing.T) {
	runtime, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	endpoint, err := StartDesktopEndpoint(runtime)
	if err != nil {
		t.Fatalf("StartDesktopEndpoint: %v", err)
	}
	t.Cleanup(func() { _ = endpoint.Close() })
	info := endpoint.Info()
	if !strings.HasPrefix(info.URL, "http://127.0.0.1:") || !strings.HasSuffix(info.URL, "/mcp") {
		t.Fatalf("endpoint URL = %q; want IPv4 loopback /mcp", info.URL)
	}
	if len(info.Token) < 40 || !strings.Contains(info.Configuration, info.URL) || !strings.Contains(info.Configuration, info.Token) {
		t.Fatalf("endpoint connection info is incomplete: %+v", info)
	}

	unauthorized, err := http.Get(info.URL)
	if err != nil {
		t.Fatalf("unauthorized GET: %v", err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d; want 401", unauthorized.StatusCode)
	}

	badOrigin, err := http.NewRequest(http.MethodPost, info.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	badOrigin.Header.Set("Origin", "https://attacker.example")
	badOrigin.Header.Set("Authorization", "Bearer "+info.Token)
	badOrigin.Header.Set("Content-Type", "application/json")
	badOriginResponse, err := http.DefaultClient.Do(badOrigin)
	if err != nil {
		t.Fatalf("bad Origin request: %v", err)
	}
	badOriginResponse.Body.Close()
	if badOriginResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("bad Origin status = %d; want 403", badOriginResponse.StatusCode)
	}
	parsedEndpoint, err := url.Parse(info.URL)
	if err != nil {
		t.Fatal(err)
	}
	validOrigin := "http://" + parsedEndpoint.Host
	preflight, err := http.NewRequest(http.MethodOptions, info.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	preflight.Header.Set("Origin", validOrigin)
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	preflightResponse, err := http.DefaultClient.Do(preflight)
	if err != nil {
		t.Fatalf("valid Origin preflight: %v", err)
	}
	preflightResponse.Body.Close()
	if preflightResponse.StatusCode != http.StatusNoContent || preflightResponse.Header.Get("Access-Control-Allow-Origin") != validOrigin {
		t.Fatalf("valid Origin preflight status/header = %d/%q; want 204/%q", preflightResponse.StatusCode, preflightResponse.Header.Get("Access-Control-Allow-Origin"), validOrigin)
	}

	badHost, err := http.NewRequest(http.MethodGet, info.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	badHost.Host = "attacker.example:" + parsedEndpoint.Port()
	badHost.Header.Set("Authorization", "Bearer "+info.Token)
	badHostResponse, err := http.DefaultClient.Do(badHost)
	if err != nil {
		t.Fatalf("bad Host request: %v", err)
	}
	badHostResponse.Body.Close()
	if badHostResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("bad Host status = %d; want 403", badHostResponse.StatusCode)
	}
	badToken, err := http.NewRequest(http.MethodGet, info.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	badToken.Header.Set("Authorization", "Bearer wrong-token")
	badTokenResponse, err := http.DefaultClient.Do(badToken)
	if err != nil {
		t.Fatalf("invalid bearer request: %v", err)
	}
	badTokenResponse.Body.Close()
	if badTokenResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid bearer status = %d; want 401", badTokenResponse.StatusCode)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "desktop-endpoint-test", Version: "1"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   info.URL,
		HTTPClient: &http.Client{Transport: endpointAuthTransport{token: info.Token, base: http.DefaultTransport}},
	}
	session, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("connect authenticated MCP client: %v", err)
	}
	tools, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		_ = session.Close()
		t.Fatalf("list tools over desktop endpoint: %v", err)
	}
	if len(tools.Tools) == 0 {
		t.Fatal("desktop endpoint exposed no MCP tools")
	}

	if err := endpoint.Close(); err != nil {
		t.Fatalf("close endpoint: %v", err)
	}
	activeSessions := 0
	for range endpoint.mcp.server.Sessions() {
		activeSessions++
	}
	if activeSessions != 0 {
		t.Fatalf("desktop endpoint retained %d MCP sessions after close", activeSessions)
	}
	if _, err := session.ListTools(context.Background(), &mcp.ListToolsParams{}); err == nil {
		t.Fatal("desktop MCP client remained connected after endpoint close")
	}
	_ = session.Close()
	if err := endpoint.Close(); err != nil {
		t.Fatalf("repeat endpoint close: %v", err)
	}
}

type endpointAuthTransport struct {
	token  string
	origin string
	base   http.RoundTripper
}

func (t endpointAuthTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	if t.origin != "" {
		clone.Header.Set("Origin", t.origin)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}
