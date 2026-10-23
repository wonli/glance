# Windows emoji implementation and verification (2026-10-23)

Windows Gio text now receives an application-private `Glance Color Emoji` face backed by the embedded Noto Color Emoji CBDT/CBLC font. This avoids relying on Gio v0.10.3 to draw Segoe UI Emoji's COLR paint layers. The upstream revision, source URL, SHA-256 and OFL license are stored in `internal/emojifont/assets`. Parsed immutable font data is shared; shapers receive independent mutable faces. No system font installation or document-font mutation occurs.

The shared shaper factory is used by initial UI construction, catalog-driven replacement, grid auto-fit measurement and proofreading measurement. A Windows-specific family list keeps Latin/CJK preference and places the private face before system emoji fallbacks. macOS retains Apple Color Emoji and its previous GPU regression.

Evidence on the macOS arm64 development host, using the exact Windows collection:

- Ten sample emoji (🚀📑🎨📊🧮📈🎮🔖✨✅), each in normal/heading/code text at 1x/2x, passed actual colored-pixel tests with system fonts disabled.
- Skin-tone ZWJ (👩🏽‍💻), flag (🇨🇳), keycap (1️⃣) and variation-selector (⚠️) sequences produced visible colored glyphs at both scales.
- The pinned font deliberately renders 👨‍👩‍👧‍👦 as a monochrome family silhouette. Its actual image was inspected; regression checks require visible ink and one seven-rune cluster. A requirement for colored pixels alone would incorrectly reject that font design.
- A stable native Gio editor retained an emoji cluster while receiving Chinese text, selected the full cluster and preserved source on navigation. Existing Markdown intermediate-composition regression was rerun with the feature checks.
- Font provenance, bitmap data, private alias, shared-font/independent-face and platform family-order checks passed.

The Windows amd64 application and portable runtime package cross-built successfully. **Native Windows GPU/editor verification has not been performed on this macOS host.** Isolated font tests and successful cross-compilation must not be reported as Windows-native passes. Run the same pixel/editing checks on Windows 10/11 at normal/Retina-equivalent scaling without installing Noto externally.

These results concern Gio text and rasterized diagrams. Word/PDF emoji text uses a separate shaping/output path; no color emoji PDF text compatibility claim is made.
