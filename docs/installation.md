# Installation

Atlas ships one `tracker` binary for macOS and Linux on Intel/AMD and ARM64. The
[release page](https://github.com/myrrazor/atlas-tasker/releases/latest) lists the
current stable archives. This guide describes the current source and v1.12 setup
flow; [v1.11 release evidence](release/v1.11.0-release-evidence.md) records the
current published release.

## Install a release

Install `curl`, `tar`, a SHA-256 utility, and the GitHub CLI (`gh`) first. The
installer verifies the archive checksum and GitHub build attestation before
writing the executable:

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
VERSION=v1.11.0 BIN_DIR="$HOME/.local/bin" sh ./scripts/install.sh
```

Inspect installer scripts before running them. Use repository or release URLs,
and do not run commands copied from untrusted issues or comments.

## Set up your agents

After a terminal install, the v1.12 installer asks whether to initialize an Atlas
workspace and set up coding-agent guidance **in the displayed current directory**.
Press Enter or answer `n` to skip. Answering `yes` initializes that directory and
opens the agent picker. Run the installer from your intended project if you want
to accept that offer.

The picker lists Claude Code, Codex, Cursor, OpenClaw, Grok, and a generic agent.
Detected agents are checked; detection only looks for commands and configuration
paths and does not prove an authenticated provider account. Enter accepts the
checked agents. Names or numbers replace the selection; `none` or `q` skips it.
Only selected guidance files are written. See [integration destinations and
behavior](guides/agent-integrations.md).

You can always set up later from your project:

```bash
tracker init
# Or go directly to the picker after initialization:
tracker init --integrations
# Inspect detection without writing anything:
tracker integrations detect --json
# Reopen the picker:
tracker integrations install
# Choose targets without a prompt:
tracker integrations install --targets claude,codex,cursor
```

Interactive `tracker init` also offers agent setup. `tracker init --skip-integrations`
disables that offer. The curl installer supports `SKIP_INTEGRATIONS=1`:

```bash
curl -fsSL https://raw.githubusercontent.com/myrrazor/atlas-tasker/main/scripts/install.sh | SKIP_INTEGRATIONS=1 sh
```

Non-interactive installs and `--json` CLI invocations never prompt. For automation,
use `tracker init --skip-integrations --json`, then explicit integration targets.
Running `tracker init --integrations` without a terminal fails with instructions
for the non-interactive command.

Guidance installation writes instructions and the `atlas-worker` skill. **MCP
registration is a separate step** in the agent client. Use the [MCP setup guide](mcp.md)
to pin an initialized workspace and choose the `read` or `workflow` profile.
Neither installation route signs into an agent provider or starts an autonomous worker.

## Install with Go or build from source

With Go 1.26.6 or newer:

```bash
go install github.com/myrrazor/atlas-tasker/cmd/tracker@latest
```

Add the Go binary directory to `PATH`, then run `tracker init` in your project to
get the agent setup offer. `go install` itself does not prompt for integrations.

From a source checkout:

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
MCP client configuration. See [updating and recovery](guides/updating.md) for
pinned versions, reinstalling with `--force`, and failure behavior.

## Verify a downloaded archive

For a pinned published release:

```bash
VERSION=v1.11.0 ./scripts/verify-release.sh ./tracker_1.11.0_darwin_arm64.tar.gz
```

The script checks `checksums.txt` and GitHub artifact attestations. Authentication
or network failures can stop attestation verification; keep it enabled for normal
installs. `VERIFY_ATTESTATIONS=0` and updater `--skip-attestations` are explicit
exceptions for local fixtures or intentionally unattested artifacts, not normal
installation instructions.

## If setup fails

- A missing archive or checksum entry stops installation before the binary is written.
- A checksum or attestation failure stops installation; retry from the trusted release.
- An unwritable destination requires a writable `BIN_DIR`, not a change to your workspace permissions.
- If optional agent setup fails after installation, the binary remains installed. Read the error, then run `tracker integrations install` in the intended project.
- Running setup from inside another Atlas workspace points you back to its root. It does not create a nested tracker.
