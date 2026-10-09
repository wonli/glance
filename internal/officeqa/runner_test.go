package officeqa

import (
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

type fakeRenderResult struct {
	targets     []Target
	renderErr   error
	openErr     error
	diagnostics []string
	stream      bool
}

type fakeRenderer struct {
	results map[string]fakeRenderResult
	opened  []string
}

func (r *fakeRenderer) Open(_ context.Context, path string) (Document, error) {
	name := filepath.Base(path)
	r.opened = append(r.opened, name)
	result := r.results[name]
	if result.openErr != nil {
		return nil, result.openErr
	}
	document := &fakeRenderDocument{result: result}
	if result.stream {
		return &fakeStreamingRenderDocument{fakeRenderDocument: document}, nil
	}
	return document, nil
}

type fakeRenderDocument struct {
	result fakeRenderResult
	closed bool
}

func (d *fakeRenderDocument) Render(_ context.Context, _ Viewport) ([]Target, error) {
	return d.result.targets, d.result.renderErr
}

func (d *fakeRenderDocument) Close() error {
	d.closed = true
	return nil
}

func (d *fakeRenderDocument) Diagnostics() []string {
	return append([]string(nil), d.result.diagnostics...)
}

type fakeStreamingRenderDocument struct {
	*fakeRenderDocument
}

func (d *fakeStreamingRenderDocument) RenderEach(ctx context.Context, _ Viewport, emit func(Target) error) error {
	for _, target := range d.result.targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := emit(target); err != nil {
			return err
		}
	}
	return d.result.renderErr
}

func officeTarget(name string, fill color.NRGBA) Target {
	pixels := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for y := pixels.Bounds().Min.Y; y < pixels.Bounds().Max.Y; y++ {
		for x := pixels.Bounds().Min.X; x < pixels.Bounds().Max.X; x++ {
			pixels.Set(x, y, fill)
		}
	}
	pixels.Set(0, 0, color.NRGBA{R: fill.R ^ 0xff, G: fill.G ^ 0xff, B: fill.B ^ 0xff, A: 255})
	return Target{Name: name, Image: pixels}
}

func writeInput(t *testing.T, root, relative string, content []byte) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func caseWithExpected(entry map[string]any, expected string) map[string]any {
	entry["expected"] = expected
	return entry
}
