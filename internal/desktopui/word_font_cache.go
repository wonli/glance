package desktopui

import "glance/internal/wordfont"

func prepareWordFontScanner() error {
	return wordfont.PrepareSystemFontScanner()
}
