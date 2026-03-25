# Atlas Tasker Release Guide

Atlas Tasker v1.4 ships prebuilt macOS and Linux binaries plus a one-line install script.

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
VERSION=v1.4.0 BIN_DIR="$HOME/.local/bin" curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
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
git init -b main
git config user.email atlas@example.com
git config user.name Atlas
printf '# atlas\n' > README.md
git add README.md
git commit -m init

tracker init
tracker project create APP "App Project"
tracker ticket create --project APP --title "Smoke" --type task --actor human:owner
tracker ticket move APP-1 ready --actor human:owner
tracker agent create builder-1 --name "Builder One" --provider codex --capability go --actor human:owner
tracker run dispatch APP-1 --agent builder-1 --actor human:owner
tracker run launch <RUN-ID> --actor human:owner
tracker run start <RUN-ID> --actor human:owner
tracker ticket move APP-1 in_progress --actor human:owner
tracker run checkpoint <RUN-ID> --title "Smoke checkpoint" --body "runtime + worktree ready" --actor human:owner
tracker run handoff <RUN-ID> --next-actor agent:reviewer-1 --next-gate review --actor human:owner
tracker approvals --json
tracker gate approve <GATE-ID> --actor agent:reviewer-1 --reason "release smoke"
tracker run complete <RUN-ID> --actor human:owner
tracker ticket request-review APP-1 --actor agent:builder-1
tracker ticket approve APP-1 --actor agent:reviewer-1
tracker ticket complete APP-1 --actor human:owner
tracker run cleanup <RUN-ID> --actor human:owner
tracker inspect APP-1 --actor human:owner --json
tracker tui --actor human:owner
```

Release is not done until that flow works against the real published artifacts.

## Release Manager Checklist

- [ ] Prerelease tag created
- [ ] GitHub release artifacts published
- [ ] `checksums.txt` downloaded and verified against the chosen archive
- [ ] install script run against the published artifacts in a clean temp directory
- [ ] installed binary completed the smoke flow
- [ ] any failures captured below before retrying

Record the proof here for the actual release:

- prerelease tag:
- artifact URLs:
- checksum verification:
- smoke-flow result:
- notes:

## Local Rehearsal

For a local dry run before you cut the real prerelease:

```bash
VERSION=v1.4.0-rc1 ./scripts/release-rehearsal.sh
sh scripts/stability-smoke.sh
```

That script:

1. builds the current `tracker` binary
2. packages it with the same archive naming shape as the release workflow
3. serves the archive from a local HTTP server
4. installs it through `scripts/install.sh`
5. runs the orchestration smoke flow with the installed binary in a clean temp workspace

`scripts/stability-smoke.sh` is the short stabilization lane used in CI. It runs the
targeted race suite plus short fuzzers for slash parsing, query parsing, markdown
frontmatter parsing, automation TOML parsing, and event entry parsing.
