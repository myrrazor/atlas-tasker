# Mentions and Inbox

Mentions are canonical immutable records extracted from supported write surfaces.
Inbox items are derived from mentions, approvals, ownership, handoffs, and sync conflicts.

Mention parsing ignores:
- fenced code blocks
- inline code
- email addresses
- URLs
- file paths
- escaped `@`

Unknown mention targets do not create mention records and produce a warning.
