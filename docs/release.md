# Atlas Tasker Release Guide

Atlas Tasker v1.6 ships prebuilt macOS and Linux binaries plus a one-line install script.

## Artifacts

Each release tag publishes these archives:

- `tracker_<version>_darwin_amd64.tar.gz`
- `tracker_<version>_darwin_arm64.tar.gz`
- `tracker_<version>_linux_amd64.tar.gz`
- `tracker_<version>_linux_arm64.tar.gz`
- `checksums.txt`

Each archive contains a single `tracker` binary.

## In-Repo Deliverables

`PR-608` proves the release workflow shape inside the repo:

- release archives are built for macOS and Linux
- `checksums.txt` is generated and published with the archives
- GitHub artifact attestations are generated for the archives and checksum file
- `scripts/install.sh` installs a published archive
- `scripts/verify-release.sh` verifies checksums and, by default, artifact attestations
- `scripts/release-rehearsal.sh` exercises the installed-binary single-workspace and multi-workspace smoke flows locally

These deliverables are necessary, but they are not the same thing as a real hosted prerelease.

## Post-Merge Release Gate

Release is not done until a real prerelease proves the published artifacts:

0. run `sh scripts/preflight-release-proof.sh`
1. cut a prerelease tag
2. let GitHub publish the real archives and `checksums.txt`
3. download the chosen archive
4. run checksum verification
5. run artifact attestation verification
6. install from the published release assets
7. run the packaged smoke flow end to end

## One-Line Install

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
```

Optional overrides:

```bash
VERSION=v1.6.1 BIN_DIR="$HOME/.local/bin" curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
```

## Manual Install

1. Download the archive for your OS and architecture from the GitHub release.
2. Download `checksums.txt` from the same release.
3. Run `scripts/verify-release.sh` against the archive.
4. Extract the archive.
5. Put `tracker` somewhere on your `PATH`.

Example:

```bash
curl -fsSLO https://github.com/myrrazor/atlas-tasker/releases/download/v1.6.1/tracker_1.6.1_darwin_arm64.tar.gz
VERSION=v1.6.1 ./scripts/verify-release.sh ./tracker_1.6.1_darwin_arm64.tar.gz
```

If you already have the GitHub CLI authenticated, `scripts/verify-release.sh` also runs:

```bash
gh attestation verify ./tracker_1.6.0_darwin_arm64.tar.gz --repo myrrazor/atlas-tasker
```

Set `VERIFY_ATTESTATIONS=0` only for local rehearsals or when GitHub attestations are intentionally unavailable.

`bundle verify` proves archive integrity and manifest correctness. It does not claim cryptographic signer authenticity for the bundle source.

## Release Workflow

The release workflow runs on tags matching `v*` and:

1. builds macOS and Linux archives for amd64 and arm64
2. generates per-archive SHA256 files and a combined `checksums.txt`
3. generates GitHub artifact attestations for the archives and checksum file
4. publishes `-rc` tags as prereleases
5. uploads the archives, `checksums.txt`, and `scripts/install.sh` to the GitHub release

## Prerelease Rehearsal

Before cutting a real prerelease:

1. build the current release candidate locally
2. verify the local archive against a served `checksums.txt`
3. install through `scripts/install.sh`
4. run the installed-binary smoke flow in a clean temp workspace

Local rehearsal command:

```bash
VERSION=v1.6.1-rc1 ./scripts/release-rehearsal.sh
sh scripts/stability-smoke.sh
```

That script:

1. builds the current `tracker` binary
2. packages it using the release archive naming convention
3. generates `checksums.txt`
4. verifies the archive with `scripts/verify-release.sh`
5. serves the artifacts over a local HTTP server
6. installs through `scripts/install.sh`
7. runs the packaged smoke flow with the installed binary

## Packaged Smoke Flow

The v1.6 packaged smoke flow covers:

- run dispatch
- runtime launch
- checkpoint and evidence capture
- local change creation and status
- review gate approval
- run and ticket completion
- runtime archival and restore
- worktree cleanup
- collaborator add, trust, and membership bind
- git remote sync across multiple workspaces
- explicit sync conflict creation and resolution
- offline bundle create, verify, and import
- three-workspace convergence
- compact, reindex, and doctor after synced history exists

Suggested flow:

```bash
git init -b main
git config user.email atlas@example.com
git config user.name Atlas
printf '# atlas\n' > README.md
git add README.md
git commit -m init

tracker init
tracker project create APP "App Project"
tracker ticket create --project APP --title "Smoke" --type task --reviewer agent:reviewer-1 --actor human:owner
tracker ticket move APP-1 ready --actor human:owner
tracker agent create builder-1 --name "Builder One" --provider codex --capability go --actor human:owner
tracker run dispatch APP-1 --agent builder-1 --actor human:owner
tracker run launch <RUN-ID> --actor human:owner
tracker run start <RUN-ID> --actor human:owner
tracker ticket move APP-1 in_progress --actor human:owner
tracker run checkpoint <RUN-ID> --title "Smoke checkpoint" --body "runtime + worktree ready" --actor human:owner
tracker run evidence add <RUN-ID> --type note --title "Smoke evidence" --body "packaged rehearsal" --actor human:owner
tracker change create <RUN-ID> --actor human:owner
tracker change status <CHANGE-ID>
tracker run handoff <RUN-ID> --next-actor agent:reviewer-1 --next-gate review --actor human:owner
tracker gate approve <GATE-ID> --actor agent:reviewer-1 --reason "release smoke"
tracker run complete <RUN-ID> --actor human:owner --summary "smoke complete"
tracker ticket request-review APP-1 --actor agent:builder-1
tracker ticket approve APP-1 --actor agent:reviewer-1
tracker ticket complete APP-1 --actor human:owner
tracker archive plan --target runtime --project APP
tracker archive apply --target runtime --project APP --yes --actor human:owner
tracker archive restore <ARCHIVE-ID> --actor human:owner
tracker run cleanup <RUN-ID> --actor human:owner
tracker inspect APP-1 --actor human:owner --json
```

## Collaboration Rehearsal

The release rehearsal now proves the v1.6 collaboration surface with installed binaries:

1. workspace A publishes through a git sync remote
2. workspace B pulls, receives collaborators and mentions, and publishes a conflicting edit
3. workspace A opens and resolves an explicit conflict
4. workspace B publishes a new ticket, workspace C pulls it, then publishes another ticket
5. workspace A pulls from workspace C and proves three-workspace convergence
6. workspace A creates a bundle, verifies it, and workspace D imports it
7. workspace A runs archive, compact, restore, reindex, and doctor after synced history exists

## Release Manager Checklist

### In-repo gate
- [ ] release workflow builds all supported archives
- [ ] `checksums.txt` is produced
- [ ] attestation generation is enabled in the workflow
- [ ] `scripts/install.sh` passes syntax and rehearsal
- [ ] `scripts/verify-release.sh` passes syntax and rehearsal
- [ ] `scripts/release-rehearsal.sh` passes locally
- [ ] packaged collaboration smoke proves git sync, bundle fallback, and three-workspace convergence

### Post-merge prerelease gate
- [ ] `sh scripts/preflight-release-proof.sh` passes
- [ ] prerelease tag created
- [ ] GitHub release artifacts published
- [ ] chosen archive downloaded from the published release
- [ ] checksum verification passed
- [ ] artifact attestation verification passed
- [ ] install script run against the published artifacts in a clean temp directory
- [ ] installed binary completed the packaged smoke flow
- [ ] any failures captured below before retrying

Record the proof here for the actual release:

- prerelease tag:
- artifact URLs:
- checksum verification:
- attestation verification:
- smoke-flow result:
- notes:

For v1.6.1 closeout, also capture the hosted proof and merge order in:

- `docs/v1.6.1-release-evidence.md`
- `docs/v1.6.1-merge-order.md`

For v1.7 release-candidate proof, capture local proof, hosted prerelease status, SBOM/vulnerability status, and the final ship/no-ship decision in:

- `docs/v1.7-release-evidence.md`

The v1.7.1 release-signoff closeout also stores local proof artifacts in:

- `docs/release-proof/`
