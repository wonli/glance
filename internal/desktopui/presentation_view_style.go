package desktopui

import "image/color"

const (
	pptxCategoryHeightDP  = 34
	pptxRibbonHeightDP    = 68
	pptxSidebarWidthDP    = 248
	pptxNotesHeightDP     = 76
	pptxStatusBarHeightDP = 34
)

var (
	pptxAccentColor   = color.NRGBA{R: 99, G: 91, B: 255, A: 255}   // #635BFF (Pen spec)
	pptxAccentBg      = color.NRGBA{R: 238, G: 236, B: 255, A: 255} // #EEECFF
	pptxCanvasBg      = color.NRGBA{R: 233, G: 236, B: 241, A: 255} // #E9ECF1 (Pen spec)
	pptxSidebarBg     = color.NRGBA{R: 247, G: 248, B: 250, A: 255} // #F7F8FA (Pen spec)
	pptxBorderColor   = color.NRGBA{R: 225, G: 228, B: 232, A: 255} // #E1E4E8
	pptxTextDark      = color.NRGBA{R: 23, G: 35, B: 60, A: 255}    // #17233C
	pptxTextMuted     = color.NRGBA{R: 123, G: 131, B: 141, A: 255} // #7B838D
	pptxTextSecondary = color.NRGBA{R: 89, G: 97, B: 107, A: 255}   // #59616B
	pptxStatusBg      = color.NRGBA{R: 247, G: 248, B: 250, A: 255} // #F7F8FA
	pptxStatusRule    = color.NRGBA{R: 212, G: 216, B: 222, A: 255} // #D4D8DE
)
