# Atlas Tasker Release Guide

Atlas `v1.8.0-rc1` is planned, not shipped. This guide explains the release workflow and points to the current proof gates.

## Release States

- Source build: proves the local checkout can compile.
- Local rehearsal: proves packaging, install script behavior, and smoke flows against locally built artifacts.
- Hosted release proof: proves published GitHub assets, checksums, attestations, install, and packaged smoke.

A source build or local rehearsal is not enough to call the release ship-ready.

## Artifacts

Each hosted release tag should publish:

- `tracker_<version>_darwin_amd64.tar.gz`
- `tracker_<version>_darwin_arm64.tar.gz`
- `tracker_<version>_linux_amd64.tar.gz`
- `tracker_<version>_linux_arm64.tar.gz`
- `checksums.txt`

Each archive contains a single `tracker` binary.

## Local Rehearsal

```bash
VERSION=v1.8.0-rc1 sh scripts/preflight-release.sh
VERSION=v1.8.0-rc1 ./scripts/release-rehearsal.sh
sh scripts/stability-smoke.sh
```

The preflight checks release script syntax and verifies the stamped `tracker version --json` contract. The rehearsal builds the current binary, packages archives, generates checksums, verifies a local archive, serves local artifacts, installs through `scripts/install.sh`, checks the installed version metadata, and runs the packaged smoke flow.

Local vulnerability and SBOM proof is generated explicitly:

```bash
VERSION=v1.8.0-rc1 RUN_GOVULNCHECK=1 RUN_SBOM=1 sh scripts/preflight-release.sh
```

## Hosted Release Gate

Before public sign-off, a release actor must:

1. run `VERSION=v1.8.0-rc1 sh scripts/preflight-release.sh --hosted`
2. create a prerelease tag such as `v1.8.0-rc1`
3. let GitHub publish archives and `checksums.txt`
4. download at least one published archive
5. run `scripts/verify-release.sh` against that archive with attestation verification enabled
6. install from published assets through `scripts/install.sh`
7. run `tracker version --json` from the installed binary and compare it to the hosted tag, commit, build date, and platform
8. run the packaged smoke flow from the installed binary
9. record checksum, attestation, install, smoke, SBOM, vulnerability scan, and ship/no-ship evidence

Read [public release gates](release/public-release-gates.md) for the source of truth.

## Install

The one-line installer is for published releases:

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
```

Prefer explicit verification for release candidates:

```bash
VERSION=v1.8.0-rc1 ./scripts/verify-release.sh ./tracker_1.8.0-rc1_darwin_arm64.tar.gz
VERSION=v1.8.0-rc1 BIN_DIR="$HOME/.local/bin" sh ./scripts/install.sh
```

`scripts/verify-release.sh` verifies checksums and GitHub artifact attestations by default:

```bash
gh attestation verify ./tracker_1.8.0-rc1_darwin_arm64.tar.gz --repo myrrazor/atlas-tasker
```

Set `VERIFY_ATTESTATIONS=0` only for local rehearsals or intentionally unattested artifacts.

## Packaged Smoke Coverage

The packaged smoke flow should cover:

- workspace init
- project and ticket creation
- agent registration
- run dispatch
- checkpoint, evidence, and handoff
- gate/approval visibility
- doctor and repair
- release install path

PR-807 and PR-809 record the final v1.8 proof transcript.
