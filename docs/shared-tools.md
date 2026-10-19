# Shared Treechat tools

The CLI and local MCP adapter consume the backend-owned tool catalog and execute
through the same backend operations. Model options, permissions, input ownership,
price caps, idempotency and paid execution are enforced by Treechat, not copied into Go.
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
can explain their client-side equivalents. Named remote CLI commands are thin
adapters over the shared catalog and executor; the executor dispatches to backend
controllers where appropriate.

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
`quote-reply` accept Treechat links. Shared tools now normalize those links too:
`id` for thread/post reads, `answer` for answer-based action status, `quest_id`
(or `reply_to`/`thread`) for in-thread replies, and `source_post_id` for branch/quote
replies. Bulk `messages.ids` also accepts post links. UUIDs remain valid directly.
Thread links must identify a `/quest/UUID`; post links must identify a post and
cannot be thread links. Ambiguous links containing different target UUIDs are
rejected. Parsing never fetches the URL or changes the authenticated backend.

Model discovery and `describe_action` return the backend's settings, defaults,
reference capabilities, descriptions, aliases, examples and notes. CLI commands
format these shared results without maintaining a second model catalog.

## Direct generation and spending limits

Named `generate` calls `generate_media` once with an explicit per-action ceiling
and idempotency key. Pricing, eligibility, daily budget admission and generation
execution belong to the shared backend. No preview is required:

```sh
treecli generate flux2 "wide hero banner" --out banner.webp --max-price-cents 100 --id banner-001
treecli tools call generate_media --arguments '{"model":"flux2","prompt":"wide hero banner","max_price_cents":100,"idempotency_key":"banner-001"}'
```

The ceiling is in USD cents and authorizes only this action. Reuse the same key
with identical inputs after an uncertain result; a different request needs a new
key. Do not execute both examples as separate requests with different keys unless
you intend separate generations. Local file selection, upload transport and
output downloads remain client responsibilities. Reuse uploaded reference IDs
when retrying rather than uploading the same file again.

`generate --quote` calls `quote_generation` as an optional price preview without
generating media. Existing reviewed quotes can still be executed with
`generate --quote-id QUOTE_ID --id REQUEST_ID --out FILE`, which calls
`create_generation`. Both execution paths enforce the shared direct-generation
daily budget. There is no automatic fallback to a legacy endpoint.

`billing budget` calls `get_generation_budget`; `billing budget set CENTS` and
`billing budget unset` call `set_generation_budget` with an integer or explicit
null. A zero daily cap blocks positive-cost admissions; an unset daily cap imposes
no daily limit. Per-action caps remain required. No budget values are inferred
from these examples and these commands do not create a billing subscription.

This budget covers quoted-price reservations admitted through shared direct
`generate_media` and `create_generation` calls, across CLI and MCP. It excludes
native/post-backed generation, the legacy direct endpoint, other Treechat spend,
and network fees. All admitted job states remain counted; this is not a settled
billing total. Admission days reset at midnight UTC. The status response reports
the current cap, reserved/remaining amounts, reset time, scope and exclusions.

## Shared resources and client responsibilities

`treecli_reference` provides skills list/emit, onboarding guide/agents text,
completion scripts and release-update metadata. Static resources are generated
from the canonical CLI source with revision provenance. Installation, binary
replacement, local account files and wallet-key custody remain client operations.
The shared executor owns alias, stream/branch, link, reference and catalog
normalization. Local shell argument parsing and presentation remain in Go.
