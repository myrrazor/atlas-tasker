# Contributing

Contributions are welcome. Expect maintainers to be conservative about scope; read the gates below before opening a PR.

## Before Opening A PR

- Open or reference an issue when the change is user-visible.
- Keep changes scoped. Unrelated refactors make review harder.
- Update docs with any new command, JSON shape, script, policy, or public behavior.
- Do not include private keys, tokens, webhook URLs, full `.tracker` archives, or unredacted logs.
- Run the local gates:

```bash
git diff --check
go test ./...
go vet ./...
```

If you touched the marketing site under `site/`, also run its contract tests:

```bash
node --test site/_tools/*.test.mjs
```

For installation changes, build the binary and run the terminal and unattended
installer checks. They use isolated local archives and synthetic workspaces;
they do not modify your installed tracker or agent configuration:

```bash
go build -o tracker ./cmd/tracker
python3 scripts/test-install.py --tracker ./tracker
python3 scripts/verify-mcp-workflow.py --tracker ./tracker
```

The MCP harness checks both stdio formats, profile boundaries, and an actor-separated
ticket workflow in a synthetic workspace. CI runs these checks and site/browser
contracts on macOS and Linux.

If your change affects release scripts, docs snippets, terminal output, MCP, signing, governance, redaction, audit, or backup behavior, add the relevant targeted proof in the PR body.

## Local Setup

Atlas is a Go CLI. CI builds with Go 1.26.6 (same as the `go` line in `go.mod`); use that locally when possible.

```bash
git clone https://github.com/myrrazor/atlas-tasker.git
cd atlas-tasker
go version
go build -o tracker ./cmd/tracker
./tracker version --json
go test ./...
go vet ./...
```

Most examples in the docs use `./tracker` so they work immediately after a source build.

## Commit Style

Use conventional commits:

- `feat: ...`
- `fix: ...`
- `docs: ...`
- `test: ...`
- `security: ...`
- `chore: ...`

Reference the issue or PR track when one exists, for example:

```text
docs: add public install guide (#803)
```

## Review Expectations

Maintainers review for correctness, storage compatibility, security wording, docs drift, and local proof. Public docs should not claim Atlas provides OS sandboxing, hosted identity, encrypted-at-rest storage, DLP, malicious-local-user protection, full provider-rule enforcement, or full MCP client safety.

## Release-Candidate Rule

Do not describe a change as shipped or stable unless the release evidence says so. `docs/release/public-release-gates.md` is the source of truth.
