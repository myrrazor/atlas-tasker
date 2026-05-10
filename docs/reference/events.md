# Events

Atlas records mutations as events and rebuilds derived projections from those records.

Useful commands:

```bash
tracker ticket history APP-1 --json
tracker timeline --json
tracker audit report --scope workspace --actor human:owner --reason "inspect event history" --json
tracker reindex
tracker doctor --repair --json
```

Do not edit event logs manually unless you are doing a controlled recovery with a backup and a written plan.
