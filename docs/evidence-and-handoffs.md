# Evidence and Handoffs

Evidence types:
- `note`
- `test_result`
- `file_diff_summary`
- `log_excerpt`
- `screenshot`
- `artifact_ref`
- `commit_ref`
- `manual_assertion`
- `unresolved_question`
- `review_checklist`

Rules:
- max inline body size: 64 KiB
- max artifact size: 10 MiB
- evidence is immutable in v1.4
- supersession is explicit via `supersedes_evidence_id`
- cleanup never removes evidence or handoff history

Handoff packets include:
- source run
- ticket
- actor
- status summary
- changed files
- commit refs
- tests
- evidence links
- open questions
- risks
- suggested next actor
- suggested next gate
- suggested next ticket status
- generated timestamp
- schema version
