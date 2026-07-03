# treectl

`treectl` is the command-line interface for Treechat automation.

## Install

Once releases are published, install the latest macOS or Linux binary:

```sh
curl -fsSL https://raw.githubusercontent.com/Knovigator/treectl/main/install.sh | sh
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/Knovigator/treectl/main/install.sh | TREECTL_VERSION=0.1.0 sh
```

The installer downloads the matching GitHub Release archive, verifies it against `checksums.txt`, and installs `treectl` to `~/.local/bin` by default. Override that with `TREECTL_INSTALL_DIR`.

Go users can also install directly:

```sh
go install github.com/Knovigator/treectl@latest
```

Update an installed release in place:

```sh
treectl update
```

Self-update currently supports the macOS and Linux release archives.

Check whether a newer release is available without installing it:

```sh
treectl update --check
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
gh release view v0.1.0 --repo Knovigator/treectl
```

## Agent Usage

Agents should install `treectl`, authenticate with `treectl login` or supported `TREECTL_*` environment variables, and rely on server-side authorization for all Treechat access. Do not distribute tokens inside release artifacts.

## License

`treectl` is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
