# Release Scripts

Release scripts are proof helpers, not marketing status.

- `scripts/install.sh`: verifies and installs a published archive.
- `scripts/verify-release.sh`: verifies checksums and GitHub attestations by default.
- `scripts/release-rehearsal.sh`: builds local archives and runs an installer smoke.
- `scripts/stability-smoke.sh`: runs a local end-to-end smoke.
- `scripts/preflight-release.sh`: release-manager entrypoint for local preflight checks and optional security proof generation.
- `scripts/validate-rc.sh`: offline RC polish gate for docs, snippets, release strings, leakage, quickstart, terminal output, parity, and performance budgets.
- `scripts/preflight-release-proof.sh`: checks repository release prerequisites available to the current GitHub token.

Use:

```bash
VERSION=v1.8.0-rc1 sh scripts/preflight-release.sh
VERSION=v1.8.0-rc1 sh scripts/validate-rc.sh
VERSION=v1.8.0-rc1 ./scripts/release-rehearsal.sh
VERSION=v1.8.0-rc1 ./scripts/verify-release.sh ./tracker_1.8.0-rc1_darwin_arm64.tar.gz
```

`scripts/install.sh`, `scripts/verify-release.sh`, `scripts/preflight-release.sh`, `scripts/release-rehearsal.sh`, and `scripts/validate-rc.sh` validate `VERSION` before using it for stamped builds or downloads. Release downloads require `https://` unless a local rehearsal explicitly sets `ALLOW_INSECURE_RELEASE_BASE_URL=1` for loopback HTTP.

`scripts/preflight-release.sh` is POSIX `sh` compatible. By default it checks shell syntax, builds a stamped local binary, and verifies the `tracker version --json` contract. It does not claim hosted release proof.

`scripts/validate-rc.sh` is offline and temp-workspace based. It builds a stamped temp binary, checks public docs links and command snippets, scans current public release text for stale RC versions, scans README/docs/examples/scripts for obvious secret or local-path leakage, runs the README quickstart flow in a clean git workspace, checks `NO_COLOR` and non-TTY-style output at 40/80 columns, compares CLI JSON with slash-shell JSON for board/dashboard/ticket view, verifies required MCP read-profile tools, and enforces these local performance budgets:

- `tracker version --json`: 2s
- `tracker board --json`: 3s
- `tracker dashboard --json`: 3s
- `tracker goal brief APP-1 --md`: 3s

The stale-version scan excludes historical version plans. The leakage scan includes release-proof artifacts and allowlists only sanitizer/test-pattern lines inside the validation and demo-generation scripts.

For local security proof:

```bash
VERSION=v1.8.0-rc1 \
RUN_GOVULNCHECK=1 \
RUN_SBOM=1 \
RELEASE_PROOF_DIR=docs/release-proof \
sh scripts/preflight-release.sh
```

Pinned tools:

- `go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 -show verbose ./...`
- `go run github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@v1.10.0 app -json -output docs/release-proof/sbom-v1.8.0-rc1.cdx.json -main cmd/tracker .`

The wrapper writes proof files under `RELEASE_PROOF_DIR` only when the corresponding `RUN_*` flag is set. Failures block local RC proof and final ship evidence. Hosted proof still requires:

```bash
VERSION=v1.8.0-rc1 sh scripts/preflight-release.sh --hosted
```

Hosted mode delegates to `scripts/preflight-release-proof.sh` and preserves that script's `preflight_status`, `reason_code`, and exit behavior.

If hosted assets do not exist, verification cannot complete and release evidence must say no-ship.
