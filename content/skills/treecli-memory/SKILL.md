---
name: treecli-memory
description: Use the treecli MCP memory tools (memory_recall, memory_search, memory_learn, session_note, treechat_post, session_info) to recall earlier coding-agent sessions from memdb and to save durable decisions, lessons and preferences.
---

# treecli memory

The `treecli` MCP server records this machine's coding-agent sessions (Claude Code,
Codex, Grok) into **memdb**, a local durable-memory store, and can mirror them into a
Treechat stream. Hooks do the recording; you do not need to log turns yourself.

## When to use the tools

- **Starting work in a repository** with possible history: call `memory_recall` with a
  short query describing the task (`mode=both`). It returns recent turns of this session,
  query hits from this session, and durable memories (decisions, lessons, preferences)
  saved for this project.
- **Looking for how something was done before** (a command, a fix, a decision): call
  `memory_search` with the search terms. `scope=project` (default) covers every recorded
  session in this working directory; `scope=agent` covers every project; `scope=all`
  covers every agent. Add `include_events=true` to search tool calls and outputs.
- **Something worth keeping across sessions** was decided or learned: call
  `memory_learn` with `type` (decision, lesson, preference, commitment, handoff, project,
  person), a one-line `title`, a short `summary`, and `evidence` (paths, commands,
  quotes). Reuse the same `key` to update an earlier memory instead of duplicating it.
- **Leaving a note for the log**: `session_note` appends to this session's recording;
  set `post_to_treechat=true` to also post it into the session's Treechat thread.
- **Sharing with people**: `treechat_post` posts text into the session's Treechat thread
  (created inside the configured stream on first use) and returns the thread URL. Only
  do this when the user asks for something to be posted.
- **Checking what is recorded**: `session_info`.

## Rules

- Recall before re-deriving: if a prior session already decided something, quote it and
  build on it rather than starting over.
- Save memories sparingly and precisely. One memory per fact, with evidence. Do not save
  secrets, tokens, or personal data.
- Memories are point-in-time observations. Verify claims about code before relying on them
  when the memory is old.
