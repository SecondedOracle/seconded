# Examples

| Directory | What it shows |
| --- | --- |
| [`verify-receipt/`](verify-receipt/) | Verify a receipt offline with the standalone verifier, and verify against the key document the live API publishes. |
| [`host-config/`](host-config/) | The exact MCP host configuration `seconded-mcp host-snippet` prints for Claude Code, Claude Desktop, Cursor, Codex CLI, Gemini CLI and a generic host. |
| [`tool-call/`](tool-call/) | What an agent sends to `seconded_trade_check` and what it is told to do with each answer label. |

Every file here was produced from the code and catalogs in this repository; the host
snippets were recorded from a binary built with `go build ./cmd/seconded-mcp` and the
tool input is the catalog's own `example_input`.
