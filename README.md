# treecli

`treecli` is the command-line interface for Treechat automation. It lets humans and agents read Treechat threads, create posts, submit AI action requests, generate local media, and keep the CLI updated from GitHub Releases.

## Install

Install the latest macOS or Linux binary:

```sh
curl -fsSL https://raw.githubusercontent.com/Knovigator/treecli/main/install.sh | sh
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/Knovigator/treecli/main/install.sh | TREECLI_VERSION=0.2.0 sh
```

The installer downloads the matching GitHub Release archive, verifies it against `checksums.txt`, and installs `treecli` to `~/.local/bin` by default. Override that with `TREECLI_INSTALL_DIR`.

Go users can also install directly:

```sh
go install github.com/Knovigator/treecli@latest
```

Update an installed release in place:

```sh
treecli update
```

Self-update currently supports the macOS and Linux release archives.

Check whether a newer release is available without installing it:

```sh
treecli update --check
```

## Migration From treectl

The project was renamed from `treectl` to `treecli` in `v0.2.0`.

- New automation should use `treecli`, `TREECLI_*` environment variables, and the `Knovigator/treecli` repo.
- Existing `treectl` binaries can run `treectl update` or `treectl update v0.2.0`.
- Release archives still include a `treectl` compatibility binary, and the installer writes it by default. Set `TREECLI_INSTALL_LEGACY=0` to skip that compatibility command.
- Existing `TREECTL_*` environment variables and `treectl/config.toml` are accepted as migration fallbacks.

## Onboarding

Check where your setup stands and what to do next:

```sh
treecli onboard          # checklist: environment/account, login, agent guidance, skills
treecli onboard --json   # machine-readable status
```

Give coding agents treecli guidance by installing a managed block into the
project's instruction file. The block is wrapped in marker comments and
updated in place, so re-running never duplicates it:

```sh
treecli onboard agents --write                 # AGENTS.md (or existing CLAUDE.md)
treecli onboard agents --write --file CLAUDE.md
treecli onboard agents --check                 # non-zero exit if missing or stale
treecli onboard agents                         # print the raw block instead
```

`treecli onboard guide` prints the full onboarding document, and
`treecli skills install all --claude` (or `--codex` / `--pi`) installs the
packaged agent skills. Design details are in
[docs/onboarding-architecture.md](docs/onboarding-architecture.md).

## Authentication

Create a Treechat account interactively:

```sh
treecli signup
```

The command prompts for a username and email, then reads the password and its
confirmation without echoing either value. A successful signup saves the
authenticated account, so a separate `treecli login` is not required.

Commands default to production. Use `treecli --account NAME login` to save a
login, or `treecli --env dev --account NAME login` for local development.

## Common Commands

```sh
treecli account list
treecli account show
treecli whoami
treecli login

treecli get thread <quest-id>
treecli get messages <answer-id> [...]

treecli new post "hello world"
treecli new post --reply-to <quest-id> "reply text"
```

## AI Actions And Media

Use action requests for Treechat-posted AI work:

```sh
treecli action actions
treecli action flux "a glass cathedral in the rain"
treecli action flux "a glass cathedral in the rain" --payment usd
treecli action tts "Abigail read this in a crisp narration voice"
treecli action eleven_tts "read this in a crisp narration voice"
treecli action sfx "rain, tires on wet asphalt, distant thunder"
treecli action --reply-to <quest-id> animate_kling "animate this still"
treecli action status --answer <answer-id> --watch
```

Use direct generation when an agent or script needs local media files without creating a post:

```sh
treecli generate actions --direct-only
treecli generate actions --verbose
treecli generate describe flux2
treecli generate flux2 "wide cinematic hero banner" --out banner.webp --input aspect_ratio=3:1
treecli generate flux2 "wide cinematic hero banner" --out banner.webp --payment bsv
treecli generate kling2 "slow handheld push-in" --reference @image.png --out animated.mp4
treecli generate qwen "replace the sky with stars" --reference @image.png --out edited.png
treecli generate tts "Abigail read this in a crisp narration voice" --out chatterbox.mp3
treecli generate clone "read this in the sampled voice" --reference @voice.mp3 --out clone.mp3
treecli generate eleven_tts "read this in a crisp narration voice" --out narration.mp3
treecli generate sfx "rain, tires on wet asphalt, distant thunder" --reference @clip.mp4 --out sfx.mp3
treecli generate suno "warm ambient build, 122 BPM" --duration 20 --out sketch.mp3
```

`treecli action` and `treecli generate` accept `--payment usd` for Stripe metered AI billing or `--payment bsv` / `--payment bitcoinsv` for Bitcoin SV. Omit `--payment` to use the account default.

`tts` accepts the CLI alias `chatterbox`. `eleven_tts` accepts CLI aliases `eleven`, `elevenlabs`, and `11`. `video_sfx` accepts CLI aliases `sfx`, `mmaudio`, and `foley`.

`treecli generate` supports repeatable `--input key=value`, JSON `--settings`, `--duration`, `--instrumental`, and `--reference run:<id>|https://...|@path`. For direct edits or image-to-video runs, use the base image/video action with explicit `--reference` media because direct generation has no thread context to infer it from. Clone and video sound-effect actions also require explicit reference media. Use `treecli generate describe <action>` before generating when an agent needs model descriptions, accepted inputs, settings, examples, and reference behavior.

## Environments and accounts

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

### Checking which account you are using

```sh
treecli whoami                 # verify the currently selected account
treecli whoami --json          # environment, account, backend_url, username, user_id
treecli account use gm-bot
treecli whoami                 # verify the newly selected default
```

`whoami` makes a live authenticated request. It reports the server's username
and user ID, even if the saved ID is stale. Expired/invalid credentials,
network errors, or missing identity data fail without reporting a saved identity.
It does not switch accounts or rewrite configuration. `account show` remains
an offline view of saved configuration and credentials (redacted).

You normally need no selector. Use `--env staging whoami` to check the selected
staging account, or `--account OTHER whoami` only to inspect a different saved
account without changing your default. A local account label is not proof of
the username; `whoami` verifies the actual login.

### Migrating from profiles

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

## Reading threads

```sh
treecli get threads --user me --root --limit 1
treecli get threads --user alice --branch --page 2 --limit 20 --json
treecli get threads THREAD_ID
treecli get threads --answer ANSWER_ID --json
```

Listing defaults to your authored threads in the current space, newest-created
first (ID breaks ties). `--user` accepts a username, user ID, or `me`; prefix a
username with `@` to disambiguate it from an ID or `me`. `--root` and `--branch`
are mutually exclusive; omitting both includes both. Visibility is enforced by
the server, including access to your private threads.

`--limit` sets page size (1–100, default 20); `--page` defaults to 1. Collection
JSON contains `threads` and `pagination` with `page`, `limit`, `next_page`, and
`has_more`. Changing data can move page boundaries. Answer lookup returns all
accessible child threads, including an empty array when none exist, without
pagination. Thread-ID lookup retains the existing `quest` JSON envelope.
Lookup modes cannot be combined with listing filters.

`get thread` remains compatible but is deprecated and warns on stderr. Authored
listing requires the backend's authored collection support; older backends
produce an explicit compatibility error.

## Replying to a specific post

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

## Recording Agent Sessions (MCP)

`treecli mcp` makes treecli a plugin for the coding agents you already run —
Claude Code, Codex, and Grok — so their sessions are recorded into
[memdb](docs/agent-recording-architecture.md) (a local durable-memory store:
SQLite + FTS5, optional Qdrant hybrid search) and, optionally, mirrored into a
Treechat stream. The same binary serves the memories back over MCP.

```sh
treecli mcp install --claude --codex --grok     # register the MCP server + hooks
treecli mcp backfill --claude --codex --since 30d   # ingest transcripts you already have
treecli mcp status
```

After `install`, each agent launches `treecli mcp serve` as a stdio MCP server
(tools: `memory_recall`, `memory_search`, `memory_learn`, `session_note`,
`treechat_post`, `session_info`) and runs `treecli mcp hook` on
`SessionStart` / `UserPromptSubmit` / `Stop` / `SessionEnd`. Hooks convert the
host transcript into memdb's ingest format and inject context: recent
exchanges from earlier sessions in the same project at session start, and
matching learned memories on each prompt. Codex asks you to trust the new
hooks once (`/hooks`); the Grok Bot desktop app needs the server added in its
own MCP settings (the installer prints the snippet).

Treechat recording is off until you choose a stream:

```sh
treecli mcp config --treechat-mode session --treechat-team <stream id or link>
treecli mcp config --treechat-mode turns    # also one reply per turn
```

`session` posts an opening post and a closing summary per session; `turns`
adds one reply per turn (prompt, final reply, tool summary). Posts use
deterministic ids, so retries never duplicate. Backfills never post.

memdb-rust is found on `PATH`, at `~/src/memdb-rust/target/release/memdb-rust`,
or wherever `treecli mcp config --memdb-bin` points; the database defaults to
`memdb.sqlite3` in treecli's data directory (`treecli mcp status` shows it).

## Development

```sh
go test -mod=readonly -race -shuffle=on -count=1 ./...
go run . --help
```

The testing strategy, release gates, and on-demand QA workflow are documented
in [docs/testing-architecture.md](docs/testing-architecture.md). The phased
rollout is tracked in
[docs/testing-implementation-plan.md](docs/testing-implementation-plan.md).

Before a release, run the backend smoke against an isolated `treechat-orc`
instance from a machine connected to the tailnet:

```sh
QA_API_URL=https://molt-bot.tail206f1c.ts.net:<rails-port> \
QA_APP_URL=https://molt-bot.tail206f1c.ts.net:<vite-port> \
QA_USER_PASSWORD='<qa-password>' \
scripts/qa-smoke.sh
```

The `qa-smoke` GitHub workflow exposes the same check through a manual
`workflow_dispatch`; it has no pull-request, push, or scheduled trigger.

## Release

Create a public CLI release by pushing a normal version tag:

```sh
git tag v0.2.1
git push origin v0.2.1
```

The `release` GitHub Actions workflow builds:

- macOS amd64 and arm64
- Linux amd64 and arm64
- Windows amd64

It uploads the archives and `checksums.txt` to the GitHub Release.

If the tag already exists and you need to rerun the release through GitHub CLI:

```sh
gh workflow run release.yml --ref main -f tag=v0.2.1
```

Inspect a finished release with:

```sh
gh release view v0.2.0 --repo Knovigator/treecli
```

## Agent Usage

Agents should install `treecli`, run `treecli onboard` to see what setup remains, authenticate with `treecli signup`, `treecli login`, or supported `TREECLI_*` environment variables, install project guidance with `treecli onboard agents --write`, inspect model capabilities with `treecli generate actions --verbose` or `treecli generate describe <action>`, and rely on server-side authorization for all Treechat access. Do not distribute tokens inside release artifacts.

## License

`treecli` is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
