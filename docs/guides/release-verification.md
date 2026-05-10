# Release Verification

Atlas has three different release states. Keep them separate.

## Source Build

```bash
go build -o tracker ./cmd/tracker
./tracker --help
./tracker version --json
```

This proves your local checkout can build and that the version JSON contract is available. Unstamped source builds report `version: "dev"`, `commit: "unknown"`, and `build_date: "unknown"`. They do not prove hosted release provenance.

## Local Rehearsal

```bash
VERSION=v1.8.0-rc1 sh scripts/preflight-release.sh
VERSION=v1.8.0-rc1 ./scripts/release-rehearsal.sh
sh scripts/stability-smoke.sh
```

Local rehearsal proves packaging shape, installer behavior, and smoke flow against locally built artifacts.

For the local security proof files used in release evidence:

```bash
VERSION=v1.8.0-rc1 \
RUN_GOVULNCHECK=1 \
RUN_SBOM=1 \
RELEASE_PROOF_DIR=docs/release-proof \
sh scripts/preflight-release.sh
```

This uses pinned tooling:

- `govulncheck@v1.3.0`
- `cyclonedx-gomod@v1.10.0`

## Hosted Release Proof

Hosted proof requires published GitHub assets:

1. run `VERSION=v1.8.0-rc1 sh scripts/preflight-release.sh --hosted`
2. create a prerelease tag, such as `v1.8.0-rc1`
3. confirm archives and `checksums.txt` uploaded by GitHub Actions
4. download at least one hosted archive
5. run `VERSION=v1.8.0-rc1 ./scripts/verify-release.sh <archive>` with attestation verification enabled
6. install from published assets with `VERSION=v1.8.0-rc1 BIN_DIR=<clean-dir> sh scripts/install.sh`
7. run `<clean-dir>/tracker version --json` and compare version, commit, build date, and platform to the hosted asset
8. run the packaged smoke from the installed binary

If any hosted asset is missing, the correct decision is no-ship for that session. Record the blocked command and reason in release evidence.

Read [public release gates](../release/public-release-gates.md) for the current checklist.
