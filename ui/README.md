# Reusable Gio UI controls

The public `glance/ui` package provides small Gio controls with application-owned styles and resources. It depends on Gio and the Go standard library; applications provide translated labels, colors, persistent widget state, and icon layout callbacks.

`IconTextButton` and `NavItem` receive a stable `*widget.Clickable`. `TextField` receives the stable `*widget.Editor`; callers also retain any widgets used by its suffix callback. Keep these objects in long-lived application state and pass the same instance on every frame. Hover, selected, focused, and disabled states are explicit component inputs where the component cannot infer application intent.

Vertical offsets are `unit.Dp` values relative to the control's geometric center. Positive values move rendered content downward. Components convert offsets with the current Gio metric and leave reported geometry and pointer bounds unchanged. A zero value is an explicit zero offset.

`OpticalPlacement` is the shared placement primitive used by controls and application layouts. Set `OffsetY` to translate drawing without changing layout geometry, or `TopInset` to add real layout space before the child. Both values are in dp and may be combined. Platform and component profiles remain application-owned and should pass explicit values into the UI package.

The package does not own navigation, form validation, credentials, document state, localization, or icon assets. Icon callbacks should only lay out and draw. Keep brand artwork with the application when it needs multiple independently colored layers.

The `TextField` reserves `SuffixWidth` from the editor's pointer/focus area. Its suffix callback owns its own interaction target, while clicks in the remaining field area focus the supplied editor.

Run `go run ./cmd/ui-gallery` from the repository root to compare light and dark palettes, Chinese and English labels, masked and empty editors, and zero versus optical text offsets.
