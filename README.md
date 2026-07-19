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
treecli onboard          # checklist: profile, login, agent guidance, skills
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

## Common Commands

```sh
treecli profile list
treecli profile show
treecli login --profile dev

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

## Development

```sh
go test ./...
go run . --help
```

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

Agents should install `treecli`, run `treecli onboard` to see what setup remains, authenticate with `treecli login` or supported `TREECLI_*` environment variables, install project guidance with `treecli onboard agents --write`, inspect model capabilities with `treecli generate actions --verbose` or `treecli generate describe <action>`, and rely on server-side authorization for all Treechat access. Do not distribute tokens inside release artifacts.

## License

`treecli` is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
