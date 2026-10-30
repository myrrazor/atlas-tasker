# Grok plugin: reviewer testing steps

These steps check the Atlas Tasker Grok plugin end to end in a throwaway home directory: install, connect, list tools, call one tool. They make no changes outside the temporary directory, except that `tracker init` starts the local Atlas Home board on `http://127.0.0.1:7432/` (loopback only).

Requirements: Go 1.26.6+, Git, Python 3, and the `grok` CLI (Grok Build). No Atlas account, API key or GitHub login is needed.

## 1. Fresh environment

```bash
export REVIEW=$(mktemp -d)
export HOME="$REVIEW/home" GOPATH="$REVIEW/go" && mkdir -p "$HOME"
export PATH="$GOPATH/bin:$PATH"
export TRACKER_ACTOR=human:reviewer
```

## 2. Install the `tracker` binary (no `curl | sh`)

```bash
go install github.com/myrrazor/atlas-tasker/cmd/tracker@v1.18.1
tracker version
```

Expected: first line `tracker v1.18.1`.

## 3. Create a board

```bash
mkdir "$REVIEW/board" && cd "$REVIEW/board" && git init -q
tracker init --no-agents --no-open
```

Expected: a line starting `initialized` and ending `registered; http://127.0.0.1:7432/`.

## 4. Install and connect the plugin

Until the plugin PRs merge, first run `export PLUGIN_REF=docs/grok-plugin-reviewer-steps`.

```bash
git clone -q --branch "${PLUGIN_REF:-main}" https://github.com/myrrazor/atlas-tasker "$REVIEW/plugin"
grok plugin validate "$REVIEW/plugin"
grok plugin install "$REVIEW/plugin" --trust
grok mcp doctor atlas-tasker
```

Expected: `Plugin manifest is valid.` with `name: atlas-tasker` and `version: 1.18.1`; `Installed 1 plugin(s) ... atlas-tasker`; and from the doctor:

```text
  atlas-tasker (stdio: tracker mcp serve --global --tool-profile workflow --tool-name-style portable)
    ✓ command found (.../go/bin/tracker)
    ✓ server started
    ✓ handshake OK (protocol 2025-11-25)
    ✓ 104 tools discovered

Found 1 healthy, 0 failing.
```

## 5. List tools and call one

```bash
python3 "$REVIEW/plugin/scripts/grok-plugin-smoke.py" "$REVIEW/plugin/.mcp.json"
```

Expected (tool list abbreviated):

```text
server: {'name': 'atlas-tasker', 'version': 'v1.18.1'}
tools (104): atlas_activity, atlas_agent_available, ... atlas_status, ... atlas_ticket_create, ...
atlas_status isError: False
# Atlas status
Scope `workspace`.
...
Counts: backlog=0 blocked=0 canceled=0 done=0 in_progress=0 in_review=0 ready=0.
```

Optional, inside Grok: run `grok` in `$REVIEW/board`, open `/mcps` to see `atlas-tasker` connected, and ask "show my Atlas board".

## 6. Clean up

```bash
grok plugin uninstall atlas-tasker
cd / && pkill -f "$GOPATH/bin/tracker" ; go clean -modcache && rm -rf "$REVIEW"
```
