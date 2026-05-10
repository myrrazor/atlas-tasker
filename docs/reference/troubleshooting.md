# Troubleshooting Reference

Start with [../troubleshooting.md](../troubleshooting.md), then use focused guides:

- [Doctor and repair](../guides/doctor-and-repair.md)
- [Operations](../guides/operations.md)
- [Release verification](../guides/release-verification.md)

Useful commands:

```bash
tracker doctor --json
tracker reindex
tracker inspect <TICKET-ID> --actor human:owner --json
tracker ticket history <TICKET-ID> --json
tracker notify log --json
tracker notify dead-letter --json
```
