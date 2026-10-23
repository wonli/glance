package markdowndoc

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"glance/internal/diagramrender"
	"glance/internal/document"
	"image"
)

type ResourceLoader struct {
	renderer func() (*diagramrender.Renderer, error)
}

func NewResourceLoader(provider func() (*diagramrender.Renderer, error)) *ResourceLoader {
	return &ResourceLoader{renderer: provider}
}

var defaultResourceLoader = NewResourceLoader(diagramrender.Default)

func DecodeImage(ctx context.Context, data []byte) (image.Image, error) {
	return defaultResourceLoader.decode(ctx, data)
}
func (l *ResourceLoader) render(ctx context.Context, kind string, source []byte) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r, err := l.renderer()
	if err != nil {
		return nil, err
	}
	out, err := r.Render(ctx, diagramrender.Request{Kind: kind, Source: source, Density: 2})
	if err != nil {
		return nil, err
	}
	return document.ResourceImage{Image: out.Image, Size: out.LogicalSize}, nil
}
func (l *ResourceLoader) decode(ctx context.Context, data []byte) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(data)) > markdownImageByteLimit {
		return nil, fmt.Errorf("图片超过 32 MiB")
	}
	if isSVG(data) {
		return l.render(ctx, "svg", data)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("图片格式不支持或数据无效，仅支持 PNG、JPEG、GIF 或 SVG：%w", err)
	}
	if format != "png" && format != "jpeg" && format != "gif" {
		return nil, fmt.Errorf("图片格式不支持，仅支持 PNG、JPEG、GIF 或 SVG")
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 16<<20 {
		return nil, fmt.Errorf("图片尺寸过大")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("图片数据无效：%w", err)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return img, nil
}
func isSVG(data []byte) bool {
	d := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	for n := 0; n < 128; n++ {
		t, err := d.Token()
		if err != nil {
			return false
		}
		switch t := t.(type) {
		case xml.StartElement:
			return t.Name.Local == "svg" && (t.Name.Space == "" || t.Name.Space == "http://www.w3.org/2000/svg")
		case xml.Directive:
			return false
		}
	}
	return false
}
