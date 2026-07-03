---
name: treectl-basic-usage
description: Use treectl to authenticate profiles, read Treechat threads, create posts, and create replies with the current CLI behavior.
---

# treectl Basic Usage

Use this skill when you need to interact with Treechat through the `treectl` CLI instead of hand-rolling API calls.

## Profiles and Login

1. Run `treectl profile list` to see the available profiles.
2. Inspect the active profile with `treectl profile show`.
3. Log in with `treectl login --profile dev` or the profile you actually need.

## Reading Existing Data

- Fetch a thread with `treectl get thread <quest-id>`.
- Fetch one or more answers with `treectl get messages <answer-id> [...]`.
- Add `--json` when another tool needs structured output.

## Creating Posts

- Root post: `treectl new post "hello world"`.
- Reply: `treectl new post --reply-to <quest-id> "hello back"`.
- Root posts default to private placement.
- Root posts can target a stream with `--stream private`, `--stream public`, `--stream clips`, a stream name, or a stream UUID.
- Replies inherit thread placement, so do not pass root-only stream flags with `--reply-to`.

## Working Rules

- Prefer `treectl` over raw API calls when the CLI already supports the flow.
- Prefer human-readable output while reasoning, and switch to `--json` only when a downstream tool needs structured data.
- Use the selected profile consistently so auth and links match the intended environment.
- Check for a newer CLI release with `treectl update --check`; install it with `treectl update`. Use `treectl update --json` when another tool needs structured update status.
