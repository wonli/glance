package markdowndoc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"glance/internal/diagramrender"
	"glance/internal/document"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Resource decoding is used by background preview/export workers, never layout.
func PreviewResources(p document.MarkdownSnapshot, path string) (map[string]image.Image, map[string]string) {
	return PreviewResourcesContext(context.Background(), p, path)
}

// ResourceKey distinguishes display math from inline math at the cache boundary.
func ResourceKey(span document.MarkdownSpan) string {
	if span.Kind == "diagram" {
		language := strings.ToLower(span.Language)
		if language == "" {
			language = "mermaid"
		}
		if language == "vega" || language == "vega-lite" {
			return fmt.Sprintf("diagram:%s:%d:%d:%x", language, span.SourceStart, span.SourceEnd, sha256.Sum256([]byte(span.Text)))
		}
		return fmt.Sprintf("diagram:%s:%x", language, sha256.Sum256([]byte(span.Text)))
	}
	if span.Kind == "image" {
		return "image:" + span.Destination
	}
	if span.DisplayMath {
		return "math:display:" + span.Text
	}
	return "math:" + span.Text
}

// PreviewResourcesContext bounds network concurrency and the whole resource job.
// Callers run it outside Gio frame callbacks and cancel superseded snapshots.
func PreviewResourcesContext(ctx context.Context, p document.MarkdownSnapshot, path string) (map[string]image.Image, map[string]string) {
	return defaultResourceLoader.PreviewResources(ctx, p, path)
}

func (l *ResourceLoader) PreviewResources(ctx context.Context, p document.MarkdownSnapshot, path string) (map[string]image.Image, map[string]string) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	images := map[string]image.Image{}
	errors := map[string]string{}
	seen := map[string]bool{}
	spans := []document.MarkdownSpan{}
	collect := func(s document.MarkdownSpan) {
		key := ResourceKey(s)
		if seen[key] {
			return
		}
		seen[key] = true
		if len(spans) >= 128 {
			errors[key] = "预览资源超过 128 项"
			return
		}
		spans = append(spans, s)
	}
	for _, b := range p.Blocks {
		if b.Kind == "diagram" {
			collect(DiagramSpan(b))
		}
		if b.Kind == "math" {
			collect(document.MarkdownSpan{Kind: "math", Text: b.Text, DisplayMath: true})
		}
		for _, row := range b.TableSpans {
			for _, cell := range row {
				for _, s := range cell {
					if s.Kind == "image" || s.Kind == "math" {
						collect(s)
					}
				}
			}
		}
		for _, s := range b.Spans {
			if s.Kind == "image" || s.Kind == "math" {
				collect(s)
			}
		}
	}
	type decodedResource struct {
		key string
		img image.Image
		err error
	}
	jobs := make(chan document.MarkdownSpan, len(spans))
	results := make(chan decodedResource, len(spans))
	var batchSpans []document.MarkdownSpan
	var requests []diagramrender.Request
	for _, span := range spans {
		if span.Kind == "diagram" || span.Kind == "math" {
			kind := span.Language
			if span.Kind == "math" {
				kind = "math"
				if !span.DisplayMath {
					kind = "math-inline"
				}
			} else if kind == "" {
				kind = "mermaid"
			}
			batchSpans = append(batchSpans, span)
			requests = append(requests, diagramrender.Request{Kind: kind, Source: []byte(span.Text), Density: 2})
		} else {
			jobs <- span
		}
	}
	close(jobs)
	if len(requests) > 0 {
		go func() {
			renderer, err := l.renderer()
			if err != nil {
				for _, span := range batchSpans {
					results <- decodedResource{key: ResourceKey(span), err: err}
				}
				return
			}
			rendered := renderer.RenderBatch(ctx, requests)
			for i, result := range rendered {
				resource := decodedResource{key: ResourceKey(batchSpans[i]), err: result.Err}
				if result.Err == nil {
					resource.img = document.ResourceImage{Image: result.Result.Image, Size: result.Result.LogicalSize}
				}
				results <- resource
			}
		}()
	}
	for range min(4, len(jobs)) {
		go func() {
			for span := range jobs {
				img, err := l.LoadImage(ctx, span.Destination, path)
				results <- decodedResource{key: ResourceKey(span), img: img, err: err}
			}
		}()
	}
	used := int64(0)
	for range spans {
		result := <-results
		if result.err == nil {
			result.err = ctx.Err()
		}
		if result.err != nil {
			errors[result.key] = result.err.Error()
			continue
		}
		cost := int64(result.img.Bounds().Dx()) * int64(result.img.Bounds().Dy()) * 4
		if used+cost > 64<<20 {
			errors[result.key] = "预览图片缓存超过 64 MiB"
			continue
		}
		used += cost
		images[result.key] = result.img
	}
	return images, errors
}

const markdownImageByteLimit = int64(32 << 20)

var markdownImageHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("图片重定向超过 5 次")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("不支持图片重定向协议：%s", req.URL.Scheme)
		}
		return nil
	},
}

// LoadImage resolves local paths or retrieves HTTP/HTTPS raster images.
// It validates actual bytes and decoded dimensions rather than trusting suffixes.
func LoadImage(ctx context.Context, destination, sourcePath string) (image.Image, error) {
	return defaultResourceLoader.LoadImage(ctx, destination, sourcePath)
}
func (l *ResourceLoader) LoadImage(ctx context.Context, destination, sourcePath string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	u, err := url.Parse(destination)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https") {
		u.Scheme = strings.ToLower(u.Scheme)
		return l.loadHTTP(ctx, u.String(), markdownImageHTTPClient)
	}
	return l.loadLocal(ctx, destination, sourcePath)
}
func loadHTTPImage(ctx context.Context, destination string, client *http.Client) (image.Image, error) {
	return defaultResourceLoader.loadHTTP(ctx, destination, client)
}
func (l *ResourceLoader) loadHTTP(ctx context.Context, destination string, client *http.Client) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, destination, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Glance Markdown Image Preview")
	req.Header.Set("Accept", "image/png,image/jpeg,image/gif,image/svg+xml")
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("加载远程图片：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("加载远程图片：HTTP %d", response.StatusCode)
	}
	if response.ContentLength > markdownImageByteLimit {
		return nil, fmt.Errorf("图片超过 32 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, markdownImageByteLimit+1))
	if err != nil {
		return nil, err
	}
	return l.decode(ctx, data)
}
func LoadLocalImage(destination, sourcePath string) (image.Image, error) {
	return defaultResourceLoader.loadLocal(context.Background(), destination, sourcePath)
}
func (l *ResourceLoader) loadLocal(ctx context.Context, destination, sourcePath string) (image.Image, error) {
	if len(destination) > 2 && destination[1] == ':' && (destination[2] == '\\' || destination[2] == '/') {
		destination = "file:///" + strings.ReplaceAll(destination, "\\", "/")
	}
	u, err := url.Parse(destination)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "" && u.Scheme != "file" {
		return nil, fmt.Errorf("仅加载本地图片：%s", u.Scheme)
	}
	path := u.Path
	if runtime.GOOS == "windows" && u.Scheme == "file" && len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	if u.Scheme == "" {
		path = strings.Split(destination, "#")[0]
		if decoded, e := url.PathUnescape(path); e == nil {
			path = decoded
		}
	}
	if !filepath.IsAbs(path) {
		if sourcePath == "" {
			return nil, fmt.Errorf("请先保存文档，或使用图片绝对路径")
		}
		path = filepath.Join(filepath.Dir(sourcePath), path)
	}
	// Check before opening: opening a FIFO for reading can block indefinitely,
	// so a post-open regular-file check is too late for untrusted Markdown paths.
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("图片必须是普通文件")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("图片必须是普通文件")
	}
	if info.Size() > markdownImageByteLimit {
		return nil, fmt.Errorf("图片超过 32 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, markdownImageByteLimit+1))
	if err != nil {
		return nil, err
	}
	return l.decode(ctx, data)
}
func DiagramSpan(b document.MarkdownBlock) document.MarkdownSpan {
	return document.MarkdownSpan{Kind: "diagram", Language: b.Language, Text: b.Text, SourceStart: b.SourceStart, SourceEnd: b.SourceEnd}
}
