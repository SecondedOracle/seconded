# MCP host configuration

`seconded-mcp host-snippet --host HOST` prints the configuration for one host and, on
standard error, the instruction for where it goes. The files in this directory are the
recorded outputs for every host the 0.4.1 client knows (`generic`, `claude-code`,
`claude-desktop`, `cursor`, `codex`, `gemini`). Replace `/absolute/path/to/seconded-mcp`
with the real path of your binary; hosts need an absolute path.

| Host | File | Where it goes |
| --- | --- | --- |
| Claude Code | [`claude-code.sh`](claude-code.sh) | Run the printed `claude mcp add` command in your shell (PowerShell on Windows). Exit the current Claude Code session, then start a new one. |
| Claude Desktop | [`claude-desktop.json`](claude-desktop.json) | Merge the `mcpServers` entry into your user `claude_desktop_config.json`. MCPB users connect through the bundle manifest instead and must not also merge this snippet. Fully quit Claude Desktop, including its tray or menu-bar process, then reopen it. |
| Cursor | [`cursor.json`](cursor.json) | Merge the `mcpServers` entry into your user `~/.cursor/mcp.json`. Fully quit Cursor, then reopen it. |
| Codex CLI | [`codex.toml`](codex.toml) | Merge the table into your user `~/.codex/config.toml`. Exit and restart the Codex session. |
| Gemini CLI | [`gemini.json`](gemini.json) | Merge the `mcpServers` entry into your user `~/.gemini/settings.json`. Exit and restart Gemini CLI. |
| Any other host | [`generic.json`](generic.json) | Add the command and arguments to your user-level MCP host settings, then fully quit and reopen that host. |

The `--host` argument passed to `serve` only selects host-specific output formatting;
the tools, prices and payment behaviour are the same on every host.

`setup --host HOST` prints the same snippet after creating the wallet, so you normally
do not need `host-snippet` separately. See [the install guide](../../docs/guide/install.md).
