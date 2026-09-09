# Updating Atlas Tasker

`tracker update` checks GitHub Releases for a newer Atlas binary. Applying an update downloads the
archive for the current operating system and architecture, verifies its checksum, verifies its GitHub
build attestation by default, and replaces the executable that ran the command.

## Check Before Replacing Anything

```bash
tracker update --check
tracker update --check --json
tracker update --dry-run
```

`--check` reports whether a newer release is available. `--dry-run` also resolves the target archive
but does not download it or replace the executable. Stable JSON statuses are `up_to_date`,
`update_available`, `would_update`, and `updated`.

## Apply The Latest Release

```bash
tracker update --yes
tracker version --json
```

Atlas requires `--yes` before it replaces the current executable. The install directory must be
writable by the current user. Replacement is staged next to the existing binary, preserves its file
mode, and rolls the old binary back into place if the final rename fails.

Attestation verification uses `gh attestation verify`, so the GitHub CLI must be installed unless
you explicitly pass `--skip-attestations`. Skipping attestations still verifies `checksums.txt`, but
it gives up the additional build-provenance check.

## Pin Or Reinstall A Version

```bash
tracker update --version v1.11.0 --dry-run
tracker update --version v1.11.0 --yes
```

A same-version target reports `up_to_date` and leaves the binary unchanged. A downgrade
fails unless it is explicitly allowed. Add `--force` for an intentional reinstall or downgrade:

```bash
tracker update --version v1.11.0 --force --yes
```

An unstamped source build reports its current version as `dev` and can update to a published release.
The examples above pin a historical release. The [latest release page](https://github.com/myrrazor/atlas-tasker/releases/latest)
identifies the current stable version and records its hosted verification.

## What Update Does Not Change

The command replaces only the running `tracker` binary. It does not edit workspaces, refresh
agent-integration files, register MCP clients, or change agent/provider configuration. To refresh an
installed `atlas-worker` pack after upgrading, re-run its explicit install command:

```bash
tracker integrations install codex
```

See [coding-agent integrations](agent-integrations.md) for all six targets and
[release verification](release-verification.md) for the difference between a local build and a
published release.
