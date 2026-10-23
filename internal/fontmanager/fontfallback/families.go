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

// PreferredFamilies returns the exact ordered family list used by Glance's
// existing UI theme and document fallback chain.
func PreferredFamilies() []string { return PreferredFamiliesForOS(runtime.GOOS) }

func PreferredFamiliesForOS(goos string) []string {
	families := append([]string{"Inter"}, cjkFamiliesForOS(goos)...)
	if goos == "windows" {
		families = append(families, "Glance Color Emoji")
	}
	families = append(families, "Apple Color Emoji", "Segoe UI Emoji", "Noto Color Emoji", "emoji")
	return families
}

// PreferredTypefaceForOS selects platform rendering fallbacks without installing fonts.
func PreferredTypefaceForOS(goos string) string {
	return strings.Join(PreferredFamiliesForOS(goos), ", ")
}

// WebViewTypeface preserves the existing diagram renderer's family order.
// Keep browser-backed SVG text policy here rather than in a renderer.
func WebViewTypeface() string { return "Arial,Microsoft YaHei,PingFang SC,sans-serif" }

// MonospaceFamilies keeps code-editor text on Glance's existing preferred
// monospace chain while allowing the manager to preload those faces.
func MonospaceFamilies() []string { return MonospaceFamiliesForOS(runtime.GOOS) }

func MonospaceFamiliesForOS(goos string) []string {
	platform := "DejaVu Sans Mono"
	switch goos {
	case "windows":
		platform = "Consolas"
	case "darwin":
		platform = "Menlo"
	}
	return []string{"JetBrains Mono", platform, "Liberation Mono", "Courier New", "monospace"}
}
