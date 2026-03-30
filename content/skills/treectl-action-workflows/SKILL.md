---
name: treectl-action-workflows
description: Use treectl action flows to discover tags, submit action-tag prompts, and manage synchronous or asynchronous generation workflows.
---

# treectl Action Workflows

Use this skill when you need to submit or inspect Treechat action-tag work through `treectl`.

## Discover Tags

- Run `treectl action tags` to inspect the current model-backed action tags for the active profile.
- Use `--allow-unknown-tag` only when you intentionally need to bypass local tag validation.

## Submit Action Work

- Root action: `treectl action flux "a glass cathedral in the rain"`.
- Reply action: `treectl action --reply-to <quest-id> kling "animate this as a handheld push-in"`.
- Root actions default to private placement unless you pass a root-only stream flag like `--stream public`.

## Async Workflows

- Submit without waiting: `treectl action --no-wait flux "prompt"`.
- Check a specific answer later: `treectl action status --answer <answer-id>`.
- Check a thread later: `treectl action status --thread <quest-id>`.
- Keep polling until completion: `treectl action status --answer <answer-id> --watch`.

## Output Rules

- Human-readable output is the default.
- Use `--json` when another tool needs stable machine-readable output.

## Notes

- `treectl action` builds the same bolded action-tag delta shape the frontend uses.
- Interactive polling shows a spinner on `stderr`, so JSON output on `stdout` stays clean.
