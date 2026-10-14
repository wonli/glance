# MCP connection setup

Glance's desktop MCP connection uses stdio and local IPC. It does not use a TCP port or bearer token.

## Connect a client to the current Glance session

1. In Glance, open **Settings → MCP**.
2. Copy the generated JSON configuration and add it to the MCP client's server configuration.
3. Keep Glance running while the client is connected.

The generated entry has this shape:

```json
{
  "command": "/path/to/Glance",
  "args": ["mcp", "serve", "--stdio"]
}
```

Glance gets `command` from the running process with `os.Executable()`, so paths containing spaces stay one command value and need no shell quoting. The stdio bridge connects to the one stable, private per-user Unix-domain socket on macOS/Linux or current-user named pipe on Windows. It reaches the live desktop Runtime and its unsaved document sessions. The previous `--instance-id` argument is accepted and ignored for configurations created by older releases.

Only one interactive Glance process can run for the current OS user. Starting Glance again, including from another app copy, raises the existing primary window and exits; it does not forward a file path. Multiple document windows inside the existing process remain available. Headless MCP child processes do not take the desktop lock.

The lock and activation endpoint use per-user OS synchronization and local IPC, so no fixed port is needed. If a Glance process from a release without single-instance support is still running after upgrading, close it once before starting this release. The new release cannot activate or adopt an older process.

## Replace an older HTTP connection

Older configurations may contain a loopback HTTP URL and an Authorization bearer token. Remove that URL/token entry from the MCP client and replace it with the generated `command` and `args` entry from **Settings → MCP**.

## Explicit headless use

For a separate no-window MCP runtime, open documents explicitly:

```sh
glance mcp serve --stdio --open "/path/to/document.docx"
```

Each `--open PATH` is loaded into an independent headless Runtime. Without `--open`, the stdio command connects to the running desktop Runtime and exits with a clear stderr error if its IPC endpoint is unavailable; it does not silently create a headless Runtime.
