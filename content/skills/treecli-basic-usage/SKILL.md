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

- Fetch a thread with `treecli get thread <quest-id>`.
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
