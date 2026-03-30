## treectl CLI Guidance

- Use `treectl profile list`, `treectl profile show`, and `treectl login --profile <name>` to pick and authenticate an environment.
- Read data with `treectl get thread <quest-id>` and `treectl get messages <answer-id> [...]`.
- Create a root post with `treectl new post "text"` and a reply with `treectl new post --reply-to <quest-id> "text"`.
- Root posts default to private; use `--stream` only on root posts and root actions.
- Discover action tags with `treectl action tags`.
- Submit action work with `treectl action <tag> "prompt"` or `treectl action --reply-to <quest-id> <tag> "prompt"`.
- Use `treectl action --no-wait` plus `treectl action status --answer ...` or `--thread ...` for async flows.
- Human-readable output is the default. Use `--json` when you need structured output.
- Install packaged skills with `treectl skills list` and `treectl skills install ...`.
- Turn on bash/zsh completions in the current shell with `if [ -n "${ZSH_VERSION:-}" ]; then autoload -U compinit && compinit; source <(treectl completion zsh); elif command -v complete >/dev/null 2>&1; then source <(treectl completion bash); else echo "Current shell does not support bash completion; use zsh or a bash with progcomp."; fi`.
