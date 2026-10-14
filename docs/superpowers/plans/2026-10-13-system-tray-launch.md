# System Tray and Portable Desktop Launch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the Pen Glance mark and a dedicated macOS Dock icon, keep Glance resident in the system tray after the main window closes, use one Windows executable for GUI and CLI, and provide a portable macOS `.app` ZIP.

**Architecture:** A shared desktop launcher will serve the Cobra root command and the existing `cmd/glance` entrypoint. A process-level tray controller will send show/quit intents to the existing `desktopRuntime`; closing the last window saves and releases window state while keeping that runtime alive, and tray show creates a fresh window from it. Platform packaging embeds the Glance mark for tray use, builds the Windows root executable as a GUI-subsystem binary with console attachment for CLI commands, and packages a macOS `.app` with a distinct Dock icon in a ZIP.

**Tech Stack:** Go 1.27, Gio 0.10.3, Cobra 1.10.2, a checked-API local fork of `github.com/gogpu/systray` v0.3.0 for Windows/Linux, a custom AppKit backend on Gio's existing macOS event loop, `github.com/godbus/dbus/v5` v5.2.2, `golang.org/x/sys/windows`, macOS `sips`, `iconutil`, and `ditto`.

**Spec:** [2026-10-13-system-tray-launch-design.md](../specs/2026-10-13-system-tray-launch-design.md)

## Global Constraints

- Follow the Gio UI -> desktop runtime -> workspace/session -> adapters architecture in `docs/architecture.md`.
- Keep Gio widgets owned by their window and release them when a window is destroyed.
- Closing the main window keeps the process and tray available; **Quit Glance** is the explicit process-exit action.
- Build one Windows executable with `-H=windowsgui`; CLI commands in it attach to a console and preserve redirected output.
- Keep macOS architecture-specific outputs distinct; do not label an artifact universal without building and verifying both slices.
- Keep all generated bundles and archives under `dist/`.
- Verification uses builds and interactive smoke checks; do not add or run tests unless requested.

## Review Focus

- A double-click launch has no arguments and must show the normal Glance main window without flashing a Windows console. Verify with the packaged GUI executable on Windows.
- Cobra, MCP, and office CLI commands must still print to a terminal and preserve redirected stdout/stderr. Verify terminal and redirection paths on Windows.
- Closing the only window while tray setup fails must not strand an invisible process. Verify the no-StatusNotifierWatcher Linux path.
- Close/show/quit sequences with multiple windows or repeated tray clicks must not lose workspace state, create duplicate primary windows, or stop the runtime early. Verify each sequence on a host with a tray.
- Each macOS ZIP must unpack to one runnable `.app` with the dedicated Pen Dock icon and no required sibling binary. Unpack and launch both advertised architectures on macOS hosts.

---

### Task 1: Share the Desktop and CLI Entry Points

**Files:**
- Create: `internal/desktoplaunch/launch.go`
- Create: `internal/desktoplaunch/cli.go`
- Create: `internal/desktoplaunch/officeqa_renderer.go`
- Modify: `main.go`
- Modify: `cmd/boot.go`
- Modify: `cmd/glance/main.go`
- Delete: `cmd/glance/renderer.go` after moving its implementation.

**Interfaces:**
- Produces: `desktoplaunch.DispatchCLI(args []string, stdout, stderr io.Writer) (handled bool, exitCode int)`; it checks MCP CLI first, then Office QA CLI using the existing headless renderer behavior.
- Produces: `desktoplaunch.RunDesktop(initialPath string)`; it starts `desktopui.Run(initialPath)`, enters `app.Main()`, and exits after the desktop runtime completion signal.
- Consumes: the existing `desktopui.Run(string) <-chan struct{}` and `officeqa.DispatchCLI` APIs.

- [x] **Step 1: Move the current CLI dispatch and renderer into `internal/desktoplaunch`**

Preserve command names, argument handling, exit codes, and renderer error mapping from `cmd/glance/main.go` and `cmd/glance/renderer.go`.

- [x] **Step 2: Route both current entrypoints through the shared launcher**

In `main.go`, run `DispatchCLI` before `cmd.Execute`. In `cmd/glance/main.go`, call `DispatchCLI` and then `RunDesktop`. Keep `dispatchBeforeDesktop` as a thin wrapper around `DispatchCLI` so existing package callers remain compatible. Set Cobra root `RunE` in `cmd/boot.go` to call `RunDesktop` for zero arguments or pass the single document path; reject more than one positional path.

- [x] **Step 3: Build both packages**

Run: `go build . ./cmd/glance`

Expected: both entrypoints compile and there are no import cycles.

- [ ] **Step 4: Manually check launch routing** (not run under the instruction not to run tests)

Run the built root executable without arguments and confirm the normal main window opens; pass a supported document path and confirm it opens; invoke each existing CLI command and confirm its output and exit code are unchanged.

- [x] **Step 5: Commit the shared launcher**

Commit the shared launcher, Cobra route, and `cmd/glance` delegation together so the single-entrypoint change is reviewable as one unit.

---

### Task 2: Embed the Pen Logo and Add the Tray Controller

**Files:**
- Create: `internal/desktopui/assets/brand/glance-mark.png`
- Create: `internal/desktopui/assets/brand/glance-tray-macos-22.png`
- Create: `internal/desktopui/assets/brand/glance-tray-macos-44.png`
- Create: `internal/desktopui/tray_icon.go`
- Create: `internal/desktopui/system_tray.go`
- Create: `internal/desktopui/system_tray_linux.go`
- Create: `internal/desktopui/system_tray_other.go`
- Create: `internal/desktopui/system_tray_darwin.go`
- Create: `internal/desktopui/system_tray_darwin.h`
- Create: `internal/desktopui/system_tray_darwin.m`
- Create: `internal/desktopui/system_tray_types.go`
- Create: `internal/third_party/gogpu-systray/` (checked-API fork of upstream v0.3.0)
- Modify: `go.mod`

**Interfaces:**
- Produces: `trayHostAvailable() error`, implemented per platform; Linux checks for a usable session bus and StatusNotifierWatcher before close-to-tray is enabled.
- Produces: `newSystemTray(actions chan trayAction) (*systemTray, error)` and `(*systemTray).run(ctx context.Context) error`; all platforms start tray initialization in the background after Gio's first rendered frame. Windows/Linux use a locked tray thread, while macOS registers/removes its status item on Gio's existing AppKit main loop.
- Produces: an embedded PNG accessor for the transparent Pen mark from node `aHpKc` in frame `Cgywc`, plus macOS 1×/2× template images from nodes `csn2I` and `w7wlV`.
- Consumes: a checked-API fork of `github.com/gogpu/systray` v0.3.0 on Windows/Linux, a Gio-main-loop-integrated AppKit status item on macOS, and the approved Pen export.

- [x] **Step 1: Add the approved Pen component as the source asset**

Export `aHpKc`, not the full `Cgywc` frame, to `internal/desktopui/assets/brand/glance-mark.png`. Preserve transparency and the full mark proportions.

- [x] **Step 2: Add the systray dependency and embed the icon**

Add `github.com/gogpu/systray` v0.3.0 to `go.mod`; keep a small source fork that exposes native setup errors, which upstream's fluent API suppresses. Read the PNG assets with `//go:embed` and expose bytes for the tray API; do not generate a substitute icon in drawing code.

- [x] **Step 3: Implement tray host detection and callbacks**

Create **Show Glance** and **Quit Glance** menu actions plus left-click show. On macOS, use the monochrome template icon from nodes `csn2I` (22px) and `w7wlV` (44px) as 1×/2× image representations in a 22pt status item; use `bDT83` to verify light/dark inversion. Explicitly activate the app before dispatching Show, and use right-click or Control-click for the context menu. Linux startup must return an error when the session bus or StatusNotifierWatcher is unavailable. Keep the library event loop on a locked OS thread on Windows/Linux and keep callbacks limited to sending `trayAction` values. On macOS, create and remove the retained `NSStatusItem` on Gio's existing AppKit main loop; do not launch a second `NSApplication` loop or change the Dock activation policy.

- [x] **Step 4: Build the desktop packages**

Run: `go build ./internal/desktopui ./internal/desktoplaunch`

Expected: the tray dependency and embedded asset compile for the host platform.

- [ ] **Step 5: Manually verify the tray surface** (not run under the instruction not to run tests)

Confirm the icon is visible, the menu has Show and Quit, left-click requests Show, right-click opens the menu, and quitting removes the icon. On macOS, confirm `csn2I`/`w7wlV` template images adapt to light and dark menu bars as shown in `bDT83`. On Linux without a tray watcher, confirm tray startup reports unavailable rather than claiming residency.

- [x] **Step 6: Commit the logo and tray service**

Commit the Pen asset, embed code, dependency, and tray service together.

---

### Task 3: Keep the Runtime Alive and Restore the Main Window

**Files:**
- Modify: `internal/desktopui/runtime.go`
- Modify: `internal/desktopui/runtime_workspace_persistence.go`
- Modify: `internal/desktopui/runtime_window_manager.go`
- Modify: `internal/desktopui/workspace_tab_lifecycle.go`
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/window_manager.go`

**Interfaces:**
- Produces: a synchronized `desktopRuntime` lifecycle state with tray readiness, quit-requested state, and a bounded tray action channel.
- Produces: `(*desktopRuntime).showMainWindow()`; it raises an existing primary window or creates one attached to the same runtime when none is open.
- Produces: `(*desktopRuntime).requestQuit()`; it marks quit before closing windows so a concurrent Show cannot create another window.
- Consumes: `newSystemTray`, `trayHostAvailable`, `desktopRuntime.attachWindow`, `desktopRuntime.detachWindow`, and current workspace persistence.

- [x] **Step 1: Add tray-aware lifecycle state to `desktopRuntime`**

Guard tray readiness and quit state with the existing runtime mutex. Start the tray before enabling residency; on setup failure leave close behavior as process exit.

- [x] **Step 2: Keep `closeWorkspace` from shutting down a tray-resident runtime**

Continue persisting the active session, draining pending workspace writes, detaching the window, and releasing its view widgets. Run `runtime.shutdown()` only when the last window closes and tray residency is unavailable or an explicit Quit was requested.

- [x] **Step 3: Handle Show and Quit actions exactly once**

With an open window, Show raises the primary window. With no open window, Show creates one `app.Window`, configures it, attaches a fresh `App` to the same runtime, and restores workspace state. Quit blocks later Show requests, closes visible windows, stops the tray, drains runtime workers, and closes completion once.

- [x] **Step 4: Start and stop tray residency with the desktop process**

Start tray initialization after the first Gio frame on every platform so D-Bus discovery and native setup do not delay the main window. On macOS, register the status item on Gio's existing AppKit loop so Gio owns the shared `NSApplication` lifecycle. Connect tray actions to runtime lifecycle handling. If native tray setup fails, preserve close-to-exit behavior instead of leaving an invisible process.

- [x] **Step 5: Build the desktop package**

Run: `go build ./internal/desktopui`

Expected: the window and process lifecycle compile with tray residency enabled.

- [ ] **Step 6: Manually verify close, restore, and quit** (not run under the instruction not to run tests)

Open a document and change its view, close the window, confirm the process and tray remain, select Show, and confirm the workspace/view return. Repeat Show; then choose Quit and confirm a clean process exit. Also close one of multiple windows and confirm remaining windows continue working.

- [x] **Step 7: Commit the runtime lifecycle change**

Commit the tray-residency state and window restoration behavior.

---

### Task 4: Produce One Windows GUI/CLI Executable with Diagnostics

**Files:**
- Create: `internal/desktoplaunch/console_windows.go`
- Create: `internal/desktoplaunch/console_other.go`
- Create: `internal/desktoplaunch/startup_log.go`
- Create: `internal/desktoplaunch/startup_dialog_windows.go`
- Modify: `main.go`
- Modify: `cmd/boot.go`
- Modify: `Makefile`
- Modify: `go.mod`

**Interfaces:**
- Produces: `desktoplaunch.PrepareCLIConsole() error`; on Windows it attaches to the parent console, refreshes standard handles, preserves valid redirected handles, and allocates a console only when no usable output handle exists. Other platforms return without changing stdio.
- Produces: startup logging under `os.UserConfigDir()/Glance/startup.log` and a Windows startup-error dialog for errors before the first rendered frame.
- Consumes: `golang.org/x/sys/windows`, root Cobra routing from Task 1, and tray startup errors from Task 2.

- [x] **Step 1: Attach a console only for CLI invocations**

Detect the CLI command path before `cmd.Execute` or `DispatchCLI`, call `PrepareCLIConsole`, and keep the double-click/no-argument GUI path detached from a console. Do not overwrite inherited redirection when the process already has usable standard handles.

- [x] **Step 2: Add GUI startup diagnostics**

Initialize file logging before Gio/tray startup. Log window and tray initialization errors. On Windows, show a native error dialog if the initial window fails before it renders.

- [x] **Step 3: Update the Windows Makefile target**

Build the packaged root entrypoint `.` as one `dist/glance-windows-amd64-latest.exe` using `-H=windowsgui`; do not create a second CLI executable.

- [x] **Step 4: Cross-build the Windows artifact**

Run: `make windows`

Expected: one `.exe` is produced and the Go linker reports a successful Windows GUI build.

- [ ] **Step 5: Manually verify Windows launch and CLI output**

On Windows, double-click the executable and confirm the main window appears without a console flash. From Command Prompt and PowerShell, run `--help` and existing CLI subcommands; confirm stdout, stderr, exit codes, and redirected output.

- [x] **Step 6: Commit the Windows launcher and build target**

Commit console attachment, startup diagnostics, and the single-executable Windows target.

---

### Task 5: Build Portable macOS App Bundles

**Files:**
- Create: `packaging/macos/Info.plist`
- Create: `packaging/macos/build-icon.sh`
- Create: `packaging/macos/combine-icns.go`
- Create: `internal/desktopui/assets/brand/glance-dock-icon.png`
- Modify: `Makefile`

**Interfaces:**
- Produces: `make macos-app`, which builds `dist/Glance.app` for the host architecture.
- Produces: `make macos-portable`, `make macos-portable-arm64`, and `make macos-portable-amd64`, each producing a distinct ZIP under `dist/` that contains one `Glance.app`.
- Consumes: the Pen PNG source asset and `desktoplaunch.RunDesktop` from Task 1.

- [x] **Step 1: Add the macOS application bundle metadata**

Create `Info.plist` with the Glance bundle name, identifier, executable name, and `CFBundleIconFile=Glance`. Keep the app as a normal Dock application so it can have a visible main window.

- [x] **Step 2: Generate the `.icns` from the dedicated Pen Dock icon**

Use `sips` to create the required iconset sizes from `glance-dock-icon.png` exported from Pen node `pUWMi`, then create `Glance.icns`. Keep `glance-mark.png` embedded for tray use. On this host, `iconutil -c icns` rejected correctly named and sized iconsets (including one it had extracted), so the build uses `sips`-generated ICNS chunks and a standard-library Go helper to assemble the container; `iconutil` successfully extracted the result for validation. Static review found that `icp6` requires 64×64 pixels, so it now uses the generated `icon_32x32@2x.png` resource.

- [x] **Step 3: Add app-bundle and portable ZIP targets**

`macos-app` places the executable, `Info.plist`, and `.icns` under `dist/Glance.app/Contents`. `macos-portable` zips that app with `ditto`; the arm64 and amd64 targets build distinct binaries and archive names. Keep targets parameterized and avoid duplicating packaging recipes.

- [x] **Step 4: Build and package the host architecture**

Run: `make macos-app && make macos-portable`

Expected: a runnable `dist/Glance.app` and a ZIP containing that complete bundle.

- [ ] **Step 5: Manually verify portable extraction** (bundle extraction and metadata inspected; GUI launch and tray interaction not run)

Unzip the package to a clean directory, launch the app, confirm its Dock/Finder icon uses the dedicated `pUWMi` icon and the tray uses the `csn2I`/`w7wlV` template icon, and verify tray show/quit behavior. Build the other architecture target on a compatible macOS toolchain and confirm its archive name reports the actual architecture. The arm64 archive was extracted to a clean temporary directory and its app metadata, icon, and executable architecture were inspected; GUI launch and tray interaction remain unverified.

- [ ] **Step 6: Commit the macOS package targets** (pending; repository `.git` is read-only in this workspace)

Commit the bundle metadata, icon generation, and Makefile targets.

---
