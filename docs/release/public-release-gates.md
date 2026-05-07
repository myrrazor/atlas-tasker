# Public Release Gates

Atlas Tasker can look polished before it is release-ready. This file is the public release gate source of truth for v1.8.

## Docs-Only Polish Gate

Docs and examples can merge when:

- `git diff --check` passes
- `go test ./...` passes
- `go vet ./...` passes
- docs links and snippets touched by the PR are checked
- public security wording follows `docs/security-limitations.md`

Passing this gate does not mean Atlas is ship-ready.

## Local RC Gate

`v1.8.0-rc1` local RC proof requires:

- full tests: `go test -count=1 ./... 2>&1 | tee TEST_STDOUT.log`
- `go vet ./...`
- `VERSION=v1.8.0-rc1 sh scripts/preflight-release.sh`
- `sh scripts/stability-smoke.sh`
- `VERSION=v1.8.0-rc1 ./scripts/release-rehearsal.sh`
- `go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ./...`
- CycloneDX SBOM generation for the local release target with `cyclonedx-gomod@v1.10.0`
- no private key or obvious secret material in logs, transcripts, docs assets, or `TEST_STDOUT.log`
- final GStack/Codex review attempts recorded
- GStack CSO run recorded, with verified in-scope findings fixed or explicitly deferred

Passing this gate means the local RC is green. It still does not prove hosted release assets.

## Hosted RC Gate

Hosted release sign-off requires:

1. `VERSION=v1.8.0-rc1 sh scripts/preflight-release.sh --hosted` passes with a token that can read required GitHub Actions release settings.
2. A prerelease tag such as `v1.8.0-rc1` is created.
3. GitHub publishes all supported archives and `checksums.txt`.
4. At least one published archive is downloaded from GitHub.
5. `scripts/verify-release.sh` verifies checksum and attestation for the downloaded archive.
6. `scripts/install.sh` installs from published release assets into a clean directory.
7. The installed binary reports `tracker version --json` matching the hosted tag, commit, build date, and platform.
8. The installed binary completes the packaged smoke flow.
9. The release evidence records archive name, checksum result, attestation result, install result, version metadata result, packaged smoke result, SBOM result, vulnerability result, and final ship/no-ship decision.

## Stable Release Gate

A stable public release requires the hosted RC gate plus owner sign-off in `docs/release/v1.8-release-evidence.md`. If any hosted gate is blocked, the decision must be `no-ship` with the exact blocked command and reason code.
