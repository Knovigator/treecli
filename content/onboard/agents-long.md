## treecli CLI Guidance

Use `treecli` as the CLI surface for Treechat automation in this repo.

### Environments and accounts

Select the server with `--env` and the saved identity with `--account`:

```sh
treecli --account brooz login                 # production (the default)
treecli --account gm-bot login                # another production account
treecli --account gm-bot branch-reply POST_ID "Hello"
treecli --env staging --account gm-bot login  # separate staging credentials
treecli --env dev --account brooz login       # local development

treecli account list
treecli --account gm-bot account show --json
treecli account use gm-bot                    # default account for production
treecli --env staging account use gm-bot     # default account for staging only
```

`prod` and `production` name the production environment; `dev` and `development`
name local development. `staging` is also built in. Account names are local
labels, not verified usernames. Names use letters, numbers, hyphens or underscores
and are normalized to lowercase.

The environment defaults to production. `TREECLI_ENV` sets an explicit default;
`--env` takes precedence. Account selection uses `--account`, then
`TREECLI_ACCOUNT`, then that environment's saved active account, then `default`.
Successful login/signup selects that account for its environment. Logging into
staging never changes the default environment or production's active account.
`account show` displays saved user IDs and redacted credentials; it does not
make a live identity request or prove that a token is still valid.

Custom servers use a separate environment name:

```sh
treecli --env qa --backend-url https://qa.example.test --account gm-bot login
treecli --env qa --account gm-bot branch-reply POST_ID "QA reply"
```

Credentials are stored per environment and account, bound to the backend URL
used at login. Changing `--backend-url` (or `TREECLI_BACKEND_URL`) clears the
resolved credentials for that invocation when the URL differs; log in to that
server explicitly. Reading with an override does not overwrite saved logins.
Use a distinct environment name for each server.

#### Migrating from profiles

`--profile` and `profile list/show/use` are deprecated but remain available for
legacy scripts, with warnings on stderr. Do not combine that interface (including
`TREECLI_PROFILE`/`TREECTL_PROFILE`) with `--env` or `--account`.

Existing profile records are retained. A profile named `custom` on production
can be used as `--env prod --account custom`; a built-in `prod` login is also
available as production's `default` account. Credentials are reused only if the
saved server matches the selected environment. The legacy active account is
preserved when its server matches, but `active_profile` never changes the new
default server away from production. Explicit account selection avoids ambiguity.
A custom profile for a different server needs a matching custom environment URL
or a fresh login. New logins are saved separately under environments/accounts;
legacy profile records are not deleted or rewritten.

### Reading Threads and Answers

- Fetch a thread with `treecli get threads <quest-id>`.
- Fetch one or more answers with `treecli get messages <answer-id> [...]`.
- Use `--json` when another tool needs structured output.

### Creating Posts and Replies

- Root posts: `treecli new post "message text"`.
- Replies: `treecli new post --reply-to <quest-id> "reply text"`.
- New root posts default to private.
- Root posts can target a stream with `--stream private`, `--stream public`, `--stream clips`, a stream name, or a stream UUID.
- Replies inherit the existing thread placement, so do not pass stream-placement flags with `--reply-to`.

### Replying to a specific post

Treechat posts (`Answer` IDs in the API) live in threads (`Quest` IDs). A post
can also be the title/head of its own child branch. Choose where your reply belongs:

| Command | Destination | Quoted post (`reply_to_answer_id`) |
| --- | --- | --- |
| `treecli branch-reply POST_ID "text"` | Post's child branch | Target post, the branch title/head |
| `treecli branch-reply POST_ID --no-quote "text"` | Post's child branch | None |
| `treecli quote-reply POST_ID "text"` | Post's containing thread | Target post |
| `treecli new post --thread THREAD_ID "text"` | Specified thread | None |

Both explicit reply commands accept a post UUID or post link, support `--json`,
`--attachment`, and `--id UUID` for safe retries, and inherit the destination's
visibility. `--no-quote` is only available on `branch-reply`.

For example, reply beneath an individual GM comment:

```sh
treecli branch-reply GM_COMMENT_ID "Tip limit reached for this post ☕"
# Same branch, without the title quote:
treecli branch-reply GM_COMMENT_ID --no-quote "Tip limit reached for this post ☕"
# Stay alongside the GM comment in its containing thread, quoting it:
treecli quote-reply GM_COMMENT_ID "Tip limit reached for this post ☕"
```

The CLI resolves the destination automatically. Branch replies prefer the
canonical discussion child (`side_quest`); a single child is also accepted.
Missing, inaccessible, or ambiguous destinations fail before writing. A root
post has no containing thread, so use `branch-reply` for it. Inspect available
branches with `treecli get threads --answer POST_ID --json`.

API equivalents use `POST /api/v1/answers`: set `quest_id` to the destination
above, and set `reply_to_answer_id` to the target post ID unless using
`--no-quote`. The backend accepts a quoted target in the same thread or the
thread's own title/head. The CLI checks the returned destination and quote;
if the backend does not confirm them, the command fails and reports the write
ID because the post may already exist. Inspect it before retrying; do not
retry with a new ID. This check detects incompatible backends but cannot undo
a post they already created.

`new post --reply-to THREAD_ID` remains a compatible alias for thread posting;
it takes a **thread** ID and does not quote a post. Use the explicit commands
above when starting from a **post** ID.

### Running AI Actions

- Discover model-backed AI actions with `treecli action actions`.
- Root action: `treecli action flux "a glass cathedral in the rain"`.
- Reply action: `treecli action --reply-to <quest-id> animate_kling "animate this as a handheld push-in"`.
- Chatterbox text-to-speech: `treecli action tts "Abigail read this in a crisp narration voice"` or `treecli generate tts "Abigail read this in a crisp narration voice" --out chatterbox.mp3`; alias: `chatterbox`.
- Chatterbox voice clone: `treecli action --reply-to <quest-id> clone "read this in the uploaded voice"` in a thread with audio, or `treecli generate clone "read this in the sampled voice" --reference @voice.mp3 --out clone.mp3`.
- ElevenLabs text-to-speech: `treecli action eleven_tts "read this in a crisp narration voice"` or `treecli generate eleven_tts "read this in a crisp narration voice" --out narration.mp3`; aliases include `eleven`, `elevenlabs`, and `11`.
- Video sound effects: `treecli action sfx "rain, tires on wet asphalt, distant thunder"` or `treecli generate sfx "rain, tires on wet asphalt, distant thunder" --reference @clip.mp4 --out sfx.mp3`; aliases include `sfx`, `mmaudio`, and `foley`.
- Direct existing-image animation: use the base video model with a reference, such as `treecli generate kling2 "slow handheld push-in" --reference @image.png --out animated.mp4`.
- Direct existing-image edit: use the base image model with a reference, such as `treecli generate qwen "replace the sky with stars" --reference @image.png --out edited.png`.
- Treechat action model:
- Use plain AI actions like `flux`, `veo3`, or `kling` to generate a new asset from the prompt.
- Use `animate_*` AI actions to animate an existing image from the thread or attachment context in post-backed `action` workflows.
- Use `edit_*` AI actions to edit an existing image from the thread or attachment context in post-backed `action` workflows.
- Use base video/image actions plus `--reference` for direct `generate` workflows.
- If the user wants to animate or edit a previous image, do not substitute a plain generation action.
- Actions default to private root-thread placement unless you pass a root-only stream flag.
- Use `--payment usd` for Stripe metered AI billing or `--payment bsv` / `--payment bitcoinsv` for Bitcoin SV; omit it to use the account default.
- `treecli action` waits by default and shows a spinner in interactive terminals.
- For post-less local media generation, inspect support with `treecli generate actions --direct-only`.
- Use `treecli generate actions --verbose` or `treecli generate describe <action>` to get model descriptions, accepted inputs, settings, reference behavior, and examples before generating.
- Generate local media with `treecli generate <action> "prompt" --out <file>` and pass settings with `--input key=value`, `--settings '{...}'`, `--duration`, `--instrumental`, `--reference`, or `--payment` as described by the action and billing intent. Direct image edits and image-to-video runs use the base image/video action with explicit reference media because direct generation has no thread context; do not use `edit_*` or `animate_*` with `generate`.

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
- After updating, refresh this guidance block with `treecli onboard agents --write`; verify it with `treecli onboard agents --check`.

### Shell Completion

- For bash and zsh, turn completions on in the current shell with `if [ -n "${ZSH_VERSION:-}" ]; then autoload -U compinit && compinit; source <(treecli completion zsh); elif command -v complete >/dev/null 2>&1; then source <(treecli completion bash); else echo "Current shell does not support bash completion; use zsh or a bash with progcomp."; fi`.
- Use `treecli completion bash` or `treecli completion zsh` directly if you want to install persistent completions.
