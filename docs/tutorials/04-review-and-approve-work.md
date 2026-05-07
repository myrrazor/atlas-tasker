# Tutorial 4: Review And Approve Work

Create a handoff:

```bash
./tracker run handoff <RUN-ID> --next-actor agent:reviewer-1 --next-gate review --actor agent:builder-1 --reason "ready for review"
```

List gates for the run:

```bash
./tracker gate list --run <RUN-ID>
```

Approve or reject the gate:

```bash
./tracker gate approve <GATE-ID> --actor agent:reviewer-1 --reason "reviewed handoff and evidence"
```

Complete the run:

```bash
./tracker run complete <RUN-ID> --summary "Reviewed and complete" --actor agent:builder-1 --reason "tutorial done"
```

If governance policies require signatures, quorum, or separation of duties, use `tracker governance explain` before approving:

```bash
./tracker governance explain APP-1 --action ticket_complete --actor agent:builder-1 --reason "check policy"
```

Next: [use the TUI](05-use-the-tui.md).
