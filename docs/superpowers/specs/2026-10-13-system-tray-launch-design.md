# System Tray Residency and Desktop Launch Design

## Goal

Use the Glance mark from Pen node `aHpKc` (inside frame `Cgywc`) as the desktop and tray identity. Let users reopen Glance from a system tray icon after closing its window, make Windows double-click launch reliable, preserve command-line tools in the same Windows executable, and provide a portable macOS app package.

The implementation follows the existing Gio -> desktop runtime -> workspace/session architecture. The sibling `/Users/mima1234/project/photo/photo` is a reference for `github.com/gogpu/systray` setup and Windows GUI linking, not a source of Glance product assets.

## User-visible behavior

- Starting the desktop application without a document path opens the workspace window. Starting it with a supported document path continues to open that document.
- The tray icon uses the Pen Glance mark. Clicking the icon or choosing **Open Glance** opens the workspace, or raises the already-open workspace window.
- Closing the last workspace window persists its current workspace and view state, closes window-owned work, and leaves the tray and desktop runtime available. It does not terminate the process.
- Choosing **Quit Glance** is the explicit process-exit action. It closes any open windows, drains workspace persistence, stops the tray, and shuts down runtime-owned services.
- If the platform has no usable tray host, closing the last window exits normally instead of leaving an invisible process. The launch diagnostics record why tray residency was unavailable.
- Windows users can double-click one GUI-subsystem executable. Explicit CLI commands remain in that same executable and attach to a usable console so stdout/stderr stay visible.
- macOS users can unzip a self-contained `.app` bundle and run it without an installer. The bundle contains the app icon and embedded resources.

## Design

### Logo asset

Export the symbol component `aHpKc` rather than the entire `Cgywc` presentation frame. Keep the transparent artwork and include it as an embedded application asset. Use the same source for the app identity and tray icon, scaling or converting at the asset boundary for platform requirements. Do not draw a replacement mark in code.

### Tray service and lifecycle

Add a focused tray/lifecycle boundary around `github.com/gogpu/systray`, following the sibling photo app's locked-OS-thread and callback pattern. The menu contains **Open Glance**, a separator, and **Quit Glance**; a tray click also opens Glance. Tray callbacks send lifecycle intents and do not mutate Gio widget state directly.

The desktop process owns one `desktopRuntime` for its lifetime. Closing a window still runs the existing workspace persistence and per-window cleanup. When no windows remain and tray residency is active, the runtime must not run its final shutdown path. An Open intent creates a new Gio window attached to that runtime so the saved workspace and live sessions can be restored. A Quit intent is distinct from a window close: it disables residency, closes all windows, and then shuts down the runtime and tray exactly once.

Initialize tray availability before enabling close-to-tray behavior. On Linux, use the supported StatusNotifierItem host; if the required session bus/host is unavailable, disable residency and retain the normal close-exits behavior. On Windows and macOS, use the library's native tray support. Tray setup or runtime errors must be visible in startup diagnostics, not silently leave a background-only process.

### Startup and Cobra behavior

The Cobra root command is also a desktop launch path: its no-argument `Run` opens the workspace instead of printing help, and it accepts at most one positional document path for path-based launch. Explicit CLI subcommands retain their current behavior. This makes a shell or Explorer launch with no arguments useful without requiring a special `gui` argument; the Windows GUI artifact uses the same root entrypoint.

Build one Windows executable from the Cobra desktop entrypoint with the Windows GUI subsystem flag (`-H=windowsgui`) so double-clicking does not flash a console. The no-argument root command opens the workspace; explicit Cobra, MCP, and office CLI commands continue to run in this same binary. Before running a CLI command, attach to the parent console and refresh standard handles; honor inherited redirection and allocate a console only when no usable output destination exists. This uses the Windows console APIs intended for GUI applications that need console I/O. The existing `cmd/glance` dispatch must be folded into or delegated by this single packaged entrypoint so users do not need a second executable. Record GUI startup failures to a per-user Glance log and show a Windows startup error dialog when the GUI cannot initialize, since a GUI-subsystem process has no console for diagnostics.

For macOS, build a standard `Glance.app` bundle with `Info.plist`, executable, and a multi-resolution `.icns` made from the same Pen logo. Add Makefile targets for building the app bundle and a portable ZIP containing that bundle, with arm64 and amd64 variants. The ZIP is the distribution artifact: users can extract it and launch the `.app`; no DMG, installer, or second helper binary is required. Keep architecture-specific outputs distinct rather than claiming a universal binary unless both slices are actually built and verified.

## State and error handling

- Workspace and per-window state are persisted through the existing runtime/session lifecycle before a window detaches.
- Reopening from the tray creates a fresh window and Gio-owned widgets; it does not reuse widgets from a destroyed window.
- Repeated tray Open clicks are idempotent: raise an existing window or create at most one new primary window.
- Quit is idempotent and must not race a simultaneous Open request into creating a window after shutdown begins.
- If a window fails to start, report the error and leave the tray available for retry. If tray startup fails, do not promise residency.
- Runtime shutdown, tray removal, and managed background worker draining each happen at most once.

## Scope boundaries

- No startup-at-login registration, notification center, tray status indicators, or tray submenu beyond Open and Quit.
- No changes to document formats, editing behavior, or session architecture beyond lifecycle hooks needed to retain and reopen the existing runtime.
- No replacement of Gio or the current document/workspace UI.

## Verification criteria

- The exported Pen mark is visibly rendered in app/tray assets at normal and high-density sizes with transparency intact.
- Closing the last window with an available tray keeps the process alive; tray click and **Open Glance** reopen the workspace; **Quit Glance** exits after clean shutdown.
- Repeated Open/Quit and close/open races do not create duplicate windows, leak runtime workers, or lose the last persisted workspace state.
- Without a Linux tray host, closing the final window exits and a diagnostic explains the fallback.
- The single Windows executable starts the workspace by double-click without a console flash and preserves command output for CLI commands from a terminal, including redirected output.
- The macOS Makefile targets produce a runnable `.app` and a portable ZIP for each advertised architecture; the extracted app shows the Pen logo in Finder/Dock and the tray.
- Existing CLI subcommands and path-based desktop launch remain intact.

## Approaches considered

1. **Keep the existing Gio window hidden after close.** This appears simple but depends on platform-specific hide/restore behavior, complicates Wayland, and retains destroyed-window widget state.
2. **Destroy windows while retaining the desktop runtime and recreate them on tray open (recommended).** This follows the existing window/session boundaries, runs normal per-window cleanup, works without relying on minimize/raise support for a hidden window, and preserves runtime sessions.
3. **Run tray as a separate helper process.** This isolates OS tray code but adds IPC, process coordination, and packaging complexity for a single-window workspace.

Approach 2 is selected. The tray is process-wide; Gio widgets remain window-owned. For packaging, one Windows executable with GUI-subsystem console reattachment is selected over separate GUI/CLI executables. The macOS portable deliverable is a ZIP containing one `.app` bundle.
