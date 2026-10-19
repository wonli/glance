package desktopui

import (
	"image"
	"image/color"
)

func findDialogIcon(family, name string, size int, tint color.NRGBA) (image.Image, bool) {
	if name == "" {
		return nil, false
	}
	if family != "" {
		if icon, ok := svgIconTintAtSize(family, name, size, tint); ok {
			return icon, true
		}
	}
	for _, fam := range []string{"confirm", "word", "excel", "pen"} {
		if fam == family {
			continue
		}
		if icon, ok := svgIconTintAtSize(fam, name, size, tint); ok {
			return icon, true
		}
	}
	return nil, false
}
