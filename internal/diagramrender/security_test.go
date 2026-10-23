package diagramrender

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestVegaValidationAndExternalResourceRejection(t *testing.T) {
	for kind, source := range map[string]string{"vega": testVegaSpec, "vega-lite": testVegaLiteSpec} {
		if err := validate(Request{Kind: kind, Source: []byte(source), Density: 2}); err != nil {
			t.Errorf("valid inline %s spec rejected: %v", kind, err)
		}
	}
	for name, source := range map[string]string{
		"data URL":             `{"data":{"url":"https://example.invalid/data.json"},"mark":"bar"}`,
		"embed patch URL":      `{"usermeta":{"embedOptions":{"patch":"https://example.invalid/patch.json"}},"data":{"values":[]},"mark":"bar"}`,
		"image URL":            `{"data":{"values":[{"img":"https://example.invalid/pixel.png"}]},"mark":{"type":"image"},"encoding":{"url":{"field":"img","type":"nominal"}}}`,
		"javascript image URL": `{"data":{"values":[{"asset":"javascript:alert(1)"}]},"mark":{"type":"image"},"encoding":{"url":{"field":"asset","type":"nominal"}}}`,
	} {
		err := validate(Request{Kind: "vega-lite", Source: []byte(source), Density: 2})
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "external") {
			t.Errorf("%s was not rejected as an external resource: %v", name, err)
		}
	}
	if err := validate(Request{Kind: "vega-lite", Source: []byte(`{"mark":`), Density: 2}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "json") {
		t.Errorf("invalid JSON did not produce a JSON diagnostic: %v", err)
	}
}

func TestVegaRejectsHyperlinkChannels(t *testing.T) {
	for name, source := range map[string]string{
		"literal URL":    `{"data":{"values":[{"label":"A"}]},"marks":[{"type":"symbol","encode":{"update":{"href":{"value":"https://example.invalid/"}}}}]}`,
		"field URL":      `{"data":{"values":[{"label":"A","link":"https://example.invalid/"}]},"marks":[{"type":"symbol","encode":{"update":{"href":{"field":"link"}}}}]}`,
		"javascript URL": `{"marks":[{"type":"symbol","encode":{"update":{"href":{"value":"javascript:alert(1)"}}}}]}`,
	} {
		err := validate(Request{Kind: "vega", Source: []byte(source), Density: 2})
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "hyperlink") {
			t.Errorf("%s was not rejected as a hyperlink channel: %v", name, err)
		}
	}
	if err := validate(Request{Kind: "vega", Source: []byte(`{"data":[{"name":"table","values":[{"href":"ordinary label"}]}],"marks":[{"type":"text","from":{"data":"table"},"encode":{"update":{"text":{"field":"href"}}}}]}`), Density: 2}); err != nil {
		t.Fatalf("ordinary inline field named href was rejected without a hyperlink channel: %v", err)
	}
}

func TestVegaInputAndGeometryLimits(t *testing.T) {
	for name, req := range map[string]Request{
		"source bytes": {Kind: "vega-lite", Density: 2, Source: []byte(strings.Repeat(" ", (1<<20)+1))},
		"width":        {Kind: "vega-lite", Density: 2, Source: []byte(`{"width":5000,"height":10,"data":{"values":[]},"mark":"bar"}`)},
		"pixels":       {Kind: "vega-lite", Density: 2, Source: []byte(`{"width":4000,"height":4000,"data":{"values":[]},"mark":"bar"}`)},
	} {
		if err := validate(req); err == nil {
			t.Errorf("oversized %s accepted", name)
		}
	}
	var spec strings.Builder
	spec.WriteString(`{"data":{"values":[`)
	for i := 0; i < 10001; i++ {
		if i > 0 {
			spec.WriteByte(',')
		}
		fmt.Fprintf(&spec, `{"x":%d}`, i)
	}
	spec.WriteString(`]},"mark":"point"}`)
	if err := validate(Request{Kind: "vega-lite", Source: []byte(spec.String()), Density: 2}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "row") {
		t.Fatalf("inline row limit was not enforced: %v", err)
	}
}

func TestVegaPageIsOfflineAndUsesInterpreter(t *testing.T) {
	html := getSharedHTML()
	if !strings.Contains(html, "default-src 'none'") || !strings.Contains(html, "connect-src 'none'") || !strings.Contains(html, "img-src data:") {
		t.Fatal("Vega page CSP does not restrict network resources")
	}
	if strings.Contains(html, "unsafe-eval") {
		t.Fatal("Vega page CSP enables unsafe-eval")
	}
	for _, name := range []string{"vegaEmbed", "vegaExpressionInterpreter"} {
		if !strings.Contains(html, name) {
			t.Errorf("Vega runtime is missing global %s", name)
		}
	}
	script := renderScript(Request{Kind: "vega-lite", Source: []byte(testVegaLiteSpec)})
	for _, token := range []string{"actions:false", "ast:true", "expr:vegaExpressionInterpreter", "external Vega resource disabled"} {
		if !strings.Contains(script, token) {
			t.Errorf("Vega render script is missing %q", token)
		}
	}
}

func TestRendererRejectsExternalRequests(t *testing.T) {
	r := testRenderer(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) { hits.Add(1); fmt.Fprint(w, "SECRET") }))
	defer server.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "sentinel.svg")
	os.WriteFile(path, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><text>SECRET</text></svg>`), 0600)
	source := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="80"><style>@import url('%s/css');</style><script>fetch('%s/script')</script><image href="%s/img" width="120" height="80"/><image href="file://%s" width="120" height="80"/><foreignObject width="120" height="80"><body xmlns="http://www.w3.org/1999/xhtml"><script>fetch('%s/foreign')</script><img src="%s/html"/></body></foreignObject><rect width="10" height="10" fill="red"/></svg>`, server.URL, server.URL, server.URL, path, server.URL, server.URL)
	if _, e := r.Render(context.Background(), Request{Kind: "svg", Source: []byte(source), Density: 2}); e != nil {
		t.Fatal(e)
	}
	if hits.Load() != 0 {
		t.Fatalf("SVG fetched %d requests", hits.Load())
	}
	for _, s := range []string{"%%{init:{securityLevel:'loose'}}%%\ngraph TD;A-->B", "---\nconfig:\n securityLevel: loose\n---\ngraph TD;A-->B"} {
		if _, e := r.Render(context.Background(), Request{Kind: "mermaid", Source: []byte(s), Density: 2}); e == nil {
			t.Fatal("unsafe config accepted")
		}
	}
}
func TestRenderSyntaxErrorDoesNotPoisonNextJob(t *testing.T) {
	r := testRenderer(t)
	if _, e := r.Render(context.Background(), Request{Kind: "mermaid", Source: []byte("flowchart TD\n [invalid"), Density: 2}); e == nil {
		t.Fatal("syntax error accepted")
	}
	if _, e := r.Render(context.Background(), Request{Kind: "mermaid", Source: []byte("graph LR;A-->B"), Density: 2}); e != nil {
		t.Fatal(e)
	}
}
func TestSVGRejectsDTDAndBadRoot(t *testing.T) {
	r := New(Bundle{})
	defer r.Close()
	for _, s := range []string{`<!DOCTYPE svg [<!ENTITY x SYSTEM "file:///etc/passwd">]><svg>&x;</svg>`, `<html/>`, `<svg>`, strings.Repeat("<svg>", 129) + strings.Repeat("</svg>", 129)} {
		if _, e := r.Render(context.Background(), Request{Kind: "svg", Source: []byte(s), Density: 2}); e == nil {
			t.Fatal("unsafe SVG accepted")
		}
	}
}
func TestRenderFlowchartFeatures(t *testing.T) {
	r := testRenderer(t)
	for _, source := range []string{"graph LR\n A[开始] -->|前进| B{判断}\n B --> C[结束]", "flowchart TB\n subgraph 分组\n A --> B\n end\n classDef good fill:#0f0\n class A good"} {
		if _, e := r.Render(context.Background(), Request{Kind: "mermaid", Source: []byte(source), Density: 2}); e != nil {
			t.Fatal(e)
		}
	}
}
func TestRendererRejectsClosedService(t *testing.T) {
	r := New(Bundle{})
	r.Close()
	if _, e := r.Render(context.Background(), Request{Kind: "mermaid", Source: []byte("graph TD;A-->B"), Density: 2}); e == nil {
		t.Fatal("closed service accepted")
	}
}

func TestRenderSVGRejectsHugeGeometry(t *testing.T) {
	r := testRenderer(t)
	_, e := r.Render(context.Background(), Request{Kind: "svg", Density: 2, Source: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100000" height="100000"/>`)})
	if e == nil || !strings.Contains(e.Error(), "megapixels") {
		t.Fatalf("expected pixel budget rejection, got %v", e)
	}
}
func TestRenderRejectsTooManyEdges(t *testing.T) {
	r := testRenderer(t)
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for i := 0; i < 501; i++ {
		fmt.Fprintf(&b, "A%d --> A%d\n", i, i+1)
	}
	_, e := r.Render(context.Background(), Request{Kind: "mermaid", Source: []byte(b.String()), Density: 2})
	if e == nil || !strings.Contains(strings.ToLower(e.Error()), "edge") {
		t.Fatalf("expected edge limit, got %v", e)
	}
}
func TestCloseReapsChildAndProfile(t *testing.T) {
	r := testRenderer(t)
	if _, e := r.Render(context.Background(), Request{Kind: "svg", Density: 2, Source: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8" fill="red"/></svg>`)}); e != nil {
		t.Fatal(e)
	}
	r.Close()
	if len(r.cache) != 0 || r.cacheBytes != 0 {
		t.Fatal("cache was not cleared on close")
	}
}
func TestFontVersionInvalidatesDiagramCache(t *testing.T) {
	r := testRenderer(t)
	q := Request{Kind: "svg", Density: 2, Source: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8" fill="red"/></svg>`)}
	if _, e := r.Render(context.Background(), q); e != nil {
		t.Fatal(e)
	}
	r.mu.Lock()
	cachedCount := len(r.cache)
	r.mu.Unlock()
	if cachedCount == 0 {
		t.Fatal("expected cached result")
	}
	SetFontVersion(fontVersion.Load() + 1)
	if _, e := r.Render(context.Background(), q); e != nil {
		t.Fatal(e)
	}
	r.mu.Lock()
	newCount := len(r.cache)
	r.mu.Unlock()
	if newCount != 1 {
		t.Fatalf("expected cache reset and repopulated, got count %d", newCount)
	}
}

func TestCrashRestartsRenderer(t *testing.T) {
	t.Skip("browser crash test applies only to legacy chrome subprocess")
}
func TestQueueAndCacheAreBounded(t *testing.T) {
	r := testRenderer(t)
	for i := 0; i < 33; i++ {
		q := Request{Kind: "svg", Density: 2, Source: []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect id="r%d" width="8" height="8" fill="red"/></svg>`, i))}
		if _, e := r.Render(context.Background(), q); e != nil {
			t.Fatal(e)
		}
	}
	if len(r.cache) > 32 || r.cacheBytes > 32<<20 {
		t.Fatal("unbounded retained images")
	}
	for i := 0; i < 16; i++ {
		r.queue <- struct{}{}
	}
	ctx, c := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer c()
	if _, e := r.Render(ctx, Request{Kind: "mermaid", Density: 2, Source: []byte("graph TD;A-->B")}); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("full queue ignored cancellation: %v", e)
	}
}
