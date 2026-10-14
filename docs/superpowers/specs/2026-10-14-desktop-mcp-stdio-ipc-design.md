# Desktop MCP over stdio and local IPC

**Status:** Draft for user review

**Date:** 2026-10-14

## Goal

Replace Glance's default desktop MCP connection, currently exposed as a bearer-protected loopback HTTP URL, with an MCP client configuration that launches the Glance executable over stdio and forwards requests to a selected running desktop runtime over local IPC.

The desktop runtime remains the owner of its live document sessions. MCP edits must be visible in the selected Glance instance and share its unsaved document state. Starting another Glance instance must not make the MCP connection silently route to the wrong process or fail because both instances bind port `50628`.

## User experience

- The MCP settings page lists live Glance instances. Each row identifies the app copy and process, and may show its active document to help distinguish instances.
- The user selects the target instance and copies a stdio MCP configuration. With one matching instance, Glance selects it automatically. With multiple candidates, Glance requires an explicit choice.
- The configuration's `command` is the selected instance's actual executable path, obtained from the process itself rather than guessed from an installation location. Its arguments include the MCP stdio command and a target instance identifier when needed.
- If a selected process exits, the connector must fail clearly. It may resolve by executable path only when exactly one live instance matches that path; it must never choose arbitrarily among multiple matches.
- The desktop MCP page no longer requires a fixed TCP port or bearer token. Users with an old HTTP MCP entry must replace it with the generated stdio entry.

## Architecture

### Desktop IPC endpoint

Each desktop process registers a short-lived instance record containing a unique runtime instance ID, PID, canonical executable path, and local IPC address. The executable path comes from `os.Executable()` and is normalized before it is used to identify an app copy.

The process serves the existing MCP capability catalog against its existing `application.Runtime` over one IPC connection per MCP client. Use Unix domain sockets on macOS/Linux and a same-user named pipe on Windows. Store socket and registry data in a private per-user runtime directory; use restrictive permissions, remove the socket and record at shutdown, and discard stale records after verifying that their process is no longer live.

The IPC endpoint must not create a second application runtime. It reuses the current desktop runtime so queries, edits, events, and desktop human-confirmation flows retain their existing behavior.

### stdio connector

The generated MCP configuration launches the selected Glance executable with `mcp serve --stdio` and a target instance ID when the executable path alone is ambiguous. The CLI resolves the target through the per-user instance registry, connects to its local socket or pipe, and relays MCP's newline-delimited JSON-RPC stream between the process stdio and the desktop MCP session. Protocol data uses stdout only; diagnostics use stderr.

Keep independent headless MCP available for explicit file work through the existing `--open PATH` flow. A desktop-targeted request that cannot find its target must return an actionable error rather than silently creating a headless runtime.

### Instance selection and lifecycle

The settings page owns target selection and configuration generation. Each listed row maps to one live runtime and its registered IPC endpoint. PID is display/diagnostic metadata, not the sole identity; the runtime instance ID prevents accidental routing between same-path processes. The CLI validates both the requested instance and executable identity before connecting.

If the stored process-specific instance ID is stale after restart, resolution may fall back to the executable path only when there is exactly one matching live runtime. If there are zero matches, return a not-running error. If there are multiple matches, return an ambiguity error and ask the user to copy the configuration for the intended live instance again.

Closing Glance closes its IPC listener and invalidates active stdio connections. The connector exits with a useful stderr diagnostic; it does not keep serving tools against a detached or newly created runtime.

## Compatibility and scope

- Stop automatically starting the fixed-port desktop HTTP endpoint and remove port/token editing from the primary MCP settings page.
- Keep the independent headless stdio workflow for callers that explicitly open paths.
- Existing HTTP MCP client configurations require a one-time replacement. Do not silently proxy the old URL to an arbitrary desktop process.
- Share the existing application capability catalog; do not add MCP tools or document behavior as part of this transport change.

## Validation criteria

- The generated command path is the running Glance executable path, including paths containing spaces.
- A stdio MCP client can list tools and operate on a document already open with unsaved edits in the selected desktop runtime.
- Two different running Glance instances receive distinct IPC addresses; requests sent to one instance do not appear in the other.
- When multiple instances match a command path, omitted or stale target IDs fail with a clear ambiguity error instead of selecting the first registry entry.
- Repeated launches and stale registry records do not leave a port conflict or misroute a request.
- Unix sockets and Windows named pipes are restricted to the current user and cleaned up on orderly shutdown.
- The existing explicit headless `--open PATH` MCP workflow continues to operate independently of any desktop process.

## Expected implementation areas

- `internal/mcpserver`: desktop IPC endpoint lifecycle, per-connection MCP transport, registry lookup, and stdio bridge behavior.
- `internal/desktoplaunch` / `internal/mcpserver/cli.go`: dispatch flags and target resolution while retaining the explicit headless path.
- `internal/desktopui/runtime.go` and runtime shutdown: start, publish, and stop the IPC endpoint.
- `internal/desktopui/mcp_connection_ui.go` and stable app state: enumerate/select target instances and display/copy stdio configuration.
- MCP connection, CLI, lifecycle, and settings UI documentation and regression coverage.
