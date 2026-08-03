# Testing Architecture

This document defines the release-confidence system for treecli. Older release
branches and compatibility binaries use the name `treectl`; the same testing
contracts apply to both names.

## Goal

Prevent changes from reaching users when they break a supported command, the
Treechat API contract, installation, self-update, or the release artifacts.
Testing cannot prove that a release has no defects, so the system combines
independent signals at the boundaries where regressions can escape.

## Risk model

The highest-risk boundaries are:

1. Cobra command wiring, process exit status, stdout/stderr, and persisted
   profile state.
2. HTTP methods, paths, authentication headers, payloads, response decoding,
   timeouts, and credential redaction.
3. Archive names and contents, checksums, executable permissions, legacy-name
   compatibility, installation, and self-update.
4. Compatibility with the deployed Treechat backend, which an in-process fake
   server cannot prove.

JSON output, exit codes, environment variables, config files, and the legacy
`treectl` command are public compatibility surfaces. Tests should make
intentional changes to those surfaces explicit.

## Test layers

### 1. Focused Go tests

Use package tests for deterministic behavior and individual HTTP contracts.
API tests use `httptest.Server` and assert the complete request contract as well
as successful, malformed, non-2xx, and timeout responses. Pure formatting and
state-machine tests stay at this layer.

These tests run on every pull request. They should be fast, isolated, safe to
shuffle, and safe under the race detector.

### 2. Compiled-CLI integration tests

The integration suite builds the real executable and invokes it as a child
process with:

- an isolated `XDG_CONFIG_HOME`;
- a local fake Treechat server;
- stdin supplied explicitly for secrets;
- captured stdout, stderr, and exit status.

This layer proves command registration and flag parsing, profile persistence,
request wiring, JSON output, error propagation, and redaction together. It does
not duplicate every package-level edge case.

The minimum blocking journey is:

1. Login and bootstrap.
2. Inspect the redacted saved profile.
3. Read a thread in JSON form.
4. Create a private post and read it back.
5. Read upvalue history when that command is present.
6. Receive a backend error without leaking credentials or identity data.

### 3. Installer and release-artifact tests

Installer tests run `install.sh` with a fake release transport, a real archive,
and an isolated install directory. They verify checksum enforcement, installed
permissions, and that the installed command starts.

The release build and verification scripts are shared by pull-request CI and
the release workflow. Verification happens before publication and checks:

- the full supported OS/architecture archive set;
- `checksums.txt` against every archive;
- exact archive binary names, including the legacy binary where applicable;
- native archive extraction and `--help` execution;
- the stamped version when the source exposes a release version variable.

Cross-compilation proves that every release target compiles. A Linux, macOS,
and Windows CI matrix also builds and starts a native binary. Separate native
coverage for every supported CPU architecture remains an expansion item.

### 4. On-demand QA backend smoke

The QA smoke test is deliberately not a pull-request or nightly job. It is run
on demand against a `treechat-orc` isolated instance before a release.

The smoke test:

1. Checks the Rails health endpoint.
2. Builds or accepts a candidate CLI binary.
3. Logs in with the deterministic QA user using an isolated config directory.
4. Confirms profile output is redacted.
5. Reads notifications and upvalue history.
6. Creates a uniquely named private post.
7. Reads the created thread and its root message back.

The write is intentionally retained in the disposable QA database as evidence
of the run. The script refuses obvious production and shared staging hosts
unless an explicit unsafe override is supplied.

The GitHub workflow has only a `workflow_dispatch` trigger. A GitHub-hosted
runner joins the tailnet as an ephemeral `tag:ci` node and requires:

- `TS_OAUTH_CLIENT_ID` and `TS_AUDIENCE` for Tailscale workload identity;
- `QA_USER_PASSWORD` for the deterministic QA account;
- tailnet access rules allowing `tag:ci` to reach the QA instance.

The same script can be run from a developer machine already connected to the
tailnet.

## Gates and cadence

| Signal | Pull request | Release candidate | Blocking |
| --- | --- | --- | --- |
| Go tests, race detector, shuffle, vet | Yes | Yes | Yes |
| Compiled-CLI integration | Yes | Yes | Yes |
| Installer integration | Yes | Yes | Yes |
| Cross-platform release build and archive verification | Yes | Yes | Yes |
| QA backend smoke | No | On demand | Yes for a public release |
| Production smoke | No | After publication | Observational only |

## Coverage policy

Coverage is a guardrail, not the release definition. CI records a profile and
must not allow critical journeys to disappear even if aggregate coverage stays
flat. Introduce changed-line coverage at 80%, then ratchet package floors from
the measured baseline instead of imposing a repository-wide 80% target.

No broad snapshots are required. Assert stable JSON keys and user-visible
contracts directly; keep prose and help assertions narrow.

## Test data and secrets

- Local integration tests use invented credentials and loopback servers.
- QA credentials are supplied through stdin and GitHub secrets, never command
  arguments or committed config files.
- QA writes include a `treecli-release-smoke` marker and unique timestamp.
- The QA smoke refuses known production and shared staging hosts by default.
- Tests must assert that Treechat authentication headers are never forwarded to
  third-party upload or media hosts.

## Release decision

A release is ready only when the commit has passed the pull-request gates, the
candidate archives have been verified without rebuilding them afterward, and
the on-demand QA smoke has passed against an isolated backend instance. If any
gate fails, fix the candidate and repeat the complete release-candidate run.
