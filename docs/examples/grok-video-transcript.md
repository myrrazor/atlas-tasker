# Grok Build walkthrough transcript

Recorded on September 14, 2026 in Grok Build 4.6 at xhigh effort, using synthetic Example App tickets and the v1.15 source. The [walkthrough video](../../site/assets/grok-flow.mp4) keeps the original chronological output and shortens pauses. There is no audio.

The session uses Atlas MCP to read the eight-ticket board, create the requested high-priority task, and confirm APP-9 on the nine-ticket board. Below are the actual prompts and answers; tool entries identify the Atlas operations between them.

## User

How are the Example App tickets looking?

## Assistant

I'll pull the Example App board from Atlas and summarize current ticket status.

## Tools

- IntegrationSearch (MCP tool discovery)
- IntegrationSearch (MCP tool discovery)

## Assistant

I'll load workspace context and the Example App board next so the status table is current.

## Tools

- UseTool: atlas_status
- UseTool: atlas_context
- IntegrationSearch (MCP tool discovery)

## Assistant

Example App is project `APP`. I'll pull that project's board so the table is complete.

## Tools

- UseTool: atlas_status
- UseTool: atlas_board

## Assistant

Example App (`APP`) has **8 tickets**. Active work is the high-priority upgrade test and a critical import-retry review; one ready docs ticket is holding a blocked walkthrough.

| Status | Ticket | Priority | Title |
|---|---|---|---|
| in_review | APP-4 | critical | Review the import retry fix |
| in_progress | APP-3 | high | Test the release upgrade |
| ready | APP-2 | high | Document the MCP workflow |
| blocked | APP-5 | medium | Publish the setup walkthrough |
| backlog | APP-1 | medium | Add keyboard shortcuts |
| backlog | APP-7 | medium | Keep example commands current |
| done | APP-6 | low | Add the health endpoint |
| canceled | APP-8 | low | Retire the unused import prototype |

**APP-4** is waiting on review (`agent:reviewer-1`). **APP-3** is assigned to `agent:builder-1`. **APP-2** is the next ready item and is assigned to `human:owner`; **APP-5** stays blocked until that docs ticket clears. **APP-1** and **APP-7** are not ready yet. **APP-6** is complete; **APP-8** was canceled.

## User

Add a high-priority task to Example App called "Add dark-mode onboarding screenshots", then show the updated board.

## Assistant

I'll create the high-priority Example App task, then re-read the board to confirm it landed.

## Tools

- UseTool: atlas_ticket_create
- UseTool: atlas_ticket_view
- UseTool: atlas_board

## Assistant

Created **APP-9** as a high-priority task on Example App. It is in **backlog**.

| Status | Ticket | Priority | Title |
|---|---|---|---|
| in_review | APP-4 | critical | Review the import retry fix |
| in_progress | APP-3 | high | Test the release upgrade |
| ready | APP-2 | high | Document the MCP workflow |
| blocked | APP-5 | medium | Publish the setup walkthrough |
| backlog | APP-9 | high | Add dark-mode onboarding screenshots |
| backlog | APP-1 | medium | Add keyboard shortcuts |
| backlog | APP-7 | medium | Keep example commands current |
| done | APP-6 | low | Add the health endpoint |
| canceled | APP-8 | low | Retire the unused import prototype |

Example App now has **9 tickets**. Ready work is still **APP-2**; **APP-5** remains blocked on it. **APP-4** is still in review, **APP-3** is in progress, and the new screenshots task sits in backlog with the other unstarted items.
