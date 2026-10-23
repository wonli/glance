# fontmanager

`fontmanager` is a copyable Go source subtree for cataloging, resolving, downloading, and publishing fonts. It has no dependency on Glance application, document, workspace, or Gio packages.

## Copying into another project

Copy this entire directory beneath the target module, update the `glance/internal/fontmanager` import prefix to the target module path, and add the dependencies listed below to the target `go.mod`. Keep the `fontcatalog`, `fontdownload`, `fontfallback`, and `builtin` children together. The host application supplies cache and private-resource paths, the system scanner options, locale policy, and a trusted provider through `fontmanager.Options`.

The source subtree is reusable by copying; it is not a separately versioned Go module or a stable cross-module API.

## Toolchain and dependencies

- Go 1.27.1 or newer.
- `github.com/go-text/typesetting` v0.3.5 for OpenType metadata, scanning, and validation.
- `golang.org/x/image` only for the package's existing font tests; it is not required by production packages in this subtree.
- On macOS, the system scanner uses CoreText/CoreFoundation when cgo is enabled; with cgo disabled it falls back to the font directories reported by `go-text/typesetting`. Other platforms use those reported system font directories.

No Gio dependency is required. UI and PDF renderers are host adapters that consume immutable snapshots and the bundled resource bytes.

## Ownership

Create one `fontmanager.Manager` per host process and share it across windows and document services. Capture one snapshot for each render, layout, measurement, print, or export task. The manager does not install fonts in the operating system.
