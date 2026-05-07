# Canonicalization

`atlas-c14n-v1` is the byte contract for v1.7 signatures.

Rules:

- input strings must be valid UTF-8
- Atlas does not perform Unicode normalization
- text values normalize CRLF and CR to LF
- maps use exact lexicographic JSON key sorting
- timestamps are UTC RFC3339Nano
- floats are not allowed in signed manifests
- nested payloads deeper than 128 levels are rejected
- binary artifacts are signed by SHA-256 hash, not embedded text
- volatile fields such as `generated_at` are excluded unless the artifact contract says they are part of the signed payload
- verifier-side projection fields, such as signature verification state, are excluded from signed bytes with the `atlasc14n:"-"` struct tag
- unordered sets must be sorted by the artifact builder before canonicalization; ordered arrays are preserved and documented as ordered

Golden fixtures must include Unicode, empty arrays/maps, sorted sets, path separators, multiline text, CRLF normalization, reordered input, and one-byte tamper failures.
