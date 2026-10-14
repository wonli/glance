# Single desktop Glance instance and simplified MCP Implementation Plan

> **For agentic workers:** Execute this plan directly on `main`, as requested. Keep the existing XLS/document-import working-tree changes untouched. Do not use a temporary Go cache directory. Do not run tests unless the user asks; update only existing assertions that encode the removed multi-instance behavior. Stage only files changed for this feature and create one final commit.

**Goal:** Permit one interactive Glance process per OS user, activate it on duplicate launch, and reduce desktop MCP routing to one stable per-user IPC endpoint.

**Architecture:** A per-user OS lock chooses the one desktop process. A private activation socket or named pipe sends a path-free activation request to its UI loop. MCP stdio connects directly to a single stable endpoint owned by the same desktop Runtime, while explicit headless `--open` remains separate.

**Tech Stack:** Go 1.27, Gio 0.10.3, Unix `flock` and Unix-domain sockets on macOS/Linux, Windows named mutex and named pipes, existing MCP SDK and application Runtime.

**Spec:** `docs/superpowers/specs/2026-10-14-single-desktop-instance-and-mcp-design.md`

## Global Constraints

- The singleton is scoped to the current OS user and is independent of `os.Executable()` or install path.
- Duplicate desktop launch sends only activation; it never forwards a document path.
- Headless MCP child processes do not acquire the desktop lock.
- `mcp serve --stdio --open PATH` remains independent of a running desktop process.
- Desktop MCP uses the active Runtime and one stable private per-user IPC endpoint; do not add a TCP port.
- Keep accepting `--instance-id ID` as an ignored compatibility flag; new configuration omits it.
- Preserve in-process Gio windows and the active document/session behavior.
- Do not create temporary Go cache directories or touch unrelated working-tree changes.
- Do not add new test functions or run tests unless the user asks; update existing tests only where their assertions require the removed multi-instance behavior.
- Do not commit task-by-task. At completion stage explicit feature paths only and create one commit.

## Review Focus

- Two starts from the same or different Glance binary paths: exactly one desktop process; the later process activates the primary window and exits. Update the existing launcher/MCP lifecycle assertions that can exercise this path; do not add a new test function.
- A duplicate launch during primary startup: wait for activation IPC or acquire the lock only after it is released; never start while another process holds it. Review the retry and timeout branches in the existing code path.
- A stale Unix socket after a crash: clean it only while holding the singleton lock and only after confirming it is an owned socket under the private runtime directory. Review cleanup guards; do not broaden deletion.
- An older pre-singleton process: it cannot honor the new lock or activation protocol, so the migration instructions must tell users to close existing Glance processes once before first launching this build; do not terminate an older process.
- Headless MCP launch: `mcp serve --stdio --open PATH` bypasses the desktop lock; retain the existing CLI mode assertions.
- Desktop MCP with no live endpoint: exit with a useful stderr error and do not create a headless Runtime; update the existing CLI assertions for stable endpoint discovery.

---

## File Structure

- Create `internal/desktopinstance/desktop_instance.go` for the lease API, stable activation protocol, and retry/ack flow.
- Create `internal/desktopinstance/desktop_instance_unix.go` and `desktop_instance_windows.go` for OS lock acquisition and activation transport.
- Modify `internal/desktoplaunch/launch.go` to acquire-or-activate before Gio startup and explicitly release the lease before any `os.Exit` path.
- Modify `internal/desktopui/app.go` and create `internal/desktopui/runtime_activation.go` to consume queued activation requests and raise or restore the primary window on the UI-owned path. Avoid `runtime.go`, which has concurrent edits unrelated to this feature.
- Modify `internal/mcpserver/desktop_ipc.go`, `desktop_ipc_unix.go`, and `desktop_ipc_windows.go` to serve one stable endpoint and remove the active registry/resolution flow.
- Modify `internal/mcpserver/cli.go` to dial the stable endpoint without executable matching or instance IDs, keep the old ID flag as a no-op, and preserve `--open`.
- Modify `internal/desktopui/mcp_connection_ui.go` and `app.go` state to remove the instance picker and emit one ID-free command using the actual executable path.
- Update existing affected MCP/UI test assertions in `internal/mcpserver/desktop_ipc_test.go`, `desktop_ipc_windows_test.go`, `cli_test.go`, `stdio_process_test.go`, `internal/desktopui/mcp_connection_ui_test.go`, and `mcp_connection_stdio_test.go`; do not add new test functions or run them.
- Update `docs/architecture.md`, `docs/mcp-connection.md`, and mark `docs/superpowers/specs/2026-10-14-desktop-mcp-stdio-ipc-design.md` as superseded. Keep the existing implementation plan as historical record.

## Interfaces

### `internal/desktopinstance`

```go
type Lease struct { /* lock, listener, activation queue, close state */ }

// AcquireOrActivate returns (lease, true, nil) to the primary process.
// It returns (nil, false, nil) after a secondary process has activated the primary.
// It returns a non-nil error if it cannot safely become or activate the primary.
func AcquireOrActivate() (*Lease, bool, error)

// ActivationRequests is a coalescing, buffered stream consumed by desktop UI startup.
func (l *Lease) ActivationRequests() <-chan struct{}

// Close stops the activation listener and releases the OS lock; it is idempotent.
func (l *Lease) Close() error
```

Activation frames are versioned and contain only `action=activate`; the listener acknowledges after queueing. The activation queue has capacity one and coalesces duplicate raise requests. The Unix lock file remains at a stable path while the lock is held; do not unlink it on orderly close, since another process could then lock a different inode.

### `internal/mcpserver`

```go
func StartDesktopIPCServer(runtime *application.Runtime, executablePath string, opts DesktopIPCOptions) (*DesktopIPCServer, error)
func dialDesktopIPC(address string) (net.Conn, error)
```

Production options resolve one fixed per-user endpoint. `DesktopIPCOptions` continues to allow isolated runtime/socket directories for the existing tests. The endpoint does not publish a random instance ID or JSON process registry. The deprecated `--instance-id` value is parsed for compatibility but ignored.

## Tasks

### Task 1: Implement the per-user desktop lease and activation channel

**Files:**
- Create: `internal/desktopinstance/desktop_instance.go`
- Create: `internal/desktopinstance/desktop_instance_unix.go`
- Create: `internal/desktopinstance/desktop_instance_windows.go`

**Interfaces:**
- Produces: `Lease`, `AcquireOrActivate()`, `(*Lease).ActivationRequests()`, and idempotent `(*Lease).Close()` as defined above.
- Unix lock path and activation socket live under `/tmp/gm-<uid>` rather than the process's `TMPDIR`, so Finder and shell launches share the same private per-user runtime directory; Windows names include the current user's SID and use current-user-only ACLs.

- [x] **Step 1: Create or validate the private per-user runtime directory** using owner and mode checks; keep the Unix socket path below the platform path limit.
- [x] **Step 2: Implement the Unix lock-file operation** with nonblocking `flock`; preserve the lock-file inode on close.
- [x] **Step 3: Implement the Unix activation listener/client** with versioned activation frames, bounded startup retry, acknowledgement after enqueue, and owned stale-socket cleanup.
- [x] **Step 4: Implement the Windows named mutex and activation pipe** with current-user-only ACLs and the same acquire/activate/retry contract.
- [x] **Step 5: Implement `AcquireOrActivate()` and `Lease.Close()`** so the returned state unambiguously identifies primary versus already-activated secondary; make close safe to call more than once.
- [x] **Step 6: Review the two platform implementations side by side** for matching state transitions and errors; run `gofmt` on created files and `git diff --check` on the task diff.

### Task 2: Gate desktop startup and raise the primary window

**Files:**
- Modify: `internal/desktoplaunch/launch.go`
- Modify: `internal/desktopui/app.go`
- Create: `internal/desktopui/runtime_activation.go`

**Interfaces:**
- Consumes: `desktopinstance.AcquireOrActivate()` and `Lease.ActivationRequests()`.
- Produces: an activation-aware desktop run entry while retaining `Run` and `RunWithStartupError` wrappers used by existing callers.

- [x] **Step 1: Acquire or activate in `RunDesktop`** before `desktopui.RunWithStartupError` and `app.Main`; return immediately when a secondary receives an activation acknowledgement.
- [x] **Step 2: Pass the primary lease's activation channel into desktop UI startup** without changing the existing public wrappers used by tests or other packages.
- [x] **Step 3: Queue activation until the primary window is attached** and then call a runtime method that raises the existing primary window through Gio's `system.ActionRaise` path; make it work whether the system tray is ready or unavailable.
- [x] **Step 4: Close the lease before the launcher's `os.Exit` path** and when `app.Main` returns; rely on idempotent close to cover both completion paths.
- [x] **Step 5: Review every entry point** (`cmd/glance` and `cmd/boot`) to confirm they reach the gate, while MCP CLI dispatch still occurs before it; format changed Go files and check the diff.

### Task 3: Replace per-instance MCP routing with one stable endpoint

**Files:**
- Modify: `internal/mcpserver/desktop_ipc.go`
- Modify: `internal/mcpserver/desktop_ipc_unix.go`
- Modify: `internal/mcpserver/desktop_ipc_windows.go`
- Modify: `internal/mcpserver/cli.go`
- Modify existing assertions only: `internal/mcpserver/desktop_ipc_test.go`, `desktop_ipc_windows_test.go`, `cli_test.go`, `stdio_process_test.go`

**Interfaces:**
- Consumes: the existing application `Runtime` and `DesktopIPCOptions` overrides.
- Produces: one stable MCP IPC listener per desktop process; `mcp serve --stdio` dials it directly, and `mcp serve --stdio --open PATH` keeps creating an independent headless Runtime.

- [x] **Step 1: Change desktop MCP endpoint creation to a stable address** derived from the current user, with no random instance ID or per-process JSON registry; keep test directory overrides.
- [x] **Step 2: Simplify `DesktopIPCServer.Close()` and Unix/Windows listener cleanup** to close the single stable endpoint and safely recover a stale endpoint only after the singleton lock is held.
- [x] **Step 3: Simplify CLI dispatch** so the desktop branch dials the single endpoint, the old `--instance-id` is accepted and ignored, missing desktop IPC fails on stderr, and the explicit `--open` branch is unchanged.
- [x] **Step 4: Update existing MCP test assertions** that currently require two simultaneous instances or ambiguous executable matching; preserve tests for private transport, live Runtime routing, stderr/stdout separation, and headless `--open` without adding test functions.
- [x] **Step 5: Keep the existing desktop runtime startup/shutdown calls compatible** by preserving the runtime-facing API while changing its implementation behind the API; do not edit `runtime.go` while it has concurrent changes. Format changed Go files and check the diff.

### Task 4: Remove the MCP instance picker and align documentation

**Files:**
- Modify: `internal/desktopui/mcp_connection_ui.go`
- Modify: `internal/desktopui/app.go`
- Modify existing assertions only: `internal/desktopui/mcp_connection_ui_test.go`, `internal/desktopui/mcp_connection_stdio_test.go`
- Modify: `docs/architecture.md`
- Modify: `docs/mcp-connection.md`
- Modify: `docs/superpowers/specs/2026-10-14-desktop-mcp-stdio-ipc-design.md`

**Interfaces:**
- Consumes: the current desktop executable from `os.Executable()` and the stable MCP endpoint status.
- Produces: a single ID-free configuration with `args: ["mcp", "serve", "--stdio"]`; no instance list, selection state, or selection card.
- `mcpDesktopStdioConfiguration(executablePath string) (string, error)` builds JSON with `encoding/json`; the UI checks endpoint readiness through `desktopMCPIPCStatus() (bool, error)`.

- [x] **Step 1: Replace instance discovery and selection state** with direct current-process configuration generation; preserve JSON escaping and copy feedback.
- [x] **Step 2: Simplify the MCP settings page** to explain it connects to the current desktop Runtime and show the generated JSON without the instance picker/card.
- [x] **Step 3: Update existing MCP UI assertions** to check the single generated command and the absence of instance selection behavior; do not add test functions.
- [x] **Step 4: Update active architecture and MCP connection documentation** for one stable endpoint, the activation-only duplicate launch, the one-time close of older Glance processes, and the separate headless `--open` mode; mark the earlier multi-instance design superseded.
- [x] **Step 5: Review the full feature diff and working tree**; run `gofmt` for modified Go files and `git diff --check`. Do not run tests unless the user asks.

### Task 5: Create the one selective final commit

**Files:**
- Only the explicit files created or modified by Tasks 1–4, including this feature's approved spec and plan.

- [x] **Step 1: Recheck `git status` and inspect the exact diff** to identify the feature-owned paths; do not stage unrelated XLS/doc-import changes or any concurrently synced files.
- [x] **Step 2: Stage only the reviewed feature paths** by explicit path names and inspect `git diff --cached --stat` plus `git diff --cached`.
- [x] **Step 3: Create one commit on `main`** after the user-approved implementation is complete; leave every unrelated staged/unstaged/untracked path untouched.

## Verification Boundaries

- No new test functions are added, and no test commands are run under the current instruction.
- Source-level checks during implementation are limited to `gofmt` and `git diff --check`; any runtime or automated test pass claim requires a later explicit request to run verification.
- The feature is incomplete until the desktop lock, activation channel, stable MCP routing, simplified settings page, and documentation all agree with the spec.
