# Atlas Tasker v1.13 Launch Checklist

[Candidate evidence](v1.13.0-release-evidence.md) separates local preparation from
hosted release proof. This is the source-approval snapshot; after publication,
the [v1.13.0 release page](https://github.com/myrrazor/atlas-tasker/releases/tag/v1.13.0)
records the final hosted results without changing the tag.

## Candidate preparation

- [x] All 22 reported findings have a recorded disposition and evidence.
- [x] Agreed workflow, policy, assignment, scheduling, and MCP fixes are implemented.
- [x] Current documentation and website copy are reconciled with the candidate.
- [x] New board captures use actual CLI/web output and synthetic data.
- [x] Complete Go tests and vet pass; the final authorization regression passes.
- [x] Three bounded review rounds are complete with no remaining actionable finding.
- [x] All 40 web and site contracts pass.
- [x] Local release preflight, offline validator, and packaged installation rehearsal pass.
- [x] All five hosted checks pass on PR #141's final head.
- [x] Owner approves PR #141 and merges it into `testing`.
- [x] Owner authorizes the remaining reviews, merges, and full release.

## Publication

- [ ] Owner-approved promotion reaches `main` with required code-owner review.
- [ ] Required checks pass on the exact promoted commit.
- [ ] Hosted settings preflight passes.
- [ ] v1.13.0-rc1 publishes four archives, SBOM, checksums, and installer.
- [ ] Hosted RC checksums, attestations, clean install, metadata, and packaged workflow pass.
- [ ] Stable sign-off is recorded and v1.13.0 is published from the approved source.
- [ ] Stable verification passes and the unpinned installer resolves v1.13.0.
- [ ] The matching website is deployed and verified; public release evidence is updated.

Existing tags remain immutable. Record post-publication evidence on release pages.
