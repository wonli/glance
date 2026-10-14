package wordlayout

import (
	"glance/internal/document"
	"testing"
)

func TestScriptFontInheritanceKeepsOtherSlots(t *testing.T) {
	s := document.WordTextStyle{FontFamily: "中文", FontFamilyASCII: "Latin", FontFamilyHighANSI: "Latin", FontFamilyEastAsia: "中文", FontFamilyComplexScript: "Complex"}
	mergeTextStyle(&s, document.WordTextStyle{FontFamily: "新中文", FontFamilyEastAsia: "新中文"}, document.WordTextFontFamilySet|document.WordTextFontEastAsiaSet)
	if s.FontFamilyASCII != "Latin" || s.FontFamilyEastAsia != "新中文" || s.FontFamilyComplexScript != "Complex" {
		t.Fatalf("script inheritance=%+v", s)
	}
	mergeTextStyle(&s, document.WordTextStyle{FontFamily: "Unified"}, document.WordTextFontFamilySet)
	if s.FontFamily != "Unified" || s.FontFamilyASCII != "" || s.FontFamilyEastAsia != "" {
		t.Fatalf("explicit family edit did not replace scripts: %+v", s)
	}
}
