# Reusable Gio UI Components Implementation Plan

> **For agentic workers:** This plan is being executed natively in the current session as requested by the user. Track completed work with checkboxes.

**Goal:** Extract reusable Gio controls into a root `ui` package and migrate the first Glance button, text field, and navigation use cases while preserving their layout and interaction behavior.

**Architecture:** `internal/desktopui` adapts Glance themes and icon assets into explicit styles for `ui`; `ui` depends only on Gio and the standard library. Components receive stable Gio widgets and render callbacks, and accept text/icon offsets in dp as already-resolved style values.

**Tech Stack:** Go 1.27, Gio v0.10.3, existing Glance desktop UI and localizations.

**Spec:** `docs/superpowers/specs/2026-10-19-reusable-gio-ui-design.md`

## Global Constraints

- Keep the package public at import path `glance/ui`; do not create a separate module yet.
- Public package dependencies are limited to Gio and the Go standard library.
- Keep persistent `widget.Clickable` and `widget.Editor` instances in stable caller-owned state.
- Express visual offsets as `unit.Dp`, with positive vertical values moving content down.
- Preserve whole-field focus and independent trailing-button hit areas.
- Do not claim Windows visual calibration without native Windows evidence.
- Keep API-key values out of gallery fixtures, logs, and screenshots.

## Review Focus

- Zero offsets must remain distinguishable from unspecified values; `ui` receives concrete styles and must never replace zero with a default.
- Child editor natural height must not shrink the input hit target; the whole field remains focusable.
- API-key preview may switch which stable Editor is visible; focus must target that exact editor.
- Trailing controls must receive clicks without the outer focus region stealing them.
- Layout callbacks for icons must stay resource-owned by Glance and must not apply a global tint to layered brand artwork.

---

### Task 1: Add the public `ui` package primitives

**Files:**
- Create: `ui/style.go`
- Create: `ui/icon_text_button.go`
- Create: `ui/nav_item.go`
- Create: `ui/text_field.go`
- Create: `ui/README.md`
- Create: `internal/desktopui/optical_compensation.go`
- Modify: `docs/architecture.md`

**Interfaces:**
- `IconTextButtonStyle` supplies geometry, text style/colors, state colors, gaps, corner radius and concrete `TextOffsetY`/`IconOffsetY` values.
- `IconTextButton` receives `*widget.Clickable`, label, `layout.Widget` icon callback, and style; `Layout(gtx, theme)` draws it.
- `NavItemStyle` supplies row geometry, icon size, text style, state colors and offsets; `NavItem` receives stable clickable, selected state and icon callback.
- `TextFieldStyle` supplies frame geometry, insets, fill/border/text/hint colors, text size and `TextOffsetY`.
- `TextField` receives a stable `*widget.Editor`, optional suffix `layout.Widget`, and a caller-owned `*widget.Clickable` for suffix actions. Layout registers the full field focus area while keeping suffix bounds independent.

- [x] Define style values and exported component structs with only Gio/stdlib imports.
- [x] Implement natural-height labels inside fixed parent geometry; apply explicit dp offsets using Gio operations.
- [x] Implement hover/selected/disabled state rendering with selected precedence.
- [x] Implement field pointer focus against the visible editor and carve suffix bounds out of the outer focus target.
- [x] Document caller-owned state lifecycle and offset direction in README; document package dependency direction in architecture docs.

### Task 2: Migrate settings navigation and AI settings fields

**Files:**
- Modify: `internal/desktopui/home_layout.go`
- Modify: `internal/desktopui/agent_settings_page.go`
- Create: `internal/desktopui/optical_compensation.go`

**Interfaces:**
- Glance adapters construct component styles from `a.tokens()`, existing `home*` colors and `homeTextOpticalCenterTopInset`.
- Glance passes `homeIcon` callbacks, localized labels, stable state widgets and the currently visible API-key editor.

- [x] Add GoOS-selected compensation profiles, preserving current 3dp text movement until native Windows evidence supports a change.
- [x] Migrate settings navigation rows and matching home navigation rows without moving page-switch event processing into widgets.
- [x] Migrate API URL, model and API-key fields while retaining 44dp geometry, hint behavior, preview editor selection and eye-button hit box.
- [x] Keep fonts, language picker, formula field, confirmation dialogs, home filters and Ribbon on their current local implementations.

### Task 3: Migrate the AI confirmation button

**Files:**
- Modify: `internal/desktopui/agent_confirmation_modal.go`
- Modify: `internal/desktopui/agent_confirmation_modal_test.go` (only if a targeted regression is needed and explicitly authorized)

**Interfaces:**
- Glance supplies `IconTextButtonStyle`, existing stable button state, icon callback and localized label.

- [x] Replace only the modal's custom icon/text optical placement with `ui.IconTextButton`.
- [x] Preserve text-only variant, icon dimensions, six-dp gap, colors, border, hover behavior and button size.

### Task 4: Add the component gallery command

**Files:**
- Create: `cmd/ui-gallery/main.go`
- Create: `cmd/ui-gallery/gallery.go`

**Interfaces:**
- The gallery imports only the public `glance/ui` component APIs plus Gio and standard library.
- Use synthetic English, Chinese, placeholder, masked, long-text and state examples.

- [x] Build a standalone Gio window showing light and dark palettes for button, field and navigation item.
- [x] Provide a simple control to switch between named non-zero offsets and zero offsets.
- [x] Keep all example credentials synthetic and never log widget contents.

### Task 5: Review implementation and build

**Files:**
- Review all created and modified files in Tasks 1–4.

- [x] Confirm `ui` imports only Gio and standard library.
- [x] Run `gofmt` on changed Go files and `git diff --check`.
- [x] Build `./ui`, `./cmd/ui-gallery`, and `./internal/desktopui` packages without running tests.
- [x] Report that tests and native Windows/Metal visual checks were not run in this pass; do not claim those validations.
