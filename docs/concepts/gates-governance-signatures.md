# Gates, Governance, And Signatures

Gates are approval checkpoints. Governance policies decide whether protected actions are allowed. Signatures prove artifact authenticity against trusted local keys.

Those are separate jobs:

- a gate can say a reviewer approved a handoff
- governance can say whether the actor, quorum, separation-of-duties, and signature state allow the action
- a signature can say an artifact was signed by a known key

Useful commands:

```bash
tracker gate list --run <RUN-ID>
tracker gate approve <GATE-ID> --actor agent:reviewer-1 --reason "approved evidence"
tracker governance explain APP-1 --action ticket_complete --actor agent:builder-1 --reason "explain completion"
tracker trust status
tracker verify bundle <BUNDLE-ID>
```

Atlas can enforce application-level governance. It does not provide SaaS identity, OS sandboxing, encrypted-at-rest confidentiality, or protection from a malicious local user with filesystem access.
