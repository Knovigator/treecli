---
name: treecli-basic-usage
description: Use treecli to authenticate environment/account pairs, read Treechat threads, create posts, and create replies with the current CLI behavior.
---

# treecli Basic Usage

Use this skill when you need to interact with Treechat through the `treecli` CLI instead of hand-rolling API calls.

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

## Reading Existing Data

- Get your newest authored root thread: `treecli get threads --user me --root --limit 1`.
- List another user's authored branches: `treecli get threads --user alice --branch --page 2 --limit 20`.
- Listing defaults to your threads, includes authorized private content in the current space, and sorts by creation time (newest first), then ID. Omit `--root`/`--branch` to include both; they cannot be combined.
- `--limit` is page size (1–100, default 20). `--page` starts at 1. JSON collections contain `threads` and `pagination` (`page`, `limit`, `next_page`, `has_more`). Page boundaries can move when data changes; this is not a snapshot export.
- Fetch all accessible child threads with `treecli get threads --answer <answer-id>`. An answer may have zero or multiple children; JSON contains a `threads` array without pagination.
- ID and answer lookups cannot be combined with listing filters. A single thread lookup keeps the existing `quest` JSON envelope.
- `get thread` is deprecated; it still works and warns on stderr. Authored listing requires a backend with the authored collection API; an older backend produces a compatibility error.
- Fetch a thread with `treecli get threads <quest-id>`.
- Fetch one or more answers with `treecli get messages <answer-id> [...]`.
- Add `--json` when another tool needs structured output.

## Creating Posts

- Root post: `treecli new post "hello world"`.
- Reply: `treecli new post --reply-to <quest-id> "hello back"`.
- Root posts default to private placement.
- Root posts can target a stream with `--stream private`, `--stream public`, `--stream clips`, a stream name, or a stream UUID.
- Replies inherit thread placement, so do not pass root-only stream flags with `--reply-to`.

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

## Working Rules

- Prefer `treecli` over raw API calls when the CLI already supports the flow.
- Prefer human-readable output while reasoning, and switch to `--json` only when a downstream tool needs structured data.
- Use the selected environment and account consistently so auth and links match the intended environment.
- Check for a newer CLI release with `treecli update --check`; install it with `treecli update`. Use `treecli update --json` when another tool needs structured update status.
