# Windows Emoji Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render color emoji in Windows Gio text without installing fonts or changing native text editing.

**Architecture:** Embed the CBDT/CBLC Noto font under a private rendering family. Share immutable parsed font data, create thread-local faces, and preserve the fallback whenever Gio shapers are replaced. Keep the tested macOS Apple font behavior.

**Tech Stack:** Go, Gio v0.10.3, go-text, static Noto Color Emoji bitmap font.

**Spec:** `docs/superpowers/specs/2026-10-23-markdown-diagrams-and-windows-emoji-design.md` (approved in chat).

## Global Constraints

- Use the current checkout; preserve existing uncommitted work. Do not create a worktree.
- Bundle the static CBDT/CBLC `NotoColorEmoji.ttf` variant, not the COLRv1 variant.
- Parse outside layout; share immutable data, never a mutable font face across shapers.
- Preserve Latin/CJK font preference, native editor geometry, IME, selection and scrolling.
- Windows amd64 is the initial Windows packaged target. Cross-compilation is not native verification.
- Do not claim color emoji PDF text support from Gio results.

## Review Focus

- A catalog refresh replaces the shaper: emoji must remain available (Task 2).
- A machine has a same-name COLR font: the bundled private alias must win (Task 1).
- Bold/italic or monospace queries must still resolve bitmap emoji (Task 2).
- Closing/reopening multiple windows must not duplicate parsed font memory (Task 1).
- ZWJ, variation selectors and IME interaction must retain valid selection boundaries (Task 3).

---

### Task 1: Embedded bitmap fallback and provenance

**Files:** Create `internal/emojifont/assets/NotoColorEmoji.ttf`, `OFL.txt`, `manifest.json`, `internal/emojifont/font.go`, `font_test.go`. Modify `internal/fontfallback/families.go`.

**Interfaces:** Produce `emojifont.Family = "Glance Color Emoji"`, `emojifont.Collection() ([]font.FontFace, error)`, `emojifont.Preload() error`. `Collection` exposes Gio only within the font adapter, not neutral document data. Each returned face must produce an independent go-text face. Existing `fontfallback.PreferredTypeface() string` adds the private alias on Windows before system emoji families, preserving text-family precedence.

- [ ] **Step 1: Write failing tests** `TestBundledEmojiHasBitmapGlyphs` asserts all of `🚀📑🎨📊🧮📈🎮🔖✨✅` have nonzero nominal glyphs and PNG `GlyphBitmap` data. `TestBundledEmojiUsesPrivateFamily` asserts the descriptor is the private alias. `TestBundledEmojiSharesFontButNotFaces` asserts shared immutable font identity, independent face objects. Verify recorded digest and OFL accompany the embedded bytes.
- [ ] **Step 2: Run RED** `go test ./internal/emojifont ./internal/fontfallback -count=1 -v`; observe missing implementation or unsupported font format.
- [ ] **Step 3: Implement adapter and asset** Select an immutable upstream Noto revision, fetch the bitmap file and license, pin URL/revision/SHA-256 in the manifest before use. Embed bytes, parse once with `sync.Once`, cache success/error, wrap immutable parsed font in a face factory. Add `PreferredTypefaceForOS(goos string) string` for deterministic platform-order tests, with `PreferredTypeface()` delegating to `runtime.GOOS`. Assert Windows order and unchanged macOS behavior in `families_test.go`.
- [ ] **Step 4: Run GREEN** Repeat the package command; PASS requires all glyph data and memory-sharing assertions, not just a readable TTF header.
- [ ] **Step 5: Commit only these files** `feat: bundle bitmap emoji fallback for Windows`; keep earlier unstaged changes out of the commit unless explicitly included and reviewed as part of this feature.

### Task 2: Retain fallback across production shapers

**Files:** Create `internal/desktopui/text_shaper.go`, `text_shaper_test.go`; modify `app.go`, `font_rendering.go`, `grid_auto_fit.go`, `proofread_text_layout.go`. Audit measurement construction separately from display using `rg 'text.NewShaper'` rather than relying only on this list.

**Interfaces:** Consume `emojifont.Collection`. Produce `newTextShaper(collection []font.FontFace) *text.Shaper`, preserving supplied document fonts and adding the bundled fallback on Windows. Tests may call a platform-parameterized internal helper with `"windows"` on macOS; production must use `runtime.GOOS`.

- [ ] **Step 1: Write failing tests** `TestWindowsShaperKeepsEmojiAfterCatalogReplacement` shapes/render-tests before and after the font-render result acceptance path. `TestWindowsEmojiQueriesUseBundledFace` exercises normal/semibold/italic/monospace with an isolated font collection that cannot find Apple Color Emoji. `TestWindowsEmojiDoesNotChangeLatinCJKTypeface` checks surrounding text selection and widths against the original text collection.
- [ ] **Step 2: Run RED** `go test ./internal/desktopui -run '^TestWindows(Shaper|Emoji)' -count=1 -v` with GPU access for pixel tests; observe missing fallback. A GPU SKIP is not a pass.
- [ ] **Step 3: Implement factory and preload** Preload before the window's first layout in managed initialization; background replacement and measurement jobs obtain collections through the same factory. Replace all production `text.NewShaper` call sites, including catalog replacement, grid auto-fit and proofreading measurement. Keep test-only no-system-font constructors unchanged. Report a managed initialization failure rather than parsing inside a frame or silently discarding a corrupt bundled asset.
- [ ] **Step 4: Run GREEN** Repeat targeted tests plus `go test ./internal/desktopui -run '^Test(MarkdownEmojiVisual|UIFont|FontRendering|FontCollection|Windows)' -count=1 -v`; inspect 1x/2x bitmap pixels using the Windows collection on macOS.
- [ ] **Step 5: Commit task files** `fix: preserve Windows emoji in Gio shaper lifecycle`.

### Task 3: Native editing and release evidence

**Files:** Create `internal/desktopui/windows_emoji_visual_test.go`, `windows_emoji_interaction_test.go`, `docs/windows-emoji-verification.md`. Update `docs/office-support.md`, `docs/architecture.md` without overwriting prior edits.

**Interfaces:** Use Task 2's factory and existing stable `widget.Editor` state; no new rendering API.

- [ ] **Step 1: Add regression assertions** Test `👩🏽‍💻`, `👨‍👩‍👧‍👦`, `🇨🇳`, `1️⃣`, `⚠️` alongside the ten existing samples. `TestWindowsEmojiEditorKeepsIMEAndSelection` checks Chinese composition, cancel, cursor/selection around clusters and horizontal scrolling; use synthetic text only. `TestWindowsEmojiWindowReuseSharesFont` verifies repeated shaper/window creation shares parsed immutable data rather than reparsing per frame.
- [ ] **Step 2: Run RED for any newly uncovered sequence/interaction defect** Use `go test ./internal/desktopui -run '^TestWindowsEmoji' -count=1 -v`. Do not manufacture a failure when the earlier implementation already satisfies a newly added characterization test; distinguish those from defect regressions.
- [ ] **Step 3: Correct only demonstrated defects** Keep editor handling native and preserve no-op commit/cancel/IME rules. If a sequence is unsupported by the selected font/version, record the exact sequence and observed result; common specified sequences remain required acceptance, not silently downgraded claims.
- [ ] **Step 4: Verify Windows amd64** Build the existing Windows target, run native GPU/interaction tests at 1x/2x on Windows without installed Noto, and capture results. On macOS, rerun the Apple path. If no Windows runner is available, report the native check as outstanding; do not label macOS isolation or cross-build as Windows verification.
- [ ] **Step 5: Commit evidence and docs** `test: document Windows emoji rendering and editing coverage`. Run `git diff --check`; retain any baseline failures by name in the verification report.
