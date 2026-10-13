package desktopui

import (
	"testing"

	"gioui.org/unit"
)

func TestTypographyScale(t *testing.T) {
	typo := DefaultTypography
	if typo.Display != unit.Sp(34) {
		t.Errorf("Display: got %v, want 34sp", typo.Display)
	}
	if typo.Heading != unit.Sp(17) {
		t.Errorf("Heading: got %v, want 17sp", typo.Heading)
	}
	if typo.Nav != unit.Sp(15) {
		t.Errorf("Nav: got %v, want 15sp", typo.Nav)
	}
	if typo.Title != unit.Sp(14) {
		t.Errorf("Title: got %v, want 14sp", typo.Title)
	}
	if typo.Body != unit.Sp(13) {
		t.Errorf("Body: got %v, want 13sp", typo.Body)
	}
	if typo.Secondary != unit.Sp(12) {
		t.Errorf("Secondary: got %v, want 12sp", typo.Secondary)
	}
	if typo.Caption != unit.Sp(11) {
		t.Errorf("Caption: got %v, want 11sp", typo.Caption)
	}
	if typo.Micro10 != unit.Sp(10) {
		t.Errorf("Micro10: got %v, want 10sp", typo.Micro10)
	}
	if typo.Micro != unit.Sp(9) {
		t.Errorf("Micro: got %v, want 9sp", typo.Micro)
	}
}

func TestRibbonActionLabelTextSizeWithTypography(t *testing.T) {
	typo := DefaultTypography
	cases := []struct {
		label string
		want  unit.Sp
	}{
		{"撤销", unit.Sp(10)},
		{"下划线", unit.Sp(9.5)},
		{"格式刷子", unit.Sp(9)},
		{"五个字超长", unit.Sp(8.5)},
	}
	for _, tc := range cases {
		got := ribbonLabelTextSizeWithTypography(typo, tc.label)
		if got != tc.want {
			t.Errorf("ribbonLabelTextSize(%q) = %v, want %v", tc.label, got, tc.want)
		}
	}
}

func TestDesignTokensResolution(t *testing.T) {
	t.Run("light", func(t *testing.T) {
		tokens := ResolveDesignTokens(ThemeModeLight, DefaultTypography)
		if tokens.IsDark {
			t.Errorf("expected light mode, got IsDark=true")
		}
		if tokens.Colors.Text.Primary != LightColorTokens.Text.Primary {
			t.Errorf("expected light primary text %v, got %v", LightColorTokens.Text.Primary, tokens.Colors.Text.Primary)
		}
		if tokens.Colors.Surface.Canvas != LightColorTokens.Surface.Canvas {
			t.Errorf("expected light canvas %v, got %v", LightColorTokens.Surface.Canvas, tokens.Colors.Surface.Canvas)
		}
	})

	t.Run("dark", func(t *testing.T) {
		tokens := ResolveDesignTokens(ThemeModeDark, DefaultTypography)
		if !tokens.IsDark {
			t.Errorf("expected dark mode, got IsDark=false")
		}
		if tokens.Colors.Text.Primary != DarkColorTokens.Text.Primary {
			t.Errorf("expected dark primary text %v, got %v", DarkColorTokens.Text.Primary, tokens.Colors.Text.Primary)
		}
		if tokens.Colors.Surface.Canvas != DarkColorTokens.Surface.Canvas {
			t.Errorf("expected dark canvas %v, got %v", DarkColorTokens.Surface.Canvas, tokens.Colors.Surface.Canvas)
		}
	})

	t.Run("system fallback", func(t *testing.T) {
		tokens := ResolveDesignTokens(ThemeModeSystem, Typography{})
		if tokens.Typography.Body != unit.Sp(13) {
			t.Errorf("expected fallback to DefaultTypography, got Body=%v", tokens.Typography.Body)
		}
		if tokens.Mode != ThemeModeSystem {
			t.Errorf("expected Mode=system, got %v", tokens.Mode)
		}
	})
}
