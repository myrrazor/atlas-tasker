# Permission Profiles

## Evaluation order
1. workspace default
2. project default
3. agent binding
4. runbook binding
5. ticket protection overlay
6. explicit owner override

## Rules
- explicit deny beats explicit allow
- owner override is a recorded one-shot event, never a silent bypass
- path scopes normalize to repo-root-relative slash paths
- path matching uses one documented glob implementation everywhere

## Enforcement checkpoints
- dispatch
- run launch
- change create
- merge
- gate open
- gate approve
- run completion
- ticket completion

## Path restrictions
Path restrictions are surfaced before work begins and enforced where Atlas can verify changed files.
If Atlas cannot verify the relevant changed-file set for a protected action, it blocks with `unverifiable_path_scope`.

## Explainer requirement
`tracker permissions view <TARGET>` must show:
- the ordered profiles that applied
- the effective allow/deny result
- the checkpoint being evaluated
- the exact reason codes for any block or owner-override requirement

## Command inventory
- `tracker permission-profile list`
- `tracker permission-profile view <PROFILE-ID>`
- `tracker permission-profile create <PROFILE-ID>`
- `tracker permission-profile edit <PROFILE-ID>`
- `tracker permission-profile bind <PROFILE-ID>`
- `tracker permission-profile unbind <PROFILE-ID>`
- `tracker permissions view <TARGET>`

## Current implementation notes
- project binding updates both the profile metadata and the project's default permission-profile list so dispatch and explain surfaces stay in sync after reindex
- direct ticket bindings live on the ticket snapshot and win late in the evaluation chain, after workspace, project, agent, and runbook layers
- dispatch evaluates the operator actor for authority and the selected agent for capability-gated profiles
