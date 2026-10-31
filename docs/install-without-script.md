# Install Atlas Tasker without `curl | sh`

Two install paths that never pipe a remote script into a shell. Both put a `tracker` binary on your `PATH`, which is all the Grok plugin (`.mcp.json` → `tracker mcp serve`) needs.

## Option A: verified release binary (recommended)

Needs `gh` 2.102.0 or newer (no GitHub login required; older `gh`, including Ubuntu 24.04's 2.45.0 and Debian 13's 2.46.0, asks for `gh auth login` and has no `gh attestation`), `sha256sum` (on macOS use `shasum -a 256 -c -` in its place) and `tar`. Pick your platform: `linux_amd64`, `linux_arm64`, `darwin_amd64`, `darwin_arm64`.

```bash
(
set -e
gh --version 2>/dev/null | awk 'NR==1{f=$3; split(f,v,"."); ok=(f~/^[0-9]+\.[0-9]+/ && (v[1]+0>2 || (v[1]+0==2 && v[2]+0>=102)))} END{if(!ok){print "Atlas Tasker needs GitHub CLI (gh) 2.102.0 or newer. Upgrade it from https://cli.github.com/ (distro packages are often older) and re-run. " (f=="" ? "gh not found" : "Found gh " f "; minimum is 2.102.0") > "/dev/stderr"; exit 1}}'
VERSION=1.18.1
PLATFORM=linux_amd64
WORK=$(mktemp -d) && cd "$WORK"
trap 'cd / && rm -rf "$WORK"' EXIT
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
cd / && rm -rf "$WORK"
"$HOME/.local/bin/tracker" version
)
```

Expected: `sha256sum` prints `tracker_1.18.1_linux_amd64.tar.gz: OK`, `gh attestation verify` exits 0 (it prints `✓ Verification succeeded!` when run in a terminal), and `tracker version` prints `tracker v1.18.1`. Make sure `$HOME/.local/bin` is on your `PATH`. The block runs in a subshell with `set -e`, so it stops at the first failing step (old or missing `gh`, checksum, attestation) without closing your terminal, and removes its temporary folder when it succeeds. Stop if either check fails.

Releases through v1.18.1 record `dist/<filename>` in `checksums.txt`, so the `grep` and `sed` above are required before `sha256sum -c` for those downloads. Later releases write the bare asset filename. In a folder that contains those assets and `checksums.txt`, `sha256sum -c checksums.txt` is enough. `scripts/install.sh` and `tracker update` accept both forms.

If `gh --version` is older than 2.102.0, upgrade first. The install script stops with the same message:

```text
Atlas Tasker needs GitHub CLI (gh) 2.102.0 or newer. Upgrade it from https://cli.github.com/ (distro packages are often older) and re-run.
```

## Option B: `go install` from source

Building from source requires Go 1.26.10 or newer. With the default `GOTOOLCHAIN=auto`, Go downloads Go 1.26.10 when your local toolchain is older than 1.26.10. Pin a tag rather than `@latest` so you know what you built:

```bash
go install github.com/myrrazor/atlas-tasker/cmd/tracker@v1.18.1
"$(go env GOPATH)/bin/tracker" version
```

Expected: `tracker v1.18.1` (commit and build date show `unknown`, because `go install` builds are not release-stamped and carry no build attestation). Make sure `$(go env GOPATH)/bin` is on your `PATH`.

## Upgrading and removing

Repeat either option with a newer `VERSION` or tag. `tracker uninstall` removes the binary and keeps your boards; see the README.
