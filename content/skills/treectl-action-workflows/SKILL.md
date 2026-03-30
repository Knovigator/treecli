---
name: treectl-action-workflows
description: Use treectl action flows to discover tags, submit action-tag prompts, and manage synchronous or asynchronous generation workflows.
---

# treectl Action Workflows

Use this skill when you need to submit or inspect Treechat action-tag work through `treectl`.

## Discover Tags

- Run `treectl action tags` to inspect the current model-backed action tags for the active profile.
- Use `--allow-unknown-tag` only when you intentionally need to bypass local tag validation.

## Treechat Action Model

- Use plain generation tags like `flux`, `veo3`, or `kling` when you want a brand-new asset from the prompt.
- Use an `animate_*` tag when the goal is to animate an existing image.
- Use an `edit_*` tag when the goal is to edit an existing image.
- If the user says "animate this image" or "edit the previous image", do not swap in a plain generation tag or you will likely create a new asset instead of transforming the existing one.

## Submit Action Work

- Root action: `treectl action flux "a glass cathedral in the rain"`.
- Reply action: `treectl action --reply-to <quest-id> animate_kling "animate this as a handheld push-in"`.
- Existing-image animation: `treectl action --reply-to <quest-id> animate_kling "animate this still as a handheld push-in"`.
- Existing-image edit: `treectl action --reply-to <quest-id> edit_flux "make this warmer and more cinematic"`.
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
