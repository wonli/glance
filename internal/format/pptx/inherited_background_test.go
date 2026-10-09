package pptx

import (
	"image/color"

	"strings"
	"testing"
)

func TestPPTXBackgroundReferenceUsesThemeFillStyle(t *testing.T) {
	red := encodeTestRaster(t, "png", color.NRGBA{R: 255, A: 255})
	theme := func(fillStyles, bgFillStyles string) []byte {
		return []byte(`<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `"><a:themeElements><a:clrScheme name="test"><a:dk1><a:srgbClr val="111111"/></a:dk1><a:lt1><a:srgbClr val="FFFFFF"/></a:lt1><a:accent1><a:srgbClr val="445566"/></a:accent1></a:clrScheme><a:fmtScheme name="test"><a:fillStyleLst>` + fillStyles + `</a:fillStyleLst><a:lnStyleLst/><a:effectStyleLst/><a:bgFillStyleLst>` + bgFillStyles + `</a:bgFillStyleLst></a:fmtScheme></a:themeElements></a:theme>`)
	}
	partsFor := func(idx, referenceColor, fillStyles, bgFillStyles string, imageRel bool) map[string][]byte {
		parts := backgroundFixtureParts(`<p:bg><p:bgRef idx="`+idx+`"><a:schemeClr val="`+referenceColor+`"/></p:bgRef></p:bg>`, "", "", nil, nil)
		parts["ppt/theme/theme1.xml"] = theme(fillStyles, bgFillStyles)
		if imageRel {
			parts["ppt/theme/_rels/theme1.xml.rels"] = []byte(`<Relationships xmlns="` + testPackageRelNS + `"><Relationship Id="rIdImage" Type="` + testOfficeRelNS + `/image" Target="../assets/theme-bg.png"/></Relationships>`)
			parts["ppt/assets/theme-bg.png"] = red
		}
		return parts
	}

	t.Run("solid fill style wins over reference color", func(t *testing.T) {
		parts := partsFor("1", "accent1", `<a:solidFill><a:srgbClr val="123456"/></a:solidFill>`, "", false)
		if got := openFirstPPTXSlide(t, parts).BgColorHex; got != "#123456" {
			t.Fatalf("background = %q, want selected fill style #123456", got)
		}
	})
	t.Run("placeholder color is substituted", func(t *testing.T) {
		parts := partsFor("1", "accent1", `<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>`, "", false)
		if got := openFirstPPTXSlide(t, parts).BgColorHex; got != "#445566" {
			t.Fatalf("background = %q, want reference placeholder color #445566", got)
		}
	})
	t.Run("background image style uses theme relationship", func(t *testing.T) {
		parts := partsFor("1001", "accent1", "", `<a:blipFill><a:blip r:embed="rIdImage"/><a:stretch><a:fillRect/></a:stretch></a:blipFill>`, true)
		got := openFirstPPTXSlide(t, parts)
		if got.BackgroundImage == nil {
			t.Fatal("theme background image was not projected")
		}
		pixel := color.NRGBAModel.Convert(got.BackgroundImage.At(0, 0)).(color.NRGBA)
		if pixel.R < 220 || pixel.B > 20 {
			t.Fatalf("theme image pixel = %#v, want red image resolved from theme relationship", pixel)
		}
	})
	t.Run("zero index is no fill", func(t *testing.T) {
		parts := partsFor("0", "accent1", `<a:solidFill><a:srgbClr val="123456"/></a:solidFill>`, "", false)
		if got := openFirstPPTXSlide(t, parts).BgColorHex; got != "#FFFFFF" {
			t.Fatalf("background = %q, want default white for no-fill style", got)
		}
	})
}

func TestPPTXBackgroundUsesSlideThenLayoutThenMasterFallback(t *testing.T) {
	red := encodeTestRaster(t, "png", color.NRGBA{R: 255, A: 255})
	parts := backgroundFixtureParts(
		backgroundImageFill("rId2"),
		backgroundSolidFill("00FF00"),
		backgroundSolidFill("0000FF"),
		[]testImageRel{{id: "rId2", target: "../assets/slide-background.png"}},
		map[string][]byte{"ppt/assets/slide-background.png": red},
	)
	slide := openFirstPPTXSlide(t, parts)
	if img := slide.BackgroundImage; img == nil {
		t.Fatal("explicit slide background image was not projected")
	}

	parts = backgroundFixtureParts("", backgroundSolidFill("00FF00"), backgroundSolidFill("0000FF"), nil, nil)
	slide = openFirstPPTXSlide(t, parts)
	if slide.BgColorHex != "#00FF00" {
		t.Fatalf("layout fallback background = %q, want #00FF00", slide.BgColorHex)
	}

	parts = backgroundFixtureParts("", "", backgroundSolidFill("0000FF"), nil, nil)
	slide = openFirstPPTXSlide(t, parts)
	if slide.BgColorHex != "#0000FF" {
		t.Fatalf("master fallback background = %q, want #0000FF", slide.BgColorHex)
	}

	parts = backgroundFixtureParts(backgroundSolidFill("112233"), backgroundSolidFill("00FF00"), backgroundSolidFill("0000FF"), nil, nil)
	slide = openFirstPPTXSlide(t, parts)
	if slide.BgColorHex != "#112233" {
		t.Fatalf("explicit slide background = %q, want #112233", slide.BgColorHex)
	}
}

func TestPPTXBackgroundWithUnsupportedMediaHasNoImage(t *testing.T) {
	parts := backgroundFixtureParts(
		backgroundImageFill("rId2"), "", "",
		[]testImageRel{{id: "rId2", target: "../media/broken.png"}},
		map[string][]byte{
			"ppt/media/broken.png":    []byte("not a PNG"),
			"ppt/media/unrelated.png": encodeTestRaster(t, "png", color.NRGBA{G: 255, A: 255}),
		},
	)
	if img := openFirstPPTXSlide(t, parts).BackgroundImage; img != nil {
		t.Fatal("unsupported background media borrowed an unrelated cached image")
	}
}

func TestPPTXBackgroundResolvesThemeTx1AndBg1ThroughMasterClrMap(t *testing.T) {
	parts := backgroundFixtureParts(
		backgroundSolidFill("scheme", "tx1"),
		backgroundReferenceFill("bg1"),
		"",
		nil,
		nil,
	)
	master := string(parts["ppt/slideMasters/slideMaster1.xml"])
	defaultColorMap := `<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>`
	customColorMap := `<p:clrMap bg1="dk1" tx1="accent1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>`
	if !strings.Contains(master, defaultColorMap) {
		t.Fatal("master fixture did not contain the default color map")
	}
	parts["ppt/slideMasters/slideMaster1.xml"] = []byte(strings.Replace(master, defaultColorMap, customColorMap, 1))

	slide := openFirstPPTXSlide(t, parts)
	if slide.BgColorHex != "#4472C4" {
		t.Fatalf("tx1 theme mapping = %q, want #4472C4 through master clrMap", slide.BgColorHex)
	}

	parts = backgroundFixtureParts(backgroundReferenceFill("bg1"), "", "", nil, nil)
	master = string(parts["ppt/slideMasters/slideMaster1.xml"])
	parts["ppt/slideMasters/slideMaster1.xml"] = []byte(strings.Replace(master, defaultColorMap, customColorMap, 1))
	slide = openFirstPPTXSlide(t, parts)
	if slide.BgColorHex != "#000000" {
		t.Fatalf("bg1 background reference mapping = %q, want #000000 through master clrMap", slide.BgColorHex)
	}
}

func TestPPTXBackgroundUsesExactOwningPartRelationship(t *testing.T) {
	red := encodeTestRaster(t, "png", color.NRGBA{R: 245, A: 255})
	blue := encodeTestRaster(t, "png", color.NRGBA{B: 245, A: 255})
	parts := backgroundFixtureParts(
		backgroundImageFill("rId2"), "", "",
		[]testImageRel{
			{id: "rId2", target: "../assets/background.png"},
			{id: "rId3", target: "../media/unrelated.png"},
		},
		map[string][]byte{
			"ppt/assets/background.png": red,
			"ppt/media/unrelated.png":   blue,
		},
	)
	img := openFirstPPTXSlide(t, parts).BackgroundImage
	if img == nil {
		t.Fatal("background image was not resolved through its owning slide relationship")
	}
	got := color.NRGBAModel.Convert(img.At(img.Bounds().Min.X, img.Bounds().Min.Y)).(color.NRGBA)
	if got.R < 220 || got.B > 20 {
		t.Fatalf("background resolved wrong media: got %#v, want red background.png", got)
	}
}
