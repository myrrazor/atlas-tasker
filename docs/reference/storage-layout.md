# Storage Layout

Workspace state is local to the repo or directory where Atlas was initialized.

Common paths:

- `.tracker/events/`: event records
- `.tracker/mutations/`: write-ahead mutation records
- `.tracker/index.sqlite`: derived projection database
- `.tracker/runtime/`: generated run/runtime artifacts
- `.tracker/backups/`: local backup snapshots when created

SQLite sidecars such as `.tracker/index.sqlite-wal` and `.tracker/index.sqlite-shm` are local runtime artifacts and should not be committed.
