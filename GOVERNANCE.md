# Governance

Atlas Tasker is maintained by the repository owner and CODEOWNERS.

## Decision Making

- Maintainers decide release scope, storage compatibility, security boundaries, and merge timing.
- Security, governance, redaction, signature, audit, backup, release, and MCP changes require especially conservative review.
- Public claims must match `docs/security-limitations.md` and `docs/release/public-release-gates.md`.

## Pull Requests

PRs should be small enough to review and should include local proof. CI must pass before merge. Maintainers may ask for extra proof when a change touches release scripts, storage, signed artifacts, governance, redaction, MCP, or terminal output.

## Releases

A release is not ready because code merged. Release readiness is recorded in release evidence, and hosted assets must pass checksum, attestation, install, packaged smoke, SBOM, and vulnerability gates before stable sign-off.

## License

Atlas Tasker is released under the MIT License. See [LICENSE](LICENSE).
