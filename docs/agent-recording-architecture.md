# Agent Recording Architecture (`treecli mcp`)

`treecli mcp` records the sessions of AI coding agents (Claude Code, Codex, Grok)
into memdb and Treechat, and serves the recorded memory back to those agents over
the Model Context Protocol. This document is the contract for that subsystem.

## Goals

1. Every agent session on a machine ends up in one durable, searchable store the
   person controls (memdb), without the agent having to do anything.
2. The next session in the same project starts with what earlier sessions learned.
3. Sessions can be mirrored into a Treechat stream so people can read, discuss and
   upvalue them, without duplicates on retries.
4. One binary and one install command per agent; nothing runs as a daemon.

## Non-goals

- Observing a conversation through MCP alone. No MCP revision (through 2026-07-28)
  lets a server see the host conversation; servers only see their own tool calls.
  Recording therefore rides host hooks and on-disk transcripts.
- Replacing memdb's schema or search. treecli never opens the SQLite file; it shells
  out to `memdb-rust`, which owns ingest, FTS, learned memories and the Qdrant
  sidecar.

## Components

```
agent (Claude Code | Codex | Grok Build)
  ├─ hooks ──────────► treecli mcp hook --agent <a>   (stdin: JSON event)
  │                       │  converts transcript → memdb JSONL, ingests, mirrors
  │                       ▼
  │                   <data>/recordings/<agent>/<session>.jsonl ──► memdb-rust ingest
  │                       │                                        (SQLite + FTS5 [+ Qdrant])
  │                       └──► Treechat (quests/answers API)  [mode session|turns]
  └─ MCP client ─────► treecli mcp serve --agent <a>   (stdio)
                          tools: memory_recall, memory_search, memory_learn,
                                 session_note, treechat_post, session_info
                          scope: hook marker <data>/sessions/<agent>/<project>.json
```

### Packages

| Package | Responsibility |
| --- | --- |
| `recorder/session.go` | Normalized model (`Session`, `Message`, `Turn`), project key, memdb session key, deterministic ids |
| `recorder/claude.go` | Claude Code project transcript (`~/.claude/projects/<project>/<session>.jsonl`) → `Session` |
| `recorder/codex.go` | Codex rollout (`~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`) → `Session` |
| `recorder/memdbjsonl.go` | `Session` → memdb ingest JSONL; atomic, content-stable writes |
| `recorder/memdb.go` | `memdb-rust` subprocess wrapper (`init`, `ingest`, `recall`, `query`, `learn`, `recent`) |
| `recorder/store.go` | Data directory layout: recordings, session markers, Treechat state, hook log |
| `recorder/treechat.go` | Treechat mirror: session thread, turn replies, closing summary, notes |
| `recorder/pipeline.go` | `Recorder`: transcript → recording → memdb → Treechat; backfill discovery |
| `recorder/hooks.go` | Hook payload normalization and the per-event handler (context injection) |
| `recorder/query.go` | Free text → FTS5 `OR` query (memdb passes queries straight to MATCH) |
| `recorder/mcpserver.go` | The MCP server and its six tools (official Go SDK, stdio) |
| `cmd/mcp*.go` | Cobra commands: `serve`, `hook`, `install`, `uninstall`, `status`, `config`, `backfill`, `record` |
| `content/skills/treecli-memory` | Packaged skill teaching agents when to use the tools |

## Data model

A session is identified by the tuple memdb already scopes on:

| memdb field | value | example |
| --- | --- | --- |
| `chat_app` | agent id | `claude-code`, `codex`, `grok` |
| `chat_id` | project key = working directory with every non-alphanumeric run replaced by `-` (Claude Code's own project-dir naming) | `-Users-me-src-treechat` |
| `thread_id` | the host's session id | `6ad8a064-…` |
| `session_key` | `agent:treecli:<chat_app>:group:<chat_id>:thread:<thread_id>` | |

The session key follows memdb's `coalesce_scope_from_session_key` grammar, so
`recall --session-key K` resolves to exactly the rows ingest wrote. memdb-rust needed
one change for this: when a transcript carries no Telegram/Treechat routing clues,
ingest now derives `chat_app`/`chat_id`/`thread_id` from the session key (commit
`f734b4c` on `claude/ingest-session-key-scope`). Older memdb builds still ingest the
recordings but only scope by `chat_app`.

Recording JSONL (memdb's ingest contract):

```jsonl
{"type":"session","timestamp":"…","id":"<session>","cwd":"…","sessionKey":"agent:treecli:claude-code:group:-Users-me-src-app:thread:<session>","chat_app":"claude-code","model":"…","recorder":"treecli"}
{"type":"message","timestamp":"…","id":"<msg>","message":{"role":"user","content":"prompt text"}}
{"type":"message","timestamp":"…","id":"<msg>","parentId":"<msg>","message":{"role":"assistant","content":[{"type":"text","text":"…"},{"type":"toolCall","id":"…","name":"Bash","arguments":{…}}]}}
{"type":"message","timestamp":"…","id":"<msg>","toolCallId":"…","message":{"role":"toolResult","content":"…"}}
```

Tool results are capped at 6,000 characters and tool arguments at 3,000, so a
200 KB file write does not become a memory row. Thinking blocks, subagent sidechains,
Claude's bookkeeping lines, and the context blocks Codex injects as user messages
are dropped.

## Hook flow

Hooks are the recording path because they are the only passive signal all three
agents provide. The dialect is shared (Claude-style `hooks.json`, JSON on stdin,
exit 0):

| Event | Claude Code / Codex (transcript on disk) | Grok Build (no readable transcript) |
| --- | --- | --- |
| `SessionStart` | write marker; inject "recent exchanges from earlier sessions in this project" | same, plus create an empty recording |
| `UserPromptSubmit` | inject learned memories matching the prompt's keywords | also append the prompt |
| `PostToolUse` | (not registered) | append tool call + result |
| `Stop` | convert the whole transcript, ingest, mirror finished turns | append `last_assistant_message`, ingest, mirror |
| `SessionEnd` | as `Stop`, then the closing summary; mark the marker ended | same |

Whole-transcript conversion on every `Stop` keeps the recording a pure function of
the host transcript. `WriteRecordingFile` leaves byte-identical files untouched, and
memdb skips unchanged sources by mtime/size and replaces the rows of changed ones,
so re-syncs are idempotent.

Context injection uses the documented `hookSpecificOutput.additionalContext` shape
and is capped at 1,800 characters. Both injections are configurable
(`treecli mcp config --recall-on-start/--recall-on-prompt`).

Failure policy: the hook process always exits 0. Problems are appended to
`<data>/logs/hooks.log`; `--verbose` also prints them to stderr. memdb calls time out
after 45 s. A memdb failure never blocks the recording file; a Treechat failure never
blocks memdb.

## MCP server

`treecli mcp serve --agent <a>` speaks stdio MCP through the official Go SDK
(`github.com/modelcontextprotocol/go-sdk`, which serves both the legacy
`initialize` handshake and the 2026-07-28 stateless form). Hosts do not tell MCP
servers their session id, so the server reads the hook marker for its agent and
working directory to scope tools to the live conversation; without a marker it works
at project scope and `session_info` says so.

| Tool | memdb call | Notes |
| --- | --- | --- |
| `memory_recall` | `recall --mode thread` (session scope) and/or `recall --mode long-term` (project scope) | query is keyword-OR'd |
| `memory_search` | `query` with `scope` = session / project / agent / all | `include_events` searches tool calls |
| `memory_learn` | `learn --type … --title …` | types mirror memdb's `LEARNED_MEMORY_TYPES`; `scope` project (default), session, global |
| `session_note` | appends an assistant "Note:" message to the recording, re-syncs | optional Treechat post |
| `treechat_post` | — | posts into the session thread; requires a Treechat mode |
| `session_info` | — | agent, ids, paths, Treechat thread |

Tool results are compact JSON text (time, role, text ≤ 700 chars per hit) to keep
context cost low.

## Treechat mirror

Off by default. `treecli mcp config --treechat-mode session|turns --treechat-team
<stream>` posts as the configured treecli account through the ordinary API:

- Session thread: `POST /api/v1/quests` with `team_id` (or `private=true` without a
  stream); client ids `uuid5("session-thread", agent, session)` and
  `uuid5("session-root", …)`.
- Turn replies (`turns` mode): `POST /api/v1/answers`, id `uuid5("turn", agent,
  session, index)`; posted only once the turn has a final reply.
- Closing summary once per session; notes keyed by their text.

Treechat deduplicates on client-supplied ids (`quests#create` returns the existing
root thread; `answers#create` returns the existing answer), which is what makes hook
retries and repeated syncs safe. Posts are capped at 3,500 characters.
`treecli mcp backfill` never posts.

## Installation contract

| Agent | What `treecli mcp install` writes | Loads when |
| --- | --- | --- |
| Claude Code | `~/.claude/skills/treecli/` plugin: `.claude-plugin/plugin.json`, `.mcp.json`, `hooks/hooks.json`, `skills/treecli-memory/SKILL.md` | next session, as `treecli@skills-dir` |
| Codex | `[mcp_servers.treecli]` in `~/.codex/config.toml`; treecli entries merged into `~/.codex/hooks.json` | next session; hooks need one-time trust via `/hooks` |
| Grok Build | `[mcp_servers.treecli]` in `~/.grok/config.toml`; `~/.grok/hooks/treecli.json` | next start |
| Grok Bot (desktop) | nothing (encrypted settings); the installer prints the `mcpServers` snippet | when added in its MCP settings |

Commands reference the absolute path of the installed binary. Re-running replaces only
treecli's own entries (matched by the `mcp hook --agent` invocation shape, not the
binary name); `uninstall` removes exactly those. `CLAUDE_CONFIG_DIR`, `CODEX_HOME`
and `GROK_HOME` are honoured.

## Configuration

`[mcp]` in treecli's `config.toml`, each overridable by environment:

| Key | Env | Default |
| --- | --- | --- |
| `mcp.memdb_bin` | `TREECLI_MEMDB_BIN` | `memdb-rust`/`memdb` on `PATH`, then `~/src/memdb-rust/target/release/memdb-rust`, `/root/memdb-rust/target/release/memdb-rust` |
| `mcp.memdb_db` | `TREECLI_MEMDB_DB`, `MEMDB_PATH` | `<data>/memdb.sqlite3` |
| `mcp.data_dir` | `TREECLI_MCP_DATA_DIR` | XDG data dir (`~/Library/Application Support/treecli` on macOS) |
| `mcp.treechat.mode` | `TREECLI_MCP_TREECHAT_MODE` | `off` |
| `mcp.treechat.team_id` | `TREECLI_MCP_TREECHAT_TEAM` | none (private threads) |
| `mcp.treechat.env` / `.account` | — | the usual `--env/--account` selection |
| `mcp.recall_on_start` / `.recall_on_prompt` | `TREECLI_MCP_RECALL_ON_START` / `_PROMPT` | `true` |

## Testing

`go test ./recorder ./cmd` covers: transcript parsers (fixtures modelled on real
Claude Code and Codex files), JSONL round trip and content-stable writes, the hook
handler end to end against a fake `memdb-rust` (the test binary re-executes itself
with `TREECLI_FAKE_MEMDB=1`), the Treechat mirror against `httptest` (deterministic
ids, idempotent re-syncs, turn gating), the MCP server through in-memory transports,
and the installers (TOML table upsert/removal, hooks.json merge, plugin layout).

Manual checks used while building: `treecli mcp record --agent claude <transcript>`
against the real memdb binary, `treecli mcp hook` fed the payload shapes captured
from a live Claude Code probe (`--plugin-dir`), and `treecli mcp serve` driven with raw
JSON-RPC over stdio.

## Known limits

- Claude Code and Codex rollouts are "not a stable interface" per their vendors; the
  parsers are lenient (unknown records are skipped) and fixtures should be refreshed
  when a format changes.
- Grok Build could not be tested on a live install; its hook and config formats follow
  the vendor documentation. Grok Bot only records what the agent chooses to call
  `session_note` / `memory_learn` with.
- memdb's long-term recall needs a query; `SessionStart` injection therefore shows
  recent exchanges, while learned memories surface at prompt time.
- Codex allows a single `notify` command; treecli does not touch it and uses hooks
  instead.
