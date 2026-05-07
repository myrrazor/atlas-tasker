# Contributing

Atlas Tasker is still in release-candidate polish. Contributions are welcome, but expect maintainers to be conservative until the first public release is signed off.

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

If your change affects release scripts, docs snippets, terminal output, MCP, signing, governance, redaction, audit, or backup behavior, add the relevant targeted proof in the PR body.

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

Do not describe a change as shipped or stable unless the release evidence says so. For v1.8, `docs/release/public-release-gates.md` is the source of truth.
