# Atlas Tasker v1.10 Launch Checklist

Release date: 2026-09-08. The [release evidence](v1.10.0-release-evidence.md) records the hosted RC proof and owner ship decision. The [stable release page](https://github.com/myrrazor/atlas-tasker/releases/tag/v1.10.0) records the final stable artifact and unpinned-installer verification.

## Completed before stable publication

- [x] Owner merged the six release PRs and authorized promotion, RC publication, then stable publication.
- [x] Full Go tests/vet, browser/site contracts, race/fuzz stability, vulnerability scan, full-history secret scan, and SBOM checks passed.
- [x] Source/security review, local browser QA, decision audit, and review limitations are recorded.
- [x] Hosted release settings preflight passed.
- [x] `v1.10.0-rc1` was published with four supported archives, SBOM, checksums, and installer.
- [x] All archive/SBOM checksums and all six artifact attestations passed.
- [x] Clean install from hosted assets passed with verification enabled.
- [x] Installed version, commit, build timestamp, Go version, and platform matched the hosted release.
- [x] The hosted binary passed the RC validator and the packaged agent/synchronization/repair workflow.
- [x] MIT `LICENSE` is committed.
- [x] Owner acceptance of the merged README/docs/web presentation and stable ship decision is recorded.
- [x] Public release guides, dates, and evidence links are current.

## Publication verification

After tagging `v1.10.0`, verify the stable archives and attestations, then install with no `VERSION` pin and confirm `v1.10.0` is returned by `releases/latest`. Record the exact stable commit and outcomes in the public release notes. Never move an existing release tag to include later bookkeeping.

Decision: **SHIP v1.10.0 after the evidence update passes CI.** Hosted RC proof is green; the stable release page supplies the post-publication result.
