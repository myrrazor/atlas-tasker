#!/bin/sh
# Create a synthetic board where Grok Build, Cursor, and Grok Bot share work.
# Never use a personal workspace for screenshots or marketing captures.
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: TRACKER_BIN=/path/to/tracker sh examples/create-multi-agent-demo.sh /path/to/empty-directory" >&2
  exit 2
fi
workspace="$1"
tracker_bin="${TRACKER_BIN:-tracker}"
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

"$tracker_bin" init --skip-integrations --no-backup --no-register --no-open
"$tracker_bin" config set web.owner_name User
"$tracker_bin" config set web.lang en
"$tracker_bin" config set actor.default human:owner
"$tracker_bin" project create APP "Example App"

"$tracker_bin" agent create grok-build \
  --name "Grok Build" \
  --provider custom \
  --capability go \
  --capability docs \
  --notes "Grok Build coding agent on the shared board" \
  --actor human:owner \
  --reason "register Grok Build"
"$tracker_bin" agent create cursor \
  --name "Cursor" \
  --provider custom \
  --capability go \
  --capability tests \
  --notes "Cursor coding agent on the shared board" \
  --actor human:owner \
  --reason "register Cursor"
"$tracker_bin" agent create grok-bot \
  --name "Grok Bot" \
  --provider custom \
  --capability docs \
  --capability review \
  --notes "Grok Bot reviewer on the shared board" \
  --actor human:owner \
  --reason "register Grok Bot"

ticket() {
  "$tracker_bin" ticket create --project APP --title "$1" --status "$2" --priority "$3" --type task --actor human:owner --reason "multi-agent demo setup"
}

ticket "Add dark-mode onboarding screenshots" ready high
ticket "Document the MCP workflow" ready high
ticket "Test the release upgrade" in_progress high
ticket "Review the import retry fix" in_review critical
ticket "Publish the setup walkthrough" blocked medium
ticket "Add the health endpoint" in_progress low
ticket "Keep example commands current" backlog medium
ticket "Retire the unused import prototype" backlog low

"$tracker_bin" ticket move APP-8 canceled --actor human:owner --reason "demo canceled work"
"$tracker_bin" ticket complete APP-6 --actor human:owner --reason "demo completed work"

"$tracker_bin" ticket assign APP-1 agent:grok-build --actor human:owner --reason "Grok Build owns screenshots"
"$tracker_bin" ticket assign APP-3 agent:grok-build --actor human:owner --reason "Grok Build owns the upgrade test"
"$tracker_bin" ticket assign APP-2 agent:cursor --actor human:owner --reason "Cursor owns MCP docs"
"$tracker_bin" ticket assign APP-7 agent:cursor --actor human:owner --reason "Cursor owns command docs"
"$tracker_bin" ticket assign APP-4 agent:grok-bot --actor human:owner --reason "Grok Bot owns the review"
"$tracker_bin" ticket assign APP-5 agent:grok-bot --actor human:owner --reason "Grok Bot owns the walkthrough"

"$tracker_bin" ticket edit APP-4 \
  --reviewer agent:grok-bot \
  --description "Retry an interrupted local import without creating duplicate tickets." \
  --acceptance "A second import leaves the ticket count unchanged." \
  --acceptance "The failure path is covered by a regression test." \
  --actor human:owner \
  --reason "demo review details"
"$tracker_bin" ticket edit APP-1 \
  --description "Capture dark-mode onboarding screens for the README and marketing site." \
  --acceptance "Desktop and mobile frames use the current dark UI." \
  --actor human:owner \
  --reason "demo screenshot details"
"$tracker_bin" ticket edit APP-2 \
  --description "Document how Cursor, Grok Build, and Grok Bot share one Atlas board." \
  --acceptance "The guide names claim, assign, and review commands." \
  --actor human:owner \
  --reason "demo docs details"

"$tracker_bin" ticket claim APP-3 --actor agent:grok-build --reason "start upgrade test"
"$tracker_bin" ticket claim APP-2 --actor agent:cursor --reason "start MCP docs"
"$tracker_bin" ticket claim APP-4 --actor agent:grok-bot --reason "start review"

"$tracker_bin" ticket comment APP-3 --body "Grok Build: upgrade fixture is green on v1.15.0. Recording the smoke log next." --actor agent:grok-build --reason "progress note"
"$tracker_bin" ticket comment APP-2 --body "Cursor: drafting the shared-board section with assign and claim examples." --actor agent:cursor --reason "progress note"
"$tracker_bin" ticket comment APP-4 --body "Grok Bot: review started. Checking the retry test and the duplicate-import path." --actor agent:grok-bot --reason "progress note"
"$tracker_bin" ticket comment APP-1 --body "Queued behind the upgrade test. I will capture the board once APP-3 lands." --actor agent:grok-build --reason "queue note"

"$tracker_bin" ticket link APP-5 --blocked-by APP-2 --actor human:owner --reason "walkthrough needs the MCP guide"
"$tracker_bin" ticket move APP-2 in_progress --actor agent:cursor --reason "start implementation"

echo "Multi-agent demo ready. From this directory run:"
echo "  $tracker_bin board"
echo "  $tracker_bin agent available grok-build"
echo "  $tracker_bin agent available cursor"
echo "  $tracker_bin agent available grok-bot"
echo "  $tracker_bin web serve --no-browser"
