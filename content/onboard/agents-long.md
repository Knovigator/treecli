## treectl CLI Guidance

Use `treectl` as the CLI surface for Treechat automation in this repo.

### Profiles and Auth

- Check available environments with `treectl profile list`.
- Inspect the resolved config with `treectl profile show`.
- Authenticate the profile you need with `treectl login --profile dev` or another profile name.
- The built-in profiles are `dev`, `staging`, and `prod`.

### Reading Threads and Answers

- Fetch a thread with `treectl get thread <quest-id>`.
- Fetch one or more answers with `treectl get messages <answer-id> [...]`.
- Use `--json` when another tool needs structured output.

### Creating Posts and Replies

- Root posts: `treectl new post "message text"`.
- Replies: `treectl new post --reply-to <quest-id> "reply text"`.
- New root posts default to private.
- Root posts can target a stream with `--stream private`, `--stream public`, `--stream clips`, a stream name, or a stream UUID.
- Replies inherit the existing thread placement, so do not pass stream-placement flags with `--reply-to`.

### Running AI Actions

- Discover model-backed AI actions with `treectl action tags`.
- Root action: `treectl action flux "a glass cathedral in the rain"`.
- Reply action: `treectl action --reply-to <quest-id> animate_kling "animate this as a handheld push-in"`.
- Treechat action model:
- Use plain AI actions like `flux`, `veo3`, or `kling` to generate a new asset from the prompt.
- Use `animate_*` AI actions to animate an existing image from the thread or attachment context.
- Use `edit_*` AI actions to edit an existing image from the thread or attachment context.
- If the user wants to animate or edit a previous image, do not substitute a plain generation action.
- Actions default to private root-thread placement unless you pass a root-only stream flag.
- `treectl action` waits by default and shows a spinner in interactive terminals.
- For post-less local media generation, inspect support with `treectl generate actions --direct-only`, then use `treectl generate <action> "prompt" --out <file>`.

### Async Action Workflows

- Submit and exit immediately with `treectl action --no-wait flux "prompt"`.
- Check an answer later with `treectl action status --answer <answer-id>`.
- Check a thread later with `treectl action status --thread <quest-id>`.
- Keep polling with `treectl action status --answer <answer-id> --watch`.

### Output Style

- Human-readable output is the default.
- Pass `--json` when you need machine-readable output.

### Packaged Skills

- Use `treectl skills list` to discover packaged skills.
- Install them into an agent skills directory with `treectl skills install ...`.
- The first skills to install are the basic posting workflow and the action workflow skills.

### Shell Completion

- For bash and zsh, turn completions on in the current shell with `if [ -n "${ZSH_VERSION:-}" ]; then autoload -U compinit && compinit; source <(treectl completion zsh); elif command -v complete >/dev/null 2>&1; then source <(treectl completion bash); else echo "Current shell does not support bash completion; use zsh or a bash with progcomp."; fi`.
- Use `treectl completion bash` or `treectl completion zsh` directly if you want to install persistent completions.
