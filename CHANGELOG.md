# Changelog

Atlas ships public release candidates today; the first stable release follows once the remaining release gates close. This changelog starts with the public release-candidate polish train.

## Unreleased

- The TUI opens with a branded splash (chrome-blue ATLAS TASKER art, any key skips, degrades cleanly on narrow and NO_COLOR terminals).
- All terminal colors now come from one brand palette (`internal/theme`): electric-blue primary, adaptive light/dark pairs, semantic status hues, and a real priority scale (critical/high/low) instead of uniform gray badges.
- Brand assets are committed under `assets/brand/` and the README wordmark is served from the repo.
- New `tracker gh` command family wraps the GitHub CLI: `status`, `prs`, `view`, `checks`, `create-pr`, `request-review`, and `import-url`. `create-pr` opens the pull request and links it as a ticket change in one audited mutation; everything degrades to a clear "install GitHub CLI" repair message when `gh` is missing.
- CLI color output now reaches the terminal intact: the display sanitizer preserves the SGR styling Atlas emits itself while still stripping every other escape sequence, so `tracker board` no longer prints `[90m` residue in color-capable terminals. Truncation drops styling cleanly instead of slicing escapes in half.
- README rewritten around the published release candidate: one-line verified install, terminal screenshots, demo GIF, and the agent workflow story.
- `tracker team` applies ready-made agent team presets (solo, pair, swarm, crossfire): agent profiles, the standard-build runbook, separation-of-duties permissions, and the matching completion gate in one idempotent command with `--dry-run` preview.
- Agent skill packs teach the full autonomous loop: bootstrap via team presets, wake-up acknowledgement, blocker reason codes, and the claim-build-review-handoff cycle. The Claude pack now installs to `.claude/skills/atlas-worker/` (modern Claude Code layout).

## v1.9.0-rc1 - First Public Release Candidate (2026-06-10)

- Published GitHub prerelease with archives for darwin/linux on amd64/arm64, `checksums.txt`, a CycloneDX SBOM, build attestations, and the installer script.
- Hosted release proof is green: checksum + attestation verification, clean-directory install from published assets, version metadata match, and packaged smoke.
- Go toolchain moved to 1.26.4 to clear stdlib govulncheck findings (GO-2026-5037, GO-2026-5039).
- Agent wake-ups now fire for `backlog` and `blocked` tickets when their last dependency completes; failed wake-up recording leaves a visible `failed` record instead of disappearing.
- A corrupted projection index now exits `repair_needed` with `tracker doctor --repair` guidance instead of leaking the raw sqlite error, and read-only `doctor` diagnoses it.
- Local planning docs and variant scratch trees are gitignored so they cannot ride along in a public push.
- v1.9 release-readiness work adds agent work queues, skill packs, wake-ups, workflow fixes, and this public cleanup pass.
- MIT license is committed.
- Public docs now point at the current v1.9 readiness state while historical v1.8 evidence remains available.
- Search now supports multi-word `text~` queries and reports invalid structured search input as `invalid_input`.
- v1.8 release-candidate polish is implementation-complete on the stacked PR train.
- Local RC proof for `v1.8.0-rc1` is green: tests, vet, RC validation, stability smoke, release rehearsal, `govulncheck`, SBOM generation, proof-log leakage scan, and CSO closeout.
- Hosted release assets, checksums, attestations, install smoke from published assets, and packaged smoke from a hosted binary remain blocked until a release actor creates and verifies the GitHub prerelease.
- PR-811 fixes installer verification, HTTPS-only release downloads by default, CI security gates, release SBOM publication, terminal control-byte sanitization, README/tutorial copy-paste issues, and release evidence for v1.7 carry-forward findings.
- Stable public release remains blocked by hosted proof, human docs/aesthetic review, and unresolved/deferred v1.7 governance-security carry-forward verdicts.

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
