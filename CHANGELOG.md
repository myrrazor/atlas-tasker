# Changelog

Atlas is not yet published as a stable public release. This changelog starts with the public release-candidate polish train.

## Unreleased

- v1.8 release-candidate polish is implementation-complete on the stacked PR train.
- Local RC proof for `v1.8.0-rc1` is green: tests, vet, RC validation, stability smoke, release rehearsal, `govulncheck`, SBOM generation, proof-log leakage scan, and CSO closeout.
- Hosted release assets, checksums, attestations, install smoke from published assets, and packaged smoke from a hosted binary remain blocked until a release actor creates and verifies the GitHub prerelease.

## v1.8.0-rc1 - Local RC Proof Complete

- Rewrote the public README and OSS metadata for first-time GitHub readers.
- Added public docs for installation, quickstart, tutorials, concepts, command references, troubleshooting, doctor/repair, release verification, and agent workflows.
- Polished terminal, Markdown, and TUI output with width-aware layouts, render tokens, `NO_COLOR`, plain output, ASCII fallback, and better empty states.
- Added Codex, Codex `/goal`, Claude Code, generic agent, and MCP workflow guides.
- Added demo workspace scripts, prompt packs, terminal transcripts, and screenshot-friendly examples.
- Added release preflight, `tracker version`, packaged rehearsal, SBOM/vulnerability proof hooks, and offline RC validation.
- Added final release evidence, launch checklist, release notes, and no-ship decision for missing hosted proof.

## Pre-Public Work

The v1.0 through v1.7.1 tracks were internal release-candidate implementation trains. They built the local tracker, agent orchestration, collaboration, release tooling, MCP RC adapter, and v1.7 trust/governance/security surfaces.
