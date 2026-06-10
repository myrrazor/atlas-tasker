# Branch Cleanup Receipts

Five long-lived branches were deleted on 2026-06-10 after verifying their work ships on `main`. GitHub keeps every deleted branch's history reachable through its pull request refs, so nothing here is unrecoverable.

| Deleted branch | Where the work lives on main |
|---|---|
| `codex/pr-305-git-integration` | Merged as #305 (`feat: add local git workflow integration`, commit `59cba9d`). `tracker git status/branch-name/refs/commit` and `internal/service/scm.go` are on main and exercised by the TUI's Git Context panel. |
| `codex/pr-306-gh-adapter` | The branch's service layer was already superseded by main's larger `internal/service/gh.go` (PR views, check runs, review requests). The missing CLI surface shipped fresh as #111 (`tracker gh` command family) on top of that richer service. |
| `codex/pr-310-mcp-adapter` | Superseded wholesale by the official-SDK MCP server from PR-616: `internal/mcp` with 30+ profile-gated tools, operation approvals with TTL, and audit logging. The branch's hand-rolled JSON-RPC server was an earlier design, deliberately replaced. |
| `codex/cso-security-fixes` (draft PR #67) | Every finding is on main: GitHub Actions SHA-pinned in `ci.yml`/`release.yml`, Go toolchain at 1.26.4 (newer than the branch's 1.26.2), installer checksum + attestation verification in `scripts/install.sh`, MCP approval hardening (PR-616), sync transport hardening, and the `.gstack/` gitignore entry. |
| `codex/v17-review-fixes` (draft PR #87) | Integrated through the v1.7 train (PR-701..709): governance enforcement for backup/redaction overrides, `atlas-c14n-v1` canonical audit hashes, tightened redaction defaults, atomic writes (`internal/service/atomic_write.go`), and decision log entries D-1701..D-1714 in `docs/v1.7-decision-log.md`. |

The merged feature branches from the v1.9 polish wave (`fix/v19-audit-followups`, `fix/rc-polish`, `feat/tui-splash-and-theme`, `feat/gh-commands`, `feat/agent-team-presets`, `codex/tui-help`) were deleted on merge as usual.
