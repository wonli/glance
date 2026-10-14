# Desktop MCP stdio and IPC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Glance's desktop HTTP MCP endpoint with a stdio configuration that launches the actual Glance executable and forwards MCP traffic to the selected live desktop runtime over per-instance local IPC.

**Architecture:** Each desktop process starts one MCP server over a per-user Unix socket or Windows named pipe and publishes an instance record containing a unique ID, PID, canonical executable path, and IPC address. `mcp serve --stdio --instance-id ID` becomes a byte-for-byte stdio bridge to that live server; `mcp serve --stdio --open PATH` remains an independent headless workflow. The MCP settings page lists live processes and copies a config for the selected one.

**Tech Stack:** Go 1.27, `github.com/modelcontextprotocol/go-sdk` v1.8.0 `mcp.IOTransport`, Unix domain sockets, Windows named pipes, Gio persistent widgets, JSON instance records.

**Spec:** [docs/superpowers/specs/2026-10-14-desktop-mcp-stdio-ipc-design.md](../specs/2026-10-14-desktop-mcp-stdio-ipc-design.md)

## Global Constraints

- A desktop IPC connection must use the already-running process's `application.Runtime`; it must not create another runtime.
- The desktop process obtains its executable from `os.Executable()` and canonicalizes it before publishing or comparing it.
- Use one unique runtime instance ID per process; PID and executable path are metadata and cannot select between same-path live processes.
- Store discovery records in a private per-user directory; Unix directory/socket modes are `0700`/`0600`, and Windows named pipes grant access only to the current user.
- If a requested instance ID is stale, path fallback is allowed only when exactly one live instance matches; zero or multiple matches are clear errors.
- `mcp serve --stdio --open PATH` continues to open independent headless documents; a desktop-targeted request never silently starts a headless runtime.
- MCP JSON-RPC frames use stdout only; help, errors, and diagnostics use stderr.
- Remove automatic desktop HTTP endpoint startup and port/token controls; old HTTP MCP entries must be replaced using the generated stdio config.
- Keep the existing MCP capability catalog and document behavior unchanged.

## Review Focus

- Two live instances with the same executable path: explicit IDs route to their own Runtime; a missing or stale ID with multiple matches reports ambiguity. Pin this in `TestDesktopMCPInstanceResolutionRejectsAmbiguousPath` and `TestMCPStdioSubprocessTargetsSelectedDesktopRuntime`.
- Stale instance record: dead processes are removed after liveness verification, while a live process with a temporarily unavailable endpoint is not mistaken for a different target. Pin this in `TestDesktopMCPRegistryPrunesOnlyDeadProcesses`.
- Executable path containing spaces: generated config preserves it as one JSON `command` value and the CLI validates it without shell splitting. Pin this in `TestMCPDesktopConfigurationPreservesExecutablePathWithSpaces`.
- IPC permissions and shutdown: only the current user can access the Unix socket/Windows pipe, and orderly shutdown removes the record and endpoint. Pin Unix behavior in `TestDesktopMCPIPCUsesPrivateSocketAndCleansUp`; cover Windows pipe ACL and cleanup in the Windows-specific IPC test.
- Explicit independent `--open PATH`: headless work remains isolated from desktop records, and desktop-target mode with no matching instance returns an actionable error. Pin this in `TestMCPStdioOpenPathRemainsHeadless` and `TestMCPStdioDesktopTargetDoesNotFallBackToHeadless`.

## File Map

| File | Responsibility |
|---|---|
| `internal/mcpserver/desktop_ipc.go` | Instance record model, private registry, stale-record pruning, target resolution, and `DesktopIPCServer` lifecycle. |
| `internal/mcpserver/desktop_ipc_unix.go` | Unix listener/dialer, private socket creation, and Unix process-liveness check. |
| `internal/mcpserver/desktop_ipc_windows.go` | Current-user named-pipe listener/dialer and Windows process-liveness check. |
| `internal/mcpserver/server.go` | Attach an existing MCP Server to one IPC connection using `mcp.IOTransport`; preserve the shared capability catalog. |
| `internal/mcpserver/cli.go` | Add `--instance-id`, resolve and bridge desktop stdio, keep explicit `--open` headless mode, and report errors on stderr. |
| `internal/mcpserver/desktop_ipc_test.go` | Registry, uniqueness, permissions, stale records, target resolution, and same-Runtime MCP IPC behavior. |
| `internal/mcpserver/desktop_ipc_windows_test.go` | Windows named-pipe ACL and lifecycle coverage. |
| `internal/mcpserver/cli_test.go`, `internal/mcpserver/stdio_process_test.go` | CLI mode rules, stdio bridge routing, path-with-spaces handling, and headless compatibility. |
| `internal/desktopui/runtime.go`, `internal/desktopui/runtime_workspace_persistence.go`, `internal/desktopui/app.go` | Start IPC with the real executable path, retain the endpoint on the process runtime, and stop it during shutdown. |
| `internal/desktopui/app.go` | Stable selected-instance ID, listed instance snapshots, and stable row clickables. |
| `internal/desktopui/mcp_connection_ui.go` | Settings MCP instance picker and generated stdio configuration. |
| `internal/desktopui/mcp_connection_ui_test.go` | One-instance default, explicit multi-instance selection, actual executable path, and generated command/args. |
| `internal/desktopui/mcp_port_config_test.go`, `internal/mcpserver/desktop_endpoint.go`, `internal/mcpserver/desktop_endpoint_test.go` | Remove the obsolete desktop port/token endpoint and its tests. |
| `internal/workspace/preferences.go` | Remove the unused MCP port preference constant; old persisted values remain inert and need no schema migration. |
| `docs/architecture.md`, `docs/mcp-connection.md` | Document desktop stdio-to-IPC routing, headless `--open`, Settings → MCP instance selection, and the one-time client-config replacement. |

## Tasks

### Task 1: Add a per-instance desktop IPC server and registry

**Files:**
- Create: `internal/mcpserver/desktop_ipc.go`
- Create: `internal/mcpserver/desktop_ipc_unix.go`
- Create: `internal/mcpserver/desktop_ipc_windows.go`
- Create: `internal/mcpserver/desktop_ipc_test.go`
- Create: `internal/mcpserver/desktop_ipc_windows_test.go`
- Modify: `internal/mcpserver/server.go`
- Modify: `go.mod`, `go.sum` only if the Windows named-pipe implementation needs the already-approved `go-winio` dependency.

**Interfaces:**
- `type DesktopMCPInstance struct { ID string; PID int; ExecutablePath string; Address string; StartedAt time.Time }` is the registry/configuration DTO.
- `type DesktopIPCOptions struct { RuntimeDir, SocketDir string }` lets tests use isolated roots; production stores records under the user's cache directory and Unix sockets under a short private per-user temp directory to stay below `sockaddr_un.sun_path` limits.
- `func StartDesktopIPCServer(runtime *application.Runtime, executablePath string, opts DesktopIPCOptions) (*DesktopIPCServer, error)` canonicalizes `executablePath`, creates unique runtime/socket directories and an IPC listener, then publishes the record only after the listener is ready.
- `func ListDesktopMCPInstances(opts DesktopIPCOptions) ([]DesktopMCPInstance, error)` validates records and removes only records whose owning process is confirmed dead; empty directory fields select the production per-user directories.
- `func ResolveDesktopMCPInstance(instanceID, executablePath string, opts DesktopIPCOptions) (DesktopMCPInstance, error)` returns a matching ID only when its canonical executable path also matches; if the ID is empty or no longer exists, it falls back only when exactly one live record matches the path. An ID that exists but belongs to a different executable is rejected.
- `func (s *Server) ServeConn(ctx context.Context, conn net.Conn) error` calls the existing SDK server's `Connect` with `mcp.IOTransport{Reader: conn, Writer: conn}` and waits for that session to end.
- `func (e *DesktopIPCServer) Instance() DesktopMCPInstance` and `func (e *DesktopIPCServer) Close() error` expose the registered identity and stop acceptance, close MCP sessions, and remove socket/record.
- Platform helpers provide `listenDesktopIPC(socketDir, instanceID string) (net.Listener, string, error)`, `dialDesktopIPC(address string) (net.Conn, error)`, `removeStaleDesktopIPC(socketDir, address string) error`, and `processExists(pid int) (bool, error)`.

- [x] **Step 1: Add registry and IPC lifecycle tests** for unique IDs/addresses, atomic record publication/removal, private directory and socket modes, dead-process pruning, live-process preservation, and ambiguity when two records share an executable path.
- [x] **Step 2: Run the focused tests and verify they fail** because the registry and IPC API do not exist yet.

  Run: `go test ./internal/mcpserver -run 'TestDesktopMCP(Registry|InstanceResolution|IPC)' -count=1`

  Expected: compilation fails on the new test references.
- [x] **Step 3: Implement `DesktopMCPInstance`, registry storage, and resolution** in `desktop_ipc.go`; use per-instance JSON records written by temp-file plus rename, and verify process liveness before deleting a stale record.
- [x] **Step 4: Implement Unix and Windows IPC helpers** in their build-tagged files; use `net.Listen("unix", ...)` with `0700` directory and `0600` socket on Unix, and a current-user ACL named pipe on Windows.
- [x] **Step 5: Implement `Server.ServeConn` and `DesktopIPCServer`**; accept one connection per MCP session on the already-built server, close sessions before listener cleanup, and publish the record only after successful bind.
- [ ] **Step 6: Run the focused tests and verify they pass**, including the Windows-specific package test on Windows CI.

  Run: `go test ./internal/mcpserver -run 'TestDesktopMCP(Registry|InstanceResolution|IPC)' -count=1`

  Expected: PASS; Unix tests inspect the actual permission bits and confirm record/socket removal after `Close`.
- [x] **Step 7: Commit Task 1** as `feat: add per-instance desktop MCP IPC`.

### Task 2: Route stdio MCP children to selected desktops

**Files:**
- Modify: `internal/mcpserver/cli.go`
- Modify: `internal/mcpserver/cli_test.go`
- Modify: `internal/mcpserver/stdio_process_test.go`

**Interfaces:**
- Extend `glance mcp serve --stdio` with `--instance-id ID`.
- Mode rules: `--open PATH` selects the existing headless runtime and cannot be combined with `--instance-id`; without `--open`, resolve the desktop target using `--instance-id`, the CLI's own canonical `os.Executable()` path, and default `DesktopIPCOptions{}`. Empty, missing, mismatched, or ambiguous targets fail on stderr without creating an `application.Runtime`.
- The desktop branch connects to `DesktopMCPInstance.Address` and copies bytes in both directions between stdin/stdout and the IPC connection; closing either side terminates the bridge. The bridge does not parse or write protocol-adjacent text to stdout.
- Keep `DispatchCLI(args, stdout, stderr)` as the public entrypoint and implement it through a private `dispatchCLIWithOptions(args, stdout, stderr, ipcOpts, executablePath, stdin, protocolStdout)` seam so tests can use isolated registries and actual in-memory pipes without environment-variable overrides in production.

- [x] **Step 1: Add CLI mode tests** `TestMCPCLIRejectsDesktopTargetWithoutMatchingInstance`, `TestMCPCLIRejectsAmbiguousAndMismatchedDesktopTarget`, and `TestMCPCLIDesktopAndHeadlessModesAreExclusive`; cover help/argument diagnostics staying on stderr.
- [x] **Step 2: Add subprocess MCP tests** `TestMCPStdioSubprocessTargetsSelectedDesktopRuntime` and `TestMCPStdioSubprocessFallsBackFromStaleIDWhenUnique`; launch children through `mcp.CommandTransport`, edit the parent's live Runtime, verify the selected document revision changes, and prove a same-path sibling runtime remains untouched.
- [x] **Step 3: Add headless compatibility coverage** proving `--open PATH` still creates and edits an independent document and no `--open`/no target does not fall back to headless mode.
- [x] **Step 4: Run the CLI and subprocess tests and verify the new cases fail** against the current CLI behavior.

  Run: `go test ./internal/mcpserver -run 'Test(MCPCLI|MCPStdio)' -count=1`

  Expected: new routing cases fail because `--instance-id` is not recognized and the current command creates a headless runtime.
- [x] **Step 5: Implement the `--instance-id` parsing and mode dispatch**; preserve the existing `--open` runtime creation path, and put every discovery/dial error on stderr with an actionable message.
- [x] **Step 6: Implement the bidirectional stdio bridge** with coordinated goroutine shutdown so EOF, IPC failure, context cancellation, or client termination exits the child and releases the connection.
- [x] **Step 7: Run the CLI and subprocess tests and verify they pass**, including the existing independent Word save workflow.

  Run: `go test ./internal/mcpserver -run 'Test(MCPCLI|MCPStdio)' -count=1`

  Expected: PASS; the selected desktop's live revision changes, the other same-path Runtime does not, and headless `--open` remains independent.
- [x] **Step 8: Commit Task 2** as `feat: route desktop MCP stdio to selected instance`.

### Task 3: Start IPC from the desktop runtime and add instance selection UI

**Files:**
- Modify: `internal/desktopui/runtime.go`
- Modify: `internal/desktopui/runtime_workspace_persistence.go`
- Modify: `internal/desktopui/app.go`
- Modify: `internal/desktopui/mcp_connection_ui.go`
- Modify: `internal/desktopui/mcp_connection_ui_test.go`

**Interfaces:**
- Replace `desktopRuntime.mcpEndpoint` with `desktopRuntime.mcpIPC *mcpserver.DesktopIPCServer` and `mcpIPCErr error`; start it once with the canonical path from `os.Executable()` and close it in `shutdown` before the application Runtime closes.
- Add stable App state `mcpInstances []mcpserver.DesktopMCPInstance`, `mcpSelectedInstanceID string`, and a map of stable `*widget.Clickable` row controls.
- Add `refreshMCPInstances()` and `selectMCPInstance(id string)` methods. Entering Settings → MCP refreshes records; one live target is auto-selected; multiple targets require a user click; zero targets show a clear unavailable message.
- Generate JSON as `{"command":"<selected executable>","args":["mcp","serve","--stdio","--instance-id","<selected ID>"]}`. Escape through `encoding/json`; never assemble a shell command string.

- [x] **Step 1: Add UI configuration tests** for a single-instance default, explicit selection among two same-path instances, command path equality to the real executable path, proper JSON escaping for a path containing spaces, and no config when no instance is selected.
- [x] **Step 2: Run the MCP settings tests and verify the new cases fail** against the current HTTP-only UI.

  Run: `go test ./internal/desktopui -run 'TestMCPConnection|TestDesktopMCPSettings' -count=1`

  Expected: new selection/configuration tests fail because the page has no instance model or stdio configuration.
- [x] **Step 3: Replace desktop HTTP startup/shutdown with `StartDesktopIPCServer`**; obtain the real executable via `os.Executable()`, canonicalize it inside mcpserver, and keep startup errors visible in the existing status/log path.
- [x] **Step 4: Add persistent instance picker state and event handling** to App; refresh on Settings → MCP entry and update the selected ID before regenerating config.
- [x] **Step 5: Replace the HTTP port, URL, token, and config UI** with a live-instance list, selected executable/PID detail, generated stdio JSON, and a copy button; state plainly that the client starts the stdio bridge and that the selected Glance process must remain open.
- [ ] **Step 6: Run the focused settings tests and verify they pass**, then run existing `TestSettings` layout/interaction regressions to ensure stable Gio controls and hit areas remain correct.

  Run: `go test ./internal/desktopui -run 'Test(MCPConnection|DesktopMCPSettings|Settings)' -count=1`

  Expected: PASS; selected config includes the target ID, path is JSON-escaped as one command value, and widget state persists across frames.
- [x] **Step 7: Commit Task 3** as `feat: select desktop target for MCP stdio`.

### Task 4: Remove fixed-port desktop HTTP configuration and update architecture docs

**Files:**
- Delete: `internal/mcpserver/desktop_endpoint.go`
- Delete: `internal/mcpserver/desktop_endpoint_test.go`
- Delete: `internal/desktopui/mcp_port_config_test.go`
- Modify: `internal/workspace/preferences.go`
- Modify: `internal/mcpserver/server.go`
- Modify: `internal/desktopui/app.go`, `internal/desktopui/mcp_connection_ui.go` (remove obsolete HTTP-only state/imports)
- Create: `docs/mcp-connection.md`
- Modify: `docs/architecture.md`

**Interfaces:**
- Remove `DefaultDesktopMCPPort`, `DesktopEndpoint`, `DesktopEndpointOptions`, `PreferenceMCPServerPort`, and the no-longer-used HTTP-handler/authorization path; existing stored port preferences are ignored and do not require a workspace schema migration.
- Update the architecture statement to distinguish desktop-targeted stdio-over-IPC from independent headless `--open PATH` stdio.
- Add current MCP setup documentation explaining Settings → MCP instance selection and that users must replace legacy URL/token entries with the generated `command`/`args` entry.

- [x] **Step 1: Remove desktop endpoint and port configuration references** and update `docs/architecture.md` plus new `docs/mcp-connection.md` to the new stdio/IPC ownership model; retain the MCP capability catalog and explicit headless `--open` path.
- [x] **Step 2: Search for stale endpoint/port symbols and configuration text**; remove any remaining active code references while leaving historical design documents unchanged.

  Run: `rg -n 'DefaultDesktopMCPPort|DesktopEndpoint|PreferenceMCPServerPort|GLANCE_MCP_PORT|Bearer Token|50628' internal docs/architecture.md docs/mcp-connection.md`

  Expected: no active source or current architecture documentation references; historical specs/plans may describe the previously implemented endpoint.
- [ ] **Step 3: Run MCP and desktop regression coverage** for IPC lifecycle, same-path target routing, headless `--open`, settings selection, and existing MCP capabilities.

  Run: `go test ./internal/mcpserver ./internal/desktopui -count=1`

  Expected: PASS on supported local platforms; Windows pipe-specific coverage runs on Windows CI.
- [x] **Step 4: Run `git diff --check`** and review the final diff for unintentional changes to existing workspace edits.
- [x] **Step 5: Commit Task 4** as `docs: remove desktop MCP port setup`.

## Self-Review

- **Spec coverage:** Runtime reuse and one-connection sessions are Task 1; per-user registry and stale cleanup are Task 1; actual executable path and fail-closed target resolution are Tasks 1–3; stdout bridge and independent `--open` are Task 2; UI selection and config copy are Task 3; old HTTP/port removal and migration instructions are Task 4; capability behavior remains unchanged throughout.
- **Step scan:** Each task has explicit file ownership, signatures, mode rules, focused expected test outcomes, implementation actions, and a commit boundary.
- **Type consistency:** Tasks 2–3 consume the exact `DesktopMCPInstance`, `StartDesktopIPCServer`, `ListDesktopMCPInstances`, `ResolveDesktopMCPInstance`, and `DesktopIPCServer.Close` APIs from Task 1.
- **Review focus:** Same-path routing and ambiguity are exercised by Tasks 1–2; stale records and process liveness by Task 1; paths with spaces by Tasks 2–3; permissions and cleanup by Task 1; explicit headless mode and no-fallback behavior by Task 2.
- **Proportion:** Four tasks follow the existing backend → CLI → desktop UI → cleanup/documentation dependency chain; no independent subsystem needs a separate plan.

## Verification notes

- Focused MCP IPC, same-path routing, headless `--open`, instance selection, JSON path escaping, shutdown, and copy-feedback tests passed on macOS.
- The complete `internal/mcpserver` package passed. The complete `internal/desktopui` package reproduced the pre-existing `TestHomeFavoriteNavigationSelectsOnFirstClick` failure and was then killed; the baseline run recorded the same failure before this implementation.
- The Windows MCP test binary cross-compiled successfully with `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -p=1 -c ./internal/mcpserver`; its named-pipe lifecycle test still needs execution on a Windows host.
- An earlier Windows cross-compilation attempt reported `no space left on device`; after space became available, the serialized cross-compilation passed. Existing caches and temporary artifacts were not removed.
