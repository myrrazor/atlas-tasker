# Web i18n notes

The browser welcome page, settings, and board chrome ship in English (`en`), Spanish (`es`), Indonesian (`id`), Chinese (`zh`), Japanese (`ja`), and Korean (`ko`). Ticket titles, descriptions, comments, event payloads, actors, labels, and audit reasons stay exactly as stored.

Language resolution is intentionally small: `?lang=` wins, then `web.lang`, then `Accept-Language`, then English. Set a workspace default with:

```bash
tracker config set web.lang es
```

Two known gaps, on purpose rather than by accident: config validation for `web.lang` still only accepts `en`, `es`, and `id`, so the newer three catalogs are reachable through `?lang=` and `Accept-Language` but not yet as a workspace default. And the `/schedule` page is English-only for now — its strings are not in the catalogs, and it does not render the footer language switcher.

The status vocabulary is fixed for the pilot so the catalog stays consistent:

| Atlas term | Spanish | Indonesian | Chinese | Japanese | Korean |
|---|---|---|---|---|---|
| Backlog | Pendiente | Antrean | 待办 | バックログ | 백로그 |
| Ready | Listo | Siap | 就绪 | 準備完了 | 준비됨 |
| In progress | En curso | Dikerjakan | 进行中 | 進行中 | 진행 중 |
| In review | En revisión | Ditinjau | 评审中 | レビュー中 | 리뷰 중 |
| Blocked | Bloqueado | Terhambat | 已阻塞 | ブロック中 | 차단됨 |
| Done | Finalizado | Selesai | 已完成 | 完了 | 완료 |
| Gate | Control | Gate | 关卡 | ゲート | 게이트 |

Spanish keeps “ticket,” the common concise term in technical teams. Indonesian uses “tiket,” Chinese uses “工单,” and Japanese and Korean transliterate (チケット, 티켓). “Gate” stays as product jargon in Indonesian; Spanish uses “control” where it reads more naturally.
