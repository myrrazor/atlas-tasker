# Uninstall Atlas Tasker software only

`tracker uninstall` and `/uninstall` in the tracker shell remove **Atlas-owned software**.
They do not delete your boards.

This is the v1.15 source candidate. The command exists in this tree; a published release
tag is not required for the docs, and this page does not claim a v1.15 GitHub release.

## Preview, then apply

```bash
tracker uninstall
tracker uninstall --json
tracker uninstall --yes
```

`--apply` is an alias for `--yes`. Default is a digest-bound preview of exact Atlas-owned
paths: the script-installed binary (when the receipt verifies), the Home loopback unit
when it is marked Atlas-owned and names that same binary, and Atlas-managed MCP blocks
that match `<tracker> mcp serve --global --tool-profile workflow`.

Without a verifiable install receipt, uninstall prints the preview and **refuses** to
delete files. It never guesses an executable path.

Package-manager installs (Homebrew Cellar paths, apt, pacman) emit the manager command
instead of unlinking the binary.

## What stays

Uninstall does not delete:

- workspaces, `projects/`, `.tracker/`, tickets, events, custom templates
- backup/recovery repositories, ledgers, outbox, targets, replica identity
- registry pointers (so a later install can rediscover boards)
- unrelated client config, other agents, or the machine-state root itself

Reinstall the binary, run `tracker init` or `tracker` as usual, and preserved workspaces
show up again if their paths still exist.

## Host slash entry

`/uninstall` in the tracker shell tokenizes to the same command. Provider hosts
(Codex, Claude Code, Cursor, OpenClaw, Grok) may expose the same preview/apply by
executing `tracker uninstall` / `tracker uninstall --yes`. Atlas does not silently
rewrite those hosts during uninstall tests; do not run real uninstall against a live
owner client.

## Compatibility

Removing the binary by hand still works, and your project data still stays in the
repository. Prefer `tracker uninstall` on the v1.15 candidate so managed service units
and Atlas-owned MCP blocks are included in the plan.
