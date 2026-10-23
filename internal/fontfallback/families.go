// Package fontfallback provides platform-prioritized system font families.
package fontfallback

import (
	"runtime"
	"strings"
)

// CJKFamilies returns the preferred CJK families for this operating system,
// followed by shared fallbacks for systems where the preferred fonts are absent.
func CJKFamilies() []string { return cjkFamiliesForOS(runtime.GOOS) }

func cjkFamiliesForOS(goos string) []string {
	var platform []string
	switch goos {
	case "windows":
		platform = []string{"Microsoft YaHei"}
	case "darwin":
		platform = []string{"PingFang SC", "Hiragino Sans GB"}
	default:
		platform = []string{"Noto Sans CJK SC", "WenQuanYi Micro Hei"}
	}

	candidates := append(platform,
		"Noto Sans CJK SC",
		"WenQuanYi Micro Hei",
		"Noto Sans",
		"Arial",
		"Helvetica",
		"sans-serif",
	)
	seen := make(map[string]struct{}, len(candidates))
	families := make([]string, 0, len(candidates))
	for _, family := range candidates {
		key := strings.ToLower(family)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		families = append(families, family)
	}
	return families
}

// PreferredTypeface keeps Inter first for Latin UI text, then adds platform
// CJK families, shared fallbacks and explicit color emoji families.
func PreferredTypeface() string { return PreferredTypefaceForOS(runtime.GOOS) }

// PreferredTypefaceForOS selects platform rendering fallbacks without installing fonts.
func PreferredTypefaceForOS(goos string) string {
	families := append([]string{"Inter"}, cjkFamiliesForOS(goos)...)
	// Gio's resolver does not forward the segment script to fontscan. Its
	// script-only fallback can therefore miss emoji, particularly at non-normal
	// weights. Explicit families let fontscan choose their available face.
	if goos == "windows" {
		families = append(families, "Glance Color Emoji")
	}
	families = append(families, "Apple Color Emoji", "Segoe UI Emoji", "Noto Color Emoji", "emoji")
	return strings.Join(families, ", ")
}
