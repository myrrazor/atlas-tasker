# Atlas Tasker Launch Checklist

Use this checklist before calling `v1.9.0-rc1` public-release ready. Local proof is necessary, but hosted proof is the release boundary.

## Local RC

- [x] `go test -count=1 ./... 2>&1 | tee TEST_STDOUT.log`
- [x] `go vet ./...`
- [x] `git diff --check`
- [x] `VERSION=v1.9.0-rc1 sh scripts/preflight-release.sh`
- [x] `VERSION=v1.9.0-rc1 sh scripts/validate-rc.sh`
- [x] `sh scripts/stability-smoke.sh`
- [x] `VERSION=v1.9.0-rc1 ./scripts/release-rehearsal.sh`
- [x] `VERSION=v1.9.0-rc1 RUN_GOVULNCHECK=1 RUN_SBOM=1 sh scripts/preflight-release.sh`
- [x] no obvious private-key, token, or local-path leakage in committed proof logs
- [x] release evidence updated
- [x] CSO closeout recorded

## Hosted RC

- [ ] `VERSION=v1.9.0-rc1 sh scripts/preflight-release.sh --hosted`
- [ ] create the `v1.9.0-rc1` prerelease tag
- [ ] confirm GitHub publishes all archives and `checksums.txt`
- [ ] download at least one hosted archive
- [ ] verify hosted checksums
- [ ] verify GitHub artifact attestations
- [ ] install from hosted release assets into a clean directory
- [ ] compare installed `tracker version --json` with the tag, commit, build date, and platform
- [ ] run packaged smoke from the hosted binary
- [ ] update release evidence with hosted proof

## GitHub Metadata

- [ ] repository description names Atlas as a local-first task orchestrator for AI coding agents
- [ ] topics include `cli`, `local-first`, `issue-tracker`, `ai-agents`, `codex`, and `mcp`
- [ ] social preview image or screenshot/transcript is selected
- [ ] Discussions enabled/disabled decision is recorded
- [ ] GitHub private vulnerability reporting is enabled or an alternate private contact is published
- [ ] first announcement draft links README, quickstart, release notes, and security limitations

## Stable Release

- [ ] hosted RC gate is green
- [x] MIT `LICENSE` is committed
- [ ] owner signs off in release evidence
- [ ] human README/docs/aesthetic review is complete
- [ ] README status changes from planned/blocked to released
- [ ] changelog moves `v1.9.0-rc1` from candidate status to released status
- [ ] installer docs point to verified hosted assets
- [ ] security docs still avoid claims Atlas does not make

Current decision: **no-ship from this session** because hosted proof is blocked and the `v1.9.0-rc1` release does not exist yet.
