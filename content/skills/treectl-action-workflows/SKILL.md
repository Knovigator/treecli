---
name: treectl-action-workflows
description: Use treectl action flows to discover AI actions, submit structured action_requests, set duration when supported, and manage synchronous or asynchronous generation workflows.
---

# treectl Action Workflows

Use this skill when you need to submit or inspect Treechat AI action work through `treectl`.

## Discover AI Actions

- Run `treectl action tags` to inspect the current model-backed AI actions for the active profile.
- Use `--allow-unknown-tag` only when you intentionally need to bypass local AI action validation.

## Treechat Action Model

- Use plain generation actions like `flux`, `veo3`, or `kling` when you want a brand-new asset from the prompt.
- Use an `animate_*` action when the goal is to animate an existing image.
- Use an `edit_*` action when the goal is to edit an existing image.
- If the user says "animate this image" or "edit the previous image", do not swap in a plain generation action or you will likely create a new asset instead of transforming the existing one.

## Submit Action Work

- Root action: `treectl action flux "a glass cathedral in the rain"`.
- Root action with requested audio/video duration: `treectl action stableaudio "ambient build, 120 BPM" --duration 90`.
- Reply action: `treectl action --reply-to <quest-id> animate_kling "animate this as a handheld push-in"`.
- Existing-image animation: `treectl action --reply-to <quest-id> animate_kling "animate this still as a handheld push-in"`.
- Existing-image edit: `treectl action --reply-to <quest-id> edit_flux "make this warmer and more cinematic"`.
- Root actions default to private placement unless you pass a root-only stream flag like `--stream public`.
- Use `--duration` only when the target model supports duration, such as audio or video generation; the backend clamps the value to the model's allowed range.

## Async Workflows

- Submit without waiting: `treectl action --no-wait flux "prompt"`.
- Check a thread later: `treectl action status <quest-id-or-link>`.
- Check a specific post or answer later: `treectl action status --post <answer-id>` or `treectl action status --answer <answer-id>`.
- Keep polling until completion: `treectl action status --answer <answer-id> --watch`.

## Direct Local Generation

- Use `treectl generate actions` to list all active AI actions and see which support direct post-less generation.
- Use `treectl generate actions --direct-only` when you only want actions that can save media locally without creating a post.
- Run direct generation with `treectl generate <action> "prompt" --out <file>`.

## Output Rules

- Human-readable output is the default.
- Use `--json` when another tool needs stable machine-readable output.

## Notes

- `treectl action` writes structured `action_requests` for backend execution. Each request includes a generated `id`, matching `client_id`, AI action name in the compatibility `tag` field, `prompt`, `kind: "model"`, and `generation_count: 1`.
- When `--duration` is present, `treectl action` adds `settings.duration_seconds` to the action request.
- `treectl action` also writes display `delta_json` so the post content renders like a normal AI action message in Treechat.
- Interactive polling shows a spinner on `stderr`, so JSON output on `stdout` stays clean.
