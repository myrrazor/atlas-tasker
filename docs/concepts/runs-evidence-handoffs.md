# Runs, Evidence, And Handoffs

A run is Atlas' record that a specific actor or agent is working on a ticket. Runs keep launch context, worktree/runtime paths, checkpoints, evidence, handoffs, gates, and completion state tied to the ticket.

Evidence is durable proof attached to a run. It can be a note or a copied artifact file. Handoffs are immutable markdown snapshots that tell the next actor what happened, what is risky, and what gate or status should come next.

Typical flow:

```bash
git status --short
tracker run dispatch APP-1 --agent builder-1 --actor human:owner --reason "start implementation"
tracker run checkpoint <RUN-ID> --title "First pass" --body "Implementation started." --actor agent:builder-1 --reason "checkpoint"
tracker run evidence add <RUN-ID> --type note --title "Test proof" --body "go test ./... passed" --actor agent:builder-1 --reason "attach proof"
tracker run handoff <RUN-ID> --next-actor agent:reviewer-1 --next-gate review --actor agent:builder-1 --reason "ready for review"
```

The status check should be clean before dispatch.

Runs do not replace the ticket. They explain how a specific attempt moved the ticket forward.
