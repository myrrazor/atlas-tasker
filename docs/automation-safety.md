# Automation Safety Contract

Automation in base v1.3 is intentionally constrained.

## Safe built-in actions
- add comment
- move status
- request review
- notify

## Explicitly deferred
- arbitrary local command execution
- shell interpolation
- background daemon requirements
- remote execution

## Expected safety rules
- dry-run never mutates
- explain output matches real evaluation
- automation chains are traceable through event metadata
- loop prevention is mandatory before release
