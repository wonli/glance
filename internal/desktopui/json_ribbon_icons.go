package desktopui

import "image"

const jsonToolbarIconRasterSize = 192

func jsonToolbarActionIcon(action ribbonAction, enabled bool) (image.Image, bool) {
	if _, err := svgIconAssets.ReadFile("assets/icons/json/" + action.id + ".svg"); err == nil {
		return svgIconTintAtSize("json", action.id, jsonToolbarIconRasterSize, toolbarIconColor(enabled))
	}
	if _, err := svgIconAssets.ReadFile("assets/icons/json/" + action.icon + ".svg"); err == nil {
		return svgIconTintAtSize("json", action.icon, jsonToolbarIconRasterSize, toolbarIconColor(enabled))
	}
	if _, err := svgIconAssets.ReadFile("assets/icons/word/" + action.id + ".svg"); err == nil {
		return svgIconTintAtSize("word", action.id, jsonToolbarIconRasterSize, toolbarIconColor(enabled))
	}
	if _, err := svgIconAssets.ReadFile("assets/icons/word/" + action.icon + ".svg"); err == nil {
		return svgIconTintAtSize("word", action.icon, jsonToolbarIconRasterSize, toolbarIconColor(enabled))
	}
	if assetName, ok := excelIconAssetName(action.icon); ok {
		return svgIconTintAtSize("excel", assetName, jsonToolbarIconRasterSize, toolbarIconColor(enabled))
	}
	return nil, false
}
