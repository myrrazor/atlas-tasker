#!/bin/sh
set -eu

mode="${1:---check}"
case "$mode" in
  --check|--update) ;;
  *)
    echo "usage: sh examples/generate-demo-assets.sh [--check|--update]" >&2
    exit 2
    ;;
esac

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
tmp_root=$(mktemp -d "${TMPDIR:-/tmp}/atlas-demo.XXXXXX")
trap 'rm -rf "$tmp_root"' EXIT INT TERM

generated="$tmp_root/generated"
workspace="$tmp_root/workspace"
transcript_raw="$tmp_root/transcript.raw"
transcript_norm="$tmp_root/transcript.norm"
mkdir -p "$generated/output" "$generated/prompt-packs" "$generated/transcripts" "$generated/fixtures" "$workspace"
: > "$transcript_raw"

tracker_bin="${TRACKER_BIN:-}"
if [ -z "$tracker_bin" ]; then
  tracker_bin="$tmp_root/tracker"
  (cd "$repo_root" && go build -o "$tracker_bin" ./cmd/tracker)
fi

normalize_file() {
  in_file="$1"
  out_file="$2"
  DEMO_TMP="$tmp_root" DEMO_WORKSPACE="$workspace" perl -0pe '
    s#\Q$ENV{DEMO_WORKSPACE}\E#<DEMO_WORKSPACE>#g;
    s#\Q$ENV{DEMO_TMP}\E#<TMP>#g;
    s#/private/var/folders/[^"\s)]+#<TMP_PATH>#g;
    s#/var/folders/[^"\s)]+#<TMP_PATH>#g;
    s#/tmp/atlas-demo\.[^"\s)]+#<TMP>#g;
    s#run_[0-9a-f]{16}#<RUN-ID>#g;
    s#evidence_[0-9a-f]{16}#<EVIDENCE-ID>#g;
    s#handoff_[0-9a-f]{16}#<HANDOFF-ID>#g;
    s#gate_[0-9a-f]{16}#<GATE-ID>#g;
    s#goal-[0-9a-f]+#<GOAL-MANIFEST-ID>#g;
    s#[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}#<UUID>#g;
    s#[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z#<TIMESTAMP>#g;
  ' "$in_file" > "$out_file"
}

record() {
  display="$1"
  shift
  {
    printf '$ %s\n' "$display"
    "$@" 2>&1
    printf '\n'
  } >> "$transcript_raw"
}

capture_output() {
  rel="$1"
  display="$2"
  shift 2
  raw="$tmp_root/$(echo "$rel" | tr '/.' '__').raw"
  {
    printf '$ %s\n' "$display"
    "$@" 2>&1
  } > "$raw"
  normalize_file "$raw" "$generated/$rel"
}

cd "$workspace"
git init -q
git config user.email demo@example.invalid
git config user.name "Atlas Demo"
cat > app.go <<'GO'
package main

func main() {}
GO
git add app.go
git commit -qm "seed demo app"

record "tracker init" "$tracker_bin" init
record "tracker project create APP \"Example App\"" "$tracker_bin" project create APP "Example App"
record "tracker ticket create --project APP --title \"Add health check\" --type task --actor human:owner --reason \"demo setup\" --acceptance ..." \
  "$tracker_bin" ticket create \
    --project APP \
    --title "Add health check" \
    --type task \
    --actor human:owner \
    --reason "demo setup" \
    --description "Expose a local health check and record the proof for review." \
    --acceptance "Health check behavior is documented." \
    --acceptance "Local smoke command is recorded as evidence."
record "tracker ticket move APP-1 ready --actor human:owner --reason \"ready for implementation\"" \
  "$tracker_bin" ticket move APP-1 ready --actor human:owner --reason "ready for implementation"
record "tracker ticket claim APP-1 --actor agent:builder-1" \
  "$tracker_bin" ticket claim APP-1 --actor agent:builder-1
record "tracker ticket move APP-1 in_progress --actor agent:builder-1 --reason \"start implementation\"" \
  "$tracker_bin" ticket move APP-1 in_progress --actor agent:builder-1 --reason "start implementation"
record "tracker agent create builder-1 --name \"Builder One\" --provider codex --capability go --actor human:owner --reason \"register demo agent\"" \
  "$tracker_bin" agent create builder-1 --name "Builder One" --provider codex --capability go --actor human:owner --reason "register demo agent"

printf '\ntracker\n.tracker/write.lock\n.tracker/index.sqlite\n.tracker/index.sqlite-wal\n.tracker/index.sqlite-shm\n' >> .git/info/exclude
git add .
git commit -qm "track atlas demo state"

dispatch_json="$tmp_root/dispatch.json"
{
  printf '$ tracker run dispatch APP-1 --agent builder-1 --actor human:owner --reason "start demo run" --json\n'
  "$tracker_bin" run dispatch APP-1 --agent builder-1 --actor human:owner --reason "start demo run" --json | tee "$dispatch_json"
  printf '\n'
} >> "$transcript_raw"
run_id=$(sed -n 's/.*"run_id": "\([^"]*\)".*/\1/p' "$dispatch_json" | head -n 1)
if [ -z "$run_id" ]; then
  echo "failed to parse run_id from dispatch output" >&2
  exit 1
fi

record "tracker run start <RUN-ID> --summary \"Demo implementation started\" --actor agent:builder-1 --reason \"begin work\"" \
  "$tracker_bin" run start "$run_id" --summary "Demo implementation started" --actor agent:builder-1 --reason "begin work"
record "tracker run checkpoint <RUN-ID> --title \"First pass\" --body \"Health check route added locally.\" --actor agent:builder-1 --reason \"status update\"" \
  "$tracker_bin" run checkpoint "$run_id" --title "First pass" --body "Health check route added locally." --actor agent:builder-1 --reason "status update"
record "tracker run evidence add <RUN-ID> --type test_result --title \"Smoke test\" --body \"go test ./... passed\" --actor agent:builder-1 --reason \"record verification\"" \
  "$tracker_bin" run evidence add "$run_id" --type test_result --title "Smoke test" --body "go test ./... passed" --actor agent:builder-1 --reason "record verification"
record "tracker run handoff <RUN-ID> --next-actor agent:reviewer-1 --next-gate review --actor agent:builder-1 --reason \"ready for review\"" \
  "$tracker_bin" run handoff "$run_id" --next-actor agent:reviewer-1 --next-gate review --actor agent:builder-1 --reason "ready for review"

capture_output output/board.txt "tracker board" "$tracker_bin" board
capture_output output/ticket-inspect.md "tracker inspect APP-1 --actor human:owner --md" "$tracker_bin" inspect APP-1 --actor human:owner --md
capture_output output/dashboard.txt "tracker dashboard" "$tracker_bin" dashboard
capture_output output/goal-brief.md "tracker goal brief APP-1 --md" "$tracker_bin" goal brief APP-1 --md
capture_output output/goal-manifest.md "tracker goal manifest APP-1 --actor human:owner --reason \"prepare demo goal\" --md" \
  "$tracker_bin" goal manifest APP-1 --actor human:owner --reason "prepare demo goal" --md

normalize_file "$transcript_raw" "$transcript_norm"
{
  cat <<'EOF'
# Demo Workspace Transcript

This transcript is regenerated by `examples/generate-demo-assets.sh`. Volatile timestamps, IDs, and local paths are normalized.

```console
EOF
  cat "$transcript_norm"
  cat <<'EOF'
```
EOF
} > "$generated/transcripts/demo-workspace.md"

cat > "$generated/README.md" <<'EOF'
# Atlas Examples

These examples are deterministic release-candidate demo material. Regenerate them with:

```bash
sh examples/generate-demo-assets.sh --update
```

Use `--check` in review to prove the checked-in assets are fresh.

## Outputs

- [Board](output/board.txt)
- [Ticket inspect](output/ticket-inspect.md)
- [Dashboard](output/dashboard.txt)
- [Goal brief](output/goal-brief.md)
- [Goal manifest](output/goal-manifest.md)

## Agent Prompt Packs

- [Codex](prompt-packs/codex.md)
- [Claude Code](prompt-packs/claude-code.md)
- [Generic agent](prompt-packs/generic-agent.md)

## Demo And Screenshot Fixtures

- [Terminal transcript](transcripts/demo-workspace.md)
- [Screenshot fixtures](screenshot-fixtures.md)
- [Workspace fixture notes](fixtures/demo-workspace.md)
EOF

cat > "$generated/prompt-packs/codex.md" <<'EOF'
# Codex Prompt Pack

Use this with the output from `tracker goal brief APP-1 --md`.

```text
You are working inside an Atlas Tasker ticket. Read the goal brief first.

Rules:
- stay within the Allowed Actions and Do Not Do sections
- use `tracker inspect APP-1 --actor agent:builder-1 --json` before changing state
- record progress with `tracker run checkpoint <RUN-ID> ...`
- record test proof with `tracker run evidence add <RUN-ID> --type test_result ...`
- request review or create a handoff instead of bypassing gates

Return a short summary, changed files, tests run, and any Atlas evidence IDs.
```
EOF

cat > "$generated/prompt-packs/claude-code.md" <<'EOF'
# Claude Code Prompt Pack

Use this when Claude Code is doing implementation work while Atlas owns workflow state.

```text
Start by reading the Atlas goal brief and `tracker inspect APP-1 --actor agent:builder-1 --json`.

When you make progress:
- checkpoint the run with the current status
- attach verification as `test_result` evidence
- do not mark the ticket complete unless Atlas governance allows it
- create a handoff when review context matters

Keep the final answer focused on implementation, tests, and Atlas follow-up commands.
```
EOF

cat > "$generated/prompt-packs/generic-agent.md" <<'EOF'
# Generic Agent Prompt Pack

Use this for any coding agent that can run shell commands.

```text
Atlas is the source of truth for this task.

Before editing:
1. Run `tracker inspect APP-1 --actor <actor> --json`.
2. Read `tracker goal brief APP-1 --md`.
3. Confirm the ticket is claimed or claim it.

During work, record checkpoints and evidence. If blocked, write a ticket comment or handoff instead of silently stopping.
```
EOF

cat > "$generated/screenshot-fixtures.md" <<'EOF'
# Screenshot Fixtures

These files are normalized so they are useful for README screenshots, terminal captures, and docs examples without leaking local paths.

Recommended panes:

1. `docs/examples/output/board.txt`
2. `docs/examples/output/ticket-inspect.md`
3. `docs/examples/output/dashboard.txt`
4. `docs/examples/output/goal-brief.md`

Use a terminal width around 96 columns for board/dashboard captures and 100-110 columns for the goal brief.
EOF

cat > "$generated/fixtures/demo-workspace.md" <<'EOF'
# Demo Workspace Fixture

The generator creates a temporary git repository with:

- project `APP`
- ticket `APP-1`
- agent `builder-1`
- one active run normalized as `<RUN-ID>`
- one checkpoint evidence item
- one `test_result` evidence item
- one handoff to `agent:reviewer-1`
- a goal brief and goal manifest for `APP-1`

The temp workspace is deleted after generation. The checked-in examples contain only normalized output.
EOF

leak_pattern='(/Users/|/private/var/|/var/folders/|BEGIN ((RSA|EC|OPENSSH) )?PRIVATE KEY|sk-[A-Za-z0-9]{20,}|ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|mcp_approval_[A-Za-z0-9_]+)'
if command -v rg >/dev/null 2>&1; then
  leak_scan="rg -n"
elif command -v grep >/dev/null 2>&1; then
  leak_scan="grep -R -n -E"
else
  echo "no leak scanner found: install ripgrep or grep" >&2
  exit 1
fi
if $leak_scan "$leak_pattern" "$generated"; then
  echo "generated examples contain local paths or obvious secret material" >&2
  exit 1
fi

if [ "$mode" = "--update" ]; then
  rm -rf "$repo_root/docs/examples"
  mkdir -p "$repo_root/docs/examples"
  (cd "$generated" && tar cf - .) | (cd "$repo_root/docs/examples" && tar xf -)
  echo "updated docs/examples"
else
  if ! diff -ru "$repo_root/docs/examples" "$generated"; then
    echo "docs/examples is stale; run sh examples/generate-demo-assets.sh --update" >&2
    exit 1
  fi
  echo "docs/examples is fresh"
fi
