package desktopui

import (
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestEveryWordToolbarActionHasSVGAsset(t *testing.T) {
	for tab := wordTabFile; tab <= wordTabView; tab++ {
		for _, action := range wordToolbarActions(tab) {
			if _, err := os.Stat(filepath.Join("assets", "icons", "word", action.id+".svg")); err != nil {
				t.Errorf("Word action %q has no SVG asset: %v", action.id, err)
				continue
			}
			if icon, ok := wordToolbarActionIcon(action, true); !ok || icon == nil {
				t.Errorf("Word action %q did not render from its SVG asset", action.id)
			}
		}
	}
}

func TestEveryBundledSVGIconRendersInGioImageFormat(t *testing.T) {
	for _, family := range []string{"word", "excel"} {
		entries, err := fs.ReadDir(svgIconAssets, "assets/icons/"+family)
		if err != nil {
			t.Fatalf("read %s icon directory: %v", family, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".svg" {
				continue
			}
			name := entry.Name()[:len(entry.Name())-len(filepath.Ext(entry.Name()))]
			icon, err := renderSVGIcon(family, name, 32, nil)
			if err != nil {
				t.Errorf("render %s/%s: %v", family, name, err)
				continue
			}
			if icon.Bounds().Dx() != 32 || icon.Bounds().Dy() != 32 {
				t.Errorf("rendered %s/%s bounds = %v, want 32x32", family, name, icon.Bounds())
			}
		}
	}
}

func TestSVGIconTintKeepsEveryVisiblePixelOneSolidColor(t *testing.T) {
	want := color.NRGBA{R: 13, G: 104, B: 204, A: 255}
	icon, err := renderSVGIcon("excel", "scissors", 32, &want)
	if err != nil {
		t.Fatal(err)
	}
	visible := 0
	for y := 0; y < icon.Bounds().Dy(); y++ {
		for x := 0; x < icon.Bounds().Dx(); x++ {
			got := color.NRGBAModel.Convert(icon.At(x, y)).(color.NRGBA)
			if got.A == 0 {
				continue
			}
			visible++
			if !nearByte(got.R, want.R) || !nearByte(got.G, want.G) || !nearByte(got.B, want.B) {
				t.Fatalf("tinted pixel at (%d,%d) = %v, want pure RGB %v", x, y, got, want)
			}
		}
	}
	if visible == 0 {
		t.Fatal("tinted SVG has no visible pixels")
	}
}

func nearByte(a, b uint8) bool {
	if a > b {
		return a-b <= 1
	}
	return b-a <= 1
}

func TestSVGIconRendererReportsUnknownAssets(t *testing.T) {
	if _, err := renderSVGIcon("excel", "does-not-exist", 24, nil); err == nil {
		t.Fatal("expected an error for an unknown SVG icon")
	}
	if _, err := renderSVGIcon("markdown", "file", 24, nil); err == nil {
		t.Fatal("expected an error for an unknown icon family")
	}
}

func TestEveryExcelToolbarGlyphHasSVGAsset(t *testing.T) {
	for name := range toolbarIconNames {
		assetName, ok := excelIconAssetNames[name]
		if !ok {
			t.Errorf("Excel glyph %q has no SVG asset mapping", name)
			continue
		}
		if _, err := os.Stat(filepath.Join("assets", "icons", "excel", assetName+".svg")); err != nil {
			t.Errorf("Excel glyph %q maps to missing SVG asset %q: %v", name, assetName, err)
		}
	}
}
