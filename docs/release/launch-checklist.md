# Atlas Tasker v1.12 Launch Checklist

The [candidate evidence](v1.12.0-release-evidence.md) separates local validation from hosted release proof. v1.11.0 remains the current published stable release.

## Candidate preparation

- [x] Owner requested the v1.12 scope and delegated implementation/validation.
- [x] MCP, self-update, integration, web-language and website documentation match the candidate behavior.
- [x] Installer consent/cancellation and the real stdio MCP workflow have isolated proof.
- [x] Current web screenshots use synthetic data; README/site use the UI wordmark.
- [x] Docs and Connect Your Agent browser interactions have desktop/phone proof.
- [ ] Fresh required Go, site, stability, local RC, vulnerability, secret and SBOM checks pass.
- [ ] Final review and decision audit are recorded on the approval-ready PR head.
- [ ] Owner approval of the final changes and presentation is recorded.

## Publication

- [ ] Owner-approved testing-to-main promotion is complete.
- [ ] Hosted settings preflight passes.
- [ ] v1.12.0-rc1 publishes all four supported archives, SBOM, checksums and installer.
- [ ] Hosted RC checksums/attestations, clean install, metadata and packaged workflow pass.
- [ ] v1.12.0 is published after the hosted RC gate.
- [ ] Stable archives and attestations pass; the unpinned installer resolves v1.12.0.
- [ ] Public release references and post-publication evidence are updated.

Existing release tags remain immutable. Record hosted results on their release pages rather than moving a tag to include its own verification.
