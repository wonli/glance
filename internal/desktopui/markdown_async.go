package desktopui

import (
	"context"
	"glance/internal/format/markdowndoc"
	"image"
)

func (a *App) requestMarkdownParse() {
	v := a.ensureMarkdownView()
	source, path := v.text(), a.path
	unchanged := v.requested == source && v.requestedPath == path
	v.requested, v.requestedPath = source, path
	if (v.pending && unchanged) || (!v.pending && v.snapshot.Source == source && v.images != nil && v.parsedPath == path) {
		return
	}
	if v.parseCancel != nil {
		v.parseCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.parseCancel = cancel
	v.parseGeneration++
	generation := v.parseGeneration
	v.pending = true
	window := a.window
	a.launchManagedBackground(func() {
		defer cancel()
		done := make(chan struct{})
		defer close(done)
		a.launchManagedBackground(func() {
			select {
			case <-a.shutdown:
				cancel()
			case <-done:
			}
		})
		publish := func(result markdownParseResult) bool {
			select {
			case <-ctx.Done():
				return false
			default:
			}
			select {
			case v.results <- result:
			case <-ctx.Done():
				return false
			}
			if window != nil {
				window.Invalidate()
			}
			return true
		}
		snapshot, err := markdowndoc.Parse(source)
		result := markdownParseResult{source: source, path: path, generation: generation, snapshot: snapshot, images: map[string]image.Image{}, errors: map[string]string{}, err: err, resourcesPending: err == nil}
		if !publish(result) || err != nil {
			return
		}
		result.images, result.errors = markdowndoc.PreviewResourcesContext(ctx, snapshot, path)
		result.resourcesPending = false
		publish(result)
	})
}
