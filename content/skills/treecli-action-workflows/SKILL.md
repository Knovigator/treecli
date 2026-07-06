---
name: treecli-action-workflows
description: Use treecli action flows to discover AI actions, submit structured action_requests, set duration when supported, and manage synchronous or asynchronous generation workflows.
---

# treecli Action Workflows

Use this skill when you need to submit or inspect Treechat AI action work through `treecli`.

## Discover AI Actions

- Run `treecli action actions` to inspect the current model-backed AI actions for the active profile.
- Use `--allow-unknown-action` only when you intentionally need to bypass local AI action validation.

## Treechat Action Model

- Use plain generation actions like `flux`, `veo3`, or `kling` when you want a brand-new asset from the prompt.
- Use an `animate_*` action when the goal is to animate an existing image.
- Use an `edit_*` action when the goal is to edit an existing image.
- Use `eleven_tts` for text-to-speech audio. Accepted CLI aliases are `eleven`, `elevenlabs`, and `11`.
- Use `video_sfx` for video sound effects or foley. Accepted CLI aliases are `sfx`, `mmaudio`, and `foley`.
- If the user says "animate this image" or "edit the previous image", do not swap in a plain generation action or you will likely create a new asset instead of transforming the existing one.

## Submit Action Work

- Root action: `treecli action flux "a glass cathedral in the rain"`.
- Text-to-speech action: `treecli action eleven_tts "read this in a crisp narration voice"`.
- Video sound effects action: `treecli action sfx "rain, tires on wet asphalt, distant thunder"`.
- Root action with requested audio/video duration: `treecli action stableaudio "ambient build, 120 BPM" --duration 90`.
- Reply action: `treecli action --reply-to <quest-id> animate_kling "animate this as a handheld push-in"`.
- Existing-image animation: `treecli action --reply-to <quest-id> animate_kling "animate this still as a handheld push-in"`.
- Existing-image edit: `treecli action --reply-to <quest-id> edit_flux "make this warmer and more cinematic"`.
- Root actions default to private placement unless you pass a root-only stream flag like `--stream public`.
- Use `--duration` only when the target model supports duration, such as audio or video generation; the backend clamps the value to the model's allowed range.
- Use `--payment usd` for Stripe metered AI billing or `--payment bsv` / `--payment bitcoinsv` for Bitcoin SV. Omit `--payment` to use the account default.

## Async Workflows

- Submit without waiting: `treecli action --no-wait flux "prompt"`.
- Check a thread later: `treecli action status <quest-id-or-link>`.
- Check a specific post or answer later: `treecli action status --post <answer-id>` or `treecli action status --answer <answer-id>`.
- Keep polling until completion: `treecli action status --answer <answer-id> --watch`.

## Direct Local Generation

- Use `treecli generate actions` to list all active AI actions and see which support direct post-less generation.
- Use `treecli generate actions --direct-only` when you only want actions that can save media locally without creating a post.
- Use `treecli generate actions --verbose` for a full human/agent-readable catalog with model descriptions, inputs, settings, reference behavior, examples, and notes.
- Use `treecli generate describe <action>` before generating when you need focused help for one model.
- Run direct generation with `treecli generate <action> "prompt" --out <file>`.
- Text-to-speech direct generation: `treecli generate eleven_tts "read this in a crisp narration voice" --out narration.mp3`.
- Video sound effects direct generation: `treecli generate sfx "rain, tires on wet asphalt, distant thunder" --reference @clip.mp4 --out sfx.mp3`.
- Pass settings with `--input key=value` for individual values, `--settings '{...}'` for a JSON settings object, `--duration` for duration-aware actions, `--instrumental` for music actions, `--reference run:<id>|https://...|@path` for reference-aware actions, and `--payment usd|bsv` for per-run billing rail selection.

## Output Rules

- Human-readable output is the default.
- Use `--json` when another tool needs stable machine-readable output.

## Notes

- `treecli action` writes structured `action_requests` for backend execution. Each request includes a generated `id`, matching `client_id`, AI action name in the compatibility `tag` field, `prompt`, `kind: "model"`, and `generation_count: 1`.
- When `--duration` is present, `treecli action` adds `settings.duration_seconds` to the action request.
- `treecli action` also writes display `delta_json` so the post content renders like a normal AI action message in Treechat.
- Interactive polling shows a spinner on `stderr`, so JSON output on `stdout` stays clean.
