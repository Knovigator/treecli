# Onboarding Architecture

How `treecli onboard` is structured, why it is shaped that way, and the
invariants the implementation maintains. Code lives in `cmd/onboard.go`
(command surface) and `content/packaged.go` + `content/onboard/*.md`
(packaged content).

## Goals

Onboarding serves two audiences with different needs:

- **Humans** need to know *where their setup stands* and the next command to
  run — not a document. Reading state and printing a checklist beats printing
  instructions that may already be satisfied.
- **Agents** need a way to install treecli guidance into a project's
  instruction file (`AGENTS.md` / `CLAUDE.md`) that is **idempotent** — safe
  to run on every session without duplicating content — plus a machine-checkable
  way to verify it and structured output for orchestration.

Everything runs **offline**: no network calls, so `onboard` is instant and
safe to run in any environment. Update checks stay in `treecli update --check`.

## Command surface

```
treecli onboard                 # status checklist + next steps (--json for structured)
treecli onboard agents          # print the guidance block (--short | --long)
treecli onboard agents --write  # install/update the block idempotently (--file to target)
treecli onboard agents --check  # verify present + current; non-zero exit otherwise
treecli onboard guide           # full onboarding guide document
```

The split is deliberate: the bare command is *stateful and personal* (reads
your profile, your directory, your skill dirs), while `agents` and `guide` are
*pure content emitters* whose output is stable for a given CLI build. Tooling
can depend on `agents` output being exactly the packaged block and nothing else.

### Legacy compatibility

The pre-subcommand flags — `--agents-md`, `--short`, `--long`, `-o/--output`
on the root command — keep their exact old behavior because they are
documented in previously-installed guidance blocks in the wild
(`treecli onboard --agents-md >> AGENTS.md`). Setting any of them routes to
`runOnboardLegacy`, which is a frozen copy of the old code path. They are
marked deprecated via pflag, which hides them from `--help` and prints a
pointer to the replacement on use. Deprecation notices go to **stderr**, so
existing stdout pipes and redirects are unaffected.

Removal policy: keep the legacy flags at least until guidance blocks that
reference them have aged out (they now instruct agents to refresh via
`onboard agents --write`, which rewrites the instructions themselves).

## The managed guidance block

`--write` never blind-appends. The block is wrapped in marker comments:

```markdown
<!-- treecli:onboard:begin variant=long (managed by `treecli onboard agents --write`; edits inside are overwritten) -->
## treecli CLI Guidance
...
<!-- treecli:onboard:end -->
```

Invariants:

- **One block per file.** Upsert finds the begin marker prefix
  (`<!-- treecli:onboard:begin`) and replaces everything through the end
  marker. Content outside the markers is preserved byte-for-byte.
- **Idempotent.** Re-running `--write` on a current block reports `unchanged`
  and does not touch the file (no mtime churn, no VCS noise).
- **Variant is recorded, not remembered.** The begin marker carries
  `variant=short|long`. A variant-less `--write` preserves the recorded
  variant; an explicit `--short`/`--long` switches it. Nothing about the
  block's state lives outside the file itself.
- **Corruption is an error, not a guess.** A begin marker without an end
  marker aborts with instructions rather than risking a mangled rewrite.

The upsert is a four-outcome state machine, reported to the user verbatim:

| Existing file state          | Action      |
| ---------------------------- | ----------- |
| missing / empty              | `created`   |
| content, no markers          | `appended`  |
| markers, differing content   | `updated`   |
| markers, identical content   | `unchanged` |

### Staleness

`--check` (and the bare-command checklist) re-renders the block for the
file's recorded variant from the *current binary* and compares it to what is
between the markers. Any drift — hand edits inside the markers, or a block
written by an older CLI whose packaged content has since changed — reads as
`stale`. There is no version number to keep honest; the content comparison
*is* the version check. `--check` exits non-zero unless at least one target
file carries a current block, which makes it usable as a CI gate or an agent
precondition.

### Target resolution

With `--file`, exactly that file. Without it, `--write` resolves targets in
the current directory in priority order:

1. Every candidate (`AGENTS.md`, `CLAUDE.md`) that **already has a block** —
   all of them are refreshed, so a repo maintaining both stays consistent.
2. Otherwise the first candidate that **exists** — join the project's
   established instruction file rather than creating a second one.
3. Otherwise **create `AGENTS.md`** (the cross-agent convention).

## Status collection (bare `onboard`)

`collectOnboardStatus` gathers, without network access:

- **CLI version** — `CurrentVersion` (release-stamped, `dev` locally).
- **Profile + login** — `resolveProfileName`/`resolveProfile` from the login
  subsystem; "signed in" is the same access-token/client/uid triple the rest
  of the CLI uses. Credentials are never printed, only the boolean.
- **Agent files** — marker state (`absent`/`current`/`stale`) and recorded
  variant for each candidate in the current directory.
- **Skills** — for each install target (`~/.claude/skills`, `~/.codex/skills`,
  `~/.pi/agent/skills`), how many packaged skills (from
  `content.ListPackagedSkills`) are present, detected by `SKILL.md` existence.

`buildOnboardNextSteps` derives remediation from that status — each unmet item
becomes a description plus the exact command. The human renderer prints the
checklist and numbered steps; `--json` emits the same struct
(`onboardStatus`), so the two views cannot drift. Only unmet steps appear:
a fully onboarded user sees example commands to try instead.

## Packaged content pipeline

Content is embedded in the binary via `go:embed` (`content/packaged.go`):

- `onboard/agents-long.md` / `agents-short.md` — the guidance block variants.
- `onboard/wrapper.md` — the guide document; `BuildOnboardContent` splices a
  variant into its `{{AGENTS_MD_BLOCK}}` placeholder.

Because the block ships inside the binary, the block a user has installed is
exactly as old as their binary — which is why the guidance block itself tells
agents to run `onboard agents --write` after `treecli update`, and why
staleness is defined by comparison against the running binary. The content
files are the single source of truth: editing them changes `agents`, `guide`,
staleness detection, and the legacy flags together.

## Testing

`cmd/onboard_test.go` covers the invariants above: the create/append/update/
unchanged transitions, byte-identical idempotent re-runs, variant preservation
and explicit switching, surrounding-content preservation, missing-end-marker
rejection, staleness after in-block edits, target-resolution priority, and
next-step derivation. Tests run in per-test temp directories and never touch
the user's config or home directory.
