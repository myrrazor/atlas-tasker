# Install Atlas Tasker without `curl | sh`

Two install paths that never pipe a remote script into a shell. Both put a `tracker` binary on your `PATH`, which is all the Grok plugin (`.mcp.json` → `tracker mcp serve`) needs.

## Option A: verified release binary (recommended)

Needs `gh` 2.102.0 or newer (no GitHub login required; older `gh`, including Ubuntu 24.04's 2.45.0 and Debian 13's 2.46.0, asks for `gh auth login` and has no `gh attestation`), `sha256sum` (on macOS use `shasum -a 256 -c -` in its place) and `tar`. Pick your platform: `linux_amd64`, `linux_arm64`, `darwin_amd64`, `darwin_arm64`.

```bash
gh --version  # must print gh version 2.102.0 or newer
VERSION=1.18.0
PLATFORM=linux_amd64
WORK=$(mktemp -d) && cd "$WORK"
gh release download "v$VERSION" --repo myrrazor/atlas-tasker \
  --pattern "tracker_${VERSION}_${PLATFORM}.tar.gz" --pattern checksums.txt \
  --pattern attestation-bundle.jsonl
grep " dist/tracker_${VERSION}_${PLATFORM}.tar.gz$" checksums.txt | sed "s# dist/# #" | sha256sum -c -
gh attestation verify "tracker_${VERSION}_${PLATFORM}.tar.gz" --repo myrrazor/atlas-tasker \
  --bundle attestation-bundle.jsonl \
  --signer-workflow myrrazor/atlas-tasker/.github/workflows/release.yml \
  --source-ref "refs/tags/v$VERSION"
tar -xzf "tracker_${VERSION}_${PLATFORM}.tar.gz" tracker
mkdir -p "$HOME/.local/bin" && install -m 0755 tracker "$HOME/.local/bin/tracker"
"$HOME/.local/bin/tracker" version
```

Expected: `sha256sum` prints `tracker_1.18.0_linux_amd64.tar.gz: OK`, `gh attestation verify` exits 0 (it prints `✓ Verification succeeded!` when run in a terminal), and `tracker version` prints `tracker v1.18.0`. Make sure `$HOME/.local/bin` is on your `PATH`. Stop if either check fails.

If `gh --version` is older than 2.102.0, upgrade first. The install script stops with the same message:

```text
Atlas Tasker needs GitHub CLI (gh) 2.102.0 or newer. Upgrade it from https://cli.github.com/ (distro packages are often older) and re-run.
```

## Option B: `go install` from source

Needs Go 1.26.6 or newer (Go downloads the toolchain named in `go.mod` when `GOTOOLCHAIN=auto`, the default). Pin a tag rather than `@latest` so you know what you built:

```bash
go install github.com/myrrazor/atlas-tasker/cmd/tracker@v1.18.0
"$(go env GOPATH)/bin/tracker" version
```

Expected: `tracker v1.18.0` (commit and build date show `unknown`, because `go install` builds are not release-stamped and carry no build attestation). Make sure `$(go env GOPATH)/bin` is on your `PATH`.

## Upgrading and removing

Repeat either option with a newer `VERSION` or tag. `tracker uninstall` removes the binary and keeps your boards; see the README.
