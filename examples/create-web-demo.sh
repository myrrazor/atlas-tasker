#!/bin/sh
# Create synthetic UI capture data. Never use a personal workspace for screenshots.
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: TRACKER_BIN=/path/to/tracker sh examples/create-web-demo.sh /path/to/empty-directory" >&2
  exit 2
fi
workspace="$1"
tracker_bin="${TRACKER_BIN:-tracker}"
demo_date="${DEMO_DATE:-$(date +%Y-%m-%d)}"
if ! command -v "$tracker_bin" >/dev/null 2>&1; then
  echo "tracker is not available; build it and set TRACKER_BIN to its absolute path" >&2
  exit 2
fi
if [ -L "$workspace" ] || { [ -e "$workspace" ] && { [ ! -d "$workspace" ] || [ -n "$(ls -A "$workspace")" ]; }; }; then
  echo "refusing to change a non-empty demo destination" >&2
  exit 2
fi
mkdir -p "$workspace"
cd "$workspace"

"$tracker_bin" init --skip-integrations
"$tracker_bin" config set web.owner_name User
"$tracker_bin" config set web.lang en
"$tracker_bin" config set actor.default human:owner
"$tracker_bin" project create APP "Example App"
"$tracker_bin" project create OPS "Release Checks"
"$tracker_bin" agent create builder-1 --name Builder --provider codex --capability go --actor human:owner --reason "synthetic demo setup"
"$tracker_bin" agent create reviewer-1 --name Reviewer --provider claude --capability go --actor human:owner --reason "synthetic demo setup"

ticket() {
  "$tracker_bin" ticket create --project "$1" --title "$2" --status "$3" --priority "$4" --type task --actor human:owner --reason "synthetic demo setup"
}

ticket APP "Add keyboard shortcuts" backlog medium
ticket APP "Document the MCP workflow" ready high
ticket APP "Test the release upgrade" in_progress high
ticket APP "Review the import retry fix" in_review critical
ticket APP "Publish the setup walkthrough" blocked medium
ticket APP "Add the health endpoint" in_progress low
ticket APP "Keep example commands current" backlog medium
ticket OPS "Verify release checksums" ready high
ticket OPS "Record install evidence" backlog medium

"$tracker_bin" ticket assign APP-3 agent:builder-1 --actor human:owner --reason "synthetic demo assignment"
"$tracker_bin" ticket assign APP-4 agent:builder-1 --actor human:owner --reason "synthetic demo assignment"
"$tracker_bin" ticket edit APP-4 --reviewer agent:reviewer-1 --description "Retry an interrupted local import without creating duplicate tickets." --acceptance "A second import leaves the ticket count unchanged." --acceptance "The failure path is covered by a regression test." --actor human:owner --reason "synthetic demo details"
"$tracker_bin" ticket comment APP-4 --body "Synthetic demo: the retry test passes and the change is ready for review." --actor agent:builder-1 --reason "synthetic demo progress"
"$tracker_bin" ticket link APP-5 --blocked-by APP-2 --actor human:owner --reason "the walkthrough needs the MCP guide"

"$tracker_bin" schedule set APP-6 --at "${demo_date}T12:00:00Z" --runner human:owner --actor human:owner --reason "synthetic demo schedule"
"$tracker_bin" ticket complete APP-6 --actor human:owner --reason "synthetic demo completed work"
"$tracker_bin" schedule set APP-3 --at "${demo_date}T13:00:00Z" --runner agent:builder-1 --actor human:owner --reason "synthetic demo schedule"
"$tracker_bin" schedule set APP-2 --at "${demo_date}T15:00:00Z" --runner human:owner --actor human:owner --reason "synthetic demo schedule"
"$tracker_bin" schedule set OPS-1 --at "${demo_date}T17:00:00Z" --runner human:owner --actor human:owner --reason "synthetic demo schedule"

echo "Synthetic demo ready. Run tracker web serve --no-browser from this directory."
