# Testing Implementation Plan

This plan implements the architecture in
[`testing-architecture.md`](testing-architecture.md) incrementally. Each phase
adds an independent signal and can ship without waiting for a coverage rewrite.

## Phase 1: Foundation and pull-request gates

Status: implemented in this change.

- Run all Go tests with dependency immutability, race detection, randomized
  order, a fresh test process, and a coverage profile.
- Keep `go vet` blocking.
- Add a compiled-binary integration package with isolated configuration and a
  fake Treechat backend.
- Cover login/bootstrap, profile redaction, read, write, upvalue history, and
  redacted backend failures at the process boundary.

Exit criterion: a broken command registration, non-zero exit contract,
credential persistence path, or critical HTTP route fails pull-request CI.

## Phase 2: Installer and release artifacts

Status: implemented in this change.

- Exercise `install.sh` with a fake release transport and real checksums.
- Share release build logic between CI and the publication workflow.
- Verify all archive names, contents, and checksums before publication.
- Extract and execute the native candidate binary.

Exit criterion: CI rejects missing, corrupt, misnamed, non-executable, or
non-starting native release artifacts.

## Phase 3: On-demand QA backend smoke

Status: implemented in this change.

- Add `scripts/qa-smoke.sh` for local or CI use.
- Add a manual-only GitHub workflow that joins Tailscale ephemerally.
- Run the smoke against a `treechat-orc` instance before a public release.
- Keep QA credentials in stdin/GitHub secrets and reject production-like hosts.

Exit criterion: login, authenticated reads, a private write, and read-after-write
all succeed against the deployed QA backend.

## Phase 4: Expand endpoint contracts

Status: pending follow-up work.

Add focused `httptest` coverage for every remaining API method, prioritizing:

1. Thread, message, notification, and leaderboard reads.
2. Quest, answer, and clip writes, including attachment variants.
3. Generation polling, failure, timeout, and multi-output downloads.
4. Billing status/mode/sync and wallet command orchestration.
5. All malformed JSON, non-2xx, and sensitive-error responses.

Exit criterion: every exported API operation has at least one complete request
contract and response/error contract test.

## Phase 5: Compatibility and complete native platform matrix

Status: major-OS native smoke implemented; architecture and upgrade expansion
remain pending.

- Keep the implemented native Linux, macOS, and Windows build/start matrix.
- Expand it to execute release archives on native Linux amd64/arm64, macOS
  amd64/arm64, and Windows amd64 runners.
- Update from the previous two supported releases to the candidate and execute
  the installed result.
- Assert config, wallet, environment-variable, JSON, and legacy command
  compatibility across the upgrade.
- Add changed-line coverage of at least 80% and ratchet package floors.

Exit criterion: the candidate is proven on every supported native platform and
through every supported upgrade path.

## Release checklist

1. Merge only after the pull-request gates pass.
2. Launch or select an isolated `treechat-orc` QA instance.
3. Run the manual QA smoke workflow with its Rails and app URLs.
4. Build the candidate archives once.
5. Verify and publish those exact archives.
6. Install the published release on one macOS and one Linux machine as a final
   observational check.
