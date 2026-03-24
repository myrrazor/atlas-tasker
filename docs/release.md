# Atlas Tasker Release Guide

Atlas Tasker v1.3 ships prebuilt macOS and Linux binaries plus a one-line install script.

## Artifacts

Each release tag publishes these archives:

- `tracker_<version>_darwin_amd64.tar.gz`
- `tracker_<version>_darwin_arm64.tar.gz`
- `tracker_<version>_linux_amd64.tar.gz`
- `tracker_<version>_linux_arm64.tar.gz`
- `checksums.txt`

Each archive contains a single `tracker` binary.

## One-Line Install

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
```

Optional overrides:

```bash
VERSION=v1.2.0 BIN_DIR="$HOME/.local/bin" curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
```

## Manual Install

1. Download the archive for your OS/architecture from the GitHub release.
2. Verify the checksum from `checksums.txt`.
3. Extract the archive.
4. Put `tracker` somewhere on your `PATH`.

## Release Workflow

The release workflow runs on tags matching `v*` and:

1. builds macOS/Linux archives for amd64 and arm64
2. generates SHA256 checksums
3. uploads the archives and `checksums.txt` to the GitHub release
4. publishes `scripts/install.sh` alongside the release assets for reference

## Prerelease Rehearsal

Before cutting a real release:

1. create a prerelease tag
2. let GitHub build real artifacts
3. install with the published install script into a clean temp directory
4. run a smoke flow end to end

Suggested smoke flow:

```bash
tracker init
tracker project create APP "App Project"
tracker ticket create --project APP --title "Smoke" --type task --actor human:owner
tracker ticket move APP-1 ready --actor human:owner
tracker queue --actor human:owner
tracker tui --actor human:owner
```

Release is not done until that flow works against the real published artifacts.
