# Installation

Atlas ships one `tracker` binary for macOS and Linux on Intel/AMD and ARM64. The
[release page](https://github.com/myrrazor/atlas-tasker/releases/latest) lists the
current **published** archives and their hosted verification. Ordinary
use is `tracker init` then `tracker` in each workspace, then restart detected
coding agents. Unstamped source builds report `"version": "dev"`.

## Install a release

Install `curl`, `tar`, a SHA-256 utility, and the GitHub CLI (`gh`) first. The
installer verifies the archive checksum and a local GitHub attestation bundle
before writing the executable. That `gh` check does not need a GitHub login.
Missing `gh` fails early, before a long download.

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | sh
tracker version --json
```

The default destination is `/usr/local/bin`; it must be writable. To install in a
user-owned directory without elevated privileges:

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | BIN_DIR="$HOME/.local/bin" sh
export PATH="$HOME/.local/bin:$PATH"
tracker version --json
```

`BIN_DIR` selects the destination and `VERSION` pins a published tag. When running
a checked-out installer, for example:

```bash
VERSION=v1.16.0 BIN_DIR="$HOME/.local/bin" sh ./scripts/install.sh
```

Inspect installer scripts before running them. Use repository or release URLs,
and do not run commands copied from untrusted issues or comments.

Checksum mismatch, missing bundle, or identity failure refuses the install.
There is no checksum-only fallback. `VERIFY_ATTESTATIONS=0` is an explicit
local-fixture override, not a normal install.

## Uninstall

Preview then apply a software-only removal:

```bash
tracker uninstall
tracker uninstall --yes
```

Boards, history, backups, and registry pointers stay. See [uninstall](guides/uninstall.md).
Removing the binary by hand still leaves repository data in place.

## Set up your agents

The verified installer only places the `tracker` binary. It does not initialize
the directory `curl | sh` happened to run in without consent. When an interactive
terminal is available, it shows the current directory and offers `tracker init`,
defaulting to **no**. Accepting runs `tracker init --no-open`. Unattended installs
never prompt or initialize. `SKIP_INTEGRATIONS=1` skips that offer.

The short path is `tracker init` inside your project. That writes Atlas-managed
MCP entries named `atlas-tasker` (`mcp serve --global --tool-profile workflow`)
for detected agents unless you pass `--no-agents`. Restart the client afterward.
Grok also needs its own project-trust prompt before local skills appear.

Ordinary init does **not** open a picker. Detected agents are configured
automatically. The older Herder-style picker is still `--integrations` or
bare `tracker integrations install`:

```text
Set up coding-agent integrations now? [Y/n]
Coding agents on this machine:
  1. [x] cursor    found cursor in PATH
  2. [ ] claude    not detected
  3. [ ] grok      not detected
  ...
Press Enter to install the checked agents
```

Detected agents are checked. Detection looks at commands and configuration
paths; it does not prove an authenticated provider account. Enter accepts the
checked agents. Names or numbers replace the selection; `none` or `q` skips it.
Only selected guidance files are written. Open that repo in the agent and ask
for the board — there is no second product install. See [integration
destinations and behavior](guides/agent-integrations.md).

You can always set up later from your project:

```bash
tracker init
# Open the picker after initialization (TTY only):
tracker init --integrations
# Inspect detection without writing anything:
tracker integrations detect --json
# Reopen the picker:
tracker integrations install
# Choose targets without a prompt:
tracker integrations install --targets claude,codex,cursor
```

`tracker init --skip-integrations` disables automatic agent writes. The curl
installer supports `SKIP_INTEGRATIONS=1`:

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | SKIP_INTEGRATIONS=1 sh
```

Non-interactive installs and `--json` CLI invocations never prompt. For automation,
use `tracker init --skip-integrations --json`, then explicit integration targets.
Running `tracker init --integrations` without a terminal fails with instructions
for the non-interactive command.

`tracker init` writes Atlas-managed MCP entries for detected agents unless you
opt out. `tracker integrations install` writes instructions and the `atlas-worker`
skill only. Manual pinned `--workspace` serve remains in the [MCP setup guide](mcp.md).
Neither installation route signs into an agent provider or starts an autonomous worker.

## Install with Go or build from source

With Go 1.26.6 or newer:

```bash
go install github.com/myrrazor/atlas-tasker/cmd/tracker@latest
```

Add the Go binary directory to `PATH`, then run `tracker init` in your project.
`go install` itself does not prompt for integrations.

From a source checkout (**source-build walkthrough** — use `./tracker` until
the binary is on `PATH`):

```bash
go build -o tracker ./cmd/tracker
./tracker --help
./tracker version --json
./tracker init
```

Unstamped source builds report `version: "dev"` in JSON. You can keep that binary
local or move it onto your `PATH`.

## Update an existing install

The updater has distinct inspect and apply steps:

```bash
tracker update --check --json
tracker update --dry-run --json
tracker update --yes
tracker version --json
```

`--check` only reports whether an update is available. `--dry-run` reports the
planned archive and destination. Neither downloads or replaces the executable.
`--yes` downloads the archive, checks the checksum and GitHub attestation, then
atomically replaces the executable that ran the command. That destination must
be writable. The updater does not change tickets, workspaces, guidance files, or
MCP client configuration. After a binary replace, re-run `tracker init` or the
documented scoped setup **inside each workspace** to refresh managed skills.
See [updating and recovery](guides/updating.md).

## Verify a downloaded archive

For a pinned published release:

```bash
VERSION=v1.15.0 ./scripts/verify-release.sh ./tracker_1.15.0_darwin_arm64.tar.gz
```

The script checks `checksums.txt` and GitHub artifact attestations. Authentication
or network failures can stop attestation verification; keep it enabled for normal
installs. `VERIFY_ATTESTATIONS=0` and updater `--skip-attestations` are explicit
exceptions for local fixtures or intentionally unattested artifacts, not normal
installation instructions.

## If setup fails

- A missing archive, checksum entry, attestation bundle, or `gh` stops
  installation before the binary is written.
- A checksum or attestation failure stops installation; retry from the trusted release.
- An unwritable destination requires a writable `BIN_DIR`, not a change to your workspace permissions.
- If optional agent setup fails after installation, the binary remains installed. Read the error, then run `tracker integrations install` in the intended project.
- Running setup from inside another Atlas workspace points you back to its root. It does not create a nested tracker.
