# Release Verification

Atlas has three different release states. Keep them separate.

## Source Build

```bash
go build -o tracker ./cmd/tracker
./tracker --help
```

This proves your local checkout can build. It does not prove hosted release provenance.

## Local Rehearsal

```bash
VERSION=v1.8.0-rc1 ./scripts/release-rehearsal.sh
sh scripts/stability-smoke.sh
```

Local rehearsal proves packaging shape, installer behavior, and smoke flow against locally built artifacts.

## Hosted Release Proof

Hosted proof requires published GitHub assets:

1. prerelease tag, such as `v1.8.0-rc1`
2. archives and `checksums.txt` uploaded by GitHub Actions
3. downloaded archive verified by `scripts/verify-release.sh`
4. artifact attestation verified by GitHub
5. clean install from published assets
6. packaged smoke from the installed binary

If any hosted asset is missing, the correct decision is no-ship for that session. Record the blocked command and reason in release evidence.

Read [public release gates](../release/public-release-gates.md) for the current checklist.
