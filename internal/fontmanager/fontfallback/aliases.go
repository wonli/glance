package fontfallback

import (
	"strings"
)

// FamilyCandidates returns prioritized candidate family names for a requested
// font family. The requested family is always returned first, followed by
// metric-compatible substitutes, cross-platform equivalents, and generic fallbacks.
func FamilyCandidates(family string) []string {
	clean := strings.Trim(family, "\\\"' \t\r\n")
	if clean == "" {
		return nil
	}
	normalized := strings.ToLower(clean)
	var substitutes []string
	switch normalized {
	// --- Western Sans-Serif (Modern Office / Web) ---
	case "calibri":
		substitutes = []string{"Calibri", "Carlito", "Arial", "Helvetica Neue", "Helvetica", "Liberation Sans", "sans-serif"}
	case "calibri light":
		substitutes = []string{"Calibri Light", "Calibri", "Carlito", "Helvetica Neue Light", "Helvetica Neue", "Arial", "sans-serif"}
	case "aptos":
		substitutes = []string{"Aptos", "Calibri", "Carlito", "Arial", "sans-serif"}
	case "aptos display":
		substitutes = []string{"Aptos Display", "Aptos", "Calibri Light", "Arial", "sans-serif"}
	case "arial":
		substitutes = []string{"Arial", "Liberation Sans", "Arimo", "Helvetica Neue", "Helvetica", "sans-serif"}
	case "arial black":
		substitutes = []string{"Arial Black", "Impact", "Arial", "sans-serif"}
	case "helvetica", "helvetica neue":
		substitutes = []string{"Helvetica Neue", "Helvetica", "Arial", "Liberation Sans", "sans-serif"}
	case "segoe ui":
		substitutes = []string{"Segoe UI", "SF Pro Text", "SF Pro", "Helvetica Neue", "Arial", "sans-serif"}
	case "sf pro", "sf pro text", "sf pro display":
		substitutes = []string{"SF Pro Text", "SF Pro", "Helvetica Neue", "Arial", "Segoe UI", "sans-serif"}
	case "tahoma":
		substitutes = []string{"Tahoma", "Verdana", "Arial", "sans-serif"}
	case "verdana":
		substitutes = []string{"Verdana", "Tahoma", "Arial", "sans-serif"}
	case "trebuchet ms":
		substitutes = []string{"Trebuchet MS", "Helvetica Neue", "Arial", "sans-serif"}
	case "open sans":
		substitutes = []string{"Open Sans", "Arial", "sans-serif"}
	case "roboto":
		substitutes = []string{"Roboto", "Arial", "sans-serif"}
	case "lato":
		substitutes = []string{"Lato", "Arial", "sans-serif"}
	case "inter":
		substitutes = []string{"Inter", "Arial", "sans-serif"}
	case "sans-serif":
		substitutes = []string{"Arial", "Helvetica Neue", "Helvetica", "sans-serif"}

	// --- Western Serif (Editorial / Academic) ---
	case "times new roman", "times":
		substitutes = []string{"Times New Roman", "Times", "Liberation Serif", "Tinos", "serif"}
	case "cambria":
		substitutes = []string{"Cambria", "Caladea", "Georgia", "Times New Roman", "serif"}
	case "cambria math":
		substitutes = []string{"Cambria Math", "Cambria", "Times New Roman", "serif"}
	case "georgia":
		substitutes = []string{"Georgia", "Cambria", "Times New Roman", "serif"}
	case "garamond":
		substitutes = []string{"Garamond", "Georgia", "Times New Roman", "serif"}
	case "palatino", "palatino linotype", "book antiqua":
		substitutes = []string{"Palatino Linotype", "Palatino", "Book Antiqua", "Georgia", "serif"}
	case "baskerville":
		substitutes = []string{"Baskerville", "Georgia", "Times New Roman", "serif"}
	case "serif":
		substitutes = []string{"Times New Roman", "Times", "Georgia", "serif"}

	// --- Western Monospace (Code / Data) ---
	case "courier new", "courier":
		substitutes = []string{"Courier New", "Courier", "Liberation Mono", "Cousine", "Menlo", "Consolas", "monospace"}
	case "consolas":
		substitutes = []string{"Consolas", "Menlo", "SF Mono", "DejaVu Sans Mono", "Liberation Mono", "Courier New", "monospace"}
	case "menlo":
		substitutes = []string{"Menlo", "SF Mono", "Consolas", "Monaco", "Courier New", "monospace"}
	case "monaco":
		substitutes = []string{"Monaco", "Menlo", "Consolas", "Courier New", "monospace"}
	case "sf mono":
		substitutes = []string{"SF Mono", "Menlo", "Consolas", "monospace"}
	case "jetbrains mono":
		substitutes = []string{"JetBrains Mono", "Menlo", "Consolas", "monospace"}
	case "dejavu sans mono":
		substitutes = []string{"DejaVu Sans Mono", "Menlo", "Consolas", "monospace"}
	case "monospace":
		substitutes = []string{"Menlo", "Consolas", "Courier New", "monospace"}

	// --- CJK Sans-Serif (HeiTi) ---
	case "simhei", "黑体":
		substitutes = []string{"SimHei", "黑体", "Heiti SC", "STHeiti", "Noto Sans CJK SC"}
	case "microsoft yahei", "微软雅黑":
		substitutes = []string{"Microsoft YaHei", "微软雅黑", "PingFang SC", "Hiragino Sans GB", "Noto Sans CJK SC"}
	case "pingfang sc", "苹方", "苹方-简":
		substitutes = []string{"PingFang SC", "苹方", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans CJK SC"}

	// --- CJK Serif (SongTi / MingTi) ---
	case "simsun", "宋体":
		substitutes = []string{"SimSun", "宋体", "Songti SC", "STSong", "Noto Serif CJK SC"}

	// --- CJK Imitation Song (FangSong) ---
	case "fangsong", "fangsong_gb2312", "仿宋", "仿宋_gb2312":
		substitutes = []string{"FangSong", "仿宋", "STFangsong", "Songti SC", "Noto Serif CJK SC"}

	// --- CJK Regular Script (KaiTi) ---
	case "kaiti", "kaiti_gb2312", "楷体", "楷体_gb2312":
		substitutes = []string{"KaiTi", "楷体", "Kaiti SC", "STKaiti", "Songti SC"}
	}

	seen := make(map[string]bool, len(substitutes)+1)
	out := make([]string, 0, len(substitutes)+1)

	// Keep user's exact input case-preserved first.
	seen[normalized] = true
	out = append(out, clean)

	for _, sub := range substitutes {
		normSub := strings.ToLower(sub)
		if !seen[normSub] {
			seen[normSub] = true
			out = append(out, sub)
		}
	}
	return out
}
