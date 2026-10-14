# MCP connection setup

Glance's desktop MCP connection uses stdio and local IPC. It does not use a TCP port or bearer token.

## Connect a client to a running Glance window

1. In Glance, open **Settings → MCP**.
2. Select the row for the Glance process the client should control. Each row shows its process ID and executable path, so two windows launched from the same app can still be selected separately. If only one Glance instance is running, it is selected automatically.
3. Copy the generated JSON configuration and add it to the MCP client's server configuration.
4. Keep the selected Glance process running while the client is connected.

The generated entry has this shape:

```json
{
  "command": "/path/to/Glance",
  "args": ["mcp", "serve", "--stdio", "--instance-id", "INSTANCE_ID"]
}
```

Glance obtains the `command` value from the running process with `os.Executable()` and canonicalizes it before registering the instance. The JSON keeps paths containing spaces as one command value; no shell quoting is needed. The stdio bridge uses the selected process's unique instance ID to reach that process over a private Unix-domain socket on macOS/Linux or a current-user named pipe on Windows. There is no fixed port, so multiple Glance processes can run at once.

If the selected process exits, the client receives a connection error. When an old configuration contains an expired instance ID, Glance can recover only if exactly one live instance has the same canonical executable path. It reports an error if there are no matching instances or if multiple same-path instances make the target ambiguous.

## Replace an older HTTP connection

Older configurations may contain a loopback HTTP URL and an Authorization bearer token. Remove that URL/token entry from the MCP client and replace it with the generated `command` and `args` entry from **Settings → MCP**.

## Explicit headless use

For a separate no-window MCP runtime, open documents explicitly:

```sh
glance mcp serve --stdio --open "/path/to/document.docx"
```

Each `--open PATH` is loaded into the independent headless runtime. Without `--open`, the stdio command must resolve a live desktop instance and never silently creates a headless runtime.
