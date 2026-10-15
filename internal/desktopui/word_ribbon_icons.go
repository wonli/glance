package desktopui

import (
	"image"
	"image/color"
)

const wordToolbarIconRasterSize = 192

func wordToolbarIconScale(size int) float32 {
	return float32(size) / float32(wordToolbarIconRasterSize)
}

// Word uses its Pen SVGs first and falls back to shared Excel SVGs for
// contextual commands that have no dedicated Word asset.
var wordExcelIconAliases = map[string]string{
	"file-plus": "file-text", "refresh-cw": "save", "printer": "file-text", "file-down": "file-text",
	"share-2": "copy", "info": "file-text", "shield-check": "settings-2", "history": "clock-3",
	"split": "row-insert", "table": "file-spreadsheet", "image": "presentation", "shapes": "grid",
	"chart-no-axes-column": "presentation", "link": "copy", "bookmark": "star", "panel-top": "file-text",
	"panel-bottom": "file-text", "list-ordered": "sort-asc", "text-cursor-input": "type",
	"function-square": "sigma", "omega": "sigma", "layout-grid": "grid", "palette": "paint-bucket",
	"sparkles": "star", "rows-3": "list", "rows-4": "list", "stamp": "star", "square": "grid",
	"panel-left": "file-text", "rectangle-horizontal": "file-text", "columns-2": "grid",
	"indent-increase": "plus", "arrow-up-to-line": "sort-asc", "arrow-down-to-line": "sort-desc",
	"move": "grip-horizontal", "wrap-text": "text-wrap", "align-horizontal-justify-center": "align-center",
	"spell-check": "type", "whole-word": "file-text", "languages": "file-text",
	"message-square-plus": "square-plus", "message-square-x": "trash-2", "highlighter": "paintbrush",
	"check": "save", "x": "clear", "git-compare": "align-distributed", "book-open": "file-text",
	"panels-top-left": "grid", "ruler": "grip-horizontal", "grid-2x2": "grid", "zoom-in": "search",
	"files": "copy", "app-window": "file", "code-xml": "file-code",
}

func wordExcelIconName(name string) string {
	if _, ok := toolbarIconNames[name]; ok {
		return name
	}
	return wordExcelIconAliases[name]
}

func wordRibbonIcon(name string, enabled bool) (image.Image, bool) {
	return tintedToolbarIconAtSize(wordExcelIconName(name), toolbarIconColor(enabled), wordToolbarIconRasterSize)
}

func wordToolbarActionIconWithTint(action wordToolbarAction, tint color.NRGBA) (image.Image, bool) {
	if _, err := svgIconAssets.ReadFile("assets/icons/word/" + action.id + ".svg"); err == nil {
		return svgIconTintAtSize("word", action.id, wordToolbarIconRasterSize, tint)
	}
	return tintedToolbarIconAtSize(wordExcelIconName(action.icon), tint, wordToolbarIconRasterSize)
}

func wordToolbarActionIcon(action wordToolbarAction, enabled bool) (image.Image, bool) {
	return wordToolbarActionIconWithTint(action, toolbarIconColor(enabled))
}

func (a *App) themedWordToolbarActionIcon(action wordToolbarAction, enabled bool) (image.Image, bool) {
	return wordToolbarActionIconWithTint(action, a.toolbarIconColor(enabled))
}
