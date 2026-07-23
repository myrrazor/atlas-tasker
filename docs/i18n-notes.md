# Web i18n notes

The browser welcome page, settings, and board chrome ship in English (`en`), Spanish (`es`), and Indonesian (`id`). Ticket titles, descriptions, comments, event payloads, actors, labels, and audit reasons stay exactly as stored.

Language resolution is intentionally small: `?lang=` wins, then `web.lang`, then `Accept-Language`, then English. Set a workspace default with:

```bash
tracker config set web.lang es
```

The status vocabulary is fixed for the pilot so the catalog stays consistent:

| Atlas term | Spanish | Indonesian |
|---|---|---|
| Backlog | Pendiente | Antrean |
| Ready | Listo | Siap |
| In progress | En curso | Dikerjakan |
| In review | En revisión | Ditinjau |
| Blocked | Bloqueado | Terhambat |
| Done | Finalizado | Selesai |
| Gate | Control | Gate |

Spanish keeps “ticket,” the common concise term in technical teams. Indonesian uses “tiket.” “Gate” stays as product jargon in Indonesian; Spanish uses “control” where it reads more naturally.
