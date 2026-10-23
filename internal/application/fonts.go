package application

import "glance/internal/fontmanager"

// These aliases keep existing application embedders source-compatible while
// font lifecycle ownership moves to the reusable fontmanager package.
type FontScanner = fontmanager.FontScanner
type FontOptions = fontmanager.Options
type FontEvent = fontmanager.Event
type FontService = fontmanager.Manager

func NewFontService(options FontOptions) *FontService {
	return fontmanager.New(options)
}
