package markdowndoc

import (
	"bytes"
	"context"
	"fmt"
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
	results := make(chan decodedResource)
	for _, span := range spans {
		jobs <- span
	}
	close(jobs)
	for range min(4, len(spans)) {
		go func() {
			for span := range jobs {
				result := decodedResource{key: ResourceKey(span), err: ctx.Err()}
				if result.err == nil {
					if span.Kind == "math" {
						result.img, result.err = RenderMath(span.Text, span.DisplayMath)
					} else {
						result.img, result.err = LoadImage(ctx, span.Destination, path)
					}
				}
				results <- result
			}
		}()
	}
	used := int64(0)
	for range spans {
		result := <-results
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	u, err := url.Parse(destination)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https") {
		u.Scheme = strings.ToLower(u.Scheme)
		return loadHTTPImage(ctx, u.String(), markdownImageHTTPClient)
	}
	return LoadLocalImage(destination, sourcePath)
}

func loadHTTPImage(ctx context.Context, destination string, client *http.Client) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, destination, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Glance Markdown Image Preview")
	req.Header.Set("Accept", "image/png,image/jpeg,image/gif")
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
		return nil, fmt.Errorf("读取远程图片：%w", err)
	}
	if int64(len(data)) > markdownImageByteLimit {
		return nil, fmt.Errorf("图片超过 32 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("图片格式不支持或数据无效，仅支持 PNG、JPEG 或 GIF：%w", err)
	}
	if format != "png" && format != "jpeg" && format != "gif" {
		return nil, fmt.Errorf("图片格式不支持，仅支持 PNG、JPEG 或 GIF")
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 16<<20 {
		return nil, fmt.Errorf("图片尺寸过大")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("图片数据无效：%w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return img, nil
}
func LoadLocalImage(destination, sourcePath string) (image.Image, error) {
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
	metadata, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !metadata.Mode().IsRegular() {
		return nil, fmt.Errorf("图片必须是普通文件")
	}
	if metadata.Size() > 32<<20 {
		return nil, fmt.Errorf("图片超过 32 MiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 32<<20 {
		return nil, fmt.Errorf("图片超过 32 MiB")
	}
	config, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 16<<20 {
		return nil, fmt.Errorf("图片尺寸过大")
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(f)
	return img, err
}
