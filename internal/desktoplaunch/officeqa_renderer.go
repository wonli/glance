package desktoplaunch

import (
	"context"
	"errors"
	"fmt"

	"glance/internal/desktopui"
	"glance/internal/officeqa"
)

type headlessOfficeRenderer struct{}

func (headlessOfficeRenderer) Open(ctx context.Context, path string) (officeqa.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	doc, err := desktopui.OpenHeadlessDocument(path)
	if err != nil {
		if errors.Is(err, desktopui.ErrHeadlessUnavailable) {
			return nil, fmt.Errorf("%w: %v", officeqa.ErrRendererUnavailable, err)
		}
		return nil, err
	}
	return &headlessOfficeDocument{document: doc}, nil
}

type headlessOfficeDocument struct {
	document *desktopui.HeadlessDocument
}

func (d *headlessOfficeDocument) Render(ctx context.Context, viewport officeqa.Viewport) ([]officeqa.Target, error) {
	targets, err := d.document.Render(ctx, headlessOptions(viewport))
	converted := make([]officeqa.Target, 0, len(targets))
	for _, target := range targets {
		converted = append(converted, officeqa.Target{Name: target.Name, Image: target.Image})
	}
	return converted, adaptRenderError(err)
}

func (d *headlessOfficeDocument) RenderEach(ctx context.Context, viewport officeqa.Viewport, emit func(officeqa.Target) error) error {
	return adaptRenderError(d.document.RenderEach(ctx, headlessOptions(viewport), func(target desktopui.HeadlessTarget) error {
		return emit(officeqa.Target{Name: target.Name, Image: target.Image})
	}))
}

func (d *headlessOfficeDocument) Close() error { return d.document.Close() }

func (d *headlessOfficeDocument) Diagnostics() []string { return d.document.Diagnostics() }

func headlessOptions(viewport officeqa.Viewport) desktopui.HeadlessRenderOptions {
	return desktopui.HeadlessRenderOptions{Width: viewport.Width, Height: viewport.Height, Scale: viewport.Scale}
}

func adaptRenderError(err error) error {
	if errors.Is(err, desktopui.ErrHeadlessUnavailable) {
		return fmt.Errorf("%w: %v", officeqa.ErrRendererUnavailable, err)
	}
	return err
}
