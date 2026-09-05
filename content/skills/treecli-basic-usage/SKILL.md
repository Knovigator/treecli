---
name: treecli-basic-usage
description: Use treecli to authenticate profiles, read Treechat threads, create posts, and create replies with the current CLI behavior.
---

# treecli Basic Usage

Use this skill when you need to interact with Treechat through the `treecli` CLI instead of hand-rolling API calls.

## Profiles and Authentication

1. Run `treecli profile list` to see the available profiles.
2. Inspect the active profile with `treecli profile show`.
3. Production is the default. Create an account with `treecli signup`, or log in
   to an existing account with `treecli login`.
4. For local development, explicitly pass `--profile dev` to each command.

## Reading Existing Data

- Get your newest authored root thread: `treecli get threads --user me --root --limit 1`.
- List another user's authored branches: `treecli get threads --user alice --branch --page 2 --limit 20`.
- Listing defaults to your threads, includes authorized private content in the current space, and sorts by creation time (newest first), then ID. Omit `--root`/`--branch` to include both; they cannot be combined.
- `--limit` is page size (1–100, default 20). `--page` starts at 1. JSON collections contain `threads` and `pagination` (`page`, `limit`, `next_page`, `has_more`). Page boundaries can move when data changes; this is not a snapshot export.
- Fetch all accessible child threads with `treecli get threads --answer <answer-id>`. An answer may have zero or multiple children; JSON contains a `threads` array without pagination.
- ID and answer lookups cannot be combined with listing filters. A single thread lookup keeps the existing `quest` JSON envelope.
- `get thread` is deprecated; it still works and warns on stderr. Authored listing requires a backend with the authored collection API; an older backend produces a compatibility error.
- Fetch a thread with `treecli get threads <quest-id>`.
- Fetch one or more answers with `treecli get messages <answer-id> [...]`.
- Add `--json` when another tool needs structured output.

## Creating Posts

- Root post: `treecli new post "hello world"`.
- Reply: `treecli new post --reply-to <quest-id> "hello back"`.
- Root posts default to private placement.
- Root posts can target a stream with `--stream private`, `--stream public`, `--stream clips`, a stream name, or a stream UUID.
- Replies inherit thread placement, so do not pass root-only stream flags with `--reply-to`.

## Working Rules

- Prefer `treecli` over raw API calls when the CLI already supports the flow.
- Prefer human-readable output while reasoning, and switch to `--json` only when a downstream tool needs structured data.
- Use the selected profile consistently so auth and links match the intended environment.
- Check for a newer CLI release with `treecli update --check`; install it with `treecli update`. Use `treecli update --json` when another tool needs structured update status.
