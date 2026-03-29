# Upgrade v1.5 to v1.6

v1.6 stays non-destructive, but collaboration requires a deterministic identity stamp before multi-workspace sync is allowed.

Upgrade rules:
- legacy workspaces load with missing v1.6 sync identities
- v1.6 stamps deterministic UIDs locally before first sync
- sync commands refuse with `migration_incomplete` until that stamp completes
- local-only operational state stays local and is not reconstructed by sync

The first post-upgrade validation must prove:
- deterministic UID stamping completed
- no duplicate logical entities appear across independently upgraded workspaces
- archive/restore/compact and doctor/reindex still behave safely after synced history exists
