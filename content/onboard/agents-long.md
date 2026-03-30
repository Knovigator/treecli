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

### Running Action Tags

- Discover model-backed tags with `treectl action tags`.
- Root action: `treectl action flux "a glass cathedral in the rain"`.
- Reply action: `treectl action --reply-to <quest-id> kling "animate this as a handheld push-in"`.
- Actions default to private root-thread placement unless you pass a root-only stream flag.
- `treectl action` waits by default and shows a spinner in interactive terminals.

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
