# Tutorial 4: Review And Approve Work

Create a handoff:

```bash
./tracker run handoff "$RUN_ID" --next-actor agent:reviewer-1 --next-gate review --actor agent:builder-1 --reason "ready for review"
```

Capture the review gate:

```bash
GATE_JSON="$(mktemp)"
./tracker gate list --run "$RUN_ID" --json > "$GATE_JSON"
GATE_ID="$(python3 -c 'import json,sys; items=json.load(open(sys.argv[1], encoding="utf-8"))["items"]; print(items[0]["gate_id"] if items else "")' "$GATE_JSON")"
echo "$GATE_ID"
```

Approve or reject the gate:

```bash
./tracker gate approve "$GATE_ID" --actor agent:reviewer-1 --reason "reviewed handoff and evidence"
```

Complete the run and close the ticket:

```bash
./tracker run complete "$RUN_ID" --summary "Reviewed and complete" --actor agent:builder-1 --reason "tutorial run done"
./tracker ticket request-review APP-1 --actor agent:builder-1 --reason "ready for final review"
./tracker ticket approve APP-1 --actor human:owner --reason "tutorial approved"
./tracker ticket view APP-1 --json
# This tutorial uses the default open mode, so approval leaves the ticket in_review.
./tracker ticket complete APP-1 --actor human:owner --reason "tutorial done"
```

No reviewer is configured on this tutorial ticket, so only the assignee, active worker, or `human:owner` may approve — `agent:reviewer-1` would be rejected here. Set one with `tracker ticket policy set --required-reviewer` when you want a specific reviewer to hold that power.
With `review_gate`, reviewer approval also moves the ticket to `done`; inspect the returned status and
do not run `ticket complete` a second time. The final command above applies to this tutorial's
default `open` completion policy.

If governance policies require signatures, quorum, or separation of duties, use `tracker governance explain` before approving:

```bash
./tracker governance explain APP-1 --action ticket_complete --actor agent:builder-1 --reason "check policy"
```

Next: [use the TUI](05-use-the-tui.md).
