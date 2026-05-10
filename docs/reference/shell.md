# Slash Shell

The Atlas slash shell is a convenience layer over the same service paths as the CLI. It should not be treated as a separate authority model.

Use the slash shell for quick local inspection and small workflow actions. Use CLI JSON for scripts, tests, or automation that needs stable parsing.

Parity expectations:

- read surfaces should agree with equivalent CLI JSON
- mutations must still pass actor, reason, permission, governance, and write-lock checks
- shell output is display output, not a storage contract
