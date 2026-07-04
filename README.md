# treecli

`treecli` is the command-line interface for Treechat automation.

## Install

Once releases are published, install the latest macOS or Linux binary:

```sh
curl -fsSL https://raw.githubusercontent.com/Knovigator/treecli/main/install.sh | sh
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/Knovigator/treecli/main/install.sh | TREECLI_VERSION=0.1.0 sh
```

The installer downloads the matching GitHub Release archive, verifies it against `checksums.txt`, and installs `treecli` to `~/.local/bin` by default. Override that with `TREECLI_INSTALL_DIR`.

During the rename from `treectl`, releases also publish compatibility archives and the installer also writes a `treectl` command by default. Existing `TREECTL_*` environment variables are still accepted as fallbacks, but new automation should use `treecli` and `TREECLI_*`.

Go users can also install directly:

```sh
go install github.com/Knovigator/treecli@latest
```

Update an installed release in place:

```sh
treecli update
```

Self-update currently supports the macOS and Linux release archives. Existing `treectl` binaries can run `treectl update` and receive the new code path from the compatibility release archive.

Check whether a newer release is available without installing it:

```sh
treecli update --check
```

## Development

```sh
go test ./...
go run . --help
```

## Release

Create a public CLI release by pushing a normal version tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The `release` GitHub Actions workflow builds:

- macOS amd64 and arm64
- Linux amd64 and arm64
- Windows amd64

It uploads the archives and `checksums.txt` to the GitHub Release.

If the tag already exists and you need to rerun the release through GitHub CLI:

```sh
gh workflow run release.yml --ref main -f tag=v0.1.0
```

Inspect a finished release with:

```sh
gh release view v0.1.0 --repo Knovigator/treecli
```

## Agent Usage

Agents should install `treecli`, authenticate with `treecli login` or supported `TREECLI_*` environment variables, and rely on server-side authorization for all Treechat access. `TREECTL_*` variables only exist for migration from old installs. Do not distribute tokens inside release artifacts.

## License

`treecli` is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
