# fontmanager

`fontmanager` is a copyable Go source subtree for cataloging, resolving, downloading, and publishing fonts. Its core has no dependency on Glance application, document, workspace, or Gio packages.

## Copying into another project

Copy this entire directory beneath the target module, update the `glance/internal/fontmanager` import prefix to the target module path, and add the dependencies listed below to the target `go.mod`. Keep the `fontcatalog`, `fontdownload`, `fontfallback`, and `builtin` children together. Copy `emojifont` when using its optional Gio adapter. The host application supplies cache and private-resource paths, the system scanner options, locale policy, and a trusted provider through `fontmanager.Options`.

The source subtree is reusable by copying; it is not a separately versioned Go module or a stable cross-module API.

## Toolchain and dependencies

- Go 1.27.1 or newer.
- `github.com/go-text/typesetting` v0.3.5 for OpenType metadata, scanning, and validation.
- `gioui.org` v0.10.3 and `golang.org/x/image` are needed only by the optional `emojifont` adapter; the manager core does not require them.
- On macOS, the system scanner uses CoreText/CoreFoundation when cgo is enabled; with cgo disabled it falls back to the font directories reported by `go-text/typesetting`. Other platforms use those reported system font directories.

The core `fontmanager`, `fontcatalog`, `fontdownload`, `fontfallback`, and `builtin` packages do not require Gio. `emojifont` is an optional Gio adapter inside this subtree; it consumes the normalized bundled emoji bytes from `builtin`. Other renderers can use `builtin.EmojiFont` directly or omit that adapter.

## Ownership

Create one `fontmanager.Manager` per host process and share it across windows and document services. Capture one snapshot for each render, layout, measurement, print, or export task. The manager does not install fonts in the operating system.
