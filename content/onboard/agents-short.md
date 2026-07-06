## treecli CLI Guidance

- Use `treecli profile list`, `treecli profile show`, and `treecli login --profile <name>` to pick and authenticate an environment.
- Read data with `treecli get thread <quest-id>` and `treecli get messages <answer-id> [...]`.
- Create a root post with `treecli new post "text"` and a reply with `treecli new post --reply-to <quest-id> "text"`.
- Root posts default to private; use `--stream` only on root posts and root actions.
- Discover AI actions with `treecli action actions`.
- Submit action work with `treecli action <action> "prompt"` or `treecli action --reply-to <quest-id> <action> "prompt"`.
- Use `eleven_tts` for text-to-speech and `video_sfx` for video sound effects; CLI aliases include `eleven`, `elevenlabs`, `11`, `sfx`, `mmaudio`, and `foley`.
- Use plain AI actions for new assets, `animate_*` to animate an existing image, and `edit_*` to edit an existing image.
- If the goal is to animate or edit a previous image, do not use a plain generation action.
- Use `--payment usd` for Stripe metered AI billing or `--payment bsv` / `--payment bitcoinsv` for Bitcoin SV; omit it to use the account default.
- For post-less local media generation, inspect support with `treecli generate actions --direct-only`; use `treecli generate actions --verbose` or `treecli generate describe <action>` for descriptions, inputs, settings, reference behavior, and examples.
- Generate local media with `treecli generate <action> "prompt" --out <file>` and pass settings with `--input key=value`, `--settings '{...}'`, `--duration`, `--instrumental`, `--reference`, or `--payment` as described by the action and billing intent.
- Use `treecli action --no-wait` plus `treecli action status --answer ...` or `--thread ...` for async flows.
- Human-readable output is the default. Use `--json` when you need structured output.
- Install packaged skills with `treecli skills list` and `treecli skills install ...`.
- Check for a newer CLI release with `treecli update --check`; install it with `treecli update`. Use `--json` for machine-readable update status.
- Turn on bash/zsh completions in the current shell with `if [ -n "${ZSH_VERSION:-}" ]; then autoload -U compinit && compinit; source <(treecli completion zsh); elif command -v complete >/dev/null 2>&1; then source <(treecli completion bash); else echo "Current shell does not support bash completion; use zsh or a bash with progcomp."; fi`.
