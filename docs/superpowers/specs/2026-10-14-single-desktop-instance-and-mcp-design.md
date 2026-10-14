# Single desktop Glance instance and simplified MCP routing

**Status:** Implemented

**Date:** 2026-10-14

## Goal

Allow only one interactive Glance desktop process for the current operating-system user, regardless of which Glance binary copy starts it. A second desktop launch activates the already-running Glance window and exits. It does not send command-line file paths to the running process.

Use that single-runtime guarantee to simplify desktop MCP routing. Keep the stdio bridge and local IPC needed to reach the live desktop `application.Runtime`, but remove per-instance discovery, target selection, executable-path matching, and ambiguity handling.

## User-visible behavior

- The first desktop launch acquires a per-user Glance desktop lock and starts normally. A path passed to this first launch keeps its existing behavior.
- A later desktop launch from the same user, including a launch from another Glance binary copy, does not create another desktop process. It asks the existing process to raise its primary window, then exits.
- The activation request contains no document path. A path supplied only to the duplicate launch is not opened by the existing process.
- Existing Gio windows opened inside the one desktop process remain supported.
- `glance mcp serve --stdio` without `--open` connects to the one live desktop Runtime through one stable, private per-user IPC address. With no desktop Runtime available, it exits with an actionable stderr error and does not create a headless Runtime.
- `glance mcp serve --stdio --open PATH` remains an independent headless workflow and does not acquire the desktop lock.
- The MCP settings page shows one configuration for the current Glance process. It uses the process's actual executable path from `os.Executable()` and `mcp serve --stdio` arguments; it has no instance picker.

## Architecture

### Desktop process ownership

Place the single-instance gate in `internal/desktoplaunch.RunDesktop`, after command dispatch and before Gio or desktop Runtime startup. All desktop entry points already converge on this function. MCP CLI commands are dispatched before it, so they do not compete for the desktop lock.

Use a stable product-level lock identity scoped to the current OS user, not to `os.Executable()` or an install path. Use an OS-held lock for process lifetime: a lock file held with an advisory file lock on macOS/Linux, and a named mutex on Windows. Different Glance copies and versions therefore contend for the same desktop slot without relying on a fixed TCP port.

The process holding the lock owns a private per-user activation endpoint. Use a Unix domain socket on macOS/Linux and a current-user named pipe on Windows. The endpoint accepts only a versioned activation request and acknowledges it after queuing the request for the UI. The listener must be ready before the primary launch is considered available.

If a process cannot acquire the lock, it retries connecting to the activation endpoint while the primary may still be starting. On acknowledgement it exits successfully. If the lock remains held but no endpoint becomes available, it reports a startup error and must not start a second desktop process. If the lock becomes available during the retry window, it retries acquisition and may become the primary process.

The primary queues activation requests until its main window is ready. It then uses the existing window-manager path to show and raise the primary window, including the app-level activation behavior used on macOS. Activation must not run Gio operations from the IPC accept goroutine.

Private runtime directories and endpoint permissions are restricted to the current user. On Unix, the short `/tmp/gm-<uid>` directory is used instead of the process's `TMPDIR`, so launches from Finder and shells share the same lock and sockets even when their temporary-directory environments differ. A Unix endpoint left after a crash may be removed only while holding the singleton lock and after validating that the path is an owned socket in the expected private directory. The OS releases the process lock when the process exits.

### Single desktop MCP endpoint

Replace the per-process random MCP socket plus JSON registry with one stable per-user MCP socket or named pipe. The desktop process starts it against its existing `application.Runtime`; it does not construct another Runtime. The stdio child dials this address directly and relays MCP bytes between stdin/stdout and the desktop connection.

The activation endpoint and MCP endpoint have separate responsibilities and addresses. Activation carries only a small control request; MCP carries the existing protocol stream. This avoids adding activation messages to the MCP capability catalog or mixing control payloads into MCP framing.

Remove desktop target IDs, per-process instance records, path-based matching, and ambiguous-target errors from the active routing flow. Continue accepting the old `--instance-id ID` flag as a deprecated ignored flag for compatibility with configurations generated by the current release; it must never use the ID to select or reject a desktop process. The MCP configuration page emits the new ID-free command.

If the MCP endpoint is missing or cannot be reached, the stdio bridge writes a clear error to stderr and exits without creating an independent Runtime. The explicit `--open PATH` mode retains its current behavior.

### Settings page and documentation

Replace the instance list and selection card with a concise statement that the configuration connects to the current Glance desktop Runtime, followed by the generated JSON and existing connection guidance. Preserve the copy action and display the real running executable path in the JSON. Remove stale multi-instance wording from the active MCP documentation and architecture description. Mark the earlier multi-instance stdio/IPC design as superseded by this design.

## Compatibility and scope

- The singleton is per OS user and covers interactive desktop processes started by builds that implement this lock. Headless MCP children are intentionally excluded.
- No HTTP MCP endpoint or fixed port is reintroduced.
- Existing `--instance-id` desktop configurations continue to work because the flag is accepted and ignored; old HTTP configurations still require replacement.
- A Glance process already running from a pre-singleton build cannot participate in the new activation protocol or lock. Before first launching the singleton build, users must close any already-running older Glance processes once. The new build does not adopt or terminate them.
- This change does not add MCP tools or alter document capabilities, save behavior, or workspace state ownership.

## Acceptance criteria

- Two concurrent desktop launches from the same user, including different Glance executable paths, result in one desktop process; the later launch activates its primary window and exits.
- Repeated desktop launches during primary startup either reach the primary activation endpoint or become the primary only after acquiring the released lock; they never start a second desktop Runtime while the lock is held.
- A duplicate launch does not forward or open its command-line path.
- The one-process rule does not block internal Gio windows, `mcp serve --stdio`, or `mcp serve --stdio --open PATH`.
- The settings page generates an ID-free stdio configuration using the active process's actual executable path.
- A stdio MCP child connects to the current desktop Runtime and can operate on its existing unsaved document sessions. If the desktop endpoint is unavailable, the child fails clearly and does not silently create a headless Runtime.
- Unix sockets and named pipes are private to the current user; orderly shutdown closes endpoints, and the OS releases the singleton lock on process exit.
- No fixed TCP port is needed for desktop MCP or singleton coordination.

## Expected implementation areas

- New focused single-instance lifecycle code, with platform-specific locking and activation endpoint implementations.
- `internal/desktoplaunch`: acquire or activate before starting Gio; preserve pre-desktop MCP CLI dispatch.
- `internal/desktopui`: queue activation requests and raise the primary window through the existing runtime/window manager.
- `internal/mcpserver`: replace per-instance registry/resolution with the stable single desktop endpoint; preserve explicit headless `--open` mode.
- `internal/desktopui/mcp_connection_ui.go` and app state: remove instance selection and generate one ID-free configuration.
- `docs/architecture.md`, `docs/mcp-connection.md`, and the prior multi-instance design status.
