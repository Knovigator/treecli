# Shared Treechat tools

The CLI and local MCP adapter consume the backend-owned tool catalog and execute
through the same backend operations. Model options, permissions, input ownership,
quote consent and paid execution are enforced by Treechat, not copied into Go.
A backend exposing `/api/v1/treecli/tools` and `/api/v1/treecli/call` is required.

```sh
treecli --env production --account default tools list
treecli tools call TOOL_NAME --arguments '{"key":"value"}'
treecli --env production --account default mcp serve
```

`mcp serve` uses newline-delimited JSON-RPC on stdin/stdout, initialized with MCP
protocol version 2024-11-05. It loads the selected account once and keeps that
explicit client for the process. Launch a separate process for another account.
It does not expose arbitrary HTTP endpoints or execute shell commands. The server
fetches tool schemas from Treechat; new backend tools do not need another Go list.
Transport failures are sanitized. Backend tool errors preserve `isError` and the
structured tool result. Input messages are limited to 72 MiB (to accommodate base64 uploads with a backend 50 MiB decoded limit).

This is the Treechat API adapter, separate from the unmerged local session recorder.
It does not inspect client transcripts, expose local private wallet keys, install
files, or update the CLI binary. Local-only commands remain local; backend guidance
can explain their client-side equivalents. Existing commands continue using their
existing backend controllers.

The API package also supports explicit request-scoped OAuth bearer clients without
saved-credential fallback. This is transport support only: backend endpoints must
independently authorize the bearer and its scopes. The command adapter uses normal
selected CLI token authentication, not a shared service account.

## Structured equivalents

Discover all account-visible actions, then inspect one using the same catalog:

```sh
treecli tools call treechat_read --arguments '{"operation":"generation_actions","arguments":{}}'
treecli tools call treechat_read --arguments '{"operation":"describe_action","arguments":{"action":"!11"}}'
```

CLI action invocation syntax becomes explicit `action`, `prompt`, `settings` and
`payment_mode` arguments to `quote_treechat_write`, with `operation: "action"`
and an explicit `max_price_cents`. For example, `!11 hello --payment usd` becomes
`{"action":"!11","prompt":"hello","payment_mode":"usd"}`. Backend canonicalization
resolves aliases. Duration belongs in `settings.duration_seconds` (or the action
operation's `duration_seconds` convenience field). Providers come from the model
catalog and backend; they are not executable client-selected adapters.

For multiple explicit actions, posting accepts an `action_requests` list using the
existing backend contract: each model request has `tag`, `kind: "model"`, `prompt`,
`generation_count`, optional `settings` and `payment_mode`. The quote and durable
write authorization apply to the operation's aggregate generation fees. This
structured representation avoids copying the CLI's shell/text invocation parser.
Always show the returned authorization terms before calling `create_treechat_write`.

For media references, use `upload_file` with the selected client file's base64
bytes, or the upload registration flow when the client supports direct upload.
The server enforces its decoded file-size and ownership checks. Never substitute a
server-local path for a client file. Tool catalog schemas define exact upload
argument names and bounds.

## CLI syntax and structured arguments

Commands such as `action status`, `new post --reply-to`, `branch-reply`, and
`quote-reply` accept Treechat links as a CLI convenience. The shared tool surface
uses the corresponding UUID explicitly: `id` for a thread/post read, `answer`
for answer-based action status, `quest_id` for an in-thread reply, and
`source_post_id` for branch/quote replies. Use the relevant thread or post UUID
from the link; do not pass an entire URL in an ID field. Returned API objects also
provide these IDs, avoiding link parsing entirely.

Model discovery and `describe_action` return the backend's settings, defaults,
reference capabilities, descriptions and aliases. The CLI's additional handcrafted
example commands and help notes remain in CLI help; this adapter does not maintain
a second copy of that prose or a separate model/tool catalog.

Existing CLI commands retain their argument parsing, formatting and convenience
normalization above the existing backend controllers. The new `tools` commands
and MCP adapter share one backend-owned catalog and executor. Authorization,
pricing, billing, generation dispatch and persistence remain shared backend
business logic. This change does not claim to have replaced every legacy CLI
parser with the new tool interface.
