package markdowndoc

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"glance/internal/document"
)

func resourceImageBytes(t *testing.T, format string) []byte {
	t.Helper()
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 12, 8))
	var err error
	switch format {
	case "png":
		err = png.Encode(&b, img)
	case "jpeg":
		err = jpeg.Encode(&b, img, nil)
	case "gif":
		err = gif.Encode(&b, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestLoadImageRemoteFormatsAndRedirect(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		t.Run(format, func(t *testing.T) {
			data := resourceImageBytes(t, format)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirect" {
					http.Redirect(w, r, "/image?version=1", http.StatusFound)
					return
				}
				if r.URL.RawQuery != "version=1" {
					t.Errorf("lost image URL query %q", r.URL.RawQuery)
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Write(data)
			}))
			defer server.Close()
			img, err := LoadImage(context.Background(), server.URL+"/redirect", "")
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != 12 || img.Bounds().Dy() != 8 {
				t.Fatal(img.Bounds())
			}
		})
	}
}

func TestLoadImageRejectsHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{{"404", "missing", "HTTP 404", 404}, {"invalid", "not an image", "PNG、JPEG、GIF 或 SVG", 200}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) }))
			defer server.Close()
			_, err := LoadImage(context.Background(), server.URL, "")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestLoadImageRejectsOversizeAndTruncatedImages(t *testing.T) {
	data := resourceImageBytes(t, "png")
	oversized := append([]byte(nil), data...)
	binary.BigEndian.PutUint32(oversized[16:20], 20000)
	binary.BigEndian.PutUint32(oversized[20:24], 20000)
	binary.BigEndian.PutUint32(oversized[29:33], crc32.ChecksumIEEE(oversized[12:29]))
	for _, tc := range []struct {
		name   string
		body   []byte
		length string
	}{{"bytes", nil, "33554433"}, {"pixels", oversized, ""}, {"truncated", data[:40], ""}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.length != "" {
					w.Header().Set("Content-Length", tc.length)
				}
				w.Write(tc.body)
			}))
			defer server.Close()
			if _, err := LoadImage(context.Background(), server.URL, ""); err == nil {
				t.Fatal("unsafe image accepted")
			}
		})
	}
}

func TestLoadImageCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := LoadImage(ctx, server.URL, ""); done <- err }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
}

func TestLoadImageLocalAndRemotePreview(t *testing.T) {
	dir := t.TempDir()
	data := resourceImageBytes(t, "png")
	if err := os.WriteFile(filepath.Join(dir, "a b.png"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if img, err := LoadImage(context.Background(), "a%20b.png", filepath.Join(dir, "source.md")); err != nil || img == nil {
		t.Fatalf("local=%v", err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.Write(data) }))
	defer server.Close()
	span := document.MarkdownSpan{Kind: "image", Destination: server.URL + "/image.png"}
	snapshot := document.MarkdownSnapshot{Blocks: []document.MarkdownBlock{{Kind: "paragraph", Spans: []document.MarkdownSpan{span, span}}}}
	images, diagnostics := PreviewResourcesContext(context.Background(), snapshot, "")
	if len(images) != 1 || len(diagnostics) != 0 || requests.Load() != 1 {
		t.Fatalf("images=%d errors=%v requests=%d", len(images), diagnostics, requests.Load())
	}
}

func TestLoadImageHTTPSAndRedirectLimit(t *testing.T) {
	data := resourceImageBytes(t, "png")
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer secure.Close()
	if img, err := loadHTTPImage(context.Background(), secure.URL+"/image.png", secure.Client()); err != nil || img == nil {
		t.Fatalf("HTTPS image: %v", err)
	}
	loop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/again", http.StatusFound) }))
	defer loop.Close()
	if _, err := LoadImage(context.Background(), loop.URL, ""); err == nil || !strings.Contains(err.Error(), "重定向") {
		t.Fatalf("redirect loop: %v", err)
	}
}

func TestLoadImageStreamingLimitAndTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			<-r.Context().Done()
			return
		}
		w.(http.Flusher).Flush()
		chunk := make([]byte, 1<<20)
		for range 33 {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	if _, err := LoadImage(context.Background(), server.URL+"/chunked", ""); err == nil || !strings.Contains(err.Error(), "32 MiB") {
		t.Fatalf("stream limit: %v", err)
	}
	client := &http.Client{Timeout: 20 * time.Millisecond}
	if _, err := loadHTTPImage(context.Background(), server.URL+"/slow", client); err == nil {
		t.Fatal("request timeout ignored")
	}
}

func TestPreviewResourcesBoundsConcurrentNetworkRequests(t *testing.T) {
	data := resourceImageBytes(t, "png")
	var active, peak atomic.Int32
	gate := make(chan struct{})
	started := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		started <- struct{}{}
		<-gate
		w.Write(data)
	}))
	defer server.Close()
	snapshot := document.MarkdownSnapshot{}
	for i := 0; i < 8; i++ {
		snapshot.Blocks = append(snapshot.Blocks, document.MarkdownBlock{Kind: "paragraph", Spans: []document.MarkdownSpan{{Kind: "image", Destination: fmt.Sprintf("%s/%d.png", server.URL, i)}}})
	}
	done := make(chan int, 1)
	go func() {
		images, diagnostics := PreviewResourcesContext(context.Background(), snapshot, "")
		if len(diagnostics) > 0 {
			t.Errorf("diagnostics=%v", diagnostics)
		}
		done <- len(images)
	}()
	for range 4 {
		<-started
	}
	close(gate)
	if count := <-done; count != 8 || peak.Load() > 4 {
		t.Fatalf("images=%d concurrent requests=%d", count, peak.Load())
	}
}

func TestResourceKeySeparatesFormulaStyles(t *testing.T) {
	inline := document.MarkdownSpan{Kind: "math", Text: "x^2"}
	display := inline
	display.DisplayMath = true
	if ResourceKey(inline) == ResourceKey(display) {
		t.Fatal("display and inline formula cache collide")
	}
}
