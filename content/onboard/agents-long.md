## treecli CLI Guidance

Use `treecli` as the CLI surface for Treechat automation in this repo.

### Profiles and Auth

- Check available environments with `treecli profile list`.
- Inspect the resolved config with `treecli profile show`.
- Authenticate the profile you need with `treecli login --profile dev` or another profile name.
- The built-in profiles are `dev`, `staging`, and `prod`.

### Reading Threads and Answers

- Fetch a thread with `treecli get thread <quest-id>`.
- Fetch one or more answers with `treecli get messages <answer-id> [...]`.
- Use `--json` when another tool needs structured output.

### Creating Posts and Replies

- Root posts: `treecli new post "message text"`.
- Replies: `treecli new post --reply-to <quest-id> "reply text"`.
- New root posts default to private.
- Root posts can target a stream with `--stream private`, `--stream public`, `--stream clips`, a stream name, or a stream UUID.
- Replies inherit the existing thread placement, so do not pass stream-placement flags with `--reply-to`.

### Running AI Actions

- Discover model-backed AI actions with `treecli action actions`.
- Root action: `treecli action flux "a glass cathedral in the rain"`.
- Reply action: `treecli action --reply-to <quest-id> animate_kling "animate this as a handheld push-in"`.
- Treechat action model:
- Use plain AI actions like `flux`, `veo3`, or `kling` to generate a new asset from the prompt.
- Use `animate_*` AI actions to animate an existing image from the thread or attachment context.
- Use `edit_*` AI actions to edit an existing image from the thread or attachment context.
- If the user wants to animate or edit a previous image, do not substitute a plain generation action.
- Actions default to private root-thread placement unless you pass a root-only stream flag.
- `treecli action` waits by default and shows a spinner in interactive terminals.
- For post-less local media generation, inspect support with `treecli generate actions --direct-only`.
- Use `treecli generate actions --verbose` or `treecli generate describe <action>` to get model descriptions, accepted inputs, settings, reference behavior, and examples before generating.
- Generate local media with `treecli generate <action> "prompt" --out <file>` and pass settings with `--input key=value`, `--settings '{...}'`, `--duration`, `--instrumental`, or `--reference` as described by the action.

### Async Action Workflows

- Submit and exit immediately with `treecli action --no-wait flux "prompt"`.
- Check an answer later with `treecli action status --answer <answer-id>`.
- Check a thread later with `treecli action status --thread <quest-id>`.
- Keep polling with `treecli action status --answer <answer-id> --watch`.

### Output Style

- Human-readable output is the default.
- Pass `--json` when you need machine-readable output.

### Packaged Skills

- Use `treecli skills list` to discover packaged skills.
- Install them into an agent skills directory with `treecli skills install ...`.
- The first skills to install are the basic posting workflow and the action workflow skills.

### CLI Updates

- Check for a newer release with `treecli update --check`.
- Install the latest release with `treecli update`.
- Use `treecli update --json` or `treecli update --check --json` when another tool needs structured update status.

### Shell Completion

- For bash and zsh, turn completions on in the current shell with `if [ -n "${ZSH_VERSION:-}" ]; then autoload -U compinit && compinit; source <(treecli completion zsh); elif command -v complete >/dev/null 2>&1; then source <(treecli completion bash); else echo "Current shell does not support bash completion; use zsh or a bash with progcomp."; fi`.
- Use `treecli completion bash` or `treecli completion zsh` directly if you want to install persistent completions.
