## treectl CLI Guidance

- Use `treectl profile list`, `treectl profile show`, and `treectl login --profile <name>` to pick and authenticate an environment.
- Read data with `treectl get thread <quest-id>` and `treectl get messages <answer-id> [...]`.
- Create a root post with `treectl new post "text"` and a reply with `treectl new post --reply-to <quest-id> "text"`.
- Root posts default to private; use `--stream` only on root posts and root actions.
- Discover AI actions with `treectl action tags`.
- Submit action work with `treectl action <action> "prompt"` or `treectl action --reply-to <quest-id> <action> "prompt"`.
- Use plain AI actions for new assets, `animate_*` to animate an existing image, and `edit_*` to edit an existing image.
- If the goal is to animate or edit a previous image, do not use a plain generation action.
- For post-less local media generation, inspect support with `treectl generate actions --direct-only`; use `treectl generate actions --verbose` or `treectl generate describe <action>` for descriptions, settings, and examples.
- Generate local media with `treectl generate <action> "prompt" --out <file>` and pass settings with `--input key=value`, `--settings '{...}'`, `--duration`, `--instrumental`, or `--reference` as described by the action.
- Use `treectl action --no-wait` plus `treectl action status --answer ...` or `--thread ...` for async flows.
- Human-readable output is the default. Use `--json` when you need structured output.
- Install packaged skills with `treectl skills list` and `treectl skills install ...`.
- Check for a newer CLI release with `treectl update --check`; install it with `treectl update`. Use `--json` for machine-readable update status.
- Turn on bash/zsh completions in the current shell with `if [ -n "${ZSH_VERSION:-}" ]; then autoload -U compinit && compinit; source <(treectl completion zsh); elif command -v complete >/dev/null 2>&1; then source <(treectl completion bash); else echo "Current shell does not support bash completion; use zsh or a bash with progcomp."; fi`.
